package crosssystem_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
)

func TestDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping determinism test in short mode")
	}

	// Use Flask (fast to query, all systems can handle it).
	repoPath := filepath.Join("corpus", "repos", "flask")

	tasks := []benchtype.Task{
		{ID: "det-001", Repo: "flask", Description: "add a before_request hook that validates API keys"},
		{ID: "det-002", Repo: "flask", Description: "register a new Flask blueprint for user profiles"},
		{ID: "det-003", Repo: "flask", Description: "implement session-based authentication with login"},
	}

	const runs = 10

	type systemResult struct {
		name         string
		uniquePerTask []int // unique outputs per task
	}
	var results []systemResult

	for _, adapter := range adapters.Available() {
		name := adapter.Name()
		if name == "grep" {
			continue // grep is deterministic by definition (rg is stateless)
		}

		t.Logf("=== %s ===", name)
		if _, err := adapter.Index(repoPath); err != nil {
			t.Logf("  %s: index failed: %v (skipping)", name, err)
			continue
		}

		sr := systemResult{name: name}

		for _, task := range tasks {
			seen := make(map[string]int) // hash of output -> count

			for i := 0; i < runs; i++ {
				res, err := adapter.Retrieve(repoPath, task, 5000)
				if err != nil || res.Error != "" {
					seen["ERROR"]++
					continue
				}

				// Hash the ordered symbol list to detect output variation.
				h := hashOutput(res.Symbols)
				seen[h]++
			}

			unique := len(seen)
			sr.uniquePerTask = append(sr.uniquePerTask, unique)

			if unique == 1 {
				t.Logf("  %s: %d/%d runs identical (deterministic)", task.ID, runs, runs)
			} else {
				t.Logf("  %s: %d unique outputs in %d runs (NON-DETERMINISTIC)", task.ID, unique, runs)
				for h, count := range seen {
					t.Logf("    output %s...: %d/%d runs", h[:12], count, runs)
				}
			}
		}

		results = append(results, sr)
	}

	// Summary.
	t.Log("")
	t.Log("=== Determinism Summary ===")
	t.Logf("  | System    | Task 1 | Task 2 | Task 3 | Verdict |")
	t.Logf("  |-----------|--------|--------|--------|---------|")
	for _, sr := range results {
		verdict := "DETERMINISTIC"
		for _, u := range sr.uniquePerTask {
			if u > 1 {
				verdict = "NON-DETERMINISTIC"
				break
			}
		}
		cols := make([]string, len(sr.uniquePerTask))
		for i, u := range sr.uniquePerTask {
			if u == 1 {
				cols[i] = "1 (ok)"
			} else {
				cols[i] = fmt.Sprintf("%d (!)", u)
			}
		}
		t.Logf("  | %-9s | %-6s | %-6s | %-6s | %s |",
			sr.name, cols[0], cols[1], cols[2], verdict)
	}
}

// hashOutput produces a deterministic hash of the ordered symbol list.
func hashOutput(symbols []benchtype.RetrievedSymbol) string {
	var b strings.Builder
	for _, s := range symbols {
		b.WriteString(s.QualifiedName)
		b.WriteByte('\n')
	}
	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", h)
}
