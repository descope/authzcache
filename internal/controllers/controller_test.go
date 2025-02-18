package controllers

import (
	"context"
	"testing"

	"github.com/descope/authzcache/internal/services"
	authzv1 "github.com/descope/authzservice/pkg/authzservice/proto/v1"
	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup() (*authzController, *services.AuthzCacheMock) {
	mockAuthzCache := &services.AuthzCacheMock{}
	return New(mockAuthzCache), mockAuthzCache
}

func TestCreateFGASchema(t *testing.T) {
	controller, mockAuthzCache := setup()
	var authzCalled bool
	mockAuthzCache.CreateFGASchemaFunc = func(_ context.Context, dsl string) error {
		require.Equal(t, "test-dsl", dsl)
		authzCalled = true
		return nil
	}

	req := &authzv1.SaveDSLSchemaRequest{Dsl: "test-dsl"}
	resp, err := controller.CreateFGASchema(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, authzCalled)
}

func TestCreateFGASchemaError(t *testing.T) {
	controller, mockAuthzCache := setup()
	mockAuthzCache.CreateFGASchemaFunc = func(_ context.Context, _ string) error {
		return assert.AnError
	}

	req := &authzv1.SaveDSLSchemaRequest{Dsl: "test-dsl"}
	resp, err := controller.CreateFGASchema(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCreateFGARelations(t *testing.T) {
	controller, mockAuthzCache := setup()
	var authzCalled bool
	tuples := []*authzv1.Tuple{
		{Resource: "testR", Target: "testT", ResourceType: "testRT", Relation: "testRel", TargetType: "testTT"},
	}
	mockAuthzCache.CreateFGARelationsFunc = func(_ context.Context, relations []*descope.FGARelation) error {
		authzCalled = true
		require.Equal(t, 1, len(relations))
		require.Equal(t, tuples[0].Resource, relations[0].Resource)
		require.Equal(t, tuples[0].Target, relations[0].Target)
		require.Equal(t, tuples[0].ResourceType, relations[0].ResourceType)
		require.Equal(t, tuples[0].Relation, relations[0].Relation)
		require.Equal(t, tuples[0].TargetType, relations[0].TargetType)
		return nil
	}

	req := &authzv1.CreateTuplesRequest{Tuples: tuples}
	resp, err := controller.CreateFGARelations(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, authzCalled)
}

func TestCreateFGARelationsError(t *testing.T) {
	controller, mockAuthzCache := setup()
	mockAuthzCache.CreateFGARelationsFunc = func(_ context.Context, _ []*descope.FGARelation) error {
		return assert.AnError
	}

	req := &authzv1.CreateTuplesRequest{Tuples: []*authzv1.Tuple{}}
	resp, err := controller.CreateFGARelations(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestDeleteFGARelations(t *testing.T) {
	controller, mockAuthzCache := setup()
	var authzCalled bool
	tuples := []*authzv1.Tuple{
		{Resource: "testR", Target: "testT", ResourceType: "testRT", Relation: "testRel", TargetType: "testTT"},
	}
	mockAuthzCache.DeleteFGARelationsFunc = func(_ context.Context, relations []*descope.FGARelation) error {
		require.Equal(t, 1, len(relations))
		require.Equal(t, tuples[0].Resource, relations[0].Resource)
		require.Equal(t, tuples[0].Target, relations[0].Target)
		require.Equal(t, tuples[0].ResourceType, relations[0].ResourceType)
		require.Equal(t, tuples[0].Relation, relations[0].Relation)
		require.Equal(t, tuples[0].TargetType, relations[0].TargetType)
		authzCalled = true
		return nil
	}

	req := &authzv1.DeleteTuplesRequest{Tuples: tuples}
	resp, err := controller.DeleteFGARelations(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, authzCalled)
}

func TestDeleteFGARelationsError(t *testing.T) {
	controller, mockAuthzCache := setup()
	mockAuthzCache.DeleteFGARelationsFunc = func(_ context.Context, _ []*descope.FGARelation) error {
		return assert.AnError
	}

	req := &authzv1.DeleteTuplesRequest{Tuples: []*authzv1.Tuple{}}
	resp, err := controller.DeleteFGARelations(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestCheck(t *testing.T) {
	controller, mockAuthzCache := setup()
	var authzCalled bool
	tuples := []*authzv1.Tuple{
		{Resource: "testR", Target: "testT", ResourceType: "testRT", Relation: "testRel", TargetType: "testTT"},
	}
	mockAuthzCache.CheckFunc = func(_ context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error) {
		require.Equal(t, 1, len(relations))
		require.Equal(t, tuples[0].Resource, relations[0].Resource)
		require.Equal(t, tuples[0].Target, relations[0].Target)
		require.Equal(t, tuples[0].ResourceType, relations[0].ResourceType)
		require.Equal(t, tuples[0].Relation, relations[0].Relation)
		require.Equal(t, tuples[0].TargetType, relations[0].TargetType)
		authzCalled = true
		return []*descope.FGACheck{
			{Allowed: true, Relation: relations[0], Info: &descope.FGACheckInfo{Direct: true}},
		}, nil
	}

	req := &authzv1.CheckRequest{Tuples: tuples}
	resp, err := controller.Check(context.Background(), req)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, authzCalled)
	require.Equal(t, 1, len(resp.Tuples))
	require.Equal(t, tuples[0].Resource, resp.Tuples[0].Tuple.Resource)
	require.Equal(t, tuples[0].Target, resp.Tuples[0].Tuple.Target)
	require.Equal(t, tuples[0].ResourceType, resp.Tuples[0].Tuple.ResourceType)
	require.Equal(t, tuples[0].Relation, resp.Tuples[0].Tuple.Relation)
	require.Equal(t, tuples[0].TargetType, resp.Tuples[0].Tuple.TargetType)
	require.True(t, resp.Tuples[0].Allowed)
	require.True(t, resp.Tuples[0].Info.Direct)
}

func TestCheckError(t *testing.T) {
	controller, mockAuthzCache := setup()
	mockAuthzCache.CheckFunc = func(_ context.Context, _ []*descope.FGARelation) ([]*descope.FGACheck, error) {
		return nil, assert.AnError
	}

	req := &authzv1.CheckRequest{Tuples: []*authzv1.Tuple{}}
	resp, err := controller.Check(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
}
