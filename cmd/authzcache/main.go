package main

import (
	"context"
	"net/http"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/controllers"
	"github.com/descope/authzcache/internal/middlewares"
	"github.com/descope/authzcache/internal/services"
	"github.com/descope/authzcache/internal/services/caches"
	"github.com/descope/authzcache/internal/services/remote"
	authzcv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	cconfig "github.com/descope/common/pkg/common/config"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/server"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

const defaultServiceName = "authzcache"

func main() {
	serve()
}

func serve() {
	server.InitServiceName(defaultServiceName)

	ctx, err := server.StartServerWithGatewayAndOptions(
		[]server.RegisterGRPCFunc{
			func(ctx context.Context, s *grpc.Server) error {
				//authz cache service init
				as, err := services.New(ctx, caches.NewProjectAuthzCache, remote.NewDescopeClientWithProjectID)
				if err != nil {
					cctx.Logger(ctx).Err(err).Msg("Failed creating authz cache")
					return err
				}
				ctrl := controllers.New(as)
				authzcv1.RegisterAuthzCacheServer(s, ctrl)
				return nil
			},
		},
		[]server.RegisterHTTPFunc{
			func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn, _ *http.Server) error {
				return authzcv1.RegisterAuthzCacheHandler(ctx, mux, conn)
			},
		},
		server.ServerOptions{
			ServiceMiddlewares: []func(context.Context) func(h http.Handler) http.Handler{
				middlewares.ProjectIDParser,
			},
			SkipInitCommonProjectCache: true,
		})

	if err != nil {
		cctx.Logger(ctx).Fatal().Str(config.MetricsKeyResourceServiceName, cconfig.GetServiceName()).Err(err).Msg("Failed to start server")
	}
}
