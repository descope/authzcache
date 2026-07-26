package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/middlewares"
	"github.com/descope/authzcache/internal/services/metrics"
	authczv1 "github.com/descope/authzcache/pkg/authzcache/proto/v1"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
	mgmtmocks "github.com/descope/go-sdk/descope/tests/mocks/mgmt"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// startRealGateway wires the real authzcache stack — gateway http.Server → gRPC
// server → controller → service — reusing main's newAuthzController and
// registerGateway, mocking only the downstream Descope SDK.
func startRealGateway(t *testing.T, sdkDelay time.Duration) string {
	t.Helper()
	ctx := context.Background()

	// only mock: the downstream backend call, made to sleep to simulate a slow check
	mockSDK := &mgmtmocks.MockManagement{
		MockAuthz: &mgmtmocks.MockAuthz{},
		MockFGA: &mgmtmocks.MockFGA{
			CheckWithContextAssert: func(_ []*descope.FGARelation, _ map[string]any) { time.Sleep(sdkDelay) },
			CheckWithContextResponse: []*descope.FGACheck{{
				Allowed:  true,
				Relation: &descope.FGARelation{Resource: "doc:1", ResourceType: "doc", Relation: "viewer", Target: "user:1", TargetType: "user"},
				Info:     &descope.FGACheckInfo{Direct: true},
			}},
		},
	}
	remoteCreator := func(_ string, _ logger.LoggerInterface) (sdk.Management, error) { return mockSDK, nil }

	ctrl, err := newAuthzController(ctx, remoteCreator, metrics.NewCollector())
	require.NoError(t, err)

	grpcLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcSrv := grpc.NewServer()
	authczv1.RegisterAuthzCacheServer(grpcSrv, ctrl)
	go func() { _ = grpcSrv.Serve(grpcLn) }()
	t.Cleanup(grpcSrv.Stop)

	conn, err := grpc.NewClient(grpcLn.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	mux := runtime.NewServeMux()

	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: middlewares.ProjectIDParser(ctx)(mux)}
	require.NoError(t, registerGateway(ctx, mux, conn, srv))
	go func() { _ = srv.Serve(httpLn) }()
	t.Cleanup(func() { _ = srv.Close() })

	return httpLn.Addr().String()
}

func postCheck(t *testing.T, addr string) error {
	t.Helper()
	body := `{"tuples":[{"resource":"doc:1","resourceType":"doc","relation":"viewer","target":"user:1","targetType":"user"}]}`
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/mgmt/fga/check", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer P2example:key")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

// TestGatewayWriteTimeoutAffectsCheckEndpoint drives the real /v1/mgmt/fga/check
// endpoint with a slow (mocked) downstream: a low AUTHZCACHE_HTTP_WRITE_TIMEOUT_IN_SECONDS
// severs the connection (socket hang up), a high one lets the slow response through.
func TestGatewayWriteTimeoutAffectsCheckEndpoint(t *testing.T) {
	const sdkDelay = 2 * time.Second

	t.Run("low timeout cuts the connection", func(t *testing.T) {
		t.Setenv(config.ConfigKeyGatewayWriteTimeoutInSeconds, "1")
		require.Error(t, postCheck(t, startRealGateway(t, sdkDelay)))
	})

	t.Run("high timeout lets the slow response through", func(t *testing.T) {
		t.Setenv(config.ConfigKeyGatewayWriteTimeoutInSeconds, "10")
		require.NoError(t, postCheck(t, startRealGateway(t, sdkDelay)))
	})
}
