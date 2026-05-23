package gotsextractor

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// makeThrowEdge creates a throws edge for panic() calls.
func makeThrowEdge(opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, errorName string) *types.Edge {
	var targetPkg string
	if errorName == "panic" {
		targetPkg = "builtin"
	} else {
		targetPkg = "errors"
	}
	targetHash := types.ComputeNodeHash(opts.RepoURL, targetPkg, types.EmptyHash, errorName, "error")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Throws, provenance)
	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.Throws,
		Confidence: 0.7,
		Provenance: provenance,
	}
}

// checkReturnForThrows checks a return_statement node for error-constructing calls
// (fmt.Errorf, errors.New) and returns throws edges.
func checkReturnForThrows(returnNode *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string) []types.Edge {
	var edges []types.Edge
	// Walk children of the return statement (non-recursive since we only need
	// immediate call expressions within the return).
	walkReturnChildrenForErrors(returnNode, opts, pkgPath, sourceHash, imports, &edges)
	return edges
}

// walkReturnChildrenForErrors walks a return statement's children looking for
// fmt.Errorf or errors.New calls. This is a shallow walk (only within the return).
func walkReturnChildrenForErrors(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string, edges *[]types.Edge) {
	if node == nil {
		return
	}
	if node.Type() == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil && funcNode.Type() == "selector_expression" {
			operandNode := funcNode.ChildByFieldName("operand")
			fieldNode := funcNode.ChildByFieldName("field")
			if operandNode != nil && fieldNode != nil {
				pkg := operandNode.Content(opts.Content)
				fn := fieldNode.Content(opts.Content)

				var errorName string
				if fn == "Errorf" {
					if importPath, ok := imports[pkg]; ok && importPath == "fmt" {
						errorName = "fmt.Errorf"
					} else if pkg == "fmt" {
						errorName = "fmt.Errorf"
					}
				} else if fn == "New" {
					if importPath, ok := imports[pkg]; ok && importPath == "errors" {
						errorName = "errors.New"
					} else if pkg == "errors" {
						errorName = "errors.New"
					}
				}

				if errorName != "" {
					if edge := makeThrowEdge(opts, pkgPath, sourceHash, errorName); edge != nil {
						*edges = append(*edges, *edge)
					}
				}
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkReturnChildrenForErrors(node.Child(i), opts, pkgPath, sourceHash, imports, edges)
	}
}

// NOTE: tryExtractRoute is defined in extractor.go with signature:
// func tryExtractRoute(funcNode, callNode *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string) *routeSymbol
// It is called from walkBodyOnce in walk_body.go.

// tryExtractFeatureFlag checks if a call is a feature flag SDK call.
// Delegates to the existing detection logic but for a single call node.
func tryExtractFeatureFlag(callNode, funcNode *sitter.Node, fnText string, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, seen map[types.Hash]struct{}) ([]types.Node, []types.Edge) {
	// Feature flag patterns: client.BoolVariation("flag"), unleash.IsEnabled("flag"), etc.
	if !isFeatureFlagCall(fnText) {
		return nil, nil
	}

	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return nil, nil
	}

	flagName := extractFirstStringArg(args, opts.Content)
	if flagName == "" {
		return nil, nil
	}

	// Create synthetic flag node + gated_by_flag edge.
	flagHash := types.ComputeNodeHash(opts.RepoURL, "feature_flags", types.EmptyHash, flagName, "feature_flag")
	if _, exists := seen[flagHash]; exists {
		return nil, nil
	}
	seen[flagHash] = struct{}{}

	flagNode := types.Node{
		NodeHash:      flagHash,
		QualifiedName: opts.RepoURL + "://feature_flags/" + flagName,
		Kind:          "feature_flag",
		Line:          int(callNode.StartPoint().Row) + 1,
	}

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(funcNodeHash, flagHash, edgetype.GatedByFlag, provenance)
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: funcNodeHash,
		TargetHash: flagHash,
		EdgeType:   edgetype.GatedByFlag,
		Confidence: 0.8,
		Provenance: provenance,
	}

	return []types.Node{flagNode}, []types.Edge{edge}
}

// isFeatureFlagCall checks if a function call text matches feature flag SDK patterns.
func isFeatureFlagCall(fnText string) bool {
	patterns := []string{
		"BoolVariation", "StringVariation", "IntVariation", "Float64Variation",
		"JSONVariation", "Variation",
		"IsEnabled", "isEnabled", "IsFeatureEnabled", "isFeatureEnabled",
		"GetFeatureFlag", "getFeatureFlag", "FeatureEnabled", "featureEnabled",
	}
	for _, p := range patterns {
		if strings.HasSuffix(fnText, "."+p) || fnText == p {
			return true
		}
	}
	return false
}

// tryExtractGoEndpoint checks if a call is an HTTP client call with a URL.
func tryExtractGoEndpoint(callNode, funcNode *sitter.Node, fnText string, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, imports map[string]string, seen map[types.Hash]struct{}) ([]types.Node, []types.Edge) {
	// HTTP client patterns: http.Get("/api/..."), http.Post(...), client.Get(...)
	if !isHTTPClientCall(fnText, imports) {
		return nil, nil
	}

	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return nil, nil
	}

	url := extractFirstStringArg(args, opts.Content)
	if url == "" || !looksLikeURL(url) {
		return nil, nil
	}

	// Normalize URL to just the path.
	path := extractPathFromURL(url)
	if path == "" {
		return nil, nil
	}

	epHash := types.ComputeNodeHash(opts.RepoURL, "endpoints", types.EmptyHash, path, "endpoint")
	if _, exists := seen[epHash]; exists {
		return nil, nil
	}
	seen[epHash] = struct{}{}

	epNode := types.Node{
		NodeHash:      epHash,
		QualifiedName: opts.RepoURL + "://endpoints" + path,
		Kind:          "endpoint",
		Line:          int(callNode.StartPoint().Row) + 1,
	}

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(funcNodeHash, epHash, edgetype.ConsumesEndpoint, provenance)
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: funcNodeHash,
		TargetHash: epHash,
		EdgeType:   edgetype.ConsumesEndpoint,
		Confidence: 0.6,
		Provenance: provenance,
	}

	return []types.Node{epNode}, []types.Edge{edge}
}

// isHTTPClientCall checks if the function text looks like an HTTP client call.
func isHTTPClientCall(fnText string, imports map[string]string) bool {
	httpMethods := []string{".Get", ".Post", ".Put", ".Delete", ".Patch", ".Do", ".NewRequest"}
	for _, m := range httpMethods {
		if strings.HasSuffix(fnText, m) {
			// Check if the receiver maps to net/http or a known HTTP client.
			parts := strings.SplitN(fnText, ".", 2)
			if len(parts) == 2 {
				receiver := parts[0]
				if receiver == "http" || receiver == "client" || receiver == "Client" {
					return true
				}
				if importPath, ok := imports[receiver]; ok && strings.Contains(importPath, "net/http") {
					return true
				}
			}
			return false
		}
	}
	return false
}

// looksLikeURL checks if a string looks like a URL or path.
func looksLikeURL(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// extractPathFromURL extracts the path component from a URL string.
func extractPathFromURL(url string) string {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		// Find the path after the host.
		idx := strings.Index(url[8:], "/")
		if idx < 0 {
			return ""
		}
		return url[8+idx:]
	}
	return url
}
