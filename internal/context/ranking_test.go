package context

import (
	"math"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestRankSymbols_Empty(t *testing.T) {
	result := RankSymbols(nil)
	if result != nil {
		t.Errorf("RankSymbols(nil) = %v, want nil", result)
	}

	result = RankSymbols([]ScoringInput{})
	if result != nil {
		t.Errorf("RankSymbols([]) = %v, want nil", result)
	}
}

func TestRankSymbols_SingleSymbol(t *testing.T) {
	now := time.Now().Unix()
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.Foo", Kind: "function"},
			CallerCount:        10,
			Confidence:         0.9,
			LastObserved:       now,
			DistanceFromTarget: 0,
		},
	}

	result := RankSymbols(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	r := result[0]
	// blast_radius: (10/10) * 0.40 = 0.40 (single element, max=self)
	// confidence: 0.9 * 0.25 = 0.225
	// recency: 1.0 * 0.20 = 0.20 (within 1 day)
	// distance: (1/(1+0)) * 0.15 = 0.15
	// total: 0.40 + 0.225 + 0.20 + 0.15 = 0.975
	wantScore := 0.975
	if math.Abs(r.Score-wantScore) > 0.001 {
		t.Errorf("Score = %f, want %f", r.Score, wantScore)
	}
}

func TestRankSymbols_BlastRadiusWeight(t *testing.T) {
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.High"},
			CallerCount:        50,
			Confidence:         0.5,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
		{
			Node:               types.Node{QualifiedName: "pkg.Low"},
			CallerCount:        1,
			Confidence:         0.5,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
	}

	result := RankSymbols(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// High: blast_radius = min(1.0, 50/50) * 0.40 = 0.40
	// Low: blast_radius = min(1.0, 1/50) * 0.40 = 0.02 * 0.40 = 0.008
	highBR := result[0].Components.BlastRadius
	lowBR := result[1].Components.BlastRadius

	wantHighBR := 0.40
	wantLowBR := 0.008

	if math.Abs(highBR-wantHighBR) > 0.001 {
		t.Errorf("High BlastRadius = %f, want %f", highBR, wantHighBR)
	}
	if math.Abs(lowBR-wantLowBR) > 0.001 {
		t.Errorf("Low BlastRadius = %f, want %f", lowBR, wantLowBR)
	}
}

func TestRankSymbols_ConfidenceWeight(t *testing.T) {
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.HighConf"},
			CallerCount:        0,
			Confidence:         1.0,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
		{
			Node:               types.Node{QualifiedName: "pkg.MidConf"},
			CallerCount:        0,
			Confidence:         0.7,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
	}

	result := RankSymbols(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// HighConf: confidence = 1.0 * 0.25 = 0.25
	// MidConf: confidence = 0.7 * 0.25 = 0.175
	for _, r := range result {
		if r.Node.QualifiedName == "pkg.HighConf" {
			if math.Abs(r.Components.Confidence-0.25) > 0.001 {
				t.Errorf("HighConf Confidence = %f, want 0.25", r.Components.Confidence)
			}
		}
		if r.Node.QualifiedName == "pkg.MidConf" {
			if math.Abs(r.Components.Confidence-0.175) > 0.001 {
				t.Errorf("MidConf Confidence = %f, want 0.175", r.Components.Confidence)
			}
		}
	}
}

func TestRankSymbols_RecencyWeight(t *testing.T) {
	now := time.Now().Unix()
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.Recent"},
			CallerCount:        0,
			Confidence:         0.5,
			LastObserved:       now,
			DistanceFromTarget: 0,
		},
		{
			Node:               types.Node{QualifiedName: "pkg.Never"},
			CallerCount:        0,
			Confidence:         0.5,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
	}

	result := RankSymbols(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Recent: recency = 1.0 * 0.20 = 0.20
	// Never: recency = 0.0 * 0.20 = 0.0
	for _, r := range result {
		if r.Node.QualifiedName == "pkg.Recent" {
			if math.Abs(r.Components.Recency-0.20) > 0.001 {
				t.Errorf("Recent Recency = %f, want 0.20", r.Components.Recency)
			}
		}
		if r.Node.QualifiedName == "pkg.Never" {
			// Static-only edges get base recency of 0.3 * 0.20 = 0.06
			if math.Abs(r.Components.Recency-0.06) > 0.001 {
				t.Errorf("Never Recency = %f, want 0.06", r.Components.Recency)
			}
		}
	}
}

func TestRankSymbols_DistanceWeight(t *testing.T) {
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.Direct"},
			CallerCount:        0,
			Confidence:         0.5,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
		{
			Node:               types.Node{QualifiedName: "pkg.Far"},
			CallerCount:        0,
			Confidence:         0.5,
			LastObserved:       0,
			DistanceFromTarget: 3,
		},
	}

	result := RankSymbols(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// Direct: distance = (1/(1+0)) * 0.15 = 0.15
	// Far: distance = (1/(1+3)) * 0.15 = 0.25 * 0.15 = 0.0375
	for _, r := range result {
		if r.Node.QualifiedName == "pkg.Direct" {
			if math.Abs(r.Components.Distance-0.15) > 0.001 {
				t.Errorf("Direct Distance = %f, want 0.15", r.Components.Distance)
			}
		}
		if r.Node.QualifiedName == "pkg.Far" {
			if math.Abs(r.Components.Distance-0.0375) > 0.001 {
				t.Errorf("Far Distance = %f, want 0.0375", r.Components.Distance)
			}
		}
	}
}

func TestRankSymbols_Ordering(t *testing.T) {
	now := time.Now().Unix()
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.Low"},
			CallerCount:        1,
			Confidence:         0.3,
			LastObserved:       0,
			DistanceFromTarget: 3,
		},
		{
			Node:               types.Node{QualifiedName: "pkg.High"},
			CallerCount:        50,
			Confidence:         1.0,
			LastObserved:       now,
			DistanceFromTarget: 0,
		},
		{
			Node:               types.Node{QualifiedName: "pkg.Mid"},
			CallerCount:        25,
			Confidence:         0.7,
			LastObserved:       now - 86400*5, // 5 days ago
			DistanceFromTarget: 1,
		},
	}

	result := RankSymbols(input)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// Results should be sorted by score descending.
	for i := 0; i < len(result)-1; i++ {
		if result[i].Score < result[i+1].Score {
			t.Errorf("results not sorted: result[%d].Score=%f < result[%d].Score=%f",
				i, result[i].Score, i+1, result[i+1].Score)
		}
	}

	// High should be first.
	if result[0].Node.QualifiedName != "pkg.High" {
		t.Errorf("expected first result to be pkg.High, got %s", result[0].Node.QualifiedName)
	}
}

func TestRankSymbols_ComponentBreakdown(t *testing.T) {
	now := time.Now().Unix()
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.Test"},
			CallerCount:        25,
			Confidence:         0.8,
			LastObserved:       now - 86400*3, // 3 days ago (within a week)
			DistanceFromTarget: 1,
		},
	}

	result := RankSymbols(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	r := result[0]
	// blast_radius: (25/25) * 0.40 = 1.0 * 0.40 = 0.40 (single element, max=self)
	// confidence: 0.8 * 0.25 = 0.20
	// recency: 0.8 * 0.20 = 0.16 (within 7 days)
	// distance: (1/(1+1)) * 0.15 = 0.5 * 0.15 = 0.075
	wantBR := 0.40
	wantConf := 0.20
	wantRec := 0.16
	wantDist := 0.075
	wantTotal := wantBR + wantConf + wantRec + wantDist

	if math.Abs(r.Components.BlastRadius-wantBR) > 0.001 {
		t.Errorf("BlastRadius = %f, want %f", r.Components.BlastRadius, wantBR)
	}
	if math.Abs(r.Components.Confidence-wantConf) > 0.001 {
		t.Errorf("Confidence = %f, want %f", r.Components.Confidence, wantConf)
	}
	if math.Abs(r.Components.Recency-wantRec) > 0.001 {
		t.Errorf("Recency = %f, want %f", r.Components.Recency, wantRec)
	}
	if math.Abs(r.Components.Distance-wantDist) > 0.001 {
		t.Errorf("Distance = %f, want %f", r.Components.Distance, wantDist)
	}
	if math.Abs(r.Score-wantTotal) > 0.001 {
		t.Errorf("Score = %f, want %f", r.Score, wantTotal)
	}
}

func TestRankSymbols_MaxBlastRadius(t *testing.T) {
	input := []ScoringInput{
		{
			Node:               types.Node{QualifiedName: "pkg.Popular"},
			CallerCount:        100, // > 50, should be capped
			Confidence:         0.5,
			LastObserved:       0,
			DistanceFromTarget: 0,
		},
	}

	result := RankSymbols(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// CallerCount=100: min(1.0, 100/50) = min(1.0, 2.0) = 1.0
	// blast_radius = 1.0 * 0.40 = 0.40
	wantBR := 0.40
	if math.Abs(result[0].Components.BlastRadius-wantBR) > 0.001 {
		t.Errorf("BlastRadius = %f, want %f (should be capped at 0.40)", result[0].Components.BlastRadius, wantBR)
	}
}
