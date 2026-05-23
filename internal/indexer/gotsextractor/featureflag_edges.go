package gotsextractor

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// featureFlagMethods is the set of method names on feature flag SDK clients
// that indicate a feature flag check. These are called as selector expressions
// like client.BoolVariation("flag-name", ...).
var featureFlagMethods = map[string]bool{
	"BoolVariation":    true,
	"StringVariation":  true,
	"IntVariation":     true,
	"Float64Variation": true,
	"JSONVariation":    true,
	"IsEnabled":        true,
	"isEnabled":        true,
	"GetFlag":          true,
	"Enabled":          true,
}

// featureFlagFunctions is the set of plain function names (not method calls)
// that indicate a feature flag check. These are called as identifiers like
// IsFeatureEnabled("flag-name").
var featureFlagFunctions = map[string]bool{
	"IsFeatureEnabled":  true,
	"isFeatureEnabled":  true,
}

// ExtractFeatureFlagEdges walks function bodies looking for feature flag SDK
// call patterns and creates 'gated_by_flag' edges from the enclosing function
// to a synthetic flag node.
// Parameters:
//
//	body: tree-sitter node of the function body
//	opts: standard ExtractOptions
//	pkgPath: resolved Go package path
//	funcNodeHash: hash of the enclosing function node
//	imports: import alias map (unused but kept for interface consistency)
//
// Returns: (flagNodes []types.Node, edges []types.Edge)
func ExtractFeatureFlagEdges(body *sitter.Node, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, imports map[string]string) ([]types.Node, []types.Edge) {
	if body == nil {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	seen := make(map[types.Hash]struct{})

	walkForFeatureFlags(body, opts, pkgPath, funcNodeHash, &nodes, &edges, seen)

	return nodes, edges
}

// walkForFeatureFlags recursively walks nodes looking for call_expression nodes
// that match feature flag SDK patterns.
func walkForFeatureFlags(node *sitter.Node, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			flagName := matchFeatureFlagCall(funcNode, node, opts.Content)
			if flagName != "" {
				addFlagEdge(flagName, opts, funcNodeHash, nodes, edges, seen)
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForFeatureFlags(node.Child(i), opts, pkgPath, funcNodeHash, nodes, edges, seen)
	}
}

// matchFeatureFlagCall checks if a call expression matches a feature flag pattern
// and returns the flag name if found, or empty string otherwise.
func matchFeatureFlagCall(funcNode, callNode *sitter.Node, content []byte) string {
	switch funcNode.Type() {
	case "selector_expression":
		// Pattern: receiver.MethodName("flag-name", ...)
		fieldNode := funcNode.ChildByFieldName("field")
		if fieldNode == nil {
			return ""
		}
		methodName := fieldNode.Content(content)
		if !featureFlagMethods[methodName] {
			return ""
		}
		return extractFlagNameFromArgs(callNode, content)

	case "identifier":
		// Pattern: IsFeatureEnabled("flag-name")
		funcName := funcNode.Content(content)
		if !featureFlagFunctions[funcName] {
			return ""
		}
		return extractFlagNameFromArgs(callNode, content)
	}

	return ""
}

// extractFlagNameFromArgs extracts the first string literal argument from a
// call_expression node. Returns empty string if no string literal is found.
func extractFlagNameFromArgs(callNode *sitter.Node, content []byte) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}

	// Walk arguments looking for the first string literal.
	for i := 0; i < int(args.ChildCount()); i++ {
		arg := args.Child(i)
		switch arg.Type() {
		case "interpreted_string_literal":
			// Strip the quotes.
			raw := arg.Content(content)
			return strings.Trim(raw, `"`)
		case "raw_string_literal":
			// Strip the backtick quotes.
			raw := arg.Content(content)
			return strings.Trim(raw, "`")
		}
	}

	return ""
}

// addFlagEdge creates a synthetic feature_flag node and a 'gated_by_flag' edge,
// deduplicating by edge hash.
func addFlagEdge(flagName string, opts types.ExtractOptions, funcNodeHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	flagHash := types.ComputeNodeHash(opts.RepoURL, "flags", types.EmptyHash, flagName, "feature_flag")

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(funcNodeHash, flagHash, edgetype.GatedByFlag, provenance)

	// Deduplicate by edge hash (same function + same flag = one edge).
	if _, exists := seen[edgeHash]; exists {
		return
	}
	seen[edgeHash] = struct{}{}

	// Create synthetic flag node.
	flagNode := types.Node{
		NodeHash:      flagHash,
		FileHash:      types.EmptyHash, // synthetic, not tied to a file
		QualifiedName: fmt.Sprintf("%s://flags.%s", opts.RepoURL, flagName),
		Kind:          "feature_flag",
		Signature:     "flag " + flagName,
	}
	*nodes = append(*nodes, flagNode)

	// Create gated_by_flag edge.
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: funcNodeHash,
		TargetHash: flagHash,
		EdgeType:   edgetype.GatedByFlag,
		Confidence: 0.8,
		Provenance: provenance,
	}
	*edges = append(*edges, edge)
}
