package main

import (
	"context"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/controllers"
	"github.com/descope/authzcache/internal/services"
	authzcv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	cconfig "github.com/descope/common/pkg/common/config"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/server"
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
				as, err := services.New(ctx)
				if err != nil {
					cctx.Logger(ctx).Err(err).Msg("Failed creating authz cache")
					return err
				}
				ctrl := controllers.New(as)
				authzcv1.RegisterAuthzCacheServer(s, ctrl)
				return nil
			},
		},
		// TODO: Do we need to add HTTP handlers in order to enable HTTP endpoints?
		[]server.RegisterHTTPFunc{})

	if err != nil {
		cctx.Logger(ctx).Fatal().Str(config.MetricsKeyResourceServiceName, cconfig.GetServiceName()).Err(err).Msg("Failed to start server")
	}
}
