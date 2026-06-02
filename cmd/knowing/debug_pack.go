package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdDebugPack shows packing decisions for a task: which symbols were packed,
// which were skipped, and why (budget, density, proximity).
// Usage: knowing debug-pack -task "description" [-db path] [-budget N] <repo-path>
func cmdDebugPack(args []string) error {
	fs := flag.NewFlagSet("debug-pack", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	task := fs.String("task", "", "Task description (required)")
	budget := fs.Int("budget", 5000, "Token budget")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	top := fs.Int("top", 30, "Maximum symbols to display")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *task == "" {
		return fmt.Errorf("usage: knowing debug-pack -task \"description\" [-db path] [-budget N] [repo-path]")
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	repoPath, _ = filepath.Abs(repoPath)

	if *repoURL == "" {
		*repoURL = detectRepoURL(repoPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	engine := knowingctx.NewContextEngine(st)
	engine.DisablePersistentCache()

	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: *task,
		TokenBudget:     *budget,
		RepoURL:         *repoURL,
	})
	if err != nil {
		return fmt.Errorf("ForTask: %v", err)
	}

	if block == nil || len(block.Symbols) == 0 {
		fmt.Println("No symbols packed.")
		return nil
	}

	// Build packed set for marking.
	packedSet := make(map[string]int) // QN -> rank
	for i, sym := range block.Symbols {
		packedSet[sym.Node.QualifiedName] = i + 1
	}

	fmt.Printf("=== Packing Debug ===\n")
	fmt.Printf("Task: %q\n", *task)
	fmt.Printf("Budget: %d tokens\n", *budget)
	fmt.Printf("Packed: %d symbols, %d tokens\n", len(block.Symbols), block.TokensUsed)
	fmt.Printf("PackRoot: %s\n\n", block.PackRoot.String()[:16])

	// Show packed symbols with details.
	fmt.Printf("--- Packed Symbols (by density rank) ---\n")
	fmt.Printf("%4s  %-45s  %6s  %5s  %6s  %5s\n", "RANK", "SYMBOL", "SCORE", "COST", "DENSTY", "RWR")
	fmt.Printf("%4s  %-45s  %6s  %5s  %6s  %5s\n", "----", "------", "-----", "----", "------", "---")

	shown := 0
	for i, sym := range block.Symbols {
		if shown >= *top {
			fmt.Printf("\n... truncated at %d (use -top to show more)\n", *top)
			break
		}
		name := terminalSymbol(sym.Node.QualifiedName)
		if len(name) > 45 {
			name = name[:42] + "..."
		}
		cost := knowingctx.EstimateNodeTokensForFormat(sym.Node, "xml")
		density := 0.0
		if cost > 0 {
			density = sym.Score / float64(cost)
		}
		fmt.Printf("%4d  %-45s  %6.3f  %5d  %6.4f  %5.3f\n",
			i+1, name, sym.Score, cost, density, sym.RWRScore)
		shown++
	}

	// Summary stats.
	if len(block.Symbols) > 0 {
		first := block.Symbols[0]
		last := block.Symbols[len(block.Symbols)-1]
		fmt.Printf("\nTop score: %.3f  Bottom score: %.3f  Ratio: %.1fx\n",
			first.Score, last.Score, first.Score/last.Score)
	}

	// Show what was NOT packed (symbols that scored but didn't make budget).
	// We can't recover the full ranked list from ForTask, but we can show
	// the budget utilization.
	remaining := *budget - block.TokensUsed
	fmt.Printf("\nBudget: %d/%d used (%d remaining, %.0f%% utilization)\n",
		block.TokensUsed, *budget, remaining,
		float64(block.TokensUsed)/float64(*budget)*100)

	// File distribution.
	fileCount := make(map[string]int)
	for _, sym := range block.Symbols {
		qn := sym.Node.QualifiedName
		// Extract file path from QN.
		if idx := strings.Index(qn, "://"); idx >= 0 {
			path := qn[idx+3:]
			if dotIdx := strings.LastIndex(path, ".go."); dotIdx >= 0 {
				path = path[:dotIdx+3]
			} else if dotIdx := strings.LastIndex(path, ".py."); dotIdx >= 0 {
				path = path[:dotIdx+3]
			} else if dotIdx := strings.LastIndex(path, ".ts."); dotIdx >= 0 {
				path = path[:dotIdx+3]
			}
			// Take last 2 path components for readability.
			parts := strings.Split(path, "/")
			if len(parts) > 2 {
				path = strings.Join(parts[len(parts)-2:], "/")
			}
			fileCount[path]++
		}
	}
	if len(fileCount) > 0 {
		fmt.Printf("\nFile distribution (%d files):\n", len(fileCount))
		// Sort by count descending.
		type fc struct {
			file  string
			count int
		}
		var sorted []fc
		for f, c := range fileCount {
			sorted = append(sorted, fc{f, c})
		}
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j].count > sorted[i].count {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		for i, s := range sorted {
			if i >= 10 {
				fmt.Printf("  ... and %d more files\n", len(sorted)-10)
				break
			}
			fmt.Printf("  %3d  %s\n", s.count, s.file)
		}
	}

	return nil
}
