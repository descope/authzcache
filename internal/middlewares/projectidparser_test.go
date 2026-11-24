package middlewares

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/backend/common/pkg/common/grpc/httpgateway"
	"github.com/descope/backend/common/pkg/common/utils"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
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

func TestProjectIDHeaderMatcher(t *testing.T) {
	tests := []struct {
		header  string
		wantKey string
		wantOK  bool
	}{
		{header: "Grpc-Metadata-Project-Id", wantKey: "project-id", wantOK: true},
		{header: "grpc-metadata-project-id", wantKey: "project-id", wantOK: true},
		{header: "Grpc-Metadata-Tenant-Id"},
		{header: "Grpc-Metadata-Authorization"},
		{header: "X-Custom"},
		{header: "Authorization", wantKey: runtime.MetadataPrefix + "Authorization", wantOK: true},
	}
	for _, tc := range tests {
		t.Run(tc.header, func(t *testing.T) {
			key, ok := ProjectIDHeaderMatcher(tc.header)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantKey, key)
		})
	}
}

func TestProjectIDReachesGRPCMetadata(t *testing.T) {
	req, err := http.NewRequest("GET", "/whatever", nil)
	require.NoError(t, err)
	projectID := utils.CreateProjectID()
	req.Header.Set(cctx.AuthorizationHeader, bearerAuthorizationPrefix+projectID)
	req.Header.Set(mdProjectHeader, "planted")
	req.Header.Set(httpgateway.MetadataHeader(cctx.ContextKeyTenantIDHeader), "planted")
	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(ProjectIDHeaderMatcher))
	var md metadata.MD
	gateway := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx, err := runtime.AnnotateIncomingContext(r.Context(), mux, r, "/authzcv1.AuthzCache/Check")
		require.NoError(t, err)
		md, _ = metadata.FromIncomingContext(ctx)
	})
	ProjectIDParser(context.Background())(gateway).ServeHTTP(httptest.NewRecorder(), req)
	assert.Equal(t, []string{projectID}, md.Get(cctx.ContextKeyProjectIDHeader))
	assert.Empty(t, md.Get(cctx.ContextKeyTenantIDHeader))
}
