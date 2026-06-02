package crosssystem_test

// groundtruth_rewrite_test.go: utility to rewrite ground truth symbols
// to use knowing's actual normalized qualified names from the graph.
//
// Audit mode (default, dry-run):
//   GOWORK=off go test ./bench/cross-system/ -run TestRewriteGroundTruth -v
//
// Apply mode (rewrites fixture files):
//   BENCH_REWRITE_APPLY=1 GOWORK=off go test ./bench/cross-system/ -run TestRewriteGroundTruth -v
//
// Single repo:
//   BENCH_REPOS=django GOWORK=off go test ./bench/cross-system/ -run TestRewriteGroundTruth -v
//
// For each fixture, for each ground truth symbol, this:
// 1. Queries the repo's graph.db for nodes containing the terminal name
// 2. Disambiguates multiple matches using file path hints from fixture notes
// 3. Normalizes the matched QN via normalize.Symbol()
// 4. In apply mode, rewrites the fixture YAML with the improved ground truth

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
	_ "modernc.org/sqlite"
	"gopkg.in/yaml.v3"
)

// rewriteCandidate is a potential graph match for a ground truth symbol.
type rewriteCandidate struct {
	QualifiedName string
	Normalized    string
	FilePath      string // extracted from QN (the ://...file.ext part)
}

func TestRewriteGroundTruth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ground truth rewrite in short mode")
	}
	tasks := loadTasks(t, "corpus/tasks")
	applyMode := os.Getenv("BENCH_REWRITE_APPLY") == "1"
	repoFilter := os.Getenv("BENCH_REPOS")

	// Group tasks by repo, tracking fixture file paths
	type taskWithPath struct {
		Task benchtype.Task
		Path string
	}
	byRepo := make(map[string][]taskWithPath)

	err := filepath.Walk("corpus/tasks", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var task benchtype.Task
		if parseErr := yaml.Unmarshal(data, &task); parseErr != nil {
			return nil
		}
		if task.ID != "" {
			byRepo[task.Repo] = append(byRepo[task.Repo], taskWithPath{Task: task, Path: path})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk tasks: %v", err)
	}

	_ = tasks // loaded for count verification only

	totalSymbols := 0
	resolved := 0
	upgraded := 0 // symbols where we found a better (more qualified) form
	ambiguous := 0
	unresolved := 0
	alreadyGood := 0

	for repo, repoTasks := range byRepo {
		if repoFilter != "" && !strings.Contains(repoFilter, repo) {
			continue
		}

		dbPath := filepath.Join("corpus", "repos", repo, ".knowing", "graph.db")
		if _, statErr := os.Stat(dbPath); statErr != nil {
			t.Logf("SKIP %s: no graph.db", repo)
			continue
		}

		db, openErr := sql.Open("sqlite", dbPath+"?mode=ro")
		if openErr != nil {
			t.Logf("SKIP %s: %v", repo, openErr)
			continue
		}

		t.Logf("=== %s (%d tasks) ===", repo, len(repoTasks))

		for i := range repoTasks {
			twp := &repoTasks[i]
			task := twp.Task
			fileHints := extractFileHints(task.Notes)
			var replacements []gtReplacement

			for _, gt := range task.GroundTruth {
				totalSymbols++
				gtNorm := normalize.Symbol(gt)

				// Extract terminal name for search
				terminal := terminalNameOf(gtNorm)
				if terminal == "" {
					t.Logf("  SKIP %s | %q -> empty terminal", task.ID, gt)
					unresolved++
					continue
				}

				// Query graph.db with %terminal pattern
				candidates := findCandidates(db, terminal)
				if len(candidates) == 0 {
					t.Logf("  UNRESOLVED %s | %s -> no nodes matching %%%s", task.ID, gt, terminal)
					unresolved++
					continue
				}

				// Disambiguate
				best := disambiguate(candidates, gtNorm, fileHints)

				if best == nil {
					t.Logf("  AMBIGUOUS %s | %s -> %d candidates, no clear winner", task.ID, gt, len(candidates))
					for k, c := range candidates {
						if k >= 5 {
							t.Logf("    ... and %d more", len(candidates)-5)
							break
						}
						t.Logf("    [%d] %s", k, c.Normalized)
					}
					ambiguous++
					continue
				}

				resolved++

				if best.Normalized == gtNorm {
					alreadyGood++
					continue
				}

				// We found a better form
				upgraded++
				t.Logf("  UPGRADE %s | %s -> %s", task.ID, gt, best.Normalized)

				if applyMode {
					replacements = append(replacements, gtReplacement{Old: gt, New: best.Normalized})
				}
			}

			if applyMode && len(replacements) > 0 {
				rewriteGroundTruthInFile(t, twp.Path, replacements)
				replacements = replacements[:0]
			}
		}

		db.Close()
	}

	t.Logf("")
	t.Logf("=== SUMMARY ===")
	t.Logf("Total symbols:  %d", totalSymbols)
	t.Logf("Resolved:       %d (%.0f%%)", resolved, pct(resolved, totalSymbols))
	t.Logf("  Already good: %d", alreadyGood)
	t.Logf("  Upgraded:     %d", upgraded)
	t.Logf("Ambiguous:      %d (%.0f%%)", ambiguous, pct(ambiguous, totalSymbols))
	t.Logf("Unresolved:     %d (%.0f%%)", unresolved, pct(unresolved, totalSymbols))
	if applyMode {
		t.Logf("MODE: APPLY (fixtures rewritten)")
	} else {
		t.Logf("MODE: DRY-RUN (set BENCH_REWRITE_APPLY=1 to apply)")
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

// terminalNameOf returns the last dot/:: separated component.
func terminalNameOf(s string) string {
	// Normalize :: to . for Ruby
	s = strings.ReplaceAll(s, "::", ".")
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// findCandidates queries graph.db for nodes whose QN ends with the terminal name.
func findCandidates(db *sql.DB, terminal string) []rewriteCandidate {
	// Use LIKE with %<terminal> to find nodes ending with this name.
	// Also search for %.terminal to catch qualified names.
	rows, err := db.Query(
		`SELECT qualified_name FROM nodes
		 WHERE qualified_name LIKE ?
		    OR qualified_name LIKE ?
		 ORDER BY qualified_name`,
		"%."+terminal, "%::"+terminal,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var candidates []rewriteCandidate
	seen := make(map[string]bool)
	for rows.Next() {
		var qn string
		if scanErr := rows.Scan(&qn); scanErr != nil {
			continue
		}
		norm := normalize.Symbol(qn)
		if seen[norm] {
			continue
		}
		seen[norm] = true
		candidates = append(candidates, rewriteCandidate{
			QualifiedName: qn,
			Normalized:    norm,
			FilePath:      extractFilePath(qn),
		})
	}
	return candidates
}

// extractFilePath pulls the file path from a knowing QN.
// "repo://path/to/file.py.ClassName.method" -> "path/to/file.py"
func extractFilePath(qn string) string {
	idx := strings.Index(qn, "://")
	if idx < 0 {
		return ""
	}
	rest := qn[idx+3:]
	// Find the file extension to know where the file path ends
	exts := []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java", ".cs", ".rb"}
	for _, ext := range exts {
		if extIdx := strings.Index(rest, ext+"."); extIdx >= 0 {
			return rest[:extIdx+len(ext)]
		}
		if strings.HasSuffix(rest, ext) {
			return rest
		}
	}
	return rest
}

// disambiguate picks the best candidate for a ground truth symbol.
// Strategy:
// 1. Exact normalized match -> done
// 2. File path hint match -> filter to those candidates
// 3. Suffix/prefix match with ground truth qualifiers
// 4. Shortest qualifying match (most specific without over-qualifying)
func disambiguate(candidates []rewriteCandidate, gtNorm string, fileHints []string) *rewriteCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}

	// Strategy 1: exact normalized match (case-sensitive)
	for i := range candidates {
		if candidates[i].Normalized == gtNorm {
			return &candidates[i]
		}
	}

	// Strategy 2: suffix match with file hint disambiguation.
	// Runs before case-insensitive exact because bare ground truth like
	// "collect" shouldn't match class "Collect" when file hints point to
	// "Collector.collect" in deletion.py.
	var suffixMatches []int
	for i := range candidates {
		cn := candidates[i].Normalized
		if strings.HasSuffix(cn, "."+gtNorm) || strings.HasSuffix(cn, "::"+gtNorm) {
			suffixMatches = append(suffixMatches, i)
		}
	}

	if len(suffixMatches) > 0 {
		// Try file hint disambiguation first
		if len(fileHints) > 0 {
			for _, idx := range suffixMatches {
				if matchesFileHint(candidates[idx].FilePath, fileHints) {
					return &candidates[idx]
				}
			}
		}
		// If only one suffix match, use it
		if len(suffixMatches) == 1 {
			return &candidates[suffixMatches[0]]
		}
	}

	// Strategy 3: case-insensitive exact match (after suffix+hint)
	for i := range candidates {
		if strings.EqualFold(candidates[i].Normalized, gtNorm) {
			return &candidates[i]
		}
	}

	// Strategy 4: qualifier overlap (ground truth has qualifiers that match)
	// e.g., gtNorm="Collector.can_fast_delete" matches candidate with "Collector" in QN
	if strings.Contains(gtNorm, ".") || strings.Contains(gtNorm, "::") {
		gtParts := strings.FieldsFunc(gtNorm, func(r rune) bool { return r == '.' || r == ':' })
		var qualMatches []int
		for i := range candidates {
			cn := candidates[i].Normalized
			cnParts := strings.FieldsFunc(cn, func(r rune) bool { return r == '.' || r == ':' })
			shared := 0
			for _, gp := range gtParts {
				for _, cp := range cnParts {
					if strings.EqualFold(gp, cp) {
						shared++
					}
				}
			}
			if shared >= 2 { // terminal + at least one qualifier
				qualMatches = append(qualMatches, i)
			}
		}
		if len(qualMatches) == 1 {
			return &candidates[qualMatches[0]]
		}
		// Multiple qualifier matches + file hint
		if len(qualMatches) > 1 && len(fileHints) > 0 {
			for _, idx := range qualMatches {
				if matchesFileHint(candidates[idx].FilePath, fileHints) {
					return &candidates[idx]
				}
			}
		}
	}

	// Strategy 5: file hint as last resort on all candidates
	if len(fileHints) > 0 {
		var hintMatches []int
		for i := range candidates {
			if matchesFileHint(candidates[i].FilePath, fileHints) {
				hintMatches = append(hintMatches, i)
			}
		}
		if len(hintMatches) == 1 {
			return &candidates[hintMatches[0]]
		}
	}

	// Can't disambiguate
	return nil
}

// matchesFileHint checks if a file path matches any of the file hints.
func matchesFileHint(filePath string, hints []string) bool {
	for _, hint := range hints {
		// Normalize both paths for comparison
		fp := strings.ToLower(filePath)
		h := strings.ToLower(hint)
		if strings.HasSuffix(fp, h) || strings.Contains(fp, h) {
			return true
		}
	}
	return false
}

// extractFileHints pulls file paths from fixture notes.
// Looks for patterns like "Modified files: path/to/file.py, path/to/file2.py"
var fileHintRe = regexp.MustCompile(`(?:Modified files|Files?|modified):\s*(.+)`)

func extractFileHints(notes string) []string {
	if notes == "" {
		return nil
	}
	matches := fileHintRe.FindStringSubmatch(notes)
	if matches == nil {
		return nil
	}
	raw := matches[1]
	parts := strings.Split(raw, ",")
	var hints []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			hints = append(hints, p)
		}
	}
	return hints
}

// gtReplacement is an old->new ground truth symbol mapping.
type gtReplacement struct {
	Old string
	New string
}

// rewriteGroundTruthInFile does targeted string replacement of ground truth
// entries in a YAML fixture file, preserving all other fields and formatting.
func rewriteGroundTruthInFile(t *testing.T, path string, replacements []gtReplacement) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read %s: %v", path, err)
		return
	}
	content := string(data)
	for _, r := range replacements {
		// Match the YAML list entry: "- old_value" with various indentation
		// Try exact line match first (handles both "- value" and "  - value")
		oldLine := "- " + r.Old
		newLine := "- " + r.New
		if strings.Contains(content, oldLine) {
			content = strings.Replace(content, oldLine, newLine, 1)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Errorf("failed to write %s: %v", path, err)
		return
	}
	t.Logf("  WROTE %s", path)
}

// TestRewriteGroundTruthStats shows per-repo resolution stats without full detail.
func TestRewriteGroundTruthStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping ground truth stats in short mode")
	}
	tasks := loadTasks(t, "corpus/tasks")
	repoFilter := os.Getenv("BENCH_REPOS")

	byRepo := make(map[string][]benchtype.Task)
	for _, task := range tasks {
		byRepo[task.Repo] = append(byRepo[task.Repo], task)
	}

	fmt.Printf("\n%-15s %6s %8s %8s %8s %8s\n", "Repo", "Syms", "Resolved", "Upgraded", "Ambig", "Unresolved")
	fmt.Printf("%s\n", strings.Repeat("-", 65))

	totalAll, resolvedAll, upgradedAll, ambigAll, unresolvedAll := 0, 0, 0, 0, 0

	for repo, repoTasks := range byRepo {
		if repoFilter != "" && !strings.Contains(repoFilter, repo) {
			continue
		}

		dbPath := filepath.Join("corpus", "repos", repo, ".knowing", "graph.db")
		if _, statErr := os.Stat(dbPath); statErr != nil {
			continue
		}
		db, openErr := sql.Open("sqlite", dbPath+"?mode=ro")
		if openErr != nil {
			continue
		}

		total, resolved, upgraded, ambig, unresolved := 0, 0, 0, 0, 0

		for _, task := range repoTasks {
			fileHints := extractFileHints(task.Notes)
			for _, gt := range task.GroundTruth {
				total++
				gtNorm := normalize.Symbol(gt)
				terminal := terminalNameOf(gtNorm)
				if terminal == "" {
					unresolved++
					continue
				}
				candidates := findCandidates(db, terminal)
				if len(candidates) == 0 {
					unresolved++
					continue
				}
				best := disambiguate(candidates, gtNorm, fileHints)
				if best == nil {
					ambig++
					continue
				}
				resolved++
				if best.Normalized != gtNorm {
					upgraded++
				}
			}
		}
		db.Close()

		fmt.Printf("%-15s %6d %8d %8d %8d %8d\n", repo, total, resolved, upgraded, ambig, unresolved)
		totalAll += total
		resolvedAll += resolved
		upgradedAll += upgraded
		ambigAll += ambig
		unresolvedAll += unresolved
	}

	fmt.Printf("%s\n", strings.Repeat("-", 65))
	fmt.Printf("%-15s %6d %8d %8d %8d %8d\n", "TOTAL", totalAll, resolvedAll, upgradedAll, ambigAll, unresolvedAll)
}
