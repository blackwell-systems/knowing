package wire

import (
	"strings"
	"testing"
)

func TestSessionBasic(t *testing.T) {
	sess := NewSession()
	if sess.Size() != 0 {
		t.Errorf("new session should be empty, got size %d", sess.Size())
	}

	sess.Record([]Symbol{
		{QualifiedName: "pkg.A"},
		{QualifiedName: "pkg.B"},
	})

	if sess.Size() != 2 {
		t.Errorf("session size: got %d, want 2", sess.Size())
	}
	if !sess.Transmitted("pkg.A") {
		t.Error("pkg.A should be transmitted")
	}
	if sess.Transmitted("pkg.C") {
		t.Error("pkg.C should not be transmitted")
	}
}

func TestSessionDedup(t *testing.T) {
	sess := NewSession()

	// First response: 3 new symbols.
	p1 := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  500,
		TokenBudget: 2000,
		Symbols: []Symbol{
			{QualifiedName: "pkg.A", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "pkg.B", Kind: "method", Score: 0.70, Provenance: "lsp_resolved", Distance: 1},
			{QualifiedName: "pkg.C", Kind: "type", Score: 0.50, Provenance: "ast_inferred", Distance: 2},
		},
		Edges: []Edge{
			{Source: "pkg.A", Target: "pkg.B", EdgeType: "calls"},
		},
	}

	out1 := EncodeWithSession(p1, sess)

	// All symbols should be fully declared in the first response.
	if strings.Contains(out1, "previously transmitted") {
		t.Error("first response should not have any previously transmitted symbols")
	}
	if !strings.Contains(out1, "session=true") {
		t.Error("session header missing")
	}
	if !strings.Contains(out1, "pkg.A") {
		t.Error("first response should contain pkg.A declaration")
	}

	// Session should now have 3 symbols.
	if sess.Size() != 3 {
		t.Errorf("session size after first response: got %d, want 3", sess.Size())
	}

	// Second response: 2 symbols, one previously transmitted.
	p2 := &Payload{
		Tool:        "context_for_files",
		TokensUsed:  300,
		TokenBudget: 2000,
		Symbols: []Symbol{
			{QualifiedName: "pkg.A", Kind: "function", Score: 0.85, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "pkg.D", Kind: "function", Score: 0.60, Provenance: "lsp_resolved", Distance: 0},
		},
		Edges: []Edge{
			{Source: "pkg.D", Target: "pkg.A", EdgeType: "calls"},
		},
	}

	out2 := EncodeWithSession(p2, sess)

	// pkg.A should be a bare reference.
	if !strings.Contains(out2, "previously transmitted") {
		t.Error("second response should have previously transmitted symbols")
	}
	// pkg.D should be fully declared.
	if !strings.Contains(out2, "pkg.D") {
		t.Error("second response should contain pkg.D declaration")
	}
	// pkg.A full declaration should NOT appear.
	if strings.Contains(out2, "fn pkg.A") {
		t.Error("pkg.A should not be fully declared in second response")
	}

	// Session should now have 4 symbols.
	if sess.Size() != 4 {
		t.Errorf("session size after second response: got %d, want 4", sess.Size())
	}
}

func TestSessionTokenSavings(t *testing.T) {
	sess := NewSession()

	p := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  1000,
		TokenBudget: 5000,
		Symbols: []Symbol{
			{QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.requireHash", Kind: "function", Score: 0.78, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools", Kind: "method", Score: 0.74, Provenance: "lsp_resolved", Distance: 0},
			{QualifiedName: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Kind: "function", Score: 0.54, Provenance: "lsp_resolved", Distance: 1},
		},
		Edges: []Edge{
			{Source: "github.com/blackwell-systems/knowing/internal/mcp.NewServer", Target: "github.com/blackwell-systems/knowing/internal/mcp.requireHash", EdgeType: "calls"},
		},
	}

	// First call: no savings (all new).
	out1 := EncodeWithSession(p, sess)
	noSession := Encode(p)
	if len(out1) >= len(noSession)+20 {
		// Session adds "session=true" to header, so slightly longer on first call. That's fine.
		t.Logf("First call: session=%d bytes, no-session=%d bytes", len(out1), len(noSession))
	}

	// Second call with same symbols: should be much shorter.
	out2 := EncodeWithSession(p, sess)
	savings := 1.0 - float64(len(out2))/float64(len(out1))
	t.Logf("Second call savings: %.1f%% (session=%d bytes vs first=%d bytes)", savings*100, len(out2), len(out1))

	if savings < 0.30 {
		t.Errorf("session dedup should save at least 30%% on repeated symbols, got %.1f%%", savings*100)
	}
}

func TestSessionReset(t *testing.T) {
	sess := NewSession()
	sess.Record([]Symbol{{QualifiedName: "pkg.A"}})
	if sess.Size() != 1 {
		t.Fatal("expected size 1")
	}
	sess.Reset()
	if sess.Size() != 0 {
		t.Errorf("after reset: got size %d, want 0", sess.Size())
	}
	if sess.Transmitted("pkg.A") {
		t.Error("pkg.A should not be transmitted after reset")
	}
}

func TestSessionNil(t *testing.T) {
	// EncodeWithSession with nil session should behave like Encode.
	p := &Payload{
		Tool:        "test",
		TokensUsed:  100,
		TokenBudget: 500,
		Symbols: []Symbol{
			{QualifiedName: "pkg.A", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
		},
	}

	withNil := EncodeWithSession(p, nil)
	regular := Encode(p)
	if withNil != regular {
		t.Errorf("nil session should produce same output as Encode")
	}
}
