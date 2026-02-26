package metrics

import (
	"math"
	"sync"
)

type APIName string

const (
	APIWhoCanAccess        APIName = "WhoCanAccess"
	APIWhatCanTargetAccess APIName = "WhatCanTargetAccess"
)

// CallMetrics captures data from a single API call (recorded per-request).
type CallMetrics struct {
	CacheHit        bool
	CandidatesCount int
	FilteredCount   int
	ResultSize      int
	DurationMs      int64
}

// AggregatedMetrics accumulates data across many calls.
type AggregatedMetrics struct {
	HitCount            int64
	MissCount           int64
	TotalCalls          int64
	SumCandidates       int64
	SumFiltered         int64
	SumResultSize       int64
	SumDurationHitsMs   int64
	SumDurationMissesMs int64
	MinDurationHitsMs   int64
	MaxDurationHitsMs   int64
	MinDurationMissesMs int64
	MaxDurationMissesMs int64
}

func newAggregatedMetrics() *AggregatedMetrics {
	return &AggregatedMetrics{
		MinDurationHitsMs:   math.MaxInt64,
		MinDurationMissesMs: math.MaxInt64,
	}
}

type projectMetrics struct {
	mu    sync.Mutex
	byAPI map[APIName]*AggregatedMetrics
}

// Collector is a thread-safe per-project, per-API metrics accumulator.
type Collector struct {
	projects sync.Map // projectID -> *projectMetrics
}

func NewCollector() *Collector {
	return &Collector{}
}

// Record atomically accumulates metrics from a single call.
func (c *Collector) Record(projectID string, api APIName, m CallMetrics) {
	pm := c.getOrCreateProjectMetrics(projectID)
	pm.mu.Lock()
	defer pm.mu.Unlock()

	agg, ok := pm.byAPI[api]
	if !ok {
		agg = newAggregatedMetrics()
		pm.byAPI[api] = agg
	}

	agg.TotalCalls++
	agg.SumCandidates += int64(m.CandidatesCount)
	agg.SumFiltered += int64(m.FilteredCount)
	agg.SumResultSize += int64(m.ResultSize)

	if m.CacheHit {
		agg.HitCount++
		agg.SumDurationHitsMs += m.DurationMs
		if m.DurationMs < agg.MinDurationHitsMs {
			agg.MinDurationHitsMs = m.DurationMs
		}
		if m.DurationMs > agg.MaxDurationHitsMs {
			agg.MaxDurationHitsMs = m.DurationMs
		}
	} else {
		agg.MissCount++
		agg.SumDurationMissesMs += m.DurationMs
		if m.DurationMs < agg.MinDurationMissesMs {
			agg.MinDurationMissesMs = m.DurationMs
		}
		if m.DurationMs > agg.MaxDurationMissesMs {
			agg.MaxDurationMissesMs = m.DurationMs
		}
	}
}

// SnapshotAndReset copies all accumulated data and resets to zero.
// Returns map[projectID]map[APIName]*AggregatedMetrics.
func (c *Collector) SnapshotAndReset() map[string]map[APIName]*AggregatedMetrics {
	snapshot := make(map[string]map[APIName]*AggregatedMetrics)
	c.projects.Range(func(key, value any) bool {
		projectID := key.(string)
		pm := value.(*projectMetrics)
		pm.mu.Lock()
		defer pm.mu.Unlock()

		if len(pm.byAPI) == 0 {
			return true
		}

		projectSnapshot := make(map[APIName]*AggregatedMetrics, len(pm.byAPI))
		for api, agg := range pm.byAPI {
			// deep copy
			copy := *agg
			projectSnapshot[api] = &copy
		}
		snapshot[projectID] = projectSnapshot

		// reset
		pm.byAPI = make(map[APIName]*AggregatedMetrics)
		return true
	})
	return snapshot
}

func (c *Collector) getOrCreateProjectMetrics(projectID string) *projectMetrics {
	if pm, ok := c.projects.Load(projectID); ok {
		return pm.(*projectMetrics)
	}
	pm := &projectMetrics{byAPI: make(map[APIName]*AggregatedMetrics)}
	actual, _ := c.projects.LoadOrStore(projectID, pm)
	return actual.(*projectMetrics)
}
