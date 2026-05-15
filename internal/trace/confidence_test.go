package trace

import (
	"strings"
	"testing"
	"time"
)

func TestConfidenceFromCount(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  float64
	}{
		{"over 1000", 1500, 0.95},
		{"exactly 1001", 1001, 0.95},
		{"exactly 1000", 1000, 0.85},
		{"mid range 500", 500, 0.85},
		{"exactly 100", 100, 0.85},
		{"exactly 99", 99, 0.7},
		{"mid range 50", 50, 0.7},
		{"exactly 10", 10, 0.7},
		{"exactly 9", 9, 0.5},
		{"exactly 1", 1, 0.5},
		{"zero", 0, 0.2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConfidenceFromCount(tt.count)
			if got != tt.want {
				t.Errorf("ConfidenceFromCount(%d) = %v, want %v", tt.count, got, tt.want)
			}
		})
	}
}

func TestComputeConfidence(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		days     int
		want     float64
	}{
		{"active high volume", 1500, 3, 0.95},
		{"active medium volume", 500, 5, 0.85},
		{"active low volume", 50, 7, 0.7},
		{"active minimal", 2, 1, 0.5},
		{"active zero count", 0, 0, 0.2},
		{"recent boundary 30", 100, 30, 0.85},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeConfidence(tt.count, tt.days)
			if got != tt.want {
				t.Errorf("ComputeConfidence(%d, %d) = %v, want %v", tt.count, tt.days, got, tt.want)
			}
		})
	}
}

func TestComputeConfidence_Stale(t *testing.T) {
	tests := []struct {
		name  string
		count int
		days  int
	}{
		{"31 days", 1000, 31},
		{"60 days", 500, 60},
		{"90 days exactly", 100, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeConfidence(tt.count, tt.days)
			if got != 0.2 {
				t.Errorf("ComputeConfidence(%d, %d) = %v, want 0.2 (stale)", tt.count, tt.days, got)
			}
		})
	}
}

func TestComputeConfidence_GCEligible(t *testing.T) {
	tests := []struct {
		name  string
		count int
		days  int
	}{
		{"91 days", 1000, 91},
		{"180 days", 500, 180},
		{"365 days", 100, 365},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeConfidence(tt.count, tt.days)
			if got != 0.0 {
				t.Errorf("ComputeConfidence(%d, %d) = %v, want 0.0 (gc eligible)", tt.count, tt.days, got)
			}
		})
	}
}

func TestShouldGarbageCollect(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name         string
		lastObserved int64
		threshold    int
		want         bool
	}{
		{"just observed", now, 90, false},
		{"one day ago within threshold", now - 86400, 90, false},
		{"exactly at threshold", now - 90*86400, 90, false},
		{"one second past threshold", now - 90*86400 - 1, 90, true},
		{"way past threshold", now - 365*86400, 90, true},
		{"custom threshold 30 days within", now - 29*86400, 30, false},
		{"custom threshold 30 days past", now - 31*86400, 30, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldGarbageCollect(tt.lastObserved, tt.threshold)
			if got != tt.want {
				t.Errorf("ShouldGarbageCollect(%d, %d) = %v, want %v", tt.lastObserved, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestBuildProvenance(t *testing.T) {
	tests := []struct {
		name     string
		traceIDs []string
		check    func(t *testing.T, got string)
	}{
		{
			"empty",
			nil,
			func(t *testing.T, got string) {
				if got != "otel_trace" {
					t.Errorf("got %q, want %q", got, "otel_trace")
				}
			},
		},
		{
			"single",
			[]string{"abc123"},
			func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "otel_trace:") {
					t.Fatalf("missing prefix: %q", got)
				}
				if !strings.Contains(got, `"abc123"`) {
					t.Errorf("missing trace ID in %q", got)
				}
			},
		},
		{
			"three IDs",
			[]string{"a", "b", "c"},
			func(t *testing.T, got string) {
				if !strings.Contains(got, `"a"`) || !strings.Contains(got, `"b"`) || !strings.Contains(got, `"c"`) {
					t.Errorf("missing trace IDs in %q", got)
				}
			},
		},
		{
			"truncated to 5",
			[]string{"1", "2", "3", "4", "5", "6", "7"},
			func(t *testing.T, got string) {
				if strings.Contains(got, `"6"`) || strings.Contains(got, `"7"`) {
					t.Errorf("should truncate to 5 IDs: %q", got)
				}
				if !strings.Contains(got, `"5"`) {
					t.Errorf("should include 5th ID: %q", got)
				}
			},
		},
		{
			"exactly 5",
			[]string{"a", "b", "c", "d", "e"},
			func(t *testing.T, got string) {
				for _, id := range []string{"a", "b", "c", "d", "e"} {
					if !strings.Contains(got, `"`+id+`"`) {
						t.Errorf("missing ID %q in %q", id, got)
					}
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildProvenance(tt.traceIDs)
			tt.check(t, got)
		})
	}
}

func TestDecayBracket(t *testing.T) {
	tests := []struct {
		name string
		days int
		want string
	}{
		{"day 0", 0, "active"},
		{"day 7", 7, "active"},
		{"day 8", 8, "recent"},
		{"day 30", 30, "recent"},
		{"day 31", 31, "stale"},
		{"day 90", 90, "stale"},
		{"day 91", 91, "gc_eligible"},
		{"day 365", 365, "gc_eligible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecayBracket(tt.days)
			if got != tt.want {
				t.Errorf("DecayBracket(%d) = %q, want %q", tt.days, got, tt.want)
			}
		})
	}
}
