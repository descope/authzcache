package remote

import (
	"os"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
)

var baseURL = os.Getenv(descope.EnvironmentVariableBaseURL)

func NewDescopeClientWithProjectID(projectID string) (sdk.Management, error) {
	descopeClient, err := client.NewWithConfig(&client.Config{
		ProjectID:           string(projectID),
		SessionJWTViaCookie: true,
		DescopeBaseURL:      baseURL,
		LogLevel:            getLogLevel(),
	})
	if err != nil {
		return nil, err
	}
	return descopeClient.Management, nil
}

func getLogLevel() logger.LogLevel {
	if config.GetSDKDebugLog() {
		return logger.LogDebugLevel
	}
	return logger.LogInfoLevel
}
