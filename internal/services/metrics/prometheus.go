package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/descope/authzcache/internal/services/caches"
	cconfig "github.com/descope/backend/common/pkg/common/config"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// promMetrics are cumulative, as Prometheus counters must be; the reporter's window resets every interval.
type promMetrics struct {
	calls    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func (pm *promMetrics) record(projectID string, api APIName, m CallMetrics) {
	result := "miss"
	if m.CacheHit {
		result = "hit"
	}
	labels := []string{projectID, string(api), result}
	pm.calls.WithLabelValues(labels...).Inc()
	pm.duration.WithLabelValues(labels...).Observe(m.Duration.Seconds())
}

// cacheEntries reads the cache sizes on each scrape.
type cacheEntries struct {
	desc  *prometheus.Desc
	stats func() map[string]caches.Stats
	seen  *sync.Map // project IDs that ever had entries; they keep reporting 0 after a purge
}

func (c cacheEntries) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c cacheEntries) Collect(ch chan<- prometheus.Metric) {
	for projectID, s := range c.stats() {
		if s == (caches.Stats{}) {
			if _, ok := c.seen.Load(projectID); !ok {
				continue // never cached anything, e.g. a bogus project ID
			}
		} else {
			c.seen.Store(projectID, struct{}{})
		}
		for cache, n := range map[string]int{"direct": s.DirectEntries, "indirect": s.IndirectEntries, "lookup": s.LookupEntries} {
			ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(n), projectID, cache)
		}
	}
}

// newPrometheusHandler starts recording calls into collector and returns the scrape handler.
func newPrometheusHandler(collector *Collector, cacheStats func() map[string]caches.Stats) http.Handler {
	labels := []string{"project_id", "api", "result"}
	pm := &promMetrics{
		calls:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "authzcache_calls_total", Help: "API calls by cache outcome."}, labels),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "authzcache_call_duration_seconds", Help: "API call duration.", Buckets: prometheus.ExponentialBuckets(0.0001, 4, 10)}, labels),
	}
	// own registry: only our metrics, no Go runtime collectors
	reg := prometheus.NewRegistry()
	reg.MustRegister(pm.calls, pm.duration, cacheEntries{
		desc:  prometheus.NewDesc("authzcache_cache_entries", "Entries per project cache.", []string{"project_id", "cache"}, nil),
		stats: cacheStats,
		seen:  &sync.Map{},
	})
	collector.prom.Store(pm)
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// ServePrometheus serves GET /metrics on its own port until ctx ends.
func ServePrometheus(ctx context.Context, port int, collector *Collector, cacheStats func() map[string]caches.Stats) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", newPrometheusHandler(collector, cacheStats))
	srv := &http.Server{Addr: net.JoinHostPort(cconfig.GetHTTPHost(), strconv.Itoa(port)), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	// drain like the gateway does, so a /metrics read isn't cut off on SIGTERM
	cctx.ShutdownHandler(ctx).AddPreShutdownStep(func(ctx context.Context) error {
		if timeout := time.Duration(cconfig.GetGracefulShutdownDrainTimeout()) * time.Second; timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		if err := srv.Shutdown(ctx); err != nil {
			_ = srv.Close() // notest
		}
		return nil
	})
	cctx.Go(ctx, func(ctx context.Context) {
		cctx.Logger(ctx).Info().Int("port", port).Msg("Serving Prometheus metrics")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cctx.Logger(ctx).Error().Err(err).Int("port", port).Msg("Prometheus metrics endpoint stopped") // notest
		}
	})
}
