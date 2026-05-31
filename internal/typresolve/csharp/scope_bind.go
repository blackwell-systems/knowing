package csresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ProcessStatement processes a C# statement node to bind variables into the
// current scope. Handles local_declaration_statement, foreach_statement,
// for_statement, using_statement, variable_declaration, and expression_statement.
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "local_declaration_statement":
		// Contains a variable_declaration child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "variable_declaration" {
				processVariableDeclaration(ctx, child)
			}
		}

	case "variable_declaration":
		processVariableDeclaration(ctx, node)

	case "foreach_statement":
		processForeach(ctx, node)

	case "for_statement":
		processFor(ctx, node)

	case "using_statement":
		processUsingStatement(ctx, node)

	case "expression_statement":
		// Process assignments for side effects (binding in pattern matching).
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "assignment_expression" {
				processAssignment(ctx, child)
			}
		}

	case "local_function_statement":
		// Bind the function name into scope with its signature type.
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nodeContent(nameNode, ctx.Content)
			if name != "" {
				// Build a basic func type (Unknown params/returns for now).
				ctx.Scope.Bind(name, typresolve.Func(nil, nil))
			}
		}

	case "is_pattern_expression":
		// Pattern matching: `expr is Type name` binds name to Type.
		processIsPattern(ctx, node)
	}
}

// processVariableDeclaration handles `Type x = expr` or `var x = expr`.
// A variable_declaration node has a type child and variable_declarator children.
func processVariableDeclaration(ctx *ResolveContext, node *sitter.Node) {
	// Get the type node (first child, typically).
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		// Try first named child as type.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() != "variable_declarator" {
				typeNode = child
				break
			}
		}
	}

	isVar := false
	var declType *typresolve.Type

	if typeNode != nil {
		text := nodeContent(typeNode, ctx.Content)
		if text == "var" || typeNode.Type() == "implicit_type" {
			isVar = true
		} else {
			declType = parseTypeStub(typeNode, ctx.Content)
		}
	}

	// Process each variable_declarator.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil || child.Type() != "variable_declarator" {
			continue
		}
		processDeclarator(ctx, child, declType, isVar)
	}
}

// processDeclarator binds a single variable_declarator.
// In the tree-sitter C# grammar, variable_declarator has structure:
//   identifier "=" expression (where identifier and expression are named children)
func processDeclarator(ctx *ResolveContext, node *sitter.Node, declType *typresolve.Type, isVar bool) {
	// Find the identifier (first named child of type "identifier").
	var nameNode *sitter.Node
	var initNode *sitter.Node

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}
		if child.Type() == "identifier" && nameNode == nil {
			nameNode = child
		} else if nameNode != nil && initNode == nil {
			// Everything after the identifier is the initializer expression
			// (or an equals_value_clause wrapping it).
			initNode = child
		}
	}

	if nameNode == nil {
		return
	}

	name := nodeContent(nameNode, ctx.Content)
	if name == "" || name == "_" {
		return
	}

	if isVar {
		// Infer type from initializer expression.
		if initNode != nil {
			var exprNode *sitter.Node
			if initNode.Type() == "equals_value_clause" {
				// The expression is inside the equals_value_clause.
				if initNode.NamedChildCount() > 0 {
					exprNode = initNode.NamedChild(0)
				}
			} else {
				exprNode = initNode
			}
			if exprNode != nil {
				inferredType := EvalExprType(ctx, exprNode)
				ctx.Scope.Bind(name, inferredType)
				return
			}
		}
		ctx.Scope.Bind(name, typresolve.Unknown())
	} else if declType != nil {
		ctx.Scope.Bind(name, declType)
	} else {
		ctx.Scope.Bind(name, typresolve.Unknown())
	}
}

// processForeach handles `foreach (Type x in collection)`.
func processForeach(ctx *ResolveContext, node *sitter.Node) {
	typeNode := node.ChildByFieldName("type")
	nameNode := node.ChildByFieldName("left")
	if nameNode == nil {
		// Try identifier field.
		nameNode = node.ChildByFieldName("identifier")
	}

	// Try to find the variable name among named children.
	var varName string
	if nameNode != nil {
		varName = nodeContent(nameNode, ctx.Content)
	} else {
		// Scan for identifier child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "identifier" {
				varName = nodeContent(child, ctx.Content)
				break
			}
		}
	}

	if varName == "" || varName == "_" {
		return
	}

	isVar := false
	if typeNode != nil {
		text := nodeContent(typeNode, ctx.Content)
		if text == "var" || typeNode.Type() == "implicit_type" {
			isVar = true
		}
	}

	if isVar || typeNode == nil {
		// Infer element type from collection.
		collNode := node.ChildByFieldName("right")
		if collNode == nil {
			collNode = node.ChildByFieldName("value")
		}
		if collNode != nil {
			collType := EvalExprType(ctx, collNode)
			if collType != nil {
				switch collType.Kind {
				case typresolve.KindSlice, typresolve.KindArray:
					if collType.Elem != nil {
						ctx.Scope.Bind(varName, collType.Elem)
						return
					}
				case typresolve.KindMap:
					// foreach over Dictionary yields KeyValuePair, approximate as Unknown
					ctx.Scope.Bind(varName, typresolve.Unknown())
					return
				}
			}
		}
		ctx.Scope.Bind(varName, typresolve.Unknown())
	} else {
		varType := parseTypeStub(typeNode, ctx.Content)
		ctx.Scope.Bind(varName, varType)
	}
}

// processFor handles for statement initializer declarations.
func processFor(ctx *ResolveContext, node *sitter.Node) {
	// For statements may have a variable_declaration in their initializer.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "variable_declaration" {
			processVariableDeclaration(ctx, child)
			break // Only process the initializer.
		}
	}
}

// processUsingStatement handles `using (var x = expr)` or `using var x = expr`.
func processUsingStatement(ctx *ResolveContext, node *sitter.Node) {
	// Look for variable_declaration child.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "variable_declaration" {
			processVariableDeclaration(ctx, child)
			return
		}
	}
}

// processAssignment handles assignment expressions for pattern binding purposes.
func processAssignment(ctx *ResolveContext, node *sitter.Node) {
	// For simple assignments, we don't typically bind new variables.
	// This is here for completeness; in C# assignments don't introduce
	// new variables (that's done by declarations).
	_ = ctx
	_ = node
}

// processIsPattern handles `expr is Type varName` pattern matching.
func processIsPattern(ctx *ResolveContext, node *sitter.Node) {
	// In C# tree-sitter grammar, an is_pattern has:
	// left: expression, right: pattern (declaration_pattern or type_pattern)
	if node.NamedChildCount() < 2 {
		return
	}

	patternNode := node.NamedChild(1)
	if patternNode == nil {
		return
	}

	if patternNode.Type() == "declaration_pattern" {
		// declaration_pattern has a type and a designation (name).
		typeNode := patternNode.ChildByFieldName("type")
		desigNode := patternNode.ChildByFieldName("designation")
		if typeNode == nil && patternNode.NamedChildCount() >= 1 {
			typeNode = patternNode.NamedChild(0)
		}
		if desigNode == nil && patternNode.NamedChildCount() >= 2 {
			desigNode = patternNode.NamedChild(1)
		}

		if typeNode != nil && desigNode != nil {
			varType := parseTypeStub(typeNode, ctx.Content)
			name := nodeContent(desigNode, ctx.Content)
			if name != "" && name != "_" {
				ctx.Scope.Bind(name, varType)
			}
		}
	}
}
