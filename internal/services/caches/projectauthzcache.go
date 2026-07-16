package caches

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/descope/authzcache/internal/config"
	edgecel "github.com/descope/authzcache/internal/services/cel"
	cctx "github.com/descope/backend/common/pkg/common/context"
	lru "github.com/descope/backend/common/pkg/common/utils/monitoredlru"
	"github.com/descope/go-sdk/descope"
)

const getModifiedLimitReachedErrorCode = "E173012"

func isTooManyModifiedRelationsError(err error) bool {
	return descope.IsError(err, getModifiedLimitReachedErrorCode)
}

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
type lookupKey string

type keyComponents struct {
	r resource
	t target
}

// LookupCacheEntry holds a cached lookup result with TTL tracking
type LookupCacheEntry struct {
	Results   []string
	CachedAt  time.Time
	ExpiresAt time.Time
}

// conditionalCert holds the condition IDs that evaluated true/false (raw, pre-NOT) on the deciding path;
// a conditional grant stays cacheable while each still re-evaluates to its recorded bucket.
type conditionalCert struct {
	trueConds  []int32
	falseConds []int32
}

type cachedGrant struct {
	allowed bool
	cond    *conditionalCert
}

type projectAuthzCache struct {
	schemaCache *descope.FGASchema // schema

	// relation caches
	directRelationCache   *lru.MonitoredLRUCache[resourceTargetRelation, *cachedGrant]
	directResourcesIndex  map[resource]map[target][]resourceTargetRelation
	directTargetsIndex    map[target]map[resource][]resourceTargetRelation
	directKeyComponents   map[resourceTargetRelation]keyComponents
	indirectRelationCache *lru.MonitoredLRUCache[resourceTargetRelation, *cachedGrant]

	// edge CEL evaluation
	compiledConditions  map[int32]*edgecel.CompiledCondition // condition ID -> compiled CEL program, from the loaded schema
	loadedSchemaVersion string
	celEvalTimeout      time.Duration

	// lookups
	lookupCache          *lru.MonitoredLRUCache[lookupKey, *LookupCacheEntry] // lookup cache for WhoCanAccess/WhatCanTargetAccess
	lookupCacheEnabled   bool
	lookupCacheTTL       time.Duration
	lookupCacheMaxResult int

	remoteChanges *remoteChangesTracking
	mutex         sync.RWMutex
}

type ProjectAuthzCache interface {
	GetSchema() *descope.FGASchema
	CheckRelations(ctx context.Context, relations []*descope.FGARelation, extraContext map[string]any) (checks []*descope.FGACheck, unchecked []*descope.FGARelation, indexToCheck map[int]*descope.FGACheck)
	UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema)
	EnsureSchemaLoaded(ctx context.Context, schema *descope.FGASchema)
	UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck)
	StartRemoteChangesPolling(ctx context.Context)
	// Lookup cache methods
	GetWhoCanAccessCached(ctx context.Context, resource, relationDefinition, namespace string) (targets []string, ok bool)
	SetWhoCanAccessCached(ctx context.Context, resource, relationDefinition, namespace string, targets []string)
	GetWhatCanTargetAccessCached(ctx context.Context, target string) (relations []*descope.AuthzRelation, ok bool)
	SetWhatCanTargetAccessCached(ctx context.Context, target string, relations []*descope.AuthzRelation)
	InvalidateLookupCache(ctx context.Context)
}

var _ ProjectAuthzCache = &projectAuthzCache{} // ensure projectAuthzCache implements ProjectAuthzCache

func NewProjectAuthzCache(ctx context.Context, remoteChangesChecker RemoteChangesChecker) (ProjectAuthzCache, error) {
	indirectRelationCache, err := lru.New[resourceTargetRelation, *cachedGrant](config.GetIndirectRelationCacheSizePerProject(), "authz-indirect-relations-"+cctx.ProjectID(ctx))
	if err != nil {
		return nil, err // notest
	}
	lookupCacheEnabled := config.GetLookupCacheEnabled()
	var lookupCache *lru.MonitoredLRUCache[lookupKey, *LookupCacheEntry]
	if lookupCacheEnabled {
		lookupCache, err = lru.New[lookupKey, *LookupCacheEntry](config.GetLookupCacheSizePerProject(), "authz-lookup-"+cctx.ProjectID(ctx))
		if err != nil {
			return nil, err // notest
		}
	}
	pc := &projectAuthzCache{
		directResourcesIndex:  make(map[resource]map[target][]resourceTargetRelation),
		directTargetsIndex:    make(map[target]map[resource][]resourceTargetRelation),
		directKeyComponents:   make(map[resourceTargetRelation]keyComponents),
		indirectRelationCache: indirectRelationCache,
		celEvalTimeout:        time.Millisecond * time.Duration(config.GetCELEvalTimeoutInMillis()),
		lookupCache:           lookupCache,
		lookupCacheEnabled:    lookupCacheEnabled,
		lookupCacheTTL:        time.Second * time.Duration(config.GetLookupCacheTTLInSeconds()),
		lookupCacheMaxResult:  config.GetLookupCacheMaxResultSize(),
		remoteChanges: &remoteChangesTracking{
			lastPollTime:          time.Now(),
			remote:                remoteChangesChecker,
			remotePollingInterval: time.Millisecond * time.Duration(config.GetRemotePollingIntervalInMillis()),
			purgeCooldownWindow:   time.Minute * time.Duration(config.GetPurgeCooldownWindowInMinutes()),
		},
	}
	directRelationCache, err := lru.NewWithEvict[resourceTargetRelation, *cachedGrant](config.GetDirectRelationCacheSizePerProject(), "authz-direct-relations-"+cctx.ProjectID(ctx), pc.removeIndexOnCacheEviction)
	if err != nil {
		return nil, err // notest
	}
	pc.directRelationCache = directRelationCache
	pc.remoteChanges.tickHandler = pc.updateCacheWithRemotePolling
	cctx.Logger(ctx).Info().
		Int("direct_relation_cache_size", config.GetDirectRelationCacheSizePerProject()).
		Int("indirect_relation_cache_size", config.GetIndirectRelationCacheSizePerProject()).
		Int("remote_polling_interval_ms", config.GetRemotePollingIntervalInMillis()).
		Int("purge_cooldown_window_minutes", config.GetPurgeCooldownWindowInMinutes()).
		Bool("lookup_cache_enabled", lookupCacheEnabled).
		Int("lookup_cache_size", config.GetLookupCacheSizePerProject()).
		Int("lookup_cache_ttl_seconds", config.GetLookupCacheTTLInSeconds()).
		Int("lookup_cache_max_result", config.GetLookupCacheMaxResultSize()).
		Msg("Project authz cache initialized")
	return pc, nil
}

func (pc *projectAuthzCache) GetSchema() *descope.FGASchema {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	return pc.schemaCache
}

func (pc *projectAuthzCache) CheckRelations(ctx context.Context, relations []*descope.FGARelation, extraContext map[string]any) (checks []*descope.FGACheck, unchecked []*descope.FGARelation, indexToCheck map[int]*descope.FGACheck) {
	indexToCheck = make(map[int]*descope.FGACheck, len(relations))
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	for i, r := range relations {
		grant, direct, ok := pc.getCachedGrant(ctx, r)
		if !ok {
			unchecked = append(unchecked, r)
			continue
		}
		var check *descope.FGACheck
		if grant.cond == nil {
			// unconditional grant (direct or indirect)
			check = &descope.FGACheck{Allowed: grant.allowed, Relation: r, Info: &descope.FGACheckInfo{Direct: direct}}
		} else if pc.certMatches(ctx, grant.cond, extraContext) {
			// conditional grant whose conditions all still re-evaluate to their recorded values — safe to serve
			check = &descope.FGACheck{Allowed: grant.allowed, Relation: r, Info: &descope.FGACheckInfo{Conditional: true}}
		} else {
			// cached conditional grant no longer matches this request's context — needs a fresh Check
			unchecked = append(unchecked, r)
			continue
		}
		checks = append(checks, check)
		indexToCheck[i] = check
	}
	return
}

// getCachedGrant returns the cached grant for r, preferring the direct cache over the indirect cache.
// The second return reports whether the hit was a direct grant.
func (pc *projectAuthzCache) getCachedGrant(ctx context.Context, r *descope.FGARelation) (grant *cachedGrant, direct bool, ok bool) {
	k := key(r)
	if grant, ok := pc.directRelationCache.Get(ctx, k); ok {
		return grant, true, true
	}
	if grant, ok := pc.indirectRelationCache.Get(ctx, k); ok {
		return grant, false, true
	}
	return nil, false, false
}

func (pc *projectAuthzCache) certMatches(ctx context.Context, cert *conditionalCert, extraContext map[string]any) bool {
	if len(cert.trueConds) == 0 && len(cert.falseConds) == 0 {
		return false
	}
	return pc.conditionsEvalTo(ctx, cert.trueConds, true, extraContext) &&
		pc.conditionsEvalTo(ctx, cert.falseConds, false, extraContext)
}

func (pc *projectAuthzCache) conditionsEvalTo(ctx context.Context, ids []int32, want bool, extraContext map[string]any) bool {
	for _, id := range ids {
		compiled, ok := pc.compiledConditions[id]
		if !ok {
			return false
		}
		got, evaluated := compiled.Eval(ctx, extraContext, pc.celEvalTimeout)
		if !evaluated || got != want {
			return false
		}
	}
	return true
}

func (pc *projectAuthzCache) UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.setSchema(ctx, schema)
	// on schema update, we need to purge all relations
	pc.purgeRelationCaches(ctx)
}

func (pc *projectAuthzCache) EnsureSchemaLoaded(ctx context.Context, schema *descope.FGASchema) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	if pc.schemaCache != nil && pc.loadedSchemaVersion != "" {
		return
	}
	pc.setSchema(ctx, schema)
}

func (pc *projectAuthzCache) setSchema(ctx context.Context, schema *descope.FGASchema) {
	pc.schemaCache = schema
	pc.compiledConditions = nil
	pc.loadedSchemaVersion = ""
	if schema == nil {
		return
	}
	pc.loadedSchemaVersion = schema.Version
	compiled := make(map[int32]*edgecel.CompiledCondition, len(schema.Conditions))
	for _, c := range schema.Conditions {
		program, err := edgecel.Compile(ctx, c)
		if err != nil {
			cctx.Logger(ctx).Error().Str("condition", c.Name).Err(err).Msg("Failed to compile schema condition for edge evaluation, conditional grants gated by it won't be cached")
			continue
		}
		compiled[c.ID] = program
	}
	pc.compiledConditions = compiled
}

func (pc *projectAuthzCache) UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.indirectRelationCache.Purge(ctx) // added (direct) relations can change the result of indirect checks, so we must purge all indirect relations
	for _, r := range relations {
		pc.addToLookupCache(ctx, r)
	}
}

func (pc *projectAuthzCache) UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation) {
	if len(relations) == 0 {
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.indirectRelationCache.Purge(ctx) // deleted (direct) relations can change the result of indirect checks, so we must purge all indirect relations
	// Lookup cache not purged: candidate filtering verifies each candidate via CheckRelation
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
		if c.Info.FactUsed {
			// fact-involved results depend on mutable backend state the edge can't observe — never cache
			continue
		}
		grant := &cachedGrant{allowed: c.Allowed}
		if c.Info.Conditional {
			switch {
			case c.Info.SchemaVersion != pc.loadedSchemaVersion ||
				(len(c.Info.TrueConditions) == 0 && len(c.Info.FalseConditions) == 0) ||
				!pc.hasAllCompiledConditions(c.Info.TrueConditions) || !pc.hasAllCompiledConditions(c.Info.FalseConditions):
				// Schema/state skew that should rarely happen (version mismatch, empty certificate, or an
				// uncompiled condition) — worth a warning. The grant isn't cached and falls through to backend.
				cctx.Logger(ctx).Warn().
					Str("schema_version", c.Info.SchemaVersion).
					Str("loaded_schema_version", pc.loadedSchemaVersion).
					Int("true_conditions_count", len(c.Info.TrueConditions)).
					Int("false_conditions_count", len(c.Info.FalseConditions)).
					Msg("Conditional grant not cacheable at the edge (schema state), skipping")
				continue
			case len(c.Info.MissingContext) != 0 || c.Info.ConditionalErr != "":
				// Driven by the request context and expected to happen (a param a condition needs wasn't
				// supplied, or a condition couldn't be evaluated for this context) — defer to backend.
				cctx.Logger(ctx).Debug().
					Int("missing_context_count", len(c.Info.MissingContext)).
					Str("conditional_err", c.Info.ConditionalErr).
					Msg("Conditional grant not cacheable at the edge (request context), skipping")
				continue
			}
			grant.cond = &conditionalCert{trueConds: c.Info.TrueConditions, falseConds: c.Info.FalseConditions}
		}
		if c.Info.Direct {
			pc.addDirectRelation(ctx, c.Relation, grant)
		} else {
			pc.addIndirectRelation(ctx, c.Relation, grant)
		}
	}
}

func (pc *projectAuthzCache) hasAllCompiledConditions(ids []int32) bool {
	for _, id := range ids {
		if _, ok := pc.compiledConditions[id]; !ok {
			return false
		}
	}
	return true
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
		pc.setSchema(ctx, nil)
		cctx.Logger(ctx).Debug().Msg("No cached relations, skipping remote changes check")
		return
	}
	// get the changes since the last poll time
	cctx.Logger(ctx).Debug().Msg("Checking for remote changes...")
	remoteChanges, err := pc.fetchRemoteChanges(ctx)
	// if there was an error, purge all caches after cooldown (if configured) or immediately
	if err != nil {
		if isTooManyModifiedRelationsError(err) {
			cctx.Logger(ctx).Warn().Err(err).Msg("Too many modified relations, purging all caches immediately")
			pc.cancelPurgeCooldown(ctx)
			pc.purgeAllCaches(ctx)
			return
		}
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
	// Lookup cache not purged: candidate filtering verifies each candidate via CheckRelation
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

func (pc *projectAuthzCache) addDirectRelation(ctx context.Context, r *descope.FGARelation, grant *cachedGrant) {
	key := key(r)
	pc.directRelationCache.Add(ctx, key, grant)
	resource := resource(r.Resource)
	target := target(r.Target)
	pc.directKeyComponents[key] = keyComponents{r: resource, t: target}
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

func (pc *projectAuthzCache) addIndirectRelation(ctx context.Context, r *descope.FGARelation, grant *cachedGrant) {
	key := key(r)
	pc.indirectRelationCache.Add(ctx, key, grant)
}

func (pc *projectAuthzCache) removeDirectRelation(ctx context.Context, r *descope.FGARelation) {
	key := key(r)
	pc.directRelationCache.Remove(ctx, key)
	resource := resource(r.Resource)
	target := target(r.Target)
	pc.removeKeyFromResourceIndex(resource, target, key)
	pc.removeKeyFromTargetIndex(resource, target, key)
	delete(pc.directKeyComponents, key)
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

func key(r *descope.FGARelation) resourceTargetRelation {
	return resourceTargetRelation(r.Resource + ":" + r.Target + ":" + r.Relation)
}

func (pc *projectAuthzCache) removeIndexOnCacheEviction(key resourceTargetRelation, _ *cachedGrant) {
	// on eviction, we need to remove the keys from the indexes as well
	components, ok := pc.directKeyComponents[key]
	if !ok {
		return
	}
	pc.removeKeyFromResourceIndex(components.r, components.t, key)
	pc.removeKeyFromTargetIndex(components.r, components.t, key)
	delete(pc.directKeyComponents, key)
}

// must be called while holding the mutex
func (pc *projectAuthzCache) purgeAllCaches(ctx context.Context) {
	pc.setSchema(ctx, nil)
	pc.purgeRelationCaches(ctx)
	// since all caches were purged, we can update the last poll time to the current time
	pc.remoteChanges.lastPollTime = time.Now()
}

func (pc *projectAuthzCache) purgeRelationCaches(ctx context.Context) {
	pc.directResourcesIndex = make(map[resource]map[target][]resourceTargetRelation)
	pc.directTargetsIndex = make(map[target]map[resource][]resourceTargetRelation)
	pc.directKeyComponents = make(map[resourceTargetRelation]keyComponents)
	pc.directRelationCache.Purge(ctx)
	pc.indirectRelationCache.Purge(ctx)
	if pc.lookupCache != nil {
		pc.lookupCache.Purge(ctx)
	}
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

		// start the cooldown timer - use new context as the timer may outlive the current request context
		// Note: The timer callback acquires the mutex and checks that purgeCooldownTimer is not nil to prevent purging if
		// cancelPurgeCooldown() was called in a race condition just before the timer fires (it sets purgeCooldownTimer to nil).
		timerCtx := context.WithoutCancel(ctx)
		pc.remoteChanges.purgeCooldownTimer = time.AfterFunc(pc.remoteChanges.purgeCooldownWindow, func() {
			pc.mutex.Lock()
			defer pc.mutex.Unlock()
			// only purge if we're still in error state (not cancelled)
			if pc.remoteChanges.purgeCooldownTimer != nil {
				cctx.Logger(timerCtx).Error().Dur("cooldown_window", pc.remoteChanges.purgeCooldownWindow).Msg("Cooldown window elapsed, purging all caches")
				pc.purgeAllCaches(timerCtx)
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

func whoCanAccessKey(resource, relationDefinition, namespace string) lookupKey {
	return lookupKey("wca:" + resource + ":" + relationDefinition + ":" + namespace)
}

func whatCanTargetAccessKey(target string) lookupKey {
	return lookupKey("wcta:" + target)
}

func (pc *projectAuthzCache) GetWhoCanAccessCached(ctx context.Context, resource, relationDefinition, namespace string) ([]string, bool) {
	if !pc.lookupCacheEnabled || pc.lookupCache == nil {
		return nil, false
	}
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	key := whoCanAccessKey(resource, relationDefinition, namespace)
	entry, ok := pc.lookupCache.Get(ctx, key)
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	return entry.Results, true
}

func (pc *projectAuthzCache) SetWhoCanAccessCached(ctx context.Context, resource, relationDefinition, namespace string, targets []string) {
	if !pc.lookupCacheEnabled || pc.lookupCache == nil {
		return
	}
	if len(targets) > pc.lookupCacheMaxResult {
		cctx.Logger(ctx).Debug().Int("result_size", len(targets)).Int("max_size", pc.lookupCacheMaxResult).Msg("Skipping lookup cache - result too large")
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	key := whoCanAccessKey(resource, relationDefinition, namespace)
	now := time.Now()
	pc.lookupCache.Add(ctx, key, &LookupCacheEntry{
		Results:   targets,
		CachedAt:  now,
		ExpiresAt: now.Add(pc.lookupCacheTTL),
	})
}

func (pc *projectAuthzCache) GetWhatCanTargetAccessCached(ctx context.Context, target string) ([]*descope.AuthzRelation, bool) {
	if !pc.lookupCacheEnabled || pc.lookupCache == nil {
		return nil, false
	}
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	key := whatCanTargetAccessKey(target)
	entry, ok := pc.lookupCache.Get(ctx, key)
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	relations := make([]*descope.AuthzRelation, 0, len(entry.Results))
	for _, r := range entry.Results {
		parts := strings.SplitN(r, "|", 3)
		if len(parts) != 3 {
			continue
		}
		relations = append(relations, &descope.AuthzRelation{Resource: parts[0], RelationDefinition: parts[1], Namespace: parts[2], Target: target})
	}
	if len(relations) == 0 {
		return nil, false
	}
	return relations, true
}

func (pc *projectAuthzCache) SetWhatCanTargetAccessCached(ctx context.Context, target string, relations []*descope.AuthzRelation) {
	if !pc.lookupCacheEnabled || pc.lookupCache == nil {
		return
	}
	if len(relations) > pc.lookupCacheMaxResult {
		cctx.Logger(ctx).Debug().Int("result_size", len(relations)).Int("max_size", pc.lookupCacheMaxResult).Msg("Skipping lookup cache - result too large")
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	key := whatCanTargetAccessKey(target)
	results := make([]string, len(relations))
	for i, r := range relations {
		results[i] = r.Resource + "|" + r.RelationDefinition + "|" + r.Namespace
	}
	now := time.Now()
	pc.lookupCache.Add(ctx, key, &LookupCacheEntry{
		Results:   results,
		CachedAt:  now,
		ExpiresAt: now.Add(pc.lookupCacheTTL),
	})
}

func (pc *projectAuthzCache) InvalidateLookupCache(ctx context.Context) {
	if !pc.lookupCacheEnabled || pc.lookupCache == nil {
		return
	}
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	pc.lookupCache.Purge(ctx)
	cctx.Logger(ctx).Info().Msg("Lookup cache invalidated")
}

// addToLookupCache adds a direct relation to existing lookup cache entries.
// Must be called while holding the mutex.
func (pc *projectAuthzCache) addToLookupCache(ctx context.Context, r *descope.FGARelation) {
	if !pc.lookupCacheEnabled || pc.lookupCache == nil {
		return
	}
	wcaKey := whoCanAccessKey(r.Resource, r.Relation, r.ResourceType)
	if entry, ok := pc.lookupCache.Get(ctx, wcaKey); ok && time.Now().Before(entry.ExpiresAt) {
		if !slices.Contains(entry.Results, r.Target) {
			entry.Results = append(entry.Results, r.Target)
		}
	}
	wctaKey := whatCanTargetAccessKey(r.Target)
	if entry, ok := pc.lookupCache.Get(ctx, wctaKey); ok && time.Now().Before(entry.ExpiresAt) {
		newResult := r.Resource + "|" + r.Relation + "|" + r.ResourceType
		if !slices.Contains(entry.Results, newResult) {
			entry.Results = append(entry.Results, newResult)
		}
	}
}
