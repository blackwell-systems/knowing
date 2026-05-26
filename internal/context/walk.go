package context

import (
	"bytes"
	stdctx "context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// adjacencyCacheKey is the note key used to store the pre-computed adjacency map.
const adjacencyCacheKey = "adjacency_cache"

// defaultEdgeWeight is used for unknown edge types in the weight lookup.
const defaultEdgeWeight = 0.3

// crossModuleAttenuation is the weight multiplier applied when RWR transitions
// between nodes in different top-level directories (modules). This prevents
// library/dependency code from absorbing probability mass that should stay in
// the module containing the query seeds.
const crossModuleAttenuation = 0.3

// buildNodeModuleMap constructs a mapping from node hash to its top-level
// directory (the first path component of the file path). Nodes in the same
// top-level directory are considered "same module" for attenuation purposes.
// Returns nil if the store doesn't support the required query or if all nodes
// share the same top-level directory (no attenuation needed).
func buildNodeModuleMap(ctx stdctx.Context, store types.GraphStore, nodeHashes []types.Hash) map[types.Hash]string {
	type nodePathQuerier interface {
		NodeTopDirs(ctx stdctx.Context, hashes []types.Hash) (map[types.Hash]string, error)
	}

	// Fast path: use dedicated batch query if available.
	if q, ok := store.(nodePathQuerier); ok {
		m, err := q.NodeTopDirs(ctx, nodeHashes)
		if err != nil || len(m) == 0 {
			return nil
		}
		// Check if there's actually more than one module. If all nodes share
		// the same top-level dir, attenuation is a no-op (skip the cost).
		var first string
		allSame := true
		for _, dir := range m {
			if first == "" {
				first = dir
			} else if dir != first {
				allSame = false
				break
			}
		}
		if allSame {
			return nil
		}
		return m
	}

	// Fallback: use qualified name parsing. QN format: "repoURL://filePath.Symbol"
	type qnQuerier interface {
		GetNode(ctx stdctx.Context, hash types.Hash) (*types.Node, error)
	}
	q, ok := store.(qnQuerier)
	if !ok {
		return nil
	}

	// Sample a subset to avoid loading all nodes (cap at 2000 for performance).
	sampleSize := len(nodeHashes)
	if sampleSize > 2000 {
		sampleSize = 2000
	}

	m := make(map[types.Hash]string, sampleSize)
	var first string
	allSame := true

	for i := 0; i < sampleSize; i++ {
		node, err := q.GetNode(ctx, nodeHashes[i])
		if err != nil || node == nil {
			continue
		}
		dir := topDirFromQN(node.QualifiedName)
		if dir != "" {
			m[nodeHashes[i]] = dir
			if first == "" {
				first = dir
			} else if dir != first {
				allSame = false
			}
		}
	}

	if allSame || len(m) == 0 {
		return nil
	}
	return m
}

// topDirFromQN extracts the top-level directory from a qualified name.
// QN format: "repoURL://path/to/file.go.SymbolName"
// Returns the first path component (e.g., "pkg", "staging", "cmd", "internal").
func topDirFromQN(qn string) string {
	idx := strings.Index(qn, "://")
	if idx < 0 {
		return ""
	}
	path := qn[idx+3:]
	// First component of the path.
	slashIdx := strings.IndexByte(path, '/')
	if slashIdx < 0 {
		return ""
	}
	return path[:slashIdx]
}

// collectNodeHashes returns all unique node hashes present in the adjacency maps.
func collectNodeHashes(adjFrom, adjTo map[types.Hash][]types.Edge) []types.Hash {
	seen := make(map[types.Hash]struct{}, len(adjFrom)+len(adjTo))
	for h := range adjFrom {
		seen[h] = struct{}{}
	}
	for h := range adjTo {
		seen[h] = struct{}{}
	}
	result := make([]types.Hash, 0, len(seen))
	for h := range seen {
		result = append(result, h)
	}
	return result
}

// ExcludeEdgeTypes is a set of edge types to exclude from adjacency map construction.
// When non-nil, edges of these types are skipped during BFS expansion AND during RWR
// iteration. Used for ablation studies (diagnosing which edge types cause dilution).
// Set via bench adapter or CLI; nil means all edge types are included.
var ExcludeEdgeTypes map[string]bool

// BFSMaxDepth controls the BFS expansion depth for adjacency map construction.
// Default 4. On dense graphs (>50K nodes), reducing to 2-3 limits how many nodes
// enter the RWR walk, preventing probability mass dilution.
// 0 means use default (4).
var BFSMaxDepth int

// HubDampeningThreshold: after RWR, penalize nodes with in-degree above this
// threshold by dividing their score by sqrt(in-degree/threshold). 0 means disabled.
// Targets hub nodes that absorb probability regardless of query (Disposable, Event, etc.)
var HubDampeningThreshold int

// PreferTypeSeeds: when true, BM25/tiered results are reordered to prioritize
// type/interface/class nodes over methods/functions as RWR seeds. On dense graphs,
// types are better seeds because RWR walks from them to methods via contains edges.
var PreferTypeSeeds bool

// AdaptiveDensity: when true, automatically enable hub dampening and type-seed
// preference based on graph density (nodes/edges ratio from the store). Dense
// graphs (>50K nodes or edges/nodes > 5) get hub dampening at threshold 50 and
// type-seed preference enabled. This eliminates the need for manual env var tuning.
var AdaptiveDensity bool

// GraphNodeCount is set by the adapter/engine when the graph size is known.
// Used by AdaptiveDensity to decide thresholds. 0 means unknown.
var GraphNodeCount int

// ReRankOriginalWeight controls the blend between original RWR score and embedding
// similarity in the re-ranker. Range [0.0, 1.0]. Higher = more conservative (preserves
// original ranking/MRR). Lower = more aggressive re-ranking (better recall, worse MRR).
// Default 0.0 (pure re-rank by embedding similarity). Validated on full 167-task corpus:
// P@10 0.207 -> 0.242 (+17%), R@10 +18.3%, MRR +8.1%. All metrics improved.
// Set via BENCH_RERANK_WEIGHT for parameter sweep experiments.
var ReRankOriginalWeight = 0.0

// AdaptiveSeedCount, when true, increases maxSeeds based on GraphNodeCount.
// On large graphs (>40K nodes), more seeds compensate for higher disconnection
// rates where ground truth symbols are further from any individual seed.
// Default false; auto-enabled when AdaptiveDensity is true.
var AdaptiveSeedCount bool

// CoherenceBonus controls the density boost for symbols that share a file with
// already-packed symbols. Range [0.0, 1.0]. At 0.0 (default), packing is purely
// density-ranked (current behavior). At 0.3, a symbol co-located with a packed
// symbol gets a 30% density boost, favoring coherent subgraphs over scattered
// high-scoring singletons. Set via BENCH_COHERENCE_BONUS for experiments.
var CoherenceBonus = 0.0

// edgeWeights maps edge type strings to weight multipliers used during RWR iteration.
// Higher weights cause more probability to flow along those edge types.
var edgeWeights = map[string]float64{
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
	"co_tested_with":    0.5,
	"type_hint_of":      0.5,
	"accesses_field":    0.6,
	"reads_env":         0.4,
	"executes_process":  0.5,
	"references":        0.4,
	"throws":            0.4,
	"deployed_by":       0.4,
	"gated_by_flag":     0.3,
	"decorates":         0.3,
	"documents":         0.2,
	"similar_to":        0.15,
	"contains":          0.0, // structural: excluded from walk (path seeding uses them directly)
	"member_of":         0.0, // structural: excluded from walk (reverse of contains)
	"owned_by":          0.0,
	"authored_by":       0.0,
}

// adjEdgeTypeToID maps edge type strings to compact uint8 IDs for binary encoding.
// ID 0 is reserved for unknown types.
var adjEdgeTypeToID = map[string]uint8{
	"calls":             1,
	"imports":           2,
	"implements":        3,
	"references":        4,
	"handles_route":     5,
	"depends_on":        6,
	"extends":           7,
	"overrides":         8,
	"decorates":         9,
	"throws":            10,
	"owned_by":          11,
	"authored_by":       12,
	"tests":             13,
	"documents":         14,
	"consumes_endpoint": 15,
	"implements_rpc":    16,
	"consumes_rpc":      17,
	"gated_by_flag":     18,
	"deployed_by":       19,
	"tested_by":         20,
	"deploys":           21,
	"exposes":           22,
	"configures":        23,
	"publishes":         24,
	"subscribes":        25,
	"connects_to":       26,
	"runtime_calls":     27,
	"runtime_rpc":       28,
	"runtime_produces":  29,
	"runtime_consumes":  30,
	"similar_to":        31,
	"co_tested_with":    32,
	"type_hint_of":      33,
	"accesses_field":    34,
	"reads_env":         35,
	"executes_process":  36,
}

// adjIDToEdgeType is the reverse mapping from uint8 ID to edge type string.
var adjIDToEdgeType = func() map[uint8]string {
	m := make(map[uint8]string, len(adjEdgeTypeToID))
	for k, v := range adjEdgeTypeToID {
		m[v] = k
	}
	return m
}()

// BuildAdjacencyCache builds the full adjacency map and stores it as a compact
// binary blob (base64-encoded) in the notes table. Format: [num_edges:4 LE]
// followed by num_edges records of [source:32][target:32][type_id:1] = 65 bytes
// per edge. Call after indexing. Subsequent RWR queries load this cache in one
// read instead of per-node edge queries.
func BuildAdjacencyCache(ctx stdctx.Context, store types.GraphStore) error {
	type edgeLoader interface {
		AllEdges(ctx stdctx.Context) ([]types.Edge, error)
	}
	type noteWriter interface {
		PutNote(ctx stdctx.Context, note types.Note) error
	}

	bulk, ok := store.(edgeLoader)
	if !ok {
		return fmt.Errorf("store %T does not implement edgeLoader", store)
	}
	ns, ok := store.(noteWriter)
	if !ok {
		return nil
	}

	allEdges, err := bulk.AllEdges(ctx)
	if err != nil {
		return fmt.Errorf("AllEdges: %w", err)
	}
	if len(allEdges) == 0 {
		return fmt.Errorf("AllEdges returned 0 edges")
	}

	// Encode: 4-byte LE edge count + 65 bytes per edge.
	buf := bytes.NewBuffer(make([]byte, 0, 4+len(allEdges)*65))
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(allEdges)))
	buf.Write(header)

	for _, e := range allEdges {
		buf.Write(e.SourceHash[:])
		buf.Write(e.TargetHash[:])
		tid := adjEdgeTypeToID[e.EdgeType] // 0 for unknown
		buf.WriteByte(tid)
	}

	cacheHash := types.NewHash([]byte("adjacency_cache_v2"))
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return ns.PutNote(ctx, types.Note{
		ObjectHash: cacheHash,
		Key:        adjacencyCacheKey,
		Value:      encoded,
		UpdatedAt:  time.Now().Unix(),
	})
}


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

	// Build module map for cross-module attenuation.
	allNodes := collectNodeHashes(adjFrom, adjTo)
	modMap := buildNodeModuleMap(ctx, store, allNodes)

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

	return rwrIterate(seedVec, alpha, maxIter, adjFrom, adjTo, modMap), nil
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

	// Build module map for cross-module attenuation.
	allNodes := collectNodeHashes(adjFrom, adjTo)
	modMap := buildNodeModuleMap(ctx, store, allNodes)

	// Initialize: uniform probability across seeds.
	seedWeight := 1.0 / float64(len(seeds))
	seedVec := make(map[types.Hash]float64, len(seeds))
	for _, s := range seeds {
		seedVec[s] = seedWeight
	}

	return rwrIterate(seedVec, alpha, maxIter, adjFrom, adjTo, modMap), nil
}

// rwrIterate runs the core RWR power-iteration loop. It is shared by both
// RandomWalkWithRestartWeighted and CommunityFilteredRWR.
//
// It uses a double-buffer pattern to avoid per-iteration map allocation and
// iterates over adjacency edges without temporary slice allocation.
//
// nodeModule maps each node hash to its top-level directory (module indicator).
// When non-nil, transitions between nodes in different modules are attenuated
// by crossModuleAttenuation. This prevents dependency code from absorbing
// probability mass that should stay in the query's home module.
func rwrIterate(seedVec map[types.Hash]float64, alpha float64, maxIter int, adjFrom, adjTo map[types.Hash][]types.Edge, nodeModule ...map[types.Hash]string) map[types.Hash]float64 {
	var modMap map[types.Hash]string
	if len(nodeModule) > 0 {
		modMap = nodeModule[0]
	}
	// Double-buffer pattern: reuse two maps across iterations instead of
	// allocating a new map each iteration.
	mapA := make(map[types.Hash]float64, len(seedVec)*4)
	mapB := make(map[types.Hash]float64, len(seedVec)*4)

	// Initialize prob (mapA) with seed vector.
	for k, v := range seedVec {
		mapA[k] = v
	}
	prob := mapA

	// Early termination: stop when the top-K ranking is stable for 2 consecutive
	// iterations. On large graphs, low-ranked nodes keep shifting even after the
	// top results have converged, wasting iterations.
	const earlyTopK = 10
	var prevTopK [earlyTopK]types.Hash
	stableCount := 0

	for iter := 0; iter < maxIter; iter++ {
		next := mapB
		// Clear the next buffer.
		for k := range next {
			delete(next, k)
		}

		// Restart component: alpha * seed_vector.
		for s, w := range seedVec {
			next[s] += alpha * w
		}

		// Walk component: (1-alpha) * transition from current distribution.
		for node, nodeProb := range prob {
			if nodeProb < 0.0001 {
				continue // skip negligible nodes
			}

			fromEdges := adjFrom[node]
			toEdges := adjTo[node]

			if len(fromEdges) == 0 && len(toEdges) == 0 {
				// Dead end: redistribute to seeds (effectively a restart).
				for s, w := range seedVec {
					next[s] += (1 - alpha) * nodeProb * w
				}
				continue
			}

			// Compute total edge weight for normalization.
			totalWeight := 0.0
			for _, e := range fromEdges {
				w, ok := edgeWeights[e.EdgeType]
				if !ok {
					w = defaultEdgeWeight
				}
				totalWeight += w
			}
			for _, e := range toEdges {
				w, ok := edgeWeights[e.EdgeType]
				if !ok {
					w = defaultEdgeWeight
				}
				totalWeight += w
			}

			// Distribute probability along edges proportional to weight.
			// Cross-module transitions are attenuated to prevent dependency
			// code from absorbing probability meant for the query's home module.
			srcMod := ""
			if modMap != nil {
				srcMod = modMap[node]
			}
			for _, e := range fromEdges {
				w, ok := edgeWeights[e.EdgeType]
				if !ok {
					w = defaultEdgeWeight
				}
				target := e.TargetHash
				if target == node {
					target = e.SourceHash
				}
				if modMap != nil && srcMod != modMap[target] && srcMod != "" && modMap[target] != "" {
					w *= crossModuleAttenuation
				}
				next[target] += (1 - alpha) * nodeProb * (w / totalWeight)
			}
			for _, e := range toEdges {
				w, ok := edgeWeights[e.EdgeType]
				if !ok {
					w = defaultEdgeWeight
				}
				target := e.TargetHash
				if target == node {
					target = e.SourceHash
				}
				if modMap != nil && srcMod != modMap[target] && srcMod != "" && modMap[target] != "" {
					w *= crossModuleAttenuation
				}
				next[target] += (1 - alpha) * nodeProb * (w / totalWeight)
			}
		}

		// Check convergence: sum of absolute differences.
		delta := 0.0
		for k, v := range next {
			delta += math.Abs(v - prob[k])
		}
		for k, v := range prob {
			if _, exists := next[k]; !exists {
				delta += v
			}
		}

		// Swap buffers.
		prob, next = next, prob
		mapA, mapB = mapB, mapA

		if delta < 0.001 {
			break
		}

		// Top-K stability check: if the top-10 nodes by score haven't changed
		// ordering for 2 consecutive iterations, the ranking is converged even
		// if low-ranked nodes are still shifting.
		curTopK := topKFromProb(prob, earlyTopK)
		if curTopK == prevTopK {
			stableCount++
			if stableCount >= 2 {
				break
			}
		} else {
			stableCount = 0
		}
		prevTopK = curTopK
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

	return prob
}

// buildAdjacencyMap pre-loads edges for the reachable subgraph (BFS from seeds,
// depth-limited) into in-memory maps so the RWR iteration loop requires zero
// database queries.
func buildAdjacencyMap(ctx stdctx.Context, store types.GraphStore, seeds []types.Hash) (adjFrom, adjTo map[types.Hash][]types.Edge, err error) {
	// Try pre-computed cache first (one read, in-memory BFS).
	type noteReader interface {
		GetNote(ctx stdctx.Context, objectHash types.Hash, key string) (*types.Note, error)
	}

	externals := loadExternalHashes(ctx, store)

	if ns, ok := store.(noteReader); ok {
		cacheHash := types.NewHash([]byte("adjacency_cache_v2"))
		if note, nErr := ns.GetNote(ctx, cacheHash, adjacencyCacheKey); nErr == nil && note != nil && len(note.Value) > 100 {
			if from, to, cErr := buildFromCache(note.Value, seeds, externals); cErr == nil {
				return from, to, nil
			}
		}
	}

	// Fallback: per-node BFS loading.
	adjFrom = make(map[types.Hash][]types.Edge)
	adjTo = make(map[types.Hash][]types.Edge)

	maxDepth := BFSMaxDepth
	if maxDepth == 0 {
		maxDepth = 4
	}

	visited := make(map[types.Hash]bool, len(seeds)*4)
	frontier := make([]types.Hash, len(seeds))
	copy(frontier, seeds)
	for _, s := range seeds {
		visited[s] = true
	}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []types.Hash
		for _, node := range frontier {
			if _, loaded := adjFrom[node]; !loaded {
				from, qErr := store.EdgesFrom(ctx, node, "")
				if qErr != nil {
					return nil, nil, qErr
				}
				adjFrom[node] = from
				for _, e := range from {
					if ExcludeEdgeTypes != nil && ExcludeEdgeTypes[e.EdgeType] {
						continue
					}
					if !visited[e.TargetHash] && !externals[e.TargetHash] {
						// Don't expand frontier through weight-0 edges (contains,
						// member_of, authored_by). These create structural connections
						// but including their targets in BFS floods the adjacency map
						// with thousands of nodes that dilute RWR probability.
						if w, ok := edgeWeights[e.EdgeType]; ok && w == 0 {
							continue
						}
						visited[e.TargetHash] = true
						nextFrontier = append(nextFrontier, e.TargetHash)
					}
				}
			}

			if _, loaded := adjTo[node]; !loaded {
				to, qErr := store.EdgesTo(ctx, node, "")
				if qErr != nil {
					return nil, nil, qErr
				}
				adjTo[node] = to
				for _, e := range to {
					if ExcludeEdgeTypes != nil && ExcludeEdgeTypes[e.EdgeType] {
						continue
					}
					if !visited[e.SourceHash] && !externals[e.SourceHash] {
						if w, ok := edgeWeights[e.EdgeType]; ok && w == 0 {
							continue
						}
						visited[e.SourceHash] = true
						nextFrontier = append(nextFrontier, e.SourceHash)
					}
				}
			}
		}
		frontier = nextFrontier
	}

	return adjFrom, adjTo, nil
}

// buildFromCache deserializes a base64-encoded compact binary adjacency cache
// and runs BFS in memory (zero DB queries for the walk itself).
// Binary format: [num_edges:4 LE][source:32][target:32][type_id:1] * num_edges.
func buildFromCache(data string, seeds []types.Hash, externals map[types.Hash]bool) (map[types.Hash][]types.Edge, map[types.Hash][]types.Edge, error) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) < 4 {
		return nil, nil, fmt.Errorf("adjacency cache too short: %d bytes", len(raw))
	}

	numEdges := int(binary.LittleEndian.Uint32(raw[:4]))
	expected := 4 + numEdges*65
	if len(raw) < expected {
		return nil, nil, fmt.Errorf("adjacency cache truncated: have %d bytes, need %d", len(raw), expected)
	}

	// Decode into from/to adjacency maps keyed by hash.
	type compactEdge struct {
		source   types.Hash
		target   types.Hash
		edgeType string
	}
	fromMap := make(map[types.Hash][]compactEdge, numEdges/4)
	toMap := make(map[types.Hash][]compactEdge, numEdges/4)

	off := 4
	for i := 0; i < numEdges; i++ {
		var src, tgt types.Hash
		copy(src[:], raw[off:off+32])
		copy(tgt[:], raw[off+32:off+64])
		tid := raw[off+64]
		off += 65

		et := adjIDToEdgeType[tid]
		if et == "" {
			et = "references" // fallback for unknown IDs
		}
		ce := compactEdge{source: src, target: tgt, edgeType: et}
		fromMap[src] = append(fromMap[src], ce)
		toMap[tgt] = append(toMap[tgt], ce)
	}

	// BFS in memory.
	visited := make(map[types.Hash]bool, len(seeds)*4)
	frontier := make([]types.Hash, len(seeds))
	copy(frontier, seeds)
	for _, s := range seeds {
		visited[s] = true
	}

	maxDepth := BFSMaxDepth
	if maxDepth == 0 {
		maxDepth = 4
	}
	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []types.Hash
		for _, node := range frontier {
			for _, ce := range fromMap[node] {
				if ExcludeEdgeTypes != nil && ExcludeEdgeTypes[ce.edgeType] {
					continue
				}
				if !visited[ce.target] && !externals[ce.target] {
					visited[ce.target] = true
					nextFrontier = append(nextFrontier, ce.target)
				}
			}
			for _, ce := range toMap[node] {
				if ExcludeEdgeTypes != nil && ExcludeEdgeTypes[ce.edgeType] {
					continue
				}
				if !visited[ce.source] && !externals[ce.source] {
					visited[ce.source] = true
					nextFrontier = append(nextFrontier, ce.source)
				}
			}
		}
		frontier = nextFrontier
	}

	// Convert to types.Edge for visited nodes.
	adjFrom := make(map[types.Hash][]types.Edge, len(visited))
	adjTo := make(map[types.Hash][]types.Edge, len(visited))
	for node := range visited {
		for _, ce := range fromMap[node] {
			if ExcludeEdgeTypes != nil && ExcludeEdgeTypes[ce.edgeType] {
				continue
			}
			adjFrom[node] = append(adjFrom[node], types.Edge{
				SourceHash: ce.source,
				TargetHash: ce.target,
				EdgeType:   ce.edgeType,
			})
		}
		for _, ce := range toMap[node] {
			if ExcludeEdgeTypes != nil && ExcludeEdgeTypes[ce.edgeType] {
				continue
			}
			adjTo[node] = append(adjTo[node], types.Edge{
				SourceHash: ce.source,
				TargetHash: ce.target,
				EdgeType:   ce.edgeType,
			})
		}
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

// topKFromProb returns the top-K node hashes by score as a fixed-size array.
// Used for early termination: if this array is unchanged between iterations,
// the ranking has converged.
func topKFromProb(prob map[types.Hash]float64, k int) [10]types.Hash {
	var result [10]types.Hash
	var scores [10]float64

	for h, s := range prob {
		// Find insertion point (simple insertion sort for small k).
		for i := 0; i < k; i++ {
			if s > scores[i] {
				// Shift down.
				copy(scores[i+1:], scores[i:k-1])
				copy(result[i+1:], result[i:k-1])
				scores[i] = s
				result[i] = h
				break
			}
		}
	}
	return result
}
