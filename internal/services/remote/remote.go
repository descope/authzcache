package remote

import (
	"os"
	"strings"

	"github.com/descope/authzcache/internal/config"
	ae "github.com/descope/authzservice/pkg/authzservice/errors"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
)

var baseURL = os.Getenv(descope.EnvironmentVariableBaseURL)
var managementKey = strings.Trim(os.Getenv(descope.EnvironmentVariableManagementKey), "\"")

func NewDescopeClientWithProjectID(projectID string, loggerInstance logger.LoggerInterface) (sdk.Management, error) {
	if projectID == "" {
		return nil, ae.UnknownProject.New("projectID is empty")
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

func getLogLevel() logger.LogLevel {
	if config.GetSDKDebugLog() {
		return logger.LogDebugLevel // notest
	}
	return logger.LogInfoLevel
}
