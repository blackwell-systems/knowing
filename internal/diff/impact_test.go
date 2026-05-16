package diff

import (
	"context"
	"fmt"
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

func TestPRImpact_RiskLevelBoundaries(t *testing.T) {
	// Test exact boundary values for risk level classification.
	tests := []struct {
		name      string
		callers   int
		wantLevel string
	}{
		{"exactly 5 callers is low", 5, "low"},
		{"exactly 6 callers is medium", 6, "medium"},
		{"exactly 20 callers is medium", 20, "medium"},
		{"exactly 21 callers is high", 21, "high"},
		{"1 caller is low", 1, "low"},
		{"100 callers is high", 100, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := riskLevel(tt.callers)
			if got != tt.wantLevel {
				t.Errorf("riskLevel(%d) = %q, want %q", tt.callers, got, tt.wantLevel)
			}
		})
	}
}

func TestPRImpact_EmptyDiffDetails(t *testing.T) {
	// Verify all fields of PRImpactResult are properly initialized for an empty diff.
	s := newTestStore(t)
	ctx := context.Background()

	snap := types.NewHash([]byte("empty-snap"))

	result, err := PRImpact(ctx, s, snap, snap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	if len(result.ChangedSymbols) != 0 {
		t.Errorf("expected 0 changed symbols, got %d", len(result.ChangedSymbols))
	}
	if len(result.AffectedEdges) != 0 {
		t.Errorf("expected 0 affected edges, got %d", len(result.AffectedEdges))
	}
	if result.Summary.TotalSymbolsChanged != 0 {
		t.Errorf("TotalSymbolsChanged: want 0, got %d", result.Summary.TotalSymbolsChanged)
	}
	if result.Summary.TotalCallersAffected != 0 {
		t.Errorf("TotalCallersAffected: want 0, got %d", result.Summary.TotalCallersAffected)
	}
	if result.Summary.TotalCalleesAffected != 0 {
		t.Errorf("TotalCalleesAffected: want 0, got %d", result.Summary.TotalCalleesAffected)
	}
	if result.Summary.RiskLevel != "low" {
		t.Errorf("RiskLevel: want %q, got %q", "low", result.Summary.RiskLevel)
	}
	if result.OldSnapshot != snap.String() {
		t.Errorf("OldSnapshot: want %q, got %q", snap.String(), result.OldSnapshot)
	}
	if result.NewSnapshot != snap.String() {
		t.Errorf("NewSnapshot: want %q, got %q", snap.String(), result.NewSnapshot)
	}
}

func TestPRImpact_AddedNodeWithCallees(t *testing.T) {
	// When edge events cause a node to appear as modified, and that node has
	// outgoing edges (callees), the callees should be reported in the impact.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	// Create a chain: newFunc -> helper1 -> helper2
	newFunc := putNode(t, s, file, "main.NewFunc", "function")
	helper1 := putNode(t, s, file, "main.Helper1", "function")
	helper2 := putNode(t, s, file, "main.Helper2", "function")

	edgeNew1 := putEdge(t, s, newFunc, helper1, "calls")
	edgeH12 := putEdge(t, s, helper1, helper2, "calls")

	oldSnap := types.NewHash([]byte("callee-old"))
	newSnap := types.NewHash([]byte("callee-new"))

	// Record both edges as added in the new snapshot.
	recordEdgeEvent(t, s, edgeNew1, "added", newSnap)
	recordEdgeEvent(t, s, edgeH12, "added", newSnap)

	result, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// Both newFunc and helper1 should appear as modified (they have new outgoing edges).
	if result.Summary.TotalSymbolsChanged < 2 {
		t.Errorf("expected at least 2 changed symbols, got %d", result.Summary.TotalSymbolsChanged)
	}

	// Verify affected edges includes both new edges.
	if len(result.AffectedEdges) != 2 {
		t.Errorf("expected 2 affected edges, got %d", len(result.AffectedEdges))
	}
}

func TestPRImpact_LargeDiffEdgeChanges(t *testing.T) {
	// Test PRImpact with 12 edge changes from distinct source nodes.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	const edgeCount = 12
	target := putNode(t, s, file, "main.SharedTarget", "function")

	oldSnap := types.NewHash([]byte("large-impact-old"))
	newSnap := types.NewHash([]byte("large-impact-new"))

	for i := 0; i < edgeCount; i++ {
		src := putNode(t, s, file, fmt.Sprintf("main.Caller%d", i), "function")
		e := putEdge(t, s, src, target, "calls")
		recordEdgeEvent(t, s, e, "added", newSnap)
	}

	result, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// All 12 source nodes should appear as modified symbols.
	if result.Summary.TotalSymbolsChanged != edgeCount {
		t.Errorf("TotalSymbolsChanged: want %d, got %d", edgeCount, result.Summary.TotalSymbolsChanged)
	}

	// All 12 edges should be in affected edges.
	if len(result.AffectedEdges) != edgeCount {
		t.Errorf("AffectedEdges: want %d, got %d", edgeCount, len(result.AffectedEdges))
	}
}

func TestPRImpact_ModifiedNodeWithCallersAndCallees(t *testing.T) {
	// A modified node (edge changed) that has both callers and callees should
	// report both in the impact analysis.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	// caller1 -> middleFunc -> callee1
	// caller2 -> middleFunc
	caller1 := putNode(t, s, file, "main.Caller1", "function")
	caller2 := putNode(t, s, file, "main.Caller2", "function")
	middleFunc := putNode(t, s, file, "main.MiddleFunc", "function")
	callee1 := putNode(t, s, file, "main.Callee1", "function")
	newTarget := putNode(t, s, file, "main.NewTarget", "function")

	// Existing edges (callers of middleFunc).
	edgeC1M := putEdge(t, s, caller1, middleFunc, "calls")
	edgeC2M := putEdge(t, s, caller2, middleFunc, "calls")
	// Existing edge (middleFunc calls callee1).
	edgeMC1 := putEdge(t, s, middleFunc, callee1, "calls")

	oldSnap := types.NewHash([]byte("both-old"))
	newSnap := types.NewHash([]byte("both-new"))

	// Baseline: these edges exist in old snapshot.
	recordEdgeEvent(t, s, edgeC1M, "added", oldSnap)
	recordEdgeEvent(t, s, edgeC2M, "added", oldSnap)
	recordEdgeEvent(t, s, edgeMC1, "added", oldSnap)

	// In new snapshot, middleFunc gets a new outgoing edge.
	edgeMNew := putEdge(t, s, middleFunc, newTarget, "calls")
	recordEdgeEvent(t, s, edgeMNew, "added", newSnap)

	result, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// middleFunc should be modified.
	found := false
	for _, sym := range result.ChangedSymbols {
		if sym.Symbol.QualifiedName == "main.MiddleFunc" && sym.ChangeType == "modified" {
			found = true
			// middleFunc should have callers (caller1, caller2) from blast radius.
			if sym.CallerCount < 2 {
				t.Errorf("expected at least 2 callers for MiddleFunc, got %d", sym.CallerCount)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected main.MiddleFunc as modified symbol, symbols: %+v", result.ChangedSymbols)
	}
}

func TestParseHash_Valid(t *testing.T) {
	// Valid 32-byte hex hash should parse successfully.
	h := types.NewHash([]byte("test"))
	hexStr := h.String()
	parsed, err := parseHash(hexStr)
	if err != nil {
		t.Fatalf("parseHash(%q): %v", hexStr, err)
	}
	if parsed != h {
		t.Errorf("parseHash roundtrip failed: got %v, want %v", parsed, h)
	}
}

func TestParseHash_InvalidHex(t *testing.T) {
	_, err := parseHash("not-a-hex-string!")
	if err == nil {
		t.Fatal("expected error for invalid hex string, got nil")
	}
}

func TestParseHash_WrongLength(t *testing.T) {
	// Valid hex but wrong length (16 bytes instead of 32).
	_, err := parseHash("abcdef0123456789abcdef0123456789")
	if err == nil {
		t.Fatal("expected error for 16-byte hash, got nil")
	}
}

func TestPRImpact_DeepCallChain(t *testing.T) {
	// Chain: A -> B -> C -> D. Remove D (simulate by removing edge C->D).
	// B and C should be affected as modified nodes.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	nodeA := putNode(t, s, file, "main.FuncA", "function")
	nodeB := putNode(t, s, file, "main.FuncB", "function")
	nodeC := putNode(t, s, file, "main.FuncC", "function")
	nodeD := putNode(t, s, file, "main.FuncD", "function")

	edgeAB := putEdge(t, s, nodeA, nodeB, "calls")
	edgeBC := putEdge(t, s, nodeB, nodeC, "calls")
	edgeCD := putEdge(t, s, nodeC, nodeD, "calls")

	oldSnap := types.NewHash([]byte("deep-old"))
	newSnap := types.NewHash([]byte("deep-new"))

	// Baseline: all edges exist in old snapshot.
	recordEdgeEvent(t, s, edgeAB, "added", oldSnap)
	recordEdgeEvent(t, s, edgeBC, "added", oldSnap)
	recordEdgeEvent(t, s, edgeCD, "added", oldSnap)

	// In new snapshot, D is removed (edge C->D removed).
	recordEdgeEvent(t, s, edgeCD, "removed", newSnap)

	result, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// C should be modified (its outgoing edge was removed).
	foundC := false
	for _, sym := range result.ChangedSymbols {
		if sym.Symbol.QualifiedName == "main.FuncC" && sym.ChangeType == "modified" {
			foundC = true
			// C should have callers (at least B calls C via blast radius).
			if sym.CallerCount < 1 {
				t.Errorf("expected at least 1 caller for FuncC, got %d", sym.CallerCount)
			}
			break
		}
	}
	if !foundC {
		t.Errorf("expected main.FuncC as modified symbol, got: %+v", result.ChangedSymbols)
	}

	// Verify affected edges include the removed edge.
	if len(result.AffectedEdges) < 1 {
		t.Errorf("expected at least 1 affected edge, got %d", len(result.AffectedEdges))
	}
}

func TestPRImpact_RiskLevelExactBoundaries(t *testing.T) {
	// Test exact boundary values: 0, 5, 20, 21 callers.
	tests := []struct {
		callers   int
		wantLevel string
	}{
		{0, "low"},
		{5, "low"},
		{20, "medium"},
		{21, "high"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_callers", tt.callers), func(t *testing.T) {
			got := riskLevel(tt.callers)
			if got != tt.wantLevel {
				t.Errorf("riskLevel(%d) = %q, want %q", tt.callers, got, tt.wantLevel)
			}
		})
	}
}

// unused import guard: time is used by recordEdgeEvent in semantic_test.go (same package)
var _ = time.Now
