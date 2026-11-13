package metrics

import (
	"context"
	"runtime"
	"time"

	"github.com/descope/authzcache/internal/config"
	"github.com/descope/authzcache/internal/services/caches"
	cctx "github.com/descope/backend/common/pkg/common/context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

const meterName = "github.com/descope/authzcache"

// Global meter delegates to the provider once StartOTel sets it; no-op until then.
var relationsCounter, _ = otel.Meter(meterName).Int64Counter("authzcache.check.relations",
	metric.WithDescription("Check relations by cache outcome"), metric.WithUnit("{relation}"))

func recordRelationsOTel(projectID string, api APIName, rc RelationCounts) {
	ctx := context.Background()
	if !relationsCounter.Enabled(ctx) {
		return // no attribute allocations while export is off
	}
	add := func(n int64, outcome string) {
		if n > 0 {
			relationsCounter.Add(ctx, n, metric.WithAttributes(
				attribute.String("project_id", projectID), attribute.String("api", string(api)), attribute.String("outcome", outcome)))
		}
	}
	add(rc.HitsDirect, "hit_direct")
	add(rc.HitsIndirect, "hit_indirect")
	add(rc.HitsConditional, "hit_conditional")
	add(rc.Misses, "miss")
}

// StartOTel exports metrics over OTLP/HTTP, configured by the standard OTEL_* env vars.
func StartOTel(ctx context.Context, cacheStats func() map[string]caches.Stats) error {
	if !config.GetOTelMetricsEnabled() {
		return nil
	}
	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return err
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "authzcache")),
		resource.WithFromEnv(), // OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES win
		resource.WithTelemetrySDK())
	if err != nil {
		return err // notest
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	if err := registerCacheGauges(provider.Meter(meterName), cacheStats); err != nil {
		return err // notest
	}
	otel.SetMeterProvider(provider)
	cctx.Go(ctx, func(ctx context.Context) {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(shutdownCtx) // flushes the last window
	})
	cctx.Logger(ctx).Info().Msg("OTel metrics export started")
	return nil
}

func registerCacheGauges(meter metric.Meter, cacheStats func() map[string]caches.Stats) error {
	entries, err := meter.Int64ObservableGauge("authzcache.cache.entries", metric.WithDescription("Entries per project cache or index"), metric.WithUnit("{entry}"))
	if err != nil {
		return err // notest
	}
	keyBytes, err := meter.Int64ObservableGauge("authzcache.cache.direct_key_bytes", metric.WithDescription("Bytes of direct cache key strings"), metric.WithUnit("By"))
	if err != nil {
		return err // notest
	}
	heap, err := meter.Int64ObservableGauge("authzcache.process.heap_inuse", metric.WithDescription("Go heap in use by the whole process"), metric.WithUnit("By"))
	if err != nil {
		return err // notest
	}
	_, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		o.ObserveInt64(heap, int64(m.HeapInuse)) //nolint:gosec // heap stays far below MaxInt64
		for projectID, s := range cacheStats() {
			p := attribute.String("project_id", projectID)
			for cache, n := range map[string]int{"direct": s.DirectEntries, "indirect": s.IndirectEntries, "lookup": s.LookupEntries, "index": s.IndexEntries} {
				o.ObserveInt64(entries, int64(n), metric.WithAttributes(p, attribute.String("cache", cache)))
			}
			o.ObserveInt64(keyBytes, s.DirectKeyBytes, metric.WithAttributes(p))
		}
		return nil
	}, entries, keyBytes, heap)
	return err
}
