package caches

import (
	"context"
	"fmt"
	"time"

	"github.com/descope/authzcache/internal/config"
	cctx "github.com/descope/common/pkg/common/context"
	lru "github.com/descope/common/pkg/common/utils/monitoredlru"
	"github.com/descope/go-sdk/descope"
)

type RemoteChangesChecker interface {
	GetModified(ctx context.Context, since time.Time) (*descope.AuthzModified, error)
}

type resourceTargetKey string
type resourceTargetRelationKey string

type remoteChangesTracking struct {
	lastPollTime          time.Time
	remotePollingInterval time.Duration
	remote                RemoteChangesChecker
}

type projectAuthzCache struct {
	schemaCache                     *descope.FGASchema                                      // schema
	directRelationCache             *lru.MonitoredLRUCache[resourceTargetKey, []string]     // resource:target -> [relation, relation, ...], e.g. p1:file1:user1 -> [owner, reader]
	indirectOrNegativeRelationCache *lru.MonitoredLRUCache[resourceTargetRelationKey, bool] // resource:target:relation -> true/false, e.g. p1:file1:user2:owner -> false
	remoteChanges                   *remoteChangesTracking
}

type ProjectAuthzCache interface {
	GetSchema() *descope.FGASchema
	CheckRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool)
	UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema)
	UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck)
}

var _ ProjectAuthzCache = &projectAuthzCache{} // ensure projectAuthzCache implements ProjectAuthzCache

type ProjectAuthzCacheCreator struct{}

func (p ProjectAuthzCacheCreator) NewProjectAuthzCache(ctx context.Context, remoteChangesChecker RemoteChangesChecker) (ProjectAuthzCache, error) {
	directRelationCache, err := lru.New[resourceTargetKey, []string](config.GetDirectRelationCacheSizePerProject(), "authz-direct-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err
	}
	indirectOrNegativeRelationCache, err := lru.New[resourceTargetRelationKey, bool](config.GetInderectAndNegativeRelationCacheSizePerProject(), "authz-indirect-or-negative-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err
	}
	pc := &projectAuthzCache{
		directRelationCache:             directRelationCache,
		indirectOrNegativeRelationCache: indirectOrNegativeRelationCache,
		remoteChanges: &remoteChangesTracking{
			lastPollTime:          time.Now(),
			remote:                remoteChangesChecker,
			remotePollingInterval: time.Second * time.Duration(config.GetRemotePollingIntervalInSeconds()),
		},
	}
	pc.invalidateCacheOnRemoteChanges(ctx)
	return pc, nil
}

func (pc *projectAuthzCache) GetSchema() *descope.FGASchema {
	return pc.schemaCache
}

func (pc *projectAuthzCache) CheckRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
	if pc.checkDirectRelation(ctx, r) {
		return true, true
	}
	if allowed, ok := pc.checkIndirectOrNegativeRelation(ctx, r); ok {
		return allowed, ok
	}
	return false, false
}

func (pc *projectAuthzCache) UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema) {
	pc.schemaCache = schema
	// on schema update, we need to purge all relations
	pc.indirectOrNegativeRelationCache.Purge(ctx)
	pc.directRelationCache.Purge(ctx)
}

func (pc *projectAuthzCache) UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.indirectOrNegativeRelationCache.Purge(ctx) // added relations can change the result of indirect/negative checks
	for _, r := range relations {
		pc.addDirectRelation(ctx, r)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.indirectOrNegativeRelationCache.Purge(ctx) // deleted relations can change the result of indirect/negative checks
	for _, r := range relations {
		pc.removeDirectRelation(ctx, r)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck) {
	if len(sdkChecks) == 0 {
		return
	}
	for _, c := range sdkChecks {
		if !c.Allowed || !c.Direct {
			pc.addIndirectOrNegativeRelation(ctx, c.Relation, c.Allowed)
		} else {
			pc.addDirectRelation(ctx, c.Relation)
		}
	}
}

func (pc *projectAuthzCache) invalidateCacheOnRemoteChanges(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pc.remoteChanges.remotePollingInterval):
				// in case of no cached relations, we skip the server call but invalidate the schema cache to make sure we don't miss schema changes
				if noRelations := pc.directRelationCache.Len(ctx) == 0 && pc.indirectOrNegativeRelationCache.Len(ctx) == 0; noRelations {
					pc.remoteChanges.lastPollTime = time.Now()
					pc.schemaCache = nil
					cctx.Logger(ctx).Debug().Msg("No cached relations, skipping remote changes check")
					continue
				}
				// get the changes since the last poll time
				cctx.Logger(ctx).Debug().Msg("Checking for remote changes...")
				remoteChanges, err := pc.fetchRemoteChanges(ctx)
				if err != nil {
					cctx.Logger(ctx).Error().Err(err).Msg("Failed to get new remote changes")
					continue
				}
				// if the schema changed, we need to purge all caches
				if remoteChanges.SchemaChanged {
					cctx.Logger(ctx).Info().Msg("Remote changes show schema changed, purging all caches")
					pc.schemaCache = nil
					pc.directRelationCache.Purge(ctx)
					pc.indirectOrNegativeRelationCache.Purge(ctx)
					continue
				}
				// update the local cache only if there are missing remote changes
				if len(remoteChanges.Resources) > 0 || len(remoteChanges.Targets) > 0 {
					cctx.Logger(ctx).Debug().Msg(fmt.Sprintf("Remote changes found, Resources: %v, Targets: %v, updating caches", remoteChanges.Resources, remoteChanges.Targets))
					pc.indirectOrNegativeRelationCache.Purge(ctx)
					for _, r := range remoteChanges.Resources {
						for _, t := range remoteChanges.Targets {
							pc.directRelationCache.Remove(ctx, directRelationKey(&descope.FGARelation{Resource: r, Target: t}))
						}
					}
				} else {
					cctx.Logger(ctx).Debug().Msg("No new remote changes, skipping cache update")
				}
				continue
			}
		}
	}()
}

func (pc *projectAuthzCache) fetchRemoteChanges(ctx context.Context) (*descope.AuthzModified, error) {
	// get the current time BEFORE polling to use as the last poll time after the poll
	currPollTime := time.Now()
	// get changes from remote since last poll time
	remoteChanges, err := pc.remoteChanges.remote.GetModified(ctx, pc.remoteChanges.lastPollTime)
	if err != nil {
		return nil, err
	}
	// update the last poll time
	pc.remoteChanges.lastPollTime = currPollTime
	// return the remote changes
	return remoteChanges, nil
}

func (pc *projectAuthzCache) addDirectRelation(ctx context.Context, r *descope.FGARelation) {
	key := directRelationKey(r)
	relations, ok := pc.directRelationCache.Get(ctx, key)
	if !ok {
		relations = []string{}
	}
	relations = append(relations, r.Relation)
	pc.directRelationCache.Add(ctx, key, relations)
}

func (pc *projectAuthzCache) addIndirectOrNegativeRelation(ctx context.Context, r *descope.FGARelation, isAllowed bool) {
	key := indirectOrNegativeRelationKey(r)
	pc.indirectOrNegativeRelationCache.Add(ctx, key, isAllowed)
}

func (pc *projectAuthzCache) removeDirectRelation(ctx context.Context, r *descope.FGARelation) {
	key := directRelationKey(r)
	relations, ok := pc.directRelationCache.Get(ctx, key)
	if !ok {
		return // nothing to remove
	}
	// overwrite the relations slice without the relation to remove
	for i, relation := range relations {
		if relation == r.Relation {
			relations = append(relations[:i], relations[i+1:]...)
			break
		}
	}
	// update the cache with the new relations slice, or remove the key if there are no relations left
	if len(relations) == 0 {
		pc.directRelationCache.Remove(ctx, key)
	} else {
		pc.directRelationCache.Add(ctx, key, relations)
	}
}

func (pc *projectAuthzCache) checkDirectRelation(ctx context.Context, r *descope.FGARelation) bool {
	key := directRelationKey(r)
	relations, ok := pc.directRelationCache.Get(ctx, key)
	if !ok {
		return false
	}
	for _, relation := range relations {
		if relation == r.Relation {
			return true
		}
	}
	return false
}

func (pc *projectAuthzCache) checkIndirectOrNegativeRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
	key := indirectOrNegativeRelationKey(r)
	if allowed, ok := pc.indirectOrNegativeRelationCache.Get(ctx, key); ok {
		return allowed, ok
	}
	return false, false
}

func directRelationKey(r *descope.FGARelation) resourceTargetKey {
	return resourceTargetKey(r.Resource + ":" + r.Target)
}

func indirectOrNegativeRelationKey(r *descope.FGARelation) resourceTargetRelationKey {
	return resourceTargetRelationKey(r.Resource + ":" + r.Target + ":" + r.Relation)
}
