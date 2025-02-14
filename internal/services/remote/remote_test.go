package remote

import (
	"testing"

	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/require"
)

func TestNewDescopeClientWithProjectID(t *testing.T) {
	// remove default project ID environment variable
	t.Setenv(descope.EnvironmentVariableProjectID, "")
	// Test with a valid project ID
	projectID := "testProjectID"
	client, err := NewDescopeClientWithProjectID(projectID)
	require.NoError(t, err)
	require.NotNil(t, client)
	// Test with an invalid project ID
	projectID = ""
	_, err = NewDescopeClientWithProjectID(projectID)
	require.Error(t, err)
}
