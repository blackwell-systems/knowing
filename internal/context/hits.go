package context

import (
	stdctx "context"

	"github.com/blackwell-systems/knowing/internal/types"
)

// HITSScores holds the authority and hub scores for a node.
type HITSScores struct {
	Authority float64
	Hub       float64
}

// ComputeHITS runs the HITS (Hyperlink-Induced Topic Search) algorithm on a
// subgraph defined by the given node hashes. It computes authority scores
// (nodes that are heavily pointed to) and hub scores (nodes that point to
// many authorities).
//
// In the context of code graphs:
//   - Authority = heavily called functions, core types, key interfaces
//   - Hub = orchestrators, entry points, functions that wire things together
//
// Parameters:
//   - nodes: the subgraph to analyze (typically top-200 RWR results)
//   - store: graph store for edge lookups
//   - maxIter: iterations (5-10 is typical for convergence)
//
// Returns a map from node hash to HITS scores.
func ComputeHITS(ctx stdctx.Context, store types.GraphStore, nodes []types.Hash, maxIter int) (map[types.Hash]HITSScores, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	if maxIter <= 0 {
		maxIter = 10
	}

	// Build the subgraph adjacency: only edges between nodes in our set.
	nodeSet := make(map[types.Hash]bool, len(nodes))
	for _, h := range nodes {
		nodeSet[h] = true
	}

	// outLinks[A] = list of nodes that A points to (A calls B, A imports C)
	// inLinks[A] = list of nodes that point to A (B calls A)
	outLinks := make(map[types.Hash][]types.Hash, len(nodes))
	inLinks := make(map[types.Hash][]types.Hash, len(nodes))

	for _, node := range nodes {
		edges, err := store.EdgesFrom(ctx, node, "")
		if err != nil {
			continue
		}
		for _, e := range edges {
			if nodeSet[e.TargetHash] {
				outLinks[node] = append(outLinks[node], e.TargetHash)
				inLinks[e.TargetHash] = append(inLinks[e.TargetHash], node)
			}
		}
	}

	// Initialize: all scores = 1.0.
	auth := make(map[types.Hash]float64, len(nodes))
	hub := make(map[types.Hash]float64, len(nodes))
	for _, h := range nodes {
		auth[h] = 1.0
		hub[h] = 1.0
	}

	// Iterate.
	for iter := 0; iter < maxIter; iter++ {
		// Update authority: auth(A) = sum of hub scores of nodes pointing to A.
		newAuth := make(map[types.Hash]float64, len(nodes))
		for _, node := range nodes {
			sum := 0.0
			for _, src := range inLinks[node] {
				sum += hub[src]
			}
			newAuth[node] = sum
		}

		// Update hub: hub(A) = sum of authority scores of nodes A points to.
		newHub := make(map[types.Hash]float64, len(nodes))
		for _, node := range nodes {
			sum := 0.0
			for _, tgt := range outLinks[node] {
				sum += newAuth[tgt] // use updated authority
			}
			newHub[node] = sum
		}

		// Normalize.
		authNorm := 0.0
		hubNorm := 0.0
		for _, v := range newAuth {
			authNorm += v * v
		}
		for _, v := range newHub {
			hubNorm += v * v
		}

		if authNorm > 0 {
			authNorm = sqrt(authNorm)
			for k := range newAuth {
				newAuth[k] /= authNorm
			}
		}
		if hubNorm > 0 {
			hubNorm = sqrt(hubNorm)
			for k := range newHub {
				newHub[k] /= hubNorm
			}
		}

		auth = newAuth
		hub = newHub
	}

	// Build result.
	result := make(map[types.Hash]HITSScores, len(nodes))
	for _, h := range nodes {
		result[h] = HITSScores{
			Authority: auth[h],
			Hub:       hub[h],
		}
	}

	return result, nil
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method.
	z := x
	for i := 0; i < 20; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}
