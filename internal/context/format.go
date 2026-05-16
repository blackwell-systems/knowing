package context

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatContextBlock renders a ContextBlock into the requested format.
// Supported formats: "xml" (default), "markdown", "json".
// Returns an error for unknown formats.
func FormatContextBlock(block *ContextBlock, format string) (string, error) {
	if format == "" {
		format = "xml"
	}

	switch format {
	case "xml":
		return formatXML(block)
	case "markdown":
		return formatMarkdown(block)
	case "json":
		return formatJSON(block)
	default:
		return "", fmt.Errorf("unknown format: %q (supported: xml, markdown, json)", format)
	}
}

// formatXML produces an XML representation of the context block,
// grouping symbols by their distance from the target.
func formatXML(block *ContextBlock) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "<context tokens_used=\"%d\" token_budget=\"%d\">\n",
		block.TokensUsed, block.TokenBudget)

	// Group symbols by distance
	targets := filterByDistance(block.Symbols, 0)
	related := filterByDistance(block.Symbols, 1)
	extended := filterByDistanceGTE(block.Symbols, 2)

	// Target symbols (distance 0)
	if len(targets) > 0 {
		b.WriteString("  <target_symbols>\n")
		for _, sym := range targets {
			writeXMLSymbol(&b, sym, true)
		}
		b.WriteString("  </target_symbols>\n")
	}

	// Related symbols (distance 1)
	if len(related) > 0 {
		b.WriteString("  <related_symbols>\n")
		for _, sym := range related {
			writeXMLSymbol(&b, sym, true)
		}
		b.WriteString("  </related_symbols>\n")
	}

	// Extended context (distance 2+)
	if len(extended) > 0 {
		b.WriteString("  <extended_context>\n")
		for _, sym := range extended {
			writeXMLSymbol(&b, sym, false)
		}
		b.WriteString("  </extended_context>\n")
	}

	// Relationship summary
	b.WriteString("  <relationship_summary>\n")
	fmt.Fprintf(&b, "    <total_symbols>%d</total_symbols>\n", len(block.Symbols))
	b.WriteString("    <by_distance>\n")
	distanceCounts := countByDistance(block.Symbols)
	for _, dc := range distanceCounts {
		fmt.Fprintf(&b, "      <distance hop=\"%d\" count=\"%d\"/>\n", dc.hop, dc.count)
	}
	b.WriteString("    </by_distance>\n")
	b.WriteString("  </relationship_summary>\n")

	b.WriteString("</context>\n")

	return b.String(), nil
}

// writeXMLSymbol writes a single symbol element to the builder.
// If includeConfidence is true, the confidence attribute is included.
func writeXMLSymbol(b *strings.Builder, sym RankedSymbol, includeConfidence bool) {
	if includeConfidence {
		fmt.Fprintf(b, "    <symbol name=%q kind=%q score=\"%.2f\" confidence=\"%.2f\" provenance=%q distance=\"%d\">\n",
			sym.QualifiedName, sym.Kind, sym.Score, sym.Confidence, sym.Provenance, sym.Distance)
	} else {
		fmt.Fprintf(b, "    <symbol name=%q kind=%q score=\"%.2f\" distance=\"%d\">\n",
			sym.QualifiedName, sym.Kind, sym.Score, sym.Distance)
	}
	if sym.Signature != "" {
		fmt.Fprintf(b, "      <signature>%s</signature>\n", xmlEscape(sym.Signature))
	}
	b.WriteString("    </symbol>\n")
}

// formatMarkdown produces a Markdown representation of the context block.
func formatMarkdown(block *ContextBlock) (string, error) {
	var b strings.Builder

	fmt.Fprintf(&b, "# Context (%d/%d tokens)\n\n", block.TokensUsed, block.TokenBudget)

	// Target symbols (distance 0)
	targets := filterByDistance(block.Symbols, 0)
	if len(targets) > 0 {
		b.WriteString("## Target Symbols\n")
		for _, sym := range targets {
			fmt.Fprintf(&b, "- `%s` (%s, score: %.2f, confidence: %.2f)\n",
				sym.QualifiedName, sym.Kind, sym.Score, sym.Confidence)
			if sym.Signature != "" {
				fmt.Fprintf(&b, "  Signature: `%s`\n", sym.Signature)
			}
		}
		b.WriteString("\n")
	}

	// Related symbols (distance 1)
	related := filterByDistance(block.Symbols, 1)
	if len(related) > 0 {
		b.WriteString("## Related Symbols (distance: 1)\n")
		for _, sym := range related {
			fmt.Fprintf(&b, "- `%s` (%s, score: %.2f)\n",
				sym.QualifiedName, sym.Kind, sym.Score)
			if sym.Signature != "" {
				fmt.Fprintf(&b, "  Signature: `%s`\n", sym.Signature)
			}
		}
		b.WriteString("\n")
	}

	// Extended context (distance 2+)
	extended := filterByDistanceGTE(block.Symbols, 2)
	if len(extended) > 0 {
		b.WriteString("## Extended Context (distance: 2+)\n")
		for _, sym := range extended {
			fmt.Fprintf(&b, "- `%s` (%s, score: %.2f)\n",
				sym.QualifiedName, sym.Kind, sym.Score)
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

// JSON output types with struct tags for marshaling.
type jsonOutput struct {
	TokensUsed  int          `json:"tokens_used"`
	TokenBudget int          `json:"token_budget"`
	Symbols     []jsonSymbol `json:"symbols"`
}

type jsonSymbol struct {
	QualifiedName string         `json:"qualified_name"`
	Kind          string         `json:"kind"`
	Score         float64        `json:"score"`
	Signature     string         `json:"signature"`
	Provenance    string         `json:"provenance"`
	Distance      int            `json:"distance"`
	Components    jsonComponents `json:"components"`
}

type jsonComponents struct {
	BlastRadius float64 `json:"blast_radius"`
	Confidence  float64 `json:"confidence"`
	Recency     float64 `json:"recency"`
	Distance    float64 `json:"distance"`
}

// formatJSON produces a JSON representation of the context block.
func formatJSON(block *ContextBlock) (string, error) {
	out := jsonOutput{
		TokensUsed:  block.TokensUsed,
		TokenBudget: block.TokenBudget,
		Symbols:     make([]jsonSymbol, 0, len(block.Symbols)),
	}

	for _, sym := range block.Symbols {
		out.Symbols = append(out.Symbols, jsonSymbol{
			QualifiedName: sym.QualifiedName,
			Kind:          sym.Kind,
			Score:         sym.Score,
			Signature:     sym.Signature,
			Provenance:    sym.Provenance,
			Distance:      sym.Distance,
			Components: jsonComponents{
				BlastRadius: sym.Components.BlastRadius,
				Confidence:  sym.Components.Confidence,
				Recency:     sym.Components.Recency,
				Distance:    sym.Components.Distance,
			},
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("json marshal: %w", err)
	}
	return string(data), nil
}

// Helper functions

// filterByDistance returns symbols at exactly the specified distance.
func filterByDistance(symbols []RankedSymbol, distance int) []RankedSymbol {
	var result []RankedSymbol
	for _, s := range symbols {
		if s.Distance == distance {
			result = append(result, s)
		}
	}
	return result
}

// filterByDistanceGTE returns symbols at or beyond the specified distance.
func filterByDistanceGTE(symbols []RankedSymbol, minDistance int) []RankedSymbol {
	var result []RankedSymbol
	for _, s := range symbols {
		if s.Distance >= minDistance {
			result = append(result, s)
		}
	}
	return result
}

// distanceCount holds a distance hop value and the number of symbols at that distance.
type distanceCount struct {
	hop   int
	count int
}

// countByDistance returns ordered distance counts for the relationship summary.
func countByDistance(symbols []RankedSymbol) []distanceCount {
	counts := make(map[int]int)
	for _, s := range symbols {
		counts[s.Distance]++
	}

	// Collect and sort by hop
	result := make([]distanceCount, 0, len(counts))
	for hop, count := range counts {
		result = append(result, distanceCount{hop: hop, count: count})
	}

	// Simple insertion sort (small N)
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].hop < result[j-1].hop; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}

	return result
}

// xmlEscape escapes special XML characters in a string.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
