package crosssystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/metrics"
	"gopkg.in/yaml.v3"
)

// TestCompounding measures task memory compounding across multiple passes.
// Each pass runs all tasks; task memory accumulates between passes.
// Pass 1 is truly cold (empty memory). Subsequent passes have memory
// from all prior passes.
//
// Usage:
//
//	BENCH_ADAPTERS=knowing BENCH_COMPOUND_ROUNDS=5 GOWORK=off go test ./bench/cross-system/ -run TestCompounding -v -timeout 0
//	BENCH_REPOS=django BENCH_COMPOUND_ROUNDS=5 GOWORK=off go test ./bench/cross-system/ -run TestCompounding -v -timeout 0
func TestCompounding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compounding test in short mode")
	}
	rounds := 5
	if r := os.Getenv("BENCH_COMPOUND_ROUNDS"); r != "" {
		fmt.Sscanf(r, "%d", &rounds)
	}

	tasks := loadCompoundingTasks(t)
	if len(tasks) == 0 {
		t.Skip("compounding test requires task fixtures")
	}

	repoFilter := os.Getenv("BENCH_REPOS")

	// Initialize adapter.
	adapter := adapters.NewKnowing()

	// Index all repos needed by tasks.
	corpusDir := "corpus"
	if cd := os.Getenv("BENCH_CORPUS"); cd != "" {
		corpusDir = cd
	}
	indexed := make(map[string]bool)
	repoPaths := make(map[string]string) // repo name -> absolute path
	for _, task := range tasks {
		if repoFilter != "" && task.Repo != repoFilter {
			continue
		}
		if indexed[task.Repo] {
			continue
		}
		repoPath := filepath.Join(corpusDir, "repos", task.Repo)
		if _, err := os.Stat(repoPath + "/.knowing/graph.db"); err != nil {
			continue
		}
		absPath, _ := filepath.Abs(repoPath)
		if _, err := adapter.Index(absPath); err != nil {
			t.Logf("Warning: failed to index %s: %v", task.Repo, err)
			continue
		}
		indexed[task.Repo] = true
		repoPaths[task.Repo] = absPath
	}

	// Enable memory and clear for clean start.
	adapter.EnableMemory()
	adapter.ClearAllMemory()

	t.Logf("Task memory enabled and cleared. Running %d rounds on %d tasks.", rounds, len(tasks))
	if repoFilter != "" {
		t.Logf("Repo filter: %s", repoFilter)
	}

	// Run rounds, accumulating memory.
	type roundResult struct {
		Round int
		P10   float64
		R10   float64
		MRR   float64
		Tasks int
	}
	var results []roundResult

	for round := 1; round <= rounds; round++ {
		var sumP10, sumR10, sumMRR float64
		taskCount := 0

		for _, task := range tasks {
			if repoFilter != "" && task.Repo != repoFilter {
				continue
			}
			rp, ok := repoPaths[task.Repo]
			if !ok {
				continue
			}

			result, err := adapter.Retrieve(rp, task, 5000)
			if err != nil {
				continue
			}

			m := metrics.Compute(result, task.GroundTruth)
			sumP10 += m.PrecisionAt10
			sumR10 += m.RecallAt10
			sumMRR += m.MRR
			taskCount++

			// Simulate agent usage: tell the implicit feedback tracker that
			// the ground truth symbols were "used" by the agent. This provides
			// the positive counterbalance to FlushUnused's negative signal.
			// Without this, all returned symbols get demoted (round 5 regression).
			// In real MCP usage, this happens when agents open files/edit code.
			adapter.SimulateAgentUsage(task.GroundTruth)
		}

		if taskCount == 0 {
			t.Fatal("No tasks completed")
		}

		r := roundResult{
			Round: round,
			P10:   sumP10 / float64(taskCount),
			R10:   sumR10 / float64(taskCount),
			MRR:   sumMRR / float64(taskCount),
			Tasks: taskCount,
		}
		results = append(results, r)

		t.Logf("Round %d: P@10=%.3f  R@10=%.3f  MRR=%.3f  Tasks=%d",
			round, r.P10, r.R10, r.MRR, r.Tasks)
	}

	// Print compounding summary.
	t.Logf("\n=== COMPOUNDING CURVE ===")
	t.Logf("| Round | P@10  | R@10  | MRR   | Delta P@10 |")
	t.Logf("|-------|-------|-------|-------|------------|")
	for i, r := range results {
		delta := 0.0
		if i > 0 {
			delta = r.P10 - results[0].P10
		}
		t.Logf("| %5d | %.3f | %.3f | %.3f | %+.4f     |", r.Round, r.P10, r.R10, r.MRR, delta)
	}

	baseline := results[0].P10
	final := results[len(results)-1].P10
	t.Logf("\nBaseline (round 1, cold): %.3f", baseline)
	t.Logf("Final (round %d): %.3f", rounds, final)
	if baseline > 0 {
		t.Logf("Total compounding: %+.4f (%+.1f%%)", final-baseline, (final-baseline)/baseline*100)
	}
}

func loadCompoundingTasks(t *testing.T) []benchtype.Task {
	t.Helper()
	dir := "corpus/tasks"
	if cd := os.Getenv("BENCH_CORPUS"); cd != "" {
		dir = filepath.Join(cd, "tasks")
	}

	var tasks []benchtype.Task
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
	return tasks
}
