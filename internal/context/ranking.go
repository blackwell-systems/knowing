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
// Blast radius is normalized relative to the max in the input set, ensuring the full
// 0.0-1.0 range is used regardless of codebase size.
func RankSymbols(symbols []ScoringInput) []RankedSymbol {
	if len(symbols) == 0 {
		return nil
	}

	// Find max caller count for relative normalization.
	maxCallers := 1
	for _, s := range symbols {
		if s.CallerCount > maxCallers {
			maxCallers = s.CallerCount
		}
	}

	results := make([]RankedSymbol, 0, len(symbols))
	for _, s := range symbols {
		// Blast radius: normalize relative to max in set (not hardcoded).
		// A symbol with the most callers gets the full 0.40 weight.
		blastRadius := (float64(s.CallerCount) / float64(maxCallers)) * 0.40

		// Confidence: direct from provenance tier.
		confidence := s.Confidence * 0.25

		// Recency: for static-only edges (no runtime observations), use a base
		// score of 0.3 instead of 0.0. This prevents 20% of the score from being
		// zero for all symbols in codebases without runtime data.
		recency := recencyFromTimestamp(s.LastObserved) * 0.20

		// Distance: inverse of hops from target.
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
// A zero timestamp (static-only edge) returns 0.3 as a base score, so that codebases
// without runtime data still get some recency contribution rather than losing 20% of
// the total score to zeros. More recent runtime observations receive higher scores.
func recencyFromTimestamp(ts int64) float64 {
	if ts == 0 {
		return 0.3 // base score for static-only symbols
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

