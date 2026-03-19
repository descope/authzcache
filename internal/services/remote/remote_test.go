package remote

import (
	"context"
	"fmt"
	"testing"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseURLForProject(t *testing.T) {
	t.Cleanup(func() { baseURL = "" })

	// Without DESCOPE_BASE_URL set — derive from project ID
	t.Setenv(descope.EnvironmentVariableBaseURL, "")
	baseURL = ""

	useURL := fmt.Sprintf("%s.use1.%s", defaultAPIPrefix, defaultDomainName)
	assert.EqualValues(t, defaultBaseURL, BaseURLForProject("P2aAc4T2V93bddihGEx2Ryhc8e5Z"))
	assert.EqualValues(t, defaultBaseURL, BaseURLForProject(""))
	assert.EqualValues(t, defaultBaseURL, BaseURLForProject("Puse"))
	assert.EqualValues(t, defaultBaseURL, BaseURLForProject("Puse1ar"))
	assert.EqualValues(t, useURL, BaseURLForProject("Puse12aAc4T2V93bddihGEx2Ryhc8e5Zfoobar"))
	assert.EqualValues(t, useURL, BaseURLForProject("Puse12aAc4T2V93bddihGEx2Ryhc8e5Z"))

	// With DESCOPE_BASE_URL set — always returns that override
	baseURL = "https://custom.example.com"
	assert.EqualValues(t, "https://custom.example.com", BaseURLForProject("Puse12aAc4T2V93bddihGEx2Ryhc8e5Z"))
	assert.EqualValues(t, "https://custom.example.com", BaseURLForProject(""))
}

func TestNewDescopeClientWithProjectID(t *testing.T) {
	// make sure projectID env variable is set (it should be ignored)
	t.Setenv(descope.EnvironmentVariableProjectID, "some_project_id")
	// Test with a valid project ID
	projectID := "testProjectID"
	client, err := NewDescopeClientWithProjectID(projectID, cctx.Logger(context.Background()))
	require.NoError(t, err)
	require.NotNil(t, client)
	// Test with an invalid project ID
	projectID = ""
	_, err = NewDescopeClientWithProjectID(projectID, cctx.Logger(context.Background()))
	require.Error(t, err)
}
