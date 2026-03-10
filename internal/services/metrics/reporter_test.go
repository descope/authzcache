package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/descope/common/pkg/common/utils"
	"github.com/stretchr/testify/require"
)

func TestReporterPostsMetrics(t *testing.T) {
	var receivedBody metricsRequest
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/mgmt/fga/cache/metrics", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		receivedAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = utils.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	collector := NewCollector()
	collector.Record("proj1", APIWhoCanAccess, CallMetrics{CacheHit: true, CandidatesCount: 10, FilteredCount: 2, ResultSize: 8, DurationMs: 5})
	collector.Record("proj1", APIWhoCanAccess, CallMetrics{CacheHit: false, CandidatesCount: 0, FilteredCount: 0, ResultSize: 3, DurationMs: 30})

	reporter := NewReporter(collector, srv.URL, "mgmt-key", 1, true)
	reporter.report(context.Background())

	require.Equal(t, "Bearer proj1:mgmt-key", receivedAuth)
	require.Len(t, receivedBody.Metrics, 1)
	m := receivedBody.Metrics[0]
	require.Equal(t, string(APIWhoCanAccess), m.API)
	require.Equal(t, int64(1), m.HitCount)
	require.Equal(t, int64(1), m.MissCount)
	require.Equal(t, int64(2), m.TotalCalls)
	require.InDelta(t, 10.0, float64(m.AvgHitCandidates), 0.01) // 10 candidates / 1 hit
	require.InDelta(t, 2.0, float64(m.AvgHitFiltered), 0.01)    // 2 filtered / 1 hit
	require.InDelta(t, 5.5, m.AvgResultSize, 0.01)
	require.Equal(t, int64(5), m.AvgDurationHitMs)
	require.Equal(t, int64(5), m.MinDurationHitMs)
	require.Equal(t, int64(5), m.MaxDurationHitMs)
	require.Equal(t, int64(30), m.AvgDurationMissMs)
	require.Equal(t, int64(30), m.MinDurationMissMs)
	require.Equal(t, int64(30), m.MaxDurationMissMs)
	require.Equal(t, int64(5), m.MinDurationMs)
	require.Equal(t, int64(30), m.MaxDurationMs)
}

func TestReporterDisabled(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	collector := NewCollector()
	collector.Record("proj1", APIWhoCanAccess, CallMetrics{CacheHit: true, DurationMs: 1})

	reporter := NewReporter(collector, srv.URL, "key", 1, false)
	ctx, cancel := context.WithCancel(context.Background())
	reporter.Start(ctx)
	cancel()
	time.Sleep(50 * time.Millisecond)

	require.False(t, called, "disabled reporter should not post")
}

func TestReporterHandlesHTTPError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	collector := NewCollector()
	collector.Record("proj1", APIWhoCanAccess, CallMetrics{CacheHit: true, DurationMs: 1})

	reporter := NewReporter(collector, srv.URL, "key", 1, true)
	// should not panic on 500
	reporter.report(context.Background())
	require.Equal(t, 1, callCount)
}

func TestReporterComputesAverages(t *testing.T) {
	var receivedBody metricsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = utils.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	collector := NewCollector()
	// 3 cache hits with durations 10, 20, 30; candidates 5, 10, 15; filtered 1, 2, 3; results 4, 8, 12
	for i, d := range []int64{10, 20, 30} {
		collector.Record("proj1", APIWhoCanAccess, CallMetrics{
			CacheHit:        true,
			CandidatesCount: (i + 1) * 5,
			FilteredCount:   (i + 1),
			ResultSize:      (i + 1) * 4,
			DurationMs:      d,
		})
	}

	reporter := NewReporter(collector, srv.URL, "key", 1, true)
	reporter.report(context.Background())

	require.Len(t, receivedBody.Metrics, 1)
	m := receivedBody.Metrics[0]
	require.Equal(t, int64(3), m.TotalCalls)
	require.Equal(t, int64(3), m.HitCount)
	require.Equal(t, int64(0), m.MissCount)
	require.InDelta(t, 10.0, m.AvgHitCandidates, 0.01) // (5+10+15)/3 hits
	require.InDelta(t, 2.0, m.AvgHitFiltered, 0.01)    // (1+2+3)/3 hits
	require.InDelta(t, 8.0, m.AvgResultSize, 0.01)     // (4+8+12)/3
	require.Equal(t, int64(20), m.AvgDurationHitMs)    // (10+20+30)/3
	require.Equal(t, int64(10), m.MinDurationHitMs)
	require.Equal(t, int64(30), m.MaxDurationHitMs)
	require.Equal(t, int64(20), m.AvgDurationMs)
	require.Equal(t, int64(10), m.MinDurationMs)
	require.Equal(t, int64(30), m.MaxDurationMs)
}

func TestReporterSkipsEmptySnapshot(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	collector := NewCollector()
	reporter := NewReporter(collector, srv.URL, "key", 1, true)
	reporter.report(context.Background())

	require.False(t, called, "no metrics to report, should not post")
}
