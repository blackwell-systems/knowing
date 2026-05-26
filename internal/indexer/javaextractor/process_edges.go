package javaextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractProcessExecEdges walks a Java method body and creates executes_process
// edges for each Runtime.getRuntime().exec() or new ProcessBuilder() call.
//
// For a method like:
//
//	public void deploy() {
//	    Runtime.getRuntime().exec("docker");
//	    new ProcessBuilder("kubectl").start();
//	}
//
// This creates executes_process edges from the method node to process nodes:
//   - deploy -> process://docker
//   - deploy -> process://kubectl
//
// If the command argument is not a string literal, uses QN="process://dynamic".
func extractProcessExecEdges(methodNode *sitter.Node, opts types.ExtractOptions, pkgPath string, className string, methodHash types.Hash) ([]types.Node, []types.Edge) {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var nodes []types.Node
	var edges []types.Edge

	walkForJavaProcessExec(body, opts, pkgPath, methodHash, seen, &nodes, &edges)
	return nodes, edges
}

// walkForJavaProcessExec recursively walks the AST looking for Runtime.exec()
// and ProcessBuilder construction patterns.
func walkForJavaProcessExec(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, seen map[string]bool, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	// Pattern 1: Runtime.getRuntime().exec("cmd")
	// This is a method_invocation where name is "exec" and object is itself
	// a method_invocation (Runtime.getRuntime()).
	if node.Type() == "method_invocation" {
		name := node.ChildByFieldName("name")
		if name != nil && name.Content(opts.Content) == "exec" {
			object := node.ChildByFieldName("object")
			if object != nil && isRuntimeGetRuntime(object, opts.Content) {
				cmdName := extractFirstJavaStringArgForProcess(node, opts.Content)
				if !seen[cmdName] {
					seen[cmdName] = true
					n, e := makeJavaProcessEdge(opts, pkgPath, sourceHash, cmdName)
					*nodes = append(*nodes, n)
					*edges = append(*edges, e)
				}
			}
		}
	}

	// Pattern 2: new ProcessBuilder("cmd")
	// This is an object_creation_expression where type is "ProcessBuilder".
	if node.Type() == "object_creation_expression" {
		typeNode := node.ChildByFieldName("type")
		if typeNode != nil && typeNode.Content(opts.Content) == "ProcessBuilder" {
			cmdName := extractFirstJavaStringArgFromCreation(node, opts.Content)
			if !seen[cmdName] {
				seen[cmdName] = true
				n, e := makeJavaProcessEdge(opts, pkgPath, sourceHash, cmdName)
				*nodes = append(*nodes, n)
				*edges = append(*edges, e)
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForJavaProcessExec(node.Child(i), opts, pkgPath, sourceHash, seen, nodes, edges)
	}
}

// isRuntimeGetRuntime checks if a node represents Runtime.getRuntime() call.
func isRuntimeGetRuntime(node *sitter.Node, content []byte) bool {
	if node.Type() != "method_invocation" {
		return false
	}
	object := node.ChildByFieldName("object")
	name := node.ChildByFieldName("name")
	if object == nil || name == nil {
		return false
	}
	return object.Content(content) == "Runtime" && name.Content(content) == "getRuntime"
}

// extractFirstJavaStringArgForProcess extracts the first string argument from a
// method_invocation's arguments list. Returns "dynamic" if not a string literal.
func extractFirstJavaStringArgForProcess(callNode *sitter.Node, content []byte) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return "dynamic"
	}

	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
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

// extractFirstJavaStringArgFromCreation extracts the first string argument from
// an object_creation_expression's arguments list. Returns "dynamic" if not a string literal.
func extractFirstJavaStringArgFromCreation(creationNode *sitter.Node, content []byte) string {
	args := creationNode.ChildByFieldName("arguments")
	if args == nil {
		return "dynamic"
	}

	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
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

// makeJavaProcessEdge creates a target process node and an executes_process edge to it.
func makeJavaProcessEdge(opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, cmdName string) (types.Node, types.Edge) {
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
