package wire

import (
	"strings"
	"testing"
)

func TestRegistryBuiltinCodecs(t *testing.T) {
	// Both kwf and json should be registered by init().
	names := ListNames()
	if !strings.Contains(names, "kwf") {
		t.Errorf("expected 'kwf' in registry, got: %s", names)
	}
	if !strings.Contains(names, "json") {
		t.Errorf("expected 'json' in registry, got: %s", names)
	}
}

func TestRegistryGet(t *testing.T) {
	c, err := Get("kwf")
	if err != nil {
		t.Fatalf("Get(kwf): %v", err)
	}
	if c.Name != "kwf" {
		t.Errorf("expected name 'kwf', got %q", c.Name)
	}

	_, err = Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent codec")
	}
}

func TestEncodeWithKWF(t *testing.T) {
	p := &Payload{
		Tool:        "test",
		TokensUsed:  100,
		TokenBudget: 500,
		Symbols: []Symbol{
			{QualifiedName: "pkg.Func", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
		},
	}

	out, err := EncodeWith("kwf", p)
	if err != nil {
		t.Fatalf("EncodeWith(kwf): %v", err)
	}
	if !strings.HasPrefix(out, "KWF ") {
		t.Errorf("expected KWF header, got: %s", out[:20])
	}
}

func TestEncodeWithJSON(t *testing.T) {
	p := &Payload{
		Tool:        "test",
		TokensUsed:  100,
		TokenBudget: 500,
		Symbols: []Symbol{
			{QualifiedName: "pkg.Func", Kind: "function", Score: 0.90, Provenance: "lsp_resolved", Distance: 0},
		},
	}

	out, err := EncodeWith("json", p)
	if err != nil {
		t.Fatalf("EncodeWith(json): %v", err)
	}
	if !strings.HasPrefix(out, "{") {
		t.Errorf("expected JSON object, got: %s", out[:20])
	}
}

func TestJSONRoundTrip(t *testing.T) {
	p := &Payload{
		Tool:        "context_for_task",
		TokensUsed:  1500,
		TokenBudget: 5000,
		Symbols: []Symbol{
			{QualifiedName: "pkg.A", Kind: "function", Score: 0.95, Provenance: "lsp_resolved", Distance: 0, Signature: "func A()"},
			{QualifiedName: "pkg.B", Kind: "method", Score: 0.70, Provenance: "ast_inferred", Distance: 1},
		},
		Edges: []Edge{
			{Source: "pkg.A", Target: "pkg.B", EdgeType: "calls"},
		},
	}

	encoded, err := EncodeWith("json", p)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeWith("json", encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Tool != p.Tool {
		t.Errorf("Tool: got %q, want %q", decoded.Tool, p.Tool)
	}
	if len(decoded.Symbols) != len(p.Symbols) {
		t.Fatalf("Symbols: got %d, want %d", len(decoded.Symbols), len(p.Symbols))
	}
	if len(decoded.Edges) != len(p.Edges) {
		t.Fatalf("Edges: got %d, want %d", len(decoded.Edges), len(p.Edges))
	}
	if decoded.Symbols[0].Signature != "func A()" {
		t.Errorf("Signature: got %q, want %q", decoded.Symbols[0].Signature, "func A()")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	Register(&Codec{
		Name:   "kwf",
		Encode: func(p *Payload) (string, error) { return "", nil },
		Decode: func(s string) (*Payload, error) { return nil, nil },
	})
}
