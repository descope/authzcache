package controllers

import (
	"context"
	"strings"

	"github.com/descope/authzcache/internal/services"
	"github.com/descope/authzcache/internal/services/remote"
	se "github.com/descope/authzcache/pkg/authzcache/errors"
	authczv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	authzv1 "github.com/descope/backend/authzservice/pkg/authzservice/proto/v1"
	cctx "github.com/descope/backend/common/pkg/common/context"
	ce "github.com/descope/backend/common/pkg/common/errors"
	"github.com/descope/go-sdk/descope"
	"google.golang.org/grpc/metadata"
)

const bearerPrefix = "Bearer "

// withCallerKey sets the caller's management key on ctx, so callers never act with the cache's own key
func withCallerKey(ctx context.Context) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	for _, v := range md.Get("authorization") {
		if _, key, ok := strings.Cut(strings.TrimPrefix(v, bearerPrefix), ":"); ok && key != "" {
			return remote.WithCallerKey(ctx, key), nil
		}
	}
	return ctx, ce.MissingAuthorization.New(ctx)
}

type authzController struct {
	authzCache services.AuthzCache
	authczv1.UnsafeAuthzCacheServer
}

func New(authzCache services.AuthzCache) *authzController {
	return &authzController{authzCache: authzCache}
}

func (ac *authzController) CreateFGASchema(ctx context.Context, req *authzv1.SaveDSLSchemaRequest) (*authzv1.SaveDSLSchemaResponse, error) {
	cctx.Logger(ctx).Info().Msg("Saving authz DSL schema")

	ctx, err := withCallerKey(ctx)
	if err != nil {
		return nil, err
	}
	err = ac.authzCache.CreateFGASchema(ctx, req.Dsl)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authzv1.SaveDSLSchemaResponse{}, err
}

func (ac *authzController) CreateFGARelations(ctx context.Context, req *authzv1.CreateTuplesRequest) (*authzv1.CreateTuplesResponse, error) {
	cctx.Logger(ctx).Info().Msg("Creating authz tuples")
	relations := relationsFromTuples(req.Tuples)

	ctx, err := withCallerKey(ctx)
	if err != nil {
		return nil, err
	}
	err = ac.authzCache.CreateFGARelations(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authzv1.CreateTuplesResponse{}, nil
}

func (ac *authzController) DeleteFGARelations(ctx context.Context, req *authzv1.DeleteTuplesRequest) (*authzv1.DeleteTuplesResponse, error) {
	cctx.Logger(ctx).Info().Msg("Deleting authz tuples")
	relations := relationsFromTuples(req.Tuples)

	ctx, err := withCallerKey(ctx)
	if err != nil {
		return nil, err
	}
	err = ac.authzCache.DeleteFGARelations(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authzv1.DeleteTuplesResponse{}, nil
}

func (ac *authzController) Check(ctx context.Context, req *authzv1.CheckRequest) (*authzv1.CheckResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking authz")
	relations := relationsFromTuples(req.Tuples)
	var extraContext map[string]any
	if req.Context != nil {
		extraContext = req.Context.AsMap()
	}

	ctx, err := withCallerKey(ctx)
	if err != nil {
		return nil, err
	}
	checks, err := ac.authzCache.Check(ctx, relations, extraContext)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	responseTuples := make([]*authzv1.CheckResponseTuple, len(checks))
	for i := range checks {
		check := checks[i]
		responseTuples[i] = &authzv1.CheckResponseTuple{
			Tuple: &authzv1.Tuple{
				Resource:     check.Relation.Resource,
				ResourceType: check.Relation.ResourceType,
				Relation:     check.Relation.Relation,
				Target:       check.Relation.Target,
				TargetType:   check.Relation.TargetType,
			},
			Allowed: check.Allowed,
			Info: &authzv1.CheckResponseInfo{
				Direct:         check.Info.Direct,
				Conditional:    check.Info.Conditional,
				MissingContext: check.Info.MissingContext,
				ConditionalErr: check.Info.ConditionalErr,
			},
		}
	}

	return &authzv1.CheckResponse{Tuples: responseTuples}, nil
}

func (ac *authzController) WhoCanAccess(ctx context.Context, req *authzv1.WhoCanAccessRequest) (*authzv1.WhoCanAccessResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking who can access")
	var extraContext map[string]any
	if req.Context != nil {
		extraContext = req.Context.AsMap()
	}
	ctx, err := withCallerKey(ctx)
	if err != nil {
		return nil, err
	}
	targets, err := ac.authzCache.WhoCanAccess(ctx, req.Resource, req.RelationDefinition, req.Namespace, extraContext)
	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authzv1.WhoCanAccessResponse{Targets: targets}, nil
}

func (ac *authzController) WhatCanTargetAccess(ctx context.Context, req *authzv1.WhatCanTargetAccessRequest) (*authzv1.WhatCanTargetAccessResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking what can target access")
	var extraContext map[string]any
	if req.Context != nil {
		extraContext = req.Context.AsMap()
	}
	ctx, err := withCallerKey(ctx)
	if err != nil {
		return nil, err
	}
	relations, err := ac.authzCache.WhatCanTargetAccess(ctx, req.Target, extraContext)
	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	protoRelations := make([]*authzv1.Relation, len(relations))
	for i, r := range relations {
		protoRelations[i] = relationFromAuthzRelation(r)
	}
	return &authzv1.WhatCanTargetAccessResponse{Relations: protoRelations}, nil
}

func relationFromAuthzRelation(r *descope.AuthzRelation) *authzv1.Relation {
	return &authzv1.Relation{
		Resource:                             r.Resource,
		RelationDefinition:                   r.RelationDefinition,
		Namespace:                            r.Namespace,
		Target:                               r.Target,
		TargetSetResource:                    r.TargetSetResource,
		TargetSetRelationDefinition:          r.TargetSetRelationDefinition,
		TargetSetRelationDefinitionNamespace: r.TargetSetRelationDefinitionNamespace,
	}
}

func relationsFromTuples(tuples []*authzv1.Tuple) []*descope.FGARelation {
	relations := make([]*descope.FGARelation, len(tuples))
	for i, t := range tuples {
		relations[i] = &descope.FGARelation{
			Resource:     t.Resource,
			ResourceType: t.ResourceType,
			Relation:     t.Relation,
			Target:       t.Target,
			TargetType:   t.TargetType,
		}
	}
	return relations
}
