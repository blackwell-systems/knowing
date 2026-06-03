// Package delta_packing benchmarks delta context encoding: given sequential
// context_for_task calls within a session, how much token savings does delta
// encoding provide over full retransmission?
//
// This simulates a real agent session: multiple tasks against the same repo,
// each producing a context pack. For every consecutive pair, we compute the
// delta and measure token savings, symbol overlap, and delta frequency.
//
// Usage:
//
//	BENCH_REPOS=django GOWORK=off go test ./bench/delta-packing/ -v -timeout 30m
//	GOWORK=off go test ./bench/delta-packing/ -v -timeout 0
package delta_packing

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"

	stdctx "context"

	"gopkg.in/yaml.v3"
)

type deltaResult struct {
	Repo        string
	TaskA       string
	TaskB       string
	FullTokens  int
	DeltaTokens int
	Savings     float64 // percent
	Overlap     float64 // percent
	Outcome     string  // "delta", "unchanged", "full"
}

type repoSummary struct {
	Repo       string
	Tasks      int
	Unchanged  int
	Delta      int
	Full       int
	AvgSavings float64
	AvgOverlap float64
}

func TestDeltaPacking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping delta packing benchmark in short mode")
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
	if _, err := os.Stat(corpusDir); err != nil {
		corpusDir = "../cross-system/corpus"
	}

	budget := 5000

	// Group tasks by repo.
	repoTasks := make(map[string][]benchtype.Task)
	for _, task := range tasks {
		if repoFilter != "" && task.Repo != repoFilter {
			continue
		}
		repoTasks[task.Repo] = append(repoTasks[task.Repo], task)
	}

	// Open stores.
	stores := make(map[string]*store.SQLiteStore)
	for repo := range repoTasks {
		repoPath := filepath.Join(corpusDir, "repos", repo)
		dbPath := repoPath + "/.knowing/graph.db"
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		s, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			t.Logf("Warning: failed to open %s: %v", repo, err)
			continue
		}
		stores[repo] = s
	}
	defer func() {
		for _, s := range stores {
			s.Close()
		}
	}()

	var allResults []deltaResult
	var summaries []repoSummary

	// Sort repo names for deterministic output.
	repoNames := make([]string, 0, len(repoTasks))
	for repo := range repoTasks {
		if stores[repo] != nil {
			repoNames = append(repoNames, repo)
		}
	}
	sort.Strings(repoNames)

	for _, repo := range repoNames {
		repoTaskList := repoTasks[repo]
		s := stores[repo]
		ctx := stdctx.Background()

		var repoURL string
		if repos, err := s.AllRepos(ctx); err == nil && len(repos) > 0 {
			repoURL = repos[0].RepoURL
		}

		// Run all tasks sequentially, collecting context blocks.
		var blocks []*knowingctx.ContextBlock
		var taskDescs []string
		for _, task := range repoTaskList {
			engine := knowingctx.NewContextEngine(s)
			engine.DisablePersistentCache()
			block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
				TaskDescription: task.Description,
				TokenBudget:     budget,
				RepoURL:         repoURL,
				Format:          "gcf",
			})
			if err != nil || len(block.Symbols) == 0 {
				continue
			}
			block.PackRoot = knowingctx.ComputePackRootExported(task.Description, block.Symbols)
			blocks = append(blocks, block)
			taskDescs = append(taskDescs, task.ID)
		}

		if len(blocks) < 2 {
			continue
		}

		summary := repoSummary{Repo: repo, Tasks: len(blocks)}
		var totalSavings, totalOverlap float64
		deltaCount := 0

		// Compare consecutive pairs.
		for i := 1; i < len(blocks); i++ {
			prior := blocks[i-1]
			current := blocks[i]

			var result deltaResult
			result.Repo = repo
			result.TaskA = taskDescs[i-1]
			result.TaskB = taskDescs[i]

			if prior.PackRoot == current.PackRoot {
				result.Outcome = "unchanged"
				result.Savings = 100.0
				result.Overlap = 100.0
				result.FullTokens = current.TokensUsed
				summary.Unchanged++
			} else {
				delta := knowingctx.DiffPacks(prior, current, "gcf")
				result.FullTokens = delta.FullTokens
				result.DeltaTokens = delta.DeltaTokens
				result.Savings = delta.SavingsPercent()
				result.Overlap = delta.SymbolOverlapPercent()

				if delta.IsWorthIt() {
					result.Outcome = "delta"
					summary.Delta++
					totalSavings += result.Savings
					totalOverlap += result.Overlap
					deltaCount++
				} else {
					result.Outcome = "full"
					summary.Full++
				}
			}

			allResults = append(allResults, result)
		}

		if deltaCount > 0 {
			summary.AvgSavings = totalSavings / float64(deltaCount)
			summary.AvgOverlap = totalOverlap / float64(deltaCount)
		}
		summaries = append(summaries, summary)
	}

	if len(allResults) == 0 {
		t.Fatal("no consecutive task pairs found")
	}

	// Print per-repo summary table.
	t.Log("")
	t.Log("=== Delta Packing Benchmark ===")
	t.Log("")
	t.Logf("%-14s %5s %10s %7s %7s %11s %11s",
		"Repo", "Tasks", "Unchanged", "Delta", "Full", "Avg Savings", "Avg Overlap")
	t.Logf("%-14s %5s %10s %7s %7s %11s %11s",
		"----", "-----", "---------", "-----", "----", "-----------", "-----------")

	var aggUnchanged, aggDelta, aggFull int
	var aggSavings, aggOverlap float64
	var aggDeltaCount int

	for _, s := range summaries {
		pairs := s.Unchanged + s.Delta + s.Full
		unchangedPct := 100.0 * float64(s.Unchanged) / float64(pairs)
		deltaPct := 100.0 * float64(s.Delta) / float64(pairs)
		fullPct := 100.0 * float64(s.Full) / float64(pairs)

		savingsStr := "n/a"
		overlapStr := "n/a"
		if s.Delta > 0 {
			savingsStr = fmt.Sprintf("%.1f%%", s.AvgSavings)
			overlapStr = fmt.Sprintf("%.1f%%", s.AvgOverlap)
		}

		t.Logf("%-14s %5d %7.0f%% (%d) %4.0f%% (%d) %4.0f%% (%d) %11s %11s",
			s.Repo, s.Tasks,
			unchangedPct, s.Unchanged,
			deltaPct, s.Delta,
			fullPct, s.Full,
			savingsStr, overlapStr)

		aggUnchanged += s.Unchanged
		aggDelta += s.Delta
		aggFull += s.Full
		if s.Delta > 0 {
			aggSavings += s.AvgSavings * float64(s.Delta)
			aggOverlap += s.AvgOverlap * float64(s.Delta)
			aggDeltaCount += s.Delta
		}
	}

	totalPairs := aggUnchanged + aggDelta + aggFull
	t.Log("")
	t.Logf("%-14s %5d %7.0f%% (%d) %4.0f%% (%d) %4.0f%% (%d) %11s %11s",
		"AGGREGATE", totalPairs+len(summaries), // tasks = pairs + repos (first task per repo has no pair)
		100.0*float64(aggUnchanged)/float64(totalPairs), aggUnchanged,
		100.0*float64(aggDelta)/float64(totalPairs), aggDelta,
		100.0*float64(aggFull)/float64(totalPairs), aggFull,
		fmt.Sprintf("%.1f%%", aggSavings/float64(aggDeltaCount)),
		fmt.Sprintf("%.1f%%", aggOverlap/float64(aggDeltaCount)))

	t.Log("")
	t.Logf("Total pairs: %d (across %d repos)", totalPairs, len(summaries))
	t.Logf("Delta frequency: %.1f%% of changed packs use delta encoding",
		100.0*float64(aggDelta)/float64(aggDelta+aggFull))
}

// TestDeltaPacking_ReQuery simulates the primary delta use case: the same task
// re-queried at a slightly different budget (simulating a code change that
// shifts a few symbols in/out of the pack). This is where delta shines:
// 90%+ symbols are identical, only the boundary changes.
func TestDeltaPacking_ReQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping delta re-query benchmark in short mode")
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
	if _, err := os.Stat(corpusDir); err != nil {
		corpusDir = "../cross-system/corpus"
	}

	stores := make(map[string]*store.SQLiteStore)
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
		s, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			continue
		}
		stores[task.Repo] = s
	}
	defer func() {
		for _, s := range stores {
			s.Close()
		}
	}()

	// Budget pairs: original and reduced (simulates pack change after code edit).
	budgetPairs := [][2]int{{5000, 4500}, {5000, 4000}, {5000, 3500}}

	t.Log("")
	t.Log("=== Delta Re-Query Benchmark (same task, different budget) ===")
	t.Log("Simulates: user edits code, re-queries same task, pack shifts slightly.")
	t.Log("")

	for _, bp := range budgetPairs {
		budgetA, budgetB := bp[0], bp[1]
		t.Logf("--- Budget %d -> %d (%.0f%% reduction) ---", budgetA, budgetB,
			100.0*(1.0-float64(budgetB)/float64(budgetA)))

		var totalSavings, totalOverlap float64
		var deltaCount, fullCount, unchangedCount int

		for _, task := range tasks {
			if repoFilter != "" && task.Repo != repoFilter {
				continue
			}
			s, ok := stores[task.Repo]
			if !ok {
				continue
			}
			ctx := stdctx.Background()

			var repoURL string
			if repos, err := s.AllRepos(ctx); err == nil && len(repos) > 0 {
				repoURL = repos[0].RepoURL
			}

			engine := knowingctx.NewContextEngine(s)
			engine.DisablePersistentCache()

			blockA, err := engine.ForTask(ctx, knowingctx.TaskOptions{
				TaskDescription: task.Description,
				TokenBudget:     budgetA,
				RepoURL:         repoURL,
				Format:          "gcf",
			})
			if err != nil || len(blockA.Symbols) == 0 {
				continue
			}
			blockA.PackRoot = knowingctx.ComputePackRootExported(task.Description, blockA.Symbols)

			engine2 := knowingctx.NewContextEngine(s)
			engine2.DisablePersistentCache()

			blockB, err := engine2.ForTask(ctx, knowingctx.TaskOptions{
				TaskDescription: task.Description,
				TokenBudget:     budgetB,
				RepoURL:         repoURL,
				Format:          "gcf",
			})
			if err != nil || len(blockB.Symbols) == 0 {
				continue
			}
			blockB.PackRoot = knowingctx.ComputePackRootExported(task.Description, blockB.Symbols)

			if blockA.PackRoot == blockB.PackRoot {
				unchangedCount++
				continue
			}

			delta := knowingctx.DiffPacks(blockA, blockB, "gcf")
			if delta.IsWorthIt() {
				deltaCount++
				totalSavings += delta.SavingsPercent()
				totalOverlap += delta.SymbolOverlapPercent()
			} else {
				fullCount++
			}
		}

		total := unchangedCount + deltaCount + fullCount
		if total == 0 {
			t.Log("  No tasks completed")
			continue
		}

		avgSavings := 0.0
		avgOverlap := 0.0
		if deltaCount > 0 {
			avgSavings = totalSavings / float64(deltaCount)
			avgOverlap = totalOverlap / float64(deltaCount)
		}

		t.Logf("  Tasks: %d | Unchanged: %d (%.0f%%) | Delta: %d (%.0f%%) | Full: %d (%.0f%%)",
			total,
			unchangedCount, 100.0*float64(unchangedCount)/float64(total),
			deltaCount, 100.0*float64(deltaCount)/float64(total),
			fullCount, 100.0*float64(fullCount)/float64(total))
		if deltaCount > 0 {
			t.Logf("  Avg savings: %.1f%% | Avg overlap: %.1f%%", avgSavings, avgOverlap)
		}
	}
}

// loadTasks loads all task fixtures from the cross-system corpus.
func loadTasks(t *testing.T) []benchtype.Task {
	t.Helper()
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
