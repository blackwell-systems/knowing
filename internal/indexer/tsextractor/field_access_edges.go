package tsextractor

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractFieldAccessEdges walks a TypeScript/JavaScript method body and creates
// accesses_field edges for each field the method reads or writes via `this`.
//
// For a method like:
//
//	class Server {
//	  start() { const p = this.port; this.running = true; }
//	}
//
// This creates accesses_field edges from the method node to field nodes:
//   - start -> Server.port
//   - start -> Server.running
//
// Method calls (this.stop()) are excluded because they are not field accesses.
func extractFieldAccessEdges(methodBody *sitter.Node, opts types.ExtractOptions, qnamePrefix, className string, methodHash types.Hash) []types.Edge {
	if methodBody == nil || className == "" {
		return nil
	}

	// Walk the body collecting unique field names accessed via this.
	fields := make(map[string]bool)
	walkForThisFieldAccess(methodBody, opts.Content, fields)

	if len(fields) == 0 {
		return nil
	}

	// Create edges from method -> field node (ClassName.fieldName).
	var edges []types.Edge
	for fieldName := range fields {
		if isCommonFieldName(fieldName) {
			continue
		}
		scopedName := className + "." + fieldName
		targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, scopedName, types.KindField)
		provenance := "ast_inferred"
		edgeHash := types.ComputeEdgeHash(methodHash, targetHash, edgetype.AccessesField, provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: methodHash,
			TargetHash: targetHash,
			EdgeType:   edgetype.AccessesField,
			Confidence: 0.8,
			Provenance: provenance,
		})
	}
	return edges
}

// walkForThisFieldAccess recursively walks an AST subtree collecting field names
// accessed via `this.fieldName`. A field access is a member_expression where the
// object is a `this` node AND the member_expression is NOT the `function` child
// of a call_expression (which would be a method call like this.start()).
func walkForThisFieldAccess(node *sitter.Node, content []byte, fields map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "member_expression" {
		obj := node.ChildByFieldName("object")
		prop := node.ChildByFieldName("property")
		if obj != nil && prop != nil && obj.Type() == "this" {
			fieldName := prop.Content(content)
			// Check if this member_expression is the function of a call_expression.
			// If so, it's a method call (e.g., this.start()), not a field access.
			if !isTSFunctionOfCall(node) && fieldName != "" {
				fields[fieldName] = true
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForThisFieldAccess(node.Child(i), content, fields)
	}
}

// isTSFunctionOfCall checks whether a member_expression node is the "function"
// child of a call_expression parent. If so, it represents a method call rather
// than a field access.
func isTSFunctionOfCall(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Type() != "call_expression" {
		return false
	}
	funcChild := parent.ChildByFieldName("function")
	return funcChild != nil && funcChild == node
}

// extractClassFieldNodes walks a class body and creates field nodes for class
// field declarations (public_field_definition, field_definition, property_signature).
//
// For a class like:
//
//	class Server {
//	  port: number;
//	  config: Config;
//	}
//
// This creates field nodes: Server.port, Server.config.
func extractClassFieldNodes(classBody *sitter.Node, opts types.ExtractOptions, qnamePrefix, className string) []types.Node {
	if classBody == nil || className == "" {
		return nil
	}

	var nodes []types.Node
	seen := make(map[string]bool)

	for i := 0; i < int(classBody.ChildCount()); i++ {
		child := classBody.Child(i)
		if child == nil {
			continue
		}

		var fieldName string
		switch child.Type() {
		case "public_field_definition", "field_definition", "property_signature":
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				fieldName = nameNode.Content(opts.Content)
			}
		default:
			continue
		}

		if fieldName == "" || seen[fieldName] {
			continue
		}
		seen[fieldName] = true

		scopedName := className + "." + fieldName
		qn := fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, qnamePrefix, className, fieldName)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, scopedName, types.KindField)
		line := int(child.StartPoint().Row) + 1

		nodes = append(nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          types.KindField,
			Line:          line,
			Signature:     fmt.Sprintf("%s.%s", className, fieldName),
		})
	}
	return nodes
}

// isCommonFieldName returns true for field names that are too generic to be
// useful as retrieval signals (used everywhere, like "err", "ctx", "mu").
func isCommonFieldName(name string) bool {
	switch strings.ToLower(name) {
	case "mu", "mutex", "lock", "wg", "once", "ctx", "err", "logger", "log":
		return true
	}
	return false
}
