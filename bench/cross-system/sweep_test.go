// Package crosssystem_test provides parameter sweep testing for P@10 optimization.
//
// Usage:
//
//	go test ./bench/cross-system/ -run TestParameterSweep -v -timeout 60m
//	BENCH_REPOS=flask,django go test ./bench/cross-system/ -run TestParameterSweep -v -timeout 30m
package crosssystem_test

import (
	stdctx "context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/metrics"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// SweepConfig holds one parameter configuration to test.
type SweepConfig struct {
	Name string

	// RWR parameters
	Alpha      float64 // restart probability (default 0.2)
	MaxIter    int     // iterations (default 20)
	ScoreCutoff float64 // min RWR score to include (default 0.02)

	// Seed selection
	MaxSeeds int     // max RWR seeds (default 15)
	RRFk     float64 // RRF constant (default 60)

	// Ranking weights (HITS mode)
	BlastW     float64 // blast radius weight (default 0.35)
	ConfW      float64 // confidence weight (default 0.20)
	RecencyW   float64 // recency weight (default 0.15)
	DistanceW  float64 // distance weight (default 0.15)

	// Test penalty
	TestPenalty float64 // multiplier for test files (default 0.3)
}

var defaultConfig = SweepConfig{
	Name:        "baseline",
	Alpha:       0.2,
	MaxIter:     20,
	ScoreCutoff: 0.02,
	MaxSeeds:    15,
	RRFk:        60,
	BlastW:      0.35,
	ConfW:       0.20,
	RecencyW:    0.15,
	DistanceW:   0.15,
	TestPenalty: 0.3,
}

// TestParameterSweep runs the retrieval pipeline with different parameter
// configurations and reports P@10 for each. This identifies which parameters
// have the most impact on retrieval quality.
func TestParameterSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("parameter sweep requires pre-indexed repos; run with -timeout 60m")
	}

	rawTasks := loadTasks(t, "corpus/tasks")
	tasks := filterAchievableGroundTruth(rawTasks, "corpus/repos")
	repoFilter := buildRepoFilter(t)

	// Filter tasks by repo.
	var filtered []benchtype.Task
	for _, task := range tasks {
		if repoAllowed(task.Repo, repoFilter) {
			filtered = append(filtered, task)
		}
	}
	tasks = filtered
	t.Logf("Sweeping %d tasks", len(tasks))

	// Open stores once.
	stores := make(map[string]*store.SQLiteStore)
	defer func() {
		for _, s := range stores {
			s.Close()
		}
	}()

	// Ensure contains edges are present (same as knowing adapter).
	for _, task := range tasks {
		if stores[task.Repo] != nil {
			continue
		}
		repoPath := filepath.Join("corpus", "repos", task.Repo)
		dbPath := filepath.Join(repoPath, ".knowing", "graph.db")
		s, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			continue
		}
		stores[task.Repo] = s
		ctx := stdctx.Background()
		adapters.EnsureContainsEdgesPublic(ctx, s)
	}

	// Define sweep configurations.
	configs := []SweepConfig{
		defaultConfig,

		// Alpha sweep (restart probability)
		{Name: "alpha=0.10", Alpha: 0.10, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "alpha=0.15", Alpha: 0.15, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "alpha=0.30", Alpha: 0.30, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "alpha=0.40", Alpha: 0.40, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},

		// MaxSeeds sweep
		{Name: "seeds=10", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 10, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "seeds=20", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 20, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "seeds=25", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 25, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "seeds=30", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 30, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},

		// RWR score cutoff sweep
		{Name: "cutoff=0.005", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.005, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "cutoff=0.01", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.01, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "cutoff=0.05", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.05, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "cutoff=0.10", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.10, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},

		// Ranking weight sweeps
		{Name: "blast=0.50", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.50, ConfW: 0.15, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "blast=0.20", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.20, ConfW: 0.25, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "distance=0.30", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.30, ConfW: 0.15, RecencyW: 0.10, DistanceW: 0.30, TestPenalty: 0.3},
		{Name: "distance=0.40", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.25, ConfW: 0.10, RecencyW: 0.10, DistanceW: 0.40, TestPenalty: 0.3},
		{Name: "conf=0.40", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.30, ConfW: 0.40, RecencyW: 0.10, DistanceW: 0.10, TestPenalty: 0.3},

		// RRF k sweep
		{Name: "rrfk=20", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 20, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "rrfk=40", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 40, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},
		{Name: "rrfk=100", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 100, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},

		// Test penalty sweep
		{Name: "testpen=0.0", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.0},
		{Name: "testpen=0.5", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.5},
		{Name: "testpen=0.7", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.7},

		// Combined: more seeds + lower cutoff
		{Name: "seeds=25+cutoff=0.01", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.01, MaxSeeds: 25, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},

		// Combined: higher alpha + more seeds (explore more broadly)
		{Name: "alpha=0.3+seeds=25", Alpha: 0.3, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 25, RRFk: 60, BlastW: 0.35, ConfW: 0.20, RecencyW: 0.15, DistanceW: 0.15, TestPenalty: 0.3},

		// Distance-dominated ranking (seeds are most relevant, walk less)
		{Name: "seedheavy", Alpha: 0.2, MaxIter: 20, ScoreCutoff: 0.02, MaxSeeds: 15, RRFk: 60, BlastW: 0.15, ConfW: 0.10, RecencyW: 0.05, DistanceW: 0.50, TestPenalty: 0.3},
	}

	type result struct {
		Config string
		P10    float64
		R10    float64
		MRR    float64
	}
	var results []result

	for _, cfg := range configs {
		// Apply config via exported setters.
		knowingctx.SetSweepParams(knowingctx.SweepParams{
			Alpha:       cfg.Alpha,
			MaxIter:     cfg.MaxIter,
			ScoreCutoff: cfg.ScoreCutoff,
			MaxSeeds:    cfg.MaxSeeds,
			RRFk:        cfg.RRFk,
			BlastW:      cfg.BlastW,
			ConfW:       cfg.ConfW,
			RecencyW:    cfg.RecencyW,
			DistanceW:   cfg.DistanceW,
			TestPenalty: cfg.TestPenalty,
		})

		var allMetrics []benchtype.MetricResult
		for _, task := range tasks {
			s := stores[task.Repo]
			if s == nil {
				continue
			}
			ctx := stdctx.Background()
			engine := knowingctx.NewContextEngine(s)

			var repoURL string
			if repos, err := s.AllRepos(ctx); err == nil && len(repos) > 0 {
				repoURL = repos[0].RepoURL
			}

			res, err := engine.ForTask(ctx, knowingctx.TaskOptions{
				TaskDescription: task.Description,
				TokenBudget:     5000,
				Format:          "json",
				RepoURL:         repoURL,
			})
			if err != nil || res == nil {
				continue
			}

			retrieved := make([]benchtype.RetrievedSymbol, len(res.Symbols))
			for i, sym := range res.Symbols {
				retrieved[i] = benchtype.RetrievedSymbol{
					QualifiedName: sym.Node.QualifiedName,
					Score:         sym.Score,
					Rank:          i + 1,
				}
			}

			metric := metrics.Compute(benchtype.RetrievalResult{
				System:  "knowing",
				TaskID:  task.ID,
				Symbols: retrieved,
			}, task.GroundTruth)
			allMetrics = append(allMetrics, metric)
		}

		agg := metrics.Aggregate(allMetrics, "knowing")
		results = append(results, result{
			Config: cfg.Name,
			P10:    agg.MeanPrecisionAt10,
			R10:    agg.MeanRecallAt10,
			MRR:    agg.MeanMRR,
		})

		t.Logf("  %s: P@10=%.3f R@10=%.3f MRR=%.3f", cfg.Name, agg.MeanPrecisionAt10, agg.MeanRecallAt10, agg.MeanMRR)
	}

	// Reset to defaults.
	knowingctx.SetSweepParams(knowingctx.SweepParams{})

	// Sort by P@10 descending and report.
	sort.Slice(results, func(i, j int) bool { return results[i].P10 > results[j].P10 })

	t.Log("\n=== PARAMETER SWEEP RESULTS (sorted by P@10) ===\n")
	t.Log("| Config | P@10 | R@10 | MRR |")
	t.Log("|--------|------|------|-----|")
	for _, r := range results {
		marker := ""
		if r.P10 > results[len(results)-1].P10+0.005 {
			marker = " *"
		}
		t.Logf("| %s | %.3f | %.3f | %.3f |%s", r.Config, r.P10, r.R10, r.MRR, marker)
	}

	best := results[0]
	baseline := results[len(results)-1]
	for _, r := range results {
		if r.Config == "baseline" {
			baseline = r
			break
		}
	}
	t.Logf("\nBest: %s (P@10=%.3f, delta=+%.3f from baseline %.3f)",
		best.Config, best.P10, best.P10-baseline.P10, baseline.P10)
}
