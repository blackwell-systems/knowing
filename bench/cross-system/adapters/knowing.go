package adapters

import (
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
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

	result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: task.Description,
		TokenBudget:     tokenBudget,
		Format:          "json",
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
