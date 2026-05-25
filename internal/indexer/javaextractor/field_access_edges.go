package javaextractor

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractFieldAccessEdges walks a Java method body and creates accesses_field
// edges for each field accessed via explicit `this.fieldName` patterns.
//
// For a method like:
//
//	public void process() { int x = this.count; this.name = "updated"; }
//
// This creates accesses_field edges from the method node to field nodes:
//   - process -> ClassName.count
//   - process -> ClassName.name
//
// Only explicit this.field access is extracted (high confidence). Bare field
// access without `this.` is skipped because without full type resolution we
// cannot distinguish fields from local variables.
func extractFieldAccessEdges(methodNode *sitter.Node, opts types.ExtractOptions, pkgPath, className string, methodHash types.Hash) []types.Edge {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	// Walk the body collecting unique field names accessed via this.field.
	fields := make(map[string]bool)
	walkForJavaFieldAccess(body, opts.Content, fields)

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

// walkForJavaFieldAccess recursively walks an AST subtree collecting field names
// accessed via `this.fieldName`. In the Java tree-sitter grammar, `this.field`
// is represented as a `field_access` node with:
//   - object child: `this` keyword
//   - field child: identifier (the field name)
//
// Note: `this.method()` is a `method_invocation` node (not a field_access),
// so method calls are naturally excluded from this walk.
func walkForJavaFieldAccess(node *sitter.Node, content []byte, fields map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "field_access" {
		objectNode := node.ChildByFieldName("object")
		fieldNode := node.ChildByFieldName("field")
		if objectNode != nil && fieldNode != nil && objectNode.Type() == "this" {
			fieldName := fieldNode.Content(content)
			if fieldName != "" {
				fields[fieldName] = true
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForJavaFieldAccess(node.Child(i), content, fields)
	}
}

// extractClassFieldNodes walks a class body and creates field nodes for each
// field_declaration found. In the Java tree-sitter grammar:
//
//	class_body -> field_declaration -> variable_declarator (name: identifier)
//
// Each field gets a node with kind=KindField and a QN of:
//
//	repoURL://pkgPath.className.fieldName
func extractClassFieldNodes(classBody *sitter.Node, opts types.ExtractOptions, pkgPath, className string) []types.Node {
	if classBody == nil {
		return nil
	}

	var nodes []types.Node
	for i := 0; i < int(classBody.ChildCount()); i++ {
		child := classBody.Child(i)
		if child.Type() != "field_declaration" {
			continue
		}

		// Walk children of field_declaration for variable_declarator nodes.
		for j := 0; j < int(child.ChildCount()); j++ {
			declarator := child.Child(j)
			if declarator.Type() != "variable_declarator" {
				continue
			}
			nameNode := declarator.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			fieldName := nameNode.Content(opts.Content)
			if fieldName == "" {
				continue
			}

			scopedName := className + "." + fieldName
			nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, scopedName, types.KindField)
			line := int(nameNode.StartPoint().Row) + 1

			nodes = append(nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, scopedName),
				Kind:          types.KindField,
				Line:          line,
				Signature:     scopedName,
			})
		}
	}
	return nodes
}

// isCommonFieldName returns true for field names that are too generic to be
// useful as retrieval signals. Matches are case-insensitive.
// Filters: mu, mutex, lock, wg, once, ctx, err, logger, log, and
// Java-specific: LOG, LOGGER, serialVersionUID.
func isCommonFieldName(name string) bool {
	switch strings.ToLower(name) {
	case "mu", "mutex", "lock", "wg", "once", "ctx", "err", "logger", "log",
		"serialversionuid":
		return true
	}
	return false
}
