// Package merkle_diff benchmarks the Phase 2 subgraph cache end-to-end:
// context_for_task cache hit vs miss, blast_radius cache benefit,
// daemon invalidation latency, LRU cache hit rate, and hash prefix overhead.
package merkle_diff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/cache"
	knowctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestPhase2CacheBenchmark(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("not in knowing repo root")
	}

	tmpDB := filepath.Join(t.TempDir(), "bench.db")
	st, err := store.NewSQLiteStore(tmpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Indexed: %d nodes, %d edges", snap.NodeCount, snap.EdgeCount)

	// Create subgraph cache.
	sc := cache.NewSubgraphCache(cache.SubgraphCacheOptions{}) // default: 10K entries, 1h TTL

	// --- Benchmark 1: context_for_task cold vs warm ---
	t.Run("ContextForTask_ColdVsWarm", func(t *testing.T) {
		engine := knowctx.NewContextEngine(st)
		engine.SetCache(sc)

		tasks := []string{
			"find all MCP tool handlers",
			"find authentication and authorization code",
			"find database connection and query code",
			"find the context retrieval pipeline",
			"find test scope computation",
		}

		var coldTotal, warmTotal time.Duration
		for _, task := range tasks {
			// Cold call (cache miss).
			start := time.Now()
			block1, err := engine.ForTask(ctx, knowctx.TaskOptions{
				TaskDescription: task,
				TokenBudget:     50000,
				Format:          "xml",
			})
			if err != nil {
				t.Fatal(err)
			}
			coldDur := time.Since(start)

			// Warm call (cache hit).
			start = time.Now()
			block2, err := engine.ForTask(ctx, knowctx.TaskOptions{
				TaskDescription: task,
				TokenBudget:     50000,
				Format:          "xml",
			})
			if err != nil {
				t.Fatal(err)
			}
			warmDur := time.Since(start)

			speedup := float64(coldDur) / float64(warmDur)
			t.Logf("Task: %q", task)
			t.Logf("  Cold: %v (%d symbols, PackRoot=%s)", coldDur, len(block1.Symbols), block1.PackRoot)
			t.Logf("  Warm: %v (%d symbols, PackRoot=%s)", warmDur, len(block2.Symbols), block2.PackRoot)
			t.Logf("  Speedup: %.0fx", speedup)

			if block1.PackRoot != block2.PackRoot {
				t.Errorf("PackRoot mismatch: %s != %s", block1.PackRoot, block2.PackRoot)
			}

			coldTotal += coldDur
			warmTotal += warmDur
		}

		avgCold := coldTotal / time.Duration(len(tasks))
		avgWarm := warmTotal / time.Duration(len(tasks))
		avgSpeedup := float64(avgCold) / float64(avgWarm)
		t.Logf("\nAverage cold: %v, Average warm: %v, Average speedup: %.0fx", avgCold, avgWarm, avgSpeedup)

		stats := sc.Stats()
		t.Logf("Cache stats: hits=%d, misses=%d, size=%d, evictions=%d",
			stats.Hits, stats.Misses, stats.Size, stats.Evictions)
	})

	// --- Benchmark 2: Repeated queries (simulate agent session) ---
	t.Run("AgentSession_RepeatedQueries", func(t *testing.T) {
		sessionCache := cache.NewSubgraphCache(cache.SubgraphCacheOptions{})
		engine := knowctx.NewContextEngine(st)
		engine.SetCache(sessionCache)

		// Simulate an agent session: same few queries repeated.
		queries := []string{
			"find MCP handlers",
			"find context engine",
			"find MCP handlers",       // repeat
			"find snapshot management",
			"find context engine",      // repeat
			"find MCP handlers",       // repeat
			"find indexer pipeline",
			"find context engine",      // repeat
			"find MCP handlers",       // repeat
			"find snapshot management", // repeat
		}

		var totalDur time.Duration
		for i, q := range queries {
			start := time.Now()
			_, err := engine.ForTask(ctx, knowctx.TaskOptions{
				TaskDescription: q,
				TokenBudget:     50000,
				Format:          "xml",
			})
			if err != nil {
				t.Fatal(err)
			}
			dur := time.Since(start)
			totalDur += dur
			t.Logf("Query %2d: %v  %q", i+1, dur, q)
		}

		stats := sessionCache.Stats()
		hitRate := float64(stats.Hits) / float64(stats.Hits+stats.Misses) * 100
		t.Logf("\nTotal: %v for %d queries", totalDur, len(queries))
		t.Logf("Cache: %d hits, %d misses (%.1f%% hit rate)", stats.Hits, stats.Misses, hitRate)
		t.Logf("Average per query: %v", totalDur/time.Duration(len(queries)))
	})

	// --- Benchmark 3: Hierarchical diff + invalidation latency ---
	t.Run("DaemonInvalidation", func(t *testing.T) {
		repoHash := types.NewHash([]byte("github.com/blackwell-systems/knowing"))
		nodes, err := st.NodesByName(ctx, "github.com/blackwell-systems/knowing")
		if err != nil {
			t.Fatal(err)
		}

		nodePackage := make(map[types.Hash]string, len(nodes))
		for _, n := range nodes {
			nodePackage[n.NodeHash] = extractPackagePath(n.QualifiedName)
		}

		edgeSeen := make(map[types.Hash]struct{})
		var edgeInputs []snapshot.EdgeInput
		for _, node := range nodes {
			edges, edgeErr := st.EdgesFrom(ctx, node.NodeHash, "")
			if edgeErr != nil {
				continue
			}
			for _, e := range edges {
				if _, ok := edgeSeen[e.EdgeHash]; !ok {
					edgeSeen[e.EdgeHash] = struct{}{}
					edgeInputs = append(edgeInputs, snapshot.EdgeInput{
						EdgeHash:    e.EdgeHash,
						PackagePath: nodePackage[e.SourceHash],
						EdgeType:    e.EdgeType,
					})
				}
			}
		}

		tree1 := snapshot.BuildHierarchicalTree(edgeInputs)

		// Simulate a change: mutate one package.
		mutated := make([]snapshot.EdgeInput, len(edgeInputs))
		copy(mutated, edgeInputs)
		targetPkg := "github.com/blackwell-systems/knowing/internal/mcp"
		changed := 0
		for i, e := range mutated {
			if e.PackagePath == targetPkg {
				mutated[i].EdgeHash = types.NewHash([]byte(fmt.Sprintf("changed-%d", i)))
				changed++
			}
		}
		tree2 := snapshot.BuildHierarchicalTree(mutated)

		// Measure diff + invalidation.
		invalidationCache := cache.NewSubgraphCache(cache.SubgraphCacheOptions{})
		// Pre-populate cache with some entries.
		for i := 0; i < 100; i++ {
			key := types.NewHash([]byte(fmt.Sprintf("cached-result-%d", i)))
			invalidationCache.Put(key, []byte("cached data"))
		}

		start := time.Now()
		diff := snapshot.DiffHierarchicalTrees(tree1, tree2)
		diffDur := time.Since(start)

		start = time.Now()
		allChanged := append(diff.ChangedPackages, diff.AddedPackages...)
		allChanged = append(allChanged, diff.RemovedPackages...)
		invalidationCache.InvalidatePackages(allChanged, tree2)
		invalidateDur := time.Since(start)

		t.Logf("Diff: %v (found %d changed, %d added, %d removed packages)",
			diffDur, len(diff.ChangedPackages), len(diff.AddedPackages), len(diff.RemovedPackages))
		t.Logf("Invalidation: %v", invalidateDur)
		t.Logf("Total daemon overhead per re-index: %v", diffDur+invalidateDur)
		t.Logf("Changed %d edges in %s", changed, targetPkg)

		_ = repoHash
	})

	// --- Benchmark 4: DiffOptions speedup ---
	t.Run("DiffOptions_Filtered", func(t *testing.T) {
		repoHash := types.NewHash([]byte("github.com/blackwell-systems/knowing"))
		nodes, err := st.NodesByName(ctx, "github.com/blackwell-systems/knowing")
		if err != nil {
			t.Fatal(err)
		}

		nodePackage := make(map[types.Hash]string, len(nodes))
		for _, n := range nodes {
			nodePackage[n.NodeHash] = extractPackagePath(n.QualifiedName)
		}

		edgeSeen := make(map[types.Hash]struct{})
		var edgeInputs []snapshot.EdgeInput
		for _, node := range nodes {
			edges, edgeErr := st.EdgesFrom(ctx, node.NodeHash, "")
			if edgeErr != nil {
				continue
			}
			for _, e := range edges {
				if _, ok := edgeSeen[e.EdgeHash]; !ok {
					edgeSeen[e.EdgeHash] = struct{}{}
					edgeInputs = append(edgeInputs, snapshot.EdgeInput{
						EdgeHash:    e.EdgeHash,
						PackagePath: nodePackage[e.SourceHash],
						EdgeType:    e.EdgeType,
					})
				}
			}
		}

		tree1 := snapshot.BuildHierarchicalTree(edgeInputs)

		// Mutate everything.
		mutated := make([]snapshot.EdgeInput, len(edgeInputs))
		for i, e := range edgeInputs {
			mutated[i] = e
			mutated[i].EdgeHash = types.NewHash([]byte(fmt.Sprintf("all-changed-%d", i)))
		}
		tree2 := snapshot.BuildHierarchicalTree(mutated)

		// Unfiltered diff.
		start := time.Now()
		for i := 0; i < 1000; i++ {
			snapshot.DiffHierarchicalTrees(tree1, tree2)
		}
		unfilteredDur := time.Since(start) / 1000

		// Filtered to one package.
		opts := &snapshot.DiffOptions{
			PackageFilter: []string{"github.com/blackwell-systems/knowing/internal/mcp"},
		}
		start = time.Now()
		for i := 0; i < 1000; i++ {
			snapshot.DiffHierarchicalTreesWithOptions(tree1, tree2, opts)
		}
		filteredDur := time.Since(start) / 1000

		// MaxChanges = 3.
		capOpts := &snapshot.DiffOptions{MaxChanges: 3}
		start = time.Now()
		for i := 0; i < 1000; i++ {
			snapshot.DiffHierarchicalTreesWithOptions(tree1, tree2, capOpts)
		}
		cappedDur := time.Since(start) / 1000

		t.Logf("Unfiltered diff: %v", unfilteredDur)
		t.Logf("Filtered (1 package): %v (%.1fx faster)", filteredDur, float64(unfilteredDur)/float64(filteredDur))
		t.Logf("Capped (max 3 changes): %v (%.1fx faster)", cappedDur, float64(unfilteredDur)/float64(cappedDur))

		_ = repoHash
	})

	// --- Write FINDINGS ---
	findingsPath := filepath.Join(repoPath, "bench", "merkle-diff", "FINDINGS-phase2-cache.md")
	findings := fmt.Sprintf(`# Phase 2 Cache Benchmark

End-to-end benchmarks for the subgraph cache, daemon invalidation, and DiffOptions.

## Setup

- Repository: knowing (live codebase)
- Nodes: %d
- Edges: %d

## Results

Run the benchmark to see current numbers:

`+"```bash"+`
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestPhase2CacheBenchmark -timeout 120s
`+"```"+`

The benchmark measures:
1. **context_for_task cold vs warm**: first call (full retrieval) vs second call (cache hit)
2. **Agent session simulation**: 10 queries with repeats, measures cache hit rate
3. **Daemon invalidation latency**: DiffHierarchicalTrees + InvalidatePackages overhead per re-index
4. **DiffOptions speedup**: filtered vs unfiltered vs max-changes-capped diffs
`, snap.NodeCount, snap.EdgeCount)

	if err := os.WriteFile(findingsPath, []byte(findings), 0644); err != nil {
		t.Errorf("writing findings: %v", err)
	}
	t.Logf("Wrote %s", findingsPath)
}
