package javaextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractEnvReadEdges walks a Java method body and creates reads_env edges
// for each System.getenv("KEY") call with a string literal argument.
//
// For a method like:
//
//	public void configure() {
//	    String host = System.getenv("DB_HOST");
//	    String port = System.getenv("DB_PORT");
//	}
//
// This creates reads_env edges from the method node to env_var nodes:
//   - configure -> env://DB_HOST
//   - configure -> env://DB_PORT
func extractEnvReadEdges(methodNode *sitter.Node, opts types.ExtractOptions, pkgPath string, className string, methodHash types.Hash) ([]types.Node, []types.Edge) {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var nodes []types.Node
	var edges []types.Edge

	walkForJavaEnvReads(body, opts, pkgPath, methodHash, seen, &nodes, &edges)
	return nodes, edges
}

// walkForJavaEnvReads recursively walks the AST looking for System.getenv() calls.
func walkForJavaEnvReads(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, seen map[string]bool, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	if node.Type() == "method_invocation" {
		object := node.ChildByFieldName("object")
		name := node.ChildByFieldName("name")
		if object != nil && name != nil {
			objContent := object.Content(opts.Content)
			nameContent := name.Content(opts.Content)
			if objContent == "System" && nameContent == "getenv" {
				varName := extractFirstJavaStringArg(node, opts.Content)
				if varName != "" && !seen[varName] {
					seen[varName] = true
					n, e := makeJavaEnvEdge(opts, pkgPath, sourceHash, varName)
					*nodes = append(*nodes, n)
					*edges = append(*edges, e)
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForJavaEnvReads(node.Child(i), opts, pkgPath, sourceHash, seen, nodes, edges)
	}
}

// extractFirstJavaStringArg extracts the value of the first string_literal argument
// in a method_invocation's arguments. Returns empty string if not found.
func extractFirstJavaStringArg(callNode *sitter.Node, content []byte) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}

	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
		// First real argument found.
		if child.Type() == "string_literal" {
			raw := child.Content(content)
			// Strip surrounding quotes.
			if len(raw) >= 2 {
				return raw[1 : len(raw)-1]
			}
		}
		return ""
	}
	return ""
}

// makeJavaEnvEdge creates a target env_var node and a reads_env edge to it.
func makeJavaEnvEdge(opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, varName string) (types.Node, types.Edge) {
	qn := "env://" + varName
	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qn, types.KindEnvVar)

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.ReadsEnv, provenance)

	node := types.Node{
		NodeHash:      targetHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          types.KindEnvVar,
	}

	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.ReadsEnv,
		Confidence: 0.9,
		Provenance: provenance,
	}

	return node, edge
}
