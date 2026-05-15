package trace

import (
	"encoding/json"
	"fmt"
	"time"
)

// ComputeConfidence combines observation volume and recency to produce a
// confidence score for a runtime edge.
func ComputeConfidence(observationCount int, daysSinceLastObserved int) float64 {
	if daysSinceLastObserved > 90 {
		return 0.0
	}
	if daysSinceLastObserved > 30 {
		return 0.2
	}
	return ConfidenceFromCount(observationCount)
}

// ShouldGarbageCollect returns true if the edge has not been observed within
// the given threshold (in days).
func ShouldGarbageCollect(lastObserved int64, gcThresholdDays int) bool {
	return time.Now().Unix()-lastObserved > int64(gcThresholdDays)*86400
}

// DecayBracket returns a human-readable decay bracket for diagnostics.
func DecayBracket(daysSinceLastObserved int) string {
	switch {
	case daysSinceLastObserved <= 7:
		return "active"
	case daysSinceLastObserved <= 30:
		return "recent"
	case daysSinceLastObserved <= 90:
		return "stale"
	default:
		return "gc_eligible"
	}
}

// BuildProvenance returns the provenance string for runtime edges.
// Format: otel_trace:{"trace_ids":["id1","id2"]} (max 5 trace IDs).
// If traceIDs is empty, returns "otel_trace".
func BuildProvenance(traceIDs []string) string {
	if len(traceIDs) == 0 {
		return "otel_trace"
	}
	ids := traceIDs
	if len(ids) > 5 {
		ids = ids[:5]
	}
	payload, err := json.Marshal(map[string][]string{"trace_ids": ids})
	if err != nil {
		return "otel_trace"
	}
	return fmt.Sprintf("otel_trace:%s", payload)
}
