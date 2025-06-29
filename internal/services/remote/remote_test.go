package remote

import (
	"context"
	"testing"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/require"
)

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
