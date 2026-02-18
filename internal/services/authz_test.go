package services

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
	mocksmgmt "github.com/descope/go-sdk/descope/tests/mocks/mgmt"
	"github.com/stretchr/testify/require"
)

type mockCache struct {
	caches.ProjectAuthzCacheMock
	pollingStarted bool
}

func TestNewAuthzCache(t *testing.T) {
	mockCacheCreator := func(_ context.Context, _ caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error) {
		return nil, nil
	}
	mockRemoteClientCreator := func(_ string, _ logger.LoggerInterface) (sdk.Management, error) {
		return nil, nil
	}
	ac, err := New(context.TODO(), mockCacheCreator, mockRemoteClientCreator)
	require.NoError(t, err)
	require.NotNil(t, ac)
	require.NotNil(t, ac.(*authzCache).projectCacheCreator)
	require.NotNil(t, ac.(*authzCache).remoteClientCreator)
}

func injectAuthzMocks(t *testing.T) (AuthzCache, *mocksmgmt.MockManagement, *mockCache) {
	mockSDK := &mocksmgmt.MockManagement{
		MockFGA:   &mocksmgmt.MockFGA{},
		MockAuthz: &mocksmgmt.MockAuthz{},
	}
	mockRemoteClientCreator := func(_ string, _ logger.LoggerInterface) (sdk.Management, error) {
		return mockSDK, nil
	}
	mockCache := &mockCache{}
	mockCache.StartRemoteChangesPollingFunc = func(_ context.Context) {
		mockCache.pollingStarted = true
	}
	mockCacheCreator := func(_ context.Context, _ caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error) {
		return mockCache, nil
	}
	// create AuthzCache with mocks
	ac, err := New(context.TODO(), mockCacheCreator, mockRemoteClientCreator)
	require.NoError(t, err)
	return ac, mockSDK, mockCache
}

func TestCreateFGASchema(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	cacheUpdateCallCount := 0
	sdkUpdateCallCount := 0
	dsl := "schema definition"
	mockSDK.MockFGA.SaveSchemaAssert = func(schema *descope.FGASchema) {
		require.Equal(t, dsl, schema.Schema)
		sdkUpdateCallCount++
	}
	mockCache.UpdateCacheWithSchemaFunc = func(_ context.Context, schema *descope.FGASchema) {
		require.Equal(t, dsl, schema.Schema)
		cacheUpdateCallCount++
	}
	// run test
	err := ac.CreateFGASchema(context.TODO(), dsl)
	require.NoError(t, err)
	// check call counts
	require.Equal(t, 1, cacheUpdateCallCount)
	require.Equal(t, 1, sdkUpdateCallCount)
	require.True(t, mockCache.pollingStarted)
}

func TestCreateFGARelations(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	cacheUpdateCallCount := 0
	sdkUpdateCallCount := 0
	relations := []*descope.FGARelation{{Resource: "mario", Target: "luigi", Relation: "littleBro"}, {Resource: "luigi", Target: "mario", Relation: "bigBro"}}
	mockSDK.MockFGA.CreateRelationsAssert = func(rels []*descope.FGARelation) {
		require.Equal(t, relations, rels)
		sdkUpdateCallCount++
	}
	mockCache.UpdateCacheWithAddedRelationsFunc = func(_ context.Context, relations []*descope.FGARelation) {
		require.Equal(t, relations, relations)
		cacheUpdateCallCount++
	}
	// run test
	err := ac.CreateFGARelations(context.TODO(), relations)
	require.NoError(t, err)
	// check call counts
	require.Equal(t, 1, cacheUpdateCallCount)
	require.Equal(t, 1, sdkUpdateCallCount)
	require.True(t, mockCache.pollingStarted)
}

func TestCreateFGAEmptyRelations(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	mockSDK.MockFGA.CreateRelationsAssert = func(_ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	mockCache.UpdateCacheWithAddedRelationsFunc = func(_ context.Context, _ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	// run test
	err := ac.CreateFGARelations(context.TODO(), nil)
	require.NoError(t, err)
	err = ac.CreateFGARelations(context.TODO(), []*descope.FGARelation{})
	require.NoError(t, err)
	require.False(t, mockCache.pollingStarted)
}

func TestDeleteFGARelations(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	cacheUpdateCallCount := 0
	sdkUpdateCallCount := 0
	relations := []*descope.FGARelation{{Resource: "mario", Target: "luigi", Relation: "littleBro"}, {Resource: "luigi", Target: "mario", Relation: "bigBro"}}
	mockSDK.MockFGA.DeleteRelationsAssert = func(rels []*descope.FGARelation) {
		require.Equal(t, relations, rels)
		sdkUpdateCallCount++
	}
	mockCache.UpdateCacheWithDeletedRelationsFunc = func(_ context.Context, relations []*descope.FGARelation) {
		require.Equal(t, relations, relations)
		cacheUpdateCallCount++
	}
	// run test
	err := ac.DeleteFGARelations(context.TODO(), relations)
	require.NoError(t, err)
	// check call counts
	require.Equal(t, 1, cacheUpdateCallCount)
	require.Equal(t, 1, sdkUpdateCallCount)
	require.True(t, mockCache.pollingStarted)
}

func TestDeleteFGAEmptyRelations(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	mockSDK.MockFGA.DeleteRelationsAssert = func(_ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	mockCache.UpdateCacheWithDeletedRelationsFunc = func(_ context.Context, _ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	// run test
	err := ac.DeleteFGARelations(context.TODO(), nil)
	require.NoError(t, err)
	err = ac.DeleteFGARelations(context.TODO(), []*descope.FGARelation{})
	require.NoError(t, err)
	require.False(t, mockCache.pollingStarted)
}

func TestCheckEmptyRelations(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	// setup test data
	mockSDK.MockFGA.CheckAssert = func(_ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	mockCache.CheckRelationFunc = func(_ context.Context, _ *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		require.Fail(t, "should not be called")
		return false, false, false
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {
		require.Fail(t, "should not be called")
	}
	// run test with nil relations
	result, err := ac.Check(context.TODO(), nil)
	require.NoError(t, err)
	require.Empty(t, result)
	// run test with empty relations
	result, err = ac.Check(context.TODO(), []*descope.FGARelation{})
	require.NoError(t, err)
	require.Empty(t, result)
	require.True(t, mockCache.pollingStarted)
}

func TestCheckAllInCache(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	// setup test data
	relations := []*descope.FGARelation{{Resource: "mario", Target: "luigi", Relation: "bigBro"}, {Resource: "luigi", Target: "mario", Relation: "bigBro"}}
	expected := []*descope.FGACheck{
		{
			Allowed:  true,
			Relation: relations[0],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
		{
			Allowed:  false,
			Relation: relations[1],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
	}
	mockSDK.MockFGA.CheckAssert = func(_ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		if r.Resource == "mario" {
			return true, true, true
		}
		return false, true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {
		require.Fail(t, "should not be called")
	}
	// run test
	result, err := ac.Check(context.TODO(), relations)
	require.NoError(t, err)
	require.Equal(t, expected, result)
	require.True(t, mockCache.pollingStarted)
}

func TestCheckAllInSDK(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	// setup test data
	relations := []*descope.FGARelation{{Resource: "mario", Target: "luigi", Relation: "bigBro"}, {Resource: "luigi", Target: "mario", Relation: "bigBro"}}
	expected := []*descope.FGACheck{
		{
			Allowed:  true,
			Relation: relations[0],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
		{
			Allowed:  false,
			Relation: relations[1],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
	}
	mockSDK.MockFGA.CheckAssert = func(rels []*descope.FGARelation) {
		require.Equal(t, relations, rels)
	}
	mockCache.CheckRelationFunc = func(_ context.Context, _ *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		return false, false, false
	}
	mockSDK.MockFGA.CheckResponse = expected
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, checks []*descope.FGACheck) {
		require.Equal(t, expected, checks)
	}
	// run test
	result, err := ac.Check(context.TODO(), relations)
	require.NoError(t, err)
	require.Equal(t, expected, result)
	require.True(t, mockCache.pollingStarted)
}

func TestCheckMixed(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	// setup test data
	relations := []*descope.FGARelation{
		{Resource: "mario", Target: "luigi", Relation: "bigBro"},
		{Resource: "luigi", Target: "mario", Relation: "bigBro"},
		{Resource: "mario", Target: "bowser", Relation: "enemy"},
	}
	expectedSdkRelations := []*descope.FGARelation{relations[0], relations[2]} // only the 1st and 3rd relations should be checked via sdk
	expected := []*descope.FGACheck{
		{
			Allowed:  true,
			Relation: relations[0],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
		{
			Allowed:  false,
			Relation: relations[1],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
		{
			Allowed:  true,
			Relation: relations[2],
			Info:     &descope.FGACheckInfo{Direct: true},
		},
	}
	mockSDK.MockFGA.CheckAssert = func(rels []*descope.FGARelation) {
		require.Equal(t, expectedSdkRelations, rels)
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		if r.Resource == "luigi" && r.Target == "mario" { // only the second relation is in cache
			return false, true, true
		}
		return false, false, false
	}
	sdkChecks := []*descope.FGACheck{expected[0], expected[2]}
	mockSDK.MockFGA.CheckResponse = sdkChecks
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, checks []*descope.FGACheck) {
		require.Equal(t, sdkChecks, checks) // only the checks returning from sdk should be updated in cache
	}
	// run test
	result, err := ac.Check(context.TODO(), relations)
	require.NoError(t, err)
	require.Equal(t, expected, result)
	require.True(t, mockCache.pollingStarted)
}

func TestWhoCanAccess_CacheMiss(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	expectedTargets := []string{"user1", "user2", "user3"}
	setCacheCalled := false
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		return nil, false
	}
	mockCache.SetWhoCanAccessCachedFunc = func(_ context.Context, resource, rd, ns string, targets []string) {
		require.Equal(t, "doc1", resource)
		require.Equal(t, "viewer", rd)
		require.Equal(t, "docs", ns)
		require.Equal(t, expectedTargets, targets)
		setCacheCalled = true
	}
	mockSDK.MockAuthz.WhoCanAccessResponse = expectedTargets
	result, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, expectedTargets, result)
	require.True(t, setCacheCalled)
}

func TestWhoCanAccess_CacheHitWithCandidateFiltering(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	cachedCandidates := []string{"user1", "user2", "user3"}
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		return cachedCandidates, true
	}
	mockCache.SetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string, _ []string) {
		require.Fail(t, "should not update cache on hit")
	}
	checkCallCount := 0
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		checkCallCount++
		if r.Target == "user2" {
			return false, true, true
		}
		return true, true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	mockSDK.MockAuthz.WhoCanAccessAssert = func(_, _, _ string) {
		require.Fail(t, "should not call SDK on cache hit")
	}
	result, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1", "user3"}, result)
	require.Equal(t, 3, checkCallCount)
}

func TestWhoCanAccess_CacheHitFiltersStaleResults(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	cachedCandidates := []string{"user1", "user2", "user3", "user4"}
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		return cachedCandidates, true
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		return r.Target == "user1" || r.Target == "user4", true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	result, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1", "user4"}, result)
}

func TestWhatCanTargetAccess_CacheMiss(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	expectedRelations := []*descope.AuthzRelation{
		{Resource: "doc1", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"},
		{Resource: "doc2", RelationDefinition: "editor", Namespace: "docs", Target: "user1"},
	}
	setCacheCalled := false
	mockCache.GetWhatCanTargetAccessCachedFunc = func(_ context.Context, _ string) ([]*descope.AuthzRelation, bool) {
		return nil, false
	}
	mockCache.SetWhatCanTargetAccessCachedFunc = func(_ context.Context, target string, relations []*descope.AuthzRelation) {
		require.Equal(t, "user1", target)
		require.Equal(t, expectedRelations, relations)
		setCacheCalled = true
	}
	mockSDK.MockAuthz.WhatCanTargetAccessResponse = expectedRelations
	result, err := ac.WhatCanTargetAccess(context.TODO(), "user1")
	require.NoError(t, err)
	require.Equal(t, expectedRelations, result)
	require.True(t, setCacheCalled)
}

func TestWhatCanTargetAccess_CacheHitWithCandidateFiltering(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	cachedCandidates := []*descope.AuthzRelation{
		{Resource: "doc1", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"},
		{Resource: "doc2", RelationDefinition: "editor", Namespace: "docs", Target: "user1"},
		{Resource: "doc3", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"},
	}
	mockCache.GetWhatCanTargetAccessCachedFunc = func(_ context.Context, _ string) ([]*descope.AuthzRelation, bool) {
		return cachedCandidates, true
	}
	mockCache.SetWhatCanTargetAccessCachedFunc = func(_ context.Context, _ string, _ []*descope.AuthzRelation) {
		require.Fail(t, "should not update cache on hit")
	}
	checkCallCount := 0
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		checkCallCount++
		if r.Resource == "doc2" {
			return false, true, true
		}
		return true, true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	mockSDK.MockAuthz.WhatCanTargetAccessAssert = func(_ string) {
		require.Fail(t, "should not call SDK on cache hit")
	}
	result, err := ac.WhatCanTargetAccess(context.TODO(), "user1")
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, "doc1", result[0].Resource)
	require.Equal(t, "doc3", result[1].Resource)
	require.Equal(t, 3, checkCallCount)
}

func TestWhoCanAccess_EmptyCacheHitFallsBackToSDK(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		return []string{}, true
	}
	mockCache.SetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string, _ []string) {}
	mockCache.CheckRelationFunc = func(_ context.Context, _ *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		require.Fail(t, "should not check empty candidates")
		return false, false, false
	}
	mockSDK.MockAuthz.WhoCanAccessResponse = []string{"user1"}
	result, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1"}, result)
}

func TestWhoCanAccess_DirectRelationRemoved_FilteredImmediately(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		return []string{"user1", "user2", "user3"}, true
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		if r.Target == "user2" {
			return false, true, true
		}
		return true, true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	result, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1", "user3"}, result)
}

func TestWhoCanAccess_IndirectRelationRemoved_FilteredImmediately(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		return []string{"user1", "user2", "user3"}, true
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		if r.Target == "user2" {
			return false, false, true
		}
		return true, false, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	result, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1", "user3"}, result)
}

func TestWhoCanAccess_NewCandidateAddedViaLocalMutation_VisibleImmediately(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	callCount := 0
	mockCache.GetWhoCanAccessCachedFunc = func(_ context.Context, _, _, _ string) ([]string, bool) {
		callCount++
		if callCount == 1 {
			return []string{"user1", "user2"}, true
		}
		return []string{"user1", "user2", "user3"}, true
	}
	mockCache.CheckRelationFunc = func(_ context.Context, _ *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		return true, true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	result1, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1", "user2"}, result1)
	result2, err := ac.WhoCanAccess(context.TODO(), "doc1", "viewer", "docs")
	require.NoError(t, err)
	require.Equal(t, []string{"user1", "user2", "user3"}, result2)
}

func TestWhatCanTargetAccess_DirectRelationRemoved_FilteredImmediately(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	mockCache.GetWhatCanTargetAccessCachedFunc = func(_ context.Context, _ string) ([]*descope.AuthzRelation, bool) {
		return []*descope.AuthzRelation{
			{Resource: "doc1", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"},
			{Resource: "doc2", RelationDefinition: "editor", Namespace: "docs", Target: "user1"},
			{Resource: "doc3", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"},
		}, true
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		if r.Resource == "doc2" {
			return false, true, true
		}
		return true, true, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	result, err := ac.WhatCanTargetAccess(context.TODO(), "user1")
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, "doc1", result[0].Resource)
	require.Equal(t, "doc3", result[1].Resource)
}

func TestWhatCanTargetAccess_IndirectRelationRemoved_FilteredImmediately(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	mockCache.GetWhatCanTargetAccessCachedFunc = func(_ context.Context, _ string) ([]*descope.AuthzRelation, bool) {
		return []*descope.AuthzRelation{
			{Resource: "doc1", RelationDefinition: "viewer", Namespace: "docs", Target: "user1"},
			{Resource: "doc2", RelationDefinition: "editor", Namespace: "docs", Target: "user1"},
		}, true
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
		if r.Resource == "doc2" {
			return false, false, true
		}
		return true, false, true
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	result, err := ac.WhatCanTargetAccess(context.TODO(), "user1")
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "doc1", result[0].Resource)
}

func BenchmarkCheck(b *testing.B) {
	for _, numRelations := range []int{500, 1000, 5000} {
		name := fmt.Sprintf("relations=%d", numRelations)
		b.Run(name, func(b *testing.B) {
			ctx := context.TODO()
			projectCache, err := caches.NewProjectAuthzCache(ctx, nil)
			if err != nil {
				b.Fatal(err)
			}
			halfIdx := numRelations / 2
			// pre-populate cache with first half — these are always cache hits
			fixedRelations := make([]*descope.FGARelation, halfIdx)
			cachedChecks := make([]*descope.FGACheck, halfIdx)
			for i := range halfIdx {
				fixedRelations[i] = &descope.FGARelation{
					Resource:     fmt.Sprintf("cached-r-%d", i),
					Target:       fmt.Sprintf("cached-t-%d", i),
					Relation:     "viewer",
					ResourceType: "doc",
				}
				cachedChecks[i] = &descope.FGACheck{
					Allowed:  i%2 == 0,
					Relation: fixedRelations[i],
					Info:     &descope.FGACheckInfo{Direct: true},
				}
			}
			projectCache.UpdateCacheWithChecks(ctx, cachedChecks)
			// SDK response for the miss half (fixed length, reused across goroutines)
			missCount := numRelations - halfIdx
			sdkResponse := make([]*descope.FGACheck, missCount)
			for i := range sdkResponse {
				sdkResponse[i] = &descope.FGACheck{
					Allowed:  i%2 == 0,
					Relation: &descope.FGARelation{Resource: fmt.Sprintf("sdk-r-%d", i), Target: fmt.Sprintf("sdk-t-%d", i), Relation: "viewer", ResourceType: "doc"},
					Info:     &descope.FGACheckInfo{Direct: true},
				}
			}
			mockFGA := &mocksmgmt.MockFGA{
				CheckResponse: sdkResponse,
				CheckAssert: func(_ []*descope.FGARelation) {
					time.Sleep(time.Millisecond) // simulate realistic SDK latency
				},
			}
			mockSDK := &mocksmgmt.MockManagement{
				MockFGA:   mockFGA,
				MockAuthz: &mocksmgmt.MockAuthz{},
			}
			mockRemoteClientCreator := func(_ string, _ logger.LoggerInterface) (sdk.Management, error) {
				return mockSDK, nil
			}
			mockCacheCreator := func(_ context.Context, _ caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error) {
				return projectCache, nil
			}
			ac, err := New(ctx, mockCacheCreator, mockRemoteClientCreator)
			if err != nil {
				b.Fatal(err)
			}
			// warm up: trigger project cache creation
			warmupResp := mockFGA.CheckResponse
			mockFGA.CheckResponse = []*descope.FGACheck{{Allowed: true, Relation: fixedRelations[0], Info: &descope.FGACheckInfo{Direct: true}}}
			mockFGA.CheckAssert = nil
			_, _ = ac.Check(ctx, fixedRelations[:1])
			mockFGA.CheckResponse = warmupResp
			mockFGA.CheckAssert = func(_ []*descope.FGARelation) {
				time.Sleep(time.Millisecond)
			}

			var counter atomic.Int64
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					id := counter.Add(1)
					// first half: fixed relations (cache hits), second half: unique (always misses)
					rels := make([]*descope.FGARelation, numRelations)
					copy(rels[:halfIdx], fixedRelations)
					for j := halfIdx; j < numRelations; j++ {
						rels[j] = &descope.FGARelation{
							Resource:     fmt.Sprintf("r-%d-%d", id, j),
							Target:       fmt.Sprintf("t-%d-%d", id, j),
							Relation:     "viewer",
							ResourceType: "doc",
						}
					}
					_, _ = ac.Check(ctx, rels)
				}
			})
		})
	}
}
