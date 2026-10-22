//go:build loadtest

// Loadtest build: replaces the Descope backend with an in-process fake so a stress test measures
// the edge cache alone. Never compiled into production builds — this fake allows every check.
package remote

import (
	"context"
	"math/rand/v2"
	"net/http"
	_ "net/http/pprof" // exposes /debug/pprof on pprofAddr
	"runtime"
	"time"

	"github.com/descope/go-sdk/descope"
	"github.com/descope/go-sdk/descope/sdk"
)

const (
	minBackendLatency = 10 * time.Millisecond
	maxBackendLatency = 50 * time.Millisecond
	pprofAddr         = ":6060"
)

func init() {
	println("!!! LOADTEST BUILD: remote backend is a fake that allows every check. NEVER run in production !!!")
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(int(time.Microsecond))
	go func() { _ = http.ListenAndServe(pprofAddr, nil) }() // for the contention/heap profiles
}

func fakeClient(_ string) (sdk.Management, bool) {
	return &fakeManagement{fga: &fakeFGA{}, authz: &fakeAuthz{}}, true
}

// Embedded nil interfaces satisfy the SDK interfaces at compile time; any method the load test
// doesn't exercise panics loudly instead of silently returning a zero value.
type fakeManagement struct {
	sdk.Management
	fga   sdk.FGA
	authz sdk.Authz
}

func (f *fakeManagement) FGA() sdk.FGA     { return f.fga }
func (f *fakeManagement) Authz() sdk.Authz { return f.authz }

type fakeFGA struct{ sdk.FGA }

// CheckWithContext answers every relation with a direct grant after a random 10-50ms delay.
// Direct-only on purpose: an indirect result purges the whole indirect cache on every write.
func (f *fakeFGA) CheckWithContext(ctx context.Context, relations []*descope.FGARelation, _ map[string]any) ([]*descope.FGACheck, error) {
	if err := sleepCtx(ctx, backendLatency()); err != nil {
		return nil, err
	}
	checks := make([]*descope.FGACheck, len(relations))
	for i, r := range relations {
		checks[i] = &descope.FGACheck{Allowed: true, Relation: r, Info: &descope.FGACheckInfo{Direct: true}}
	}
	return checks, nil
}

func (f *fakeFGA) Check(ctx context.Context, relations []*descope.FGARelation) ([]*descope.FGACheck, error) {
	return f.CheckWithContext(ctx, relations, nil)
}

func (f *fakeFGA) SetListConditions(bool) {}

func (f *fakeFGA) LoadSchema(context.Context) (*descope.FGASchema, error) {
	return &descope.FGASchema{}, nil
}

type fakeAuthz struct{ sdk.Authz }

// GetModified always reports no changes, so the cache never purges and grows monotonically.
func (f *fakeAuthz) GetModified(context.Context, time.Time) (*descope.AuthzModified, error) {
	return &descope.AuthzModified{}, nil
}

func backendLatency() time.Duration {
	return minBackendLatency + rand.N(maxBackendLatency-minBackendLatency+1)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
