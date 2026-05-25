package csharpextractor

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractFieldAccessEdges walks a method body and creates accesses_field edges
// for each class field the method accesses via explicit `this.FieldName` syntax.
//
// For a method like:
//
//	public void Start() { var port = this.Port; this.running = true; }
//
// This creates accesses_field edges from the method node to field nodes:
//   - Start -> ClassName.Port
//   - Start -> ClassName.running
//
// Only explicit `this.` access is extracted (high confidence because the `this`
// keyword unambiguously refers to the enclosing class). Implicit field access
// (bare field names without `this.`) is skipped because without type resolution
// we cannot distinguish fields from locals or parameters.
func extractFieldAccessEdges(methodBody *sitter.Node, opts types.ExtractOptions, parentContext string, methodHash types.Hash) []types.Edge {
	if methodBody == nil {
		return nil
	}

	// Walk the body collecting unique field names accessed via this.Field.
	fields := make(map[string]bool)
	walkForThisFieldAccess(methodBody, opts.Content, fields)

	if len(fields) == 0 {
		return nil
	}

	// Create edges from method -> field node (ClassName.FieldName).
	var edges []types.Edge
	for fieldName := range fields {
		if isCommonFieldName(fieldName) {
			continue
		}
		scopedName := parentContext + "." + fieldName
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, scopedName, types.KindField)
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
// accessed via `this.FieldName`. A field access is a member_access_expression
// where the expression is `this_expression` (or identifier "this") AND the
// member_access_expression is NOT the function child of an invocation_expression
// (which would be a method call like `this.Start()`).
func walkForThisFieldAccess(node *sitter.Node, content []byte, fields map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "member_access_expression" {
		exprNode := node.ChildByFieldName("expression")
		nameNode := node.ChildByFieldName("name")
		if exprNode != nil && nameNode != nil {
			// Check if expression is `this`. In the C# tree-sitter grammar, the `this`
			// keyword has node type "this" (not "this_expression"). Also check for
			// "this_expression" and identifier "this" as fallbacks.
			isThis := exprNode.Type() == "this" ||
				exprNode.Type() == "this_expression" ||
				(exprNode.Type() == "identifier" && exprNode.Content(content) == "this")

			if isThis {
				fieldName := nameNode.Content(content)
				// Exclude method calls: check if this member_access_expression is the
				// function child of an invocation_expression parent.
				if fieldName != "" && !isInvocationFunction(node) {
					fields[fieldName] = true
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForThisFieldAccess(node.Child(i), content, fields)
	}
}

// isInvocationFunction checks whether a member_access_expression node is the
// "function" child of an invocation_expression parent. If so, it represents a
// method call (e.g., this.Start()) rather than a field access.
func isInvocationFunction(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Type() != "invocation_expression" {
		return false
	}
	// In C# tree-sitter grammar, the function of an invocation_expression is
	// accessed via the "function" field name or is the first child.
	funcChild := parent.ChildByFieldName("function")
	if funcChild != nil {
		return funcChild == node
	}
	// Fallback: check if this node is the first child of the invocation_expression.
	if parent.ChildCount() > 0 && parent.Child(0) == node {
		return true
	}
	return false
}

// extractClassFieldNodes walks a class_declaration for field_declaration and
// property_declaration nodes and creates field nodes for each.
//
// Tree-sitter C#: class_declaration -> declaration_list -> field_declaration
// -> variable_declaration -> variable_declarator (identifier child)
//
// Also handles property_declaration nodes (C# properties are field-like).
func extractClassFieldNodes(classNode *sitter.Node, opts types.ExtractOptions, className string) []types.Node {
	var nodes []types.Node

	// Find the declaration_list child of the class.
	var declList *sitter.Node
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() == "declaration_list" {
			declList = child
			break
		}
	}
	if declList == nil {
		return nil
	}

	for i := 0; i < int(declList.ChildCount()); i++ {
		child := declList.Child(i)
		switch child.Type() {
		case "field_declaration":
			fieldNodes := extractFieldDeclNodes(child, opts, className)
			nodes = append(nodes, fieldNodes...)
		case "property_declaration":
			propNode := extractPropertyDeclNode(child, opts, className)
			if propNode != nil {
				nodes = append(nodes, *propNode)
			}
		}
	}
	return nodes
}

// extractFieldDeclNodes extracts field nodes from a field_declaration.
// field_declaration -> variable_declaration -> variable_declarator (name: identifier)
func extractFieldDeclNodes(fieldDecl *sitter.Node, opts types.ExtractOptions, className string) []types.Node {
	var nodes []types.Node

	// Walk children for variable_declaration.
	for i := 0; i < int(fieldDecl.ChildCount()); i++ {
		child := fieldDecl.Child(i)
		if child.Type() != "variable_declaration" {
			continue
		}
		// Walk variable_declaration for variable_declarator children.
		for j := 0; j < int(child.ChildCount()); j++ {
			declarator := child.Child(j)
			if declarator.Type() != "variable_declarator" {
				continue
			}
			// The identifier is the name of the field.
			nameNode := declarator.ChildByFieldName("name")
			if nameNode == nil {
				// Fallback: look for first identifier child.
				for k := 0; k < int(declarator.ChildCount()); k++ {
					c := declarator.Child(k)
					if c.Type() == "identifier" {
						nameNode = c
						break
					}
				}
			}
			if nameNode == nil {
				continue
			}
			fieldName := nameNode.Content(opts.Content)
			if fieldName == "" {
				continue
			}

			fieldLine := int(nameNode.StartPoint().Row) + 1
			scopedName := className + "." + fieldName
			qn := qualifiedFieldName(opts, className, fieldName)
			fieldHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, scopedName, types.KindField)
			nodes = append(nodes, types.Node{
				NodeHash:      fieldHash,
				FileHash:      opts.FileHash,
				QualifiedName: qn,
				Kind:          types.KindField,
				Line:          fieldLine,
			})
		}
	}
	return nodes
}

// extractPropertyDeclNode extracts a field node from a property_declaration.
// property_declaration has a "name" field that is the property identifier.
func extractPropertyDeclNode(propDecl *sitter.Node, opts types.ExtractOptions, className string) *types.Node {
	nameNode := propDecl.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	propName := nameNode.Content(opts.Content)
	if propName == "" {
		return nil
	}

	propLine := int(nameNode.StartPoint().Row) + 1
	scopedName := className + "." + propName
	qn := qualifiedFieldName(opts, className, propName)
	fieldHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, scopedName, types.KindField)
	return &types.Node{
		NodeHash:      fieldHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          types.KindField,
		Line:          propLine,
	}
}

// qualifiedFieldName builds a fully qualified name for a field.
func qualifiedFieldName(opts types.ExtractOptions, className, fieldName string) string {
	return opts.RepoURL + "://" + opts.ModuleRoot + "/" + opts.FilePath + "." + className + "." + fieldName
}

// isCommonFieldName returns true for field names that are too generic to be
// useful as retrieval signals. Includes both general patterns (mu, ctx, err)
// and C#-specific patterns (_logger, _context, _disposed).
func isCommonFieldName(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "mu", "mutex", "lock", "wg", "once", "ctx", "err", "logger", "log",
		"_logger", "_context", "_disposed":
		return true
	}
	// Also match with underscore prefix for C# conventions.
	trimmed := strings.TrimPrefix(lower, "_")
	switch trimmed {
	case "logger", "context", "disposed":
		return true
	}
	return false
}
