// Command validate-fixtures checks each ground truth symbol against the actual
// DB contents and reports mismatches. For each unmatched symbol, it suggests
// the closest alternative from the DB using the benchmark's normalization logic.
//
// Usage:
//
//	go run ./bench/cross-system/cmd/validate-fixtures [--fix]
//
// Without --fix: reports mismatches. With --fix: rewrites fixture files.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
	"gopkg.in/yaml.v3"
)

type fixture struct {
	ID          string   `yaml:"id"`
	Repo        string   `yaml:"repo"`
	Tier        string   `yaml:"tier"`
	Description string   `yaml:"description"`
	Source      string   `yaml:"source"`
	GroundTruth []string `yaml:"ground_truth"`
	Tags        []string `yaml:"tags"`
	Notes       string   `yaml:"notes"`
}

type symbolMatch struct {
	groundTruth string
	dbMatch     string // empty if no match found
	normalized  string // what the normalizer produces
}

func main() {
	fix := len(os.Args) > 1 && os.Args[1] == "--fix"

	corpusDir := "bench/cross-system/corpus"
	tasksDir := filepath.Join(corpusDir, "tasks")

	// Open all DBs.
	dbs := map[string]*sql.DB{}
	repos := []string{"flask", "django", "cargo", "kubernetes", "vscode"}
	for _, repo := range repos {
		dbPath := filepath.Join(corpusDir, "repos", repo, ".knowing", "graph.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: cannot open %s: %v\n", dbPath, err)
			continue
		}
		defer db.Close()
		dbs[repo] = db
	}

	// Walk all fixtures.
	var totalSymbols, matched, unmatched, fixed int
	var allMisses []string

	err := filepath.Walk(tasksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var f fixture
		if err := yaml.Unmarshal(data, &f); err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: parse %s: %v\n", path, err)
			return nil
		}

		db, ok := dbs[f.Repo]
		if !ok {
			return nil
		}

		// Check each ground truth symbol.
		var matches []symbolMatch
		var newGT []string
		anyChange := false

		for _, gt := range f.GroundTruth {
			totalSymbols++
			normalized := normalize.Symbol(gt)
			dbMatch := findBestMatch(db, gt, normalized)

			if dbMatch != "" {
				matched++
				matches = append(matches, symbolMatch{gt, dbMatch, normalized})
				newGT = append(newGT, gt) // keep original
			} else {
				unmatched++
				// Try to find a close alternative.
				alt := findAlternative(db, normalized)
				if alt != "" {
					fixed++
					matches = append(matches, symbolMatch{gt, alt, normalized})
					newGT = append(newGT, alt)
					anyChange = true
				} else {
					allMisses = append(allMisses, fmt.Sprintf("  [%s] %s -> (no match, normalized=%q)", f.ID, gt, normalized))
					// Keep original but mark it
					newGT = append(newGT, gt)
				}
			}
		}

		if fix && anyChange {
			f.GroundTruth = newGT
			out, err := yaml.Marshal(&f)
			if err == nil {
				os.WriteFile(path, out, 0644)
			}
		}

		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk error: %v\n", err)
		os.Exit(1)
	}

	// Report.
	fmt.Printf("\n=== Ground Truth Validation ===\n")
	fmt.Printf("Total symbols: %d\n", totalSymbols)
	fmt.Printf("Matched in DB: %d (%.0f%%)\n", matched, float64(matched)*100/float64(totalSymbols))
	fmt.Printf("Unmatched: %d (%.0f%%)\n", unmatched, float64(unmatched)*100/float64(totalSymbols))
	if fix {
		fmt.Printf("Fixed (replaced with alternative): %d\n", fixed)
	}

	if len(allMisses) > 0 && len(allMisses) <= 50 {
		fmt.Printf("\nUnresolvable misses:\n")
		for _, m := range allMisses {
			fmt.Println(m)
		}
	} else if len(allMisses) > 50 {
		fmt.Printf("\nShowing first 50 of %d unresolvable misses:\n", len(allMisses))
		for _, m := range allMisses[:50] {
			fmt.Println(m)
		}
	}
}

// findBestMatch checks if the ground truth symbol can be matched to any node in the DB
// using the benchmark's actual matching logic.
func findBestMatch(db *sql.DB, original, normalized string) string {
	if normalized == "" {
		return ""
	}

	// Strategy 1: check if any qualified_name contains the normalized terminal.
	terminal := lastComponent(normalized)
	if terminal == "" {
		terminal = normalized
	}

	rows, err := db.Query(
		`SELECT qualified_name FROM nodes WHERE qualified_name LIKE ? LIMIT 20`,
		"%"+terminal+"%")
	if err != nil {
		return ""
	}
	defer rows.Close()

	for rows.Next() {
		var qn string
		rows.Scan(&qn)
		if normalize.MatchesGroundTruth(qn, original) {
			return original // original is matchable, keep it
		}
	}
	return ""
}

// findAlternative searches the DB for the closest symbol to the ground truth.
// Returns the normalized DB symbol or empty string.
func findAlternative(db *sql.DB, normalized string) string {
	if normalized == "" {
		return ""
	}

	// Extract the terminal name (most specific part).
	terminal := lastComponent(normalized)
	if terminal == "" {
		terminal = normalized
	}

	// Search for symbols containing the terminal name.
	rows, err := db.Query(
		`SELECT qualified_name FROM nodes WHERE qualified_name LIKE ? LIMIT 50`,
		"%"+terminal+"%")
	if err != nil {
		return ""
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var qn string
		rows.Scan(&qn)
		// Skip test files.
		if strings.Contains(qn, "/test") || strings.Contains(qn, "_test.") {
			continue
		}
		candidates = append(candidates, qn)
	}

	if len(candidates) == 0 {
		return ""
	}

	// Rank by similarity to the normalized ground truth.
	sort.Slice(candidates, func(i, j int) bool {
		return similarity(candidates[i], normalized) > similarity(candidates[j], normalized)
	})

	// Return the normalized form of the best candidate.
	best := normalize.Symbol(candidates[0])
	if best != "" {
		return best
	}
	return ""
}

func similarity(qn, normalized string) int {
	n := normalize.Symbol(qn)
	if n == normalized {
		return 100
	}
	// Count shared dot-separated components.
	aParts := strings.Split(strings.ToLower(n), ".")
	bParts := strings.Split(strings.ToLower(normalized), ".")
	shared := 0
	bSet := map[string]bool{}
	for _, p := range bParts {
		bSet[p] = true
	}
	for _, p := range aParts {
		if bSet[p] {
			shared++
		}
	}
	return shared
}

func lastComponent(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		return s[idx+1:]
	}
	return s
}
