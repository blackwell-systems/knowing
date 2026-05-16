package context

import "github.com/blackwell-systems/knowing/internal/types"

// EstimateTokens returns an approximate token count for a given text string.
// Uses the heuristic that code averages ~4 characters per token.
func EstimateTokens(text string) int {
	return len(text) / 4
}

// EstimateNodeTokens estimates the token cost of including a node's full
// representation in context output.
func EstimateNodeTokens(n types.Node) int {
	return EstimateTokens(n.QualifiedName + " " + n.Kind + " " + n.Signature)
}
