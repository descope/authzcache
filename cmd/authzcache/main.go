package main

import (
	"context"
	"net/http"
	"os"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/controllers"
	"github.com/descope/authzcache/internal/services"
	"github.com/descope/authzcache/internal/services/caches"
	authzcv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	cconfig "github.com/descope/common/pkg/common/config"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/server"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/client"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

const defaultServiceName = "authzcache"

func main() {
	serve()
}

func serve() {
	server.InitServiceName(defaultServiceName)

	ctx, err := server.StartServerWithGateway(
		[]server.RegisterGRPCFunc{
			func(ctx context.Context, s *grpc.Server) error {
				// sdk init
				baseURL := os.Getenv(descope.EnvironmentVariableBaseURL) // TODO: used for testing inside descope local env, should probably be removed
				descopeClient, err := client.NewWithConfig(&client.Config{
					SessionJWTViaCookie: true,
					DescopeBaseURL:      baseURL,
					LogLevel:            logger.LogDebugLevel, // TODO: extract to env var
				})
				if err != nil {
					return err
				}
				//authz cache service init
				as, err := services.New(ctx, descopeClient.Management, caches.ProjectAuthzCacheCreator{})
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
		})

	if err != nil {
		cctx.Logger(ctx).Fatal().Str(config.MetricsKeyResourceServiceName, cconfig.GetServiceName()).Err(err).Msg("Failed to start server")
	}
}
