package caches

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/go-sdk/descope"
	lru "github.com/descope/golang-lru"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRemoteChangesChecker struct {
	GetModifiedFunc func(ctx context.Context, since time.Time) (*descope.AuthzModified, error)
}

func (m *mockRemoteChangesChecker) GetModified(ctx context.Context, since time.Time) (*descope.AuthzModified, error) {
	return m.GetModifiedFunc(ctx, since)
}

var _ RemoteChangesChecker = &mockRemoteChangesChecker{} // ensure mockRemoteChangesChecker implements RemoteChangesChecker

// helper struct to hold a cached relation and its expected allowed value
type cachedRelation struct {
	allowed bool
	direct  bool
	r       *descope.FGARelation
}

func TestNewProjectAuthzCache(t *testing.T) {
	ctx := context.TODO()
	remoteChecker := &mockRemoteChangesChecker{}
	cache, err := NewProjectAuthzCache(ctx, remoteChecker)
	require.NoError(t, err)
	require.NotNil(t, cache)
}

func TestSchemaUpdate(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	schemaDef := "schema definition"
	schema := &descope.FGASchema{
		Schema: schemaDef,
	}
	// prepare some data in the caches
	updateBothCachesWithChecks(ctx, t, cache)
	// update the schema
	cache.UpdateCacheWithSchema(ctx, schema)
	// check that all caches are now empty
	assert.Equal(t, 0, cache.directRelationCache.Len(ctx))
	assert.Equal(t, 0, cache.indirectRelationCache.Len(ctx))
	// check that the schema is updated
	fromCache := cache.GetSchema()
	require.Equal(t, schemaDef, fromCache.Schema)
}

func TestUpdateCacheWithChecks(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// validate CheckRelation results
	for _, cr := range cachedRelations {
		allowed, direct, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok)
		assert.Equal(t, cr.allowed, allowed)
		assert.Equal(t, cr.direct, direct)
	}
	// call UpdateCacheWithChecks using the already cached relations, verify that no new entries are added to the cache and that the cache is not invalidated
	directSize := cache.directRelationCache.Len(ctx)
	indirectSize := cache.indirectRelationCache.Len(ctx)
	for _, cr := range cachedRelations {
		cache.UpdateCacheWithChecks(ctx, []*descope.FGACheck{{Allowed: cr.allowed, Relation: cr.r, Info: &descope.FGACheckInfo{Direct: cr.direct}}})
		// validate CheckRelation after the 2nd update, should return the same results
		allowed, direct, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok)
		assert.Equal(t, cr.allowed, allowed)
		assert.Equal(t, cr.direct, direct)
	}
	assert.Equal(t, directSize, cache.directRelationCache.Len(ctx))
	assert.Equal(t, indirectSize, cache.indirectRelationCache.Len(ctx))
}

func TestUpdateCacheWithAddedRelations(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	// prepare some data in the caches
	oldRelations := updateBothCachesWithChecks(ctx, t, cache)
	// add new relations
	newRelations := []*descope.FGARelation{
		{Resource: "p1:file1", Target: "user1", Relation: "owner"},
		{Resource: "p1:file1", Target: "user2", Relation: "owner"},
		{Resource: "p1:file1", Target: "user3", Relation: "owner"},
		{Resource: "p1:file1", Target: "user4", Relation: "owner"},
	}
	cache.UpdateCacheWithAddedRelations(ctx, newRelations)
	// direct old relations should still be there, indirect should now be removed
	for _, old := range oldRelations {
		expectedToRemainInCache := old.direct // non direct relations should have been removed, direct should still be there
		allowed, _, ok := cache.CheckRelation(ctx, old.r)
		assert.Equal(t, expectedToRemainInCache, ok)
		expectedAllowed := expectedToRemainInCache && old.allowed
		assert.Equal(t, expectedAllowed, allowed) // if the relation is still in the cache, it should have the same allowed value
	}
	// all new relations should be allowed
	for _, r := range newRelations {
		allowed, _, ok := cache.CheckRelation(ctx, r)
		assert.True(t, ok)
		assert.True(t, allowed)
	}
}

func TestUpdateCacheWithDeletedRelations(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	// prepare some data in the caches
	oldRelations := updateBothCachesWithChecks(ctx, t, cache)
	// delete 1 direct relation
	toDelete := oldRelations[0]
	require.True(t, toDelete.direct) // sanity check, verify that the relation is direct
	cache.UpdateCacheWithDeletedRelations(ctx, []*descope.FGARelation{toDelete.r})
	// verify that:
	// 1. deleted relation is not in the cache anymore
	// 2. all other direct relations are still in the cache
	// 3. all indirect relations are removed
	for _, old := range oldRelations {
		expectedToRemainInCache := old.direct && old.r != toDelete.r
		allowed, _, ok := cache.CheckRelation(ctx, old.r)
		assert.Equal(t, expectedToRemainInCache, ok)
		expectedAllowed := expectedToRemainInCache && old.allowed
		assert.Equal(t, expectedAllowed, allowed)
	}
	// delete all remaining direct relations + perform re-deletes
	for _, old := range oldRelations {
		cache.UpdateCacheWithDeletedRelations(ctx, []*descope.FGARelation{old.r})
		allowed, direct, ok := cache.CheckRelation(ctx, old.r)
		assert.False(t, ok)
		assert.False(t, allowed)
		assert.False(t, direct)
	}
	// verify that indices are now empty
	assert.Equal(t, 0, len(cache.directResourcesIndex), "%v", cache.directResourcesIndex)
	assert.Equal(t, 0, len(cache.directTargetsIndex), "%v", cache.directTargetsIndex)
}

func TestUpdateCacheWithAddedRelations_AddsToLookupCache(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	cache.lookupCacheEnabled = true
	cache.SetWhoCanAccessCached(ctx, "doc1", "viewer", "docs", []string{"user1", "user2"})
	cache.SetWhatCanTargetAccessCached(ctx, "user3", []*descope.AuthzRelation{{Resource: "other", RelationDefinition: "editor", Namespace: "docs", Target: "user3"}})
	cache.UpdateCacheWithAddedRelations(ctx, []*descope.FGARelation{{Resource: "doc1", Target: "user3", Relation: "viewer", ResourceType: "docs"}})
	targets, ok := cache.GetWhoCanAccessCached(ctx, "doc1", "viewer", "docs")
	assert.True(t, ok, "lookup cache entry should still exist")
	assert.Contains(t, targets, "user1", "existing target should remain")
	assert.Contains(t, targets, "user2", "existing target should remain")
	assert.Contains(t, targets, "user3", "new target should be added to lookup cache")
	relations, ok := cache.GetWhatCanTargetAccessCached(ctx, "user3")
	assert.True(t, ok, "lookup cache entry should still exist")
	assert.Len(t, relations, 2, "new relation should be added to existing entry")
}

func TestUpdateCacheWithDeletedRelations_DoesNotPurgeLookupCache(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	cache.lookupCacheEnabled = true
	cache.SetWhoCanAccessCached(ctx, "doc1", "viewer", "docs", []string{"user1", "user2"})
	cache.SetWhatCanTargetAccessCached(ctx, "user1", []*descope.AuthzRelation{{Resource: "doc1", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"}})
	cache.UpdateCacheWithDeletedRelations(ctx, []*descope.FGARelation{{Resource: "doc1", Target: "user1", Relation: "viewer"}})
	targets, ok := cache.GetWhoCanAccessCached(ctx, "doc1", "viewer", "docs")
	assert.True(t, ok, "lookup cache should NOT be purged - candidate filtering handles removed access")
	assert.Equal(t, []string{"user1", "user2"}, targets, "candidates remain until filtered by CheckRelation")
	relations, ok := cache.GetWhatCanTargetAccessCached(ctx, "user1")
	assert.True(t, ok, "lookup cache should NOT be purged")
	assert.Len(t, relations, 1, "candidates remain until filtered by CheckRelation")
}

func TestEmptyActions(t *testing.T) {
	ctx := context.TODO()
	// pre populate the cache
	cache, _ := setup(t)
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// get the initial cache size
	expectedDirectSize := cache.directRelationCache.Len(ctx)
	expectedIndirectSize := cache.indirectRelationCache.Len(ctx)
	// perform empty updates
	cache.UpdateCacheWithAddedRelations(ctx, nil)
	cache.UpdateCacheWithDeletedRelations(ctx, nil)
	cache.UpdateCacheWithChecks(ctx, nil)
	// cache should not change
	assert.Equal(t, expectedDirectSize, cache.directRelationCache.Len(ctx))
	assert.Equal(t, expectedIndirectSize, cache.indirectRelationCache.Len(ctx))
	for _, cr := range cachedRelations {
		allowed, direct, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok)
		assert.Equal(t, cr.allowed, allowed)
		assert.Equal(t, cr.direct, direct)
	}
}

func TestHandleRemotePollingTick_NoCachedRelations(t *testing.T) {
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// populate the schema cache only
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	// remote changes checker should not be called since there are no cached relations
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		require.Fail(t, "should not be called since there are no cached relations")
		return nil, nil
	}
	// call the tick handler directly (for testing purposes)
	cache.updateCacheWithRemotePolling(ctx)
	// verify that the schema cache was invalidated
	assert.Nil(t, cache.schemaCache)
}

func TestHandleRemotePollingTick_RemoteChangesError(t *testing.T) {
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// simulate an error from remote changes checker
	var remoteCalled bool
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		remoteCalled = true
		return nil, assert.AnError
	}
	// populate the schema cache
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	// populate the cache with some relations
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// sanity: the cache is now populated
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	require.Greater(t, cache.indirectRelationCache.Len(ctx), 0)
	require.NotNil(t, cache.GetSchema())
	//record the current time
	timeBeforePolling := time.Now()
	// sanity: last poll time is in the past
	require.Less(t, cache.remoteChanges.lastPollTime, timeBeforePolling)
	// call the tick handler directly (for testing purposes)
	cache.updateCacheWithRemotePolling(ctx)
	// sanity: verify that the remote was called
	require.True(t, remoteCalled)
	// verify that all relations were invalidated
	for _, cr := range cachedRelations {
		_, _, ok := cache.CheckRelation(ctx, cr.r)
		assert.False(t, ok)
	}
	// verify indices are now empty
	assert.Equal(t, 0, len(cache.directResourcesIndex))
	assert.Equal(t, 0, len(cache.directTargetsIndex))
	// verify that the schema cache was invalidated
	assert.Nil(t, cache.GetSchema())
	// verify that the last polling time was updated
	assert.Greater(t, cache.remoteChanges.lastPollTime, timeBeforePolling)
}

func TestHandleRemotePollingTick_CooldownWindowZero(t *testing.T) {
	// Test that with cooldown window = 0 (default), the cache is purged immediately on error
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// explicitly set cooldown window to 0
	cache.remoteChanges.purgeCooldownWindow = 0
	// simulate an error from remote changes checker
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return nil, assert.AnError
	}
	// populate the cache
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	require.Greater(t, cache.indirectRelationCache.Len(ctx), 0)
	// call the tick handler
	cache.updateCacheWithRemotePolling(ctx)
	// verify that all caches were purged immediately
	for _, cr := range cachedRelations {
		_, _, ok := cache.CheckRelation(ctx, cr.r)
		assert.False(t, ok)
	}
	assert.Nil(t, cache.GetSchema())
	assert.Nil(t, cache.remoteChanges.purgeCooldownTimer, "no timer should be active")
}

func TestHandleRemotePollingTick_CooldownWindowStartsOnFirstError(t *testing.T) {
	// Test that cooldown timer starts on first error and cache is NOT purged immediately
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// set cooldown window to 5 minutes
	cache.remoteChanges.purgeCooldownWindow = 5 * time.Minute
	// simulate an error
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return nil, assert.AnError
	}
	// populate the cache
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	// call the tick handler
	cache.updateCacheWithRemotePolling(ctx)
	// verify that caches were NOT purged
	for _, cr := range cachedRelations {
		_, _, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok, "cache should still be valid during cooldown")
	}
	assert.NotNil(t, cache.GetSchema(), "schema should still be cached")
	// verify cooldown is active
	assert.NotNil(t, cache.remoteChanges.purgeCooldownTimer, "timer should be active")
}

func TestHandleRemotePollingTick_SuccessfulResponseCancelsCooldown(t *testing.T) {
	// Test that a successful response during cooldown cancels the purge
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// set cooldown window to 5 minutes
	cache.remoteChanges.purgeCooldownWindow = 5 * time.Minute
	// first call: simulate an error to start cooldown
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return nil, assert.AnError
	}
	// populate the cache
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// trigger first error
	cache.updateCacheWithRemotePolling(ctx)
	// verify cooldown is active
	require.NotNil(t, cache.remoteChanges.purgeCooldownTimer, "timer should be active")
	// second call: simulate successful response
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return &descope.AuthzModified{SchemaChanged: false}, nil
	}
	cache.updateCacheWithRemotePolling(ctx)
	// verify cooldown was cancelled
	assert.Nil(t, cache.remoteChanges.purgeCooldownTimer, "timer should be cancelled")
	// verify caches are still valid
	for _, cr := range cachedRelations {
		_, _, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok, "cache should still be valid after successful response")
	}
	assert.NotNil(t, cache.GetSchema())
}

func TestHandleRemotePollingTick_CooldownWindowElapsesAndPurges(t *testing.T) {
	// Test that cache is purged after cooldown window elapses
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// set cooldown window to a very short duration for testing
	cooldownDuration := 100 * time.Millisecond
	cache.remoteChanges.purgeCooldownWindow = cooldownDuration
	// simulate an error
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return nil, assert.AnError
	}
	// populate the cache
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	updateBothCachesWithChecks(ctx, t, cache)
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	// trigger error to start cooldown
	cache.updateCacheWithRemotePolling(ctx)
	// verify cooldown is active and cache is still valid
	require.NotNil(t, cache.remoteChanges.purgeCooldownTimer)
	require.Greater(t, cache.directRelationCache.Len(ctx), 0, "cache should still be valid immediately after error")
	// wait for cooldown to elapse with generous buffer to avoid flakiness
	time.Sleep(cooldownDuration + 50*time.Millisecond)
	// verify that caches were purged
	assert.Nil(t, cache.GetSchema(), "schema should be purged after cooldown")
	cache.mutex.RLock()
	defer cache.mutex.RUnlock()
	assert.Equal(t, 0, cache.directRelationCache.Len(ctx), "cache should be purged after cooldown")
	assert.Equal(t, 0, cache.indirectRelationCache.Len(ctx), "cache should be purged after cooldown")
}

func TestHandleRemotePollingTick_SubsequentErrorsDuringCooldown(t *testing.T) {
	// Test that subsequent errors during cooldown don't reset the timer
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// set cooldown window to 5 minutes
	cache.remoteChanges.purgeCooldownWindow = 5 * time.Minute
	// simulate an error
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return nil, assert.AnError
	}
	// populate the cache
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	updateBothCachesWithChecks(ctx, t, cache)
	// trigger first error
	cache.updateCacheWithRemotePolling(ctx)
	firstTimer := cache.remoteChanges.purgeCooldownTimer
	require.NotNil(t, firstTimer)
	// wait a bit
	time.Sleep(10 * time.Millisecond)
	// trigger second error
	cache.updateCacheWithRemotePolling(ctx)
	// verify that the timer didn't change (same timer instance)
	assert.Same(t, firstTimer, cache.remoteChanges.purgeCooldownTimer, "timer should not be reset")
	// cache should still be valid
	assert.Greater(t, cache.directRelationCache.Len(ctx), 0)
}

func TestHandleRemotePollingTick_RemoteSchemaChange(t *testing.T) {
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// populate the cache with some relations
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// sanity check: the cache is now populated
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	require.Greater(t, cache.indirectRelationCache.Len(ctx), 0)
	// Simulate a schema change in the remote
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return &descope.AuthzModified{SchemaChanged: true}, nil
	}
	//record the current time
	timeBeforePolling := time.Now()
	// sanity: last poll time is in the past
	require.Less(t, cache.remoteChanges.lastPollTime, timeBeforePolling)
	// call the tick handler directly (for testing purposes)
	cache.updateCacheWithRemotePolling(ctx)
	// verify that the schema cache was invalidated
	assert.Nil(t, cache.GetSchema())
	// verify that all relations were invalidated
	for _, cr := range cachedRelations {
		allowed, direct, ok := cache.CheckRelation(ctx, cr.r)
		assert.False(t, ok)
		assert.False(t, allowed)
		assert.False(t, direct)
	}
	// verify indices are now empty
	assert.Equal(t, 0, len(cache.directResourcesIndex))
	assert.Equal(t, 0, len(cache.directTargetsIndex))
	// verify that the last polling time was updated
	assert.Greater(t, cache.remoteChanges.lastPollTime, timeBeforePolling)
}

func TestHandleRemotePollingTick_RemoteRelationChange(t *testing.T) {
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// populate the cache with a schema and some relations
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// sanity check: the cache is now populated
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	require.Greater(t, cache.indirectRelationCache.Len(ctx), 0)
	// get one of the cached (direct) relations resource
	resourceChanged := cachedRelations[0].r.Resource
	targetChanged := "user2"
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return &descope.AuthzModified{
			Resources: []string{resourceChanged, "not_in_cache"},
			Targets:   []string{targetChanged, "not_in_cache"},
		}, nil
	}
	// call the tick handler directly (for testing purposes)
	cache.updateCacheWithRemotePolling(ctx)
	// Verify that the schema cache was not invalidated
	assert.NotNil(t, cache.GetSchema())
	// Verify that all indirect relations are now invalidated
	assert.Equal(t, 0, cache.indirectRelationCache.Len(ctx))
	// verify that all relations not changed remotely are still in the cache
	var atLeastOneStillInCache bool
	for _, cr := range cachedRelations {
		expectedToRemainInCache := cr.direct && cr.r.Resource != resourceChanged && cr.r.Target != targetChanged
		atLeastOneStillInCache = atLeastOneStillInCache || expectedToRemainInCache
		allowed, _, ok := cache.CheckRelation(ctx, cr.r)
		assert.Equal(t, expectedToRemainInCache, ok, "relation: %v cache state is wrong", cr.r)
		// if the relation is still in the cache, it should have the same allowed value
		expectedAllowed := expectedToRemainInCache && cr.allowed
		assert.Equal(t, expectedAllowed, allowed)
		// validate no index was wrongly removed
		if expectedToRemainInCache {
			resource := resource(cr.r.Resource)
			target := target(cr.r.Target)
			assert.True(t, slices.Contains(cache.directResourcesIndex[resource][target], key(cr.r)))
			assert.True(t, slices.Contains(cache.directTargetsIndex[target][resource], key(cr.r)))
		}
	}
	// verify the the changed target and relation were removed from the indices
	_, rOK := cache.directResourcesIndex[resource(resourceChanged)]
	assert.False(t, rOK, "resource: %s should have been removed from the cache and the index", resourceChanged)
	_, tOK := cache.directTargetsIndex[target(targetChanged)]
	assert.False(t, tOK, "target: %s should have been removed from the cache and the index", targetChanged)
	// we know that there is one direct relation that should still be in the cache since it has a different resource
	assert.True(t, atLeastOneStillInCache)
}

func TestHandleRemotePollingTick_RemoteRelationChange_DoesNotPurgeLookupCache(t *testing.T) {
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	cache.lookupCacheEnabled = true
	updateBothCachesWithChecks(ctx, t, cache)
	cache.SetWhoCanAccessCached(ctx, "doc1", "viewer", "docs", []string{"user1", "user2"})
	cache.SetWhatCanTargetAccessCached(ctx, "user1", []*descope.AuthzRelation{{Resource: "doc1", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"}})
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return &descope.AuthzModified{Resources: []string{"some-resource"}, Targets: []string{"some-target"}}, nil
	}
	cache.updateCacheWithRemotePolling(ctx)
	targets, ok := cache.GetWhoCanAccessCached(ctx, "doc1", "viewer", "docs")
	assert.True(t, ok, "lookup cache should NOT be purged - candidate filtering handles stale candidates")
	assert.Equal(t, []string{"user1", "user2"}, targets)
	relations, ok := cache.GetWhatCanTargetAccessCached(ctx, "user1")
	assert.True(t, ok, "lookup cache should NOT be purged")
	assert.Len(t, relations, 1)
}

func TestHandleRemotePollingTick_NoRemoteChanges(t *testing.T) {
	ctx := context.TODO()
	cache, remoteChecker := setup(t)
	// populate the cache with a schema and some relations
	cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: "schema"})
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// sanity check: the cache is now populated
	require.Greater(t, cache.directRelationCache.Len(ctx), 0)
	require.Greater(t, cache.indirectRelationCache.Len(ctx), 0)
	// Simulate no changes in the remote
	var remoteCalled bool
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		remoteCalled = true
		return &descope.AuthzModified{}, nil
	}
	// call the tick handler directly (for testing purposes)
	cache.updateCacheWithRemotePolling(ctx)
	// sanity: verify that the remote was called
	require.True(t, remoteCalled)
	// verify that the schema cache was not invalidated
	assert.NotNil(t, cache.GetSchema())
	// verify that all relations are still in the cache
	for _, cr := range cachedRelations {
		allowed, direct, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok)
		assert.Equal(t, cr.allowed, allowed)
		assert.Equal(t, cr.direct, direct)
	}
}

func TestRemotePolling(t *testing.T) {
	// use context with cancel to avoid goroutine leak
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	cache, _ := setup(t)
	// force the polling interval to be 10ms
	cache.remoteChanges.remotePollingInterval = 10 * time.Millisecond
	// mock tick handler func
	var mutex sync.RWMutex
	var tickHandlerCalled bool
	cache.remoteChanges.tickHandler = func(_ context.Context) {
		mutex.Lock() // must lock to avoid race condition between the test and the goroutine started in StartRemoteChangesPolling
		defer mutex.Unlock()
		tickHandlerCalled = true
	}
	// wait for the polling interval to pass
	cache.StartRemoteChangesPolling(ctx)
	time.Sleep(15 * time.Millisecond)
	// verify that the remote was called
	mutex.RLock() // must lock to avoid race condition between the test and the goroutine started in StartRemoteChangesPolling
	defer mutex.RUnlock()
	assert.True(t, tickHandlerCalled)
}

func TestCooldownMechanism_ConcurrentStressTest(t *testing.T) {
	// This test verifies thread safety of the cooldown mechanism by running concurrent
	// operations that interact with the cache and cooldown timer. We want to try and ensure:
	// 1. No panics from concurrent access
	// 2. No deadlocks
	// 3. No race conditions (run with -race flag)
	ctx := context.TODO()
	cache, remoteChecker := setup(t)

	// set cooldown window to a short duration
	cache.remoteChanges.purgeCooldownWindow = 50 * time.Millisecond

	// track error count for randomizing responses
	var errorCount, successCount int64
	var responseMutex sync.Mutex

	// remote checker randomly returns errors or successes
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		responseMutex.Lock()
		defer responseMutex.Unlock()
		// randomly return error ~40% of the time
		if time.Now().UnixNano()%10 < 4 {
			errorCount++
			return nil, assert.AnError
		}
		successCount++
		// randomly return different types of successful responses
		switch time.Now().UnixNano() % 3 {
		case 0:
			return &descope.AuthzModified{SchemaChanged: true}, nil
		case 1:
			return &descope.AuthzModified{
				Resources: []string{"r1", "r2"},
				Targets:   []string{"t1", "t2"},
			}, nil
		default:
			return &descope.AuthzModified{}, nil
		}
	}

	const numGoroutines = 100
	const iterationsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// channel to collect any panics
	panicChan := make(chan interface{}, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicChan <- r
				}
			}()

			for i := 0; i < iterationsPerGoroutine; i++ {
				// randomly choose an operation based on iteration and goroutine ID
				op := (goroutineID + i) % 7

				switch op {
				case 0:
					// trigger remote polling (this exercises the cooldown logic)
					cache.updateCacheWithRemotePolling(ctx)
				case 1:
					// update cache with schema
					cache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: uuid.NewString()})
				case 2:
					// add relations
					cache.UpdateCacheWithAddedRelations(ctx, []*descope.FGARelation{
						{Resource: "r" + uuid.NewString(), Target: "t" + uuid.NewString(), Relation: "owner"},
					})
				case 3:
					// delete relations
					cache.UpdateCacheWithDeletedRelations(ctx, []*descope.FGARelation{
						{Resource: "r" + uuid.NewString(), Target: "t" + uuid.NewString(), Relation: "owner"},
					})
				case 4:
					// check relation
					cache.CheckRelation(ctx, &descope.FGARelation{
						Resource: "r" + uuid.NewString(),
						Target:   "t" + uuid.NewString(),
						Relation: "owner",
					})
				case 5:
					// update cache with checks
					cache.UpdateCacheWithChecks(ctx, []*descope.FGACheck{
						{
							Allowed:  true,
							Relation: &descope.FGARelation{Resource: "r" + uuid.NewString(), Target: "t" + uuid.NewString(), Relation: "owner"},
							Info:     &descope.FGACheckInfo{Direct: i%2 == 0},
						},
					})
				case 6:
					// get schema (read operation)
					_ = cache.GetSchema()
				}

				// add small random delay to increase contention variety
				if i%10 == 0 {
					time.Sleep(time.Duration(i%5) * time.Microsecond)
				}
			}
		}(g)
	}

	// wait for all goroutines to complete with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success - all goroutines completed
	case <-time.After(30 * time.Second):
		t.Fatal("Test timed out - possible deadlock detected")
	}

	// check for any panics
	close(panicChan)
	var panics []interface{}
	for p := range panicChan {
		panics = append(panics, p)
	}
	if len(panics) > 0 {
		t.Fatalf("Panics detected during concurrent execution: %v", panics)
	}

	// log statistics
	responseMutex.Lock()
	t.Logf("Test completed successfully. Errors: %d, Successes: %d", errorCount, successCount)
	responseMutex.Unlock()
}

func TestCooldownMechanism_ConcurrentPollingWithTimerExpiry(t *testing.T) {
	// This test specifically focuses on the race between timer expiry and successful responses
	ctx := context.TODO()
	cache, remoteChecker := setup(t)

	// use a very short cooldown to increase timer expiry frequency
	cache.remoteChanges.purgeCooldownWindow = 5 * time.Millisecond

	var callCount int64
	var callMutex sync.Mutex

	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		callMutex.Lock()
		callCount++
		current := callCount
		callMutex.Unlock()

		// alternate between error and success to trigger cooldown start/cancel frequently
		if current%2 == 0 {
			return nil, assert.AnError
		}
		return &descope.AuthzModified{}, nil
	}

	const numGoroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// populate some data before polling
				cache.UpdateCacheWithChecks(ctx, []*descope.FGACheck{
					{
						Allowed:  true,
						Relation: &descope.FGARelation{Resource: uuid.NewString(), Target: uuid.NewString(), Relation: "owner"},
						Info:     &descope.FGACheckInfo{Direct: true},
					},
				})
				// trigger polling
				cache.updateCacheWithRemotePolling(ctx)
				// small sleep to allow timer callbacks to potentially fire
				if i%20 == 0 {
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Logf("Test completed successfully with %d remote calls", callCount)
	case <-time.After(30 * time.Second):
		t.Fatal("Test timed out - possible deadlock detected")
	}
}

func TestUnderstandEvictionCallback(t *testing.T) {
	cbCallCount := 0
	cb := func(_, _ int) {
		cbCallCount++
	}
	cache, err := lru.NewWithEvict[int, int](2, cb)
	require.NoError(t, err)
	cache.Add(1, 1)
	cache.Add(2, 2)
	cache.Remove(1)
	cache.Remove(1) //again while key not in cache
	cache.Remove(2)
	cache.Add(1, 1)
	cache.Add(2, 2)
	require.Equal(t, 0, cbCallCount, "evict callback should not have been called since no eviction happened")
	cache.Add(3, 3) // this addition should call the evict CB since the cache is full
	require.Equal(t, 1, cbCallCount)
	cache.Purge()
	require.Equal(t, 3, cbCallCount) // purge also calls the evict CB
}

func TestDirectIndices(t *testing.T) {
	ctx := context.TODO()
	cache, _ := setup(t)
	// add 2 direct relations
	cache.addDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"}, true)
	cache.addDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"}, true)
	// assert all indices created and added
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t1"], key(&descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t2"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t1"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t2"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	// assert nothing extra was added
	assert.Equal(t, 1, len(cache.directResourcesIndex))
	assert.Equal(t, 2, len(cache.directResourcesIndex["r1"]))
	assert.Equal(t, 2, len(cache.directTargetsIndex))
	assert.Equal(t, 1, len(cache.directTargetsIndex["t1"]))
	assert.Equal(t, 1, len(cache.directTargetsIndex["t2"]))
	// remove 1st relation
	cache.removeDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"})
	// assert that the removed relation is not in the indices, but the other one is
	_, ok := cache.directResourcesIndex["r1"]["t1"]
	assert.False(t, ok)
	_, ok = cache.directTargetsIndex["t1"]["r1"]
	assert.False(t, ok)
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t2"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t2"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	// remove 2nd relation
	cache.removeDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})
	// assert that both indexes are now empty
	assert.Equal(t, 0, len(cache.directResourcesIndex["r1"]))
	assert.Equal(t, 0, len(cache.directTargetsIndex["t1"]))
	assert.Equal(t, 0, len(cache.directTargetsIndex["t2"]))
	// test removal of elements which are not in the cache (don't panic)
	cache.removeDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})
	cache.removeDirectRelation(ctx, &descope.FGARelation{Resource: uuid.NewString(), Target: uuid.NewString(), Relation: uuid.NewString()})
}

func TestRemoveIndexOnEviction(t *testing.T) {
	ctx := context.TODO()
	// set cache size to 2 so that 3rd addition triggers an eviction
	t.Setenv(config.ConfigKeyDirectRelationCacheSizePerProject, "2")
	cache, _ := setup(t)
	// add 2 direct relations
	cache.addDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"}, true)
	cache.addDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"}, true)
	// assert all indices created and added
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t1"], key(&descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t2"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t1"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t2"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	// add 3rd relation (this should trigger an eviction)
	cache.addDirectRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t3", Relation: "owner"}, true)
	// assert that the 1st relation was evicted from the cache and the indices
	_, _, ok := cache.CheckRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t1", Relation: "owner"})
	assert.False(t, ok)
	_, ok = cache.directResourcesIndex["r1"]["t1"]
	assert.False(t, ok)
	_, ok = cache.directTargetsIndex["t1"]["r1"]
	assert.False(t, ok)
	// assert that the other 2 relations are still in the cache and the indices
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t2"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t2"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directResourcesIndex["r1"]["t3"], key(&descope.FGARelation{Resource: "r1", Target: "t3", Relation: "owner"})))
	assert.True(t, slices.Contains(cache.directTargetsIndex["t3"]["r1"], key(&descope.FGARelation{Resource: "r1", Target: "t3", Relation: "owner"})))
	_, _, ok = cache.CheckRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t2", Relation: "owner"})
	assert.True(t, ok)
	_, _, ok = cache.CheckRelation(ctx, &descope.FGARelation{Resource: "r1", Target: "t3", Relation: "owner"})
	assert.True(t, ok)
}

// benchmark cache checks with 1,000,000 direct relations
func BenchmarkCheckRelation(b *testing.B) {
	// prepare the cache with 1,000,000 direct relations
	ctx := context.TODO()
	b.Run("ApproximateCacheSize", func(b *testing.B) {
		b.ResetTimer()
		cache, _, _ := populateLargeDirectCache(ctx)
		sizeOfMap := unsafe.Sizeof(cache.directRelationCache)
		sizeOfKey := unsafe.Sizeof(uuid.NewString()) * 3 // ~  resource:target:relation
		sizeOfValue := unsafe.Sizeof(true)
		sizeOfIndexes := unsafe.Sizeof(map[string][]resourceTargetRelation{}) + sizeOfKey*2_000_000
		approxTotalSize := sizeOfMap + 1_000_000*(sizeOfKey+sizeOfValue) + sizeOfIndexes
		b.ReportMetric(float64(approxTotalSize)/(1024*1024), "approx_direct_cache_MB")
	})
	b.Run("AddRelations_AboveCacheSize", func(b *testing.B) {
		cache, _, _ := populateLargeDirectCache(ctx)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(b.N)
		for i := 0; i < b.N; i++ {
			go func() {
				defer wg.Done()
				cache.UpdateCacheWithAddedRelations(ctx, []*descope.FGARelation{{Resource: uuid.NewString(), Target: uuid.NewString(), Relation: "owner"}})
			}()
		}
		wg.Wait()
	})
	b.Run("CheckRelation_CacheHit", func(b *testing.B) {
		cache, resources, targets := populateLargeDirectCache(ctx)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(b.N)
		for i := 0; i < b.N; i++ {
			// set j to an int between 0 and 999,999
			go func() {
				defer wg.Done()
				j := i % 1_000_000
				cache.CheckRelation(ctx, &descope.FGARelation{Resource: resources[j], Target: targets[j], Relation: "owner"}) // true
			}()
		}
		wg.Wait()
	})
	b.Run("CheckRelation_CacheMiss", func(b *testing.B) {
		cache, _, _ := populateLargeDirectCache(ctx)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(b.N)
		for i := 0; i < b.N; i++ {
			go func() {
				defer wg.Done()
				cache.CheckRelation(ctx, &descope.FGARelation{Resource: uuid.NewString(), Target: uuid.NewString(), Relation: "owner"}) // false
			}()
		}
		wg.Wait()
	})
	b.Run("DeleteRelations", func(b *testing.B) {
		cache, resources, targets := populateLargeDirectCache(ctx)
		b.ResetTimer()
		var wg sync.WaitGroup
		wg.Add(b.N)
		for i := 0; i < b.N; i++ {
			go func() {
				defer wg.Done()
				j := i % 1_000_000
				cache.UpdateCacheWithDeletedRelations(ctx, []*descope.FGARelation{{Resource: resources[j], Target: targets[j], Relation: "owner"}})
			}()
		}
		wg.Wait()
	})
	b.Run("DeleteRelationsByResourceOrTarget_Hits", func(b *testing.B) {
		cache, resources, targets := populateLargeDirectCache(ctx)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			j := i % 1_000_000
			if i%2 == 0 {
				cache.removeDirectRelationByResource(ctx, resource(resources[j]))
			} else {
				cache.removeDirectRelationByTarget(ctx, target(targets[j]))
			}
		}
	})
	b.Run("DeleteRelationsByResourceOrTarget_Misses", func(b *testing.B) {
		cache, _, _ := populateLargeDirectCache(ctx)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				cache.removeDirectRelationByResource(ctx, resource(uuid.NewString()))
			} else {
				cache.removeDirectRelationByTarget(ctx, target(uuid.NewString()))
			}
		}
	})
}

func populateLargeDirectCache(ctx context.Context) (*projectAuthzCache, []string, []string) {
	remoteChecker := &mockRemoteChangesChecker{}
	cache, _ := NewProjectAuthzCache(ctx, remoteChecker)
	resources := make([]string, 1_000_000)
	targets := make([]string, 1_000_000)
	// insert 1,000,000 direct relation keys with 1 relation each into the cache
	for i := 0; i < 1_000_000; i++ {
		resources[i] = uuid.NewString()
		targets[i] = uuid.NewString()
		cache.UpdateCacheWithChecks(ctx, []*descope.FGACheck{{Allowed: true, Relation: &descope.FGARelation{Resource: resources[i], Target: targets[i], Relation: "owner"}, Info: &descope.FGACheckInfo{Direct: true}}})
	}
	return cache.(*projectAuthzCache), resources, targets
}

func setup(t *testing.T) (*projectAuthzCache, *mockRemoteChangesChecker) {
	ctx := context.TODO()
	remoteChecker := &mockRemoteChangesChecker{}
	cache, err := NewProjectAuthzCache(ctx, remoteChecker)
	require.NoError(t, err)
	require.NotNil(t, cache)
	return cache.(*projectAuthzCache), remoteChecker
}

func updateBothCachesWithChecks(ctx context.Context, t *testing.T, cache *projectAuthzCache) []*cachedRelation {
	resourceOneID := "1_" + uuid.NewString()
	// direct relations
	directTrueRelation := &descope.FGARelation{Resource: resourceOneID, Target: "user1", Relation: "owner"}
	directFalseRelation := &descope.FGARelation{Resource: "3_" + uuid.NewString(), Target: "user2", Relation: "owner"}
	extraDirectTrueRelation := &descope.FGARelation{Resource: resourceOneID, Target: "user1", Relation: "parent"}
	differentResourceAndTargetDirectTrueRelation := &descope.FGARelation{Resource: "5_" + uuid.NewString(), Target: uuid.NewString(), Relation: "parent"}
	// indirect relations
	indirectTrueRelation := &descope.FGARelation{Resource: "2_" + uuid.NewString(), Target: "user3", Relation: "owner"}
	indirectFalseRelation := &descope.FGARelation{Resource: "4_" + uuid.NewString(), Target: "user4", Relation: "owner"}
	// mock checks response
	checks := []*descope.FGACheck{
		{Allowed: true, Relation: directTrueRelation, Info: &descope.FGACheckInfo{Direct: true}},
		{Allowed: true, Relation: indirectTrueRelation, Info: &descope.FGACheckInfo{Direct: false}},
		{Allowed: false, Relation: directFalseRelation, Info: &descope.FGACheckInfo{Direct: true}},
		{Allowed: false, Relation: indirectFalseRelation, Info: &descope.FGACheckInfo{Direct: false}},
		{Allowed: true, Relation: extraDirectTrueRelation, Info: &descope.FGACheckInfo{Direct: true}},
		{Allowed: true, Relation: differentResourceAndTargetDirectTrueRelation, Info: &descope.FGACheckInfo{Direct: true}},
	}
	cache.UpdateCacheWithChecks(ctx, checks)
	// validate cache distribution
	require.Equal(t, 4, cache.directRelationCache.Len(ctx))
	require.Equal(t, 2, cache.indirectRelationCache.Len(ctx))
	return []*cachedRelation{
		{allowed: true, direct: true, r: directTrueRelation},
		{allowed: false, direct: true, r: directFalseRelation},
		{allowed: true, direct: false, r: indirectTrueRelation},
		{allowed: false, direct: false, r: indirectFalseRelation},
		{allowed: true, direct: true, r: extraDirectTrueRelation},
		{allowed: true, direct: true, r: differentResourceAndTargetDirectTrueRelation},
	}
}
