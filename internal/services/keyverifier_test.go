package services

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/services/remote"
	cctx "github.com/descope/backend/common/pkg/common/context"
	ce "github.com/descope/backend/common/pkg/common/errors"
	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCallerKey = "caller-key"

func keyCtx() context.Context {
	return remote.WithCallerKey(context.TODO(), testCallerKey)
}

func sdkErr(status int) *descope.Error {
	return &descope.Error{Code: "E011003", Info: map[string]any{descope.ErrorInfoKeys.HTTPResponseStatusCode: status}}
}

func rejectedKeyErr() error {
	return sdkErr(http.StatusUnauthorized)
}

func TestKeyVerifierEnsureCachesSuccess(t *testing.T) {
	var calls int
	v := &keyVerifier{verify: func(_ context.Context, _, _ string) error {
		calls++
		return nil
	}}
	require.NoError(t, v.ensure(context.TODO(), "p1", "k1"))
	require.NoError(t, v.ensure(context.TODO(), "p1", "k1"))
	assert.Equal(t, 1, calls)
	// same key on another project is verified separately
	require.NoError(t, v.ensure(context.TODO(), "p2", "k1"))
	assert.Equal(t, 2, calls)
}

func TestKeyVerifierEnsureDoesNotCacheFailure(t *testing.T) {
	var calls int
	v := &keyVerifier{verify: func(_ context.Context, _, _ string) error {
		calls++
		return rejectedKeyErr()
	}}
	require.Error(t, v.ensure(context.TODO(), "p1", "k1"))
	require.Error(t, v.ensure(context.TODO(), "p1", "k1"))
	assert.Equal(t, 2, calls)
}

func TestKeyVerifierMarkVerifiedSkipsVerify(t *testing.T) {
	v := &keyVerifier{verify: func(_ context.Context, _, _ string) error {
		t.Fatal("verify should not be called")
		return nil
	}}
	v.markVerified("p1", "k1")
	require.NoError(t, v.ensure(context.TODO(), "p1", "k1"))
}

func TestKeyVerifierRevalidateDropsOnlyRejectedKeys(t *testing.T) {
	results := map[string]error{
		"valid":     nil,
		"revoked":   rejectedKeyErr(),
		"forbidden": sdkErr(http.StatusForbidden),
		"outage":    sdkErr(http.StatusServiceUnavailable),
	}
	verifyCalls := map[string]int{}
	v := &keyVerifier{verify: func(_ context.Context, _, key string) error {
		verifyCalls[key]++
		return results[key]
	}}
	for key := range results {
		v.markVerified("p1", key)
	}
	v.revalidate(context.TODO())
	for key := range results {
		assert.Equal(t, 1, verifyCalls[key], key)
	}
	// a key still cached is served without verifying again
	for key, kept := range map[string]bool{"valid": true, "revoked": false, "forbidden": false, "outage": true} {
		_ = v.ensure(context.TODO(), "p1", key)
		if kept {
			assert.Equal(t, 1, verifyCalls[key], key)
		} else {
			assert.Equal(t, 2, verifyCalls[key], key)
		}
	}
}

func TestKeyVerifierStartRevalidation(t *testing.T) {
	ctx := t.Context()
	revalidated := make(chan struct{}, 1)
	v := &keyVerifier{verify: func(_ context.Context, _, _ string) error {
		select {
		case revalidated <- struct{}{}:
		default:
		}
		return nil
	}}
	v.markVerified("p1", "k1")
	v.startRevalidation(ctx, time.Millisecond)
	select {
	case <-revalidated:
	case <-time.After(5 * time.Second):
		t.Fatal("revalidation did not run")
	}
}

func checkAllCached(mockCache *mockCache) {
	mockCache.CheckRelationsFunc = func(_ context.Context, relations []*descope.FGARelation, _ map[string]any) ([]*descope.FGACheck, []*descope.FGARelation, map[int]*descope.FGACheck) {
		checks := make([]*descope.FGACheck, len(relations))
		indexed := map[int]*descope.FGACheck{}
		for i, r := range relations {
			checks[i] = &descope.FGACheck{Allowed: true, Relation: r, Info: &descope.FGACheckInfo{}}
			indexed[i] = checks[i]
		}
		return checks, nil, indexed
	}
}

var testRelations = []*descope.FGARelation{{Resource: "r1", ResourceType: "doc", Relation: "owner", Target: "u1", TargetType: "user"}}

func TestCheckCacheHitVerifiesCallerKeyOnce(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	checkAllCached(mockCache)
	var verifyCalls int
	mockSDK.MockAuthz.GetModifiedAssert = func(_ time.Time) { verifyCalls++ }
	ctx := cctx.AddProjectID(keyCtx(), "proj1")
	for range 3 {
		_, err := ac.Check(ctx, testRelations, nil)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, verifyCalls)
}

func TestCheckCacheHitRejectsInvalidCallerKey(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	checkAllCached(mockCache)
	rejected := rejectedKeyErr()
	mockSDK.MockAuthz.GetModifiedError = rejected
	_, err := ac.Check(cctx.AddProjectID(keyCtx(), "proj1"), testRelations, nil)
	require.Equal(t, rejected, err)
}

func TestCheckCacheHitRequiresCallerKey(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	checkAllCached(mockCache)
	_, err := ac.Check(cctx.AddProjectID(context.TODO(), "proj1"), testRelations, nil)
	require.True(t, ce.MissingAuthorization.Is(err))
}

func TestCheckCacheHitSkipsVerificationWhenDisabled(t *testing.T) {
	t.Setenv(config.VerifyCallerKeyOnCacheHit.Flag, "false")
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	checkAllCached(mockCache)
	mockSDK.MockAuthz.GetModifiedAssert = func(_ time.Time) { t.Fatal("verification should be skipped") }
	_, err := ac.Check(cctx.AddProjectID(keyCtx(), "proj1"), testRelations, nil)
	require.NoError(t, err)
}

func TestCheckRemoteSuccessMarksCallerKeyVerified(t *testing.T) {
	ac, mockSDK, mockCache := injectAuthzMocks(t)
	ctx := cctx.AddProjectID(keyCtx(), "proj1")
	// first call misses the cache and goes to Descope with the caller's key
	mockCache.CheckRelationsFunc = func(_ context.Context, relations []*descope.FGARelation, _ map[string]any) ([]*descope.FGACheck, []*descope.FGARelation, map[int]*descope.FGACheck) {
		return nil, relations, map[int]*descope.FGACheck{}
	}
	mockCache.UpdateCacheWithChecksFunc = func(_ context.Context, _ []*descope.FGACheck) {}
	mockSDK.MockFGA.CheckWithContextResponse = []*descope.FGACheck{{Allowed: true, Relation: testRelations[0], Info: &descope.FGACheckInfo{}}}
	_, err := ac.Check(ctx, testRelations, nil)
	require.NoError(t, err)
	// the following cache hit needs no extra verification
	checkAllCached(mockCache)
	mockSDK.MockAuthz.GetModifiedAssert = func(_ time.Time) { t.Fatal("key should already be verified") }
	_, err = ac.Check(ctx, testRelations, nil)
	require.NoError(t, err)
}

func TestRemotePollingDoesNotUseCallerKey(t *testing.T) {
	ac, _, mockCache := injectAuthzMocks(t)
	checkAllCached(mockCache)
	var pollingCtx context.Context
	mockCache.StartRemoteChangesPollingFunc = func(ctx context.Context) { pollingCtx = ctx }
	_, err := ac.Check(cctx.AddProjectID(keyCtx(), "proj1"), testRelations, nil)
	require.NoError(t, err)
	require.NotNil(t, pollingCtx)
	assert.Empty(t, remote.CallerKey(pollingCtx))
}
