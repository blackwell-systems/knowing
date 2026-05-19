// End-to-end benchmark for the daemon community detection path:
// load previous assignments from notes -> build graph -> mark changed nodes
// from package list -> DetectIncremental -> save assignments back to notes.
//
// This measures the production path, not just the algorithm in isolation.
// The goal is to prove that SQLite I/O for load/save doesn't dominate.
//
// Run: GOWORK=off go test ./bench/community-detection/ -v -count=1 -run TestE2ECommunityBenchmark -timeout 120s
package community_detection

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/community"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestE2ECommunityBenchmark(t *testing.T) {
	repoRoot := findRepoRoot(t)
	st := indexAndBuildStore(t, repoRoot)

	ctx := context.Background()
	repoURL := "github.com/blackwell-systems/knowing"

	// Load all nodes.
	allNodes, err := st.NodesByName(ctx, repoURL)
	if err != nil {
		t.Fatalf("NodesByName: %v", err)
	}
	t.Logf("Nodes: %d", len(allNodes))

	g := buildGraphFromDB(t, st)
	algo := &community.Louvain{Resolution: 1.0, MaxPasses: 20}

	// --- Phase 1: Full detection + initial save ---
	var fullDetectTime, saveTime, loadTime, incDetectTime, reSaveTime time.Duration

	start := time.Now()
	fullResult := algo.Detect(g)
	fullDetectTime = time.Since(start)

	start = time.Now()
	if err := community.SaveAssignments(ctx, st, fullResult); err != nil {
		t.Fatalf("SaveAssignments: %v", err)
	}
	saveTime = time.Since(start)

	t.Logf("Full detect: %v", fullDetectTime)
	t.Logf("Save %d assignments: %v", len(fullResult), saveTime)

	// --- Phase 2: Incremental path (simulating daemon re-index) ---

	// Load previous.
	start = time.Now()
	previous, err := community.LoadAssignments(ctx, st)
	if err != nil {
		t.Fatalf("LoadAssignments: %v", err)
	}
	loadTime = time.Since(start)
	t.Logf("Load %d assignments: %v", len(previous), loadTime)

	// Mark changed nodes (simulate 1 package change from Merkle diff).
	changedPkgs := []string{"github.com/blackwell-systems/knowing/internal/store"}
	changedNodes := make(map[types.Hash]bool)
	for _, n := range allNodes {
		if _, inPrev := previous[n.NodeHash]; !inPrev {
			changedNodes[n.NodeHash] = true
			continue
		}
		for _, pkg := range changedPkgs {
			if strings.HasPrefix(n.QualifiedName, pkg) {
				changedNodes[n.NodeHash] = true
				break
			}
		}
	}
	t.Logf("Changed nodes: %d of %d (%.1f%%)",
		len(changedNodes), len(g.Nodes), float64(len(changedNodes))/float64(len(g.Nodes))*100)

	// Incremental detect.
	start = time.Now()
	incResult := algo.DetectIncremental(g, previous, changedNodes)
	incDetectTime = time.Since(start)
	t.Logf("Incremental detect: %v", incDetectTime)

	// Save new assignments.
	start = time.Now()
	if err := community.SaveAssignments(ctx, st, incResult); err != nil {
		t.Fatalf("SaveAssignments: %v", err)
	}
	reSaveTime = time.Since(start)
	t.Logf("Re-save %d assignments: %v", len(incResult), reSaveTime)

	// --- Phase 3: Statistical measurement of full e2e cycle ---
	const runs = 10
	const warmup = 2

	statsE2E := measure(runs, warmup, func() {
		prev, _ := community.LoadAssignments(ctx, st)
		changed := make(map[types.Hash]bool)
		for _, n := range allNodes {
			if _, inPrev := prev[n.NodeHash]; !inPrev {
				changed[n.NodeHash] = true
				continue
			}
			for _, pkg := range changedPkgs {
				if strings.HasPrefix(n.QualifiedName, pkg) {
					changed[n.NodeHash] = true
					break
				}
			}
		}
		result := algo.DetectIncremental(g, prev, changed)
		_ = community.SaveAssignments(ctx, st, result)
	})
	t.Logf("E2E incremental cycle (load+mark+detect+save): %s", statsE2E)

	statsFullE2E := measure(runs, warmup, func() {
		result := algo.Detect(g)
		_ = community.SaveAssignments(ctx, st, result)
	})
	t.Logf("E2E full cycle (detect+save):                  %s", statsFullE2E)

	speedup := float64(statsFullE2E.Median) / float64(statsE2E.Median)
	t.Logf("")
	t.Logf("=== E2E Speedup ===")
	t.Logf("Full e2e:        %v", statsFullE2E.Median)
	t.Logf("Incremental e2e: %v", statsE2E.Median)
	t.Logf("Speedup:         %.1fx", speedup)

	// --- Breakdown ---
	totalInc := loadTime + incDetectTime + reSaveTime
	t.Logf("")
	t.Logf("=== Cost Breakdown (single run) ===")
	t.Logf("Load assignments:    %v (%.0f%%)", loadTime, pct(loadTime, totalInc))
	t.Logf("Incremental detect:  %v (%.0f%%)", incDetectTime, pct(incDetectTime, totalInc))
	t.Logf("Save assignments:    %v (%.0f%%)", reSaveTime, pct(reSaveTime, totalInc))
	t.Logf("Total incremental:   %v", totalInc)
	t.Logf("Full detect (no IO): %v", fullDetectTime)

	// Performance contract: e2e incremental should be under 1 second.
	if statsE2E.Median > 1*time.Second {
		t.Errorf("E2E incremental median %v exceeds 1s contract", statsE2E.Median)
	}
}

func pct(part, total time.Duration) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// indexAndBuildStore indexes the knowing repo and returns the store.
// Reuses the same setup as the algorithm benchmark.
func indexAndBuildStore(t *testing.T, repoRoot string) *store.SQLiteStore {
	t.Helper()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "bench.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ctx := context.Background()
	repoURL := "github.com/blackwell-systems/knowing"
	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	if _, err := idx.IndexRepo(ctx, repoURL, repoRoot, "bench"); err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}

	return st
}
