package services

import (
	"testing"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/services/caches"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/logger"
	"github.com/descope/go-sdk/descope/sdk"
	mocksmgmt "github.com/descope/go-sdk/descope/tests/mocks/mgmt"
)

// BenchmarkCheckCacheHit measures a fully cached Check with caller key verification on and off
func BenchmarkCheckCacheHit(b *testing.B) {
	for _, verify := range []string{"true", "false"} {
		b.Run("verify="+verify, func(b *testing.B) {
			b.Setenv(config.VerifyCallerKeyOnCacheHit.Flag, verify)
			mockSDK := &mocksmgmt.MockManagement{MockFGA: &mocksmgmt.MockFGA{}, MockAuthz: &mocksmgmt.MockAuthz{}}
			mockSDK.MockAuthz.GetModifiedResponse = &descope.AuthzModified{}
			mockSDK.MockFGA.CheckWithContextResponse = []*descope.FGACheck{{Allowed: true, Relation: testRelations[0], Info: &descope.FGACheckInfo{Direct: true}}}
			ac, err := New(b.Context(), caches.NewProjectAuthzCache, func(_ string, _ logger.LoggerInterface) (sdk.Management, error) {
				return mockSDK, nil
			}, nil)
			if err != nil {
				b.Fatal(err)
			}
			ctx := cctx.AddProjectID(keyCtx(), "proj1")
			// warm the relation cache, and the key cache when verification is on
			for range 2 {
				if _, err := ac.Check(ctx, testRelations, nil); err != nil {
					b.Fatal(err)
				}
			}
			mockSDK.MockFGA.CheckWithContextAssert = func(_ []*descope.FGARelation, _ map[string]any) { b.Fatal("expected a cache hit") }
			mockSDK.MockAuthz.GetModifiedAssert = nil
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := ac.Check(ctx, testRelations, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
