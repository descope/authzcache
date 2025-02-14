package caches

import (
	"context"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/descope/go-sdk/descope"
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
	// populate the cache with some relations
	cachedRelations := updateBothCachesWithChecks(ctx, t, cache)
	// call the tick handler directly (for testing purposes)
	cache.updateCacheWithRemotePolling(ctx)
	// sanity: verify that the remote was called
	require.True(t, remoteCalled)
	// verify that the cache was not invalidated
	for _, cr := range cachedRelations {
		allowed, direct, ok := cache.CheckRelation(ctx, cr.r)
		assert.True(t, ok)
		assert.Equal(t, cr.allowed, allowed)
		assert.Equal(t, cr.direct, direct)
	}
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
	// get one of the cached (direct) relations resource and target
	resourceChanged := cachedRelations[0].r.Resource
	targetChanged := cachedRelations[0].r.Target
	// Simulate a relation change in the remote
	remoteChecker.GetModifiedFunc = func(_ context.Context, _ time.Time) (*descope.AuthzModified, error) {
		return &descope.AuthzModified{
			Resources: []string{resourceChanged},
			Targets:   []string{targetChanged},
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
		assert.Equal(t, expectedToRemainInCache, ok)
		// if the relation is still in the cache, it should have the same allowed value
		expectedAllowed := expectedToRemainInCache && cr.allowed
		assert.Equal(t, expectedAllowed, allowed)
	}
	// we know that there is one direct relation that should still be in the cache since it has different resource and target
	assert.True(t, atLeastOneStillInCache)
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

// benchmark cache checks with 1,000,000 direct relations
func BenchmarkCheckRelation(b *testing.B) {
	// prepare the cache with 1,000,000 direct relations
	ctx := context.TODO()
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
	b.Run("ApproximateCacheSize", func(b *testing.B) {
		sizeOfMap := unsafe.Sizeof(cache.(*projectAuthzCache).directRelationCache)
		sizeOfKey := unsafe.Sizeof(uuid.NewString()) * 2 // ~  resource:target
		sizeOfValue := unsafe.Sizeof(map[string]bool{}) + unsafe.Sizeof("owner") + unsafe.Sizeof(true)
		approxTotalSize := sizeOfMap + 1_000_000*(sizeOfKey+sizeOfValue)
		b.ReportMetric(float64(approxTotalSize)/(1024*1024), "approx_direct_cache_MB")
	})
	b.Run("CheckRelation_CacheHit", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// set j to an int between 0 and 999,999
			j := i % 1_000_000
			cache.CheckRelation(ctx, &descope.FGARelation{Resource: resources[j], Target: targets[j], Relation: "owner"}) // true
		}
	})
	b.Run("CheckRelation_CacheMiss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.CheckRelation(ctx, &descope.FGARelation{Resource: uuid.NewString(), Target: uuid.NewString(), Relation: "owner"}) // false
		}
	})
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
	require.Equal(t, 3, cache.directRelationCache.Len(ctx)) // 4 entries, but 2 relations are saved under the same key (resourceOneID:user1) in the direct cache
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
