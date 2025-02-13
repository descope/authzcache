package services

import (
	"context"

	"github.com/descope/authzcache/internal/services/caches"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/sdk"
)

type AuthzCache interface {
	CreateFGASchema(ctx context.Context, dsl string) error
	CreateFGARelations(ctx context.Context, relations []*descope.FGARelation) error
	DeleteFGARelations(ctx context.Context, relations []*descope.FGARelation) error
	Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error)
}

type RemoteClientCreator func(projectID string) (sdk.Management, error)
type ProjectAuthzCacheCreator func(ctx context.Context, remoteChangesChecker caches.RemoteChangesChecker) (caches.ProjectAuthzCache, error)

type projectIDKey string

type project struct {
	cache   caches.ProjectAuthzCache // projectID -> caches
	mgmtSDK sdk.Management
}

type authzCache struct {
	projects            map[projectIDKey]project
	projectCacheCreator ProjectAuthzCacheCreator
	remoteClientCreator RemoteClientCreator
}

var _ AuthzCache = &authzCache{} // validate interface implementation

func New(ctx context.Context, projectCacheCreator ProjectAuthzCacheCreator, remoteClientCreator RemoteClientCreator) (AuthzCache, error) {
	cctx.Logger(ctx).Info().Msg("Starting new authz cache")
	ac := &authzCache{projectCacheCreator: projectCacheCreator, projects: make(map[projectIDKey]project), remoteClientCreator: remoteClientCreator}
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
	// iterate over relations and check cache, if not found, check later in sdk
	var cachedChecks []*descope.FGACheck
	var toCheckViaSDK []*descope.FGARelation
	var indexToCachedChecks map[int]*descope.FGACheck = make(map[int]*descope.FGACheck, len(relations)) // map "relations index" -> check, used to retain same order of relations in checks response
	for i, r := range relations {
		if allowed, direct, ok := projectCache.CheckRelation(ctx, r); ok {
			check := &descope.FGACheck{Allowed: allowed, Relation: r, Info: &descope.FGACheckInfo{Direct: direct}}
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

func (a *authzCache) getOrCreateProjectCache(ctx context.Context) (caches.ProjectAuthzCache, sdk.Management, error) {
	projectID := projectIDKey(cctx.ProjectID(ctx))
	if project, ok := a.projects[projectID]; ok {
		return project.cache, project.mgmtSDK, nil
	}
	cctx.Logger(ctx).Info().Msg("Creating new project cache")
	// create project mgmt sdk
	mgmtSdk, err := a.remoteClientCreator(string(projectID))
	if err != nil {
		return nil, nil, err // notest
	}
	// create project cache
	projectCache, err := a.projectCacheCreator(ctx, mgmtSdk.Authz())
	if err != nil {
		return nil, nil, err // notest
	}
	// start remote changes polling
	projectCache.StartRemoteChangesPolling(ctx)
	// save cache and sdk
	a.projects[projectID] = project{cache: projectCache, mgmtSDK: mgmtSdk}
	// return cache and sdk
	return projectCache, mgmtSdk, nil
}
