package gotsextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractProcessExecEdges walks a function/method body and creates executes_process
// edges for each exec.Command or exec.CommandContext call.
//
// For a function like:
//
//	func RunBuild() {
//	    cmd := exec.Command("go", "build", "./...")
//	    exec.CommandContext(ctx, "docker", "run", "app")
//	}
//
// This creates executes_process edges from the function node to process nodes:
//   - RunBuild -> process://go
//   - RunBuild -> process://docker
//
// If the command argument is not a string literal, uses QN="process://dynamic".
func extractProcessExecEdges(funcNode *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var nodes []types.Node
	var edges []types.Edge

	walkForProcessExec(body, opts, pkgPath, sourceHash, seen, &nodes, &edges)
	return nodes, edges
}

// walkForProcessExec recursively walks the AST looking for exec.Command/exec.CommandContext calls.
func walkForProcessExec(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, seen map[string]bool, nodes *[]types.Node, edges *[]types.Edge) {
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
				if opName == "exec" && (fieldName == "Command" || fieldName == "CommandContext") {
					cmdName := extractCommandArg(node, opts.Content, fieldName == "CommandContext")
					if !seen[cmdName] {
						seen[cmdName] = true
						n, e := makeProcessEdge(opts, pkgPath, sourceHash, cmdName)
						*nodes = append(*nodes, n)
						*edges = append(*edges, e)
					}
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForProcessExec(node.Child(i), opts, pkgPath, sourceHash, seen, nodes, edges)
	}
}

// extractCommandArg extracts the command name from exec.Command or exec.CommandContext.
// For CommandContext, the first argument is ctx, so we skip it and take the second.
// Returns "dynamic" if the relevant argument is not a string literal.
func extractCommandArg(callNode *sitter.Node, content []byte, isContextVariant bool) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return "dynamic"
	}

	argIndex := 0
	if isContextVariant {
		argIndex = 1 // skip ctx argument
	}

	current := 0
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
		if current == argIndex {
			if child.Type() == "interpreted_string_literal" {
				raw := child.Content(content)
				if len(raw) >= 2 {
					return raw[1 : len(raw)-1]
				}
			}
			return "dynamic"
		}
		current++
	}
	return "dynamic"
}

// makeProcessEdge creates a target process node and an executes_process edge to it.
func makeProcessEdge(opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, cmdName string) (types.Node, types.Edge) {
	qn := "process://" + cmdName
	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qn, types.KindProcess)

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
