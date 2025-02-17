package caches

import (
	"context"
	"fmt"
	"sync"
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
	tickHandler           func(ctx context.Context)
}

// cache key type aliases
type resourceTargetRelation string

type projectAuthzCache struct {
	schemaCache           *descope.FGASchema                                   // schema
	directRelationCache   *lru.MonitoredLRUCache[resourceTargetRelation, bool] // resource:target:relation -> bool, e.g. file1:user2:owner -> false
	directResourcesIndex  map[string][]resourceTargetRelation
	directTargetsIndex    map[string][]resourceTargetRelation
	indirectRelationCache *lru.MonitoredLRUCache[resourceTargetRelation, bool] // resource:target:relation -> bool, e.g. file1:user2:owner -> false
	remoteChanges         *remoteChangesTracking
	mutex                 sync.RWMutex
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
	directRelationCache, err := lru.New[resourceTargetRelation, bool](config.GetDirectRelationCacheSizePerProject(), "authz-direct-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	indirectRelationCache, err := lru.New[resourceTargetRelation, bool](config.GetIndirectRelationCacheSizePerProject(), "authz-indirect-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	pc := &projectAuthzCache{
		directRelationCache:   directRelationCache,
		directResourcesIndex:  make(map[string][]resourceTargetRelation),
		directTargetsIndex:    make(map[string][]resourceTargetRelation),
		indirectRelationCache: indirectRelationCache,
		remoteChanges: &remoteChangesTracking{
			lastPollTime:          time.Now(),
			remote:                remoteChangesChecker,
			remotePollingInterval: time.Millisecond * time.Duration(config.GetRemotePollingIntervalInMillis()),
		},
	}
	pc.remoteChanges.tickHandler = pc.updateCacheWithRemotePolling // set the tick handler (to be used in the polling goroutine)
	return pc, nil
}

func (pc *projectAuthzCache) GetSchema() *descope.FGASchema {
	return pc.schemaCache
}

func (pc *projectAuthzCache) CheckRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	if allowed, ok := pc.checkDirectRelation(ctx, r); ok {
		return allowed, true, true
	}
	if allowed, ok := pc.checkIndirectRelation(ctx, r); ok {
		return allowed, false, true
	}
	return false, false, false
}

func (pc *projectAuthzCache) UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.schemaCache = schema
	// on schema update, we need to purge all relations
	pc.indirectRelationCache.Purge(ctx)
	pc.directRelationCache.Purge(ctx)
}

func (pc *projectAuthzCache) UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.indirectRelationCache.Purge(ctx) // added (direct) relations can change the result of indirect checks, so we must purge all indirect relations
	for _, r := range relations {
		pc.addDirectRelation(ctx, r, true)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.indirectRelationCache.Purge(ctx) // deleted (direct) relations can change the result of indirect checks, so we must purge all indirect relations
	for _, r := range relations {
		pc.removeDirectRelation(ctx, r)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck) {
	if len(sdkChecks) == 0 {
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	for _, c := range sdkChecks {
		if c.Info.Direct {
			pc.addDirectRelation(ctx, c.Relation, c.Allowed)
		} else {
			pc.addIndirectRelation(ctx, c.Relation, c.Allowed)
		}
	}
}

func (pc *projectAuthzCache) StartRemoteChangesPolling(ctx context.Context) {
	ticker := time.NewTicker(pc.remoteChanges.remotePollingInterval)
	cctx.Go(ctx, func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				cctx.Logger(ctx).Info().Msg("Remote changes polling stopped due to context cancellation")
				return
			case <-ticker.C:
				pc.remoteChanges.tickHandler(ctx)
			}
		}
	})
}

func (pc *projectAuthzCache) updateCacheWithRemotePolling(ctx context.Context) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	// in case of no cached relations, we skip the server call but invalidate the schema cache to make sure we don't miss schema changes
	if noRelations := pc.directRelationCache.Len(ctx) == 0 && pc.indirectRelationCache.Len(ctx) == 0; noRelations {
		pc.remoteChanges.lastPollTime = time.Now()
		pc.schemaCache = nil
		cctx.Logger(ctx).Debug().Msg("No cached relations, skipping remote changes check")
		return
	}
	// get the changes since the last poll time
	cctx.Logger(ctx).Debug().Msg("Checking for remote changes...")
	remoteChanges, err := pc.fetchRemoteChanges(ctx)
	if err != nil {
		cctx.Logger(ctx).Error().Err(err).Msg("Failed to get new remote changes")
		return
	}
	// if the schema changed, we need to purge all caches
	if remoteChanges.SchemaChanged {
		cctx.Logger(ctx).Info().Msg("Remote changes show schema changed, purging all caches")
		pc.schemaCache = nil
		pc.directRelationCache.Purge(ctx)
		pc.indirectRelationCache.Purge(ctx)
		return
	}
	// update the local cache only if there are missing remote changes
	if len(remoteChanges.Resources) <= 0 && len(remoteChanges.Targets) <= 0 {
		cctx.Logger(ctx).Debug().Msg("No new remote changes, skipping cache update")
		return
	}
	cctx.Logger(ctx).Debug().Msg(fmt.Sprintf("Remote changes found, Resources: %v, Targets: %v, updating caches", remoteChanges.Resources, remoteChanges.Targets))
	pc.indirectRelationCache.Purge(ctx)
	for _, r := range remoteChanges.Resources {
		pc.removeDirectRelationByResource(ctx, r)
	}
	for _, t := range remoteChanges.Targets {
		pc.removeDirectRelationByTarget(ctx, t)
	}
}

func (pc *projectAuthzCache) removeDirectRelationByResource(ctx context.Context, r string) {
	keys := pc.directResourcesIndex[r]
	for _, k := range keys {
		pc.directRelationCache.Remove(ctx, k)
	}
	delete(pc.directResourcesIndex, r)
}

func (pc *projectAuthzCache) removeDirectRelationByTarget(ctx context.Context, t string) {
	keys := pc.directTargetsIndex[t]
	for _, k := range keys {
		pc.directRelationCache.Remove(ctx, k)
	}
	delete(pc.directTargetsIndex, t)
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
	key := key(r)
	pc.directRelationCache.Add(ctx, key, isAllowed)
	pc.directResourcesIndex[r.Resource] = append(pc.directResourcesIndex[r.Resource], key)
	pc.directTargetsIndex[r.Target] = append(pc.directTargetsIndex[r.Target], key)
}

func (pc *projectAuthzCache) addIndirectRelation(ctx context.Context, r *descope.FGARelation, isAllowed bool) {
	key := key(r)
	pc.indirectRelationCache.Add(ctx, key, isAllowed)
}

func (pc *projectAuthzCache) removeDirectRelation(ctx context.Context, r *descope.FGARelation) {
	key := key(r)
	pc.directRelationCache.Remove(ctx, key)
	// iterate over the keys saved in the indexes and remove the key from the indexes slices
	for i, k := range pc.directResourcesIndex[r.Resource] {
		if k == key {
			pc.directResourcesIndex[r.Resource] = append(pc.directResourcesIndex[r.Resource][:i], pc.directResourcesIndex[r.Resource][i+1:]...)
			break
		}
	}
	for i, k := range pc.directTargetsIndex[r.Target] {
		if k == key {
			pc.directTargetsIndex[r.Target] = append(pc.directTargetsIndex[r.Target][:i], pc.directTargetsIndex[r.Target][i+1:]...)
			break
		}
	}
}

func (pc *projectAuthzCache) checkDirectRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
	key := key(r)
	if allowed, ok := pc.directRelationCache.Get(ctx, key); ok {
		return allowed, true
	}
	return false, false
}

func (pc *projectAuthzCache) checkIndirectRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
	key := key(r)
	if allowed, ok := pc.indirectRelationCache.Get(ctx, key); ok {
		return allowed, true
	}
	return false, false
}

func key(r *descope.FGARelation) resourceTargetRelation {
	return resourceTargetRelation(r.Resource + ":" + r.Target + ":" + r.Relation)
}
