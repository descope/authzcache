package controllers

import (
	"context"

	"github.com/descope/authzcache/internal/services"
	se "github.com/descope/authzcache/pkg/authzcache/errors"
	authczv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
)

type authzController struct {
	authzCache services.AuthzCache
	authczv1.UnsafeAuthzCacheServer
}

func New(authzCache services.AuthzCache) *authzController {
	return &authzController{authzCache: authzCache}
}

func (ac *authzController) CreateFGASchema(ctx context.Context, req *authczv1.SaveDSLSchemaRequest) (*authczv1.SaveDSLSchemaResponse, error) {
	cctx.Logger(ctx).Info().Msg("Saving authz DSL schema")

	err := ac.authzCache.CreateFGASchema(ctx, req.Dsl)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authczv1.SaveDSLSchemaResponse{}, err
}

func (ac *authzController) CreateFGARelations(ctx context.Context, req *authczv1.CreateTuplesRequest) (*authczv1.CreateTuplesResponse, error) {
	cctx.Logger(ctx).Info().Msg("Creating authz tuples")
	relations := relationsFromTuples(req.Tuples)

	err := ac.authzCache.CreateFGARelations(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authczv1.CreateTuplesResponse{}, nil
}

func (ac *authzController) DeleteFGARelations(ctx context.Context, req *authczv1.DeleteTuplesRequest) (*authczv1.DeleteTuplesResponse, error) {
	cctx.Logger(ctx).Info().Msg("Deleting authz tuples")
	relations := relationsFromTuples(req.Tuples)

	err := ac.authzCache.DeleteFGARelations(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authczv1.DeleteTuplesResponse{}, nil
}

func (ac *authzController) Check(ctx context.Context, req *authczv1.CheckRequest) (*authczv1.CheckResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking authz")
	relations := relationsFromTuples(req.Tuples)
	var extraContext map[string]any
	if req.Context != nil {
		extraContext = req.Context.AsMap()
	}

	checks, err := ac.authzCache.Check(ctx, relations, extraContext)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	responseTuples := make([]*authczv1.CheckResponseTuple, len(checks))
	for i := range checks {
		check := checks[i]
		responseTuples[i] = &authczv1.CheckResponseTuple{
			Tuple: &authczv1.Tuple{
				Resource:     check.Relation.Resource,
				ResourceType: check.Relation.ResourceType,
				Relation:     check.Relation.Relation,
				Target:       check.Relation.Target,
				TargetType:   check.Relation.TargetType,
			},
			Allowed: check.Allowed,
			Info: &authczv1.CheckResponseInfo{
				Direct:         check.Info.Direct,
				Conditional:    check.Info.Conditional,
				MissingContext: check.Info.MissingContext,
				ConditionalErr: check.Info.ConditionalErr,
			},
		}
	}

	return &authczv1.CheckResponse{Tuples: responseTuples}, nil
}

func (ac *authzController) WhoCanAccess(ctx context.Context, req *authczv1.WhoCanAccessRequest) (*authczv1.WhoCanAccessResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking who can access")
	var extraContext map[string]any
	if req.Context != nil {
		extraContext = req.Context.AsMap()
	}
	targets, err := ac.authzCache.WhoCanAccess(ctx, req.Resource, req.RelationDefinition, req.Namespace, extraContext)
	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authczv1.WhoCanAccessResponse{Targets: targets}, nil
}

func (ac *authzController) WhatCanTargetAccess(ctx context.Context, req *authczv1.WhatCanTargetAccessRequest) (*authczv1.WhatCanTargetAccessResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking what can target access")
	var extraContext map[string]any
	if req.Context != nil {
		extraContext = req.Context.AsMap()
	}
	relations, err := ac.authzCache.WhatCanTargetAccess(ctx, req.Target, extraContext)
	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	protoRelations := make([]*authczv1.Relation, len(relations))
	for i, r := range relations {
		protoRelations[i] = relationFromAuthzRelation(r)
	}
	return &authczv1.WhatCanTargetAccessResponse{Relations: protoRelations}, nil
}

func relationFromAuthzRelation(r *descope.AuthzRelation) *authczv1.Relation {
	return &authczv1.Relation{
		Resource:                             r.Resource,
		RelationDefinition:                   r.RelationDefinition,
		Namespace:                            r.Namespace,
		Target:                               r.Target,
		TargetSetResource:                    r.TargetSetResource,
		TargetSetRelationDefinition:          r.TargetSetRelationDefinition,
		TargetSetRelationDefinitionNamespace: r.TargetSetRelationDefinitionNamespace,
	}
}

func relationsFromTuples(tuples []*authczv1.Tuple) []*descope.FGARelation {
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
