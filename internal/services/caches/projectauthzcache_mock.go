package caches

import (
	"context"

	"github.com/descope/go-sdk/descope"
)

var _ ProjectAuthzCache = &ProjectAuthzCacheMock{} // ensure ProjectAuthzCacheMock implements ProjectAuthzCache

type ProjectAuthzCacheMock struct {
	GetSchemaFunc                       func() *descope.FGASchema
	CheckRelationFunc                   func(ctx context.Context, r *descope.FGARelation) (allowed bool, direct bool, ok bool)
	UpdateCacheWithSchemaFunc           func(ctx context.Context, schema *descope.FGASchema)
	UpdateCacheWithAddedRelationsFunc   func(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithDeletedRelationsFunc func(ctx context.Context, relations []*descope.FGARelation)
	UpdateCacheWithChecksFunc           func(ctx context.Context, sdkChecks []*descope.FGACheck)
	StartRemoteChangesPollingFunc       func(ctx context.Context)
	GetWhoCanAccessCachedFunc           func(ctx context.Context, resource, relationDefinition, namespace string) ([]string, bool)
	SetWhoCanAccessCachedFunc           func(ctx context.Context, resource, relationDefinition, namespace string, targets []string)
	GetWhatCanTargetAccessCachedFunc    func(ctx context.Context, target string) ([]*descope.AuthzRelation, bool)
	SetWhatCanTargetAccessCachedFunc    func(ctx context.Context, target string, relations []*descope.AuthzRelation)
	InvalidateLookupCacheFunc           func(ctx context.Context)
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

func (m *ProjectAuthzCacheMock) GetWhoCanAccessCached(ctx context.Context, resource, relationDefinition, namespace string) ([]string, bool) {
	if m.GetWhoCanAccessCachedFunc != nil {
		return m.GetWhoCanAccessCachedFunc(ctx, resource, relationDefinition, namespace)
	}
	return nil, false
}

func (m *ProjectAuthzCacheMock) SetWhoCanAccessCached(ctx context.Context, resource, relationDefinition, namespace string, targets []string) {
	if m.SetWhoCanAccessCachedFunc != nil {
		m.SetWhoCanAccessCachedFunc(ctx, resource, relationDefinition, namespace, targets)
	}
}

func (m *ProjectAuthzCacheMock) GetWhatCanTargetAccessCached(ctx context.Context, target string) ([]*descope.AuthzRelation, bool) {
	if m.GetWhatCanTargetAccessCachedFunc != nil {
		return m.GetWhatCanTargetAccessCachedFunc(ctx, target)
	}
	return nil, false
}

func (m *ProjectAuthzCacheMock) SetWhatCanTargetAccessCached(ctx context.Context, target string, relations []*descope.AuthzRelation) {
	if m.SetWhatCanTargetAccessCachedFunc != nil {
		m.SetWhatCanTargetAccessCachedFunc(ctx, target, relations)
	}
}

func (m *ProjectAuthzCacheMock) InvalidateLookupCache(ctx context.Context) {
	if m.InvalidateLookupCacheFunc != nil {
		m.InvalidateLookupCacheFunc(ctx)
	}
}
