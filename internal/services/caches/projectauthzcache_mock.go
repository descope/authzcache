package caches

import (
	"context"

	"github.com/descope/go-sdk/descope"
)

var _ ProjectAuthzCache = &ProjectAuthzCacheMock{} // ensure ProjectAuthzCacheMock implements ProjectAuthzCache

// ProjectAuthzCacheMock is a mock type for the ProjectAuthzCacheMock type
type ProjectAuthzCacheMock struct {
	GetSchemaFunc                       func() *descope.FGASchema
	CheckRelationFunc                   func(ctx context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool)
	UpdateCacheWithSchemaFunc           func(ctx context.Context, schema *descope.FGASchema)
	UpdateCacheWithAddedRelationsFunc   func(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithDeletedRelationsFunc func(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithChecksFunc           func(ctx context.Context, sdkChecks []*descope.FGACheck)
	StartRemoteChangesPollingFunc       func(ctx context.Context)
}

func (m *ProjectAuthzCacheMock) GetSchema() *descope.FGASchema {
	return m.GetSchemaFunc() // notest
}

func (m *ProjectAuthzCacheMock) CheckRelation(ctx context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool) {
	return m.CheckRelationFunc(ctx, r)
}

func (m *ProjectAuthzCacheMock) UpdateCacheWithSchema(ctx context.Context, schema *descope.FGASchema) {
	m.UpdateCacheWithSchemaFunc(ctx, schema)
}

func (m *ProjectAuthzCacheMock) UpdateCacheWithAddedRelations(ctx context.Context, relations []*descope.FGARelation) {
	m.UpdateCacheWithAddedRelationsFunc(ctx, relations)
}

func (m *ProjectAuthzCacheMock) UpdateCacheWithDeletedRelations(ctx context.Context, relations []*descope.FGARelation) {
	m.UpdateCacheWithDeletedRelationsFunc(ctx, relations)
}

func (m *ProjectAuthzCacheMock) UpdateCacheWithChecks(ctx context.Context, sdkChecks []*descope.FGACheck) {
	m.UpdateCacheWithChecksFunc(ctx, sdkChecks)
}

func (m *ProjectAuthzCacheMock) StartRemoteChangesPolling(ctx context.Context) {
	m.StartRemoteChangesPollingFunc(ctx)
}
