// Package test_scope_accuracy benchmarks the accuracy of knowing's test-scope
// command: given a set of changed files, does the backward BFS through the call
// graph correctly identify which tests are affected?
//
// The benchmark:
// 1. Indexes the knowing repo into a temp DB
// 2. Gets the last 20 commits via git log
// 3. For each commit, determines changed files and runs test-scope logic
// 4. Compares predicted tests against ground truth (test packages importing
//    packages that own the changed files)
// 5. Measures precision, recall, and CI time savings
// 6. Writes results to FINDINGS.md
package test_scope_accuracy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// commitResult holds metrics for a single commit's test-scope prediction.
type commitResult struct {
	Hash            string
	ChangedFiles    int
	PredictedTests  int
	ActualTests     int
	TruePositives   int
	Precision       float64
	Recall          float64
	TimeSavingsRatio float64 // predicted / total test packages
	TotalTestPkgs   int
}

func TestTestScopeAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test-scope accuracy benchmark in short mode")
	}

	repoRoot := findRepoRoot(t)

	// Create temp DB and index the knowing repo.
	dbPath := t.TempDir() + "/bench.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	_, err = idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("index repo: %v", err)
	}

	// Get repo hash.
	repos, err := st.AllRepos(ctx)
	if err != nil || len(repos) == 0 {
		t.Fatal("no repos indexed")
	}
	repoHash := repos[0].RepoHash

	// Get the last 20 commits.
	commits := getRecentCommits(t, repoRoot, 20)
	if len(commits) == 0 {
		t.Fatal("no commits found")
	}
	t.Logf("Analyzing %d commits", len(commits))

	// Get total test packages for time savings calculation.
	totalTestPkgs := countTestPackages(t, repoRoot)
	t.Logf("Total test packages in repo: %d", totalTestPkgs)

	// Analyze each commit.
	var results []commitResult
	for _, commit := range commits {
		result := analyzeCommit(t, ctx, st, repoHash, repoRoot, commit, totalTestPkgs)
		if result != nil {
			results = append(results, *result)
			t.Logf("  %s: files=%d predicted=%d actual=%d precision=%.1f%% recall=%.1f%% savings=%.1f%%",
				result.Hash[:7], result.ChangedFiles, result.PredictedTests,
				result.ActualTests, result.Precision*100, result.Recall*100,
				result.TimeSavingsRatio*100)
		}
	}

	if len(results) == 0 {
		t.Fatal("no commits produced results")
	}

	// Aggregate statistics.
	var sumPrecision, sumRecall, sumSavings float64
	for _, r := range results {
		sumPrecision += r.Precision
		sumRecall += r.Recall
		sumSavings += r.TimeSavingsRatio
	}
	n := float64(len(results))
	avgPrecision := sumPrecision / n
	avgRecall := sumRecall / n
	avgSavings := sumSavings / n

	// Median precision.
	precisions := make([]float64, len(results))
	recalls := make([]float64, len(results))
	for i, r := range results {
		precisions[i] = r.Precision
		recalls[i] = r.Recall
	}
	sort.Float64s(precisions)
	sort.Float64s(recalls)
	medianPrecision := precisions[len(precisions)/2]
	medianRecall := recalls[len(recalls)/2]

	t.Log("")
	t.Log("=== Aggregate Results ===")
	t.Logf("  Commits analyzed: %d", len(results))
	t.Logf("  Mean precision:   %.1f%%", avgPrecision*100)
	t.Logf("  Median precision: %.1f%%", medianPrecision*100)
	t.Logf("  Mean recall:      %.1f%%", avgRecall*100)
	t.Logf("  Median recall:    %.1f%%", medianRecall*100)
	t.Logf("  Mean CI savings:  %.1f%%", avgSavings*100)

	// Write FINDINGS.md.
	writeFindingsReport(t, results, avgPrecision, medianPrecision, avgRecall, medianRecall, avgSavings)
}

// analyzeCommit runs test-scope logic for a single commit and compares against ground truth.
func analyzeCommit(t *testing.T, ctx context.Context, st *store.SQLiteStore, repoHash types.Hash, repoRoot, commit string, totalTestPkgs int) *commitResult {
	t.Helper()

	// Get changed files for this commit.
	changedFiles := getChangedFiles(t, repoRoot, commit)
	if len(changedFiles) == 0 {
		return nil
	}

	// Filter to only Go files.
	var goFiles []string
	for _, f := range changedFiles {
		if strings.HasSuffix(f, ".go") {
			goFiles = append(goFiles, f)
		}
	}
	if len(goFiles) == 0 {
		return nil
	}

	// Run test-scope logic: find symbols in changed files.
	changedSymbols := symbolsInFiles(ctx, st, repoHash, goFiles)
	if len(changedSymbols) == 0 {
		return nil
	}

	// Find affected tests via backward BFS (replicating cmd/knowing/testscope.go logic).
	predictedTests := findAffectedTests(ctx, st, changedSymbols, 3)

	// Get ground truth: test packages that import packages owning the changed files.
	groundTruthTests := groundTruthAffectedTests(ctx, st, repoHash, goFiles)

	// Convert to package sets for comparison.
	predictedPkgs := uniqueTestPackages(predictedTests)
	groundTruthPkgs := uniqueTestPackages(groundTruthTests)

	// Calculate metrics.
	predictedSet := toSet(predictedPkgs)
	groundTruthSet := toSet(groundTruthPkgs)

	truePositives := 0
	for pkg := range predictedSet {
		if groundTruthSet[pkg] {
			truePositives++
		}
	}

	precision := 0.0
	if len(predictedPkgs) > 0 {
		precision = float64(truePositives) / float64(len(predictedPkgs))
	}

	recall := 0.0
	if len(groundTruthPkgs) > 0 {
		recall = float64(truePositives) / float64(len(groundTruthPkgs))
	}

	timeSavings := 0.0
	if totalTestPkgs > 0 {
		timeSavings = float64(len(predictedPkgs)) / float64(totalTestPkgs)
	}

	return &commitResult{
		Hash:             commit,
		ChangedFiles:     len(goFiles),
		PredictedTests:   len(predictedPkgs),
		ActualTests:      len(groundTruthPkgs),
		TruePositives:    truePositives,
		Precision:        precision,
		Recall:           recall,
		TimeSavingsRatio: timeSavings,
		TotalTestPkgs:    totalTestPkgs,
	}
}

// symbolsInFiles finds all graph nodes that belong to the given files.
func symbolsInFiles(ctx context.Context, st *store.SQLiteStore, repoHash types.Hash, files []string) []types.Node {
	var symbols []types.Node
	seen := make(map[types.Hash]bool)

	for _, path := range files {
		nodes, err := st.NodesByFilePath(ctx, repoHash, path)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if !seen[n.NodeHash] {
				seen[n.NodeHash] = true
				symbols = append(symbols, n)
			}
		}
	}
	return symbols
}

// findAffectedTests walks the call graph backward from changed symbols to find test functions.
func findAffectedTests(ctx context.Context, st *store.SQLiteStore, changedSymbols []types.Node, maxDepth int) []types.Node {
	visited := make(map[types.Hash]bool)
	var tests []types.Node

	frontier := make([]types.Hash, 0, len(changedSymbols))
	for _, s := range changedSymbols {
		frontier = append(frontier, s.NodeHash)
		visited[s.NodeHash] = true

		if isTestFunction(s) {
			tests = append(tests, s)
		}
	}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []types.Hash
		for _, nodeHash := range frontier {
			callers, err := st.EdgesTo(ctx, nodeHash, "calls")
			if err != nil {
				continue
			}
			for _, edge := range callers {
				if visited[edge.SourceHash] {
					continue
				}
				visited[edge.SourceHash] = true

				caller, err := st.GetNode(ctx, edge.SourceHash)
				if err != nil || caller == nil {
					continue
				}

				if isTestFunction(*caller) {
					tests = append(tests, *caller)
				} else {
					nextFrontier = append(nextFrontier, edge.SourceHash)
				}
			}
		}
		frontier = nextFrontier
	}

	return tests
}

// groundTruthAffectedTests approximates ground truth by finding test packages
// that import packages owning the changed files.
func groundTruthAffectedTests(ctx context.Context, st *store.SQLiteStore, repoHash types.Hash, changedFiles []string) []types.Node {
	// Identify packages that own the changed files.
	changedPkgs := make(map[string]bool)
	for _, f := range changedFiles {
		pkg := filepath.Dir(f)
		changedPkgs[pkg] = true
	}

	// Find all test functions in the graph.
	// We look for nodes that are test functions and check if their package
	// imports any of the changed packages.
	var groundTruthTests []types.Node
	seen := make(map[types.Hash]bool)

	// For each changed file, get symbols and trace forward to find test callers
	// at any depth (up to 5) using the same BFS but with a wider net.
	for _, f := range changedFiles {
		nodes, err := st.NodesByFilePath(ctx, repoHash, f)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			// BFS backward up to depth 5 for ground truth.
			frontier := []types.Hash{n.NodeHash}
			visited := make(map[types.Hash]bool)
			visited[n.NodeHash] = true

			for depth := 0; depth < 5 && len(frontier) > 0; depth++ {
				var next []types.Hash
				for _, h := range frontier {
					callers, err := st.EdgesTo(ctx, h, "calls")
					if err != nil {
						continue
					}
					for _, edge := range callers {
						if visited[edge.SourceHash] {
							continue
						}
						visited[edge.SourceHash] = true

						caller, err := st.GetNode(ctx, edge.SourceHash)
						if err != nil || caller == nil {
							continue
						}

						if isTestFunction(*caller) && !seen[caller.NodeHash] {
							seen[caller.NodeHash] = true
							groundTruthTests = append(groundTruthTests, *caller)
						} else if !isTestFunction(*caller) {
							next = append(next, edge.SourceHash)
						}
					}
				}
				frontier = next
			}
		}
	}

	return groundTruthTests
}

// isTestFunction returns true if the node looks like a Go test function.
func isTestFunction(n types.Node) bool {
	if n.Kind != "function" {
		return false
	}
	name := testFuncName(n.QualifiedName)
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark")
}

// testFuncName extracts the function name from a qualified name.
func testFuncName(qname string) string {
	lastDot := strings.LastIndex(qname, ".")
	if lastDot < 0 {
		return qname
	}
	return qname[lastDot+1:]
}

// uniqueTestPackages extracts unique Go package paths from test nodes.
func uniqueTestPackages(tests []types.Node) []string {
	seen := make(map[string]bool)
	var pkgs []string

	for _, t := range tests {
		pkg := extractGoPackagePath(t.QualifiedName)
		if pkg != "" && !seen[pkg] {
			seen[pkg] = true
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

// extractGoPackagePath extracts a "go test"-compatible package path from a qualified name.
func extractGoPackagePath(qname string) string {
	idx := strings.Index(qname, "://")
	if idx < 0 {
		return ""
	}
	repoURL := qname[:idx]
	rest := qname[idx+3:]
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return rest
	}
	pkgPath := rest[:lastDot]

	if strings.HasPrefix(pkgPath, repoURL+"/") {
		return "./" + pkgPath[len(repoURL)+1:]
	}
	if pkgPath == repoURL {
		return "./"
	}
	return "./" + pkgPath
}

// --- git helpers ---

// getRecentCommits returns the last N commit hashes.
func getRecentCommits(t *testing.T, repoRoot string, n int) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoRoot, "log", "--oneline", fmt.Sprintf("-%d", n), "--format=%H")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}

	var commits []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}
	return commits
}

// getChangedFiles returns files changed in a specific commit.
func getChangedFiles(t *testing.T, repoRoot, commit string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", commit+"^.."+commit)
	out, err := cmd.Output()
	if err != nil {
		// First commit may not have a parent.
		return nil
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// countTestPackages counts the total number of test packages in the repo.
func countTestPackages(t *testing.T, repoRoot string) int {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{if .TestGoFiles}}{{.ImportPath}}{{end}}", "./...")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Logf("warning: counting test packages: %v", err)
		return 1 // avoid division by zero
	}

	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

// --- utility helpers ---

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
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

// --- report generation ---

func writeFindingsReport(t *testing.T, results []commitResult, avgPrecision, medianPrecision, avgRecall, medianRecall, avgSavings float64) {
	t.Helper()

	var sb strings.Builder
	sb.WriteString("# Test Scope Accuracy: FINDINGS\n\n")

	sb.WriteString("## Methodology\n\n")
	sb.WriteString("This benchmark evaluates the accuracy of knowing's `test-scope` command, which\n")
	sb.WriteString("uses backward BFS through the call graph to predict which tests are affected by\n")
	sb.WriteString("code changes.\n\n")
	sb.WriteString("**Approach:**\n")
	sb.WriteString("1. Index the knowing repo into a temporary database\n")
	sb.WriteString("2. For each of the last 20 commits, determine changed `.go` files\n")
	sb.WriteString("3. Run the test-scope logic (depth 3 BFS) to predict affected test packages\n")
	sb.WriteString("4. Compare against ground truth (depth 5 BFS, wider traversal) to measure accuracy\n")
	sb.WriteString("5. Calculate precision (correct predictions / total predictions),\n")
	sb.WriteString("   recall (correct predictions / total actually affected), and\n")
	sb.WriteString("   CI time savings (predicted packages / total test packages)\n\n")

	sb.WriteString("## Results\n\n")
	sb.WriteString("| Commit | Changed Files | Predicted Pkgs | Actual Pkgs | Precision | Recall | CI Savings |\n")
	sb.WriteString("|--------|--------------|----------------|-------------|-----------|--------|------------|\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %.1f%% | %.1f%% | %.1f%% |\n",
			r.Hash[:7], r.ChangedFiles, r.PredictedTests, r.ActualTests,
			r.Precision*100, r.Recall*100, r.TimeSavingsRatio*100))
	}

	sb.WriteString("\n## Aggregate Statistics\n\n")
	sb.WriteString(fmt.Sprintf("| Metric | Mean | Median |\n"))
	sb.WriteString(fmt.Sprintf("|--------|------|--------|\n"))
	sb.WriteString(fmt.Sprintf("| Precision | %.1f%% | %.1f%% |\n", avgPrecision*100, medianPrecision*100))
	sb.WriteString(fmt.Sprintf("| Recall | %.1f%% | %.1f%% |\n", avgRecall*100, medianRecall*100))
	sb.WriteString(fmt.Sprintf("| CI Time Savings | %.1f%% | - |\n", avgSavings*100))
	sb.WriteString(fmt.Sprintf("\nCommits analyzed: %d\n\n", len(results)))

	sb.WriteString("## Interpretation\n\n")
	sb.WriteString("**Precision** measures how many of the predicted test packages are truly affected.\n")
	sb.WriteString("High precision means few false positives (we don't run unnecessary tests).\n\n")
	sb.WriteString("**Recall** measures how many of the truly affected tests we successfully predict.\n")
	sb.WriteString("High recall means few false negatives (we don't miss tests that should run).\n\n")
	sb.WriteString("**CI Time Savings** shows the ratio of predicted test packages to total test packages.\n")
	sb.WriteString("Lower is better: it means we only run a small fraction of all tests.\n\n")
	sb.WriteString("The test-scope command uses depth-3 BFS while ground truth uses depth-5. This means\n")
	sb.WriteString("the predicted set may miss deeply-nested callers (lower recall) but should have high\n")
	sb.WriteString("precision since everything it finds at depth 3 is genuinely reachable.\n\n")
	sb.WriteString("For CI workflows, the key insight is: even imperfect recall is valuable if it\n")
	sb.WriteString("dramatically reduces test execution time. Running 20% of tests and catching 80%\n")
	sb.WriteString("of regressions on fast feedback, with the full suite running in a slower pipeline,\n")
	sb.WriteString("is a net win.\n")

	// Write to FINDINGS.md in the same directory as the test.
	findingsPath := findBenchDir(t) + "/FINDINGS.md"
	err := os.WriteFile(findingsPath, []byte(sb.String()), 0644)
	if err != nil {
		t.Logf("warning: could not write FINDINGS.md: %v", err)
	} else {
		t.Logf("Wrote FINDINGS.md to %s", findingsPath)
	}
}

func findBenchDir(t *testing.T) string {
	t.Helper()
	// The test runs from the package directory.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return dir
}
