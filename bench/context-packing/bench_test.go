// Package context_packing benchmarks packing strategies: given the same set of
// ranked candidate symbols, which strategy produces the best context within a
// token budget?
//
// This isolates the packing layer from the retrieval layer. The cross-system
// benchmark measures retrieval quality (who finds the right symbols). This
// benchmark measures packing quality (given candidates, who assembles the best
// context for an agent).
//
// Strategies compared:
//   1. Density-ranked (knowing's packIntoBudget): score/tokens * proximity
//   2. Top-K by score: take highest-scored symbols until budget exhausted
//   3. File-grouped: group by file, pack densest files first
//   4. Random: shuffle and pack (baseline)
//
// Metrics:
//   - GT Coverage: fraction of ground truth symbols included in the pack
//   - Token Utilization: fraction of budget used (higher = less waste)
//   - File Coherence: fraction of packed symbols that share a file with another packed symbol
//   - Density: GT coverage per 1000 tokens
//
// Usage:
//
//	BENCH_REPOS=django GOWORK=off go test ./bench/context-packing/ -v -timeout 30m
//	GOWORK=off go test ./bench/context-packing/ -v -timeout 0
package context_packing

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"

	stdctx "context"

	"gopkg.in/yaml.v3"
)

type packResult struct {
	Strategy       string
	GTCoverage     float64 // fraction of ground truth symbols in pack
	TokenUtil      float64 // fraction of budget used
	FileCoherence  float64 // fraction of symbols sharing a file with another
	Density        float64 // GT coverage per 1000 tokens
	SymbolsPacked  int
	TokensUsed     int
}

func TestContextPacking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping packing benchmark in short mode")
	}

	tasks := loadTasks(t)
	if len(tasks) == 0 {
		t.Skip("no task fixtures found")
	}

	repoFilter := os.Getenv("BENCH_REPOS")
	corpusDir := "corpus"
	if cd := os.Getenv("BENCH_CORPUS"); cd != "" {
		corpusDir = cd
	}
	// Also check cross-system corpus path.
	if _, err := os.Stat(corpusDir); err != nil {
		corpusDir = "../cross-system/corpus"
	}

	stores := make(map[string]*store.SQLiteStore)
	repoPaths := make(map[string]string)
	budget := 5000

	// Index repos.
	for _, task := range tasks {
		if repoFilter != "" && task.Repo != repoFilter {
			continue
		}
		if stores[task.Repo] != nil {
			continue
		}
		repoPath := filepath.Join(corpusDir, "repos", task.Repo)
		dbPath := repoPath + "/.knowing/graph.db"
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		absPath, _ := filepath.Abs(repoPath)
		s, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			t.Logf("Warning: failed to open %s: %v", task.Repo, err)
			continue
		}
		stores[task.Repo] = s
		repoPaths[task.Repo] = absPath
	}
	defer func() {
		for _, s := range stores {
			s.Close()
		}
	}()

	// Aggregate results per strategy.
	type stratAgg struct {
		sumGT, sumUtil, sumCoherence, sumDensity float64
		count                                     int
	}
	aggs := map[string]*stratAgg{
		"density-ranked": {},
		"top-k-score":    {},
		"file-grouped":   {},
		"random":         {},
	}

	taskCount := 0
	for _, task := range tasks {
		if repoFilter != "" && task.Repo != repoFilter {
			continue
		}
		s, ok := stores[task.Repo]
		if !ok {
			continue
		}

		ctx := stdctx.Background()

		// Get ranked candidates from knowing's retrieval pipeline.
		engine := knowingctx.NewContextEngine(s)
		engine.DisablePersistentCache()

		var repoURL string
		if repos, err := s.AllRepos(ctx); err == nil && len(repos) > 0 {
			repoURL = repos[0].RepoURL
		}

		result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.Description,
			TokenBudget:     50000, // large budget to get all candidates
			RepoURL:         repoURL,
		})
		if err != nil || len(result.Symbols) == 0 {
			continue
		}

		// Build ranked symbols list and ground truth set.
		ranked := result.Symbols
		gtSet := make(map[string]bool, len(task.GroundTruth))
		for _, gt := range task.GroundTruth {
			gtSet[strings.ToLower(gt)] = true
		}

		// Run each strategy.
		results := []packResult{
			packDensityRanked(ranked, budget, gtSet),
			packTopKScore(ranked, budget, gtSet),
			packFileGrouped(ranked, budget, gtSet),
			packRandom(ranked, budget, gtSet),
		}

		for _, r := range results {
			a := aggs[r.Strategy]
			a.sumGT += r.GTCoverage
			a.sumUtil += r.TokenUtil
			a.sumCoherence += r.FileCoherence
			a.sumDensity += r.Density
			a.count++
		}
		taskCount++
	}

	if taskCount == 0 {
		t.Fatal("no tasks completed")
	}

	// Report.
	t.Logf("\n=== CONTEXT PACKING BENCHMARK ===")
	t.Logf("Tasks: %d  Budget: %d tokens\n", taskCount, budget)
	t.Logf("| Strategy | GT Coverage | Token Util | File Coherence | Density (GT/1K tok) |")
	t.Logf("|----------|-------------|------------|----------------|---------------------|")

	strategies := []string{"density-ranked", "top-k-score", "file-grouped", "random"}
	for _, name := range strategies {
		a := aggs[name]
		if a.count == 0 {
			continue
		}
		n := float64(a.count)
		t.Logf("| %-14s | %.3f       | %.3f      | %.3f           | %.3f               |",
			name, a.sumGT/n, a.sumUtil/n, a.sumCoherence/n, a.sumDensity/n)
	}

	// Pairwise comparison: density-ranked vs each other.
	baseline := aggs["density-ranked"]
	if baseline.count > 0 {
		t.Log("\nDensity-ranked vs others:")
		bn := float64(baseline.count)
		for _, name := range strategies[1:] {
			a := aggs[name]
			if a.count == 0 {
				continue
			}
			n := float64(a.count)
			gtDelta := (baseline.sumGT/bn - a.sumGT/n)
			t.Logf("  vs %s: GT coverage %+.3f, density %+.3f",
				name, gtDelta, baseline.sumDensity/bn-a.sumDensity/n)
		}
	}

	fmt.Fprintf(os.Stderr, "\n[context-packing] %d tasks, budget=%d\n", taskCount, budget)
}

// packDensityRanked uses knowing's production packing algorithm.
func packDensityRanked(ranked []knowingctx.RankedSymbol, budget int, gtSet map[string]bool) packResult {
	block := knowingctx.PackIntoBudget(ranked, budget, "json")
	return evalPack("density-ranked", block.Symbols, budget, block.TokensUsed, gtSet)
}

// packTopKScore takes symbols in score order until budget is exhausted.
func packTopKScore(ranked []knowingctx.RankedSymbol, budget int, gtSet map[string]bool) packResult {
	// ranked is already sorted by score descending.
	var packed []knowingctx.RankedSymbol
	tokensUsed := 0
	for _, sym := range ranked {
		cost := knowingctx.EstimateNodeTokensForFormat(sym.Node, "json")
		if cost < 1 {
			cost = 1
		}
		if tokensUsed+cost > budget {
			continue
		}
		tokensUsed += cost
		packed = append(packed, sym)
	}
	return evalPack("top-k-score", packed, budget, tokensUsed, gtSet)
}

// packFileGrouped groups symbols by file, ranks files by total score, packs densest files first.
func packFileGrouped(ranked []knowingctx.RankedSymbol, budget int, gtSet map[string]bool) packResult {
	// Group by file.
	type fileGroup struct {
		fileHash types.Hash
		symbols  []knowingctx.RankedSymbol
		totalScore float64
		totalCost  int
	}
	groups := make(map[types.Hash]*fileGroup)
	var order []types.Hash
	for _, sym := range ranked {
		fh := sym.Node.FileHash
		g, ok := groups[fh]
		if !ok {
			g = &fileGroup{fileHash: fh}
			groups[fh] = g
			order = append(order, fh)
		}
		cost := knowingctx.EstimateNodeTokensForFormat(sym.Node, "json")
		if cost < 1 {
			cost = 1
		}
		g.symbols = append(g.symbols, sym)
		g.totalScore += sym.Score
		g.totalCost += cost
	}

	// Sort files by density (score/cost) descending.
	sort.Slice(order, func(i, j int) bool {
		gi, gj := groups[order[i]], groups[order[j]]
		di := gi.totalScore / float64(gi.totalCost)
		dj := gj.totalScore / float64(gj.totalCost)
		return di > dj
	})

	// Pack whole files greedily.
	var packed []knowingctx.RankedSymbol
	tokensUsed := 0
	for _, fh := range order {
		g := groups[fh]
		if tokensUsed+g.totalCost > budget {
			// Try individual symbols from this file.
			for _, sym := range g.symbols {
				cost := knowingctx.EstimateNodeTokensForFormat(sym.Node, "json")
				if cost < 1 {
					cost = 1
				}
				if tokensUsed+cost <= budget {
					tokensUsed += cost
					packed = append(packed, sym)
				}
			}
			continue
		}
		tokensUsed += g.totalCost
		packed = append(packed, g.symbols...)
	}
	return evalPack("file-grouped", packed, budget, tokensUsed, gtSet)
}

// packRandom shuffles and packs until budget is exhausted.
func packRandom(ranked []knowingctx.RankedSymbol, budget int, gtSet map[string]bool) packResult {
	shuffled := make([]knowingctx.RankedSymbol, len(ranked))
	copy(shuffled, ranked)
	rng := rand.New(rand.NewSource(42)) // deterministic for reproducibility
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	var packed []knowingctx.RankedSymbol
	tokensUsed := 0
	for _, sym := range shuffled {
		cost := knowingctx.EstimateNodeTokensForFormat(sym.Node, "json")
		if cost < 1 {
			cost = 1
		}
		if tokensUsed+cost > budget {
			continue
		}
		tokensUsed += cost
		packed = append(packed, sym)
	}
	return evalPack("random", packed, budget, tokensUsed, gtSet)
}

// evalPack computes metrics for a packed set of symbols.
func evalPack(strategy string, packed []knowingctx.RankedSymbol, budget, tokensUsed int, gtSet map[string]bool) packResult {
	if len(packed) == 0 {
		return packResult{Strategy: strategy}
	}

	// GT coverage: how many ground truth symbols are in the pack?
	gtHits := 0
	for _, sym := range packed {
		qn := strings.ToLower(sym.Node.QualifiedName)
		for gt := range gtSet {
			if strings.Contains(qn, gt) || strings.Contains(gt, qn) {
				gtHits++
				break
			}
		}
	}
	gtCoverage := 0.0
	if len(gtSet) > 0 {
		gtCoverage = float64(gtHits) / float64(len(gtSet))
	}

	// Token utilization.
	tokenUtil := float64(tokensUsed) / float64(budget)

	// File coherence: fraction of symbols that share a file with at least one other.
	fileCounts := make(map[types.Hash]int)
	for _, sym := range packed {
		fileCounts[sym.Node.FileHash]++
	}
	coherent := 0
	for _, sym := range packed {
		if fileCounts[sym.Node.FileHash] > 1 {
			coherent++
		}
	}
	fileCoherence := float64(coherent) / float64(len(packed))

	// Density: GT coverage per 1000 tokens.
	density := 0.0
	if tokensUsed > 0 {
		density = gtCoverage / (float64(tokensUsed) / 1000.0)
	}

	return packResult{
		Strategy:      strategy,
		GTCoverage:    gtCoverage,
		TokenUtil:     tokenUtil,
		FileCoherence: fileCoherence,
		Density:       density,
		SymbolsPacked: len(packed),
		TokensUsed:    tokensUsed,
	}
}

func loadTasks(t *testing.T) []benchtype.Task {
	t.Helper()
	// Try cross-system corpus tasks.
	dirs := []string{"corpus/tasks", "../cross-system/corpus/tasks"}
	if cd := os.Getenv("BENCH_CORPUS"); cd != "" {
		dirs = []string{filepath.Join(cd, "tasks")}
	}

	var tasks []benchtype.Task
	for _, dir := range dirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var task benchtype.Task
			if err := yaml.Unmarshal(data, &task); err != nil {
				return nil
			}
			if task.ID != "" {
				tasks = append(tasks, task)
			}
			return nil
		})
		if len(tasks) > 0 {
			break
		}
	}
	return tasks
}
