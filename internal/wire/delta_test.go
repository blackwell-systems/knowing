package wire

import (
	"strings"
	"testing"
)

func TestEncodeDelta_Basic(t *testing.T) {
	d := &DeltaPayload{
		Tool:     "context_for_task",
		BaseRoot: "aaa111",
		NewRoot:  "bbb222",
		Removed: []Symbol{
			{QualifiedName: "pkg://OldFunc", Kind: "function", Score: 0.5},
		},
		Added: []Symbol{
			{QualifiedName: "pkg://NewFunc", Kind: "function", Score: 0.9, Provenance: "rwr"},
		},
		RemovedEdges: []Edge{
			{Source: "pkg://A", Target: "pkg://OldFunc", EdgeType: "calls"},
		},
		AddedEdges: []Edge{
			{Source: "pkg://A", Target: "pkg://NewFunc", EdgeType: "calls"},
		},
		DeltaTokens: 30,
		FullTokens:  200,
	}

	out := EncodeDelta(d)

	// Header.
	if !strings.Contains(out, "delta=true") {
		t.Error("missing delta=true in header")
	}
	if !strings.Contains(out, "base_root=aaa111") {
		t.Error("missing base_root")
	}
	if !strings.Contains(out, "new_root=bbb222") {
		t.Error("missing new_root")
	}
	if !strings.Contains(out, "savings=85%") {
		t.Errorf("expected savings=85%%, got output:\n%s", out)
	}

	// Sections.
	if !strings.Contains(out, "## removed") {
		t.Error("missing ## removed section")
	}
	if !strings.Contains(out, "fn pkg://OldFunc") {
		t.Error("missing removed symbol")
	}
	if !strings.Contains(out, "## added") {
		t.Error("missing ## added section")
	}
	if !strings.Contains(out, "fn pkg://NewFunc 0.90 rwr") {
		t.Error("missing added symbol")
	}
	if !strings.Contains(out, "## edges_removed") {
		t.Error("missing ## edges_removed section")
	}
	if !strings.Contains(out, "## edges_added") {
		t.Error("missing ## edges_added section")
	}
}

func TestEncodeDelta_EmptySections(t *testing.T) {
	d := &DeltaPayload{
		Tool:     "context_for_task",
		BaseRoot: "aaa",
		NewRoot:  "bbb",
		Added: []Symbol{
			{QualifiedName: "pkg://New", Kind: "function", Score: 0.8, Provenance: "rwr"},
		},
		DeltaTokens: 20,
		FullTokens:  100,
	}

	out := EncodeDelta(d)

	// Should have added section but no removed/edges sections.
	if !strings.Contains(out, "## added") {
		t.Error("missing ## added section")
	}
	if strings.Contains(out, "## removed") {
		t.Error("should not have ## removed section when no removals")
	}
	if strings.Contains(out, "## edges_removed") {
		t.Error("should not have ## edges_removed when no edge removals")
	}
	if strings.Contains(out, "## edges_added") {
		t.Error("should not have ## edges_added when no edge additions")
	}
}

func TestEncodeDelta_ZeroFullTokens(t *testing.T) {
	d := &DeltaPayload{
		Tool:        "context_for_task",
		BaseRoot:    "aaa",
		NewRoot:     "bbb",
		DeltaTokens: 0,
		FullTokens:  0,
	}

	out := EncodeDelta(d)

	// Should not panic, should contain savings=0%.
	if !strings.Contains(out, "savings=0%") {
		t.Errorf("expected savings=0%% for zero tokens, got:\n%s", out)
	}
}
