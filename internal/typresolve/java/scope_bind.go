package javaresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ProcessStatement processes a Java statement node to bind variables into
// the current scope. Handles local_variable_declaration, enhanced_for_statement,
// for_statement, try_with_resources_statement, catch_clause, and
// expression_statement (assignments).
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "local_variable_declaration":
		processLocalVarDecl(ctx, node)
	case "enhanced_for_statement":
		processEnhancedFor(ctx, node)
	case "for_statement":
		processForStatement(ctx, node)
	case "try_with_resources_statement":
		processTryWithResources(ctx, node)
	case "catch_clause":
		processCatchClause(ctx, node)
	case "expression_statement":
		processExpressionStatement(ctx, node)
	}
}

// processLocalVarDecl handles local variable declarations.
// e.g., int x = 42; var x = obj.method(); String a, b;
func processLocalVarDecl(ctx *ResolveContext, node *sitter.Node) {
	// The type is the first named child (type node).
	// variable_declarator children follow.
	typeNode := node.ChildByFieldName("type")

	var declType *typresolve.Type
	isVar := false

	if typeNode != nil {
		typeText := nodeContent(typeNode, ctx.Content)
		if typeText == "var" {
			isVar = true
		} else {
			declType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
		}
	}

	// Walk variable_declarator children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil || child.Type() != "variable_declarator" {
			continue
		}

		nameNode := child.ChildByFieldName("name")
		valueNode := child.ChildByFieldName("value")

		if nameNode == nil {
			continue
		}
		name := nodeContent(nameNode, ctx.Content)

		varType := declType
		if isVar {
			// Java 10+ var: infer from initializer.
			if valueNode != nil {
				varType = EvalExprType(ctx, valueNode)
			} else {
				varType = typresolve.Unknown()
			}
		}

		if varType == nil {
			if valueNode != nil {
				varType = EvalExprType(ctx, valueNode)
			} else {
				varType = typresolve.Unknown()
			}
		}

		ctx.Scope.Bind(name, varType)
	}
}

// processEnhancedFor handles enhanced for loops.
// e.g., for (String s : list) { ... }
func processEnhancedFor(ctx *ResolveContext, node *sitter.Node) {
	typeNode := node.ChildByFieldName("type")
	nameNode := node.ChildByFieldName("name")

	if typeNode == nil || nameNode == nil {
		return
	}

	varType := ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	name := nodeContent(nameNode, ctx.Content)
	ctx.Scope.Bind(name, varType)
}

// processForStatement handles traditional for loops.
// e.g., for (int i = 0; i < n; i++) { ... }
func processForStatement(ctx *ResolveContext, node *sitter.Node) {
	initNode := node.ChildByFieldName("init")
	if initNode == nil {
		return
	}
	// The init may be a local_variable_declaration.
	if initNode.Type() == "local_variable_declaration" {
		processLocalVarDecl(ctx, initNode)
	}
}

// processTryWithResources handles try-with-resources statements.
// e.g., try (BufferedReader br = new BufferedReader(...)) { ... }
func processTryWithResources(ctx *ResolveContext, node *sitter.Node) {
	resourcesNode := node.ChildByFieldName("resources")
	if resourcesNode == nil {
		return
	}

	// Walk resource declarations.
	for i := 0; i < int(resourcesNode.NamedChildCount()); i++ {
		child := resourcesNode.NamedChild(int(i))
		if child == nil {
			continue
		}
		// Each resource is a resource node containing type, name, value.
		processResource(ctx, child)
	}
}

// processResource handles a single resource in try-with-resources.
func processResource(ctx *ResolveContext, node *sitter.Node) {
	typeNode := node.ChildByFieldName("type")
	nameNode := node.ChildByFieldName("name")

	if nameNode == nil {
		return
	}

	name := nodeContent(nameNode, ctx.Content)

	var resType *typresolve.Type
	if typeNode != nil {
		resType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	} else {
		resType = typresolve.Unknown()
	}

	ctx.Scope.Bind(name, resType)
}

// processCatchClause handles catch clauses.
// e.g., catch (IOException e) { ... }
func processCatchClause(ctx *ResolveContext, node *sitter.Node) {
	// Find the catch_formal_parameter child.
	var paramNode *sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "catch_formal_parameter" {
			paramNode = child
			break
		}
	}

	if paramNode == nil {
		return
	}

	// Get the type and name from the catch parameter.
	nameNode := paramNode.ChildByFieldName("name")

	// The type might be a catch_type or a regular type node.
	var catchType *typresolve.Type
	for i := 0; i < int(paramNode.NamedChildCount()); i++ {
		child := paramNode.NamedChild(int(i))
		if child == nil {
			continue
		}
		switch child.Type() {
		case "catch_type":
			// Multi-catch: catch (IOException | SQLException e)
			// Use the first type.
			if child.NamedChildCount() > 0 {
				firstType := child.NamedChild(0)
				if firstType != nil {
					catchType = ParseTypeNode(firstType, ctx.Content, ctx.PkgQN, ctx.Imports)
				}
			}
		case "identifier":
			// Skip: this is the name, not the type.
		default:
			if catchType == nil {
				catchType = ParseTypeNode(child, ctx.Content, ctx.PkgQN, ctx.Imports)
			}
		}
	}

	if catchType == nil {
		catchType = typresolve.Unknown()
	}

	if nameNode != nil {
		name := nodeContent(nameNode, ctx.Content)
		ctx.Scope.Bind(name, catchType)
	}
}

// processExpressionStatement handles expression statements that contain
// assignments.
func processExpressionStatement(ctx *ResolveContext, node *sitter.Node) {
	// Walk children looking for assignment_expression.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "assignment_expression" {
			processAssignment(ctx, child)
		}
	}
}

// processAssignment handles assignment_expression nodes.
// e.g., x = someMethod()
func processAssignment(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")

	if left == nil || right == nil {
		return
	}

	if left.Type() == "identifier" {
		rhsType := EvalExprType(ctx, right)
		if rhsType == nil {
			rhsType = typresolve.Unknown()
		}
		name := nodeContent(left, ctx.Content)
		ctx.Scope.Bind(name, rhsType)
	}
}
