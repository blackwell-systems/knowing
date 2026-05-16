package wire

import (
	"math"
	"testing"
)

func TestBinaryRoundTrip(t *testing.T) {
	p := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  1847,
		TokenBudget: 5000,
		Symbols: []Symbol{
			{
				QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.requireHash",
				Kind:          "function",
				Score:         0.78,
				Provenance:    "lsp_resolved",
				Distance:      0,
				Signature:     "func requireHash(args map[string]any, key string) (types.Hash, error)",
				Components:    Components{BlastRadius: 0.40, Confidence: 0.25, Recency: 0.06, Distance: 0.15},
			},
			{
				QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools",
				Kind:          "method",
				Score:         0.74,
				Provenance:    "lsp_resolved",
				Distance:      0,
				Signature:     "func (s *Server) registerTools()",
				Components:    Components{BlastRadius: 0.35, Confidence: 0.22, Recency: 0.08, Distance: 0.15},
			},
			{
				QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.NewServer",
				Kind:          "function",
				Score:         0.54,
				Provenance:    "lsp_resolved",
				Distance:      1,
				Signature:     "func NewServer(store types.GraphStore) *Server",
				Components:    Components{BlastRadius: 0.22, Confidence: 0.15, Recency: 0.04, Distance: 0.10},
			},
			{
				QualifiedName: "github.com/blackwell-systems/knowing/internal/types.GraphStore",
				Kind:          "interface",
				Score:         0.38,
				Provenance:    "ast_inferred",
				Distance:      2,
				Signature:     "type GraphStore interface",
				Components:    Components{BlastRadius: 0.18, Confidence: 0.12, Recency: 0.02, Distance: 0.05},
			},
		},
		Edges: []Edge{
			{Source: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Target: "github.com/blackwell-systems/knowing/internal/mcp.requireHash", EdgeType: "calls"},
			{Source: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Target: "github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools", EdgeType: "calls"},
			{Source: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Target: "github.com/blackwell-systems/knowing/internal/types.GraphStore", EdgeType: "references"},
		},
	}

	encoded, err := EncodeWith("kwb", p)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeWith("kwb", encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Header.
	if decoded.Tool != p.Tool {
		t.Errorf("Tool: got %q, want %q", decoded.Tool, p.Tool)
	}
	if decoded.TokensUsed != p.TokensUsed {
		t.Errorf("TokensUsed: got %d, want %d", decoded.TokensUsed, p.TokensUsed)
	}
	if decoded.TokenBudget != p.TokenBudget {
		t.Errorf("TokenBudget: got %d, want %d", decoded.TokenBudget, p.TokenBudget)
	}

	// Symbols.
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
		// float32 precision: compare within epsilon.
		if math.Abs(got.Score-want.Score) > 0.01 {
			t.Errorf("Symbol[%d].Score: got %.4f, want %.4f", i, got.Score, want.Score)
		}
		if got.Provenance != want.Provenance {
			t.Errorf("Symbol[%d].Provenance: got %q, want %q", i, got.Provenance, want.Provenance)
		}
		if got.Distance != want.Distance {
			t.Errorf("Symbol[%d].Distance: got %d, want %d", i, got.Distance, want.Distance)
		}
		if got.Signature != want.Signature {
			t.Errorf("Symbol[%d].Signature: got %q, want %q", i, got.Signature, want.Signature)
		}
	}

	// Edges.
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

func TestBinaryDiffStatus(t *testing.T) {
	p := &Payload{
		Tool:        "semantic_diff",
		TokensUsed:  500,
		TokenBudget: 2000,
		Symbols: []Symbol{
			{QualifiedName: "pkg.A", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "pkg.B", Kind: "function", Score: 0.80, Provenance: "lsp_resolved", Distance: 0},
		},
		Edges: []Edge{
			{Source: "pkg.A", Target: "pkg.B", EdgeType: "calls", Status: "added"},
			{Source: "pkg.B", Target: "pkg.A", EdgeType: "calls", Status: "removed"},
		},
	}

	encoded, err := EncodeWith("kwb", p)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeWith("kwb", encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Edges[0].Status != "added" {
		t.Errorf("Edge[0].Status: got %q, want %q", decoded.Edges[0].Status, "added")
	}
	if decoded.Edges[1].Status != "removed" {
		t.Errorf("Edge[1].Status: got %q, want %q", decoded.Edges[1].Status, "removed")
	}
}

func TestBinarySize(t *testing.T) {
	p := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  1847,
		TokenBudget: 5000,
		Symbols: []Symbol{
			{QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.requireHash", Kind: "function", Score: 0.78, Provenance: "lsp_resolved", Distance: 0, Signature: "func requireHash(args map[string]any, key string) (types.Hash, error)", Components: Components{BlastRadius: 0.40, Confidence: 0.25, Recency: 0.06, Distance: 0.15}},
			{QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools", Kind: "method", Score: 0.74, Provenance: "lsp_resolved", Distance: 0, Signature: "func (s *Server) registerTools()", Components: Components{BlastRadius: 0.35, Confidence: 0.22, Recency: 0.08, Distance: 0.15}},
			{QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Kind: "function", Score: 0.54, Provenance: "lsp_resolved", Distance: 1, Signature: "func NewServer(store types.GraphStore) *Server", Components: Components{BlastRadius: 0.22, Confidence: 0.15, Recency: 0.04, Distance: 0.10}},
		},
		Edges: []Edge{
			{Source: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Target: "github.com/blackwell-systems/knowing/internal/mcp.requireHash", EdgeType: "calls"},
			{Source: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Target: "github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools", EdgeType: "calls"},
		},
	}

	binOut, _ := EncodeWith("kwb", p)
	jsonOut, _ := EncodeWith("json", p)
	kwfOut, _ := EncodeWith("kwf", p)

	t.Logf("Binary: %d bytes", len(binOut))
	t.Logf("KWF:    %d bytes", len(kwfOut))
	t.Logf("JSON:   %d bytes", len(jsonOut))
	t.Logf("Binary vs JSON savings: %.1f%%", (1.0-float64(len(binOut))/float64(len(jsonOut)))*100)
	t.Logf("KWF vs JSON savings:    %.1f%%", (1.0-float64(len(kwfOut))/float64(len(jsonOut)))*100)

	// Binary should always be smaller than JSON in bytes.
	// KWF may be smaller than binary for small payloads (fewer structural bytes
	// to eliminate), but binary wins on large payloads and preserves full precision.
	if len(binOut) >= len(jsonOut) {
		t.Errorf("binary (%d bytes) should be smaller than JSON (%d bytes)", len(binOut), len(jsonOut))
	}
}

func TestBinaryRegistered(t *testing.T) {
	c, err := Get("kwb")
	if err != nil {
		t.Fatalf("binary codec not registered: %v", err)
	}
	if c.Name != "kwb" {
		t.Errorf("expected name 'binary', got %q", c.Name)
	}
}
