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

func injectAuthzMocks(t *testing.T) (*AuthzCache, *mocksmgmt.MockManagement, *mocks.ProjectAuthzCacheMock) {
	mockSDK := &mocksmgmt.MockManagement{
		MockFGA:   &mocksmgmt.MockFGA{},
		MockAuthz: &mocksmgmt.MockAuthz{},
	}
	mockCache := &mocks.ProjectAuthzCacheMock{}
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
}
