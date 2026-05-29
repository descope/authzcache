package client

import (
	"context"

	grpcauthzcv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	"github.com/descope/common/pkg/common/config"
	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/grpcclient"
)

type AuthzClient struct {
	grpcClient grpcauthzcv1.AuthzCacheClient
}

func NewClient(ctx context.Context, grpcClientConfig *grpcclient.Config) (*AuthzClient, error) {
	if grpcClientConfig.ServiceAddress == "" {
		grpcClientConfig.ServiceAddress = config.GetAuthzCacheGRPCHost()
	}
	if grpcClientConfig.ServicePort == "" {
		grpcClientConfig.ServicePort = config.GetAuthzCacheGRPCPort()
	}

	grpcClientConfig.HealthID = cctx.HealthAuthzCacheGRPC
	grpcClient, err := grpcclient.NewClient(ctx, grpcClientConfig)
	if err != nil {
		return nil, err
	}
	return &AuthzClient{
		grpcClient: grpcauthzcv1.NewAuthzCacheClient(grpcClient.GetConnection()),
	}, nil
}

func (c *AuthzClient) GrpcClient() grpcauthzcv1.AuthzCacheClient {
	return c.grpcClient
}
