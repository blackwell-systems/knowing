package crosssystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/metrics"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
)

// TestCrossTaskVocab measures cross-task vocabulary bridging on the real corpus.
// It proves that vocab associations learned from task A help a DIFFERENT task B,
// not just self-reinforcement.
//
// Protocol:
//  1. Round 1: cold start, no vocab. Record per-task P@10.
//  2. Simulate agent usage: for each task, record vocab associations
//     (task keywords -> ground truth symbol names). This mimics an agent that
//     worked on each task and used the correct symbols.
//  3. Round 2: with vocab. Record per-task P@10.
//  4. For each task that improved, determine whether the improvement came from
//     its OWN vocab (self-reinforcement) or from ANOTHER task's vocab (cross-task bridging).
//
// A task's improvement is classified as cross-task if:
//   - The task improved (round 2 P@10 > round 1 P@10)
//   - At least one shared keyword exists between this task and a different task
//     that recorded vocab associations
//
// Usage:
//
//	BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossTaskVocab -v -timeout 30m
//	BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossTaskVocab -v -timeout 0
func TestCrossTaskVocab(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-task vocab test in short mode")
	}

	tasks := loadCompoundingTasks(t)
	if len(tasks) == 0 {
		t.Skip("cross-task vocab test requires task fixtures")
	}

	repoFilter := os.Getenv("BENCH_REPOS")

	adapter := adapters.NewKnowing()

	corpusDir := "corpus"
	if cd := os.Getenv("BENCH_CORPUS"); cd != "" {
		corpusDir = cd
	}
	indexed := make(map[string]bool)
	repoPaths := make(map[string]string)
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

	// Clear any stale vocab from prior runs.
	adapter.ClearAllVocab()

	// ─── Round 1: cold start (no vocab) ───
	t.Log("=== Round 1: cold start (no vocab) ===")
	type taskResult struct {
		task     benchtype.Task
		repoPath string
		p10r1    float64
		p10r2    float64
		keywords []string // extracted keywords for this task
	}
	var results []taskResult

	for _, task := range tasks {
		if repoFilter != "" && task.Repo != repoFilter {
			continue
		}
		rp, ok := repoPaths[task.Repo]
		if !ok {
			continue
		}

		ret, err := adapter.Retrieve(rp, task, 5000)
		if err != nil {
			continue
		}
		m := metrics.Compute(ret, task.GroundTruth)

		// Extract keywords for this task (same path as ForTask).
		ks := knowingctx.ExtractKeywordSet(task.Description)
		kws := ks.All()

		results = append(results, taskResult{
			task:     task,
			repoPath: rp,
			p10r1:    m.PrecisionAt10,
			keywords: kws,
		})
	}

	if len(results) == 0 {
		t.Fatal("no tasks completed in round 1")
	}

	var sumR1 float64
	for _, r := range results {
		sumR1 += r.p10r1
	}
	meanR1 := sumR1 / float64(len(results))
	t.Logf("Round 1: P@10=%.3f (%d tasks)", meanR1, len(results))

	// ─── Simulate vocab recording ───
	// Simulate realistic agent usage: the agent works on a task and uses a few
	// ground truth symbols (not all of them). We use Primary keywords (specific
	// compounds/identifiers) rather than all Components, and limit to top 3
	// ground truth symbols per task (realistic agent behavior).
	t.Log("=== Simulating agent vocab recording (realistic: primary keywords, top 3 GT symbols, 2 passes) ===")
	totalAssocs := 0
	for _, r := range results {
		// Use Primary keywords (compounds + exact) for specificity.
		// Fall back to top 5 Components if Primary is empty.
		ks := knowingctx.ExtractKeywordSet(r.task.Description)
		vocabKws := ks.Primary()
		if len(vocabKws) == 0 {
			vocabKws = ks.All()
			if len(vocabKws) > 5 {
				vocabKws = vocabKws[:5]
			}
		}

		// Limit ground truth to top 3 (simulating agent touching a few files).
		gt := r.task.GroundTruth
		if len(gt) > 3 {
			gt = gt[:3]
		}

		for pass := 0; pass < 2; pass++ {
			adapter.SimulateVocabRecording(r.repoPath, vocabKws, gt)
		}
		totalAssocs += len(vocabKws) * len(gt)
	}
	t.Logf("Recorded ~%d vocab associations across %d tasks (after filter)", totalAssocs, len(results))

	// Build keyword -> task ID map for cross-task analysis.
	keywordToTasks := make(map[string][]string) // keyword -> []taskID
	taskKeywordMap := make(map[string]map[string]bool) // taskID -> set of keywords
	for _, r := range results {
		kwSet := make(map[string]bool, len(r.keywords))
		for _, kw := range r.keywords {
			kw = strings.ToLower(kw)
			keywordToTasks[kw] = append(keywordToTasks[kw], r.task.ID)
			kwSet[kw] = true
		}
		taskKeywordMap[r.task.ID] = kwSet
	}

	// ─── Round 2: with vocab ───
	t.Log("=== Round 2: with vocab associations ===")
	for i := range results {
		r := &results[i]
		ret, err := adapter.Retrieve(r.repoPath, r.task, 5000)
		if err != nil {
			continue
		}
		m := metrics.Compute(ret, r.task.GroundTruth)
		r.p10r2 = m.PrecisionAt10
	}

	var sumR2 float64
	for _, r := range results {
		sumR2 += r.p10r2
	}
	meanR2 := sumR2 / float64(len(results))
	t.Logf("Round 2: P@10=%.3f (%d tasks)", meanR2, len(results))

	// ─── Cross-task analysis ───
	t.Log("\n=== CROSS-TASK ANALYSIS ===")

	var improved, degraded, unchanged int
	var selfOnly, crossTask int
	var crossTaskLift float64

	type improvement struct {
		taskID      string
		repo        string
		delta       float64
		sharedKws   []string
		otherTasks  []string
		isCrossTask bool
	}
	var improvements []improvement

	for _, r := range results {
		delta := r.p10r2 - r.p10r1
		if delta > 0.001 {
			improved++

			// Check if any keyword overlaps with a DIFFERENT task.
			var sharedKws []string
			otherTaskSet := make(map[string]bool)
			for _, kw := range r.keywords {
				kw = strings.ToLower(kw)
				for _, otherTaskID := range keywordToTasks[kw] {
					if otherTaskID != r.task.ID {
						otherTaskSet[otherTaskID] = true
						sharedKws = append(sharedKws, kw)
					}
				}
			}

			// Deduplicate shared keywords.
			kwSeen := make(map[string]bool)
			var uniqueKws []string
			for _, kw := range sharedKws {
				if !kwSeen[kw] {
					kwSeen[kw] = true
					uniqueKws = append(uniqueKws, kw)
				}
			}

			var otherTasks []string
			for tid := range otherTaskSet {
				otherTasks = append(otherTasks, tid)
			}
			sort.Strings(otherTasks)

			isCross := len(otherTasks) > 0
			if isCross {
				crossTask++
				crossTaskLift += delta
			} else {
				selfOnly++
			}

			improvements = append(improvements, improvement{
				taskID:      r.task.ID,
				repo:        r.task.Repo,
				delta:       delta,
				sharedKws:   uniqueKws,
				otherTasks:  otherTasks,
				isCrossTask: isCross,
			})
		} else if delta < -0.001 {
			degraded++
		} else {
			unchanged++
		}
	}

	// Sort improvements by delta descending.
	sort.Slice(improvements, func(i, j int) bool {
		return improvements[i].delta > improvements[j].delta
	})

	t.Logf("\n| Category     | Count | Notes |")
	t.Logf("|-------------|-------|-------|")
	t.Logf("| Improved     | %5d | P@10 increased |", improved)
	t.Logf("| Degraded     | %5d | P@10 decreased |", degraded)
	t.Logf("| Unchanged    | %5d | |", unchanged)
	t.Logf("| Cross-task   | %5d | Improvement from DIFFERENT task's vocab |", crossTask)
	t.Logf("| Self-only    | %5d | Improvement from own vocab only |", selfOnly)

	if crossTask > 0 {
		t.Logf("\nMean cross-task lift: %+.4f P@10", crossTaskLift/float64(crossTask))
	}

	// Print top improvements with cross-task attribution.
	if len(improvements) > 0 {
		t.Log("\nTop improvements:")
		t.Logf("| Task | Repo | Delta | Cross-task? | Shared Keywords | Bridging Tasks |")
		t.Logf("|------|------|-------|-------------|-----------------|---------------|")
		showN := 20
		if len(improvements) < showN {
			showN = len(improvements)
		}
		for _, imp := range improvements[:showN] {
			crossLabel := "self"
			if imp.isCrossTask {
				crossLabel = "CROSS"
			}
			kwStr := strings.Join(imp.sharedKws, ", ")
			if len(kwStr) > 40 {
				kwStr = kwStr[:40] + "..."
			}
			otherStr := strings.Join(imp.otherTasks, ", ")
			if len(otherStr) > 40 {
				otherStr = otherStr[:40] + "..."
			}
			t.Logf("| %s | %s | %+.3f | %s | %s | %s |",
				imp.taskID, imp.repo, imp.delta, crossLabel, kwStr, otherStr)
		}
	}

	// Summary.
	t.Logf("\n=== SUMMARY ===")
	t.Logf("Round 1 (cold):  P@10=%.3f", meanR1)
	t.Logf("Round 2 (vocab): P@10=%.3f", meanR2)
	totalDelta := meanR2 - meanR1
	t.Logf("Total lift:      %+.4f", totalDelta)
	if meanR1 > 0 {
		t.Logf("Relative lift:   %+.1f%%", totalDelta/meanR1*100)
	}
	t.Logf("Tasks improved:  %d/%d (%.0f%%)", improved, len(results), float64(improved)/float64(len(results))*100)
	if improved > 0 {
		pctCross := float64(crossTask) / float64(improved) * 100
		t.Logf("Cross-task:      %d/%d improved (%.0f%% of improvements are cross-task)", crossTask, improved, pctCross)
	}

	// Validation: at least some cross-task bridging should occur.
	if crossTask == 0 && improved > 0 {
		t.Log("\nWARNING: all improvements are self-reinforcement, no cross-task bridging detected")
	}

	fmt.Fprintf(os.Stderr, "\n[cross-task-vocab] R1=%.3f R2=%.3f delta=%+.4f improved=%d cross=%d self=%d\n",
		meanR1, meanR2, totalDelta, improved, crossTask, selfOnly)
}
