package main

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/config"
	cconfig "github.com/descope/backend/common/pkg/common/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHTTPWriteTimeoutInSeconds(t *testing.T) {
	t.Run("default is 25", func(t *testing.T) {
		assert.Equal(t, 25, config.GetHTTPWriteTimeoutInSeconds())
	})
	t.Run("authzcache var applies", func(t *testing.T) {
		t.Setenv(config.ConfigKeyHTTPWriteTimeoutInSeconds, "40")
		assert.Equal(t, 40, config.GetHTTPWriteTimeoutInSeconds())
	})
	t.Run("raw HTTP_GATEWAY_WRITE_TIMEOUT applies when authzcache var unset", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "15")
		assert.Equal(t, 15, config.GetHTTPWriteTimeoutInSeconds())
	})
	t.Run("authzcache var wins over raw", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "15")
		t.Setenv(config.ConfigKeyHTTPWriteTimeoutInSeconds, "40")
		assert.Equal(t, 40, config.GetHTTPWriteTimeoutInSeconds())
	})
}

// serveWithConfiguredTimeout mirrors the common gateway wiring: shared timeout first, then the authzcache override.
func serveWithConfiguredTimeout(t *testing.T, handlerDelay time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{
		WriteTimeout: time.Duration(cconfig.GetHTTPGatewayWriteTimeout()) * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(handlerDelay)
			w.WriteHeader(http.StatusOK)
		}),
	}
	setHTTPWriteTimeout(srv)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

func get(t *testing.T, addr string) error {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://" + addr + "/v1/mgmt/fga/check")
	if err == nil {
		_ = resp.Body.Close()
	}
	return err
}

func TestGatewayWriteTimeoutOverNetwork(t *testing.T) {
	const handlerDelay = 2 * time.Second // exceeds the 1s timeouts below, stays under the 25s default

	t.Run("authzcache var below handler delay cuts the connection", func(t *testing.T) {
		t.Setenv(config.ConfigKeyHTTPWriteTimeoutInSeconds, "1")
		require.Error(t, get(t, serveWithConfiguredTimeout(t, handlerDelay)))
	})
	t.Run("raw HTTP_GATEWAY_WRITE_TIMEOUT achieves the same effect", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "1")
		require.Error(t, get(t, serveWithConfiguredTimeout(t, handlerDelay)))
	})
	t.Run("default 25s lets a slow response through", func(t *testing.T) {
		require.NoError(t, get(t, serveWithConfiguredTimeout(t, handlerDelay)))
	})
}
