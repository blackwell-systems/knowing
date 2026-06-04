package context

import "github.com/blackwell-systems/knowing/internal/types"

// EstimateTokens returns an approximate token count for a given text string.
// Uses the heuristic that code averages ~4 characters per token.
func EstimateTokens(text string) int {
	return len(text) / 4
}

// EstimateNodeTokens estimates the token cost of including a node's full
// representation in context output. Uses format-aware scaling when format
// is provided via EstimateNodeTokensForFormat.
func EstimateNodeTokens(n types.Node) int {
	return EstimateTokens(n.QualifiedName + " " + n.Kind + " " + n.Signature)
}

// EstimateNodeTokensForFormat estimates token cost with format-aware scaling.
// GCF uses local IDs and positional encoding, producing ~84% fewer tokens
// than JSON for the same symbol data.
func EstimateNodeTokensForFormat(n types.Node, format string) int {
	base := EstimateNodeTokens(n)
	switch format {
	case "gcf":
		// GCF is ~84% smaller than JSON. Scale cost down so more symbols
		// fit within the same token budget.
		return max(1, base*16/100) // 16% of JSON cost
	case "gcb":
		// GCB is binary, ~74% byte savings. Token cost is similar to GCF
		// since the LLM never sees the binary directly.
		return max(1, base*26/100)
	default:
		return base
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
