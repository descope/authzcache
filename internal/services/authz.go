package services

import (
	"context"
	"os"

	// 	"fmt"
	// 	"strings"
	// 	"time"

	// 	"github.com/descope/authzcache/internal/config"
	// 	se "github.com/descope/authzcache/pkg/authzcache/errors"
	// 	"github.com/descope/authzservice/pkg/authzservice/domain"
	// 	"github.com/descope/authzservice/pkg/authzservice/dsl/parser"
	// 	authzv1 "github.com/descope/authzservice/pkg/authzservice/proto/v1"
	"github.com/descope/authzcache/internal/config"
	cctx "github.com/descope/common/pkg/common/context"

	// ce "github.com/descope/common/pkg/common/errors"
	// "github.com/descope/common/pkg/common/featureflags"
	// "github.com/descope/common/pkg/common/messenger"
	// "github.com/descope/common/pkg/common/messenger/events"
	// cutils "github.com/descope/common/pkg/common/utils"
	lru "github.com/descope/common/pkg/common/utils/monitoredlru"
	// "github.com/descope/go-sdk/descope"
	// "google.golang.org/protobuf/types/known/structpb"

	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
	"github.com/descope/go-sdk/descope/logger"
)

type AuthzCache struct {
	schemaCache                      *lru.MonitoredLRUCache[string, *descope.FGASchema] // projectID -> schema
	directRelationCache              *lru.MonitoredLRUCache[string, []string]           // projectID:resource:target -> [relation, relation, ...], e.g. p1:file1:user1 -> [owner, reader]
	indirectAndNegativeRelationCache *lru.MonitoredLRUCache[string, bool]               // projectID:resource:target:relation -> true/false, e.g. p1:file1:user2:owner -> false
	sdkClient                        *client.DescopeClient
}

func New(ctx context.Context) (*AuthzCache, error) {
	cctx.Logger(ctx).Info().Msg("Starting new authz cache")
	// cache init
	schemaCache, err := lru.New[string, *descope.FGASchema](config.GetSchemaCacheSize(), "authz-schemas")
	if err != nil {
		return nil, err
	}
	directRelationCache, err := lru.New[string, []string](config.GetDirectRelationCacheSize(), "authz-direct-relations")
	if err != nil {
		return nil, err
	}
	indirectAndNegativeRelationCache, err := lru.New[string, bool](config.GetInderectAndNegativeRelationCacheSize(), "authz-indirect-and-negative-relations")
	if err != nil {
		return nil, err
	}
	// sdk init
	baseURL := os.Getenv(descope.EnvironmentVariableBaseURL) // TODO: used for testing inside descope local env, should probably be removed
	descopeClient, err := client.NewWithConfig(&client.Config{
		SessionJWTViaCookie: true,
		DescopeBaseURL:      baseURL,
		LogLevel:            logger.LogDebugLevel, // TODO: extract to env var
	})
	if err != nil {
		return nil, err
	}
	// service init
	ac := &AuthzCache{sdkClient: descopeClient, schemaCache: schemaCache, directRelationCache: directRelationCache, indirectAndNegativeRelationCache: indirectAndNegativeRelationCache}
	return ac, nil
}

// func (as *AuthzCache) LoadDSLSchema(ctx context.Context) (string, error) {
// 	panic("unimplemented")
// }

func (a *AuthzCache) CreateFGASchema(ctx context.Context, dsl string) error {
	return a.sdkClient.Management.FGA().SaveSchema(ctx, &descope.FGASchema{Schema: dsl})
}

func (a *AuthzCache) CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	return a.sdkClient.Management.FGA().CreateRelations(ctx, relations)
}

func (a *AuthzCache) DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	return a.sdkClient.Management.FGA().DeleteRelations(ctx, relations)
}

func (a *AuthzCache) Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error) {
	return a.sdkClient.Management.FGA().Check(ctx, relations)
}

// func (as *AuthzCache) ListRelations(ctx context.Context, pageNum, pageSize int) (*authzv1.ResourceRelationsResponse, error) {
// 	panic("unimplemented")
// }

// func (as *AuthzCache) DeleteSchema(ctx context.Context) error {
// 	panic("unimplemented")
// }

// --------------

// func cacheKey(projectID, resourceID, relationDefinitionID string) string {
// 	return projectID + ":*:" + resourceID + ":*:" + relationDefinitionID
// }

// func (ac *AuthzCache) schemaChangedHandler(ctx context.Context) func(*messenger.Event) *messenger.RetryStrategy {
// 	// notest
// 	return func(event *messenger.Event) *messenger.RetryStrategy {
// 		e, ok := event.Data.(*events.AuthzSchemaChangedEventData)
// 		if !ok {
// 			return nil
// 		}
// 		ac.schemaCache.Remove(ctx, e.ProjectID)
// 		return nil
// 	}
// }

// func (ac *AuthzCache) resourceChangedHandler(ctx context.Context) func(*messenger.Event) *messenger.RetryStrategy {
// 	// notest
// 	return func(event *messenger.Event) *messenger.RetryStrategy {
// 		e, ok := event.Data.(*events.AuthzResourceEventData)
// 		if !ok {
// 			return nil
// 		}
// 		ac.relationCache.Remove(ctx, cacheKey(e.ProjectID, e.ResourceID, e.RelationDefinitionID))
// 		if e.RelatedResourceID != "" {
// 			ac.targetRelationCache.Remove(ctx, cacheKey(e.ProjectID, e.RelatedResourceID, e.RelationDefinitionID))
// 		}
// 		return nil
// 	}
// }

// func (ac *AuthzCache) getSchema(ctx context.Context, projectID string) (*domain.Schema, error) {
// 	if s, ok := ac.schemaCache.Get(ctx, projectID); ok {
// 		return s, nil
// 	}
// 	sdkSchema, err := ac.sdkClient.Management.Authz().LoadSchema(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	s, err := cutils.ToType[*descope.AuthzSchema, *domain.Schema](sdkSchema)
// 	if err != nil {
// 		return nil, err
// 	}
// 	fgaSchema, err := ac.sdkClient.Management.FGA().LoadSchema(ctx)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if s != nil {
// 		p := parser.NewParser(fgaSchema.Schema)
// 		s.Model, _ = p.ParseModel()
// 	}
// 	ac.schemaCache.Add(ctx, projectID, s)
// 	return s, nil
// }

// func (ac *AuthzCache) SaveSchema(ctx context.Context, req *authzv1.SaveSchemaRequest, dsl string) error {
// 	projectID := cctx.ProjectID(ctx)
// 	s, err := cutils.ToType[*authzv1.Schema, *descope.AuthzSchema](req.Schema)
// 	if err != nil {
// 		return err
// 	}

// 	if err := ac.sdkClient.Management.Authz().SaveSchema(ctx, s, req.Upgrade); err != nil {
// 		return err // notest
// 	}
// 	ac.schemaCache.Remove(ctx, projectID)
// 	return nil
// }

// func (ac *AuthzCache) DeleteSchema(ctx context.Context) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return err // notest
// 	}
// 	if schema == nil {
// 		return nil // notest
// 	}
// 	if err = ac.sdkClient.Management.Authz().DeleteSchema(ctx); err != nil {
// 		return err
// 	}
// 	ac.schemaCache.Remove(ctx, projectID)
// 	return nil
// }

// func (ac *AuthzCache) LoadSchema(ctx context.Context) (*domain.Schema, error) {
// 	return ac.getSchema(ctx, cctx.ProjectID(ctx))
// }

// func (ac *AuthzCache) SaveNamespace(ctx context.Context, req *authzv1.SaveNamespaceRequest) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	ns, err := cutils.ToType[*authzv1.Namespace, *descope.AuthzNamespace](req.Namespace)
// 	if err != nil {
// 		return err
// 	}
// 	if err := ac.sdkClient.Management.Authz().SaveNamespace(ctx, ns, req.OldName, req.SchemaName); err != nil {
// 		return err // notest
// 	}
// 	ac.schemaCache.Remove(ctx, projectID)
// 	return nil
// }

// func (ac *AuthzCache) DeleteNamespace(ctx context.Context, namespaceName, schemaName string) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return err // notest
// 	}
// 	if schema == nil {
// 		return nil // notest
// 	}
// 	namespace := schema.GetNamespaceByName(namespaceName)
// 	if namespace == nil {
// 		return se.UnknownNamespace.New(namespaceName) // notest
// 	}
// 	for _, rd := range namespace.RelationDefinitions {
// 		dependents := schema.GetDependentRelationsNames(namespace.ID, rd.ID, true)
// 		if len(dependents) > 0 {
// 			return se.NamespaceSave.New(fmt.Sprintf("Dependent relation definitions exist: %s",
// 				strings.Join(cutils.Map(dependents, func(pair []string) string { return pair[0] + "." + pair[1] }), ",")),
// 			)
// 		}
// 	}
// 	if err := ac.sdkClient.Management.Authz().DeleteNamespace(ctx, namespaceName, namespaceName); err != nil {
// 		return err // notest
// 	}
// 	ac.schemaCache.Remove(ctx, projectID)
// 	return nil
// }

// func (ac *AuthzCache) SaveRelationDefinition(ctx context.Context, req *authzv1.SaveRelationDefinitionRequest) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	rd, err := cutils.ToType[*authzv1.RelationDefinition, *descope.AuthzRelationDefinition](req.RelationDefinition)
// 	if err != nil {
// 		return err
// 	}
// 	if err := ac.sdkClient.Management.Authz().SaveRelationDefinition(ctx, rd, req.Namespace, req.OldName, req.SchemaName); err != nil {
// 		return err // notest
// 	}
// 	ac.schemaCache.Remove(ctx, projectID)
// 	return nil
// }

// func (ac *AuthzCache) DeleteRelationDefinition(ctx context.Context, namespaceName, relationDefinitionName, schemaName string) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return err // notest
// 	}
// 	if schema == nil {
// 		return nil // notest
// 	}
// 	rd := schema.GetRelationDefinitionByName(namespaceName, relationDefinitionName)
// 	if rd == nil {
// 		return se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", namespaceName, relationDefinitionName)) // notest
// 	}
// 	ns := schema.GetNamespaceByName(namespaceName)
// 	dependents := schema.GetDependentRelationsNames(ns.ID, rd.ID, false)
// 	if len(dependents) > 0 {
// 		return se.RelationDefinitionSave.New(fmt.Sprintf("Dependent relation definitions exist: %s",
// 			strings.Join(cutils.Map(dependents, func(pair []string) string { return pair[0] + "." + pair[1] }), ",")),
// 		)
// 	}
// 	if err := ac.sdkClient.Management.Authz().DeleteRelationDefinition(ctx, relationDefinitionName, namespaceName, schemaName); err != nil {
// 		return err // notest
// 	}
// 	ac.schemaCache.Remove(ctx, projectID)
// 	return nil
// }

// func convertRelation(schema *domain.Schema, relation *authzv1.Relation) (*descope.AuthzRelation, error) {
// 	r := &descope.AuthzRelation{Resource: relation.Resource}
// 	rd := schema.GetRelationDefinitionByName(relation.Namespace, relation.RelationDefinition)
// 	if rd == nil {
// 		return nil, se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", relation.Namespace, relation.RelationDefinition)) // notest
// 	}
// 	r.RelationDefinition = rd.ID
// 	if relation.Target != "" {
// 		r.Target = relation.Target
// 	} else if relation.TargetSetRelationDefinitionNamespace != "" && relation.TargetSetRelationDefinition != "" && relation.TargetSetResource != "" {
// 		targetSetRD := schema.GetRelationDefinitionByName(relation.TargetSetRelationDefinitionNamespace, relation.TargetSetRelationDefinition)
// 		if targetSetRD == nil {
// 			return nil, se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", relation.TargetSetRelationDefinitionNamespace, relation.TargetSetRelationDefinition)) // notest
// 		}
// 		r.TargetSetRelationDefinition, r.TargetSetResource = targetSetRD.ID, relation.TargetSetResource
// 	} else if relation.Query != nil {

// 		query := &descope.AuthzUserQuery{}
// 		for _, v := range relation.Query.Statuses {
// 			query.Statuses = append(query.Statuses, descope.UserStatus(v))
// 		}
// 		query.Tenants, query.Roles, query.Text, query.SSOOnly, query.WithTestUser, query.CustomAttributes =
// 			relation.Query.Tenants, relation.Query.Roles, relation.Query.Text, relation.Query.SsoOnly, relation.Query.WithTestUser, relation.Query.CustomAttributes.AsMap()
// 		r.Query = query
// 	} else {
// 		return nil, se.RelationSave.New("Unknown target for relation", relation.Namespace, relation.RelationDefinition) // notest
// 	}
// 	return r, nil
// }

// func (ac *AuthzCache) relationCacheEviction(ctx context.Context, projectID, errMessage string, relations []*domain.Relation) {
// 	for _, r := range relations {
// 		ac.relationCache.Remove(ctx, cacheKey(projectID, r.ResourceID, r.RelationDefinitionID))
// 		if r.Target != "" {
// 			ac.targetRelationCache.Remove(ctx, cacheKey(projectID, r.Target, r.RelationDefinitionID))
// 		}
// 	}
// }

// func (ac *AuthzCache) CreateRelations(ctx context.Context, req *authzv1.CreateRelationsRequest) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return se.SchemaDoesNotExist.New() // notest
// 	}
// 	relations := []*descope.AuthzRelation{}
// 	for _, r := range req.Relations {
// 		newR, err := convertRelation(schema, r)
// 		if err != nil {
// 			return err // notest
// 		}
// 		relations = append(relations, newR)
// 	}
// 	if err = ac.sdkClient.Management.Authz().CreateRelations(ctx, relations); err != nil {
// 		return err // notest
// 	}

// 	cctx.Logger(ctx).Info().Int("relations_len", len(relations)).Msg("Created relations")

// 	ac.relationCacheEviction(ctx, projectID, "Error publishing resource eviction from creating relations", cutils.Map(relations, func(r *descope.AuthzRelation) *domain.Relation {
// 		return &domain.Relation{
// 			ResourceID:           r.Resource,
// 			RelationDefinitionID: r.RelationDefinition,
// 			Target:               r.Target,
// 		}
// 	}))
// 	return nil
// }

// func (ac *AuthzCache) DeleteRelations(ctx context.Context, req *authzv1.DeleteRelationsRequest) error {
// 	projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return se.SchemaDoesNotExist.New() // notest
// 	}
// 	relations := []*descope.AuthzRelation{}
// 	for _, r := range req.Relations {
// 		newR, err := convertRelation(schema, r)
// 		if err != nil {
// 			return err // notest
// 		}
// 		relations = append(relations, newR)
// 	}
// 	if err = ac.sdkClient.Management.Authz().DeleteRelations(ctx, relations); err != nil {
// 		return err // notest
// 	}
// 	ac.relationCacheEviction(ctx, projectID, "Error publishing resource eviction from deleting relations", cutils.Map(relations, func(r *descope.AuthzRelation) *domain.Relation {
// 		return &domain.Relation{
// 			ResourceID:           r.Resource,
// 			RelationDefinitionID: r.RelationDefinition,
// 			Target:               r.Target,
// 		}
// 	}))
// 	return nil
// }

// func (ac *AuthzCache) DeleteRelationsForResources(ctx context.Context, resources []string) error {
// 	// projectID := cctx.ProjectID(ctx)
// 	if cctx.HasNoProjectID(ctx) {
// 		return se.UnknownProject.New() // notest
// 	}
// 	err := ac.sdkClient.Management.Authz().DeleteRelationsForResources(ctx, resources)
// 	if err != nil {
// 		return err // notest
// 	}
// 	// ac.relationCacheEviction(ctx, projectID, "Error publishing resource eviction from resource delete", relations)
// 	return nil
// }

// // func (ac *AuthzCache) DeleteRelationsForTargets(ctx context.Context, targets []string) error {
// // 	projectID := cctx.ProjectID(ctx)
// // 	if cctx.HasNoProjectID(ctx) {
// // 		return se.UnknownProject.New()
// // 	}
// // 	relations, err := ac.sdkClient.Management.Authz().DeleteRelationsForTargets(ctx, projectID, targets)
// // 	if err != nil {
// // 		return err // notest
// // 	}
// // 	ac.relationCacheEviction(ctx, projectID, "Error publishing resource eviction from target delete", relations)
// // 	cctx.Logger(ctx).Info().Int("relations_len", len(relations)).Int("targets_len", len(targets)).Msg("Deleted relations for targets")
// // 	return nil
// // }

// func convertRelationQuery(schema *domain.Schema, rq *authzv1.RelationQuery) (*domain.Relation, error) {
// 	r := &domain.Relation{ResourceID: rq.Resource, Target: rq.Target}
// 	rd := schema.GetRelationDefinitionByName(rq.Namespace, rq.RelationDefinition)
// 	if rd == nil {
// 		return nil, se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", rq.Namespace, rq.RelationDefinition)) // notest
// 	}
// 	r.RelationDefinitionID = rd.ID
// 	return r, nil
// }

// type cacheRelationsLoader struct {
// 	ac             *AuthzCache
// 	projectID      string
// 	ctx            context.Context
// 	extraRelations []*domain.Relation
// }

// func (crl *cacheRelationsLoader) HasRelation(resourceID, relationDefinitionID, target string) (bool, error) {
// 	relations, _ := crl.ac.relationCache.Get(crl.ctx, cacheKey(crl.projectID, resourceID, relationDefinitionID))
// 	// if !ok {
// 	// 	var err error
// 	// 	relations, err = crl.ac.sdkClient.Management.Authz().SearchRelations(crl.ctx, crl.projectID, &domain.Relation{
// 	// 		ResourceID:           resourceID,
// 	// 		RelationDefinitionID: relationDefinitionID,
// 	// 	})
// 	// 	if err != nil {
// 	// 		// notest
// 	// 		cctx.Logger(crl.ctx).Err(err).Msg("Failed to search relations")
// 	// 		return false, err
// 	// 	}
// 	// 	crl.ac.relationCache.Add(crl.ctx, cacheKey(crl.projectID, resourceID, relationDefinitionID), relations)
// 	// }
// 	for _, r := range relations {
// 		if r.Target == target {
// 			return true, nil
// 		}
// 	}
// 	for _, r := range crl.extraRelations {
// 		if r.ResourceID == resourceID && r.RelationDefinitionID == relationDefinitionID && r.Target == target {
// 			return true, nil // notest
// 		}
// 	}
// 	return false, nil
// }

// func (crl *cacheRelationsLoader) ResourceRelations(resourceID, relationDefinitionID string) ([]*domain.Relation, error) {
// 	relations, _ := crl.ac.relationCache.Get(crl.ctx, cacheKey(crl.projectID, resourceID, relationDefinitionID))
// 	// if !ok {
// 	// 	var err error
// 	// 	relations, err = crl.ac.sdkClient.Management.Authz().SearchRelations(crl.ctx, crl.projectID, &domain.Relation{
// 	// 		ResourceID:           resourceID,
// 	// 		RelationDefinitionID: relationDefinitionID,
// 	// 	})
// 	// 	if err != nil {
// 	// 		// notest
// 	// 		cctx.Logger(crl.ctx).Err(err).Msg("Failed to search for resource relations")
// 	// 		return nil, err
// 	// 	}
// 	// 	crl.ac.relationCache.Add(crl.ctx, cacheKey(crl.projectID, resourceID, relationDefinitionID), relations)
// 	// }
// 	var extraRelations []*domain.Relation
// 	for _, r := range crl.extraRelations {
// 		if r.ResourceID == resourceID && r.RelationDefinitionID == relationDefinitionID {
// 			extraRelations = append(extraRelations, r)
// 		}
// 	}
// 	return append(relations, extraRelations...), nil
// }

// func (crl *cacheRelationsLoader) TargetRelations(target, relationDefinitionID string) ([]*domain.Relation, error) {
// 	var relations []*domain.Relation
// 	// ok := false
// 	// // We do not cache the full relations for targets ac we need it only for manual long operations
// 	// if relationDefinitionID != "" {
// 	// 	relations, ok = crl.ac.targetRelationCache.Get(crl.ctx, cacheKey(crl.projectID, target, relationDefinitionID))
// 	// }
// 	// if !ok {
// 	// 	var err error
// 	// 	query := &domain.Relation{Target: target}
// 	// 	if relationDefinitionID != "" {
// 	// 		query.RelationDefinitionID = relationDefinitionID
// 	// 	}
// 	// 	relations, err = crl.ac.sdkClient.Management.Authz().SearchRelations(crl.ctx, crl.projectID, query)
// 	// 	if err != nil {
// 	// 		// notest
// 	// 		cctx.Logger(crl.ctx).Err(err).Msg("Failed to search for related relations")
// 	// 		return nil, err
// 	// 	}
// 	// 	if relationDefinitionID != "" {
// 	// 		crl.ac.targetRelationCache.Add(crl.ctx, cacheKey(crl.projectID, target, relationDefinitionID), relations)
// 	// 	}
// 	// }
// 	var extraRelations []*domain.Relation
// 	for _, r := range crl.extraRelations {
// 		if r.Target == target && r.RelationDefinitionID == relationDefinitionID {
// 			extraRelations = append(extraRelations, r)
// 		}
// 	}
// 	return append(relations, extraRelations...), nil
// }

// func (crl *cacheRelationsLoader) TargetSetRelations(resourceID, relationDefinitionID string) ([]*domain.Relation, error) {
// 	// relations, err := crl.ac.sdkClient.Management.Authz().SearchRelations(crl.ctx, crl.projectID, &domain.Relation{
// 	// 	TargetSetResourceID: resourceID, TargetSetRelationDefinitionID: relationDefinitionID,
// 	// })
// 	// if err != nil {
// 	// 	// notest
// 	// 	cctx.Logger(crl.ctx).Err(err).Msg("Failed to search for user set relations")
// 	// 	return nil, err
// 	// }
// 	var relations []*domain.Relation

// 	var extraRelations []*domain.Relation
// 	for _, r := range crl.extraRelations {
// 		// notest
// 		if r.TargetSetResourceID == resourceID && r.TargetSetRelationDefinitionID == relationDefinitionID {
// 			extraRelations = append(extraRelations, r)
// 		}
// 	}
// 	return append(relations, extraRelations...), nil
// }

// func convertQuery(query *domain.UsersQuery) *descope.UserSearchOptions {
// 	req := &descope.UserSearchOptions{}
// 	for _, v := range query.Statuses {
// 		req.Statuses = append(req.Statuses, descope.UserStatus(v))
// 	}
// 	req.TenantIDs, req.Roles, req.Text, req.WithTestUsers, req.CustomAttributes =
// 		query.Tenants, query.Roles, query.Text, query.WithTestUser, query.CustomAttributes
// 	return req
// }

// func (crl *cacheRelationsLoader) QueryHasUser(query *domain.UsersQuery, user string) (bool, error) {
// 	req := convertQuery(query)
// 	req.LoginIDs = []string{user}
// 	resp, _, err := crl.ac.sdkClient.Management.User().SearchAll(crl.ctx, req)
// 	if err != nil {
// 		return false, err
// 	}
// 	return len(resp) > 0, nil
// }

// func (crl *cacheRelationsLoader) QueryUsers(query *domain.UsersQuery) ([]string, error) {
// 	req := convertQuery(query)
// 	resp, _, err := crl.ac.sdkClient.Management.User().SearchAll(crl.ctx, req)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return cutils.Map(resp, func(u *descope.UserResponse) string {
// 		return u.UserID
// 	}), nil
// }

// func (ac *AuthzCache) HasRelations(ctx context.Context, req *authzv1.HasRelationsRequest) (*authzv1.HasRelationsResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.HasRelationsResponse{}
// 	if cctx.HasNoProjectID(ctx) {
// 		return resp, se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New() // notest
// 	}
// 	crl := &cacheRelationsLoader{ac: ac, projectID: projectID, ctx: ctx}
// 	for _, r := range req.RelationQueries {
// 		newR, err := convertRelationQuery(schema, r)
// 		if err != nil {
// 			return nil, err // notest
// 		}
// 		r.HasRelation, err = schema.HasRelation(newR.ResourceID, newR.Target, newR.RelationDefinitionID, crl, nil)
// 		if err != nil {
// 			return nil, err // notest
// 		}
// 	}
// 	resp.RelationQueries = req.RelationQueries
// 	return resp, nil
// }

// func (ac *AuthzCache) WhoCanAccess(ctx context.Context, req *authzv1.WhoCanAccessRequest) (*authzv1.WhoCanAccessResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.WhoCanAccessResponse{}
// 	if cctx.HasNoProjectID(ctx) {
// 		return resp, se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New() // notest
// 	}
// 	crl := &cacheRelationsLoader{ac: ac, projectID: projectID, ctx: ctx}
// 	rd := schema.GetRelationDefinitionByName(req.Namespace, req.RelationDefinition)
// 	if rd == nil {
// 		return nil, se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", req.Namespace, req.RelationDefinition)) // notest
// 	}
// 	resp.Targets, err = schema.WhoCanAccess(req.Resource, rd.ID, crl, nil)
// 	return resp, err
// }

// func convertToExternalRelation(schema *domain.Schema, relation *domain.Relation) (*authzv1.Relation, error) {
// 	ns, rd := schema.GetRelationDefinition(relation.RelationDefinitionID)
// 	targetSetResource := ""
// 	targetSetRelationDefinition := ""
// 	targetSetRelationDefinitionNamespace := ""
// 	targetNamespace := relation.TargetNamespaceName
// 	if relation.HasTargetSet() {
// 		// notest
// 		targetSetResource = relation.TargetSetResourceID
// 		targetSetNS, targetSetRD := schema.GetRelationDefinition(relation.TargetSetRelationDefinitionID)
// 		targetSetRelationDefinition = targetSetRD.ID
// 		targetSetRelationDefinitionNamespace = targetSetNS.ID
// 		targetNamespace = targetSetNS.Name + "#" + targetSetRD.Name
// 	}
// 	var query *authzv1.UserQuery
// 	if relation.Query != nil {
// 		var ca *structpb.Struct
// 		var err error
// 		if relation.Query.CustomAttributes != nil {
// 			ca, err = structpb.NewStruct(relation.Query.CustomAttributes)
// 			if err != nil {
// 				return nil, err
// 			}
// 		}
// 		query = &authzv1.UserQuery{
// 			Tenants:          relation.Query.Tenants,
// 			Roles:            relation.Query.Roles,
// 			Text:             relation.Query.Text,
// 			Statuses:         relation.Query.Statuses,
// 			SsoOnly:          relation.Query.SsoOnly,
// 			WithTestUser:     relation.Query.WithTestUser,
// 			CustomAttributes: ca,
// 		}
// 	}
// 	return &authzv1.Relation{
// 		Resource:                             relation.ResourceID,
// 		Namespace:                            ns.Name,
// 		RelationDefinition:                   rd.Name,
// 		Target:                               relation.Target,
// 		TargetNamespace:                      targetNamespace,
// 		TargetSetResource:                    targetSetResource,
// 		TargetSetRelationDefinition:          targetSetRelationDefinition,
// 		TargetSetRelationDefinitionNamespace: targetSetRelationDefinitionNamespace,
// 		Query:                                query,
// 	}, nil
// }

// func (ac *AuthzCache) ResourceRelations(ctx context.Context, req *authzv1.ResourceRelationsRequest) (*authzv1.ResourceRelationsResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.ResourceRelationsResponse{}
// 	if cctx.HasNoProjectID(ctx) {
// 		return resp, se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New() // notest
// 	}
// 	// relations, err := ac.sdkClient.Management.Authz().SearchRelations(ctx, projectID, &domain.Relation{ResourceID: req.Resource})
// 	// if err != nil {
// 	// return resp, err // notest
// 	// }
// 	var relations []*domain.Relation
// 	for _, r := range relations {
// 		externalRelation, err := convertToExternalRelation(schema, r)
// 		if err != nil {
// 			return resp, err
// 		}
// 		resp.Relations = append(resp.Relations, externalRelation)
// 	}
// 	return resp, nil
// }

// func (ac *AuthzCache) TargetsRelations(ctx context.Context, req *authzv1.TargetsRelationsRequest) (*authzv1.TargetsRelationsResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.TargetsRelationsResponse{}
// 	if cctx.HasNoProjectID(ctx) {
// 		return resp, se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New() // notest
// 	}
// 	var relations []*domain.Relation
// 	// relations, err := ac.sdkClient.Management.Authz().SearchRelationsByTargets(ctx, projectID, req.Targets)
// 	// if err != nil {
// 	// return resp, err // notest
// 	// }
// 	for _, r := range relations {
// 		externalRelation, err := convertToExternalRelation(schema, r)
// 		if err != nil {
// 			return resp, err
// 		}
// 		resp.Relations = append(resp.Relations, externalRelation)
// 	}
// 	return resp, nil
// }

// func (ac *AuthzCache) WhatCanTargetAccess(ctx context.Context, req *authzv1.WhatCanTargetAccessRequest) (*authzv1.WhatCanTargetAccessResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.WhatCanTargetAccessResponse{}
// 	if cctx.HasNoProjectID(ctx) {
// 		return resp, se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New() // notest
// 	}
// 	crl := &cacheRelationsLoader{ac: ac, projectID: projectID, ctx: ctx}
// 	relations, err := schema.WhatCanTargetAccess(req.Target, crl)
// 	if err != nil {
// 		return resp, err
// 	}
// 	for _, r := range relations {
// 		externalRelation, err := convertToExternalRelation(schema, r)
// 		if err != nil {
// 			return resp, err
// 		}
// 		resp.Relations = append(resp.Relations, externalRelation)
// 	}
// 	return resp, err
// }

// func (ac *AuthzCache) WhatCanTargetAccessWithRelation(ctx context.Context, req *authzv1.WhatCanTargetAccessWithRelationRequest) (*authzv1.WhatCanTargetAccessWithRelationResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.WhatCanTargetAccessWithRelationResponse{}
// 	if cctx.HasNoProjectID(ctx) {
// 		return resp, se.UnknownProject.New() // notest
// 	}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err // notest
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New() // notest
// 	}
// 	rd := schema.GetRelationDefinitionByName(req.Namespace, req.RelationDefinition)
// 	if rd == nil {
// 		return resp, se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", req.Namespace, req.RelationDefinition)) // notest
// 	}
// 	crl := &cacheRelationsLoader{ac: ac, projectID: projectID, ctx: ctx}
// 	start := time.Now()
// 	if len(req.ParentRelationDefinition) > 0 && len(req.ParentTarget) > 0 {
// 		cctx.Logger(ctx).Info().Str("parent", req.ParentRelationDefinition).Msg("Calculating what can target access by parent definition")
// 		prd := schema.GetRelationDefinitionByName(req.Namespace, req.ParentRelationDefinition)
// 		if prd == nil {
// 			return resp, se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", req.Namespace, req.ParentRelationDefinition)) // notest
// 		}
// 		relations, err := crl.TargetRelations(req.ParentTarget, prd.ID)
// 		if err != nil {
// 			cctx.Logger(ctx).Warn().Err(err).Msg("Failed calculating target relation, for what can target access")
// 			return resp, err
// 		}
// 		for i := range relations {
// 			has, _ := schema.HasRelation(relations[i].ResourceID, req.Target, rd.ID, crl, nil)
// 			if has {
// 				resp.Resources = append(resp.Resources, relations[i].ResourceID)
// 			}
// 		}
// 	} else {
// 		cctx.Logger(ctx).Info().Msg("Calculating what can target access recursively")
// 		resources, err := schema.WhatCanTargetAccessWithRelation(req.Target, rd.ID, crl)
// 		if err != nil {
// 			return resp, err
// 		}
// 		resp.Resources = resources
// 	}
// 	cctx.Logger(ctx).Info().Dur("duration", time.Since(start)).Msg("Done calculating what can target access")
// 	return resp, err
// }

// func (ac *AuthzCache) OverrideTargetRelations(ctx context.Context, req *authzv1.OverrideTargetRelationsRequest) error {
// 	// if cctx.HasNoProjectID(ctx) {
// 	// 	return se.UnknownProject.New() // notest
// 	// }
// 	// projectID := cctx.ProjectID(ctx)
// 	// schema, err := ac.getSchema(ctx, projectID)
// 	// if err != nil {
// 	// 	return err // notest
// 	// }
// 	// if schema == nil || len(schema.Namespaces) == 0 {
// 	// 	return se.SchemaDoesNotExist.New() // notest
// 	// }
// 	// var relations []*domain.Relation
// 	// for _, rel := range req.Relations {
// 	// 	rd := schema.GetRelationDefinitionByName(rel.Namespace, rel.RelationDefinition)
// 	// 	if rd == nil {
// 	// 		return se.UnknownRelationDefinition.New(fmt.Sprintf("%s.%s", rel.Namespace, rel.RelationDefinition)) // notest
// 	// 	}
// 	// 	relations = append(relations, &domain.Relation{
// 	// 		Target:               req.Target,
// 	// 		ResourceID:           rel.Resource,
// 	// 		RelationDefinitionID: rd.ID,
// 	// 	})
// 	// }
// 	// // Make sure there are no duplicates
// 	// slices.SortFunc(relations, func(a, b *domain.Relation) int {
// 	// 	// notest
// 	// 	return a.Compare(b)
// 	// })
// 	// compactRelations := slices.CompactFunc(relations, func(r1, r2 *domain.Relation) bool {
// 	// 	return r1.Equals(r2)
// 	// })
// 	// changedRelations, err := ac.sdkClient.Management.Authz().OverrideTargetRelations(ctx, projectID, compactRelations)
// 	// if err != nil {
// 	// 	return err
// 	// }
// 	// ac.relationCacheEviction(ctx, projectID, "Error publishing resource eviction from target override", changedRelations)
// 	return nil
// }

// func (ac *AuthzCache) GetModified(ctx context.Context, since time.Time) (*authzv1.GetModifiedResponse, error) {
// 	resp := &authzv1.GetModifiedResponse{}
// 	return resp, nil
// 	// if cctx.HasNoProjectID(ctx) {
// 	// 	return resp, se.UnknownProject.New() // notest
// 	// }
// 	// projectID := cctx.ProjectID(ctx)
// 	// schema, err := ac.getSchema(ctx, projectID)
// 	// if err != nil {
// 	// 	return resp, err // notest
// 	// }
// 	// if schema == nil || len(schema.Namespaces) == 0 {
// 	// 	return resp, se.SchemaDoesNotExist.New() // notest
// 	// }
// 	// return ac.sdkClient.Management.Authz().GetModified(ctx, since)
// }

// func (ac *AuthzCache) UpdateReBACGroupsMappingsRelations(ctx context.Context, req *authzv1.UpdateReBACGroupsMappingsRelationsRequest) (*authzv1.UpdateReBACGroupsMappingsRelationsResponse, error) {
// 	// cctx.Logger(ctx).Info().Msg("Starting to update ReBAC groups mappings relations")
// 	resp := &authzv1.UpdateReBACGroupsMappingsRelationsResponse{}
// 	// if len(req.UserGroups) == 0 {
// 	// 	return resp, ce.InvalidArguments.NewWithLogLevelWarn(ctx, "UserGroups must contain at least one mapping") // notest
// 	// }

// 	// userIDs := []string{}
// 	// for userID := range req.UserGroups {
// 	// 	userIDs = append(userIDs, userID)
// 	// }

// 	// err := ac.DeleteRelationsForTargets(ctx, userIDs)
// 	// if err != nil {
// 	// 	cctx.Logger(ctx).Err(err).Msg("Failed to delete relations for targets when updating ReBAC groups mappings")
// 	// 	return resp, err
// 	// }

// 	// rebacGroupsMappings := req.RebacGroupsMappings
// 	// if rebacGroupsMappings == nil {
// 	// 	resp.Relations = []*authzv1.Relation{}
// 	// 	cctx.Logger(ctx).Info().Int("users_len", len(userIDs)).Msg("Deleted all relations for users and there are no ReBAC groups mappings to create relations")
// 	// 	return resp, nil
// 	// }

// 	// // Generate list of all the relations to create based on the mappings and groups
// 	// newRelations := []*authzv1.Relation{}
// 	// for userID, groups := range req.UserGroups {
// 	// 	for _, group := range groups.Groups {
// 	// 		if mapping, ok := rebacGroupsMappings[group]; ok {
// 	// 			for _, relation := range mapping.Relations {
// 	// 				newRelations = append(newRelations, &authzv1.Relation{
// 	// 					Target:             userID,
// 	// 					Resource:           relation.Resource,
// 	// 					RelationDefinition: relation.RelationDefinition,
// 	// 					Namespace:          relation.Namespace,
// 	// 				})
// 	// 			}
// 	// 		}
// 	// 	}
// 	// }
// 	// if len(newRelations) == 0 {
// 	// 	cctx.Logger(ctx).Info().Int("users_len", len(userIDs)).Msg("No ReBAC relations to create")
// 	// 	resp.Relations = []*authzv1.Relation{}
// 	// 	return resp, nil
// 	// }

// 	// err = ac.CreateRelations(ctx, &authzv1.CreateRelationsRequest{Relations: newRelations})
// 	// if err != nil {
// 	// 	cctx.Logger(ctx).Err(err).Msg("Failed to create relations for targets when updating ReBAC groups mappings")
// 	// 	return resp, err
// 	// }

// 	// resp.Relations = newRelations
// 	// // It's possible that actually less relations where created if there were duplicates
// 	// cctx.Logger(ctx).Info().Int("users_len", len(userIDs)).Int("relations_len", len(newRelations)).Msg("Created relations for users based on ReBAC groups mappings")
// 	return resp, nil
// }

// // TODO: Implement this
// func (ac *AuthzCache) ListRelations(ctx context.Context, pageNum, pageSize int) (*authzv1.ResourceRelationsResponse, error) {
// 	projectID := cctx.ProjectID(ctx)
// 	resp := &authzv1.ResourceRelationsResponse{}
// 	schema, err := ac.getSchema(ctx, projectID)
// 	if err != nil {
// 		return resp, err
// 	}
// 	if schema == nil || len(schema.Namespaces) == 0 {
// 		return resp, se.SchemaDoesNotExist.New()
// 	}
// 	if pageNum < 1 { // proto validation should take care of this, but just in case
// 		pageNum = 1
// 	}
// 	if pageSize == 0 || pageSize > config.GetListRelationsMaxPageSize() {
// 		pageSize = config.GetListRelationsMaxPageSize()
// 	}
// 	offset := pageSize * (pageNum - 1)
// 	cctx.Logger(ctx).Info().Int("page_num", pageNum).Int("page_size", pageSize).Int("offset", offset).Msg("Listing relations")
// 	// relations, err := ac.sdkClient.Management.Authz().ListRelations(ctx, projectID, pageSize, offset)
// 	// if err != nil {
// 	// 	cctx.Logger(ctx).Err(err).Msg("Failed to list relations")
// 	// 	return resp, err
// 	// }
// 	// for _, r := range relations {
// 	// 	externalRelation, err := convertToExternalRelation(schema, r)
// 	// 	if err != nil {
// 	// 		return nil, err
// 	// 	}
// 	// 	resp.Relations = append(resp.Relations, externalRelation)
// 	// }
// 	return resp, nil
// }

// type TypeDetails struct {
// 	Relations   map[string]*parser.Relation
// 	Permissions map[string]*parser.Permission
// }

// type SchemaRegistry map[string]*TypeDetails

// func NewSchemaRegistry(model *parser.Model) (SchemaRegistry, error) {
// 	sr := SchemaRegistry{}
// 	for _, t := range model.Types {
// 		if _, typeExist := sr[t.Name]; typeExist {
// 			return nil, se.FGADuplicateType.New(t.Name)
// 		}
// 		typeDetails := &TypeDetails{
// 			Relations:   map[string]*parser.Relation{},
// 			Permissions: map[string]*parser.Permission{},
// 		}
// 		for _, r := range t.Relations {
// 			if _, relationExist := typeDetails.Relations[r.Name]; relationExist {
// 				return nil, se.FGADuplicateRelation.New(r.Name)
// 			}
// 			typeDetails.Relations[r.Name] = r
// 		}
// 		for _, p := range t.Permissions {
// 			if _, permissionExist := typeDetails.Permissions[p.Name]; permissionExist {
// 				return nil, se.FGADuplicatePermission.New(p.Name)
// 			}
// 			if _, relationExist := typeDetails.Relations[p.Name]; relationExist {
// 				return nil, se.FGADuplicateName.New(p.Name)
// 			}
// 			typeDetails.Permissions[p.Name] = p
// 		}
// 		sr[t.Name] = typeDetails
// 	}
// 	return sr, nil
// }

// func (ac *AuthzCache) SaveDSLSchema(ctx context.Context, schemaDSL string) error {
// 	dsl := strings.TrimSpace(schemaDSL) + "\n"
// 	p := parser.NewParser(dsl)
// 	model, err := p.ParseModel()
// 	if err != nil {
// 		return se.FGAParseError.New(err.Error())
// 	}

// 	schema := &authzv1.Schema{
// 		Name: model.Name,
// 	}

// 	sr, err := NewSchemaRegistry(model)
// 	if err != nil {
// 		return err
// 	}

// 	for _, t := range model.Types {
// 		ns := &authzv1.Namespace{}
// 		ns.Name = t.Name
// 		for _, r := range t.Relations {
// 			rd := &authzv1.RelationDefinition{
// 				Name: r.Name,
// 				ComplexDefinition: &authzv1.Node{
// 					NType:      string(domain.NodeTypeChild),
// 					Expression: &authzv1.NodeExpression{NeType: string(domain.NodeExpressionTypeSelf)},
// 				},
// 			}
// 			children := []*authzv1.Node{}
// 			for _, rt := range r.Types {
// 				if _, ok := sr[rt.Type]; !ok {
// 					return se.FGAUnknownType.New(rt.Type)
// 				}
// 				if rt.Relation != "" {
// 					if _, ok := sr[rt.Type].Relations[rt.Relation]; !ok {
// 						return se.FGAUnknownRelation.New(fmt.Sprintf("%s.%s", rt.Type, rt.Relation))
// 					}
// 					children = append(children, &authzv1.Node{
// 						NType: string(domain.NodeTypeChild),
// 						Expression: &authzv1.NodeExpression{
// 							NeType:                            string(domain.NodeExpressionTypeTargetSet),
// 							TargetRelationDefinition:          rt.Relation,
// 							TargetRelationDefinitionNamespace: rt.Type,
// 						},
// 					})
// 				}
// 			}
// 			if len(children) > 0 {
// 				children = append(children, rd.ComplexDefinition)
// 				rd.ComplexDefinition = &authzv1.Node{
// 					NType:    string(domain.NodeTypeUnion),
// 					Children: children,
// 				}
// 			}
// 			ns.RelationDefinitions = append(ns.RelationDefinitions, rd)
// 		}
// 		for _, p := range t.Permissions {
// 			rd := &authzv1.RelationDefinition{}
// 			rd.Name = p.Name

// 			cd, err := convertExprToNode(p.Expr, t, sr)
// 			if err != nil {
// 				return err
// 			}
// 			rd.ComplexDefinition = cd
// 			ns.RelationDefinitions = append(ns.RelationDefinitions, rd)
// 		}
// 		schema.Namespaces = append(schema.Namespaces, ns)
// 	}

// 	schemaReq := &authzv1.SaveSchemaRequest{
// 		Schema:  schema,
// 		Upgrade: featureflags.IsFeatureEnabled(ctx, config.UpgradeSchemaForFGAFeature),
// 	}

// 	err = ac.SaveSchema(ctx, schemaReq, dsl)
// 	if err != nil {
// 		cctx.Logger(ctx).Err(err).Msg("Failed to save DSL schema")
// 		return err
// 	}
// 	return nil
// }

// func (ac *AuthzCache) LoadDSLSchema(ctx context.Context) (string, error) {
// 	schema, err := ac.getSchema(ctx, cctx.ProjectID(ctx))
// 	if err != nil {
// 		return "", err
// 	}
// 	if schema == nil {
// 		return "", se.SchemaDoesNotExist.New()
// 	}
// 	return schema.DSL, nil
// }

// func convertExprToNode(expr parser.Expr, t *parser.Type, sr SchemaRegistry) (*authzv1.Node, error) {
// 	switch e := expr.(type) {
// 	case *parser.Ident:
// 		n := &authzv1.Node{
// 			NType: string(domain.NodeTypeChild),
// 		}
// 		if sr[t.Name].Relations[e.Relation.Type] == nil && sr[t.Name].Permissions[e.Relation.Type] == nil {
// 			return nil, se.FGAUnknownRelation.New(e.Relation.Type)
// 		}

// 		if len(e.Relation.Relation) > 0 {
// 			relationTypes := sr[t.Name].Relations[e.Relation.Type].Types
// 			if len(relationTypes) == 0 {
// 				return nil, se.FGAUnknownType.New(e.Relation.Type) // notest
// 			}
// 			var firstTypeWithRelation string
// 			for _, rt := range relationTypes {
// 				if sr[rt.Type].Relations[e.Relation.Relation] != nil {
// 					firstTypeWithRelation = rt.Type
// 					break
// 				}
// 				if sr[rt.Type].Permissions[e.Relation.Relation] != nil {
// 					firstTypeWithRelation = rt.Type
// 					break
// 				}
// 			}
// 			if len(firstTypeWithRelation) == 0 {
// 				return nil, se.FGAUnknownRelation.New(fmt.Sprintf("%s.%s", e.Relation.Type, e.Relation.Relation))
// 			}
// 			n.Expression = &authzv1.NodeExpression{
// 				NeType:                            string(domain.NodeExpressionTypeRelationRight),
// 				RelationDefinition:                e.Relation.Type,
// 				RelationDefinitionNamespace:       t.Name,
// 				TargetRelationDefinition:          e.Relation.Relation,
// 				TargetRelationDefinitionNamespace: firstTypeWithRelation,
// 			}
// 		} else {
// 			n.Expression = &authzv1.NodeExpression{
// 				NeType:                            string(domain.NodeExpressionTypeTargetSet),
// 				TargetRelationDefinition:          e.Relation.Type,
// 				TargetRelationDefinitionNamespace: t.Name,
// 			}
// 		}
// 		return n, nil
// 	case *parser.SetExpr:
// 		cd := &authzv1.Node{
// 			NType: string(e.Operator),
// 		}
// 		for _, c := range e.Children {
// 			n, err := convertExprToNode(c, t, sr)
// 			if err != nil {
// 				return nil, err
// 			}
// 			cd.Children = append(cd.Children, n)
// 		}
// 		return cd, nil
// 	}
// 	return nil, se.FGAUnknownType.New(fmt.Sprintf("%T", expr)) // notest
// }

// func splitTargetType(targetType string) (string, string, error) {
// 	if strings.Contains(targetType, "#") {
// 		parts := strings.Split(targetType, "#")
// 		if len(parts) != 2 {
// 			return "", "", ce.BadRequest.New("Invalid target type format: ", targetType)
// 		}
// 		return parts[0], parts[1], nil
// 	}
// 	return targetType, "", nil
// }

// func (ac *AuthzCache) ValidateTupleWithModel(ctx context.Context, tuple *authzv1.Tuple, allowPermissions bool) error {
// 	schema, err := ac.getSchema(ctx, cctx.ProjectID(ctx))
// 	if err != nil {
// 		return err
// 	}

// 	sr, _ := NewSchemaRegistry(schema.Model)
// 	if _, ok := sr[tuple.ResourceType]; !ok {
// 		return se.FGAUnknownType.New(tuple.ResourceType)
// 	}
// 	targetType, targetRelation, err := splitTargetType(tuple.TargetType)
// 	if err != nil {
// 		return err
// 	}
// 	if _, ok := sr[targetType]; !ok {
// 		return se.FGAUnknownType.New(tuple.TargetType)
// 	}
// 	rd, ok := sr[tuple.ResourceType].Relations[tuple.Relation]
// 	if !ok {
// 		if !allowPermissions {
// 			return se.FGAUnknownRelation.New(fmt.Sprintf("%s.%s", tuple.ResourceType, tuple.Relation))
// 		}
// 		if _, ok := sr[tuple.ResourceType].Permissions[tuple.Relation]; !ok {
// 			return se.FGAUnknownRelation.New(fmt.Sprintf("%s.%s", tuple.ResourceType, tuple.Relation))
// 		}
// 	} else if len(cutils.Filter(rd.Types, func(rt parser.RelatedType) bool { return rt.Type == targetType })) == 0 {
// 		return se.FGAWrongTypeForRelation.New(tuple.Relation)
// 	}
// 	if targetRelation != "" {
// 		if _, ok := sr[targetType].Relations[targetRelation]; !ok {
// 			return se.FGAUnknownRelation.New(fmt.Sprintf("%s.%s", targetType, targetRelation))
// 		}
// 	}

// 	return nil
// }

// func (ac *AuthzCache) CreateTuples(ctx context.Context, req *authzv1.CreateTuplesRequest) (*authzv1.CreateTuplesResponse, error) {
// 	hrReq := &authzv1.CreateRelationsRequest{}
// 	for _, r := range req.Tuples {
// 		err := ac.ValidateTupleWithModel(ctx, r, false)
// 		if err != nil {
// 			return nil, err
// 		}
// 		rel := &authzv1.Relation{
// 			Resource:           r.Resource,
// 			Namespace:          r.ResourceType,
// 			RelationDefinition: r.Relation,
// 		}
// 		targetType, targetRelation, _ := splitTargetType(r.TargetType)
// 		if targetRelation != "" {
// 			rel.TargetSetResource = r.Target
// 			rel.TargetSetRelationDefinition = targetRelation
// 			rel.TargetSetRelationDefinitionNamespace = targetType
// 		} else {
// 			rel.Target = r.Target
// 			rel.TargetNamespace = targetType
// 		}
// 		hrReq.Relations = append(hrReq.Relations, rel)
// 	}

// 	err := ac.CreateRelations(ctx, hrReq)
// 	if err != nil {
// 		cctx.Logger(ctx).Warn().Err(err).Msg("Failed to create relations")
// 		return nil, err
// 	}
// 	return &authzv1.CreateTuplesResponse{}, nil
// }

// func (ac *AuthzCache) DeleteTuples(ctx context.Context, req *authzv1.DeleteTuplesRequest) (*authzv1.DeleteTuplesResponse, error) {
// 	hrReq := &authzv1.DeleteRelationsRequest{}
// 	for _, r := range req.Tuples {
// 		err := ac.ValidateTupleWithModel(ctx, r, false)
// 		if err != nil {
// 			return nil, err
// 		}
// 		rel := &authzv1.Relation{
// 			Resource:           r.Resource,
// 			Namespace:          r.ResourceType,
// 			RelationDefinition: r.Relation,
// 		}
// 		targetType, targetRelation, _ := splitTargetType(r.TargetType)
// 		if targetRelation != "" {
// 			rel.TargetSetResource = r.Target
// 			rel.TargetSetRelationDefinition = targetRelation
// 			rel.TargetSetRelationDefinitionNamespace = targetType
// 		} else {
// 			rel.Target = r.Target
// 			rel.TargetNamespace = targetType
// 		}
// 		hrReq.Relations = append(hrReq.Relations, rel)
// 	}

// 	err := ac.DeleteRelations(ctx, hrReq)
// 	if err != nil {
// 		cctx.Logger(ctx).Warn().Err(err).Msg("Failed to delete relations")
// 		return nil, err
// 	}
// 	return &authzv1.DeleteTuplesResponse{}, nil
// }

// func (ac *AuthzCache) Check(ctx context.Context, req *authzv1.CheckRequest) (*authzv1.CheckResponse, error) {
// 	hrReq := &authzv1.HasRelationsRequest{}
// 	invalidTuples := make(map[int]*authzv1.Tuple)
// 	responseTuples := make([]*authzv1.CheckResponseTuple, len(req.Tuples))

// 	for i, r := range req.Tuples {
// 		err := ac.ValidateTupleWithModel(ctx, r, true)
// 		if err != nil {
// 			invalidTuples[i] = r
// 			responseTuples[i] = &authzv1.CheckResponseTuple{
// 				Tuple:   r,
// 				Allowed: false,
// 			}
// 			continue
// 		}
// 		hrReq.RelationQueries = append(hrReq.RelationQueries, &authzv1.RelationQuery{
// 			Resource:           r.Resource,
// 			Namespace:          r.ResourceType,
// 			RelationDefinition: r.Relation,
// 			Target:             r.Target,
// 		})
// 	}

// 	hrRes, err := ac.HasRelations(ctx, hrReq)
// 	if err != nil {
// 		return nil, err
// 	}

// 	hrIndex := 0
// 	for i := range req.Tuples {
// 		if _, invalid := invalidTuples[i]; !invalid {
// 			hrQuery := hrRes.RelationQueries[hrIndex]
// 			responseTuples[i] = &authzv1.CheckResponseTuple{
// 				Tuple: &authzv1.Tuple{
// 					Resource:     hrQuery.Resource,
// 					ResourceType: hrQuery.Namespace,
// 					Relation:     hrQuery.RelationDefinition,
// 					Target:       hrQuery.Target,
// 					TargetType:   "none",
// 				},
// 				Allowed: hrQuery.HasRelation,
// 			}
// 			hrIndex++
// 		}
// 	}

// 	return &authzv1.CheckResponse{Tuples: responseTuples}, nil
// }
