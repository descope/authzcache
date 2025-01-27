package client

import (
	"testing"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/grpc/grpcclient"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	ctx, _ := cctx.CreateMainContext(cctx.ContextKeyServiceID, t.Name(), nil)
	usc, err := NewClient(ctx, &grpcclient.Config{})
	require.NoError(t, err)
	require.NotNil(t, usc)
	require.NotNil(t, usc.GrpcClient())
}
