package context

import (
	stdctx "context"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// RWRMaxDepth controls BFS expansion depth from seeds when building the adjacency map.
// RWRMaxNodes caps total nodes explored during BFS. Together these prevent the walk
// from loading the entire graph on large repos. Tunable via sweep test.
var RWRMaxDepth = 3
var RWRMaxNodes = 1000

// externalHashLoader is a local interface for stores that support querying nodes by name.
type externalHashLoader interface {
	NodesByName(ctx stdctx.Context, pattern string) ([]types.Node, error)
}

// loadExternalHashes queries the store for all phantom external nodes and returns
// their hashes as a set. These nodes are dead-end targets from LSP enrichment
// (kind="external" or qualified_name starting with "external://") that should be
// excluded from RWR walk expansion to prevent probability diffusion.
func loadExternalHashes(ctx stdctx.Context, store types.GraphStore) map[types.Hash]bool {
	result := make(map[types.Hash]bool)

	loader, ok := store.(externalHashLoader)
	if !ok {
		return result
	}

	// Query for nodes with "external" in their qualified name.
	// This catches both kind="external" nodes and "external://" prefixed names.
	nodes, err := loader.NodesByName(ctx, "%external%")
	if err != nil {
		return result
	}

	for _, n := range nodes {
		if n.Kind == "external" || strings.HasPrefix(n.QualifiedName, "external://") {
			result[n.NodeHash] = true
		}
	}

	return result
}

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
// RandomWalkWithRestart runs RWR with uniform seed weights.
// For weighted seeds (prioritizing specific keywords), use RandomWalkWithRestartWeighted.
func RandomWalkWithRestart(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash, alpha float64, maxIter int) (map[types.Hash]float64, error) {
	// Uniform weights: all seeds contribute equally to restart probability.
	weights := make(map[types.Hash]float64, len(seeds))
	w := 1.0 / float64(len(seeds))
	for _, s := range seeds {
		weights[s] = w
	}
	return RandomWalkWithRestartWeighted(ctx, store, seeds, weights, alpha, maxIter)
}

// RandomWalkWithRestartWeighted runs RWR with per-seed restart weights.
// Seeds with higher weights receive more probability mass on restart, causing
// the walk to spend more time in their neighborhood. This differentiates
// specific seeds (high weight) from generic ones (low weight).
//
// Weights are normalized to sum to 1.0 internally.
func RandomWalkWithRestartWeighted(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash, seedWeights map[types.Hash]float64, alpha float64, maxIter int) (map[types.Hash]float64, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.2
	}
	if maxIter <= 0 {
		maxIter = 20
	}

	// Pre-load edges for the reachable subgraph (seeds + 2-hop neighbors)
	// into an in-memory adjacency map. This avoids per-node DB queries during
	// the iteration loop.
	adjFrom, adjTo, err := buildAdjacencyMap(ctx, store, seeds)
	if err != nil {
		return nil, err
	}

	// Normalize seed weights to sum to 1.0.
	var totalWeight float64
	for _, s := range seeds {
		totalWeight += seedWeights[s]
	}
	if totalWeight <= 0 {
		totalWeight = float64(len(seeds))
	}
	seedVec := make(map[types.Hash]float64, len(seeds))
	for _, s := range seeds {
		w := seedWeights[s]
		if w <= 0 {
			w = 1.0
		}
		seedVec[s] = w / totalWeight
	}

	// Current probability distribution starts at seeds.
	prob := make(map[types.Hash]float64)
	for k, v := range seedVec {
		prob[k] = v
	}

	// Edge weight multipliers by type.
	edgeWeight := map[string]float64{
		"calls":             1.0,
		"implements":        0.8,
		"implements_rpc":    0.8,
		"overrides":         0.8,
		"handles_route":     0.7,
		"extends":           0.7,
		"tests":             0.6,
		"consumes_rpc":      0.6,
		"imports":           0.5,
		"depends_on":        0.5,
		"consumes_endpoint": 0.5,
		"tested_by":         0.5,
		"references":        0.4,
		"throws":            0.4,
		"deployed_by":       0.4,
		"gated_by_flag":     0.3,
		"decorates":         0.3,
		"documents":         0.2,
		"owned_by":          0.0,
		"authored_by":       0.0,
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

			// Get edges from the pre-loaded adjacency map (no DB queries).
			edges := append(adjFrom[node], adjTo[node]...)

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
				w, ok := edgeWeight[e.EdgeType]
				if !ok {
					w = 0.3 // default for unknown edge types
				}
				totalWeight += w
			}

			// Distribute probability along edges proportional to weight.
			for _, e := range edges {
				w, ok := edgeWeight[e.EdgeType]
				if !ok {
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

// CommunityFilteredRWR is like RandomWalkWithRestart but constrains the BFS
// adjacency pre-load to nodes in the specified communities. When communityIDs
// is nil, the walk is unconstrained (identical to RandomWalkWithRestart).
func CommunityFilteredRWR(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash, alpha float64, maxIter int, communityIDs map[int]bool) (map[types.Hash]float64, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	if alpha <= 0 || alpha >= 1 {
		alpha = 0.2
	}
	if maxIter <= 0 {
		maxIter = 20
	}

	// Pre-load edges for the reachable subgraph, filtered by community.
	adjFrom, adjTo, err := buildAdjacencyMapFiltered(ctx, store, seeds, communityIDs)
	if err != nil {
		return nil, err
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
		"calls":             1.0,
		"implements":        0.8,
		"implements_rpc":    0.8,
		"overrides":         0.8,
		"handles_route":     0.7,
		"extends":           0.7,
		"tests":             0.6,
		"consumes_rpc":      0.6,
		"imports":           0.5,
		"depends_on":        0.5,
		"consumes_endpoint": 0.5,
		"tested_by":         0.5,
		"references":        0.4,
		"throws":            0.4,
		"deployed_by":       0.4,
		"gated_by_flag":     0.3,
		"decorates":         0.3,
		"documents":         0.2,
		"owned_by":          0.0,
		"authored_by":       0.0,
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

			// Get edges from the pre-loaded adjacency map (no DB queries).
			edges := append(adjFrom[node], adjTo[node]...)

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
				w, ok := edgeWeight[e.EdgeType]
				if !ok {
					w = 0.3 // default for unknown edge types
				}
				totalWeight += w
			}

			// Distribute probability along edges proportional to weight.
			for _, e := range edges {
				w, ok := edgeWeight[e.EdgeType]
				if !ok {
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

// buildAdjacencyMap pre-loads edges for the reachable subgraph (BFS from seeds,
// depth-limited to 3 hops) into in-memory maps so the RWR iteration loop
// requires zero database queries. Depth limit prevents loading the entire graph
// for well-connected seed sets.
func buildAdjacencyMap(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash) (adjFrom, adjTo map[types.Hash][]types.Edge, err error) {
	adjFrom = make(map[types.Hash][]types.Edge)
	adjTo = make(map[types.Hash][]types.Edge)

	// Adaptive BFS parameters. Scale with seed count as a proxy for graph
	// complexity: many seeds (15) = real query on a large graph, cap aggressively.
	// Few seeds (1-5) = unit test or focused query, allow full exploration.
	// Middle ground (6-10) = moderate cap.
	var maxDepth, maxNodes int
	switch {
	case len(seeds) <= 5:
		maxDepth = 6
		maxNodes = 50000
	case len(seeds) <= 10:
		maxDepth = 4
		maxNodes = 2000
	default:
		maxDepth = RWRMaxDepth
		maxNodes = RWRMaxNodes
	}

	// Build set of phantom external node hashes to exclude from BFS expansion.
	externals := loadExternalHashes(ctx, store)

	// BFS from seeds with depth limit and node cap.
	visited := make(map[types.Hash]bool, len(seeds)*4)
	frontier := make([]types.Hash, len(seeds))
	copy(frontier, seeds)
	for _, s := range seeds {
		visited[s] = true
	}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []types.Hash
		for _, node := range frontier {
			if len(visited) >= maxNodes {
				break
			}

			// Load outgoing edges for this node (once).
			if _, loaded := adjFrom[node]; !loaded {
				from, qErr := store.EdgesFrom(ctx, node, "")
				if qErr != nil {
					return nil, nil, qErr
				}
				adjFrom[node] = from
				for _, e := range from {
					if !visited[e.TargetHash] && !externals[e.TargetHash] {
						visited[e.TargetHash] = true
						nextFrontier = append(nextFrontier, e.TargetHash)
					}
				}
			}

			// Load incoming edges for this node (once).
			if _, loaded := adjTo[node]; !loaded {
				to, qErr := store.EdgesTo(ctx, node, "")
				if qErr != nil {
					return nil, nil, qErr
				}
				adjTo[node] = to
				for _, e := range to {
					if !visited[e.SourceHash] && !externals[e.SourceHash] {
						visited[e.SourceHash] = true
						nextFrontier = append(nextFrontier, e.SourceHash)
					}
				}
			}
		}
		if len(visited) >= maxNodes {
			break
		}
		frontier = nextFrontier
	}

	return adjFrom, adjTo, nil
}

// communityProvider is a local interface for stores that can batch-lookup
// community assignments for nodes.
type communityProvider interface {
	CommunitiesForNodes(ctx stdctx.Context, hashes []types.Hash) (map[types.Hash]int, error)
}

// buildAdjacencyMapFiltered is like buildAdjacencyMap but skips BFS expansion
// into nodes whose community assignment doesn't match the allowed set.
// Seeds are always included regardless of community. When communityFilter is nil,
// behaves identically to buildAdjacencyMap (no filtering).
func buildAdjacencyMapFiltered(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash, communityFilter map[int]bool) (adjFrom, adjTo map[types.Hash][]types.Edge, err error) {
	if communityFilter == nil {
		return buildAdjacencyMap(ctx, store, seeds)
	}

	// Check if the store supports community lookups.
	cp, ok := store.(communityProvider)
	if !ok {
		// Store doesn't support community lookups; fall back to unfiltered.
		return buildAdjacencyMap(ctx, store, seeds)
	}

	adjFrom = make(map[types.Hash][]types.Edge)
	adjTo = make(map[types.Hash][]types.Edge)

	const maxDepth = 4

	// Exclude phantom external nodes from BFS expansion.
	externals := loadExternalHashes(ctx, store)

	// BFS from seeds with depth limit and community filtering.
	visited := make(map[types.Hash]bool, len(seeds)*4)
	seedSet := make(map[types.Hash]bool, len(seeds))
	frontier := make([]types.Hash, len(seeds))
	copy(frontier, seeds)
	for _, s := range seeds {
		visited[s] = true
		seedSet[s] = true
	}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var candidates []types.Hash
		for _, node := range frontier {
			// Load outgoing edges for this node (once).
			if _, loaded := adjFrom[node]; !loaded {
				from, qErr := store.EdgesFrom(ctx, node, "")
				if qErr != nil {
					return nil, nil, qErr
				}
				adjFrom[node] = from
				for _, e := range from {
					if !visited[e.TargetHash] && !externals[e.TargetHash] {
						candidates = append(candidates, e.TargetHash)
					}
				}
			}

			// Load incoming edges for this node (once).
			if _, loaded := adjTo[node]; !loaded {
				to, qErr := store.EdgesTo(ctx, node, "")
				if qErr != nil {
					return nil, nil, qErr
				}
				adjTo[node] = to
				for _, e := range to {
					if !visited[e.SourceHash] && !externals[e.SourceHash] {
						candidates = append(candidates, e.SourceHash)
					}
				}
			}
		}

		// Deduplicate candidates.
		uniqueCandidates := make([]types.Hash, 0, len(candidates))
		seen := make(map[types.Hash]bool, len(candidates))
		for _, h := range candidates {
			if !seen[h] && !visited[h] {
				seen[h] = true
				uniqueCandidates = append(uniqueCandidates, h)
			}
		}

		if len(uniqueCandidates) == 0 {
			break
		}

		// Batch-query community assignments for all candidates.
		communities, cErr := cp.CommunitiesForNodes(ctx, uniqueCandidates)
		if cErr != nil {
			return nil, nil, cErr
		}

		// Filter: only add candidates whose community is in the allowed set
		// or who are seeds (seeds are always included).
		var nextFrontier []types.Hash
		for _, h := range uniqueCandidates {
			visited[h] = true
			if seedSet[h] {
				nextFrontier = append(nextFrontier, h)
				continue
			}
			if cid, hasCommunity := communities[h]; hasCommunity {
				if communityFilter[cid] {
					nextFrontier = append(nextFrontier, h)
				}
			} else {
				// No community assignment: include by default (don't filter out unknowns).
				nextFrontier = append(nextFrontier, h)
			}
		}
		frontier = nextFrontier
	}

	return adjFrom, adjTo, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
