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
	"gopkg.in/yaml.v3"
)

const defaultTokenBudget = 5000

// TestCrossSystem is the main benchmark entry point.
func TestCrossSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-system benchmark requires pre-indexed repos; run with -timeout 30m")
	}

	tasks := loadTasks(t, "bench/cross-system/corpus/tasks")
	if len(tasks) == 0 {
		t.Fatal("no task fixtures found in corpus/tasks/")
	}
	t.Logf("Loaded %d task fixtures", len(tasks))

	// Initialize available adapters
	available := []benchtype.Adapter{
		adapters.NewKnowing(),
		adapters.NewGrep(),
	}

	var allResults []benchtype.MetricResult

	for _, adapter := range available {
		t.Logf("\n--- System: %s ---", adapter.Name())

		for _, task := range tasks {
			repoPath := filepath.Join("bench", "cross-system", "corpus", "repos", task.Repo)

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

	writeResults(t, allResults, available)
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
