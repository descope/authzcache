package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/descope/authzcache/internal/services/caches"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrometheusHandler(t *testing.T) {
	collector := NewCollector()
	collector.Record("p1", APICheck, CallMetrics{CacheHit: true, DurationMs: 1}) // before the endpoint: not counted
	handler := newPrometheusHandler(collector, func() map[string]caches.Stats {
		return map[string]caches.Stats{"p1": {DirectEntries: 3}}
	})
	collector.Record("p1", APIWhoCanAccess, CallMetrics{CacheHit: true, CandidatesCount: 4, FilteredCount: 1, ResultSize: 3, DurationMs: 2})
	collector.Record("p1", APICheck, CallMetrics{CandidatesCount: 7, ResultSize: 7, DurationMs: 40})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, line := range []string{
		`authzcache_calls_total{api="WhoCanAccess",cache="hit",project_id="p1"} 1`,
		`authzcache_calls_total{api="Check",cache="miss",project_id="p1"} 1`,
		`authzcache_call_candidates_total{api="WhoCanAccess",cache="hit",project_id="p1"} 4`,
		`authzcache_call_candidates_total{api="Check",cache="miss",project_id="p1"} 7`,
		`authzcache_call_filtered_total{api="WhoCanAccess",cache="hit",project_id="p1"} 1`,
		`authzcache_call_results_total{api="Check",cache="miss",project_id="p1"} 7`,
		`authzcache_call_duration_seconds_count{api="Check",cache="miss",project_id="p1"} 1`,
		`authzcache_call_duration_seconds_sum{api="Check",cache="miss",project_id="p1"} 0.04`,
		`authzcache_cache_entries{cache="direct",project_id="p1"} 3`,
	} {
		assert.Contains(t, body, line)
	}
	assert.NotContains(t, body, `authzcache_calls_total{api="Check",cache="hit"`, "calls before the endpoint started are not counted")
	assert.NotContains(t, body, "go_memstats", "no Go runtime collectors")
}
