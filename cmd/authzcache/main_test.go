package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/config"
	cconfig "github.com/descope/backend/common/pkg/common/config"
	"github.com/stretchr/testify/assert"
)

func TestGetGatewayWriteTimeoutInSeconds(t *testing.T) {
	t.Run("authzcache var applies", func(t *testing.T) {
		t.Setenv(config.ConfigKeyGatewayWriteTimeoutInSeconds, "40")
		assert.Equal(t, 40, config.GetGatewayWriteTimeoutInSeconds())
	})
	t.Run("raw HTTP_GATEWAY_WRITE_TIMEOUT applies when authzcache var unset", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "15")
		assert.Equal(t, 15, config.GetGatewayWriteTimeoutInSeconds())
	})
	t.Run("authzcache var wins over raw", func(t *testing.T) {
		t.Setenv(cconfig.ConfigKeyHTTPWriteTimeout, "15")
		t.Setenv(config.ConfigKeyGatewayWriteTimeoutInSeconds, "40")
		assert.Equal(t, 40, config.GetGatewayWriteTimeoutInSeconds())
	})
}

func TestSetGatewayWriteTimeout(t *testing.T) {
	t.Run("applies the default", func(t *testing.T) {
		srv := &http.Server{ReadHeaderTimeout: time.Second}
		setGatewayWriteTimeout(srv)
		assert.Equal(t, time.Duration(config.GetGatewayWriteTimeoutInSeconds())*time.Second, srv.WriteTimeout)
	})
	t.Run("applies the configured override", func(t *testing.T) {
		t.Setenv(config.ConfigKeyGatewayWriteTimeoutInSeconds, "45")
		srv := &http.Server{ReadHeaderTimeout: time.Second}
		setGatewayWriteTimeout(srv)
		assert.Equal(t, 45*time.Second, srv.WriteTimeout)
	})
}
