package context

import (
	stdctx "context"

	"github.com/blackwell-systems/knowing/internal/types"
)

// RandomWalkWithRestart computes relevance scores for all nodes reachable from
// the seed set by simulating random walks that restart at seed nodes with
// probability alpha. The stationary distribution assigns higher scores to nodes
// that are structurally close to the seeds and highly connected.
//
// Parameters:
//   - seeds: initial nodes to start walks from (uniform weight)
//   - alpha: restart probability (0.2 means 20% chance of returning to a seed each step)
//   - maxIter: maximum iterations (20 is typical for convergence)
//   - store: graph store for edge lookups
//
// Returns a map from node hash to relevance score (0.0 to 1.0, normalized).
func RandomWalkWithRestart(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash, alpha float64, maxIter int) (map[types.Hash]float64, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.2
	}
	if maxIter <= 0 {
		maxIter = 20
	}

	// Initialize: uniform probability across seeds.
	seedWeight := 1.0 / float64(len(seeds))
	seedVec := make(map[types.Hash]float64, len(seeds))
	for _, s := range seeds {
		seedVec[s] = seedWeight
	}

	// Current probability distribution starts at seeds.
	prob := make(map[types.Hash]float64)
	for k, v := range seedVec {
		prob[k] = v
	}

	// Edge weight multipliers by type.
	edgeWeight := map[string]float64{
		"calls":         1.0,
		"implements":    0.8,
		"handles_route": 0.7,
		"imports":       0.5,
		"references":    0.4,
	}

	// Iterate: at each step, walk along edges with (1-alpha) probability,
	// or restart at seeds with alpha probability.
	for iter := 0; iter < maxIter; iter++ {
		next := make(map[types.Hash]float64)

		// Restart component: alpha * seed_vector.
		for s, w := range seedVec {
			next[s] += alpha * w
		}

		// Walk component: (1-alpha) * transition from current distribution.
		for node, nodeProb := range prob {
			if nodeProb < 0.0001 {
				continue // skip negligible nodes
			}

			// Get outgoing edges (calls, implements, etc.).
			edges, err := store.EdgesFrom(ctx, node, "")
			if err != nil {
				return nil, err
			}

			// Also get incoming edges (who calls this node).
			incoming, err := store.EdgesTo(ctx, node, "")
			if err != nil {
				return nil, err
			}
			edges = append(edges, incoming...)

			if len(edges) == 0 {
				// Dead end: redistribute to seeds (effectively a restart).
				for s, w := range seedVec {
					next[s] += (1 - alpha) * nodeProb * w
				}
				continue
			}

			// Compute total edge weight for normalization.
			totalWeight := 0.0
			for _, e := range edges {
				w := edgeWeight[e.EdgeType]
				if w == 0 {
					w = 0.3 // default for unknown edge types
				}
				totalWeight += w
			}

			// Distribute probability along edges proportional to weight.
			for _, e := range edges {
				w := edgeWeight[e.EdgeType]
				if w == 0 {
					w = 0.3
				}
				// Target is the other end of the edge.
				target := e.TargetHash
				if target == node {
					target = e.SourceHash
				}
				next[target] += (1 - alpha) * nodeProb * (w / totalWeight)
			}
		}

		// Check convergence: sum of absolute differences.
		delta := 0.0
		for k, v := range next {
			delta += abs(v - prob[k])
		}
		for k, v := range prob {
			if _, exists := next[k]; !exists {
				delta += v
			}
		}

		prob = next

		if delta < 0.001 {
			break
		}
	}

	// Normalize to [0, 1] range relative to max.
	maxScore := 0.0
	for _, v := range prob {
		if v > maxScore {
			maxScore = v
		}
	}
	if maxScore > 0 {
		for k := range prob {
			prob[k] /= maxScore
		}
	}

	return prob, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
