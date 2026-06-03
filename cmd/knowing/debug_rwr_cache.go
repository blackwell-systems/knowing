package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"
	"time"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdDebugRWRCache shows the RWR cache state for a task description.
// Usage: knowing debug-rwr-cache -task "description" -db <path> [repo-path]
func cmdDebugRWRCache(args []string) error {
	fs := flag.NewFlagSet("debug-rwr-cache", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	task := fs.String("task", "", "Task description to compute cache key for")
	showStats := fs.Bool("stats", false, "Show global cache hit/miss stats")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	if *showStats {
		hits, misses := knowingctx.RWRCacheStats()
		total := hits + misses
		hitRate := 0.0
		if total > 0 {
			hitRate = float64(hits) / float64(total) * 100
		}
		fmt.Printf("=== RWR Cache Stats ===\n")
		fmt.Printf("Hits:    %d\n", hits)
		fmt.Printf("Misses:  %d\n", misses)
		fmt.Printf("Total:   %d\n", total)
		fmt.Printf("Hit rate: %.1f%%\n", hitRate)

		// Count RWR cache entries in the notes table.
		var count int
		row := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_notes WHERE key = 'rwr_cache'`)
		if err := row.Scan(&count); err == nil {
			fmt.Printf("Entries:  %d (in notes table)\n", count)
		}
		return nil
	}

	if *task == "" {
		return fmt.Errorf("either -task or -stats is required")
	}

	// Determine repo URL.
	var repoURL string
	if repos, err := st.AllRepos(ctx); err == nil && len(repos) > 0 {
		repoURL = repos[0].RepoURL
	}

	// Run the retrieval pipeline to get seeds (same as ForTask).
	engine := knowingctx.NewContextEngine(st)
	engine.DisablePersistentCache()

	// Extract keywords to show what seeds would be selected.
	ks := knowingctx.ExtractKeywordSet(*task)
	fmt.Printf("=== RWR Cache Debug ===\n")
	fmt.Printf("Task: %s\n", *task)
	fmt.Printf("Keywords (primary): %v\n", ks.Primary())
	fmt.Printf("Keywords (all): %v\n\n", ks.All())

	// Get the snapshot hash (used in cache key).
	snapHash := types.EmptyHash
	if repos, err := st.AllRepos(ctx); err == nil && len(repos) > 0 {
		if snap, err := st.LatestSnapshot(ctx, repos[0].RepoHash); err == nil && snap != nil {
			snapHash = snap.SnapshotHash
			fmt.Printf("Snapshot: %x (latest)\n", snapHash[:8])
		}
	}
	if snapHash == types.EmptyHash {
		fmt.Println("Snapshot: none (cache key will be empty, no caching)")
	}

	// Run ForTask to populate the cache.
	fmt.Println("\nRunning ForTask (populates cache)...")
	start := time.Now()
	knowingctx.RWRCacheEnabled = true
	result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: *task,
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	coldLatency := time.Since(start)
	if err != nil {
		return fmt.Errorf("ForTask: %w", err)
	}
	fmt.Printf("Cold: %s (%d symbols, %d tokens)\n", coldLatency, len(result.Symbols), result.TokensUsed)

	hits1, misses1 := knowingctx.RWRCacheStats()

	// Run again with the SAME engine to test cache hit.
	// In production, the MCP server reuses the engine across queries.
	engine.DisablePersistentCache() // only disables context pack cache, not RWR cache
	start = time.Now()
	result2, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: *task,
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	warmLatency := time.Since(start)
	if err != nil {
		return fmt.Errorf("ForTask(warm): %w", err)
	}
	fmt.Printf("Warm: %s (%d symbols, %d tokens)\n", warmLatency, len(result2.Symbols), result2.TokensUsed)

	hits2, misses2 := knowingctx.RWRCacheStats()

	fmt.Printf("\nCache activity:\n")
	fmt.Printf("  Cold run: hits=%d misses=%d\n", hits1, misses1)
	fmt.Printf("  Warm run: hits=%d misses=%d (delta: +%d hits)\n", hits2, misses2, hits2-hits1)

	speedup := 0.0
	if warmLatency > 0 {
		speedup = float64(coldLatency) / float64(warmLatency)
	}
	fmt.Printf("\nSpeedup: %.1fx (%s -> %s)\n", speedup, coldLatency, warmLatency)

	// Show top symbols from both runs to verify identical results.
	fmt.Printf("\nTop 5 symbols (cold):\n")
	showTop := 5
	if len(result.Symbols) < showTop {
		showTop = len(result.Symbols)
	}
	for i := 0; i < showTop; i++ {
		s := result.Symbols[i]
		parts := strings.Split(s.Node.QualifiedName, "://")
		name := s.Node.QualifiedName
		if len(parts) > 1 {
			name = parts[1]
		}
		fmt.Printf("  [%d] %.3f %s\n", i+1, s.Score, name)
	}

	// Verify identical results.
	if len(result.Symbols) == len(result2.Symbols) {
		identical := true
		for i := range result.Symbols {
			if result.Symbols[i].Node.NodeHash != result2.Symbols[i].Node.NodeHash {
				identical = false
				break
			}
		}
		if identical {
			fmt.Println("\n✓ Cold and warm results are identical (cache is correct)")
		} else {
			fmt.Println("\n✗ Cold and warm results DIFFER (cache may have a key mismatch)")
		}
	}

	// Count total cache entries.
	var count int
	row := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_notes WHERE key = 'rwr_cache'`)
	if err := row.Scan(&count); err == nil {
		fmt.Printf("\nTotal RWR cache entries: %d\n", count)
	}

	// Show cache entry sizes.
	var totalSize int
	row = st.DB().QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(value)), 0) FROM graph_notes WHERE key = 'rwr_cache'`)
	if err := row.Scan(&totalSize); err == nil {
		fmt.Printf("Total cache size: %d bytes (%.1f KB)\n", totalSize, float64(totalSize)/1024)
	}

	_ = repoURL
	_ = sort.Strings // used in imports
	return nil
}
