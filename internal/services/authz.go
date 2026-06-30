package services

import (
	"context"
	"sync"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/descope/authzcache/internal/services/metrics"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
)

type AuthzCache interface {
	CreateFGASchema(ctx context.Context, dsl string) error
	CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error
	DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error
	Check(ctx context.Context, relations []*descope.FGARelation, conditionsContext map[string]any) ([]*descope.FGACheck, error)
	WhoCanAccess(ctx context.Context, resource, relationDefinition, namespace string, conditionsContext map[string]any) ([]string, error)
	WhatCanTargetAccess(ctx context.Context, target string, conditionsContext map[string]any) ([]*descope.AuthzRelation, error)
}

type RemoteClientCreator func(projectID string, logger logger.LoggerInterface) (sdk.Management, error)
type ProjectAuthzCacheCreator func(ctx context.Context, remoteChangesChecker caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error)

type project struct {
	cache   caches.ProjectAuthzCache // projectID -> caches
	mgmtSDK sdk.Management
}

type authzCache struct {
	projects            sync.Map //[projectIDKey]project
	projectCacheCreator ProjectAuthzCacheCreator
	remoteClientCreator RemoteClientCreator
	metricsCollector    *metrics.Collector
}

var _ AuthzCache = &authzCache{} // validate interface implementation

func New(ctx context.Context, projectCacheCreator ProjectAuthzCacheCreator, remoteClientCreator RemoteClientCreator, collector *metrics.Collector) (AuthzCache, error) {
	cctx.Logger(ctx).Info().Msg("Starting new authz cache")
	ac := &authzCache{projectCacheCreator: projectCacheCreator, remoteClientCreator: remoteClientCreator, metricsCollector: collector}
	return ac, nil
}

func (a *authzCache) CreateFGASchema(ctx context.Context, dsl string) error {
	// get cache and mgmt sdk
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return err // notest
	}
	// update remote
	err = mgmtSDK.FGA().SaveSchema(ctx, &descope.FGASchema{Schema: dsl})
	if err != nil {
		return err // notest
	}
	// update cache (purges all relations — they may resolve differently under the new schema)
	projectCache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: dsl})
	return nil
}

func (a *authzCache) CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	// nothing to do
	if len(relations) == 0 {
		return nil
	}
	// get cache and mgmt sdk
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return err // notest
	}
	// update remote
	err = mgmtSDK.FGA().CreateRelations(ctx, relations)
	if err != nil {
		return err // notest
	}
	// update cache
	projectCache.UpdateCacheWithAddedRelations(ctx, relations)
	return nil
}

func (a *authzCache) DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	// nothing to do
	if len(relations) == 0 {
		return nil
	}
	// get cache and mgmt sdk
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return err // notest
	}
	// update remote
	err = mgmtSDK.FGA().DeleteRelations(ctx, relations)
	if err != nil {
		return err // notest
	}
	// update cache
	projectCache.UpdateCacheWithDeletedRelations(ctx, relations)
	return nil
}

func (a *authzCache) Check(ctx context.Context, relations []*descope.FGARelation, conditionsContext map[string]any) ([]*descope.FGACheck, error) {
	start := time.Now()
	// get cache and mgmt sdk
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err // notest
	}
	// check all relations against cache in one read-lock; cached conditional grants are re-evaluated against conditionsContext inside CheckRelations
	cachedChecks, toCheckViaSDK, indexToCachedChecks := projectCache.CheckRelations(ctx, relations, conditionsContext)
	// if all relations were found in cache, return
	if len(toCheckViaSDK) == 0 {
		// candidatesCount = 0 (nothing sent to SDK); filteredCount = 0 (Check doesn't filter, every relation gets an answer)
		a.recordMetric(ctx, metrics.APICheck, true, 0, 0, len(cachedChecks), start)
		return cachedChecks, nil
	}
	// fetch missing relations from sdk
	sdkChecks, err := mgmtSDK.FGA().CheckWithContext(ctx, toCheckViaSDK, conditionsContext)
	if err != nil {
		return nil, err // notest
	}
	// if a cacheable conditional appeared, load the schema's conditions so the edge can re-evaluate them later
	if hasCacheableConditional(sdkChecks) {
		a.ensureSchemaLoaded(ctx, projectCache, mgmtSDK)
	}
	projectCache.UpdateCacheWithChecks(ctx, sdkChecks, conditionsContext)
	// merge cached and sdk checks in the same order as input relations and return them
	var result []*descope.FGACheck
	var j int
	for i := range relations {
		if check, ok := indexToCachedChecks[i]; ok {
			result = append(result, check)
		} else {
			result = append(result, sdkChecks[j])
			j++
		}
	}
	// candidatesCount = relations sent to SDK (not in cache); filteredCount = 0 (Check doesn't filter, every relation gets an answer)
	a.recordMetric(ctx, metrics.APICheck, false, len(toCheckViaSDK), 0, len(result), start)
	return result, nil
}

// ensureSchemaLoaded lazily loads the schema (and its compiled conditions) into the project cache
// when absent — on first use and after a remote schema change purged it. Best-effort: a load failure
// is logged and leaves the cache without conditions, which is safe (conditional grants simply won't
// be cached until the next successful load).
func (a *authzCache) ensureSchemaLoaded(ctx context.Context, projectCache caches.ProjectAuthzCache, mgmtSDK sdk.Management) {
	if projectCache.GetSchema() != nil {
		return
	}
	schema, err := mgmtSDK.FGA().LoadSchema(ctx)
	if err != nil {
		cctx.Logger(ctx).Warn().Err(err).Msg("Failed to load FGA schema for edge condition handling")
		return // notest
	}
	projectCache.EnsureSchemaLoaded(ctx, schema)
}

// hasCacheableConditional reports whether any check is a cleanly-evaluated CEL grant/denial with recorded conditions — the only cacheable conditionals.
func hasCacheableConditional(checks []*descope.FGACheck) bool {
	for _, c := range checks {
		if c.Info.Conditional && !c.Info.FactUsed && len(c.Info.MissingContext) == 0 && c.Info.ConditionalErr == "" &&
			(len(c.Info.TrueConditions) > 0 || len(c.Info.FalseConditions) > 0) {
			return true
		}
	}
	return false
}

func (a *authzCache) recordMetric(ctx context.Context, api metrics.APIName, cacheHit bool, candidatesCount, filteredCount, resultSize int, start time.Time) {
	if a.metricsCollector == nil {
		return
	}
	a.metricsCollector.Record(cctx.ProjectID(ctx), api, metrics.CallMetrics{
		CacheHit:        cacheHit,
		CandidatesCount: candidatesCount,
		FilteredCount:   filteredCount,
		ResultSize:      resultSize,
		DurationMs:      time.Since(start).Milliseconds(),
	})
}

func (a *authzCache) WhoCanAccess(ctx context.Context, resource, relationDefinition, namespace string, conditionsContext map[string]any) ([]string, error) {
	start := time.Now()
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err // notest
	}
	candidates, cacheHit := projectCache.GetWhoCanAccessCached(ctx, resource, relationDefinition, namespace)
	if !cacheHit {
		candidates, err = mgmtSDK.Authz().WhoCanAccess(ctx, resource, relationDefinition, namespace)
		if err != nil {
			return nil, err // notest
		}
		projectCache.SetWhoCanAccessCached(ctx, resource, relationDefinition, namespace, candidates)
	}
	// The cached set is the context-independent candidate pool; filter it against this request's
	// context via Check, which is context-correct (hash cache) and fact-fresh (never caches facts).
	verified, err := a.filterWhoCanAccessCandidates(ctx, resource, relationDefinition, namespace, candidates, conditionsContext)
	if err != nil {
		return nil, err // notest
	}
	a.recordMetric(ctx, metrics.APIWhoCanAccess, cacheHit, len(candidates), len(candidates)-len(verified), len(verified), start)
	return verified, nil
}

func (a *authzCache) filterWhoCanAccessCandidates(ctx context.Context, resource, relationDefinition, namespace string, candidates []string, conditionsContext map[string]any) ([]string, error) {
	relations := make([]*descope.FGARelation, len(candidates))
	for i, target := range candidates {
		relations[i] = &descope.FGARelation{
			Resource:     resource,
			ResourceType: namespace,
			Relation:     relationDefinition,
			Target:       target,
			TargetType:   "*", // WhoCanAccess is TargetType-agnostic
		}
	}
	checks, err := a.Check(ctx, relations, conditionsContext)
	if err != nil {
		return nil, err
	}
	var verified []string
	for i, check := range checks {
		if check.Allowed {
			verified = append(verified, candidates[i])
		}
	}
	return verified, nil
}

func (a *authzCache) WhatCanTargetAccess(ctx context.Context, target string, conditionsContext map[string]any) ([]*descope.AuthzRelation, error) {
	start := time.Now()
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err // notest
	}
	candidates, cacheHit := projectCache.GetWhatCanTargetAccessCached(ctx, target)
	if !cacheHit {
		candidates, err = mgmtSDK.Authz().WhatCanTargetAccess(ctx, target)
		if err != nil {
			return nil, err // notest
		}
		projectCache.SetWhatCanTargetAccessCached(ctx, target, candidates)
	}
	// filter the candidate pool against this request's context via Check (see WhoCanAccess)
	verified, err := a.filterWhatCanTargetAccessCandidates(ctx, target, candidates, conditionsContext)
	if err != nil {
		return nil, err // notest
	}
	a.recordMetric(ctx, metrics.APIWhatCanTargetAccess, cacheHit, len(candidates), len(candidates)-len(verified), len(verified), start)
	return verified, nil
}

func (a *authzCache) filterWhatCanTargetAccessCandidates(ctx context.Context, target string, candidates []*descope.AuthzRelation, conditionsContext map[string]any) ([]*descope.AuthzRelation, error) {
	relations := make([]*descope.FGARelation, len(candidates))
	for i, r := range candidates {
		relations[i] = &descope.FGARelation{
			Resource:     r.Resource,
			ResourceType: r.Namespace,
			Relation:     r.RelationDefinition,
			Target:       target,
			TargetType:   "*", // WhatCanTargetAccess is TargetType-agnostic
		}
	}
	checks, err := a.Check(ctx, relations, conditionsContext)
	if err != nil {
		return nil, err
	}
	var verified []*descope.AuthzRelation
	for i, check := range checks {
		if check.Allowed {
			verified = append(verified, candidates[i])
		}
	}
	return verified, nil
}

func (a *authzCache) getOrCreateProjectCache(ctx context.Context) (caches.ProjectAuthzCache, sdk.Management, error) {
	projectID := cctx.ProjectID(ctx)
	if p, ok := a.projects.Load(projectID); ok {
		return p.(project).cache, p.(project).mgmtSDK, nil
	}
	cctx.Logger(ctx).Info().Msg("Creating new project cache")
	projectMgmtSDK, err := a.remoteClientCreator(projectID, cctx.Logger(ctx))
	if err != nil {
		return nil, nil, err // notest
	}
	projectCache, err := a.projectCacheCreator(ctx, projectMgmtSDK.Authz())
	if err != nil {
		return nil, nil, err // notest
	}
	projectCache.StartRemoteChangesPolling(ctx)
	a.projects.Store(projectID, project{cache: projectCache, mgmtSDK: projectMgmtSDK})
	return projectCache, projectMgmtSDK, nil
}
