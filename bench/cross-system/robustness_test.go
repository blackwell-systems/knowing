package crosssystem_test

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
)

// TestQueryRobustness measures how stable each system's output is when the same
// task is rephrased 5 different ways. RWR-based systems should be stable (structural
// signal is independent of phrasing); string-matching systems should be volatile.
func TestQueryRobustness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping robustness test in short mode")
	}

	repoPath := filepath.Join("corpus", "repos", "flask")

	// 3 tasks, each rephrased 5 ways. Same intent, different words.
	type rephrasedTask struct {
		id          string
		variants    []string
		groundTruth []string // substrings to match in results
	}

	tasks := []rephrasedTask{
		{
			id: "before-request-hook",
			variants: []string{
				"add a before_request hook that validates API keys",
				"implement request preprocessing to check authentication tokens",
				"create middleware that runs before each route to verify credentials",
				"add a function that intercepts incoming requests for API key validation",
				"set up pre-request validation logic for authentication",
			},
			groundTruth: []string{"before_request", "scaffold", "Scaffold", "app"},
		},
		{
			id: "blueprint-registration",
			variants: []string{
				"register a new Flask blueprint for user profiles",
				"add a modular route group for the user management section",
				"create a blueprint and attach it to the application",
				"set up a separate URL namespace for profile-related endpoints",
				"implement a pluggable module for user account routes",
			},
			groundTruth: []string{"blueprint", "Blueprint", "register"},
		},
		{
			id: "error-handling",
			variants: []string{
				"add a custom error handler for 404 responses",
				"implement a page-not-found handler that returns JSON",
				"register an errorhandler for HTTP 404 status codes",
				"create a function that catches missing route errors",
				"set up custom exception handling for not-found pages",
			},
			groundTruth: []string{"error", "exception", "handler", "404"},
		},
	}

	type systemResult struct {
		name       string
		variances  []float64 // Jaccard variance per task
		gtHits     [][]int   // ground truth hits per variant per task
	}
	var results []systemResult

	for _, adapter := range adapters.Available() {
		name := adapter.Name()
		if name == "grep" {
			continue
		}

		if _, err := adapter.Index(repoPath); err != nil {
			t.Logf("  %s: index failed (skipping)", name)
			continue
		}

		t.Logf("=== %s ===", name)
		sr := systemResult{name: name}

		for _, task := range tasks {
			var allSymbols [][]string
			var hits []int

			for _, variant := range task.variants {
				btTask := benchtype.Task{ID: task.id, Description: variant}
				res, _ := adapter.Retrieve(repoPath, btTask, 5000)

				// Collect top-10 symbol names.
				var syms []string
				top := 10
				if len(res.Symbols) < top {
					top = len(res.Symbols)
				}
				for i := 0; i < top; i++ {
					syms = append(syms, res.Symbols[i].QualifiedName)
				}
				allSymbols = append(allSymbols, syms)

				// Count ground truth hits.
				h := 0
				for i := 0; i < top; i++ {
					qn := strings.ToLower(res.Symbols[i].QualifiedName)
					for _, gt := range task.groundTruth {
						if strings.Contains(qn, strings.ToLower(gt)) {
							h++
							break
						}
					}
				}
				hits = append(hits, h)
			}

			// Compute pairwise Jaccard similarity across all 5 variants.
			var jaccards []float64
			for i := 0; i < len(allSymbols); i++ {
				for j := i + 1; j < len(allSymbols); j++ {
					jaccards = append(jaccards, jaccard(allSymbols[i], allSymbols[j]))
				}
			}
			meanJaccard := mean(jaccards)
			sr.variances = append(sr.variances, meanJaccard)
			sr.gtHits = append(sr.gtHits, hits)

			t.Logf("  %s: mean Jaccard=%.2f, GT hits=%v", task.id, meanJaccard, hits)
		}

		results = append(results, sr)
	}

	// Summary.
	t.Log("")
	t.Log("=== Query Robustness Summary ===")
	t.Log("  Jaccard similarity: 1.0 = identical output for all rephrasings, 0.0 = completely different")
	t.Log("")
	t.Logf("  | System    | Task 1 | Task 2 | Task 3 | Mean   | Verdict |")
	t.Logf("  |-----------|--------|--------|--------|--------|---------|")
	for _, sr := range results {
		avg := mean(sr.variances)
		verdict := "STABLE"
		if avg < 0.5 {
			verdict = "VOLATILE"
		} else if avg < 0.8 {
			verdict = "MODERATE"
		}
		t.Logf("  | %-9s | %.2f   | %.2f   | %.2f   | %.2f   | %s |",
			sr.name, sr.variances[0], sr.variances[1], sr.variances[2], avg, verdict)
	}

	fmt.Println()
}

// jaccard computes the Jaccard similarity between two string slices.
func jaccard(a, b []string) float64 {
	setA := make(map[string]bool, len(a))
	for _, s := range a {
		setA[s] = true
	}
	setB := make(map[string]bool, len(b))
	for _, s := range b {
		setB[s] = true
	}

	intersection := 0
	for s := range setA {
		if setB[s] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return math.Round(sum/float64(len(values))*100) / 100
}
