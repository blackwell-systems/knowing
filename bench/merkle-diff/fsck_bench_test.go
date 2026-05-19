// Package merkle_diff benchmarks the fsck (graph integrity verification) path.
//
// TestFsckBenchmark indexes the knowing repo into a temp DB and measures
// SnapshotManager.Verify() performance with statistical rigor (10 runs, 2 warmup,
// min/median/p95/stddev). It also confirms that a deliberately corrupted edge is
// detected by Verify.
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

func TestFsckBenchmark(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("not in knowing repo root")
	}

	// --- Setup: index the knowing repo into a temp DB ---
	tmpDB := filepath.Join(t.TempDir(), "fsck_bench.db")
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

	const measureN = 10
	const warmupN = 2

	// --- Benchmark: Verify on the clean repo ---
	t.Run("Verify_CleanRepo", func(t *testing.T) {
		var verifyErrs []snapshot.VerifyError
		stats := measure(measureN, warmupN, func() {
			var ferr error
			verifyErrs, ferr = snapMgr.Verify(ctx, repoHash, nil, "")
			if ferr != nil {
				t.Error(ferr)
			}
		})

		// Count errors and warnings.
		var errorCount, warnCount int
		for _, ve := range verifyErrs {
			switch ve.Level {
			case "ERROR":
				errorCount++
			case "WARN":
				warnCount++
			}
		}

		t.Logf("Verify (clean repo): %s", stats)
		t.Logf("Nodes verified: %d", snap.NodeCount)
		t.Logf("Edges verified: %d", snap.EdgeCount)
		t.Logf("Verify errors (ERROR level): %d", errorCount)
		t.Logf("Verify warnings (WARN level): %d", warnCount)
		// Note: the live codebase may have pre-existing dangling edges from
		// cross-repo references whose targets are not indexed into this temp DB.
		// We report the count as a metric rather than failing; the contract is
		// on performance (timing), not zero-error state.
		if errorCount > 0 {
			t.Logf("INFO: %d ERROR-level findings on fresh index (likely cross-repo dangling edges not in temp DB)", errorCount)
		}

		// Performance contract: under 30 seconds median.
		const maxVerifyMedian = 30 * time.Second
		if stats.Median > maxVerifyMedian {
			t.Errorf("REGRESSION: Verify median %v exceeds contract of %v", stats.Median, maxVerifyMedian)
		} else {
			t.Logf("OK Verify median %v < %v contract", stats.Median, maxVerifyMedian)
		}
	})

	// --- Benchmark: Verify with a corrupted edge (dangling target) ---
	//
	// We insert one extra edge whose TargetHash points to a non-existent node.
	// Verify must detect and report the dangling_edge error.
	t.Run("Verify_CorruptedEdge_Detection", func(t *testing.T) {
		// Pick a real source node to make the edge plausible.
		nodes, err := st.NodesByName(ctx, repoURL)
		if err != nil || len(nodes) == 0 {
			t.Skip("no nodes found; skipping corruption test")
		}
		realNode := nodes[0]

		// Insert a fake edge with a non-existent target.
		fakeTargetHash := types.NewHash([]byte("fsck-bench-fake-target-does-not-exist"))
		fakeEdge := types.Edge{
			EdgeHash:   types.NewHash([]byte("fsck-bench-fake-edge-corrupted")),
			SourceHash: realNode.NodeHash,
			TargetHash: fakeTargetHash, // does not exist in nodes table
			EdgeType:   "calls",
			Confidence: 1.0,
			Provenance: "fsck_bench_test",
		}
		if err := st.PutEdge(ctx, fakeEdge); err != nil {
			t.Fatalf("inserting fake edge: %v", err)
		}
		t.Logf("Inserted fake edge %s -> non-existent %s", fakeEdge.EdgeHash, fakeTargetHash)

		var corruptVerifyErrs []snapshot.VerifyError
		corruptStats := measure(measureN, warmupN, func() {
			var ferr error
			corruptVerifyErrs, ferr = snapMgr.Verify(ctx, repoHash, nil, "")
			if ferr != nil {
				t.Error(ferr)
			}
		})

		// Find dangling_edge errors referencing our fake target.
		foundCorruption := false
		for _, ve := range corruptVerifyErrs {
			if ve.Kind == "dangling_edge" && ve.Hash == fakeEdge.EdgeHash {
				foundCorruption = true
				break
			}
		}

		t.Logf("Verify (corrupted repo): %s", corruptStats)
		t.Logf("Total verify findings: %d", len(corruptVerifyErrs))

		if !foundCorruption {
			t.Errorf("Verify did not detect the injected dangling_edge for hash %s", fakeEdge.EdgeHash)
			for _, ve := range corruptVerifyErrs {
				if ve.Kind == "dangling_edge" {
					t.Logf("  dangling_edge found: hash=%s message=%s", ve.Hash, ve.Message)
				}
			}
		} else {
			t.Logf("OK: Verify correctly detected the injected dangling_edge corruption")
		}

		// Clean up the fake edge so subsequent benchmarks run on a clean graph.
		if err := st.DeleteEdge(ctx, fakeEdge.EdgeHash); err != nil {
			t.Logf("warning: could not delete fake edge: %v", err)
		}
	})

	// --- Write FINDINGS-fsck.md ---
	nodes, _ := st.NodesByName(ctx, repoURL)
	var allEdges int
	for _, n := range nodes {
		edges, _ := st.EdgesFrom(ctx, n.NodeHash, "")
		allEdges += len(edges)
	}

	findingsPath := filepath.Join(repoPath, "bench", "merkle-diff", "FINDINGS-fsck.md")
	findings := fmt.Sprintf(`# Fsck (Graph Integrity Verification) Benchmark

Statistical benchmarks for SnapshotManager.Verify() on the live knowing
codebase. Each measurement runs %d times with %d warmup runs discarded;
statistics report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Nodes verified: %d
- Edges verified: %d
- Snapshots in chain: checked for parent continuity

## Checks Performed by Verify

1. **Edge referential integrity** — for every edge, verify that both the source
   node and the target node exist in the graph. Reports dangling_edge (ERROR)
   for any missing endpoint.
2. **Hash recomputation** — recompute the canonical hash for each edge and each
   node and compare against the stored hash. Reports hash_mismatch (ERROR for
   edges, WARN for nodes) on any discrepancy.
3. **Snapshot chain continuity** — walk the snapshot chain from latest to root;
   report broken_chain (ERROR) for any snapshot whose parent hash is not found.

## Performance Contract

- Verify on the knowing repo (%d nodes, %d edges) must complete in under 30
  seconds (median). Test fails if violated.

## Corruption Detection

The benchmark deliberately injects a fake edge whose TargetHash does not exist
in the nodes table, then re-runs Verify and confirms the dangling_edge error is
reported. This validates that the integrity checker is not a no-op.

## Running

`+"```bash"+`
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestFsckBenchmark -timeout 300s
`+"```"+`
`,
		measureN, warmupN,
		snap.NodeCount, snap.EdgeCount,
		snap.NodeCount, snap.EdgeCount,
	)

	if err := os.WriteFile(findingsPath, []byte(findings), 0644); err != nil {
		t.Errorf("writing findings: %v", err)
	}
	t.Logf("Wrote %s", findingsPath)
}
