package services

import (
	"context"
	"testing"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/descope/authzcache/internal/services/mocks"
	"github.com/descope/go-sdk/descope"
	mocksmgmt "github.com/descope/go-sdk/descope/tests/mocks/mgmt"
	"github.com/stretchr/testify/require"
)

type mockCache struct {
	mocks.ProjectAuthzCacheMock
	pollingStarted bool
}

type mockCacheCreator struct {
	mockCache caches.ProjectAuthzCache
}

func (m mockCacheCreator) NewProjectAuthzCache(_ context.Context, _ caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error) {
	return m.mockCache, nil
}

func TestNewAuthzCache(t *testing.T) {
	mockSdk := &mocksmgmt.MockManagement{}
	mockCacheCreator := &mockCacheCreator{}
	ac, err := New(context.TODO(), mockSdk, mockCacheCreator)
	require.NoError(t, err)
	require.NotNil(t, ac)
	require.NotNil(t, ac.mgmtSdk)
	require.NotNil(t, ac.projectAuthzCaches)
}

func injectAuthzMocks(t *testing.T) (*AuthzCache, *mocksmgmt.MockManagement, *mockCache) {
	mockSDK := &mocksmgmt.MockManagement{
		MockFGA:   &mocksmgmt.MockFGA{},
		MockAuthz: &mocksmgmt.MockAuthz{},
	}
	mockCache := &mockCache{}
	mockCache.StartRemoteChangesPollingFunc = func(_ context.Context) {
		mockCache.pollingStarted = true
	}
	mockProjectCacheCreator := &mockCacheCreator{mockCache: mockCache}
	// create AuthzCache with mocks
	ac, err := New(context.TODO(), mockSDK, mockProjectCacheCreator)
	require.NoError(t, err)
	return ac, mockSDK, mockCache
}

func TestCreateFGASchema(t *testing.T) {
	// setup mocks
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	cacheUpdateCallCount := 0
	sdkUpdateCallCount := 0
	dsl := "schema definition"
	mockSDK.MockFGA.SaveSchemaAssert = func(schema *descope.FGASchema) error {
		require.Equal(t, dsl, schema.Schema)
		sdkUpdateCallCount++
		return nil
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
	mockSDK.MockFGA.CreateRelationsAssert = func(rels []*descope.FGARelation) error {
		require.Equal(t, relations, rels)
		sdkUpdateCallCount++
		return nil
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
	mockSDK.MockFGA.CreateRelationsAssert = func(_ []*descope.FGARelation) error {
		require.Fail(t, "should not be called")
		return nil
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
	mockSDK.MockFGA.DeleteRelationsAssert = func(rels []*descope.FGARelation) error {
		require.Equal(t, relations, rels)
		sdkUpdateCallCount++
		return nil
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
	mockSDK.MockFGA.DeleteRelationsAssert = func(_ []*descope.FGARelation) error {
		require.Fail(t, "should not be called")
		return nil
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
	mockCache.CheckRelationFunc = func(_ context.Context, _ *descope.FGARelation) (allowed bool, ok bool) {
		require.Fail(t, "should not be called")
		return false, false
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
		},
		{
			Allowed:  false,
			Relation: relations[1],
		},
	}
	mockSDK.MockFGA.CheckAssert = func(_ []*descope.FGARelation) {
		require.Fail(t, "should not be called")
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
		if r.Resource == "mario" {
			return true, true
		}
		return false, true
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
		},
		{
			Allowed:  false,
			Relation: relations[1],
		},
	}
	mockSDK.MockFGA.CheckAssert = func(rels []*descope.FGARelation) {
		require.Equal(t, relations, rels)
	}
	mockCache.CheckRelationFunc = func(_ context.Context, _ *descope.FGARelation) (allowed bool, ok bool) {
		return false, false
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
		},
		{
			Allowed:  false,
			Relation: relations[1],
		},
		{
			Allowed:  true,
			Relation: relations[2],
		},
	}
	mockSDK.MockFGA.CheckAssert = func(rels []*descope.FGARelation) {
		require.Equal(t, expectedSdkRelations, rels)
	}
	mockCache.CheckRelationFunc = func(_ context.Context, r *descope.FGARelation) (allowed bool, ok bool) {
		if r.Resource == "luigi" && r.Target == "mario" { // only the second relation is in cache
			return false, true
		}
		return false, false
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
