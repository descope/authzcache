package controllers

import (
	"context"
	"testing"

	"github.com/descope/authzcache/internal/services/remote"
	authzv1 "github.com/descope/backend/authzservice/pkg/authzservice/proto/v1"
	ce "github.com/descope/backend/common/pkg/common/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

const testCallerKey = "caller-key"

func keyCtx() context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", bearerPrefix+"proj1:"+testCallerKey))
}

func TestWithCallerKey(t *testing.T) {
	tests := []struct {
		name    string
		md      metadata.MD
		wantKey string
	}{
		{name: "project and key", md: metadata.Pairs("authorization", "Bearer proj1:K123"), wantKey: "K123"},
		{name: "project jwt and key forwarded as is", md: metadata.Pairs("authorization", "Bearer proj1:jwt:K123"), wantKey: "jwt:K123"},
		{name: "project only", md: metadata.Pairs("authorization", "Bearer proj1")},
		{name: "empty key", md: metadata.Pairs("authorization", "Bearer proj1:")},
		{name: "no header", md: metadata.MD{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := withCallerKey(metadata.NewIncomingContext(context.Background(), tc.md))
			if tc.wantKey == "" {
				require.True(t, ce.MissingAuthorization.Is(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantKey, remote.CallerKey(ctx))
		})
	}
}

func TestControllerRejectsMissingCallerKey(t *testing.T) {
	// unset AuthzCacheMock funcs panic, so reaching the service fails the test
	controller, _ := setup()
	ctx := context.Background()
	_, err := controller.CreateFGASchema(ctx, &authzv1.SaveDSLSchemaRequest{})
	assert.True(t, ce.MissingAuthorization.Is(err))
	_, err = controller.CreateFGARelations(ctx, &authzv1.CreateTuplesRequest{})
	assert.True(t, ce.MissingAuthorization.Is(err))
	_, err = controller.DeleteFGARelations(ctx, &authzv1.DeleteTuplesRequest{})
	assert.True(t, ce.MissingAuthorization.Is(err))
	_, err = controller.Check(ctx, &authzv1.CheckRequest{})
	assert.True(t, ce.MissingAuthorization.Is(err))
	_, err = controller.WhoCanAccess(ctx, &authzv1.WhoCanAccessRequest{})
	assert.True(t, ce.MissingAuthorization.Is(err))
	_, err = controller.WhatCanTargetAccess(ctx, &authzv1.WhatCanTargetAccessRequest{})
	assert.True(t, ce.MissingAuthorization.Is(err))
}
