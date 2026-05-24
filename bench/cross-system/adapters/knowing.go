package adapters

import (
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
	"github.com/blackwell-systems/knowing/internal/community"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"

	stdctx "context"
)

// Knowing implements benchtype.Adapter for knowing's context engine.
type Knowing struct {
	stores  map[string]*store.SQLiteStore
	memories map[string]*knowingctx.TaskMemory // per-repo task memory for compounding tests
}

func NewKnowing() *Knowing {
	return &Knowing{
		stores:   make(map[string]*store.SQLiteStore),
		memories: make(map[string]*knowingctx.TaskMemory),
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

	// Run community detection if not already computed. This enables
	// community-aware RWR filtering in the retrieval pipeline.
	ctx := stdctx.Background()
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

	return time.Since(start).Milliseconds(), nil
}

func (a *Knowing) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	s, ok := a.stores[repoPath]
	if !ok {
		return benchtype.RetrievalResult{System: "knowing", TaskID: task.ID, Error: "repo not indexed"}, nil
	}

	ctx := stdctx.Background()
	start := time.Now()

	engine := knowingctx.NewContextEngine(s)

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
