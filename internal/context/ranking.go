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
			// HITS-enhanced ranking: high authority nodes that are also seeds
			// get a boost. High authority nodes that are NOT seeds get a penalty
			// (they're generic infrastructure used by everything, not task-specific).
			//
			// The insight: in code graphs, high-authority non-seed nodes are
			// things like types.Hash, GraphStore, context.Context. They're called
			// by everything but rarely task-relevant. Seeds (directly matched
			// symbols) with high authority ARE important (heavily-used code you
			// matched on).
			h := hits[s.Node.NodeHash]
			isSeed := s.DistanceFromTarget == 0
			var authorityAdj float64
			if isSeed && h.Authority > 0.1 {
				authorityAdj = h.Authority * 0.10 // boost important seeds
			} else if !isSeed && h.Authority > 0.3 {
				authorityAdj = -h.Authority * 0.05 // penalize generic infrastructure
			}

			blastRadius = (float64(s.CallerCount) / float64(maxCallers)) * 0.40
			confidence = s.Confidence * 0.25
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

