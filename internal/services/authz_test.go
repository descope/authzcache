package services

import (
	"context"
	"testing"

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
