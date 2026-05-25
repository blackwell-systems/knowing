package rustextractor

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractFieldAccessEdges walks a Rust method body (inside an impl block) and
// creates accesses_field edges for each struct field the method reads or writes
// via self.
//
// For a method like:
//
//	impl Server {
//	    fn start(&self) { let p = self.port; self.running = true; }
//	}
//
// This creates accesses_field edges from the method node to field nodes:
//   - start -> Server.port
//   - start -> Server.running
//
// Only self-based field access is extracted (high confidence because we know
// the type from the impl block). Field access on other variables is skipped
// because without type resolution we cannot determine the target type.
func extractFieldAccessEdges(methodNode *sitter.Node, opts types.ExtractOptions, basePath string, implType string, methodHash types.Hash) []types.Edge {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	// Walk the body collecting unique field names accessed on self.
	fields := make(map[string]bool)
	walkForSelfFieldAccess(body, opts.Content, fields)

	if len(fields) == 0 {
		return nil
	}

	// Create edges from method -> field node (TypeName.fieldName).
	var edges []types.Edge
	for fieldName := range fields {
		if isCommonFieldName(fieldName) {
			continue
		}
		scopedName := implType + "." + fieldName
		targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, scopedName, types.KindField)
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

// walkForSelfFieldAccess recursively walks an AST subtree collecting field names
// accessed on self. A field access is a field_expression where the value child
// is a "self" node AND the field_expression is NOT the function child of
// a call_expression (which would be a method call like self.start()).
func walkForSelfFieldAccess(node *sitter.Node, content []byte, fields map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "field_expression" {
		valueNode := node.ChildByFieldName("value")
		fieldNode := node.ChildByFieldName("field")
		// In Rust tree-sitter grammar, `self` has node type "self", not "identifier".
		if valueNode != nil && fieldNode != nil && valueNode.Type() == "self" {
			fieldName := fieldNode.Content(content)
			// Check if this field_expression is the function of a call_expression.
			// If so, it's a method call (e.g., self.start()), not a field access.
			if !isFunctionOfCallExpression(node) {
				if fieldName != "" && !fields[fieldName] {
					fields[fieldName] = true
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForSelfFieldAccess(node.Child(i), content, fields)
	}
}

// isFunctionOfCallExpression checks whether a field_expression node is the
// "function" child of a call_expression parent. If so, it represents a method
// call rather than a field access.
func isFunctionOfCallExpression(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Type() != "call_expression" {
		return false
	}
	funcChild := parent.ChildByFieldName("function")
	return funcChild != nil && funcChild == node
}

// extractStructFieldNodes walks struct_item children for field declarations and
// creates field nodes with kind="field".
//
// For a struct like:
//
//	struct Server {
//	    port: u16,
//	    running: bool,
//	}
//
// This creates field nodes: Server.port, Server.running
func extractStructFieldNodes(node *sitter.Node, opts types.ExtractOptions, basePath string) []types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	typeName := nameNode.Content(opts.Content)

	// Find the field_declaration_list child.
	var fieldList *sitter.Node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "field_declaration_list" {
			fieldList = child
			break
		}
	}
	if fieldList == nil {
		return nil
	}

	var nodes []types.Node
	for i := 0; i < int(fieldList.ChildCount()); i++ {
		child := fieldList.Child(i)
		if child.Type() != "field_declaration" {
			continue
		}
		// In Rust tree-sitter grammar, field_declaration has a "name" field
		// (field_identifier) and a "type" field.
		fieldNameNode := child.ChildByFieldName("name")
		if fieldNameNode == nil {
			continue
		}
		fieldName := fieldNameNode.Content(opts.Content)
		if fieldName == "" {
			continue
		}
		fieldLine := int(fieldNameNode.StartPoint().Row) + 1
		scopedName := typeName + "." + fieldName
		qn := fmt.Sprintf("%s://%s/%s.%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, typeName, fieldName)
		fieldHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, scopedName, types.KindField)
		nodes = append(nodes, types.Node{
			NodeHash:      fieldHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          types.KindField,
			Line:          fieldLine,
			Signature:     fmt.Sprintf("%s: %s", fieldName, fieldTypeString(child, opts.Content)),
		})
	}
	return nodes
}

// fieldTypeString extracts the type annotation string from a field_declaration node.
func fieldTypeString(fieldDecl *sitter.Node, content []byte) string {
	typeNode := fieldDecl.ChildByFieldName("type")
	if typeNode == nil {
		return ""
	}
	return typeNode.Content(content)
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
