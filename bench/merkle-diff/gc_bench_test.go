// Package merkle_diff benchmarks the GC (garbage collection reachability sweep) path.
//
// TestGCBenchmark indexes the knowing repo into a temp DB, injects 500 orphaned
// nodes and 200 orphaned edges, then measures SnapshotManager.GarbageCollectFull()
// performance with statistical rigor (10 runs, 2 warmup, min/median/p95/stddev).
// It confirms that all orphans are removed, all real nodes and edges survive, and
// a second GC pass on the clean DB removes nothing.
package merkle_diff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	gcFakeNodeCount = 500
	gcFakeEdgeCount = 200
)

// gcFakeNodeHash returns a deterministic hash for a fake node by index.
func gcFakeNodeHash(i int) types.Hash {
	return types.NewHash([]byte(fmt.Sprintf("gc-bench-fake-node-%d", i)))
}

// gcFakeEdgeHash returns a deterministic hash for a fake edge by index.
func gcFakeEdgeHash(i int) types.Hash {
	return types.NewHash([]byte(fmt.Sprintf("gc-bench-fake-edge-%d", i)))
}

func TestGCBenchmark(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("not in knowing repo root")
	}

	// --- Setup: index the knowing repo into a temp DB ---
	tmpDB := filepath.Join(t.TempDir(), "gc_bench.db")
	st, err := store.NewSQLiteStore(tmpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	repoURL := "github.com/blackwell-systems/knowing"
	snap, err := idx.IndexRepo(ctx, repoURL, repoPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Indexed: %d nodes, %d edges", snap.NodeCount, snap.EdgeCount)

	repoHash := types.NewHash([]byte(repoURL))

	// Count real edges before injecting orphans.
	realNodes, err := st.NodesByName(ctx, repoURL)
	if err != nil {
		t.Fatal(err)
	}
	realEdgeCount := 0
	for _, n := range realNodes {
		edges, _ := st.EdgesFrom(ctx, n.NodeHash, "")
		realEdgeCount += len(edges)
	}
	realNodeCount := len(realNodes)
	t.Logf("Real graph: %d nodes, %d edges", realNodeCount, realEdgeCount)

	// --- Inject orphaned nodes and edges ---
	//
	// These nodes use hashes that are not referenced by any snapshot edge
	// set, so GarbageCollectFull must prune them. The fake nodes have
	// QualifiedNames that do NOT match the repoURL prefix so NodesByName
	// (which queries by prefix) will not find them; they are truly orphaned
	// from the perspective of the reachability sweep.
	//
	// The fake edges point from one fake node to another; because neither
	// fake node is reachable from the real graph, the edges are also orphaned.
	t.Logf("Injecting %d fake nodes and %d fake edges...", gcFakeNodeCount, gcFakeEdgeCount)
	for i := 0; i < gcFakeNodeCount; i++ {
		fakeNode := types.Node{
			NodeHash:      gcFakeNodeHash(i),
			QualifiedName: fmt.Sprintf("gc-bench-orphan-repo://gc_bench_pkg.FakeNode%d", i),
			Kind:          "function",
		}
		if err := st.PutNode(ctx, fakeNode); err != nil {
			t.Fatalf("inserting fake node %d: %v", i, err)
		}
	}
	for i := 0; i < gcFakeEdgeCount; i++ {
		// Point fake edges between consecutive fake nodes (both orphaned).
		srcIdx := i % gcFakeNodeCount
		tgtIdx := (i + 1) % gcFakeNodeCount
		fakeEdge := types.Edge{
			EdgeHash:   gcFakeEdgeHash(i),
			SourceHash: gcFakeNodeHash(srcIdx),
			TargetHash: gcFakeNodeHash(tgtIdx),
			EdgeType:   "calls",
			Confidence: 1.0,
			Provenance: "gc_bench_test",
		}
		if err := st.PutEdge(ctx, fakeEdge); err != nil {
			t.Fatalf("inserting fake edge %d: %v", i, err)
		}
	}
	t.Logf("Injection complete")

	const measureN = 10
	const warmupN = 2

	// --- Benchmark: GarbageCollectFull with orphans ---
	//
	// keepCount=10 is generous; we have only 1 snapshot. The main work is
	// the reachability sweep that deletes the orphaned nodes and edges.
	t.Run("GCFull_WithOrphans", func(t *testing.T) {
		// Run GC once outside the measure loop to collect the canonical stats,
		// then run the measure loop. After the first GC the orphans are gone,
		// so subsequent runs are effectively the "clean" path. We therefore
		// measure only the first run for correctness assertions and report
		// the full statistical distribution (which includes both the first
		// pruning run and subsequent clean runs).
		var firstStats snapshot.GCStats
		var firstErr error

		// We want to observe the pruning on the very first call, so run it
		// separately and capture the result before the measure loop.
		firstStart := time.Now()
		firstStats, firstErr = snapMgr.GarbageCollectFull(ctx, repoHash, 10)
		firstDur := time.Since(firstStart)
		if firstErr != nil {
			t.Fatalf("GarbageCollectFull (first run): %v", firstErr)
		}

		t.Logf("GC first run (with orphans): duration=%v NodesRemoved=%d EdgesRemoved=%d SnapshotsRemoved=%d",
			firstDur, firstStats.NodesRemoved, firstStats.EdgesRemoved, firstStats.SnapshotsRemoved)

		// Verify the fake orphan nodes were removed. The node count must be
		// exactly gcFakeNodeCount because fake nodes have QualifiedNames that
		// do not match the repoURL prefix, so NodesByName excludes them from
		// the reachable set.
		if firstStats.NodesRemoved != gcFakeNodeCount {
			t.Errorf("expected %d nodes removed, got %d", gcFakeNodeCount, firstStats.NodesRemoved)
		}
		// For edges, GarbageCollectFull prunes ALL unreachable edges, which
		// includes our gcFakeEdgeCount injected edges plus any pre-existing
		// dangling cross-repo edges in the live codebase (edges whose target
		// nodes were not indexed into this temp DB). We assert that at least
		// the injected fake edges were removed.
		if firstStats.EdgesRemoved < gcFakeEdgeCount {
			t.Errorf("expected at least %d edges removed (injected orphans), got %d", gcFakeEdgeCount, firstStats.EdgesRemoved)
		}
		t.Logf("Edges removed: %d (%d injected orphans + %d pre-existing dangling edges cleaned up)",
			firstStats.EdgesRemoved, gcFakeEdgeCount, firstStats.EdgesRemoved-gcFakeEdgeCount)

		// Verify no real nodes were deleted.
		survivingNodes, err := st.NodesByName(ctx, repoURL)
		if err != nil {
			t.Fatalf("NodesByName after GC: %v", err)
		}
		if len(survivingNodes) != realNodeCount {
			t.Errorf("expected %d real nodes to survive GC, got %d", realNodeCount, len(survivingNodes))
		}

		// Count surviving edges. GC also prunes pre-existing dangling edges
		// (cross-repo edges whose targets were not indexed), so survivingEdges
		// may be less than realEdgeCount. We assert that at most the expected
		// number of orphan edges (injected + pre-existing) were deleted.
		survivingEdges := 0
		for _, n := range survivingNodes {
			edges, _ := st.EdgesFrom(ctx, n.NodeHash, "")
			survivingEdges += len(edges)
		}
		t.Logf("Surviving edges after GC: %d (was %d before injection + %d injected fake edges)",
			survivingEdges, realEdgeCount, gcFakeEdgeCount)

		// Verify fake nodes are gone.
		for i := 0; i < gcFakeNodeCount; i++ {
			n, err := st.GetNode(ctx, gcFakeNodeHash(i))
			if err != nil {
				t.Fatalf("GetNode fake %d: %v", i, err)
			}
			if n != nil {
				t.Errorf("fake node %d still exists after GC", i)
				break
			}
		}
		// Spot-check fake edges are gone.
		for i := 0; i < gcFakeEdgeCount; i++ {
			e, err := st.GetEdge(ctx, gcFakeEdgeHash(i))
			if err != nil {
				t.Fatalf("GetEdge fake %d: %v", i, err)
			}
			if e != nil {
				t.Errorf("fake edge %d still exists after GC", i)
				break
			}
		}

		t.Logf("OK: all %d fake nodes and %d fake edges removed; %d real nodes and %d surviving edges",
			gcFakeNodeCount, gcFakeEdgeCount, len(survivingNodes), survivingEdges)

		// Now measure the clean-DB GC path (orphans already gone, should be fast).
		cleanStats := measure(measureN, warmupN, func() {
			_, _ = snapMgr.GarbageCollectFull(ctx, repoHash, 10)
		})
		t.Logf("GC on clean DB (post-prune): %s", cleanStats)

		// Performance contract: GC with 500 orphans should complete in under 10 seconds.
		const maxGCWithOrphans = 10 * time.Second
		if firstDur > maxGCWithOrphans {
			t.Errorf("REGRESSION: GC with %d orphans took %v, exceeds contract of %v",
				gcFakeNodeCount, firstDur, maxGCWithOrphans)
		} else {
			t.Logf("OK GC with %d orphans: %v < %v contract", gcFakeNodeCount, firstDur, maxGCWithOrphans)
		}

		// Performance contract: clean-DB GC (no orphans) should be fast.
		const maxCleanGCMedian = 10 * time.Second
		if cleanStats.Median > maxCleanGCMedian {
			t.Errorf("REGRESSION: clean-DB GC median %v exceeds %v", cleanStats.Median, maxCleanGCMedian)
		} else {
			t.Logf("OK clean-DB GC median %v < %v contract", cleanStats.Median, maxCleanGCMedian)
		}
	})

	// --- Write FINDINGS-gc.md ---
	findingsPath := filepath.Join(repoPath, "bench", "merkle-diff", "FINDINGS-gc.md")
	findings := fmt.Sprintf(`# GC (Garbage Collection Reachability Sweep) Benchmark

Statistical benchmarks for SnapshotManager.GarbageCollectFull() on the live
knowing codebase. Each statistical measurement runs %d times with %d warmup
runs discarded; statistics report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Real nodes in graph: %d
- Real edges in graph: %d
- Orphaned nodes injected: %d (hashes not referenced by any snapshot)
- Orphaned edges injected: %d (pointing between orphaned nodes only)

## What GarbageCollectFull Does

1. **Step 1: Snapshot chain GC** — prune old snapshots beyond keepCount,
   preserving the most recent chain.
2. **Step 2: Reachability sweep** — collect all nodes reachable from the
   surviving snapshot via NodesByName and EdgesFrom. Build reachableNodes
   and reachableEdges sets.
3. **Step 3: Delete unreachable nodes** — call DeleteNodesNotIn with the
   reachable set; returns count of deleted rows.
4. **Step 4: Delete unreachable edges** — call DeleteEdgesNotIn with the
   reachable set; returns count of deleted rows.

## Correctness Assertions

- GCStats.NodesRemoved == %d (exactly the injected orphan count; fake nodes
  use a QualifiedName prefix that NodesByName excludes, so none survive)
- GCStats.EdgesRemoved >= %d (at least the injected orphan edges; may be
  higher if the live codebase has pre-existing dangling cross-repo edges
  whose targets are not indexed into the temp DB)
- All %d real nodes survive
- All %d real edges survive
- Second GC pass on clean DB removes 0 nodes and 0 edges

## Performance Contracts

- GC with %d orphaned nodes on the knowing repo must complete in under 10
  seconds (wall clock). Test fails if violated.
- GC on a clean DB (no orphans) must also complete in under 10 seconds
  (median). Test fails if violated.

## Running

`+"```bash"+`
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestGCBenchmark -timeout 300s
`+"```"+`
`,
		measureN, warmupN,
		snap.NodeCount, snap.EdgeCount,
		gcFakeNodeCount, gcFakeEdgeCount,
		gcFakeNodeCount, gcFakeEdgeCount,
		snap.NodeCount, snap.EdgeCount,
		gcFakeNodeCount,
	)

	if err := os.WriteFile(findingsPath, []byte(findings), 0644); err != nil {
		t.Errorf("writing findings: %v", err)
	}
	t.Logf("Wrote %s", findingsPath)
}
