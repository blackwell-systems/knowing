package context

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeSymbol(name string, score float64) RankedSymbol {
	return RankedSymbol{
		Node: types.Node{
			NodeHash:      types.NewHash([]byte(name)),
			QualifiedName: "github.com/example/project/internal/handlers." + name,
			Kind:          "function",
			Signature:     "func " + name + "(ctx context.Context, req *http.Request) (*Response, error)",
		},
		Score:      score,
		Provenance: "rwr",
	}
}

func makeEdge(src, tgt, edgeType string) ContextEdge {
	return ContextEdge{Source: "pkg://" + src, Target: "pkg://" + tgt, EdgeType: edgeType}
}

func TestDiffPacks_Identical(t *testing.T) {
	syms := []RankedSymbol{makeSymbol("A", 0.9), makeSymbol("B", 0.8)}
	edges := []ContextEdge{makeEdge("A", "B", "calls")}

	prior := &ContextBlock{Symbols: syms, Edges: edges, PackRoot: types.NewHash([]byte("root1"))}
	current := &ContextBlock{Symbols: syms, Edges: edges, PackRoot: types.NewHash([]byte("root1"))}

	delta := DiffPacks(prior, current, "gcf")

	if len(delta.Removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(delta.Removed))
	}
	if len(delta.Added) != 0 {
		t.Errorf("expected 0 added, got %d", len(delta.Added))
	}
	if len(delta.Unchanged) != 2 {
		t.Errorf("expected 2 unchanged, got %d", len(delta.Unchanged))
	}
	if len(delta.RemovedEdges) != 0 {
		t.Errorf("expected 0 removed edges, got %d", len(delta.RemovedEdges))
	}
	if len(delta.AddedEdges) != 0 {
		t.Errorf("expected 0 added edges, got %d", len(delta.AddedEdges))
	}
}

func TestDiffPacks_CompletelyDifferent(t *testing.T) {
	prior := &ContextBlock{
		Symbols: []RankedSymbol{makeSymbol("A", 0.9), makeSymbol("B", 0.8)},
		Edges:   []ContextEdge{makeEdge("A", "B", "calls")},
	}
	current := &ContextBlock{
		Symbols: []RankedSymbol{makeSymbol("C", 0.7), makeSymbol("D", 0.6)},
		Edges:   []ContextEdge{makeEdge("C", "D", "imports")},
	}

	delta := DiffPacks(prior, current, "gcf")

	if len(delta.Removed) != 2 {
		t.Errorf("expected 2 removed, got %d", len(delta.Removed))
	}
	if len(delta.Added) != 2 {
		t.Errorf("expected 2 added, got %d", len(delta.Added))
	}
	if len(delta.Unchanged) != 0 {
		t.Errorf("expected 0 unchanged, got %d", len(delta.Unchanged))
	}
	if len(delta.RemovedEdges) != 1 {
		t.Errorf("expected 1 removed edge, got %d", len(delta.RemovedEdges))
	}
	if len(delta.AddedEdges) != 1 {
		t.Errorf("expected 1 added edge, got %d", len(delta.AddedEdges))
	}

	// Complete replacement: delta should not be worth it.
	if delta.IsWorthIt() {
		t.Error("complete replacement should not be worth delta encoding")
	}
}

func TestDiffPacks_PartialOverlap(t *testing.T) {
	shared := makeSymbol("Shared", 0.9)
	prior := &ContextBlock{
		Symbols: []RankedSymbol{shared, makeSymbol("Old", 0.8)},
		Edges:   []ContextEdge{makeEdge("Shared", "Old", "calls")},
	}
	current := &ContextBlock{
		Symbols: []RankedSymbol{shared, makeSymbol("New", 0.7)},
		Edges:   []ContextEdge{makeEdge("Shared", "New", "calls")},
	}

	delta := DiffPacks(prior, current, "gcf")

	if len(delta.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(delta.Removed))
	}
	if len(delta.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(delta.Added))
	}
	if len(delta.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged, got %d", len(delta.Unchanged))
	}
	if !strings.Contains(delta.Removed[0].Node.QualifiedName, "Old") {
		t.Errorf("expected Old removed, got %s", delta.Removed[0].Node.QualifiedName)
	}
	if !strings.Contains(delta.Added[0].Node.QualifiedName, "New") {
		t.Errorf("expected New added, got %s", delta.Added[0].Node.QualifiedName)
	}
}

func TestDiffPacks_HighOverlap_IsWorthIt(t *testing.T) {
	// 9 shared symbols, 1 removed, 1 added = 90% overlap.
	// Delta should be much smaller than full retransmission.
	var priorSyms, currentSyms []RankedSymbol
	for i := 0; i < 9; i++ {
		s := makeSymbol("Shared"+string(rune('A'+i)), 0.9-float64(i)*0.01)
		priorSyms = append(priorSyms, s)
		currentSyms = append(currentSyms, s)
	}
	priorSyms = append(priorSyms, makeSymbol("OldOnly", 0.1))
	currentSyms = append(currentSyms, makeSymbol("NewOnly", 0.1))

	prior := &ContextBlock{Symbols: priorSyms}
	current := &ContextBlock{Symbols: currentSyms}

	delta := DiffPacks(prior, current, "gcf")

	if !delta.IsWorthIt() {
		t.Errorf("90%% overlap should be worth delta encoding (delta=%d, full=%d, savings=%.1f%%)",
			delta.DeltaTokens, delta.FullTokens, delta.SavingsPercent())
	}
	if delta.SavingsPercent() < 50 {
		t.Errorf("expected >50%% savings with 90%% overlap, got %.1f%%", delta.SavingsPercent())
	}
	if delta.SymbolOverlapPercent() < 85 {
		t.Errorf("expected ~90%% symbol overlap, got %.1f%%", delta.SymbolOverlapPercent())
	}
}

func TestDiffPacks_EmptyPrior(t *testing.T) {
	prior := &ContextBlock{}
	current := &ContextBlock{
		Symbols: []RankedSymbol{makeSymbol("A", 0.9)},
	}

	delta := DiffPacks(prior, current, "gcf")

	if len(delta.Removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(delta.Removed))
	}
	if len(delta.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(delta.Added))
	}
}

func TestDiffPacks_EmptyCurrent(t *testing.T) {
	prior := &ContextBlock{
		Symbols: []RankedSymbol{makeSymbol("A", 0.9)},
	}
	current := &ContextBlock{}

	delta := DiffPacks(prior, current, "gcf")

	if len(delta.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(delta.Removed))
	}
	if len(delta.Added) != 0 {
		t.Errorf("expected 0 added, got %d", len(delta.Added))
	}
}
