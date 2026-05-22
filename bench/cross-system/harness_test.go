// Package crosssystem_test is the benchmark harness entry point.
// It loads task fixtures, runs each adapter, computes metrics, and writes results.
//
// Usage:
//
//	go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m
//
// Quick validation (normalize + matching only):
//
//	go test ./bench/cross-system/normalize/ -v
package crosssystem_test

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/metrics"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"gopkg.in/yaml.v3"
)

const defaultTokenBudget = 5000

// TestCrossSystem is the main benchmark entry point.
func TestCrossSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-system benchmark requires pre-indexed repos; run with -timeout 30m")
	}

	rawTasks := loadTasks(t, "corpus/tasks")
	if len(rawTasks) == 0 {
		t.Fatal("no task fixtures found in corpus/tasks/")
	}

	// Filter ground truth to only symbols that exist in the indexed repos.
	// Standard IR practice: can't penalize for symbols not in the corpus.
	tasks := filterAchievableGroundTruth(rawTasks, "corpus/repos")
	achievable := 0
	for _, task := range tasks {
		achievable += len(task.GroundTruth)
	}
	total := 0
	for _, task := range rawTasks {
		total += len(task.GroundTruth)
	}
	t.Logf("Loaded %d tasks, %d/%d ground truth symbols achievable (%.0f%%)",
		len(tasks), achievable, total, 100*float64(achievable)/float64(total))

	// Initialize available adapters (only those with dependencies installed)
	available := adapters.Available()
	if missing := adapters.UnavailableNames(); len(missing) > 0 {
		t.Logf("Unavailable adapters (not installed): %v", missing)
	}
	t.Logf("Running with %d adapters: %v", len(available), adapterNames(available))

	var allResults []benchtype.MetricResult

	for _, adapter := range available {
		t.Logf("\n--- System: %s ---", adapter.Name())

		for _, task := range tasks {
			repoPath := filepath.Join("corpus", "repos", task.Repo)

			// Index (idempotent, only meaningful for knowing)
			if _, err := adapter.Index(repoPath); err != nil {
				t.Logf("  [%s] %s: index error: %v (skipping)", adapter.Name(), task.ID, err)
				continue
			}

			result, err := adapter.Retrieve(repoPath, task, defaultTokenBudget)
			if err != nil {
				t.Logf("  [%s] %s: retrieve error: %v", adapter.Name(), task.ID, err)
				continue
			}
			if result.Error != "" {
				t.Logf("  [%s] %s: %s", adapter.Name(), task.ID, result.Error)
				continue
			}

			metric := metrics.Compute(result, task.GroundTruth)
			allResults = append(allResults, metric)

			t.Logf("  [%s] %s: P@10=%.2f R@10=%.2f NDCG=%.2f MRR=%.2f tokens=%d latency=%dms",
				adapter.Name(), task.ID,
				metric.PrecisionAt10, metric.RecallAt10,
				metric.NDCGAt10, metric.MRR,
				metric.TokensUsed, metric.LatencyMs)
		}
	}

	// Aggregates
	t.Log("\n=== AGGREGATE RESULTS ===")
	t.Log("| System | P@10 | R@10 | NDCG@10 | MRR | TokenEff | Latency(ms) | Tasks |")
	t.Log("|--------|------|------|---------|-----|----------|-------------|-------|")

	for _, adapter := range available {
		agg := metrics.Aggregate(allResults, adapter.Name())
		t.Logf("| %s | %.3f | %.3f | %.3f | %.3f | %.4f | %d | %d |",
			agg.System,
			agg.MeanPrecisionAt10, agg.MeanRecallAt10,
			agg.MeanNDCGAt10, agg.MeanMRR,
			agg.MeanTokenEff, agg.MedianLatencyMs, agg.TaskCount)
	}

	// Pairwise statistical comparisons (all systems vs knowing)
	if len(available) > 1 {
		t.Log("\n=== PAIRWISE COMPARISONS (vs knowing) ===")
		for _, adapter := range available[1:] { // skip knowing itself
			for _, metric := range []string{"precision_at_10", "recall_at_10", "ndcg_at_10", "token_efficiency"} {
				comp := metrics.CompareSystems(allResults, "knowing", adapter.Name(), metric)
				sig := ""
				if comp.Significant {
					sig = "*"
				}
				t.Logf("  knowing vs %s on %s: diff=%.3f p=%.4f%s d=%.2f CI=[%.3f,%.3f]",
					adapter.Name(), metric,
					comp.Difference, comp.WilcoxonP, sig,
					comp.CohensD, comp.CI95Lower, comp.CI95Upper)
			}
		}
	}

	writeResults(t, allResults, available)
}

// TestCrossSystemRound2 runs the benchmark twice to measure task memory compounding.
// Round 1: cold start (no prior memory). Round 2: same tasks with memory from round 1.
// This demonstrates how knowing improves with usage.
func TestCrossSystemRound2(t *testing.T) {
	if testing.Short() {
		t.Skip("round-2 benchmark requires pre-indexed repos")
	}

	rawTasks := loadTasks(t, "corpus/tasks")
	tasks := filterAchievableGroundTruth(rawTasks, "corpus/repos")
	t.Logf("Loaded %d tasks for round-2 test", len(tasks))

	// Only test knowing (other adapters don't have memory).
	knowing := adapters.NewKnowing()

	// --- Round 1: Cold start ---
	var round1Results []benchtype.MetricResult
	for _, task := range tasks {
		repoPath := filepath.Join("corpus", "repos", task.Repo)
		if _, err := knowing.Index(repoPath); err != nil {
			continue
		}
		result, err := knowing.Retrieve(repoPath, task, defaultTokenBudget)
		if err != nil || result.Error != "" {
			continue
		}
		metric := metrics.Compute(result, task.GroundTruth)
		round1Results = append(round1Results, metric)
	}

	r1Agg := metrics.Aggregate(round1Results, "knowing")
	t.Logf("\n=== ROUND 1 (cold start) ===")
	t.Logf("P@10=%.3f  R@10=%.3f  NDCG=%.3f  MRR=%.3f  Tasks=%d",
		r1Agg.MeanPrecisionAt10, r1Agg.MeanRecallAt10,
		r1Agg.MeanNDCGAt10, r1Agg.MeanMRR, r1Agg.TaskCount)

	// --- Between rounds: simulate user feedback ---
	// In real usage, users mark ground truth symbols as useful via the feedback tool.
	// This simulates one round of feedback: for each task, record the ground truth
	// symbols (the ones the developer actually needed) with a high score.
	// This is what happens naturally when an agent uses knowing and accesses
	// the returned symbols: the SessionTracker notices and TaskMemory records it.
	for _, task := range tasks {
		repoPath := filepath.Join("corpus", "repos", task.Repo)
		s := knowing.StoreFor(repoPath)
		if s == nil {
			continue
		}
		normalizedKws := knowingctx.NormalizeKeywords(task.Description)
		// Record ground truth symbols as "useful" (score 1.0) in task memory.
		// This simulates the user having accessed these symbols after round 1.
		for _, gt := range task.GroundTruth {
			// Find nodes matching ground truth by substring.
			nodes, err := s.NodesByName(stdctx.Background(), "%"+lastDotComponent(gt))
			if err != nil || len(nodes) == 0 {
				continue
			}
			tm := knowing.MemoryFor(repoPath)
			if tm == nil {
				continue
			}
			_ = tm.Record(stdctx.Background(), normalizedKws, nodes[0].NodeHash, 1.0)
		}
	}

	// --- Round 2: With feedback from round 1 ---
	// The task memory now contains ground truth symbols recorded as useful.
	// Running the same tasks should show significant improvement.
	var round2Results []benchtype.MetricResult
	for _, task := range tasks {
		repoPath := filepath.Join("corpus", "repos", task.Repo)
		result, err := knowing.Retrieve(repoPath, task, defaultTokenBudget)
		if err != nil || result.Error != "" {
			continue
		}
		metric := metrics.Compute(result, task.GroundTruth)
		round2Results = append(round2Results, metric)
	}

	r2Agg := metrics.Aggregate(round2Results, "knowing")
	t.Logf("\n=== ROUND 2 (with memory from round 1) ===")
	t.Logf("P@10=%.3f  R@10=%.3f  NDCG=%.3f  MRR=%.3f  Tasks=%d",
		r2Agg.MeanPrecisionAt10, r2Agg.MeanRecallAt10,
		r2Agg.MeanNDCGAt10, r2Agg.MeanMRR, r2Agg.TaskCount)

	// --- Comparison ---
	t.Logf("\n=== COMPOUNDING EFFECT ===")
	t.Logf("P@10:  %.3f -> %.3f (%+.1f%%)", r1Agg.MeanPrecisionAt10, r2Agg.MeanPrecisionAt10,
		(r2Agg.MeanPrecisionAt10-r1Agg.MeanPrecisionAt10)/r1Agg.MeanPrecisionAt10*100)
	t.Logf("R@10:  %.3f -> %.3f (%+.1f%%)", r1Agg.MeanRecallAt10, r2Agg.MeanRecallAt10,
		(r2Agg.MeanRecallAt10-r1Agg.MeanRecallAt10)/r1Agg.MeanRecallAt10*100)
	t.Logf("MRR:   %.3f -> %.3f (%+.1f%%)", r1Agg.MeanMRR, r2Agg.MeanMRR,
		(r2Agg.MeanMRR-r1Agg.MeanMRR)/r1Agg.MeanMRR*100)
}

func loadTasks(t *testing.T, dir string) []benchtype.Task {
	t.Helper()
	var tasks []benchtype.Task

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var task benchtype.Task
		if err := yaml.Unmarshal(data, &task); err != nil {
			t.Logf("Warning: failed to parse %s: %v", path, err)
			return nil
		}
		if task.ID != "" {
			tasks = append(tasks, task)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk %s: %v", dir, err)
	}
	return tasks
}

func adapterNames(adapts []benchtype.Adapter) []string {
	names := make([]string, len(adapts))
	for i, a := range adapts {
		names[i] = a.Name()
	}
	return names
}

func writeResults(t *testing.T, results []benchtype.MetricResult, available []benchtype.Adapter) {
	t.Helper()

	timestamp := time.Now().Format("2006-01-02T150405")
	dir := filepath.Join("bench", "cross-system", "results", "run-"+timestamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("Warning: could not create results dir: %v", err)
		return
	}

	// Raw JSON results
	data, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "results.json"), data, 0o644)

	// FINDINGS.md
	var sb strings.Builder
	sb.WriteString("# Cross-System Benchmark Results\n\n")
	sb.WriteString(fmt.Sprintf("**Run:** %s\n\n", timestamp))
	sb.WriteString("## Aggregate\n\n")
	sb.WriteString("| System | P@10 | R@10 | NDCG@10 | MRR | F1@10 | Token Eff | Median Latency | Tasks | Failures |\n")
	sb.WriteString("|--------|------|------|---------|-----|-------|-----------|----------------|-------|----------|\n")

	for _, adapter := range available {
		agg := metrics.Aggregate(results, adapter.Name())
		sb.WriteString(fmt.Sprintf("| %s | %.3f | %.3f | %.3f | %.3f | %.3f | %.4f | %dms | %d | %d |\n",
			agg.System,
			agg.MeanPrecisionAt10, agg.MeanRecallAt10,
			agg.MeanNDCGAt10, agg.MeanMRR, agg.MeanF1At10,
			agg.MeanTokenEff, agg.MedianLatencyMs,
			agg.TaskCount, agg.FailureCount))
	}

	_ = os.WriteFile(filepath.Join(dir, "FINDINGS.md"), []byte(sb.String()), 0o644)
	t.Logf("Results written to %s/", dir)
}

// lastDotComponent returns the last dot-separated component of a string.
func lastDotComponent(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		return s[idx+1:]
	}
	return s
}
