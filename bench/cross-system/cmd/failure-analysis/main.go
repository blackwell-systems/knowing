// Command failure-analysis examines what knowing returns vs ground truth
// for each task, categorizing misses into: related-but-unlisted, noise,
// wrong-package, correct-package-wrong-symbol.
//
// Usage:
//
//	go run ./bench/cross-system/cmd/failure-analysis [--repo flask] [--task flask-easy-001]
package main

import (
	stdctx "context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
)

type fixture struct {
	ID          string   `yaml:"id"`
	Repo        string   `yaml:"repo"`
	Tier        string   `yaml:"tier"`
	Description string   `yaml:"description"`
	GroundTruth []string `yaml:"ground_truth"`
}

type missCategory struct {
	name  string
	count int
}

func main() {
	filterRepo := ""
	filterTask := ""
	for i, arg := range os.Args[1:] {
		if arg == "--repo" && i+2 < len(os.Args) {
			filterRepo = os.Args[i+2]
		}
		if arg == "--task" && i+2 < len(os.Args) {
			filterTask = os.Args[i+2]
		}
	}

	corpusDir := "bench/cross-system/corpus"

	// Categories for misses.
	var totalReturned, totalMatched int
	categories := map[string]int{
		"same_package":   0, // returned symbol is in same package as a GT symbol
		"related_name":   0, // returned symbol name contains a GT keyword
		"test_symbol":    0, // returned symbol is from a test file
		"noise":          0, // no apparent relationship to ground truth
	}

	// Walk fixtures.
	filepath.Walk(filepath.Join(corpusDir, "tasks"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}

		data, _ := os.ReadFile(path)
		var f fixture
		yaml.Unmarshal(data, &f)

		if filterRepo != "" && f.Repo != filterRepo {
			return nil
		}
		if filterTask != "" && f.ID != filterTask {
			return nil
		}

		dbPath := filepath.Join(corpusDir, "repos", f.Repo, ".knowing", "graph.db")
		s, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			return nil
		}
		defer s.Close()

		ctx := stdctx.Background()
		engine := knowingctx.NewContextEngine(s)
		result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: f.Description,
			TokenBudget:     5000,
			Format:          "json",
		})
		if err != nil || result == nil {
			return nil
		}

		// Normalize ground truth.
		gtNormalized := make([]string, len(f.GroundTruth))
		gtPackages := make(map[string]bool)
		gtKeywords := make(map[string]bool)
		for i, gt := range f.GroundTruth {
			gtNormalized[i] = normalize.Symbol(gt)
			// Extract package (everything before last dot).
			if idx := strings.LastIndex(gtNormalized[i], "."); idx > 0 {
				gtPackages[strings.ToLower(gtNormalized[i][:idx])] = true
			}
			// Extract keywords from GT symbol names.
			parts := strings.FieldsFunc(gtNormalized[i], func(r rune) bool {
				return r == '.' || r == '_'
			})
			for _, p := range parts {
				if len(p) > 3 {
					gtKeywords[strings.ToLower(p)] = true
				}
			}
		}

		// Analyze top-10 results.
		limit := 10
		if len(result.Symbols) < limit {
			limit = len(result.Symbols)
		}

		matched := 0
		var misses []string
		for i := 0; i < limit; i++ {
			sym := result.Symbols[i]
			retrieved := sym.Node.QualifiedName
			retrievedNorm := normalize.Symbol(retrieved)

			// Check if it matches ground truth.
			isMatch := false
			for _, gt := range f.GroundTruth {
				if normalize.MatchesGroundTruth(retrieved, gt) {
					isMatch = true
					break
				}
			}

			if isMatch {
				matched++
				totalMatched++
			} else {
				totalReturned++
				// Categorize the miss.
				category := categorizeMiss(retrievedNorm, retrieved, gtPackages, gtKeywords)
				categories[category]++
				if filterTask != "" {
					misses = append(misses, fmt.Sprintf("  [%d] %s -> %s (%s)", i+1, retrievedNorm, category, sym.Node.Kind))
				}
			}
		}

		if filterTask != "" {
			fmt.Printf("\n=== %s ===\n", f.ID)
			fmt.Printf("Description: %s\n", f.Description)
			fmt.Printf("Ground truth (%d):\n", len(f.GroundTruth))
			for _, gt := range f.GroundTruth {
				fmt.Printf("  - %s -> %s\n", gt, normalize.Symbol(gt))
			}
			fmt.Printf("\nReturned top-10 (%d matched, %d missed):\n", matched, len(misses))
			for _, m := range misses {
				fmt.Println(m)
			}
		}

		return nil
	})

	if filterTask == "" {
		total := totalMatched + totalReturned
		fmt.Printf("\n=== FAILURE ANALYSIS ===\n")
		fmt.Printf("Total top-10 results examined: %d\n", total)
		fmt.Printf("Matched ground truth: %d (%.1f%%)\n", totalMatched, float64(totalMatched)*100/float64(total))
		fmt.Printf("Missed: %d (%.1f%%)\n", totalReturned, float64(totalReturned)*100/float64(total))
		fmt.Printf("\nMiss categories:\n")
		fmt.Printf("  same_package:  %d (%.1f%%) - symbol from same package as a GT symbol\n", categories["same_package"], float64(categories["same_package"])*100/float64(totalReturned))
		fmt.Printf("  related_name:  %d (%.1f%%) - symbol name contains a GT keyword\n", categories["related_name"], float64(categories["related_name"])*100/float64(totalReturned))
		fmt.Printf("  test_symbol:   %d (%.1f%%) - symbol from a test file\n", categories["test_symbol"], float64(categories["test_symbol"])*100/float64(totalReturned))
		fmt.Printf("  noise:         %d (%.1f%%) - no apparent relationship\n", categories["noise"], float64(categories["noise"])*100/float64(totalReturned))
	}
}

func categorizeMiss(normalized, raw string, gtPackages map[string]bool, gtKeywords map[string]bool) string {
	lower := strings.ToLower(normalized)
	rawLower := strings.ToLower(raw)

	// Test file?
	if strings.Contains(rawLower, "/test") || strings.Contains(rawLower, "_test.") || strings.Contains(rawLower, "test_") {
		return "test_symbol"
	}

	// Same package as a GT symbol?
	if idx := strings.LastIndex(lower, "."); idx > 0 {
		pkg := lower[:idx]
		if gtPackages[pkg] {
			return "same_package"
		}
	}

	// Name contains a GT keyword?
	parts := strings.FieldsFunc(lower, func(r rune) bool {
		return r == '.' || r == '_' || r == '/' || r == ':'
	})
	for _, p := range parts {
		if len(p) > 3 && gtKeywords[p] {
			return "related_name"
		}
	}

	return "noise"
}
