package rustextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractProcessExecEdges walks a Rust function/method body and creates
// executes_process edges for each Command::new call.
//
// For a function like:
//
//	fn build() {
//	    let cmd = Command::new("cargo");
//	    let docker = std::process::Command::new("docker");
//	}
//
// This creates executes_process edges from the function node to process nodes:
//   - build -> process://cargo
//   - build -> process://docker
//
// If the command argument is not a string literal, uses QN="process://dynamic".
func extractProcessExecEdges(funcNode *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var nodes []types.Node
	var edges []types.Edge

	walkForRustProcessExec(body, opts, basePath, sourceHash, seen, &nodes, &edges)
	return nodes, edges
}

// walkForRustProcessExec recursively walks the AST looking for Command::new calls.
func walkForRustProcessExec(node *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, seen map[string]bool, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		fn := node.ChildByFieldName("function")
		if fn != nil && fn.Type() == "scoped_identifier" {
			fnText := fn.Content(opts.Content)
			if isCommandNewCall(fnText) {
				cmdName := extractFirstRustStringArgOrDynamic(node, opts.Content)
				if !seen[cmdName] {
					seen[cmdName] = true
					n, e := makeRustProcessEdge(opts, basePath, sourceHash, cmdName)
					*nodes = append(*nodes, n)
					*edges = append(*edges, e)
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForRustProcessExec(node.Child(i), opts, basePath, sourceHash, seen, nodes, edges)
	}
}

// isCommandNewCall returns true if the scoped identifier text matches Command::new patterns.
func isCommandNewCall(text string) bool {
	switch text {
	case "Command::new", "std::process::Command::new":
		return true
	}
	return false
}

// extractFirstRustStringArgOrDynamic extracts the value of the first string_literal argument
// to a call_expression. Returns "dynamic" if not a string literal.
func extractFirstRustStringArgOrDynamic(callNode *sitter.Node, content []byte) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return "dynamic"
	}

	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
		// First real argument found.
		if child.Type() == "string_literal" {
			raw := child.Content(content)
			if len(raw) >= 2 {
				return raw[1 : len(raw)-1]
			}
		}
		return "dynamic"
	}
	return "dynamic"
}

// makeRustProcessEdge creates a target process node and an executes_process edge to it.
func makeRustProcessEdge(opts types.ExtractOptions, basePath string, sourceHash types.Hash, cmdName string) (types.Node, types.Edge) {
	qn := "process://" + cmdName
	targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, qn, types.KindProcess)

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.ExecutesProcess, provenance)

	node := types.Node{
		NodeHash:      targetHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          types.KindProcess,
	}

	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.ExecutesProcess,
		Confidence: 0.9,
		Provenance: provenance,
	}

	return node, edge
}
