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
	// purge after error cooldown tracking
	purgeCooldownWindow time.Duration
	purgeCooldownTimer  *time.Timer // non-nil indicates we're in cooldown state waiting to purge
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
	indirectRelationCache, err := lru.New[resourceTargetRelation, bool](config.GetIndirectRelationCacheSizePerProject(), "authz-indirect-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	pc := &projectAuthzCache{
		directResourcesIndex:  make(map[resource]map[target][]resourceTargetRelation),
		directTargetsIndex:    make(map[target]map[resource][]resourceTargetRelation),
		indirectRelationCache: indirectRelationCache,
		remoteChanges: &remoteChangesTracking{
			lastPollTime:          time.Now(),
			remote:                remoteChangesChecker,
			remotePollingInterval: time.Millisecond * time.Duration(config.GetRemotePollingIntervalInMillis()),
			purgeCooldownWindow:   time.Minute * time.Duration(config.GetPurgeCooldownWindowInMinutes()),
		},
	}
	directRelationCache, err := lru.NewWithEvict[resourceTargetRelation, bool](config.GetDirectRelationCacheSizePerProject(), "authz-direct-relations-"+cctx.ProjectID(ctx), pc.removeIndexOnCacheEviction)
	if err != nil {
		return nil, err // notest
	}
	pc.directRelationCache = directRelationCache
	pc.remoteChanges.tickHandler = pc.updateCacheWithRemotePolling // set the tick handler (to be used in the polling goroutine)
	cctx.Logger(ctx).Info().
		Int("direct_relation_cache_size", config.GetDirectRelationCacheSizePerProject()).
		Int("indirect_relation_cache_size", config.GetIndirectRelationCacheSizePerProject()).
		Int("remote_polling_interval_ms", config.GetRemotePollingIntervalInMillis()).
		Int("purge_cooldown_window_minutes", config.GetPurgeCooldownWindowInMinutes()).
		Msg("Project authz cache initialized")
	return pc, nil
}

func (pc *projectAuthzCache) GetSchema() *descope.FGASchema {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
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
	pc.purgeRelationCaches(ctx)
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
	// if there was an error, purge all caches after cooldown (if configured) or immediately
	if err != nil {
		pc.purgeAfterCooldown(ctx, err)
		return
	}
	// successful response - cancel any pending cooldown
	pc.cancelPurgeCooldown(ctx)
	// if the schema changed, we need to purge all caches
	if remoteChanges.SchemaChanged {
		cctx.Logger(ctx).Info().Msg("Remote changes show schema changed, purging all caches")
		pc.purgeAllCaches(ctx)
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
	pc.removeKeyFromResourceIndex(resource, target, key)
	pc.removeKeyFromTargetIndex(resource, target, key)
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

func (pc *projectAuthzCache) removeKeyFromResourceIndex(r resource, t target, keyToRemove resourceTargetRelation) {
	index := pc.directResourcesIndex
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

func (pc *projectAuthzCache) removeKeyFromTargetIndex(r resource, t target, keyToRemove resourceTargetRelation) {
	index := pc.directTargetsIndex
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

func (pc *projectAuthzCache) removeIndexOnCacheEviction(key resourceTargetRelation, _ bool) {
	// on eviction, we need to remove the keys from the indexes as well
	splitKey := strings.Split(string(key), ":")
	resource, target := resource(splitKey[0]), target(splitKey[1])
	pc.removeKeyFromResourceIndex(resource, target, key)
	pc.removeKeyFromTargetIndex(resource, target, key)
}

// must be called while holding the mutex
func (pc *projectAuthzCache) purgeAllCaches(ctx context.Context) {
	pc.schemaCache = nil
	pc.purgeRelationCaches(ctx)
	// since all caches were purged, we can update the last poll time to the current time
	pc.remoteChanges.lastPollTime = time.Now()
}

func (pc *projectAuthzCache) purgeRelationCaches(ctx context.Context) {
	pc.directResourcesIndex = make(map[resource]map[target][]resourceTargetRelation)
	pc.directTargetsIndex = make(map[target]map[resource][]resourceTargetRelation)
	pc.directRelationCache.Purge(ctx)
	pc.indirectRelationCache.Purge(ctx)
}

// purgeAfterCooldown manages the cooldown logic when an error occurs, must be called while holding the mutex.
func (pc *projectAuthzCache) purgeAfterCooldown(ctx context.Context, err error) {
	// if cooldown window is 0, purge immediately
	if pc.remoteChanges.purgeCooldownWindow == 0 {
		cctx.Logger(ctx).Error().Err(err).Msg("Purging all caches after refresh error immediately (no cooldown configured)")
		pc.purgeAllCaches(ctx)
		return
	}

	// if this is the first error, start the cooldown timer
	if pc.remoteChanges.purgeCooldownTimer == nil {
		cctx.Logger(ctx).Error().Err(err).Dur("cooldown_window", pc.remoteChanges.purgeCooldownWindow).Msg("First error detected, starting cooldown window before purge")

		// start the cooldown timer - use background context as the timer may outlive the current request context
		// Note: The timer callback acquires the mutex and checks that purgeCooldownTimer is not nil to prevent purging if
		// cancelPurgeCooldown() was called in a race condition just before the timer fires (it sets purgeCooldownTimer to nil).
		pc.remoteChanges.purgeCooldownTimer = time.AfterFunc(pc.remoteChanges.purgeCooldownWindow, func() {
			pc.mutex.Lock()
			defer pc.mutex.Unlock()
			// only purge if we're still in error state (not cancelled)
			if pc.remoteChanges.purgeCooldownTimer != nil {
				cctx.Logger(ctx).Error().Dur("cooldown_window", pc.remoteChanges.purgeCooldownWindow).Msg("Cooldown window elapsed, purging all caches")
				pc.purgeAllCaches(ctx)
				// Clean up timer state (don't call cancelPurgeCooldown() since timer already fired)
				pc.remoteChanges.purgeCooldownTimer = nil
			}
		})
	} else {
		// subsequent error during cooldown - log but don't reset timer
		cctx.Logger(ctx).Error().Err(err).Msg("Error detected during cooldown window, continuing to wait before purge")
	}
}

// cancelPurgeCooldown cancels any pending cooldown timer
// This function must be called while holding the mutex.
// It's safe to call even if the timer has already fired or doesn't exist.
func (pc *projectAuthzCache) cancelPurgeCooldown(ctx context.Context) {
	if pc.remoteChanges.purgeCooldownTimer == nil {
		return
	}
	cctx.Logger(ctx).Info().Msg("Successful remote poll during cooldown window, cancelling pending purge")
	pc.remoteChanges.purgeCooldownTimer.Stop()
	pc.remoteChanges.purgeCooldownTimer = nil
}
