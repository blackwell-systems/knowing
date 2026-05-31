package crosssystem_test

// groundtruth_rewrite_test.go: one-time utility to rewrite ground truth symbols
// to use knowing's actual normalized qualified names from the graph.
//
// Run with: GOWORK=off go test ./bench/cross-system/ -run TestRewriteGroundTruth -v
//
// For each fixture, for each ground truth symbol, this:
// 1. Queries the repo's graph.db for nodes matching the symbol
// 2. Normalizes the matched QN via normalize.Symbol()
// 3. Outputs the mapping: old -> new
//
// Review the output, then apply. Does NOT modify fixtures automatically.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
	"github.com/blackwell-systems/knowing/internal/store"
)

func TestRewriteGroundTruth(t *testing.T) {
	tasks := loadTasks(t, "corpus/tasks")

	// Group tasks by repo
	byRepo := make(map[string][]benchtype.Task)
	for _, task := range tasks {
		byRepo[task.Repo] = append(byRepo[task.Repo], task)
	}

	totalSymbols := 0
	resolved := 0
	unresolved := 0

	for repo, repoTasks := range byRepo {
		dbPath := filepath.Join("corpus", "repos", repo, ".knowing", "graph.db")
		if _, err := os.Stat(dbPath); err != nil {
			t.Logf("SKIP %s: no graph.db", repo)
			continue
		}

		s, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			t.Logf("SKIP %s: %v", repo, err)
			continue
		}

		for _, task := range repoTasks {
			for _, gt := range task.GroundTruth {
				totalSymbols++
				gtNorm := normalize.Symbol(gt)

				// Try to find this symbol in the graph
				// Search by terminal name (last dot component)
				terminal := gtNorm
				if idx := strings.LastIndex(gtNorm, "."); idx >= 0 {
					terminal = gtNorm[idx+1:]
				}

				nodes, err := s.NodesByName(context.Background(), terminal)
				if err != nil || len(nodes) == 0 {
					// Try full normalized name
					nodes, _ = s.NodesByName(context.Background(), gtNorm)
				}

				if len(nodes) == 0 {
					t.Logf("  UNRESOLVED %s | %s -> (no match for %q)", task.ID, gt, terminal)
					unresolved++
					continue
				}

				// Find the best match: the one whose normalized QN matches gtNorm
				bestMatch := ""
				bestNorm := ""
				for _, n := range nodes {
					nNorm := normalize.Symbol(n.QualifiedName)
					if nNorm == gtNorm {
						bestMatch = n.QualifiedName
						bestNorm = nNorm
						break
					}
					// Partial match: contains the ground truth
					if strings.Contains(nNorm, gtNorm) || strings.HasSuffix(nNorm, "."+gtNorm) {
						if bestMatch == "" {
							bestMatch = n.QualifiedName
							bestNorm = nNorm
						}
					}
				}

				if bestMatch == "" {
					// Use first result
					bestMatch = nodes[0].QualifiedName
					bestNorm = normalize.Symbol(bestMatch)
				}

				if bestNorm != gtNorm {
					fmt.Printf("REMAP %s | %s -> %s (was %s)\n", task.ID, gt, bestNorm, gtNorm)
				}
				resolved++
			}
		}
		s.Close()
	}

	t.Logf("\nTotal: %d symbols, %d resolved, %d unresolved", totalSymbols, resolved, unresolved)
}
