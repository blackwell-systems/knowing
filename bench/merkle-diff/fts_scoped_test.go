// Benchmarks scoped FTS rebuild (Phase 3 F3/P4): full rebuild vs package-scoped.
//
// Run: GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestScopedFTSBenchmark -timeout 120s
package merkle_diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

func TestScopedFTSBenchmark(t *testing.T) {
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

	// Full FTS rebuild.
	statsFull := measure(5, 1, func() {
		st.RebuildFTS(ctx)
	})
	t.Logf("Full FTS rebuild:    %s", statsFull)

	// Scoped: 1 package.
	statsOne := measure(10, 2, func() {
		st.RebuildFTSForPackages(ctx, []string{
			"github.com/blackwell-systems/knowing/internal/store",
		})
	})
	t.Logf("Scoped FTS (1 pkg):  %s", statsOne)

	// Scoped: 3 packages.
	statsThree := measure(10, 2, func() {
		st.RebuildFTSForPackages(ctx, []string{
			"github.com/blackwell-systems/knowing/internal/store",
			"github.com/blackwell-systems/knowing/internal/mcp",
			"github.com/blackwell-systems/knowing/internal/context",
		})
	})
	t.Logf("Scoped FTS (3 pkgs): %s", statsThree)

	speedupOne := float64(statsFull.Median) / float64(statsOne.Median)
	speedupThree := float64(statsFull.Median) / float64(statsThree.Median)

	t.Logf("")
	t.Logf("=== Scoped FTS Speedup ===")
	t.Logf("Full rebuild:  %v", statsFull.Median)
	t.Logf("1 package:     %v (%.1fx)", statsOne.Median, speedupOne)
	t.Logf("3 packages:    %v (%.1fx)", statsThree.Median, speedupThree)

	// Performance contract: scoped rebuild should be faster than full.
	if statsOne.Median > statsFull.Median {
		t.Errorf("Scoped FTS (1 pkg) %v slower than full %v", statsOne.Median, statsFull.Median)
	}
	// Scoped should complete under 75ms (CI runners have ~50% timing variance).
	if statsOne.Median > 75*time.Millisecond {
		t.Errorf("Scoped FTS (1 pkg) %v exceeds 75ms contract", statsOne.Median)
	}
}
