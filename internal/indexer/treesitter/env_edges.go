package treesitter

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractEnvReadEdges walks a Python function/method body and creates reads_env
// edges for each environment variable access it detects.
//
// Patterns matched:
//   - os.environ["VAR"] (subscript with os.environ as value)
//   - os.environ.get("VAR") (call with os.environ.get as function)
//   - os.getenv("VAR") (call with os.getenv as function)
//
// For each match, a target node (kind=env_var, QN=env://VAR_NAME) and an edge
// (type=reads_env, confidence=0.9, provenance=ast_inferred) are created.
func extractEnvReadEdges(funcNode *sitter.Node, opts types.ExtractOptions, classContext string, funcHash types.Hash) ([]types.Node, []types.Edge) {
	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	// Collect unique env var names.
	envVars := make(map[string]bool)
	walkForEnvReads(body, opts.Content, envVars)

	if len(envVars) == 0 {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	provenance := "ast_inferred"

	for varName := range envVars {
		qn := "env://" + varName
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, qn, types.KindEnvVar)

		nodes = append(nodes, types.Node{
			NodeHash:      targetHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          types.KindEnvVar,
		})

		edgeHash := types.ComputeEdgeHash(funcHash, targetHash, edgetype.ReadsEnv, provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: funcHash,
			TargetHash: targetHash,
			EdgeType:   edgetype.ReadsEnv,
			Confidence: 0.9,
			Provenance: provenance,
		})
	}

	return nodes, edges
}

// walkForEnvReads recursively walks a Python AST subtree collecting environment
// variable names from os.environ["X"], os.environ.get("X"), and os.getenv("X").
func walkForEnvReads(node *sitter.Node, content []byte, envVars map[string]bool) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "subscript":
		// os.environ["VAR"]
		if varName := extractOsEnvironSubscript(node, content); varName != "" {
			envVars[varName] = true
		}
	case "call":
		// os.environ.get("VAR") or os.getenv("VAR")
		if varName := extractOsEnvCall(node, content); varName != "" {
			envVars[varName] = true
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForEnvReads(node.Child(i), content, envVars)
	}
}

// extractOsEnvironSubscript checks if a subscript node is os.environ["KEY"]
// and returns the key string, or "" if it doesn't match.
func extractOsEnvironSubscript(node *sitter.Node, content []byte) string {
	// subscript node: value=attribute(os.environ), subscript=string
	value := node.ChildByFieldName("value")
	subscript := node.ChildByFieldName("subscript")

	if value == nil || subscript == nil {
		return ""
	}

	if !isOsEnvironAttribute(value, content) {
		return ""
	}

	// The subscript should be a string literal.
	return extractStringLiteral(subscript, content)
}

// extractOsEnvCall checks if a call node is os.environ.get("KEY") or
// os.getenv("KEY") and returns the key string, or "" if it doesn't match.
func extractOsEnvCall(node *sitter.Node, content []byte) string {
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil || funcNode.Type() != "attribute" {
		return ""
	}

	objectNode := funcNode.ChildByFieldName("object")
	attrNode := funcNode.ChildByFieldName("attribute")
	if objectNode == nil || attrNode == nil {
		return ""
	}

	attrName := attrNode.Content(content)

	// Pattern 1: os.getenv("VAR")
	if objectNode.Type() == "identifier" && objectNode.Content(content) == "os" && attrName == "getenv" {
		return extractFirstStringArg(node, content)
	}

	// Pattern 2: os.environ.get("VAR")
	if attrName == "get" && isOsEnvironAttribute(objectNode, content) {
		return extractFirstStringArg(node, content)
	}

	return ""
}

// isOsEnvironAttribute checks if a node is the attribute expression "os.environ".
func isOsEnvironAttribute(node *sitter.Node, content []byte) bool {
	if node.Type() != "attribute" {
		return false
	}
	obj := node.ChildByFieldName("object")
	attr := node.ChildByFieldName("attribute")
	if obj == nil || attr == nil {
		return false
	}
	return obj.Type() == "identifier" && obj.Content(content) == "os" && attr.Content(content) == "environ"
}

// extractFirstStringArg extracts the first string argument from a call node.
// Returns "" if the first argument is not a string literal.
func extractFirstStringArg(callNode *sitter.Node, content []byte) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child == nil {
			continue
		}
		// Skip punctuation nodes (parentheses, commas).
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
		return extractStringLiteral(child, content)
	}
	return ""
}

// extractStringLiteral extracts the content of a string node, stripping quotes.
func extractStringLiteral(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	if node.Type() != "string" {
		return ""
	}
	// String content includes quotes. Strip them.
	s := node.Content(content)
	if len(s) >= 2 {
		// Handle single, double, triple quotes.
		if (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
			return s[1 : len(s)-1]
		}
	}
	return ""
}
