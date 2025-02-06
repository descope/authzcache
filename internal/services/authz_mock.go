package services

import (
	"context"

	"github.com/descope/go-sdk/descope"
)

var _ AuthzCache = &AuthzCacheMock{} // ensure AuthzMock implements Authz

type AuthzCacheMock struct {
	GetSchemaFunc          func() *descope.FGASchema
	CheckFunc              func(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error)
	CreateFGARelationsFunc func(ctx context.Context, relations []*descope.FGARelation) error
	CreateFGASchemaFunc    func(ctx context.Context, dsl string) error
	DeleteFGARelationsFunc func(ctx context.Context, relations []*descope.FGARelation) error
}

func (a *AuthzCacheMock) Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error) {
	return a.CheckFunc(ctx, relations)
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
