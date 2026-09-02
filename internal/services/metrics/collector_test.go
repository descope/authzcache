package metrics

import (
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordAndSnapshot(t *testing.T) {
	c := NewCollector()

	// record for proj1 / WhoCanAccess
	c.Record("proj1", APIWhoCanAccess, CallMetrics{CacheHit: true, CandidatesCount: 10, FilteredCount: 2, ResultSize: 8, DurationMs: 5})
	c.Record("proj1", APIWhoCanAccess, CallMetrics{CacheHit: false, CandidatesCount: 0, FilteredCount: 0, ResultSize: 3, DurationMs: 20})
	// record for proj1 / WhatCanTargetAccess
	c.Record("proj1", APIWhatCanTargetAccess, CallMetrics{CacheHit: true, CandidatesCount: 4, FilteredCount: 1, ResultSize: 3, DurationMs: 3})
	// record for proj2 / WhoCanAccess
	c.Record("proj2", APIWhoCanAccess, CallMetrics{CacheHit: false, CandidatesCount: 0, FilteredCount: 0, ResultSize: 7, DurationMs: 30})

	snapshot := c.SnapshotAndReset()

	// proj1 / WhoCanAccess
	require.Contains(t, snapshot, "proj1")
	wca := snapshot["proj1"][APIWhoCanAccess]
	require.NotNil(t, wca)
	require.Equal(t, int64(2), wca.TotalCalls)
	require.Equal(t, int64(1), wca.HitCount)
	require.Equal(t, int64(1), wca.MissCount)
	require.Equal(t, int64(10), wca.SumCandidates)
	require.Equal(t, int64(2), wca.SumFiltered)
	require.Equal(t, int64(11), wca.SumResultSize)
	require.Equal(t, int64(5), wca.SumDurationHitsMs)
	require.Equal(t, int64(20), wca.SumDurationMissesMs)
	require.Equal(t, int64(5), wca.MinDurationHitsMs)
	require.Equal(t, int64(5), wca.MaxDurationHitsMs)
	require.Equal(t, int64(20), wca.MinDurationMissesMs)
	require.Equal(t, int64(20), wca.MaxDurationMissesMs)

	// proj1 / WhatCanTargetAccess
	wcta := snapshot["proj1"][APIWhatCanTargetAccess]
	require.NotNil(t, wcta)
	require.Equal(t, int64(1), wcta.TotalCalls)
	require.Equal(t, int64(1), wcta.HitCount)
	require.Equal(t, int64(0), wcta.MissCount)

	// proj2 / WhoCanAccess
	require.Contains(t, snapshot, "proj2")
	p2 := snapshot["proj2"][APIWhoCanAccess]
	require.NotNil(t, p2)
	require.Equal(t, int64(1), p2.TotalCalls)
	require.Equal(t, int64(0), p2.HitCount)
	require.Equal(t, int64(1), p2.MissCount)
	require.Equal(t, int64(30), p2.SumDurationMissesMs)
	require.Equal(t, int64(math.MaxInt64), p2.MinDurationHitsMs) // no hits, stays at MaxInt64

	// after snapshot, data should be reset
	snapshot2 := c.SnapshotAndReset()
	require.Empty(t, snapshot2)
}

func TestConcurrentRecord(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup
	goroutines := 50
	callsPerGoroutine := 100
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			projectID := "proj1"
			if i%2 == 0 {
				projectID = "proj2"
			}
			for range callsPerGoroutine {
				c.Record(projectID, APIWhoCanAccess, CallMetrics{CacheHit: true, DurationMs: 1})
			}
		}(i)
	}
	wg.Wait()

	snapshot := c.SnapshotAndReset()
	totalCalls := int64(0)
	for _, byAPI := range snapshot {
		if agg, ok := byAPI[APIWhoCanAccess]; ok {
			totalCalls += agg.TotalCalls
		}
	}
	require.Equal(t, int64(goroutines*callsPerGoroutine), totalCalls)
}
