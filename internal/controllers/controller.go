package controllers

import (
	"context"
	"os"

	"github.com/descope/authzcache/internal/services"
	se "github.com/descope/authzcache/pkg/authzcache/errors"
	authczv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	authzv1 "github.com/descope/authzservice/pkg/authzservice/proto/v1"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
)

type authzController struct {
	authzCache *services.AuthzCache
	authczv1.UnsafeAuthzCacheServer
	sdkClient *client.DescopeClient // TODO: only used for scaffolidng, remove from controller
}

func New(authzCache *services.AuthzCache) *authzController {
	// Leave projectId param empty to get it from DESCOPE_PROJECT_ID env variable
	baseUrl := os.Getenv(descope.EnvironmentVariableBaseURL) // TODO: used for testing inside descope local env, should probably be removed
	// TODO: sdk defined here is only used for scaffolidng, should be moved into service layer
	descopeClient, err := client.NewWithConfig(&client.Config{
		SessionJWTViaCookie: true,
		DescopeBaseURL:      baseUrl,
	})
	if err != nil {
		panic(err)
	}
	return &authzController{authzCache: authzCache, sdkClient: descopeClient}
}

func (ac *authzController) CreateFGASchema(ctx context.Context, req *authzv1.SaveDSLSchemaRequest) (*authzv1.SaveDSLSchemaResponse, error) {
	cctx.Logger(ctx).Info().Msg("Saving authz DSL schema")

	err := ac.sdkClient.Management.FGA().SaveSchema(ctx, &descope.FGASchema{Schema: req.Dsl})
	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}

	return &authzv1.SaveDSLSchemaResponse{}, err
}

func (ac *authzController) CreateFGARelations(ctx context.Context, req *authzv1.CreateTuplesRequest) (*authzv1.CreateTuplesResponse, error) {
	cctx.Logger(ctx).Info().Msg("Creating authz tuples")
	var relations []*descope.FGARelation
	for _, t := range req.Tuples {
		relations = append(relations, &descope.FGARelation{
			Resource:     t.Resource,
			ResourceType: t.ResourceType,
			Relation:     t.Relation,
			Target:       t.Target,
			TargetType:   t.TargetType,
		})
	}

	err := ac.sdkClient.Management.FGA().CreateRelations(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authzv1.CreateTuplesResponse{}, nil
}

func (ac *authzController) DeleteFGARelations(ctx context.Context, req *authzv1.DeleteTuplesRequest) (*authzv1.DeleteTuplesResponse, error) {
	cctx.Logger(ctx).Info().Msg("Deleting authz tuples")
	var relations []*descope.FGARelation
	for _, t := range req.Tuples {
		relations = append(relations, &descope.FGARelation{
			Resource:     t.Resource,
			ResourceType: t.ResourceType,
			Relation:     t.Relation,
			Target:       t.Target,
			TargetType:   t.TargetType,
		})
	}

	err := ac.sdkClient.Management.FGA().DeleteRelations(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	return &authzv1.DeleteTuplesResponse{}, nil
}

func (ac *authzController) Check(ctx context.Context, req *authzv1.CheckRequest) (*authzv1.CheckResponse, error) {
	cctx.Logger(ctx).Info().Msg("Checking authz")
	var relations []*descope.FGARelation
	for _, t := range req.Tuples {
		relations = append(relations, &descope.FGARelation{
			Resource:     t.Resource,
			ResourceType: t.ResourceType,
			Relation:     t.Relation,
			Target:       t.Target,
			TargetType:   t.TargetType,
		})
	}

	checks, err := ac.sdkClient.Management.FGA().Check(ctx, relations)

	if err != nil {
		return nil, se.ServiceErrorFromSdkError(ctx, err)
	}
	responseTuples := make([]*authzv1.CheckResponseTuple, len(checks))
	for i := range req.Tuples {
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
		}
	}

	return &authzv1.CheckResponse{Tuples: responseTuples}, nil
}

// func (ac *authzController) LoadDSLSchema(ctx context.Context, _ *authzv1.LoadDSLSchemaRequest) (*authzv1.LoadDSLSchemaResponse, error) {
// 	cctx.Logger(ctx).Info().Msg("Loading authz DSL schema")
// 	res, err := ac.authzService.LoadDSLSchema(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &authzv1.LoadDSLSchemaResponse{Dsl: res}, nil
// }

// func (ac *authzController) ListTuples(ctx context.Context, req *authzv1.ListTuplesRequest) (*authzv1.ListTuplesResponse, error) {
// 	cctx.Logger(ctx).Info().Msg("Listing authz tuples")
// 	res, err := ac.authzService.ListRelations(ctx, int(req.Page), int(req.PageSize))
// 	if err != nil {
// 		return nil, err
// 	}
// 	response := &authzv1.ListTuplesResponse{}
// 	for _, r := range res.Relations {
// 		t := &authzv1.Tuple{
// 			Resource:     r.Resource,
// 			ResourceType: r.Namespace,
// 			Relation:     r.RelationDefinition,
// 			Target:       r.Target,
// 			TargetType:   r.TargetNamespace,
// 		}
// 		if r.TargetSetResource != "" {
// 			t.Target = r.TargetSetResource
// 		}

// 		if vErr := ac.authzService.ValidateTupleWithModel(ctx, t, false); vErr == nil {
// 			response.Tuples = append(response.Tuples, t)
// 		}
// 	}
// 	return response, nil
// }

// func (ac *authzController) ExportFGASchema(ctx context.Context, _ *authzv1.ExportFGASchemaRequest) (*authzv1.ExportedFGASchema, error) {
// 	cctx.Logger(ctx).Info().Msg("Exporting authz schema")
// 	res, err := ac.authzService.LoadDSLSchema(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return &authzv1.ExportedFGASchema{Schema: res}, nil
// }

// func (ac *authzController) ImportFGASchema(ctx context.Context, req *authzv1.ImportFGASchemaRequest) (*authzv1.ImportFGASchemaResponse, error) {
// 	cctx.Logger(ctx).Info().Msg("Importing authz schema")
// 	if strings.TrimSpace(req.Schema) == "" {
// 		err := ac.authzService.DeleteSchema(ctx)
// 		if err != nil {
// 			return nil, err // notest
// 		}
// 		return &authzv1.ImportFGASchemaResponse{}, nil
// 	}
// 	err := ac.authzService.SaveDSLSchema(ctx, req.Schema)
// 	if err != nil {
// 		return nil, err // notest
// 	}
// 	return &authzv1.ImportFGASchemaResponse{}, nil
// }
