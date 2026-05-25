package gotsextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractEnvReadEdges walks a function/method body and creates reads_env edges
// for each os.Getenv or os.LookupEnv call with a string literal argument.
//
// For a function like:
//
//	func Configure() {
//	    host := os.Getenv("DB_HOST")
//	    port, ok := os.LookupEnv("DB_PORT")
//	}
//
// This creates reads_env edges from the function node to env_var nodes:
//   - Configure -> env://DB_HOST
//   - Configure -> env://DB_PORT
func extractEnvReadEdges(funcNode *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	// Collect unique env var names to avoid duplicates.
	seen := make(map[string]bool)
	var nodes []types.Node
	var edges []types.Edge

	walkForEnvReads(body, opts, pkgPath, sourceHash, seen, &nodes, &edges)
	return nodes, edges
}

// walkForEnvReads recursively walks the AST looking for os.Getenv/os.LookupEnv calls.
func walkForEnvReads(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, seen map[string]bool, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		fn := node.ChildByFieldName("function")
		if fn != nil && fn.Type() == "selector_expression" {
			operand := fn.ChildByFieldName("operand")
			field := fn.ChildByFieldName("field")
			if operand != nil && field != nil && operand.Type() == "identifier" {
				opName := operand.Content(opts.Content)
				fieldName := field.Content(opts.Content)
				if opName == "os" && (fieldName == "Getenv" || fieldName == "LookupEnv") {
					varName := extractFirstStringLiteralArg(node, opts.Content)
					if varName != "" && !seen[varName] {
						seen[varName] = true
						n, e := makeEnvEdge(opts, pkgPath, sourceHash, varName)
						*nodes = append(*nodes, n)
						*edges = append(*edges, e)
					}
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForEnvReads(node.Child(i), opts, pkgPath, sourceHash, seen, nodes, edges)
	}
}

// extractFirstStringLiteralArg extracts the value of the first argument to a call_expression
// if it is an interpreted_string_literal. Returns empty string otherwise.
func extractFirstStringLiteralArg(callNode *sitter.Node, content []byte) string {
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
		if child.Type() == "interpreted_string_literal" {
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

// makeEnvEdge creates a target env_var node and a reads_env edge to it.
func makeEnvEdge(opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, varName string) (types.Node, types.Edge) {
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
