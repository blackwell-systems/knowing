package tsextractor

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// ExtractEnvReadEdges walks a function body for process.env access patterns
// and creates 'reads_env' edges from the function to synthetic env_var nodes.
//
// Patterns matched:
//   - process.env.GITHUB_TOKEN (member_expression chain)
//   - process.env["NPM_TOKEN"] (subscript_expression with string index)
func ExtractEnvReadEdges(body *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	if body == nil {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	seen := make(map[types.Hash]struct{})

	walkForEnvReads(body, opts, qnamePrefix, sourceHash, &nodes, &edges, seen)
	return nodes, edges
}

// walkForEnvReads recursively walks nodes looking for process.env access patterns.
func walkForEnvReads(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "member_expression":
		// Pattern: process.env.VAR_NAME
		if varName := matchProcessEnvMember(node, opts.Content); varName != "" {
			addEnvEdge(varName, opts, qnamePrefix, sourceHash, nodes, edges, seen)
			return // Don't recurse into children; we already matched.
		}
	case "subscript_expression":
		// Pattern: process.env["VAR_NAME"]
		if varName := matchProcessEnvSubscript(node, opts.Content); varName != "" {
			addEnvEdge(varName, opts, qnamePrefix, sourceHash, nodes, edges, seen)
			return
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForEnvReads(node.Child(i), opts, qnamePrefix, sourceHash, nodes, edges, seen)
	}
}

// matchProcessEnvMember checks if a member_expression is process.env.X and returns X.
// Returns "" if no match.
func matchProcessEnvMember(node *sitter.Node, content []byte) string {
	// node must be member_expression with:
	//   object: member_expression (process.env)
	//   property: identifier (the var name)
	obj := node.ChildByFieldName("object")
	prop := node.ChildByFieldName("property")
	if obj == nil || prop == nil {
		return ""
	}
	if obj.Type() != "member_expression" || prop.Type() != "property_identifier" {
		// Also try plain "identifier" for property (tree-sitter version differences).
		if prop.Type() != "identifier" {
			return ""
		}
	}

	if !isProcessEnv(obj, content) {
		return ""
	}

	return prop.Content(content)
}

// matchProcessEnvSubscript checks if a subscript_expression is process.env["X"] and returns X.
func matchProcessEnvSubscript(node *sitter.Node, content []byte) string {
	obj := node.ChildByFieldName("object")
	idx := node.ChildByFieldName("index")
	if obj == nil || idx == nil {
		return ""
	}

	if !isProcessEnv(obj, content) {
		return ""
	}

	// Index must be a string literal.
	if idx.Type() != "string" {
		return ""
	}

	// Extract the string content, trimming quotes.
	val := idx.Content(content)
	if len(val) >= 2 {
		// Remove surrounding quotes (single or double).
		val = val[1 : len(val)-1]
	}
	return val
}

// isProcessEnv checks if a node represents the expression "process.env".
func isProcessEnv(node *sitter.Node, content []byte) bool {
	if node.Type() != "member_expression" {
		return false
	}
	obj := node.ChildByFieldName("object")
	prop := node.ChildByFieldName("property")
	if obj == nil || prop == nil {
		return false
	}
	return obj.Content(content) == "process" && prop.Content(content) == "env"
}

// addEnvEdge creates an env_var node and reads_env edge, deduplicating by edge hash.
func addEnvEdge(varName string, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	qn := fmt.Sprintf("env://%s", varName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, qn, types.KindEnvVar)

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, nodeHash, edgetype.ReadsEnv, provenance)

	if _, exists := seen[edgeHash]; exists {
		return
	}
	seen[edgeHash] = struct{}{}

	envNode := types.Node{
		NodeHash:      nodeHash,
		FileHash:      types.EmptyHash,
		QualifiedName: qn,
		Kind:          types.KindEnvVar,
		Signature:     varName,
	}
	*nodes = append(*nodes, envNode)

	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: nodeHash,
		EdgeType:   edgetype.ReadsEnv,
		Confidence: 0.9,
		Provenance: provenance,
	}
	*edges = append(*edges, edge)
}
