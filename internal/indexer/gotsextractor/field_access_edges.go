package gotsextractor

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractFieldAccessEdges walks a method body and creates accesses_field edges
// for each struct field the method reads or writes via its receiver.
//
// For a method like:
//
//	func (s *Server) Start() { port := s.Port; s.running = true }
//
// This creates accesses_field edges from the method node to field nodes:
//   - Start -> Server.Port
//   - Start -> Server.running
//
// Only receiver-based field access is extracted (high confidence because we know
// the receiver type from the method signature). Field access on other variables
// is skipped because without type resolution we can't determine the target type.
func extractFieldAccessEdges(methodNode *sitter.Node, opts types.ExtractOptions, pkgPath string, methodHash types.Hash) []types.Edge {
	// Get receiver info.
	receiver := methodNode.ChildByFieldName("receiver")
	if receiver == nil {
		return nil
	}

	receiverVar, receiverType := extractReceiverInfo(receiver, opts.Content)
	if receiverVar == "" || receiverType == "" || receiverType == "Unknown" {
		return nil
	}

	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	// Walk the body collecting unique field names accessed on the receiver.
	fields := make(map[string]bool)
	walkForFieldAccess(body, opts.Content, receiverVar, fields)

	if len(fields) == 0 {
		return nil
	}

	// Create edges from method -> field node (TypeName.FieldName).
	// The scoped name matches extractStructFields: "TypeName.fieldName"
	var edges []types.Edge
	for fieldName := range fields {
		if isCommonFieldName(fieldName) {
			continue
		}
		scopedName := receiverType + "." + fieldName
		targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, scopedName, types.KindField)
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

// extractReceiverInfo returns the receiver variable name and type name from a
// method receiver parameter list.
//
// For `func (s *Server) Method()`, returns ("s", "Server").
// For `func (e Enricher) Method()`, returns ("e", "Enricher").
func extractReceiverInfo(receiver *sitter.Node, content []byte) (varName, typeName string) {
	for i := 0; i < int(receiver.ChildCount()); i++ {
		child := receiver.Child(i)
		if child.Type() != "parameter_declaration" {
			continue
		}
		// The variable name is the first identifier child (before the type).
		nameNode := child.ChildByFieldName("name")
		if nameNode != nil {
			varName = nameNode.Content(content)
		}
		typeNode := child.ChildByFieldName("type")
		typeName = extractTypeName(typeNode, content)
		return
	}
	return "", ""
}

// walkForFieldAccess recursively walks an AST subtree collecting field names
// accessed on the receiver variable. A field access is a selector_expression
// where the operand is the receiver variable AND the selector_expression is NOT
// the function child of a call_expression (which would be a method call).
func walkForFieldAccess(node *sitter.Node, content []byte, receiverVar string, fields map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "selector_expression" {
		operand := node.ChildByFieldName("operand")
		field := node.ChildByFieldName("field")
		if operand != nil && field != nil && operand.Type() == "identifier" {
			if operand.Content(content) == receiverVar {
				fieldName := field.Content(content)
				// Check if this selector_expression is the function of a call.
				// If so, it's a method call (e.g., s.Start()), not a field access.
				if !isFunctionOfCall(node) {
					// Skip if the field name starts with uppercase and could be a
					// nested type access (e.g., s.Logger.Printf) - only count the
					// direct field, not chains.
					if fieldName != "" && !fields[fieldName] {
						fields[fieldName] = true
					}
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForFieldAccess(node.Child(i), content, receiverVar, fields)
	}
}

// isFunctionOfCall checks whether a selector_expression node is the "function"
// child of a call_expression parent. If so, it represents a method call rather
// than a field access.
func isFunctionOfCall(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Type() != "call_expression" {
		return false
	}
	funcChild := parent.ChildByFieldName("function")
	return funcChild != nil && funcChild == node
}

// isCommonFieldName returns true for field names that are too generic to be
// useful as retrieval signals (used everywhere, like "err", "ctx", "mu").
func isCommonFieldName(name string) bool {
	switch strings.ToLower(name) {
	case "mu", "mutex", "rwmutex", "lock", "wg", "once", "ctx", "err", "logger", "log":
		return true
	}
	return false
}
