package caches

import (
	"context"
	"fmt"
	"strings"
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
type resource string
type target string

type projectAuthzCache struct {
	schemaCache           *descope.FGASchema                                   // schema
	directRelationCache   *lru.MonitoredLRUCache[resourceTargetRelation, bool] // resource:target:relation -> bool, e.g. file1:user2:owner -> false
	directResourcesIndex  map[resource]map[target][]resourceTargetRelation
	directTargetsIndex    map[target]map[resource][]resourceTargetRelation
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
	directResourcesIndex := make(map[resource]map[target][]resourceTargetRelation)
	directTargetsIndex := make(map[target]map[resource][]resourceTargetRelation)
	removeIndexOnCacheEviction := func(key resourceTargetRelation, _ bool) {
		// on eviction, we need to remove the keys from the indexes as well
		splitKey := strings.Split(string(key), ":")
		resource, target := resource(splitKey[0]), target(splitKey[1])
		removeKeyFromResourceIndex(directResourcesIndex, resource, target, key)
		removeKeyFromTargetIndex(directTargetsIndex, resource, target, key)
	}
	directRelationCache, err := lru.NewWithEvict[resourceTargetRelation, bool](config.GetDirectRelationCacheSizePerProject(), "authz-direct-relations-"+cctx.ProjectID(ctx), removeIndexOnCacheEviction)
	if err != nil {
		return nil, err // notest
	}
	indirectRelationCache, err := lru.New[resourceTargetRelation, bool](config.GetIndirectRelationCacheSizePerProject(), "authz-indirect-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	pc := &projectAuthzCache{
		directResourcesIndex:  directResourcesIndex,
		directTargetsIndex:    directTargetsIndex,
		directRelationCache:   directRelationCache,
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
		pc.removeDirectRelationByResource(ctx, resource(r))
	}
	for _, t := range remoteChanges.Targets {
		pc.removeDirectRelationByTarget(ctx, target(t))
	}
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
	resource := resource(r.Resource)
	target := target(r.Target)
	pc.addKeyToDirectResourceIndex(key, resource, target)
	pc.addKeyToDirectTargetIndex(key, resource, target)
}

func (pc *projectAuthzCache) addKeyToDirectResourceIndex(k resourceTargetRelation, r resource, t target) {
	if targetsToKeys, ok := pc.directResourcesIndex[r]; ok {
		targetsToKeys[t] = append(targetsToKeys[t], k)
	} else {
		pc.directResourcesIndex[r] = map[target][]resourceTargetRelation{t: append([]resourceTargetRelation{}, k)}
	}
}

func (pc *projectAuthzCache) addKeyToDirectTargetIndex(k resourceTargetRelation, r resource, t target) {
	if resourcesToKeys, ok := pc.directTargetsIndex[t]; ok {
		resourcesToKeys[r] = append(resourcesToKeys[r], k)
	} else {
		pc.directTargetsIndex[t] = map[resource][]resourceTargetRelation{r: append([]resourceTargetRelation{}, k)}
	}
}

func (pc *projectAuthzCache) addIndirectRelation(ctx context.Context, r *descope.FGARelation, isAllowed bool) {
	key := key(r)
	pc.indirectRelationCache.Add(ctx, key, isAllowed)
}

func (pc *projectAuthzCache) removeDirectRelation(ctx context.Context, r *descope.FGARelation) {
	key := key(r)
	pc.directRelationCache.Remove(ctx, key)
	resource := resource(r.Resource)
	target := target(r.Target)
	removeKeyFromResourceIndex(pc.directResourcesIndex, resource, target, key)
	removeKeyFromTargetIndex(pc.directTargetsIndex, resource, target, key)
}

func (pc *projectAuthzCache) removeDirectRelationByResource(ctx context.Context, r resource) {
	targetsToKeys := pc.directResourcesIndex[r]
	for _, keys := range targetsToKeys {
		for _, k := range keys {
			pc.directRelationCache.Remove(ctx, k)
		}
	}
	delete(pc.directResourcesIndex, r)
}

func (pc *projectAuthzCache) removeDirectRelationByTarget(ctx context.Context, t target) {
	resourcesToKeys := pc.directTargetsIndex[t]
	for _, keys := range resourcesToKeys {
		for _, k := range keys {
			pc.directRelationCache.Remove(ctx, k)
		}
	}
	delete(pc.directTargetsIndex, t)
}

func removeKeyFromResourceIndex(index map[resource]map[target][]resourceTargetRelation, r resource, t target, keyToRemove resourceTargetRelation) {
	if targetsToKeys, ok := index[r]; ok {
		keys := targetsToKeys[t]
		for i, k := range keys {
			if k == keyToRemove {
				keys = append(keys[:i], keys[i+1:]...)
				targetsToKeys[t] = keys
				break
			}
		}
		if len(keys) == 0 {
			delete(targetsToKeys, t)
		}
		if len(targetsToKeys) == 0 {
			delete(index, r)
		}
	}
}

func removeKeyFromTargetIndex(index map[target]map[resource][]resourceTargetRelation, r resource, t target, keyToRemove resourceTargetRelation) {
	if resourcesToKeys, ok := index[t]; ok {
		keys := resourcesToKeys[r]
		for i, k := range keys {
			if k == keyToRemove {
				keys = append(keys[:i], keys[i+1:]...)
				resourcesToKeys[r] = keys
				break
			}
		}
		if len(keys) == 0 {
			delete(resourcesToKeys, r)
		}
		if len(resourcesToKeys) == 0 {
			delete(index, t)
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
