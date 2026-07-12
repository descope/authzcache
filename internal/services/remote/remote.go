package remote

import (
	"context"
	"os"
	"strings"

	"github.com/descope/authzcache/internal/config"
	ae "github.com/descope/backend/authzservice/pkg/authzservice/errors"
	cconfig "github.com/descope/backend/common/pkg/common/config"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
)

var baseURL = os.Getenv(descope.EnvironmentVariableBaseURL)
var managementKey = strings.Trim(os.Getenv(descope.EnvironmentVariableManagementKey), "\"")

func NewDescopeClientWithProjectID(projectID string, loggerInstance logger.LoggerInterface) (sdk.Management, error) {
	if projectID == "" {
		return nil, ae.UnknownProject.New(context.Background(), "projectID is empty")
	}
	descopeClient, err := client.NewWithConfig(&client.Config{
		ProjectID:            projectID,
		SessionJWTViaCookie:  true,
		DescopeBaseURL:       baseURL,
		LogLevel:             getLogLevel(),
		Logger:               loggerInstance,
		ManagementKey:        managementKey,
		CustomDefaultHeaders: map[string]string{cconfig.HeaderAuthzCacheGitSha: cconfig.GetGitSha()},
	})
	if err != nil {
		return nil, err // notest
	}
	// the edge cache needs per-condition results on Check to build its certificates
	descopeClient.Management.FGA().SetListConditions(true)
	return descopeClient.Management, nil
}

func BaseURL() string {
	return baseURL
}

func ManagementKey() string {
	return managementKey
}

func getLogLevel() logger.LogLevel {
	if config.GetSDKDebugLog() {
		return logger.LogDebugLevel // notest
	}
	return logger.LogInfoLevel
}
