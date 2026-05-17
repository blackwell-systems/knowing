// Package edge_accuracy measures the accuracy of tree-sitter (ast_inferred)
// edge extraction by comparing it against go/ast (ast_resolved) edges as
// ground truth.
//
// The test indexes the knowing repo with both extractors and computes:
// - Confirmation rate: what % of ast_inferred edges have a matching ast_resolved edge
// - False positive rate: what % of ast_inferred edges have no match in ast_resolved
// - Miss rate: what % of ast_resolved edges were missed by tree-sitter
//
// Edge matching: two edges match if they have the same source_hash, target_hash,
// and edge_type (provenance differs because extractors tag differently).
package edge_accuracy

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/goextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	_ "modernc.org/sqlite"
)

// edgeKey is the matching identity for edges across extractors.
// Two edges "match" if they share the same source, target, and type.
type edgeKey struct {
	SourceHash string
	TargetHash string
	EdgeType   string
}

// edgeStats holds per-edge-type accuracy metrics.
type edgeStats struct {
	TotalInferred int // edges from tree-sitter (ast_inferred)
	TotalResolved int // edges from go/ast (ast_resolved)
	Confirmed     int // edges present in both
	InferredOnly  int // edges only in ast_inferred (potential false positives)
	ResolvedOnly  int // edges only in ast_resolved (misses)
}

func TestEdgeAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping edge accuracy benchmark in short mode")
	}

	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	// Phase 1: Index with tree-sitter extractor (ast_inferred edges).
	t.Log("=== Phase 1: Indexing with tree-sitter extractor ===")
	tsDBPath := t.TempDir() + "/treesitter.db"
	tsSt, err := store.NewSQLiteStore(tsDBPath)
	if err != nil {
		t.Fatalf("create tree-sitter store: %v", err)
	}
	defer tsSt.Close()

	tsSnapMgr := snapshot.NewSnapshotManager(tsSt)
	tsIdx := indexer.NewIndexer(tsSt, tsSnapMgr)
	tsIdx.Register(gotsextractor.NewGoTreeSitterExtractor())

	_, err = tsIdx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("tree-sitter index: %v", err)
	}

	// Phase 2: Index with go/ast extractor (ast_resolved edges).
	t.Log("=== Phase 2: Indexing with go/ast extractor ===")
	goDBPath := t.TempDir() + "/goast.db"
	goSt, err := store.NewSQLiteStore(goDBPath)
	if err != nil {
		t.Fatalf("create go/ast store: %v", err)
	}
	defer goSt.Close()

	goSnapMgr := snapshot.NewSnapshotManager(goSt)
	goIdx := indexer.NewIndexer(goSt, goSnapMgr)
	goIdx.Register(goextractor.NewGoExtractor())

	_, err = goIdx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("go/ast index: %v", err)
	}

	// Phase 3: Query all edges from both DBs.
	t.Log("=== Phase 3: Comparing edges ===")
	tsEdges := queryAllEdges(t, tsDBPath)
	goEdges := queryAllEdges(t, goDBPath)

	t.Logf("  Tree-sitter edges (ast_inferred): %d", len(tsEdges))
	t.Logf("  Go/ast edges (ast_resolved): %d", len(goEdges))

	// Phase 4: Compute accuracy metrics.
	// Build sets keyed by (source_hash, target_hash, edge_type).
	tsSet := make(map[edgeKey]bool, len(tsEdges))
	for _, e := range tsEdges {
		tsSet[e] = true
	}

	goSet := make(map[edgeKey]bool, len(goEdges))
	for _, e := range goEdges {
		goSet[e] = true
	}

	// Overall stats.
	overall := computeStats(tsSet, goSet)

	// Per-edge-type stats.
	edgeTypes := []string{"calls", "imports", "implements", "references"}
	byType := make(map[string]*edgeStats)
	for _, et := range edgeTypes {
		tsSubset := filterByType(tsSet, et)
		goSubset := filterByType(goSet, et)
		byType[et] = computeStats(tsSubset, goSubset)
	}

	// Phase 5: Report results.
	t.Log("")
	t.Log("=== Edge Accuracy Results ===")
	t.Log("")
	reportStats(t, "OVERALL", overall)
	t.Log("")
	for _, et := range edgeTypes {
		stats := byType[et]
		if stats.TotalInferred > 0 || stats.TotalResolved > 0 {
			reportStats(t, strings.ToUpper(et), stats)
		}
	}

	// Sanity check: tree-sitter should find at least some edges.
	if overall.TotalInferred == 0 {
		t.Fatalf("tree-sitter found zero edges; indexing may have failed")
	}
	if overall.TotalResolved == 0 {
		t.Fatalf("go/ast found zero edges; indexing may have failed")
	}

	// Log the confirmation rate prominently.
	confirmRate := float64(overall.Confirmed) / float64(overall.TotalInferred) * 100
	t.Logf("\n  CONFIRMATION RATE: %.1f%% of tree-sitter edges confirmed by go/ast", confirmRate)
}

// queryAllEdges opens the DB directly and reads all edges as edgeKeys.
func queryAllEdges(t *testing.T, dbPath string) []edgeKey {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db %s: %v", dbPath, err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT source_hash, target_hash, edge_type FROM edges")
	if err != nil {
		t.Fatalf("query edges from %s: %v", dbPath, err)
	}
	defer rows.Close()

	var edges []edgeKey
	for rows.Next() {
		var sourceHash, targetHash, edgeType string
		if err := rows.Scan(&sourceHash, &targetHash, &edgeType); err != nil {
			t.Fatalf("scan edge: %v", err)
		}
		edges = append(edges, edgeKey{
			SourceHash: sourceHash,
			TargetHash: targetHash,
			EdgeType:   edgeType,
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration: %v", err)
	}
	return edges
}

// computeStats computes accuracy metrics between two edge sets.
func computeStats(tsSet, goSet map[edgeKey]bool) *edgeStats {
	stats := &edgeStats{
		TotalInferred: len(tsSet),
		TotalResolved: len(goSet),
	}

	for k := range tsSet {
		if goSet[k] {
			stats.Confirmed++
		} else {
			stats.InferredOnly++
		}
	}

	for k := range goSet {
		if !tsSet[k] {
			stats.ResolvedOnly++
		}
	}

	return stats
}

// filterByType returns a subset of edges matching the given edge type.
func filterByType(set map[edgeKey]bool, edgeType string) map[edgeKey]bool {
	subset := make(map[edgeKey]bool)
	for k := range set {
		if k.EdgeType == edgeType {
			subset[k] = true
		}
	}
	return subset
}

// reportStats logs accuracy metrics for a given category.
func reportStats(t *testing.T, label string, stats *edgeStats) {
	t.Helper()

	confirmRate := 0.0
	fpRate := 0.0
	missRate := 0.0

	if stats.TotalInferred > 0 {
		confirmRate = float64(stats.Confirmed) / float64(stats.TotalInferred) * 100
		fpRate = float64(stats.InferredOnly) / float64(stats.TotalInferred) * 100
	}
	if stats.TotalResolved > 0 {
		missRate = float64(stats.ResolvedOnly) / float64(stats.TotalResolved) * 100
	}

	t.Logf("  %s:", label)
	t.Logf("    Tree-sitter (ast_inferred): %d edges", stats.TotalInferred)
	t.Logf("    Go/ast (ast_resolved):      %d edges", stats.TotalResolved)
	t.Logf("    Confirmed (both):           %d (%.1f%% of inferred)", stats.Confirmed, confirmRate)
	t.Logf("    Inferred-only (FP):         %d (%.1f%% of inferred)", stats.InferredOnly, fpRate)
	t.Logf("    Resolved-only (missed):     %d (%.1f%% of resolved)", stats.ResolvedOnly, missRate)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatal("cannot find repo root")
		}
		dir = parent
	}
}

func formatPercent(numerator, denominator int) string {
	if denominator == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", float64(numerator)/float64(denominator)*100)
}
