package main

import (
	"net"
	"net/http"
	"testing"
	"time"

	cconfig "github.com/descope/backend/common/pkg/common/config"
	"github.com/stretchr/testify/require"
)

// newGatewayLikeServer mirrors the gateway http.Server timeout wiring in
// common/pkg/common/grpc/server/server.go, which authzcache uses unmodified.
func newGatewayLikeServer(handler http.Handler) *http.Server {
	return &http.Server{
		ReadTimeout:       time.Duration(cconfig.GetHTTPGatewayReadTimeout()) * time.Second,
		WriteTimeout:      time.Duration(cconfig.GetHTTPGatewayWriteTimeout()) * time.Second,
		IdleTimeout:       time.Duration(cconfig.GetHTTPGatewayIdleTimeout()) * time.Second,
		ReadHeaderTimeout: time.Duration(cconfig.GetHTTPGatewayReadHeaderTimeout()) * time.Second,
		Handler:           handler,
	}
}

func serveSlow(t *testing.T, delay time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := newGatewayLikeServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func requestErr(t *testing.T, addr string) error {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://" + addr + "/anything")
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

// TestHTTPGatewayWriteTimeoutEnvHasEffect verifies that HTTP_GATEWAY_WRITE_TIMEOUT
// already governs the authzcache gateway today: a low value severs a slow response
// (socket hang up), a high value lets the same response through.
func TestHTTPGatewayWriteTimeoutEnvHasEffect(t *testing.T) {
	const handlerDelay = 2 * time.Second

	t.Run("low HTTP_GATEWAY_WRITE_TIMEOUT cuts the connection", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "1")
		require.Error(t, requestErr(t, serveSlow(t, handlerDelay)))
	})

	t.Run("high HTTP_GATEWAY_WRITE_TIMEOUT lets the response through", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "30")
		require.NoError(t, requestErr(t, serveSlow(t, handlerDelay)))
	})
}
