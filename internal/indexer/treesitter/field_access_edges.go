package treesitter

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractFieldAccessEdges walks a Python method body and creates accesses_field
// edges for each instance field the method reads or writes via self.
//
// For a method like:
//
//	def start(self):
//	    self.running = True
//	    port = self.port
//
// This creates accesses_field edges from the method node to field nodes:
//   - start -> ClassName.running
//   - start -> ClassName.port
//
// Only self-based field access is extracted. Method calls (self.method()) are
// excluded because those are call edges, not field access.
func extractFieldAccessEdges(methodNode *sitter.Node, opts types.ExtractOptions, classContext string, methodHash types.Hash) []types.Edge {
	body := methodNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	// Walk the body collecting unique field names accessed via self.
	fields := make(map[string]bool)
	walkForPythonFieldAccess(body, opts.Content, fields)

	if len(fields) == 0 {
		return nil
	}

	// Create edges from method -> field node (ClassName.fieldName).
	var edges []types.Edge
	for fieldName := range fields {
		if isCommonFieldName(fieldName) {
			continue
		}
		scopedName := classContext + "." + fieldName
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, scopedName, types.KindField)
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

// walkForPythonFieldAccess recursively walks a Python AST subtree collecting
// field names accessed via self. A field access is an `attribute` node where
// the object is an identifier "self" AND the attribute node is NOT the function
// child of a `call` node (which would be a method call like self.start()).
func walkForPythonFieldAccess(node *sitter.Node, content []byte, fields map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "attribute" {
		objectNode := node.ChildByFieldName("object")
		attrNode := node.ChildByFieldName("attribute")
		if objectNode != nil && attrNode != nil &&
			objectNode.Type() == "identifier" &&
			objectNode.Content(content) == "self" {
			fieldName := attrNode.Content(content)
			// Check if this attribute node is the function of a call (method call).
			if !isPythonFunctionOfCall(node) && fieldName != "" {
				fields[fieldName] = true
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForPythonFieldAccess(node.Child(i), content, fields)
	}
}

// isPythonFunctionOfCall checks whether an attribute node is the "function"
// child of a call parent. If so, it represents a method call (self.method())
// rather than a field access (self.field).
func isPythonFunctionOfCall(node *sitter.Node) bool {
	parent := node.Parent()
	if parent == nil || parent.Type() != "call" {
		return false
	}
	funcChild := parent.ChildByFieldName("function")
	return funcChild != nil && funcChild == node
}

// extractClassFieldNodes walks a Python class body for field declarations and
// returns nodes with kind=field. Fields are detected from:
//   - self.field = value assignments inside __init__
//   - Class-level type annotations: field_name: type
func extractClassFieldNodes(classNode *sitter.Node, opts types.ExtractOptions, className string) []types.Node {
	body := classNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	fields := make(map[string]bool)
	var nodes []types.Node

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "function_definition":
			// Look for __init__ method and extract self.field assignments.
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil && nameNode.Content(opts.Content) == "__init__" {
				initBody := child.ChildByFieldName("body")
				if initBody != nil {
					collectInitFieldAssignments(initBody, opts.Content, fields)
				}
			}
		case "decorated_definition":
			// __init__ might be decorated (unusual but possible).
			for j := 0; j < int(child.ChildCount()); j++ {
				inner := child.Child(j)
				if inner.Type() == "function_definition" {
					nameNode := inner.ChildByFieldName("name")
					if nameNode != nil && nameNode.Content(opts.Content) == "__init__" {
						initBody := inner.ChildByFieldName("body")
						if initBody != nil {
							collectInitFieldAssignments(initBody, opts.Content, fields)
						}
					}
				}
			}
		case "expression_statement":
			// Class-level type annotations: field_name: type
			collectClassLevelAnnotation(child, opts.Content, fields)
		case "type_alias_statement":
			// Some class-level typed assignments
			collectClassLevelAnnotation(child, opts.Content, fields)
		}
	}

	// Create field nodes from collected names.
	for fieldName := range fields {
		qn := fmt.Sprintf("%s://%s/%s.%s.%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath, className, fieldName)
		scopedName := className + "." + fieldName
		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, scopedName, types.KindField)
		nodes = append(nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          types.KindField,
		})
	}

	return nodes
}

// collectInitFieldAssignments walks an __init__ method body for assignment
// statements where the left side is self.field_name.
func collectInitFieldAssignments(body *sitter.Node, content []byte, fields map[string]bool) {
	for i := 0; i < int(body.ChildCount()); i++ {
		stmt := body.Child(i)
		if stmt == nil {
			continue
		}
		// Look for expression_statement -> assignment or direct assignment.
		if stmt.Type() == "expression_statement" {
			for j := 0; j < int(stmt.ChildCount()); j++ {
				inner := stmt.Child(j)
				if inner.Type() == "assignment" {
					extractSelfAssignmentField(inner, content, fields)
				}
			}
		} else if stmt.Type() == "assignment" {
			extractSelfAssignmentField(stmt, content, fields)
		}
		// Recurse into if/else/for/try blocks inside __init__.
		switch stmt.Type() {
		case "if_statement", "for_statement", "while_statement", "try_statement", "with_statement":
			collectInitFieldAssignmentsRecursive(stmt, content, fields)
		}
	}
}

// collectInitFieldAssignmentsRecursive walks nested blocks inside __init__
// looking for self.field assignments.
func collectInitFieldAssignmentsRecursive(node *sitter.Node, content []byte, fields map[string]bool) {
	if node == nil {
		return
	}
	if node.Type() == "expression_statement" {
		for j := 0; j < int(node.ChildCount()); j++ {
			inner := node.Child(j)
			if inner.Type() == "assignment" {
				extractSelfAssignmentField(inner, content, fields)
			}
		}
		return
	}
	if node.Type() == "assignment" {
		extractSelfAssignmentField(node, content, fields)
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		collectInitFieldAssignmentsRecursive(node.Child(i), content, fields)
	}
}

// extractSelfAssignmentField checks if an assignment node assigns to self.field
// and adds the field name to the map.
func extractSelfAssignmentField(assignment *sitter.Node, content []byte, fields map[string]bool) {
	left := assignment.ChildByFieldName("left")
	if left == nil || left.Type() != "attribute" {
		return
	}
	objectNode := left.ChildByFieldName("object")
	attrNode := left.ChildByFieldName("attribute")
	if objectNode == nil || attrNode == nil {
		return
	}
	if objectNode.Type() != "identifier" || objectNode.Content(content) != "self" {
		return
	}
	fieldName := attrNode.Content(content)
	if fieldName != "" {
		fields[fieldName] = true
	}
}

// collectClassLevelAnnotation extracts field names from class-level type
// annotations like `field_name: str` or `field_name: int = default_value`.
func collectClassLevelAnnotation(stmt *sitter.Node, content []byte, fields map[string]bool) {
	// Class body type annotations appear as:
	// expression_statement -> assignment (with type annotation)
	// or expression_statement -> type (annotated name)
	for i := 0; i < int(stmt.ChildCount()); i++ {
		child := stmt.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "assignment":
			// Annotated assignment: field_name: type = value
			left := child.ChildByFieldName("left")
			if left != nil && left.Type() == "identifier" {
				name := left.Content(content)
				if name != "" && !strings.HasPrefix(name, "_") || name == "_lock" || name == "_logger" {
					// Include the field (filtering happens at edge emission).
					fields[name] = true
				}
			}
		case "type":
			// Pure annotation: field_name: type (no assignment)
			// The parent expression_statement has pattern:
			// expression_statement -> identifier : type
			// But tree-sitter represents this differently...
			// Let's check for the identifier sibling.
		}
	}
	// Handle the case where the expression_statement itself contains an identifier
	// with a type annotation. In Python tree-sitter, class-level annotations like
	// `name: str` appear as expression_statement containing a `type` node.
	// The pattern is: expression_statement has a child that's an assignment or
	// a child that's a `type` preceded by an identifier.
	// Actually in tree-sitter Python grammar, `name: str` is parsed as:
	// (expression_statement (assignment left: (identifier) type: (type) ...))
	// So we already handle it above via the assignment case.
}

// isCommonFieldName returns true for field names that are too generic to be
// useful as retrieval signals (used everywhere, like "err", "ctx", "mu").
func isCommonFieldName(name string) bool {
	switch strings.ToLower(name) {
	case "mu", "mutex", "lock", "wg", "once", "ctx", "err", "logger", "log", "_lock", "_logger":
		return true
	}
	return false
}
