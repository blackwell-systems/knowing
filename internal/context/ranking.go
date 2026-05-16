package context

import (
	"sort"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ScoringInput provides the raw data needed to compute a symbol's relevance score.
type ScoringInput struct {
	Node               types.Node
	CallerCount        int     // number of transitive callers (blast radius)
	Confidence         float64 // provenance tier confidence (0.0-1.0)
	LastObserved       int64   // unix timestamp of last runtime observation (0 = static only)
	DistanceFromTarget int     // hops from the task target symbol
}

// RankSymbols scores each symbol by a weighted formula incorporating blast radius,
// confidence, recency, and graph distance, then returns them sorted by score descending.
func RankSymbols(symbols []ScoringInput) []RankedSymbol {
	if len(symbols) == 0 {
		return nil
	}

	results := make([]RankedSymbol, 0, len(symbols))
	for _, s := range symbols {
		blastRadius := min(1.0, float64(s.CallerCount)/50.0) * 0.40
		confidence := s.Confidence * 0.25
		recency := recencyFromTimestamp(s.LastObserved) * 0.20
		distance := (1.0 / (1.0 + float64(s.DistanceFromTarget))) * 0.15

		total := blastRadius + confidence + recency + distance

		results = append(results, RankedSymbol{
			Node:  s.Node,
			Score: total,
			Components: ScoreComponents{
				BlastRadius: blastRadius,
				Confidence:  confidence,
				Recency:     recency,
				Distance:    distance,
			},
			Provenance: "",
			Distance:   s.DistanceFromTarget,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// recencyFromTimestamp converts a unix timestamp into a recency score (0.0 to 1.0).
// A zero timestamp (static-only edge) returns 0.0. More recent observations
// receive higher scores.
func recencyFromTimestamp(ts int64) float64 {
	if ts == 0 {
		return 0.0
	}

	now := time.Now().Unix()
	age := now - ts

	const (
		day  = int64(86400)
		week = 7 * day
		month = 30 * day
	)

	switch {
	case age <= day:
		return 1.0
	case age <= week:
		return 0.8
	case age <= month:
		return 0.5
	default:
		return 0.2
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
