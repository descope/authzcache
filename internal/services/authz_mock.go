package services

import (
	"context"

	"github.com/descope/go-sdk/descope"
)

var _ AuthzCache = &AuthzCacheMock{} // ensure AuthzMock implements Authz

type AuthzCacheMock struct {
	GetSchemaFunc           func() *descope.FGASchema
	CheckFunc               func(ctx context.Context, relations []*descope.FGARelation, extraContext map[string]any) ([]*descope.FGACheck, error)
	CreateFGARelationsFunc  func(ctx context.Context, relations []*descope.FGARelation) error
	CreateFGASchemaFunc     func(ctx context.Context, dsl string) error
	DeleteFGARelationsFunc  func(ctx context.Context, relations []*descope.FGARelation) error
	WhoCanAccessFunc        func(ctx context.Context, resource, relationDefinition, namespace string, extraContext map[string]any) ([]string, error)
	WhatCanTargetAccessFunc func(ctx context.Context, target string, extraContext map[string]any) ([]*descope.AuthzRelation, error)
}

func (a *AuthzCacheMock) Check(ctx context.Context, relations []*descope.FGARelation, extraContext map[string]any) ([]*descope.FGACheck, error) {
	return a.CheckFunc(ctx, relations, extraContext)
}

func (a *AuthzCacheMock) CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	return a.CreateFGARelationsFunc(ctx, relations)
}

func (a *AuthzCacheMock) CreateFGASchema(ctx context.Context, dsl string) error {
	return a.CreateFGASchemaFunc(ctx, dsl)
}

func (a *AuthzCacheMock) DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	return a.DeleteFGARelationsFunc(ctx, relations)
}

func (a *AuthzCacheMock) WhoCanAccess(ctx context.Context, resource, relationDefinition, namespace string, extraContext map[string]any) ([]string, error) {
	if a.WhoCanAccessFunc != nil {
		return a.WhoCanAccessFunc(ctx, resource, relationDefinition, namespace, extraContext)
	}
	return nil, nil
}

func (a *AuthzCacheMock) WhatCanTargetAccess(ctx context.Context, target string, extraContext map[string]any) ([]*descope.AuthzRelation, error) {
	if a.WhatCanTargetAccessFunc != nil {
		return a.WhatCanTargetAccessFunc(ctx, target, extraContext)
	}
	return nil, nil
}
