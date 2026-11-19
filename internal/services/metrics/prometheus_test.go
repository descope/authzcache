package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusHandler(t *testing.T) {
	collector := NewCollector()
	collector.Record("p1", APICheck, CallMetrics{CacheHit: true, DurationMs: 1}) // before the endpoint: not counted
	handler := newPrometheusHandler(collector, func() map[string]caches.Stats {
		return map[string]caches.Stats{"p1": {DirectEntries: 3, IndirectEntries: 2}}
	})
	collector.Record("p1", APIWhoCanAccess, CallMetrics{CacheHit: true, CandidatesCount: 4, FilteredCount: 1, ResultSize: 3, Duration: 300 * time.Microsecond})
	collector.Record("p1", APICheck, CallMetrics{CandidatesCount: 7, ResultSize: 7, Duration: 40 * time.Millisecond})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, line := range []string{
		`authzcache_calls_total{api="WhoCanAccess",project_id="p1",result="hit"} 1`,
		`authzcache_calls_total{api="Check",project_id="p1",result="miss"} 1`,
		`authzcache_call_candidates_total{api="WhoCanAccess",project_id="p1",result="hit"} 4`,
		`authzcache_call_candidates_total{api="Check",project_id="p1",result="miss"} 7`,
		`authzcache_call_filtered_total{api="WhoCanAccess",project_id="p1",result="hit"} 1`,
		`authzcache_call_results_total{api="Check",project_id="p1",result="miss"} 7`,
		`authzcache_call_duration_seconds_count{api="Check",project_id="p1",result="miss"} 1`,
		`authzcache_call_duration_seconds_sum{api="Check",project_id="p1",result="miss"} 0.04`,
		`authzcache_call_duration_seconds_sum{api="WhoCanAccess",project_id="p1",result="hit"} 0.0003`,
		`authzcache_call_duration_seconds_bucket{api="WhoCanAccess",project_id="p1",result="hit",le="0.0004"} 1`,
		`authzcache_cache_entries{cache="direct",project_id="p1"} 3`,
		`authzcache_cache_entries{cache="indirect",project_id="p1"} 2`,
		`authzcache_cache_entries{cache="lookup",project_id="p1"} 0`,
	} {
		assert.Contains(t, body, line)
	}
	assert.NotContains(t, body, `authzcache_calls_total{api="Check",project_id="p1",result="hit"}`, "calls before the endpoint started are not counted")
	assert.NotContains(t, body, "go_memstats", "no Go runtime collectors")
}
