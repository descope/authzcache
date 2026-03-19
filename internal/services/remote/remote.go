package remote

import (
	"context"
	"os"
	"strings"

	"github.com/descope/authzcache/internal/config"
	ae "github.com/descope/authzservice/pkg/authzservice/errors"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
)

const (
	defaultAPIPrefix  = "https://api"
	defaultDomainName = "descope.com"
	defaultBaseURL    = defaultAPIPrefix + "." + defaultDomainName
)

var baseURL = os.Getenv(descope.EnvironmentVariableBaseURL)
var managementKey = strings.Trim(os.Getenv(descope.EnvironmentVariableManagementKey), "\"")

func NewDescopeClientWithProjectID(projectID string, loggerInstance logger.LoggerInterface) (sdk.Management, error) {
	if projectID == "" {
		return nil, ae.UnknownProject.New(context.Background(), "projectID is empty")
	}
	descopeClient, err := client.NewWithConfig(&client.Config{
		ProjectID:           projectID,
		SessionJWTViaCookie: true,
		DescopeBaseURL:      baseURL,
		LogLevel:            getLogLevel(),
		Logger:              loggerInstance,
		ManagementKey:       managementKey,
	})
	if err != nil {
		return nil, err // notest
	}
	return descopeClient.Management, nil
}

func BaseURL() string {
	return baseURL
}

// BaseURLForProject returns the base URL for the given project.
// If DESCOPE_BASE_URL is set, it is returned as-is.
// Otherwise the URL is derived from the project ID's region prefix,
// matching the Descope SDK's default behavior.
func BaseURLForProject(projectID string) string {
	if baseURL != "" {
		return baseURL
	}
	if len(projectID) >= 32 {
		region := projectID[1:5]
		return strings.Join([]string{defaultAPIPrefix, region, defaultDomainName}, ".")
	}
	return defaultBaseURL
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
