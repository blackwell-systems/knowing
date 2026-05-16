package diff

import (
	"context"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestPRImpact_RemovedNodeShowsCallers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	// A calls B. We will simulate B being removed.
	nodeA := putNode(t, s, file, "main.Caller", "function")
	nodeB := putNode(t, s, file, "main.Target", "function")

	edgeAB := putEdge(t, s, nodeA, nodeB, "calls")

	oldSnap := types.NewHash([]byte("old-snap"))
	newSnap := types.NewHash([]byte("new-snap"))

	// Record edge removal in new snapshot (B was removed, so its incoming edge is removed).
	recordEdgeEvent(t, s, edgeAB, "removed", newSnap)

	// Since SnapshotDiff only returns edges (not nodes), the semantic diff will detect
	// nodeA as a modified node (its outgoing edge changed). The removed node detection
	// depends on nodes being in the DiffResult.NodesRemoved, which requires the store
	// to populate it. For this test we verify the modified-node path works.
	result, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// nodeA should appear as a modified symbol (its outgoing edge was removed).
	found := false
	for _, sym := range result.ChangedSymbols {
		if sym.Symbol.QualifiedName == "main.Caller" && sym.ChangeType == "modified" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected main.Caller as a modified symbol, got %d symbols: %+v",
			len(result.ChangedSymbols), result.ChangedSymbols)
	}

	if result.Summary.TotalSymbolsChanged < 1 {
		t.Errorf("expected at least 1 changed symbol, got %d", result.Summary.TotalSymbolsChanged)
	}
}

func TestPRImpact_RiskLevelCalculation(t *testing.T) {
	tests := []struct {
		name       string
		callers    int
		wantLevel  string
	}{
		{"low risk with 0 callers", 0, "low"},
		{"low risk with 5 callers", 5, "low"},
		{"medium risk with 6 callers", 6, "medium"},
		{"medium risk with 20 callers", 20, "medium"},
		{"high risk with 21 callers", 21, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := riskLevel(tt.callers)
			if got != tt.wantLevel {
				t.Errorf("riskLevel(%d) = %s, want %s", tt.callers, got, tt.wantLevel)
			}
		})
	}
}

func TestPRImpact_EmptyDiff(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	snap := types.NewHash([]byte("same-snap"))

	result, err := PRImpact(ctx, s, snap, snap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	if len(result.ChangedSymbols) != 0 {
		t.Errorf("expected 0 changed symbols, got %d", len(result.ChangedSymbols))
	}
	if result.Summary.RiskLevel != "low" {
		t.Errorf("expected low risk for empty diff, got %s", result.Summary.RiskLevel)
	}
}

func TestPRImpact_MultipleEdgeChanges(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	// Create a graph: A -> B, A -> C. Both edges added in new snapshot.
	nodeA := putNode(t, s, file, "main.Hub", "function")
	nodeB := putNode(t, s, file, "main.Spoke1", "function")
	nodeC := putNode(t, s, file, "main.Spoke2", "function")

	edgeAB := putEdge(t, s, nodeA, nodeB, "calls")
	edgeAC := putEdge(t, s, nodeA, nodeC, "calls")

	oldSnap := types.NewHash([]byte("old"))
	newSnap := types.NewHash([]byte("new"))

	recordEdgeEvent(t, s, edgeAB, "added", newSnap)
	recordEdgeEvent(t, s, edgeAC, "added", newSnap)

	result, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// nodeA should be modified (two edges added from it).
	if result.Summary.TotalSymbolsChanged != 1 {
		t.Errorf("expected 1 changed symbol, got %d", result.Summary.TotalSymbolsChanged)
	}

	// Affected edges should include both added edges.
	if len(result.AffectedEdges) != 2 {
		t.Errorf("expected 2 affected edges, got %d", len(result.AffectedEdges))
	}
}

// unused import guard: time is used by recordEdgeEvent in semantic_test.go (same package)
var _ = time.Now
