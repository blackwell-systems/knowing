package context

import (
	"os"
	"strconv"
	"sync"
)

// SweepParams holds tunable parameters for the retrieval pipeline.
// Used by the parameter sweep benchmark to test different configurations.
// Zero values mean "use default".
type SweepParams struct {
	Alpha       float64 // RWR restart probability (default 0.2)
	MaxIter     int     // RWR iterations (default 20)
	ScoreCutoff float64 // min RWR score threshold (default 0.02)
	MaxSeeds    int     // max RWR seeds (default 15)
	RRFk        float64 // RRF constant (default 60)
	BlastW      float64 // blast radius ranking weight
	ConfW       float64 // confidence ranking weight
	RecencyW    float64 // recency ranking weight
	DistanceW   float64 // distance ranking weight
	TestPenalty float64 // test file penalty multiplier (default 0.3, -1 means use default)
}

var (
	sweepMu     sync.RWMutex
	sweepParams SweepParams
)

// SetSweepParams sets the global sweep parameters for the retrieval pipeline.
// Pass a zero-value struct to reset to defaults.
func SetSweepParams(p SweepParams) {
	sweepMu.Lock()
	defer sweepMu.Unlock()
	sweepParams = p
}

// getSweepParams returns the current sweep parameters.
func getSweepParams() SweepParams {
	sweepMu.RLock()
	defer sweepMu.RUnlock()
	return sweepParams
}

// Helpers to get parameter with fallback to default.
func sweepAlpha() float64 {
	p := getSweepParams()
	if p.Alpha > 0 {
		return p.Alpha
	}
	return 0.2
}

func sweepMaxIter() int {
	p := getSweepParams()
	if p.MaxIter > 0 {
		return p.MaxIter
	}
	return 20
}

func sweepScoreCutoff() float64 {
	p := getSweepParams()
	if p.ScoreCutoff > 0 {
		return p.ScoreCutoff
	}
	return 0.02
}

func sweepMaxSeeds() int {
	p := getSweepParams()
	if p.MaxSeeds > 0 {
		return p.MaxSeeds
	}
	return 15
}

func sweepRRFk() float64 {
	p := getSweepParams()
	if p.RRFk > 0 {
		return p.RRFk
	}
	return 60
}

func sweepBlastW() float64 {
	p := getSweepParams()
	if p.BlastW > 0 {
		return p.BlastW
	}
	return 0.35
}

func sweepConfW() float64 {
	p := getSweepParams()
	if p.ConfW > 0 {
		return p.ConfW
	}
	return 0.20
}

func sweepRecencyW() float64 {
	p := getSweepParams()
	if p.RecencyW > 0 {
		return p.RecencyW
	}
	return 0.15
}

func sweepDistanceW() float64 {
	p := getSweepParams()
	if p.DistanceW > 0 {
		return p.DistanceW
	}
	return 0.15
}

func sweepTestPenalty() float64 {
	p := getSweepParams()
	if p.TestPenalty < 0 {
		return 0.15 // -1 sentinel means use default
	}
	if p.TestPenalty > 0 {
		return p.TestPenalty
	}
	if v := os.Getenv("BENCH_TEST_PENALTY"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 0.15
}

// proximityExponent returns the exponent for RWR proximity packing.
// Default 0.3 (cube root). Validated by 9-point sweep on 308 tasks:
// 0.3 = 0.282 P@10 (peak), 11/15 repos improved vs 0.5. Enriched
// repos benefit most (cargo +0.026, rails +0.025, vscode +0.015).
// Override with BENCH_PROXIMITY_EXP for sweep.
func proximityExponent() float64 {
	if v := os.Getenv("BENCH_PROXIMITY_EXP"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 0.3
}

// lspEdgeWeight returns the weight multiplier for LSP-enriched edges.
// Default 1.0 (no attenuation). Lower values reduce the influence of
// LSP-discovered edges in RWR, preventing enrichment from inflating
// centrality of framework wiring symbols above implementation symbols.
// Override with BENCH_LSP_EDGE_WEIGHT for sweep testing.
func lspEdgeWeight() float64 {
	if v := os.Getenv("BENCH_LSP_EDGE_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			return f
		}
	}
	return 0.3
}

// adaptiveProximityExponent adjusts the proximity exponent based on the
// phantom-to-real node ratio in the packing candidates. Higher phantom ratios
// need more aggressive proximity preference to prevent phantoms from filling
// budget slots.
//
// Phantom ratio 0 (no phantoms): use default 0.3
// Phantom ratio 1 (equal phantoms): use 0.4
// Phantom ratio 2+ (extreme, e.g. saleor): use 0.5-0.7
//
// Formula: clamp(0.3 + 0.2 * min(phantomRatio, 2.0), 0.3, 0.7)
// BENCH_PROXIMITY_EXP overrides this completely when set.
func adaptiveProximityExponent(phantomRatio float64) float64 {
	if v := os.Getenv("BENCH_PROXIMITY_EXP"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	exp := 0.3 + 0.2*phantomRatio
	if exp > 0.7 {
		exp = 0.7
	}
	return exp
}

// PackStrategy controls the packing algorithm used by packIntoBudget.
// "density" (default): density-ranked with RWR proximity weighting.
// "file-grouped": group symbols by file, pack densest files first.
// "top-k": take highest-scored symbols until budget exhausted.
// Override with BENCH_PACK_STRATEGY env var.
var PackStrategy string

func packStrategy() string {
	if PackStrategy != "" {
		return PackStrategy
	}
	if v := os.Getenv("BENCH_PACK_STRATEGY"); v != "" {
		return v
	}
	return "density"
}
