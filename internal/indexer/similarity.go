package indexer

import (
	"sort"
	"strings"
	"unicode"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ComputeSimilarityEdges computes pairwise Jaccard similarity between function
// bodies within the same package. Functions with Jaccard > threshold get a
// "similar_to" edge. Only compares functions in the same package (not cross-package)
// to avoid O(n^2) explosion on large repos.
//
// Called after extraction, before snapshot computation.
func ComputeSimilarityEdges(nodes []types.Node, threshold float64) []types.Edge {
	if threshold <= 0 {
		threshold = 0.5
	}

	// maxEdgesPerNode caps how many similar_to edges any single node may emit,
	// preventing hub explosion from highly generic tokens.
	const maxEdgesPerNode = 5

	// Group function/method nodes by package.
	type tokenizedNode struct {
		node   types.Node
		tokens map[string]bool
	}
	packages := make(map[string][]tokenizedNode)

	for i := range nodes {
		n := &nodes[i]
		if n.Kind != "function" && n.Kind != "method" {
			continue
		}
		pkg := extractPackage(n.QualifiedName)
		tokens := tokenize(n.QualifiedName, n.Signature)
		if len(tokens) < 3 {
			// Too few tokens to produce meaningful similarity.
			continue
		}
		packages[pkg] = append(packages[pkg], tokenizedNode{node: *n, tokens: tokens})
	}

	// Compute pairwise Jaccard within each package.
	type candidate struct {
		source  types.Hash
		target  types.Hash
		jaccard float64
	}
	var allCandidates []candidate

	for pkg, group := range packages {
		if len(group) < 2 {
			continue
		}
		// Skip oversized packages: 16K functions = 140M pairwise comparisons.
		// These produce mostly noise and consume 10GB+ of memory.
		if len(group) > 500 {
			_ = pkg // suppress unused warning
			continue
		}
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				j64 := jaccard(group[i].tokens, group[j].tokens)
				if j64 > threshold {
					allCandidates = append(allCandidates, candidate{
						source:  group[i].node.NodeHash,
						target:  group[j].node.NodeHash,
						jaccard: j64,
					})
				}
			}
		}
	}

	// Sort candidates by Jaccard descending so highest-quality edges win the per-node cap.
	sort.Slice(allCandidates, func(i, j int) bool {
		return allCandidates[i].jaccard > allCandidates[j].jaccard
	})

	// Apply per-node cap.
	edgeCount := make(map[types.Hash]int)
	var edges []types.Edge

	for _, c := range allCandidates {
		if edgeCount[c.source] >= maxEdgesPerNode || edgeCount[c.target] >= maxEdgesPerNode {
			continue
		}
		edgeHash := types.ComputeEdgeHash(c.source, c.target, "similar_to", "similarity")
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: c.source,
			TargetHash: c.target,
			EdgeType:   "similar_to",
			Confidence: c.jaccard,
			Provenance: "similarity",
		})
		edgeCount[c.source]++
		edgeCount[c.target]++
	}

	return edges
}

// extractPackage extracts the package path from a qualified name.
// QualifiedName format: "{repoURL}://{pkgPath}.{TypeName}.{SymbolName}"
// We extract just the package path portion (after the last "://").
func extractPackage(qn string) string {
	// Find the LAST "://" separator (the repo URL may contain "://" as part of
	// https:// so we need the final occurrence which separates repo from path).
	idx := strings.LastIndex(qn, "://")
	if idx < 0 {
		return qn
	}
	rest := qn[idx+3:]

	// The package path is everything up to the last slash before the first dot
	// that starts the symbol chain. For Go: "internal/indexer.Indexer.IndexRepo"
	// -> package is "internal/indexer".
	// For most languages: find last '/' or use everything before first '.' after
	// the path separators end.
	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash >= 0 {
		// Check if there's a dot after the last slash (symbol separator).
		afterSlash := rest[lastSlash+1:]
		dotIdx := strings.Index(afterSlash, ".")
		if dotIdx >= 0 {
			return rest[:lastSlash+1+dotIdx]
		}
		return rest
	}
	// No slash: everything before the first dot.
	dotIdx := strings.Index(rest, ".")
	if dotIdx >= 0 {
		return rest[:dotIdx]
	}
	return rest
}

// tokenize splits a qualified name and signature into a bag-of-words token set.
// Splits on CamelCase boundaries, underscores, dots, slashes. Lowercases all.
// Removes tokens shorter than 3 characters.
func tokenize(qualifiedName, signature string) map[string]bool {
	raw := qualifiedName + " " + signature
	words := splitTokens(raw)
	tokens := make(map[string]bool, len(words))
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) >= 3 {
			tokens[lower] = true
		}
	}
	return tokens
}

// splitTokens splits a string on CamelCase boundaries, underscores, dots,
// slashes, parentheses, spaces, and other non-alpha characters.
func splitTokens(s string) []string {
	var result []string
	var current []rune

	flush := func() {
		if len(current) > 0 {
			result = append(result, string(current))
			current = current[:0]
		}
	}

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Separator characters: flush current token.
		if r == '_' || r == '.' || r == '/' || r == ':' || r == ' ' ||
			r == '(' || r == ')' || r == '[' || r == ']' || r == '{' || r == '}' ||
			r == ',' || r == '*' || r == '&' {
			flush()
			continue
		}

		// CamelCase boundary: uppercase after lowercase.
		if unicode.IsUpper(r) && len(current) > 0 && unicode.IsLower(current[len(current)-1]) {
			flush()
		}

		// CamelCase boundary: uppercase followed by lowercase (e.g., "HTMLParser" -> "HTML", "Parser").
		if unicode.IsUpper(r) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) && len(current) > 1 && unicode.IsUpper(current[len(current)-1]) {
			flush()
		}

		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			flush()
		}
	}
	flush()

	return result
}

// jaccard computes the Jaccard similarity between two token sets.
// Returns |A ∩ B| / |A ∪ B|.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	// Iterate over the smaller set for efficiency.
	if len(a) > len(b) {
		a, b = b, a
	}

	intersection := 0
	for token := range a {
		if b[token] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}
