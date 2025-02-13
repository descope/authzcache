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

type remoteChangesTracking struct {
	lastPollTime          time.Time
	remotePollingInterval time.Duration
	remote                RemoteChangesChecker
}

// cache key type aliases
type relation string
type resourceTarget string
type resourceTargetRelation string

type projectAuthzCache struct {
	schemaCache           *descope.FGASchema                                        // schema
	directRelationCache   *lru.MonitoredLRUCache[resourceTarget, map[relation]bool] // resource:target -> map[relation]bool, e.g. p1:file1:user2 -> {owner->false, read->true}
	indirectRelationCache *lru.MonitoredLRUCache[resourceTargetRelation, bool]      // resource:target:relation -> bool, e.g. p1:file1:user2:owner -> false
	remoteChanges         *remoteChangesTracking
}

type ProjectAuthzCache interface {
	GetSchema() *descope.FGASchema
	CheckRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool)
	UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema)
	UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck)
	StartRemoteChangesPolling(ctx context.Context)
}

var _ ProjectAuthzCache = &projectAuthzCache{} // ensure projectAuthzCache implements ProjectAuthzCache

func NewProjectAuthzCache(ctx context.Context, remoteChangesChecker RemoteChangesChecker) (ProjectAuthzCache, error) {
	directRelationCache, err := lru.New[resourceTarget, map[relation]bool](config.GetDirectRelationCacheSizePerProject(), "authz-direct-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	indirectRelationCache, err := lru.New[resourceTargetRelation, bool](config.GetIndirectRelationCacheSizePerProject(), "authz-indirect-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	pc := &projectAuthzCache{
		directRelationCache:   directRelationCache,
		indirectRelationCache: indirectRelationCache,
		remoteChanges: &remoteChangesTracking{
			lastPollTime:          time.Now(),
			remote:                remoteChangesChecker,
			remotePollingInterval: time.Millisecond * time.Duration(config.GetRemotePollingIntervalInMillis()),
		},
	}
	return pc, nil
}

func (pc *projectAuthzCache) GetSchema() *descope.FGASchema {
	return pc.schemaCache
}

func (pc *projectAuthzCache) CheckRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
	if allowed, ok := pc.checkDirectRelation(ctx, r); ok {
		return allowed, true, true
	}
	if allowed, ok := pc.checkIndirectRelation(ctx, r); ok {
		return allowed, false, true
	}
	return false, false, false
}

// TODO: think about concurrent updates, might need to add locks and stuff since go maps are not safe for concurrent reads/writes
func (pc *projectAuthzCache) UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema) {
	pc.schemaCache = schema
	// on schema update, we need to purge all relations
	pc.indirectRelationCache.Purge(ctx)
	pc.directRelationCache.Purge(ctx)
}

func (pc *projectAuthzCache) UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.indirectRelationCache.Purge(ctx) // added (direct) relations can change the result of indirect checks, so we must purge all indirect relations
	for _, r := range relations {
		pc.addDirectRelation(ctx, r, true)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.indirectRelationCache.Purge(ctx) // deleted (direct) relations can change the result of indirect checks, so we must purge all indirect relations
	for _, r := range relations {
		pc.removeDirectRelation(ctx, r)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck) {
	if len(sdkChecks) == 0 {
		return
	}
	for _, c := range sdkChecks {
		if c.Info.Direct {
			pc.addDirectRelation(ctx, c.Relation, c.Allowed)
		} else {
			pc.addIndirectRelation(ctx, c.Relation, c.Allowed)
		}
	}
}

func (pc *projectAuthzCache) StartRemoteChangesPolling(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				cctx.Logger(ctx).Info().Msg("Remote changes polling stopped due to context cancellation")
				return
			case <-time.After(pc.remoteChanges.remotePollingInterval):
				// in case of no cached relations, we skip the server call but invalidate the schema cache to make sure we don't miss schema changes
				if noRelations := pc.directRelationCache.Len(ctx) == 0 && pc.indirectRelationCache.Len(ctx) == 0; noRelations {
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
					pc.indirectRelationCache.Purge(ctx)
					continue
				}
				// update the local cache only if there are missing remote changes
				if len(remoteChanges.Resources) > 0 || len(remoteChanges.Targets) > 0 {
					cctx.Logger(ctx).Debug().Msg(fmt.Sprintf("Remote changes found, Resources: %v, Targets: %v, updating caches", remoteChanges.Resources, remoteChanges.Targets))
					pc.indirectRelationCache.Purge(ctx)
					for _, r := range remoteChanges.Resources {
						for _, t := range remoteChanges.Targets {
							pc.directRelationCache.Remove(ctx, directKey(&descope.FGARelation{Resource: r, Target: t}))
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

func (pc *projectAuthzCache) addDirectRelation(ctx context.Context, r *descope.FGARelation, isAllowed bool) {
	key := directKey(r)
	relations, ok := pc.directRelationCache.Get(ctx, key)
	if ok {
		relations[relationKey(r)] = isAllowed
		return
	}
	pc.directRelationCache.Add(ctx, key, map[relation]bool{relationKey(r): isAllowed})
}

func (pc *projectAuthzCache) addIndirectRelation(ctx context.Context, r *descope.FGARelation, isAllowed bool) {
	key := indirectKey(r)
	pc.indirectRelationCache.Add(ctx, key, isAllowed)
}

func (pc *projectAuthzCache) removeDirectRelation(ctx context.Context, r *descope.FGARelation) {
	key := directKey(r)
	relations, ok := pc.directRelationCache.Get(ctx, key)
	if !ok {
		return // nothing to remove
	}
	delete(relations, relationKey(r))
}

func (pc *projectAuthzCache) checkDirectRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
	key := directKey(r)
	if relations, ok := pc.directRelationCache.Get(ctx, key); ok {
		if allowed, ok := relations[relationKey(r)]; ok {
			return allowed, true
		}
	}
	return false, false
}

func (pc *projectAuthzCache) checkIndirectRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
	key := indirectKey(r)
	if allowed, ok := pc.indirectRelationCache.Get(ctx, key); ok {
		return allowed, true
	}
	return false, false
}

func relationKey(r *descope.FGARelation) relation {
	return relation(r.Relation)
}

func directKey(r *descope.FGARelation) resourceTarget {
	return resourceTarget(r.Resource + ":" + r.Target)
}

func indirectKey(r *descope.FGARelation) resourceTargetRelation {
	return resourceTargetRelation(r.Resource + ":" + r.Target + ":" + r.Relation)
}
