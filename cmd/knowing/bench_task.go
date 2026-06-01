package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"gopkg.in/yaml.v3"
)

// cmdBenchTask runs a single benchmark task and shows detailed hit/miss analysis.
// Usage: knowing bench-task -task <task-id> [-corpus path] [-budget 5000]
func cmdBenchTask(args []string) error {
	fs := flag.NewFlagSet("bench-task", flag.ExitOnError)
	taskID := fs.String("task", "", "Task ID (e.g. terraform-hard-004) (required)")
	corpusDir := fs.String("corpus", "bench/cross-system/corpus", "Path to corpus directory")
	budget := fs.Int("budget", 5000, "Token budget")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *taskID == "" {
		return fmt.Errorf("usage: knowing bench-task -task <task-id> [-corpus path] [-budget N]")
	}

	// Find the task fixture.
	task, err := findTask(*corpusDir, *taskID)
	if err != nil {
		return err
	}

	// Find the repo's DB.
	dbPath := filepath.Join(*corpusDir, "repos", task.Repo, ".knowing", "graph.db")
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("DB not found at %s (repo: %s)", dbPath, task.Repo)
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	// Get repo URL from the DB (matches what the benchmark adapter uses).
	// Previously used detectRepoURL (git remote) which produces a different
	// hash than the local path stored during indexing, causing mismatched results.
	repoPath := filepath.Join(*corpusDir, "repos", task.Repo)
	ctx := context.Background()
	var repoURL string
	if repos, err := st.AllRepos(ctx); err == nil && len(repos) > 0 {
		repoURL = repos[0].RepoURL
	} else {
		repoURL = detectRepoURL(repoPath)
	}

	fmt.Printf("=== BENCH TASK: %s ===\n", task.ID)
	fmt.Printf("Repo: %s (%s)\n", task.Repo, repoURL)
	fmt.Printf("Tier: %s\n", task.Tier)
	fmt.Printf("Description: %s\n", task.Description)
	fmt.Printf("Ground truth: %v\n", task.GroundTruth)
	fmt.Printf("Budget: %d tokens\n", *budget)
	fmt.Printf("\n")

	// Run ForTask.
	engine := knowingctx.NewContextEngine(st)
	engine.DisablePersistentCache()
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: task.Description,
		TokenBudget:     *budget,
		RepoURL:         repoURL,
	})
	if err != nil {
		return fmt.Errorf("ForTask error: %w", err)
	}

	if block == nil || len(block.Symbols) == 0 {
		fmt.Printf("Result: NO SYMBOLS RETURNED\n")
		fmt.Printf("P@10: 0.000\n")
		return nil
	}

	// Show top 10 with ground truth hit/miss.
	fmt.Printf("--- Top 10 Retrieved ---\n")
	hits := 0
	for i, sym := range block.Symbols {
		if i >= 10 {
			break
		}
		name := terminalSymbol(sym.Node.QualifiedName)
		matched := matchesAnyGroundTruth(sym.Node.QualifiedName, task.GroundTruth)
		marker := "  MISS"
		if matched {
			marker = "  HIT"
			hits++
		}
		fmt.Printf("  [%2d] %-50s (score: %.3f)%s\n", i+1, name, sym.Score, marker)
	}

	p10 := float64(hits) / 10.0
	fmt.Printf("\n--- Metrics ---\n")
	fmt.Printf("  P@10:  %.3f (%d/%d hits in top 10)\n", p10, hits, min(10, len(block.Symbols)))
	fmt.Printf("  Total symbols returned: %d\n", len(block.Symbols))

	// Show ground truth analysis.
	fmt.Printf("\n--- Ground Truth Analysis ---\n")
	for _, gt := range task.GroundTruth {
		found := false
		rank := -1
		for i, sym := range block.Symbols {
			if matchesAnyGroundTruth(sym.Node.QualifiedName, []string{gt}) {
				found = true
				rank = i + 1
				break
			}
		}
		if found {
			status := "FOUND"
			if rank > 10 {
				status = fmt.Sprintf("FOUND (rank %d, outside top 10)", rank)
			} else {
				status = fmt.Sprintf("FOUND (rank %d)", rank)
			}
			fmt.Printf("  %-40s %s\n", gt, status)
		} else {
			fmt.Printf("  %-40s MISSING\n", gt)
		}
	}

	// Recall.
	foundCount := 0
	for _, gt := range task.GroundTruth {
		for _, sym := range block.Symbols {
			if matchesAnyGroundTruth(sym.Node.QualifiedName, []string{gt}) {
				foundCount++
				break
			}
		}
	}
	fmt.Printf("\n  Recall (full result set): %d/%d (%.1f%%)\n",
		foundCount, len(task.GroundTruth), 100*float64(foundCount)/float64(len(task.GroundTruth)))

	return nil
}

// taskFixture mirrors benchtype.Task for YAML loading without importing bench packages.
type taskFixture struct {
	ID          string   `yaml:"id"`
	Repo        string   `yaml:"repo"`
	Tier        string   `yaml:"tier"`
	Description string   `yaml:"description"`
	GroundTruth []string `yaml:"ground_truth"`
	Tags        []string `yaml:"tags"`
	Notes       string   `yaml:"notes"`
	Source      string   `yaml:"source"`
}

// findTask walks the corpus tasks directory to find a task by ID.
func findTask(corpusDir, taskID string) (*taskFixture, error) {
	tasksDir := filepath.Join(corpusDir, "tasks")
	var found *taskFixture

	err := filepath.Walk(tasksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var t taskFixture
		if err := yaml.Unmarshal(data, &t); err != nil {
			return nil
		}
		if t.ID == taskID {
			found = &t
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		return nil, fmt.Errorf("walking tasks: %w", err)
	}
	if found == nil {
		return nil, fmt.Errorf("task %q not found in %s", taskID, tasksDir)
	}
	return found, nil
}

// matchesAnyGroundTruth checks if a retrieved QN matches any ground truth symbol.
// Uses the same dot-bounded matching as the benchmark harness.
func matchesAnyGroundTruth(retrievedQN string, groundTruth []string) bool {
	// Normalize: extract terminal symbol parts.
	retrieved := normalizeForMatch(retrievedQN)
	for _, gt := range groundTruth {
		gtNorm := strings.ToLower(gt)
		rNorm := strings.ToLower(retrieved)

		// Exact match.
		if rNorm == gtNorm {
			return true
		}
		// Suffix match.
		if strings.HasSuffix(rNorm, "."+gtNorm) || strings.HasSuffix(gtNorm, "."+rNorm) {
			return true
		}
		// Dot-bounded contains.
		if dotBoundedContains(rNorm, gtNorm) || dotBoundedContains(gtNorm, rNorm) {
			return true
		}
	}
	return false
}

// normalizeForMatch extracts the symbol portion after the file path from a QN.
func normalizeForMatch(qn string) string {
	// Strip repo prefix.
	if idx := strings.Index(qn, "://"); idx >= 0 {
		qn = qn[idx+3:]
	}
	// Find file extension boundary.
	exts := []string{".go.", ".py.", ".ts.", ".tsx.", ".js.", ".rs.", ".java.", ".cs.", ".rb."}
	for _, ext := range exts {
		if idx := strings.LastIndex(qn, ext); idx >= 0 {
			return qn[idx+len(ext):]
		}
	}
	// Fallback: last path segment.
	if idx := strings.LastIndex(qn, "/"); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}

// dotBoundedContains checks if inner appears in outer at dot boundaries.
func dotBoundedContains(outer, inner string) bool {
	idx := strings.Index(outer, inner)
	if idx < 0 {
		return false
	}
	if idx > 0 && outer[idx-1] != '.' {
		return false
	}
	end := idx + len(inner)
	if end < len(outer) && outer[end] != '.' {
		return false
	}
	return true
}
