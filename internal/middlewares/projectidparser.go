package middlewares

import (
	"context"
	"net/http"
	"strings"

	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/backend/common/pkg/common/grpc/httpgateway"
	cm "github.com/descope/backend/common/pkg/common/http/middlewares"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

var mdProjectHeader = httpgateway.MetadataHeader(cctx.ContextKeyProjectIDHeader)

// ProjectIDHeaderMatcher forwards the project ID header set by ProjectIDParser to gRPC metadata, on top of
// runtime.DefaultHeaderMatcher. grpc-gateway v2.33.0 stopped forwarding Grpc-Metadata-* headers by default.
// Forwarding this one is safe only while ProjectIDParser overwrites any value the client sent.
func ProjectIDHeaderMatcher(key string) (string, bool) {
	if strings.EqualFold(key, mdProjectHeader) {
		return strings.ToLower(cctx.ContextKeyProjectIDHeader), true
	}
	return runtime.DefaultHeaderMatcher(key)
}

func ProjectIDParser(ctx context.Context) func(h http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pid, _, _ := cm.ParseAuthorizationHeader(ctx, r)
			r.Header.Set(mdProjectHeader, pid)
			next.ServeHTTP(w, r)
		})
	}
}
