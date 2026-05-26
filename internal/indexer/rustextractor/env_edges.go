package rustextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractEnvReadEdges walks a Rust function/method body and creates reads_env
// edges for each env::var or env::var_os call with a string literal argument.
//
// For a function like:
//
//	fn configure() {
//	    let host = env::var("DB_HOST").unwrap();
//	    let path = std::env::var_os("PATH");
//	}
//
// This creates reads_env edges from the function node to env_var nodes:
//   - configure -> env://DB_HOST
//   - configure -> env://PATH
func extractEnvReadEdges(funcNode *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var nodes []types.Node
	var edges []types.Edge

	walkForRustEnvReads(body, opts, basePath, sourceHash, seen, &nodes, &edges)
	return nodes, edges
}

// walkForRustEnvReads recursively walks the AST looking for env::var / env::var_os calls.
func walkForRustEnvReads(node *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, seen map[string]bool, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		fn := node.ChildByFieldName("function")
		if fn != nil && fn.Type() == "scoped_identifier" {
			fnText := fn.Content(opts.Content)
			if isEnvVarCall(fnText) {
				varName := extractEnvVarStringArg(node, opts.Content)
				if varName != "" && !seen[varName] {
					seen[varName] = true
					n, e := makeRustEnvEdge(opts, basePath, sourceHash, varName)
					*nodes = append(*nodes, n)
					*edges = append(*edges, e)
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForRustEnvReads(node.Child(i), opts, basePath, sourceHash, seen, nodes, edges)
	}
}

// isEnvVarCall returns true if the scoped identifier text matches known env var patterns.
func isEnvVarCall(text string) bool {
	switch text {
	case "env::var", "env::var_os", "std::env::var", "std::env::var_os":
		return true
	}
	return false
}

// extractEnvVarStringArg extracts the value of the first string_literal argument
// to a call_expression. Returns empty string if not a string literal.
func extractEnvVarStringArg(callNode *sitter.Node, content []byte) string {
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

// makeRustEnvEdge creates a target env_var node and a reads_env edge to it.
func makeRustEnvEdge(opts types.ExtractOptions, basePath string, sourceHash types.Hash, varName string) (types.Node, types.Edge) {
	qn := "env://" + varName
	targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, qn, types.KindEnvVar)

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
