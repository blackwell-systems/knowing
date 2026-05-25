package tsextractor

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// childProcessMethods is the set of child_process methods that execute commands.
var childProcessMethods = map[string]bool{
	"spawn":       true,
	"exec":        true,
	"execSync":    true,
	"execFile":    true,
	"fork":        true,
	"spawnSync":   true,
	"execFileSync": true,
}

// directExecFunctions is the set of direct function names (imported from child_process)
// that execute commands.
var directExecFunctions = map[string]bool{
	"spawn":        true,
	"exec":         true,
	"execSync":     true,
	"execFile":     true,
	"fork":         true,
	"spawnSync":    true,
	"execFileSync": true,
}

// ExtractProcessExecEdges walks a function body for child_process execution patterns
// and creates 'executes_process' edges from the function to synthetic process nodes.
//
// Patterns matched:
//   - child_process.spawn("cmd", ...) (member_expression call)
//   - child_process.exec("cmd", ...)
//   - child_process.execSync("cmd", ...)
//   - execSync("cmd") (direct call to imported function)
//   - spawn("cmd", ...) (direct call)
func ExtractProcessExecEdges(body *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	if body == nil {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	seen := make(map[types.Hash]struct{})

	walkForProcessExec(body, opts, qnamePrefix, sourceHash, &nodes, &edges, seen)
	return nodes, edges
}

// walkForProcessExec recursively walks nodes looking for process execution patterns.
func walkForProcessExec(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		if cmd := matchProcessExecCall(node, opts.Content); cmd != "" {
			addProcessEdge(cmd, opts, qnamePrefix, sourceHash, nodes, edges, seen)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForProcessExec(node.Child(i), opts, qnamePrefix, sourceHash, nodes, edges, seen)
	}
}

// matchProcessExecCall checks if a call_expression is a process execution pattern.
// Returns the command literal or "dynamic" for non-literal args.
// Returns "" if not a process exec call.
func matchProcessExecCall(callNode *sitter.Node, content []byte) string {
	funcNode := callNode.ChildByFieldName("function")
	if funcNode == nil {
		return ""
	}

	switch funcNode.Type() {
	case "member_expression":
		// Pattern: child_process.spawn("cmd") or cp.exec("cmd")
		obj := funcNode.ChildByFieldName("object")
		prop := funcNode.ChildByFieldName("property")
		if obj == nil || prop == nil {
			return ""
		}

		objName := obj.Content(content)
		methodName := prop.Content(content)

		// Accept child_process, cp, or any object with a recognized method.
		if objName != "child_process" && objName != "cp" {
			return ""
		}
		if !childProcessMethods[methodName] {
			return ""
		}

		return extractFirstStringArg(callNode, content)

	case "identifier":
		// Pattern: execSync("cmd"), spawn("cmd")
		funcName := funcNode.Content(content)
		if !directExecFunctions[funcName] {
			return ""
		}

		return extractFirstStringArg(callNode, content)
	}

	return ""
}

// extractFirstStringArg extracts the first string literal argument from a call.
// Returns "dynamic" if the first arg is not a string literal.
func extractFirstStringArg(callNode *sitter.Node, content []byte) string {
	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return "dynamic"
	}

	// Find first non-punctuation child (the first argument).
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		switch child.Type() {
		case "string":
			val := child.Content(content)
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
			if val == "" {
				return "dynamic"
			}
			// Extract just the command name (first word).
			parts := strings.Fields(val)
			if len(parts) > 0 {
				return parts[0]
			}
			return "dynamic"
		case "template_string":
			// Template strings without interpolation.
			hasInterpolation := false
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(j).Type() == "template_substitution" {
					hasInterpolation = true
					break
				}
			}
			if !hasInterpolation {
				val := child.Content(content)
				val = strings.Trim(val, "`")
				if val == "" {
					return "dynamic"
				}
				parts := strings.Fields(val)
				if len(parts) > 0 {
					return parts[0]
				}
			}
			return "dynamic"
		case ",", "(", ")":
			continue
		default:
			// Non-literal argument.
			return "dynamic"
		}
	}

	return "dynamic"
}

// addProcessEdge creates a process node and executes_process edge, deduplicating by edge hash.
func addProcessEdge(cmd string, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	qn := fmt.Sprintf("process://%s", cmd)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, qn, types.KindProcess)

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, nodeHash, edgetype.ExecutesProcess, provenance)

	if _, exists := seen[edgeHash]; exists {
		return
	}
	seen[edgeHash] = struct{}{}

	processNode := types.Node{
		NodeHash:      nodeHash,
		FileHash:      types.EmptyHash,
		QualifiedName: qn,
		Kind:          types.KindProcess,
		Signature:     cmd,
	}
	*nodes = append(*nodes, processNode)

	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: nodeHash,
		EdgeType:   edgetype.ExecutesProcess,
		Confidence: 0.9,
		Provenance: provenance,
	}
	*edges = append(*edges, edge)
}
