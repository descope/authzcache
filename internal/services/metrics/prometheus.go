package metrics

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// promMetrics are cumulative, as Prometheus counters must be; the reporter's window resets every interval.
type promMetrics struct {
	calls      *prometheus.CounterVec
	duration   *prometheus.HistogramVec
	candidates *prometheus.CounterVec
	filtered   *prometheus.CounterVec
	results    *prometheus.CounterVec
}

func (pm *promMetrics) record(projectID string, api APIName, m CallMetrics) {
	cache := "miss"
	if m.CacheHit {
		cache = "hit"
	}
	labels := []string{projectID, string(api), cache}
	pm.calls.WithLabelValues(labels...).Inc()
	pm.duration.WithLabelValues(labels...).Observe(float64(m.DurationMs) / 1000)
	pm.candidates.WithLabelValues(labels...).Add(float64(m.CandidatesCount))
	pm.filtered.WithLabelValues(labels...).Add(float64(m.FilteredCount))
	pm.results.WithLabelValues(labels...).Add(float64(m.ResultSize))
}

// cacheEntries reads the cache sizes on each scrape.
type cacheEntries struct {
	desc  *prometheus.Desc
	stats func() map[string]caches.Stats
}

func (c cacheEntries) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c cacheEntries) Collect(ch chan<- prometheus.Metric) {
	for projectID, s := range c.stats() {
		for cache, n := range map[string]int{"direct": s.DirectEntries, "indirect": s.IndirectEntries, "lookup": s.LookupEntries} {
			ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(n), projectID, cache)
		}
	}
}

// newPrometheusHandler starts recording calls into collector and returns the scrape handler.
func newPrometheusHandler(collector *Collector, cacheStats func() map[string]caches.Stats) http.Handler {
	labels := []string{"project_id", "api", "cache"}
	counter := func(name, help string) *prometheus.CounterVec {
		return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	}
	pm := &promMetrics{
		calls:      counter("authzcache_calls_total", "API calls by cache outcome."),
		duration:   prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "authzcache_call_duration_seconds", Help: "API call duration.", Buckets: prometheus.DefBuckets}, labels),
		candidates: counter("authzcache_call_candidates_total", "Candidates across API calls, as summed in the Descope report."),
		filtered:   counter("authzcache_call_filtered_total", "Candidates filtered out across API calls."),
		results:    counter("authzcache_call_results_total", "Results returned across API calls."),
	}
	// own registry: only our metrics, no Go runtime collectors
	reg := prometheus.NewRegistry()
	reg.MustRegister(pm.calls, pm.duration, pm.candidates, pm.filtered, pm.results, cacheEntries{
		desc:  prometheus.NewDesc("authzcache_cache_entries", "Entries per project cache.", []string{"project_id", "cache"}, nil),
		stats: cacheStats,
	})
	collector.prom.Store(pm)
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// ServePrometheus serves GET /metrics on its own port until ctx ends.
func ServePrometheus(ctx context.Context, port int, collector *Collector, cacheStats func() map[string]caches.Stats) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", newPrometheusHandler(collector, cacheStats))
	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	cctx.Go(ctx, func(ctx context.Context) {
		<-ctx.Done()
		_ = srv.Close()
	})
	cctx.Go(ctx, func(ctx context.Context) {
		cctx.Logger(ctx).Info().Int("port", port).Msg("Serving Prometheus metrics")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cctx.Logger(ctx).Error().Err(err).Int("port", port).Msg("Prometheus metrics endpoint stopped") // notest
		}
	})
}
