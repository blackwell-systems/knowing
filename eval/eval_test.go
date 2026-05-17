// Package eval implements a standardized evaluation framework for knowing's
// context engine. It measures retrieval accuracy across tiered task fixtures
// and produces publishable metrics (Precision@K, Recall@K, MRR).
//
// Run: GOWORK=off go test ./eval/ -v -count=1
// Short: GOWORK=off go test ./eval/ -short (skips indexing, uses existing DB)
package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"gopkg.in/yaml.v3"
)

// Fixture represents a single eval task with ground truth.
type Fixture struct {
	Task        string   `yaml:"task"`
	Difficulty  string   `yaml:"difficulty"`
	Tags        []string `yaml:"tags"`
	GroundTruth []string `yaml:"ground_truth"`
}

// Result holds metrics for a single fixture evaluation.
type Result struct {
	Fixture     string
	Difficulty  string
	Precision10 float64
	Recall10    float64
	MRR         float64
	TopKHits    int
	GroundTotal int
}

func TestEval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping eval in short mode")
	}

	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	// Index the knowing repo.
	dbPath := t.TempDir() + "/eval.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	_, err = idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	engine := knowingctx.NewContextEngine(st)

	// Load all fixtures.
	fixtures := loadFixtures(t, filepath.Join(repoRoot, "eval", "fixtures"))

	// Run evaluation.
	var results []Result
	topK := 10
	tokenBudget := 5000

	for _, fix := range fixtures {
		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: fix.Task,
			TokenBudget:     tokenBudget,
			Format:          "json",
		})
		if err != nil {
			t.Logf("  SKIP %s: %v", fix.Task[:40], err)
			continue
		}

		k := topK
		if len(block.Symbols) < k {
			k = len(block.Symbols)
		}

		hits := 0
		firstHitRank := 0
		for i := 0; i < k; i++ {
			if isRelevant(block.Symbols[i].Node.QualifiedName, fix.GroundTruth) {
				hits++
				if firstHitRank == 0 {
					firstHitRank = i + 1
				}
			}
		}

		precision := 0.0
		if k > 0 {
			precision = float64(hits) / float64(k)
		}
		recall := float64(hits) / float64(len(fix.GroundTruth))
		mrr := 0.0
		if firstHitRank > 0 {
			mrr = 1.0 / float64(firstHitRank)
		}

		results = append(results, Result{
			Fixture:     fix.Task,
			Difficulty:  fix.Difficulty,
			Precision10: precision,
			Recall10:    recall,
			MRR:         mrr,
			TopKHits:    hits,
			GroundTotal: len(fix.GroundTruth),
		})
	}

	// Aggregate by difficulty.
	tiers := map[string][]Result{}
	for _, r := range results {
		tiers[r.Difficulty] = append(tiers[r.Difficulty], r)
	}

	t.Log("")
	t.Log("=== EVAL RESULTS ===")
	t.Log("")
	t.Logf("%-50s | %6s | %6s | %5s | %s", "Task", "P@10", "R@10", "MRR", "Tier")
	t.Logf("%s-+-%s-+-%s-+-%s-+-%s",
		strings.Repeat("-", 50), strings.Repeat("-", 6),
		strings.Repeat("-", 6), strings.Repeat("-", 5), strings.Repeat("-", 6))

	for _, r := range results {
		taskShort := r.Fixture
		if len(taskShort) > 50 {
			taskShort = taskShort[:47] + "..."
		}
		t.Logf("%-50s | %5.1f%% | %5.1f%% | %5.2f | %s",
			taskShort, r.Precision10*100, r.Recall10*100, r.MRR, r.Difficulty)
	}

	// Per-tier summary.
	t.Log("")
	t.Log("=== PER-TIER SUMMARY ===")
	t.Log("")
	t.Logf("%-8s | %6s | %6s | %5s | %s", "Tier", "P@10", "R@10", "MRR", "N")
	t.Logf("%s-+-%s-+-%s-+-%s-+-%s",
		strings.Repeat("-", 8), strings.Repeat("-", 6),
		strings.Repeat("-", 6), strings.Repeat("-", 5), strings.Repeat("-", 3))

	tierOrder := []string{"easy", "medium", "hard"}
	var totalP, totalR, totalMRR float64
	var totalN int

	for _, tier := range tierOrder {
		tr := tiers[tier]
		if len(tr) == 0 {
			continue
		}
		var sumP, sumR, sumMRR float64
		for _, r := range tr {
			sumP += r.Precision10
			sumR += r.Recall10
			sumMRR += r.MRR
		}
		n := float64(len(tr))
		t.Logf("%-8s | %5.1f%% | %5.1f%% | %5.2f | %d",
			tier, sumP/n*100, sumR/n*100, sumMRR/n, len(tr))
		totalP += sumP
		totalR += sumR
		totalMRR += sumMRR
		totalN += len(tr)
	}

	// Overall.
	t.Log("")
	if totalN > 0 {
		n := float64(totalN)
		t.Logf("OVERALL  | %5.1f%% | %5.1f%% | %5.2f | %d fixtures",
			totalP/n*100, totalR/n*100, totalMRR/n, totalN)
	}

	// Write FINDINGS.md
	writeEvalFindings(t, results, tiers, repoRoot)
}

func loadFixtures(t *testing.T, dir string) []Fixture {
	t.Helper()
	var fixtures []Fixture

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var fix Fixture
		if err := yaml.Unmarshal(data, &fix); err != nil {
			t.Logf("warning: skip %s: %v", path, err)
			return nil
		}
		fixtures = append(fixtures, fix)
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}

	// Sort by difficulty order.
	diffOrder := map[string]int{"easy": 0, "medium": 1, "hard": 2}
	sort.Slice(fixtures, func(i, j int) bool {
		return diffOrder[fixtures[i].Difficulty] < diffOrder[fixtures[j].Difficulty]
	})

	return fixtures
}

func isRelevant(qualifiedName string, groundTruth []string) bool {
	for _, gt := range groundTruth {
		if strings.Contains(qualifiedName, gt) {
			return true
		}
	}
	return false
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatal("cannot find repo root")
		}
		dir = parent
	}
}

func writeEvalFindings(t *testing.T, results []Result, tiers map[string][]Result, repoRoot string) {
	t.Helper()

	var sb strings.Builder
	sb.WriteString("# Eval Framework: Retrieval Accuracy\n\n")
	sb.WriteString("**Auto-generated. Do not edit manually.**\n\n")

	sb.WriteString("## Methodology\n\n")
	sb.WriteString("Standardized evaluation of knowing's context engine across tiered task fixtures.\n")
	sb.WriteString("Each fixture defines a development task and hand-curated ground-truth symbols.\n")
	sb.WriteString("The engine returns ranked results; we measure Precision@10, Recall@10, and MRR.\n\n")
	sb.WriteString("**Tiers:**\n")
	sb.WriteString("- **Easy:** Single-package tasks (all relevant symbols in one package)\n")
	sb.WriteString("- **Medium:** Cross-package tasks (symbols span 2-3 packages)\n")
	sb.WriteString("- **Hard:** Cross-repo or multi-system tasks (runtime, daemon, resolver involved)\n\n")

	sb.WriteString("## Results\n\n")
	sb.WriteString("| Task | P@10 | R@10 | MRR | Tier |\n")
	sb.WriteString("|------|------|------|-----|------|\n")
	for _, r := range results {
		taskShort := r.Fixture
		if len(taskShort) > 60 {
			taskShort = taskShort[:57] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.2f | %s |\n",
			taskShort, r.Precision10*100, r.Recall10*100, r.MRR, r.Difficulty))
	}

	sb.WriteString("\n## Per-Tier Summary\n\n")
	sb.WriteString("| Tier | Precision@10 | Recall@10 | MRR | Fixtures |\n")
	sb.WriteString("|------|-------------|-----------|-----|----------|\n")

	tierOrder := []string{"easy", "medium", "hard"}
	var totalP, totalR, totalMRR float64
	var totalN int

	for _, tier := range tierOrder {
		tr := tiers[tier]
		if len(tr) == 0 {
			continue
		}
		var sumP, sumR, sumMRR float64
		for _, r := range tr {
			sumP += r.Precision10
			sumR += r.Recall10
			sumMRR += r.MRR
		}
		n := float64(len(tr))
		sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %.1f%% | %.2f | %d |\n",
			tier, sumP/n*100, sumR/n*100, sumMRR/n, len(tr)))
		totalP += sumP
		totalR += sumR
		totalMRR += sumMRR
		totalN += len(tr)
	}

	if totalN > 0 {
		n := float64(totalN)
		sb.WriteString(fmt.Sprintf("| **Overall** | **%.1f%%** | **%.1f%%** | **%.2f** | **%d** |\n\n",
			totalP/n*100, totalR/n*100, totalMRR/n, totalN))
	}

	sb.WriteString("## Reproducibility\n\n")
	sb.WriteString("```bash\nGOWORK=off go test ./eval/ -v -count=1 -timeout 5m\n```\n\n")
	sb.WriteString("Indexes the knowing repo into a temp DB and evaluates all fixtures.\n")
	sb.WriteString("Add new fixtures to `eval/fixtures/{easy,medium,hard}/` in YAML format.\n")

	findingsPath := filepath.Join(repoRoot, "eval", "FINDINGS.md")
	if err := os.WriteFile(findingsPath, []byte(sb.String()), 0644); err != nil {
		t.Logf("warning: could not write FINDINGS.md: %v", err)
	} else {
		t.Logf("Wrote %s", findingsPath)
	}
}
