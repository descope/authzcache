package services

import (
	"context"
	"sync"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/descope/authzcache/internal/services/metrics"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
)

type AuthzCache interface {
	CreateFGASchema(ctx context.Context, dsl string) error
	CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error
	DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error
	Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error)
	WhoCanAccess(ctx context.Context, resource, relationDefinition, namespace string) ([]string, error)
	WhatCanTargetAccess(ctx context.Context, target string) ([]*descope.AuthzRelation, error)
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
	// update cache
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

func (a *authzCache) Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error) {
	// get cache and mgmt sdk
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err // notest
	}
	// check all relations against cache in a single read-lock acquisition
	cachedChecks, toCheckViaSDK, indexToCachedChecks := projectCache.CheckRelations(ctx, relations)
	// if all relations were found in cache, return
	if len(toCheckViaSDK) == 0 {
		return cachedChecks, nil
	}
	// fetch missing relations from sdk
	sdkChecks, err := mgmtSDK.FGA().Check(ctx, toCheckViaSDK)
	if err != nil {
		return nil, err // notest
	}
	// update cache
	projectCache.UpdateCacheWithChecks(ctx, sdkChecks)
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
	return result, nil
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

func (a *authzCache) WhoCanAccess(ctx context.Context, resource, relationDefinition, namespace string) ([]string, error) {
	start := time.Now()
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err // notest
	}
	candidates, cacheHit := projectCache.GetWhoCanAccessCached(ctx, resource, relationDefinition, namespace)
	if cacheHit && len(candidates) > 0 {
		verified, err := a.filterWhoCanAccessCandidates(ctx, resource, relationDefinition, namespace, candidates)
		if err != nil {
			return nil, err // notest
		}
		cctx.Logger(ctx).Debug().
			Str("resource", resource).
			Int("candidates", len(candidates)).
			Int("verified", len(verified)).
			Msg("WhoCanAccess cache hit with candidate filtering")
		a.recordMetric(ctx, metrics.APIWhoCanAccess, true, len(candidates), len(candidates)-len(verified), len(verified), start)
		return verified, nil
	}
	targets, err := mgmtSDK.Authz().WhoCanAccess(ctx, resource, relationDefinition, namespace)
	if err != nil {
		return nil, err // notest
	}
	projectCache.SetWhoCanAccessCached(ctx, resource, relationDefinition, namespace, targets)
	a.recordMetric(ctx, metrics.APIWhoCanAccess, false, 0, 0, len(targets), start)
	return targets, nil
}

func (a *authzCache) filterWhoCanAccessCandidates(ctx context.Context, resource, relationDefinition, namespace string, candidates []string) ([]string, error) {
	relations := make([]*descope.FGARelation, len(candidates))
	for i, target := range candidates {
		relations[i] = &descope.FGARelation{
			Resource:     resource,
			ResourceType: namespace,
			Relation:     relationDefinition,
			Target:       target,
		}
	}
	checks, err := a.Check(ctx, relations)
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

func (a *authzCache) WhatCanTargetAccess(ctx context.Context, target string) ([]*descope.AuthzRelation, error) {
	start := time.Now()
	projectCache, mgmtSDK, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err // notest
	}
	candidates, cacheHit := projectCache.GetWhatCanTargetAccessCached(ctx, target)
	if cacheHit && len(candidates) > 0 {
		verified, err := a.filterWhatCanTargetAccessCandidates(ctx, target, candidates)
		if err != nil {
			return nil, err // notest
		}
		cctx.Logger(ctx).Debug().
			Str("target", target).
			Int("candidates", len(candidates)).
			Int("verified", len(verified)).
			Msg("WhatCanTargetAccess cache hit with candidate filtering")
		a.recordMetric(ctx, metrics.APIWhatCanTargetAccess, true, len(candidates), len(candidates)-len(verified), len(verified), start)
		return verified, nil
	}
	relations, err := mgmtSDK.Authz().WhatCanTargetAccess(ctx, target)
	if err != nil {
		return nil, err // notest
	}
	projectCache.SetWhatCanTargetAccessCached(ctx, target, relations)
	a.recordMetric(ctx, metrics.APIWhatCanTargetAccess, false, 0, 0, len(relations), start)
	return relations, nil
}

func (a *authzCache) filterWhatCanTargetAccessCandidates(ctx context.Context, target string, candidates []*descope.AuthzRelation) ([]*descope.AuthzRelation, error) {
	relations := make([]*descope.FGARelation, len(candidates))
	for i, r := range candidates {
		relations[i] = &descope.FGARelation{
			Resource:     r.Resource,
			ResourceType: r.Namespace,
			Relation:     r.RelationDefinition,
			Target:       target,
		}
	}
	checks, err := a.Check(ctx, relations)
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
