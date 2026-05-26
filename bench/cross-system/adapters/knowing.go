package adapters

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
	"github.com/blackwell-systems/knowing/internal/community"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/embedding"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"

	stdctx "context"
)

func init() {
	// BENCH_EXCLUDE_EDGES=similar_to,type_hint_of excludes these edge types
	// from RWR walk at query time (no reindex needed). For ablation studies.
	if envExclude := os.Getenv("BENCH_EXCLUDE_EDGES"); envExclude != "" {
		exclude := make(map[string]bool)
		for _, et := range strings.Split(envExclude, ",") {
			et = strings.TrimSpace(et)
			if et != "" {
				exclude[et] = true
			}
		}
		knowingctx.ExcludeEdgeTypes = exclude
	}

	// BENCH_BFS_DEPTH=2 limits BFS expansion depth (default 4).
	// On dense graphs, lower depth prevents reaching the entire graph.
	if envDepth := os.Getenv("BENCH_BFS_DEPTH"); envDepth != "" {
		if d, err := strconv.Atoi(envDepth); err == nil && d > 0 {
			knowingctx.BFSMaxDepth = d
		}
	}

	// BENCH_HUB_DAMPEN=50 penalizes nodes with in-degree > 50.
	if envHub := os.Getenv("BENCH_HUB_DAMPEN"); envHub != "" {
		if h, err := strconv.Atoi(envHub); err == nil && h > 0 {
			knowingctx.HubDampeningThreshold = h
		}
	}

	// BENCH_PREFER_TYPE_SEEDS=1 reorders candidates to prefer types as seeds.
	if os.Getenv("BENCH_PREFER_TYPE_SEEDS") == "1" {
		knowingctx.PreferTypeSeeds = true
	}

	// BENCH_RERANK_WEIGHT=0.85 sets the original score weight in the re-ranker blend.
	// Range [0.0, 1.0]. Default 0.0. Higher = more conservative (preserves MRR).
	if envWeight := os.Getenv("BENCH_RERANK_WEIGHT"); envWeight != "" {
		if w, err := strconv.ParseFloat(envWeight, 64); err == nil && w >= 0 && w <= 1 {
			knowingctx.ReRankOriginalWeight = w
		}
	}

	// BENCH_ADAPTIVE_SEEDS=1 increases seed count on large graphs.
	if os.Getenv("BENCH_ADAPTIVE_SEEDS") == "1" {
		knowingctx.AdaptiveSeedCount = true
	}

	// BENCH_MAX_SEEDS=25 overrides the max seed count directly (for sweeps).
	if envSeeds := os.Getenv("BENCH_MAX_SEEDS"); envSeeds != "" {
		if s, err := strconv.Atoi(envSeeds); err == nil && s > 0 {
			knowingctx.SetSweepParams(knowingctx.SweepParams{MaxSeeds: s})
		}
	}

	// BENCH_COHERENCE_BONUS=0.3 boosts density for symbols co-located with
	// already-packed symbols. Range [0.0, 1.0]. Default 0.0 (no bonus).
	if envCoherence := os.Getenv("BENCH_COHERENCE_BONUS"); envCoherence != "" {
		if c, err := strconv.ParseFloat(envCoherence, 64); err == nil && c >= 0 && c <= 1 {
			knowingctx.CoherenceBonus = c
		}
	}

}

// Knowing implements benchtype.Adapter for knowing's context engine.
type Knowing struct {
	stores     map[string]*store.SQLiteStore
	memories   map[string]*knowingctx.TaskMemory // per-repo task memory for compounding tests
	nodeCounts map[string]int                    // cached node count per repo for adaptive density
	searchers  map[string]*embedding.Searcher    // per-repo vector searcher (nil if embeddings disabled)
}

func NewKnowing() *Knowing {
	return &Knowing{
		stores:     make(map[string]*store.SQLiteStore),
		memories:   make(map[string]*knowingctx.TaskMemory),
		nodeCounts: make(map[string]int),
		searchers:  make(map[string]*embedding.Searcher),
	}
}

func (a *Knowing) Name() string { return "knowing" }

// StoreFor returns the SQLiteStore for a repo path (for test access).
func (a *Knowing) StoreFor(repoPath string) *store.SQLiteStore {
	return a.stores[repoPath]
}

// MemoryFor returns the TaskMemory for a repo path (for test access).
func (a *Knowing) MemoryFor(repoPath string) *knowingctx.TaskMemory {
	return a.memories[repoPath]
}

func (a *Knowing) Index(repoPath string) (int64, error) {
	start := time.Now()
	dbPath := repoPath + "/.knowing/graph.db"
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return 0, err
	}
	a.stores[repoPath] = s
	// Initialize task memory for this repo (enables compounding across queries).
	a.memories[repoPath] = knowingctx.NewTaskMemory(s.DB())

	ctx := stdctx.Background()

	// Cache node count for adaptive density (used at Retrieve time).
	if nodes, err := s.NodesByName(ctx, "%"); err == nil {
		a.nodeCounts[repoPath] = len(nodes)
	}

	// Generate contains edges (type -> method) if not already present.
	// This connects disconnected type/class nodes to their methods via QN structure.
	ensureContainsEdges(ctx, s)

	// Generate co_tested_with edges (lateral connections between non-test symbols
	// referenced from the same test file).
	ensureCoTestedEdges(ctx, s)

	// Run community detection if not already computed. This enables
	// community-aware RWR filtering in the retrieval pipeline.
	existing, _ := community.LoadAssignments(ctx, s)
	if len(existing) == 0 {
		if nodes, err := s.NodesByName(ctx, "%"); err == nil && len(nodes) > 0 {
			// Cap community detection to 5000 nodes for large repos.
			// Loading 253K+ node adjacency lists is too slow for benchmarks.
			if len(nodes) > 5000 {
				nodes = nodes[:5000]
			}
			nodeSet := make(map[types.Hash]bool, len(nodes))
			nodeHashes := make([]types.Hash, len(nodes))
			for i, n := range nodes {
				nodeSet[n.NodeHash] = true
				nodeHashes[i] = n.NodeHash
			}
			adj := make(map[types.Hash][]community.WeightedEdge, len(nodes))
			edgeCount := 0
			maxNodes := len(nodes)
			for i := 0; i < maxNodes; i++ {
				edges, err := s.EdgesFrom(ctx, nodes[i].NodeHash, "")
				if err != nil {
					continue
				}
				for _, e := range edges {
					if !nodeSet[e.TargetHash] {
						continue
					}
					adj[nodes[i].NodeHash] = append(adj[nodes[i].NodeHash], community.WeightedEdge{
						Target: e.TargetHash, Weight: e.Confidence,
					})
					adj[e.TargetHash] = append(adj[e.TargetHash], community.WeightedEdge{
						Target: nodes[i].NodeHash, Weight: e.Confidence,
					})
					edgeCount++
				}
			}
			g := &community.Graph{
				Nodes:     nodeHashes,
				Adj:       adj,
				NodeSet:   nodeSet,
				EdgeCount: edgeCount,
			}
			algo := community.Get(community.Default)
			membership := algo.Detect(g)
			_ = community.SaveAssignments(ctx, s, membership)
		}
	}

	// Build embedding index if BENCH_EMBEDDINGS=1.
	// Only embeds the first N symbols (sorted by node hash for determinism).
	// Embedding all 253K k8s nodes would take ~60 min; cap at 5000 for benchmark viability.
	// Skip if already built for this repo (Index is called per-task, not per-repo).
	if os.Getenv("BENCH_EMBEDDINGS") == "1" && a.searchers[repoPath] == nil {
		embedder, err := embedding.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [warn] embeddings disabled: %v\n", err)
		} else {
			searcher := embedding.NewSearcher(embedder)
			searcher.SetStore(s) // persistent vector cache
			nodes, err := s.NodesByName(ctx, "%")
			if err == nil && len(nodes) > 0 {
				// Cap at 5000 symbols for benchmark feasibility (~70s at 14ms/symbol).
				maxEmbed := 5000
				if len(nodes) < maxEmbed {
					maxEmbed = len(nodes)
				}
				embedNodes := nodes[:maxEmbed]
				// Batch embed in chunks of 64 for memory efficiency.
				batchSize := 64
				for i := 0; i < len(embedNodes); i += batchSize {
					end := i + batchSize
					if end > len(embedNodes) {
						end = len(embedNodes)
					}
					_ = searcher.IndexBatch(ctx, embedNodes[i:end], nil)
				}
				fmt.Fprintf(os.Stderr, "  Embeddings: %d symbols indexed (of %d total)\n", searcher.Count(), len(nodes))
			}
			a.searchers[repoPath] = searcher
		}
	}

	return time.Since(start).Milliseconds(), nil
}

func (a *Knowing) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	s, ok := a.stores[repoPath]
	if !ok {
		return benchtype.RetrievalResult{System: "knowing", TaskID: task.ID, Error: "repo not indexed"}, nil
	}

	ctx := stdctx.Background()
	start := time.Now()

	// Set graph node count for adaptive density (PreferTypeSeeds auto-enables >50K).
	knowingctx.GraphNodeCount = a.nodeCounts[repoPath]

	engine := knowingctx.NewContextEngine(s)
	engine.DisablePersistentCache() // Ensure fresh retrieval for benchmark accuracy.

	// Attach vector search if available (embedding-based semantic retrieval).
	if vs, ok := a.searchers[repoPath]; ok && vs != nil {
		engine.SetVector(vs)
	}

	// Attach task memory if available (enables compounding across queries).
	if tm, ok := a.memories[repoPath]; ok && tm != nil {
		engine.SetTaskMemory(tm)
	}

	// Determine primary repo URL for scoping (prevents cross-repo dilution
	// in multi-module DBs like k8s with staging modules).
	var repoURL string
	if repos, err := s.AllRepos(ctx); err == nil && len(repos) > 0 {
		repoURL = repos[0].RepoURL
	}

	result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: task.Description,
		TokenBudget:     tokenBudget,
		Format:          "json",
		RepoURL:         repoURL,
	})
	if err != nil {
		return benchtype.RetrievalResult{System: "knowing", TaskID: task.ID, Error: err.Error()}, nil
	}

	latency := time.Since(start).Milliseconds()

	// Record top-5 returned symbols in task memory for future compounding.
	if tm, ok := a.memories[repoPath]; ok && tm != nil && len(result.Symbols) > 0 {
		normalizedKws := knowingctx.NormalizeKeywords(task.Description)
		topN := 5
		if len(result.Symbols) < topN {
			topN = len(result.Symbols)
		}
		symbolHashes := make([]types.Hash, topN)
		for i := 0; i < topN; i++ {
			symbolHashes[i] = result.Symbols[i].Node.NodeHash
		}
		_ = tm.RecordBatch(ctx, normalizedKws, symbolHashes, 0.6)
	}

	symbols := make([]benchtype.RetrievedSymbol, len(result.Symbols))
	for i, sym := range result.Symbols {
		symbols[i] = benchtype.RetrievedSymbol{
			QualifiedName: sym.Node.QualifiedName,
			Normalized:    normalize.Symbol(sym.Node.QualifiedName),
			Score:         sym.Score,
			Rank:          i + 1,
			Kind:          sym.Node.Kind,
		}
	}

	return benchtype.RetrievalResult{
		System:     "knowing",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: result.TokensUsed,
		LatencyMs:  latency,
	}, nil
}

func (a *Knowing) SupportsLearning() bool { return true }

func (a *Knowing) RecordFeedback(repoPath string, task benchtype.Task, relevantSymbols []string) error {
	s, ok := a.stores[repoPath]
	if !ok {
		return nil
	}
	ctx := stdctx.Background()
	for _, sym := range relevantSymbols {
		hash := types.NewHash([]byte(sym))
		_ = s.RecordFeedback(ctx, hash, "benchmark", true, types.EmptyHash)
	}
	return nil
}

func (a *Knowing) Reset(repoPath string) error {
	if s, ok := a.stores[repoPath]; ok {
		s.Close()
		delete(a.stores, repoPath)
	}
	return nil
}

// EnsureContainsEdgesPublic is the exported entry point for the parameter sweep test.
func EnsureContainsEdgesPublic(ctx stdctx.Context, s *store.SQLiteStore) {
	ensureContainsEdges(ctx, s)
}

// ensureContainsEdges generates structural "contains" edges from type/class nodes
// to their method/field nodes if they don't already exist. This is a one-time
// migration for pre-indexed databases that lack these edges.
func ensureContainsEdges(ctx stdctx.Context, s *store.SQLiteStore) {
	// Check if contains edges already exist.
	// Quick heuristic: if any "contains" edge exists, assume migration is done.
	testNodes, _ := s.NodesByName(ctx, "%")
	if len(testNodes) == 0 {
		return
	}
	// Check for existing contains edges by querying a sample.
	for _, n := range testNodes[:min(len(testNodes), 10)] {
		edges, _ := s.EdgesFrom(ctx, n.NodeHash, edgetype.Contains)
		if len(edges) > 0 {
			return // already has contains edges
		}
	}

	// Load all nodes and generate contains edges.
	allNodes, err := s.NodesByName(ctx, "%")
	if err != nil || len(allNodes) == 0 {
		return
	}

	// Build map of type/class QNs to hashes.
	typeQNs := make(map[string]types.Hash)
	for i := range allNodes {
		n := &allNodes[i]
		switch n.Kind {
		case "type", "class", "struct", "interface", "trait", "module", "object":
			typeQNs[n.QualifiedName] = n.NodeHash
		}
	}
	if len(typeQNs) == 0 {
		return
	}

	// For each non-type node, check if parent QN is a known type.
	var edges []types.Edge
	seen := make(map[types.Hash]bool)
	for i := range allNodes {
		n := &allNodes[i]
		switch n.Kind {
		case "type", "class", "struct", "interface", "trait", "module", "object":
			continue
		}
		qn := n.QualifiedName
		lastDot := strings.LastIndex(qn, ".")
		if lastDot < 0 {
			continue
		}
		parentQN := qn[:lastDot]
		parentHash, ok := typeQNs[parentQN]
		if !ok {
			continue
		}
		edgeHash := types.ComputeEdgeHash(parentHash, n.NodeHash, edgetype.Contains, "structural")
		if seen[edgeHash] {
			continue
		}
		seen[edgeHash] = true
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: parentHash,
			TargetHash: n.NodeHash,
			EdgeType:   edgetype.Contains,
			Confidence: 1.0,
			Provenance: "structural",
		})
		// Reverse: method -> type (enables sibling discovery via RWR)
		revHash := types.ComputeEdgeHash(n.NodeHash, parentHash, edgetype.MemberOf, "structural")
		seen[revHash] = true
		edges = append(edges, types.Edge{
			EdgeHash:   revHash,
			SourceHash: n.NodeHash,
			TargetHash: parentHash,
			EdgeType:   edgetype.MemberOf,
			Confidence: 1.0,
			Provenance: "structural",
		})
	}

	// Batch insert.
	if len(edges) > 0 {
		_ = s.BatchPutEdges(ctx, edges)
		// Invalidate the adjacency cache since we added new edges.
		// Without this, RWR uses the stale cache (built before contains edges)
		// and can't traverse the new structural connections.
		_ = s.DeleteNote(ctx, types.NewHash([]byte("adjacency_cache_v2")), "adjacency_cache")
	}
}

// ensureCoTestedEdges generates co_tested_with edges between non-test symbols
// that are referenced from the same test file. Skips if edges already exist.
func ensureCoTestedEdges(ctx stdctx.Context, s *store.SQLiteStore) {
	// Quick check: if co_tested_with edges already exist, skip.
	allNodes, err := s.NodesByName(ctx, "%")
	if err != nil || len(allNodes) == 0 {
		return
	}
	for _, n := range allNodes[:min(len(allNodes), 20)] {
		edges, _ := s.EdgesFrom(ctx, n.NodeHash, edgetype.CoTestedWith)
		if len(edges) > 0 {
			return // already generated
		}
	}

	// Only load edges from test-file nodes (much faster than loading all edges).
	// First identify test file hashes.
	testFileHashes := make(map[types.Hash]bool)
	for i := range allNodes {
		qn := allNodes[i].QualifiedName
		if idx := strings.Index(qn, "://"); idx >= 0 {
			if indexer.IsTestFile(qn[idx+3:]) {
				testFileHashes[allNodes[i].FileHash] = true
			}
		}
	}

	// Load edges only from nodes in test files.
	var testEdges []types.Edge
	for i := range allNodes {
		if !testFileHashes[allNodes[i].FileHash] {
			continue
		}
		edges, _ := s.EdgesFrom(ctx, allNodes[i].NodeHash, "")
		testEdges = append(testEdges, edges...)
	}

	coTestedEdges := indexer.GenerateCoTestedEdges(allNodes, testEdges)
	if len(coTestedEdges) > 0 {
		_ = s.BatchPutEdges(ctx, coTestedEdges)
		_ = s.DeleteNote(ctx, types.NewHash([]byte("adjacency_cache_v2")), "adjacency_cache")
	}
}
