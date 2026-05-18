// TOON (Token-Oriented Object Notation) encoder for knowing context output.
// Uses the official toon-format/toon-go library for spec-conformant encoding.
//
// TOON is a compact, human-readable format designed for LLM contexts.
// It uses tabular arrays (header + rows) for uniform object collections,
// which is ideal for symbol lists where every entry has the same fields.
//
// Spec: https://github.com/toon-format/spec
// Library: https://github.com/toon-format/toon-go
package wire

import (
	"fmt"

	toon "github.com/toon-format/toon-go"
)

// toonPayload mirrors the Payload structure with toon struct tags
// for proper field naming in the output.
type toonPayload struct {
	Tool        string       `toon:"tool"`
	TokensUsed  int          `toon:"tokens_used"`
	TokenBudget int          `toon:"token_budget"`
	Symbols     []toonSymbol `toon:"symbols"`
	Edges       []toonEdge   `toon:"edges,omitempty"`
}

type toonSymbol struct {
	Name       string  `toon:"name"`
	Kind       string  `toon:"kind"`
	Score      float64 `toon:"score"`
	Signature  string  `toon:"signature"`
	Provenance string  `toon:"provenance"`
	Distance   int     `toon:"distance"`
}

type toonEdge struct {
	Source   string `toon:"source"`
	Target   string `toon:"target"`
	EdgeType string `toon:"type"`
}

// EncodeTOON encodes a Payload into TOON format using the official library.
func EncodeTOON(p *Payload) (string, error) {
	tp := toonPayload{
		Tool:        p.Tool,
		TokensUsed:  p.TokensUsed,
		TokenBudget: p.TokenBudget,
		Symbols:     make([]toonSymbol, len(p.Symbols)),
		Edges:       make([]toonEdge, len(p.Edges)),
	}

	for i, sym := range p.Symbols {
		prov := sym.Provenance
		if prov == "" {
			prov = "-"
		}
		tp.Symbols[i] = toonSymbol{
			Name:       sym.QualifiedName,
			Kind:       sym.Kind,
			Score:      sym.Score,
			Signature:  sym.Signature,
			Provenance: prov,
			Distance:   sym.Distance,
		}
	}

	for i, edge := range p.Edges {
		tp.Edges[i] = toonEdge{
			Source:   edge.Source,
			Target:   edge.Target,
			EdgeType: edge.EdgeType,
		}
	}

	result, err := toon.MarshalString(tp)
	if err != nil {
		return "", fmt.Errorf("toon encode: %w", err)
	}
	return result, nil
}

func init() {
	Register(&Codec{
		Name:        "toon",
		Description: "TOON (Token-Oriented Object Notation) v3.0: compact, human-readable, tabular arrays",
		Encode: func(p *Payload) (string, error) {
			return EncodeTOON(p)
		},
	})
}
