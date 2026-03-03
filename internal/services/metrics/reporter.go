package metrics

import (
	"bytes"
	"context"
	"math"
	"net/http"
	"time"

	cctx "github.com/descope/common/pkg/common/context"
	"github.com/descope/common/pkg/common/utils"
)

// APIMetricsPayload is the JSON payload for a single API's metrics.
type APIMetricsPayload struct {
	API               string  `json:"api"`
	HitCount          int64   `json:"hit_count"`
	MissCount         int64   `json:"miss_count"`
	TotalCalls        int64   `json:"total_calls"`
	AvgHitCandidates  float64 `json:"avg_hit_candidates"`
	AvgHitFiltered    float64 `json:"avg_hit_filtered"`
	AvgResultSize     float64 `json:"avg_result_size"`
	AvgDurationMs     int64   `json:"avg_duration_ms"`
	MinDurationMs     int64   `json:"min_duration_ms"`
	MaxDurationMs     int64   `json:"max_duration_ms"`
	AvgDurationHitMs  int64   `json:"avg_duration_hit_ms"`
	MinDurationHitMs  int64   `json:"min_duration_hit_ms"`
	MaxDurationHitMs  int64   `json:"max_duration_hit_ms"`
	AvgDurationMissMs int64   `json:"avg_duration_miss_ms"`
	MinDurationMissMs int64   `json:"min_duration_miss_ms"`
	MaxDurationMissMs int64   `json:"max_duration_miss_ms"`
}

type metricsRequest struct {
	Metrics []APIMetricsPayload `json:"metrics"`
}

// Reporter is a background goroutine that periodically snapshots metrics and POSTs them.
type Reporter struct {
	collector     *Collector
	baseURL       string
	managementKey string
	interval      time.Duration
	enabled       bool
	httpClient    *http.Client
}

func NewReporter(collector *Collector, baseURL, managementKey string, intervalSeconds int, enabled bool) *Reporter {
	return &Reporter{
		collector:     collector,
		baseURL:       baseURL,
		managementKey: managementKey,
		interval:      time.Duration(intervalSeconds) * time.Second,
		enabled:       enabled,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Start launches the background reporting goroutine.
func (r *Reporter) Start(ctx context.Context) {
	if !r.enabled {
		return
	}
	go r.run(ctx)
}

func (r *Reporter) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.report(ctx)
		}
	}
}

func (r *Reporter) report(ctx context.Context) {
	snapshot := r.collector.SnapshotAndReset()
	for projectID, byAPI := range snapshot {
		if len(byAPI) == 0 {
			continue
		}
		payloads := make([]APIMetricsPayload, 0, len(byAPI))
		for api, agg := range byAPI {
			if agg.TotalCalls == 0 {
				continue
			}
			payloads = append(payloads, buildPayload(api, agg))
		}
		if len(payloads) == 0 {
			continue
		}
		r.post(ctx, projectID, payloads)
	}
}

func buildPayload(api APIName, agg *AggregatedMetrics) APIMetricsPayload {
	p := APIMetricsPayload{
		API:        string(api),
		HitCount:   agg.HitCount,
		MissCount:  agg.MissCount,
		TotalCalls: agg.TotalCalls,
	}

	if agg.TotalCalls > 0 {
		p.AvgResultSize = float64(agg.SumResultSize) / float64(agg.TotalCalls)
		totalDuration := agg.SumDurationHitsMs + agg.SumDurationMissesMs
		p.AvgDurationMs = totalDuration / agg.TotalCalls
	}

	if agg.HitCount > 0 {
		p.AvgHitCandidates = float64(agg.SumCandidates) / float64(agg.HitCount)
		p.AvgHitFiltered = float64(agg.SumFiltered) / float64(agg.HitCount)
		p.AvgDurationHitMs = agg.SumDurationHitsMs / agg.HitCount
		p.MinDurationHitMs = agg.MinDurationHitsMs
		p.MaxDurationHitMs = agg.MaxDurationHitsMs
	}

	if agg.MissCount > 0 {
		p.AvgDurationMissMs = agg.SumDurationMissesMs / agg.MissCount
		p.MinDurationMissMs = agg.MinDurationMissesMs
		p.MaxDurationMissMs = agg.MaxDurationMissesMs
	}

	// overall min/max derived from hit and miss
	minHit := agg.MinDurationHitsMs
	if agg.HitCount == 0 {
		minHit = math.MaxInt64
	}
	minMiss := agg.MinDurationMissesMs
	if agg.MissCount == 0 {
		minMiss = math.MaxInt64
	}
	p.MinDurationMs = min(minHit, minMiss)
	if p.MinDurationMs == math.MaxInt64 {
		p.MinDurationMs = 0
	}
	p.MaxDurationMs = max(agg.MaxDurationHitsMs, agg.MaxDurationMissesMs)

	return p
}

func (r *Reporter) post(ctx context.Context, projectID string, payloads []APIMetricsPayload) {
	body, err := utils.Marshal(metricsRequest{Metrics: payloads})
	if err != nil {
		cctx.Logger(ctx).Error().Err(err).Str("project_id", projectID).Msg("Failed to marshal FGA cache metrics")
		return
	}

	url := r.baseURL + "/v1/mgmt/fga/cache/metrics"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		cctx.Logger(ctx).Error().Err(err).Str("project_id", projectID).Msg("Failed to create FGA cache metrics request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+projectID+":"+r.managementKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		cctx.Logger(ctx).Error().Err(err).Str("project_id", projectID).Msg("Failed to post FGA cache metrics")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cctx.Logger(ctx).Error().Int("status_code", resp.StatusCode).Str("project_id", projectID).Msg("Unexpected status posting FGA cache metrics")
	}
}
