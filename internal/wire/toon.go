// TOON (Token-Oriented Object Notation) encoder for knowing context output.
// Implements a subset of the TOON v3.0 spec sufficient for encoding
// ContextBlock payloads (symbols + edges + metadata).
//
// TOON is a compact, human-readable format designed for LLM contexts.
// It uses tabular arrays (header + rows) for uniform object collections,
// which is ideal for symbol lists where every entry has the same fields.
//
// Spec: https://github.com/toon-format/spec
package wire

import (
	"fmt"
	"strings"
)

// EncodeTOON encodes a Payload into TOON format.
func EncodeTOON(p *Payload) string {
	var b strings.Builder

	// Metadata as top-level object fields.
	b.WriteString("tool: ")
	b.WriteString(p.Tool)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "tokens_used: %d\n", p.TokensUsed)
	fmt.Fprintf(&b, "token_budget: %d\n", p.TokenBudget)
	b.WriteByte('\n')

	// Symbols as tabular array.
	if len(p.Symbols) > 0 {
		fmt.Fprintf(&b, "symbols[%d]{name,kind,score,signature,provenance,distance}:\n", len(p.Symbols))
		for _, sym := range p.Symbols {
			name := toonEscape(sym.QualifiedName)
			sig := toonEscape(sym.Signature)
			prov := sym.Provenance
			if prov == "" {
				prov = "-"
			}
			fmt.Fprintf(&b, "  %s,%s,%.3f,%s,%s,%d\n",
				name, sym.Kind, sym.Score, sig, prov, sym.Distance)
		}
		b.WriteByte('\n')
	}

	// Edges as tabular array.
	if len(p.Edges) > 0 {
		fmt.Fprintf(&b, "edges[%d]{source,target,type}:\n", len(p.Edges))
		for _, edge := range p.Edges {
			fmt.Fprintf(&b, "  %s,%s,%s\n",
				toonEscape(edge.Source), toonEscape(edge.Target), edge.EdgeType)
		}
	}

	return b.String()
}

// toonEscape quotes a string if it contains the active delimiter (comma),
// newlines, or leading/trailing whitespace. Per TOON spec section 7,
// strings are quoted only when required.
func toonEscape(s string) string {
	if s == "" {
		return `""`
	}
	needsQuote := false
	if strings.ContainsAny(s, ",\n\r\"") {
		needsQuote = true
	}
	if len(s) > 0 && (s[0] == ' ' || s[len(s)-1] == ' ') {
		needsQuote = true
	}
	if !needsQuote {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func init() {
	Register(&Codec{
		Name:        "toon",
		Description: "TOON (Token-Oriented Object Notation) v3.0: compact, human-readable, tabular arrays",
		Encode: func(p *Payload) (string, error) {
			return EncodeTOON(p), nil
		},
	})
}
