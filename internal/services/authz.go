package services

import (
	"context"

	"github.com/descope/authzcache/internal/services/caches"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/sdk"
)

type ProjectAuthzCacheCreator interface {
	NewProjectAuthzCache(ctx context.Context, remoteChangesChecker caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error)
}

type AuthzCache struct {
	mgmtSdk             sdk.Management
	projectAuthzCaches  map[string]caches.ProjectAuthzCache // projectID -> caches
	projectCacheCreator ProjectAuthzCacheCreator
}

func New(ctx context.Context, mgmtSdk sdk.Management, projectCacheCreator ProjectAuthzCacheCreator) (*AuthzCache, error) {
	cctx.Logger(ctx).Info().Msg("Starting new authz cache")
	ac := &AuthzCache{mgmtSdk: mgmtSdk, projectAuthzCaches: make(map[string]caches.ProjectAuthzCache), projectCacheCreator: projectCacheCreator}
	return ac, nil
}

func (a *AuthzCache) CreateFGASchema(ctx context.Context, dsl string) error {
	err := a.mgmtSdk.FGA().SaveSchema(ctx, &descope.FGASchema{Schema: dsl})
	if err != nil {
		return err
	}
	// update cache
	projectCache, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return err
	}
	projectCache.UpdateCacheWithSchema(ctx, &descope.FGASchema{Schema: dsl})
	return nil
}

func (a *AuthzCache) CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	err := a.mgmtSdk.FGA().CreateRelations(ctx, relations)
	if err != nil {
		return err
	}
	// update cache
	if len(relations) == 0 {
		return nil
	}
	projectCache, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return err
	}
	projectCache.UpdateCacheWithAddedRelations(ctx, relations)
	return nil
}

func (a *AuthzCache) DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error {
	err := a.mgmtSdk.FGA().DeleteRelations(ctx, relations)
	if err != nil {
		return err
	}
	// update cache
	if len(relations) == 0 {
		return nil
	}
	projectCache, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return err
	}
	projectCache.UpdateCacheWithDeletedRelations(ctx, relations)
	return nil
}

func (a *AuthzCache) Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error) {
	// get cache
	projectCache, err := a.getOrCreateProjectCache(ctx)
	if err != nil {
		return nil, err
	}
	// iterate over relations and check cache, if not found, check later in sdk
	var cachedChecks []*descope.FGACheck
	var toCheckViaSDK []*descope.FGARelation
	var indexToCachedChecks map[int]*descope.FGACheck = make(map[int]*descope.FGACheck, len(relations)) // map "relations index" -> check, used to retain same order of relations in checks response
	for i, r := range relations {
		if allowed, ok := projectCache.CheckRelation(ctx, r); ok {
			check := &descope.FGACheck{Allowed: allowed, Relation: r}
			cachedChecks = append(cachedChecks, check)
			indexToCachedChecks[i] = check
		} else {
			toCheckViaSDK = append(toCheckViaSDK, r)
		}
	}
	// if all relations were found in cache, return
	if len(toCheckViaSDK) == 0 {
		return cachedChecks, nil
	}
	// fetch missing relations from sdk
	sdkChecks, err := a.mgmtSdk.FGA().Check(ctx, toCheckViaSDK)
	if err != nil {
		return nil, err
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

func (a *AuthzCache) getOrCreateProjectCache(ctx context.Context) (caches.ProjectAuthzCache, error) {
	projectID := cctx.ProjectID(ctx)
	projectCache, ok := a.projectAuthzCaches[projectID]
	if ok {
		return projectCache, nil
	}
	cctx.Logger(ctx).Info().Msg("Creating new project cache")
	projectCache, err := a.projectCacheCreator.NewProjectAuthzCache(ctx, a.mgmtSdk.Authz())
	if err != nil {
		return nil, err
	}
	a.projectAuthzCaches[projectID] = projectCache
	return projectCache, nil
}
