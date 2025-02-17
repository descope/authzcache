package middlewares

import (
	"context"
	"net/http"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/httpgateway"
	cm "github.com/descope/common/pkg/common/http/middlewares"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

var mdProjectHeader = httpgateway.MetadataHeader(cctx.ContextKeyProjectIDHeader)

func ProjectIDParser(ctx context.Context) func(h http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, ctx := tracer.StartSpanFromContext(ctx, "middleware.ProjectIDParser")
			pid, _, _ := cm.ParseAuthorizationHeader(ctx, r)
			r.Header.Set(mdProjectHeader, pid)
			span.Finish()
			next.ServeHTTP(w, r)
		})
	}
}
