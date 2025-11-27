package middlewares

import (
	"context"
	"net/http"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/httpgateway"
	cm "github.com/descope/common/pkg/common/http/middlewares"
)

var mdProjectHeader = httpgateway.MetadataHeader(cctx.ContextKeyProjectIDHeader)

func ProjectIDParser(ctx context.Context) func(h http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pid, _, _ := cm.ParseAuthorizationHeader(ctx, r)
			r.Header.Set(mdProjectHeader, pid)
			next.ServeHTTP(w, r)
		})
	}
}
