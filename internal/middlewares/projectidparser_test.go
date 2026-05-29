package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bearerAuthorizationPrefix = "Bearer "

func TestProjectId(t *testing.T) {
	// prepare request
	req, err := http.NewRequest("GET", "/whatever", nil)
	require.NoError(t, err)
	projectID := utils.CreateProjectID()
	req.Header.Set(cctx.AuthorizationHeader, bearerAuthorizationPrefix+projectID)
	rr := httptest.NewRecorder()
	var handlerCalled bool
	// prepare handler
	verifyContext := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// verify header includes project ID
		pid := r.Header.Get(mdProjectHeader)
		assert.Equal(t, projectID, pid)
	})
	handler := http.HandlerFunc(ProjectIDParser(context.Background())(verifyContext).ServeHTTP)
	// call handler
	handler.ServeHTTP(rr, req)
	// verify handler called and response status
	require.True(t, handlerCalled)
	assert.Equal(t, http.StatusOK, rr.Code)
}
