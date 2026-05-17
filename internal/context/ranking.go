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
//
// If HITS scores are provided (non-nil map), authority scores are factored into the
// ranking, promoting structurally important nodes (heavily called) over leaf functions.
func RankSymbols(symbols []ScoringInput, hitsScores ...map[types.Hash]HITSScores) []RankedSymbol {
	if len(symbols) == 0 {
		return nil
	}

	// Extract HITS scores if provided.
	var hits map[types.Hash]HITSScores
	if len(hitsScores) > 0 && hitsScores[0] != nil {
		hits = hitsScores[0]
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
		var blastRadius, confidence, recency, distance, total float64

		if hits != nil {
			// HITS-enhanced ranking: authority and hub scores reshape the results.
			//
			// Seeds with high authority are the most valuable: they're both
			// keyword-relevant AND structurally central (many callers). Boost them.
			//
			// Non-seeds with high authority are generic infrastructure (types.Hash,
			// GraphStore, context.Context): called by everything but rarely
			// task-relevant. Penalize them to push them below task-specific symbols.
			//
			// Hubs (nodes that call many things) are useful when they're seeds
			// (entry points you matched on) but noisy otherwise.
			h := hits[s.Node.NodeHash]
			isSeed := s.DistanceFromTarget == 0
			var authorityAdj float64
			if isSeed && h.Authority > 0.05 {
				authorityAdj = h.Authority * 0.25 // strong boost for task-relevant authorities
			} else if !isSeed && h.Authority > 0.2 {
				authorityAdj = -h.Authority * 0.15 // meaningful penalty for generic infrastructure
			}
			// Hub bonus for seed entry points (orchestrators, handlers).
			if isSeed && h.Hub > 0.1 {
				authorityAdj += h.Hub * 0.10
			}

			blastRadius = (float64(s.CallerCount) / float64(maxCallers)) * 0.35
			confidence = s.Confidence * 0.20
			recency = recencyFromTimestamp(s.LastObserved) * 0.15
			distance = (1.0 / (1.0 + float64(s.DistanceFromTarget))) * 0.15
			total = blastRadius + confidence + recency + distance + authorityAdj
		} else {
			// Original ranking (no HITS): blast radius is the primary signal.
			blastRadius = (float64(s.CallerCount) / float64(maxCallers)) * 0.40
			confidence = s.Confidence * 0.25
			recency = recencyFromTimestamp(s.LastObserved) * 0.20
			distance = (1.0 / (1.0 + float64(s.DistanceFromTarget))) * 0.15
			total = blastRadius + confidence + recency + distance
		}

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

