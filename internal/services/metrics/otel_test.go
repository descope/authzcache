package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func TestStartOTelDisabledWithoutEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	require.NoError(t, StartOTel(context.Background(), func() map[string]caches.Stats { panic("must not be read") }))
}

func TestStartOTelExportsOverOTLP(t *testing.T) {
	var mu sync.Mutex
	points := map[string]int64{} // metric name + attrs -> value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/metrics", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		req := &colmetricpb.ExportMetricsServiceRequest{}
		assert.NoError(t, proto.Unmarshal(body, req))
		mu.Lock()
		defer mu.Unlock()
		for _, rm := range req.ResourceMetrics {
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					dps := m.GetGauge().GetDataPoints()
					if s := m.GetSum(); s != nil {
						dps = s.GetDataPoints()
					}
					for _, dp := range dps {
						k := m.Name
						for _, a := range dp.Attributes {
							k += "|" + a.Key + "=" + a.Value.GetStringValue()
						}
						points[k] = dp.GetAsInt()
					}
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", srv.URL)
	t.Setenv("OTEL_SDK_DISABLED", "false")

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, StartOTel(ctx, func() map[string]caches.Stats {
		return map[string]caches.Stats{"p1": {DirectEntries: 3, IndirectEntries: 2, IndexEntries: 6, DirectKeyBytes: 90}}
	}))
	NewCollector().Record("p1", APICheck, CallMetrics{Relations: RelationCounts{HitsDirect: 4, Misses: 1}})
	cancel() // shutdown flushes

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(points) > 0
	}, 5*time.Second, 20*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, int64(3), points["authzcache.cache.entries|cache=direct|project_id=p1"])
	assert.Equal(t, int64(6), points["authzcache.cache.entries|cache=index|project_id=p1"])
	assert.Equal(t, int64(90), points["authzcache.cache.direct_key_bytes|project_id=p1"])
	assert.Equal(t, int64(4), points["authzcache.check.relations|api=Check|outcome=hit_direct|project_id=p1"])
	assert.Equal(t, int64(1), points["authzcache.check.relations|api=Check|outcome=miss|project_id=p1"])
	assert.Contains(t, points, "authzcache.process.heap_inuse")
}
