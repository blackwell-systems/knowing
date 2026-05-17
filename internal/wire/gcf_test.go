package wire

import (
	"math"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	p := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  1847,
		TokenBudget: 5000,
		Symbols: []Symbol{
			{QualifiedName: "github.com/example/pkg.FuncA", Kind: "function", Score: 0.95, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "github.com/example/pkg.Server", Kind: "type", Score: 0.82, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "github.com/example/pkg.Server.Handle", Kind: "method", Score: 0.75, Provenance: "lsp_resolved", Distance: 1},
			{QualifiedName: "github.com/example/pkg.Store", Kind: "interface", Score: 0.60, Provenance: "ast_inferred", Distance: 2},
		},
		Edges: []Edge{
			{Source: "github.com/example/pkg.FuncA", Target: "github.com/example/pkg.Server", EdgeType: "calls"},
			{Source: "github.com/example/pkg.Server.Handle", Target: "github.com/example/pkg.Store", EdgeType: "references"},
		},
	}

	encoded := Encode(p)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v\nEncoded:\n%s", err, encoded)
	}

	// Verify header fields.
	if decoded.Tool != p.Tool {
		t.Errorf("Tool: got %q, want %q", decoded.Tool, p.Tool)
	}
	if decoded.TokensUsed != p.TokensUsed {
		t.Errorf("TokensUsed: got %d, want %d", decoded.TokensUsed, p.TokensUsed)
	}
	if decoded.TokenBudget != p.TokenBudget {
		t.Errorf("TokenBudget: got %d, want %d", decoded.TokenBudget, p.TokenBudget)
	}

	// Verify symbols.
	if len(decoded.Symbols) != len(p.Symbols) {
		t.Fatalf("Symbols: got %d, want %d", len(decoded.Symbols), len(p.Symbols))
	}
	for i, want := range p.Symbols {
		got := decoded.Symbols[i]
		if got.QualifiedName != want.QualifiedName {
			t.Errorf("Symbol[%d].QualifiedName: got %q, want %q", i, got.QualifiedName, want.QualifiedName)
		}
		if got.Kind != want.Kind {
			t.Errorf("Symbol[%d].Kind: got %q, want %q", i, got.Kind, want.Kind)
		}
		if math.Abs(got.Score-want.Score) > 0.01 {
			t.Errorf("Symbol[%d].Score: got %.2f, want %.2f", i, got.Score, want.Score)
		}
		if got.Provenance != want.Provenance {
			t.Errorf("Symbol[%d].Provenance: got %q, want %q", i, got.Provenance, want.Provenance)
		}
		if got.Distance != want.Distance {
			t.Errorf("Symbol[%d].Distance: got %d, want %d", i, got.Distance, want.Distance)
		}
	}

	// Verify edges.
	if len(decoded.Edges) != len(p.Edges) {
		t.Fatalf("Edges: got %d, want %d", len(decoded.Edges), len(p.Edges))
	}
	for i, want := range p.Edges {
		got := decoded.Edges[i]
		if got.Source != want.Source {
			t.Errorf("Edge[%d].Source: got %q, want %q", i, got.Source, want.Source)
		}
		if got.Target != want.Target {
			t.Errorf("Edge[%d].Target: got %q, want %q", i, got.Target, want.Target)
		}
		if got.EdgeType != want.EdgeType {
			t.Errorf("Edge[%d].EdgeType: got %q, want %q", i, got.EdgeType, want.EdgeType)
		}
	}
}

func TestRoundTripWithDiffStatus(t *testing.T) {
	p := &Payload{
		Tool:        "semantic_diff",
		TokensUsed:  2400,
		TokenBudget: 5000,
		Symbols: []Symbol{
			{QualifiedName: "pkg.FuncA", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "pkg.FuncB", Kind: "function", Score: 0.80, Provenance: "lsp_resolved", Distance: 0},
		},
		Edges: []Edge{
			{Source: "pkg.FuncA", Target: "pkg.FuncB", EdgeType: "calls", Status: "added"},
		},
	}

	encoded := Encode(p)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if len(decoded.Edges) != 1 {
		t.Fatalf("Edges: got %d, want 1", len(decoded.Edges))
	}
	if decoded.Edges[0].Status != "added" {
		t.Errorf("Edge status: got %q, want %q", decoded.Edges[0].Status, "added")
	}
}

func TestEncodeIdempotent(t *testing.T) {
	p := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  500,
		TokenBudget: 2000,
		Symbols: []Symbol{
			{QualifiedName: "pkg.A", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "pkg.B", Kind: "method", Score: 0.70, Provenance: "ast_inferred", Distance: 1},
		},
		Edges: []Edge{
			{Source: "pkg.A", Target: "pkg.B", EdgeType: "calls"},
		},
	}

	first := Encode(p)
	decoded, err := Decode(first)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	second := Encode(decoded)

	if first != second {
		t.Errorf("Encode not idempotent.\nFirst:\n%s\nSecond:\n%s", first, second)
	}
}

func TestDecodeInvalidHeader(t *testing.T) {
	_, err := Decode("INVALID header")
	if err == nil {
		t.Error("expected error for invalid header")
	}
}

func TestDecodeEmpty(t *testing.T) {
	_, err := Decode("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}
