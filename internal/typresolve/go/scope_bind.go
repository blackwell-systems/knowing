package goresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ProcessStatement processes a statement node to bind variables into the
// current scope. Handles short_var_declaration, var_spec, const_spec, and
// range_clause. This is the Go port of go_process_statement from the C
// reference implementation.
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "short_var_declaration":
		processShortVarDecl(ctx, node)
	case "var_spec":
		processVarSpec(ctx, node)
	case "const_spec":
		processConstSpec(ctx, node)
	case "range_clause":
		processRangeClause(ctx, node)
	case "var_declaration":
		// Walk children to find var_spec nodes.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "var_spec" {
				processVarSpec(ctx, child)
			}
		}
	case "const_declaration":
		// Walk children to find const_spec nodes.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "const_spec" {
				processConstSpec(ctx, child)
			}
		}
	case "for_statement":
		// Check for range clause inside for statement.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "range_clause" {
				processRangeClause(ctx, child)
			}
		}
	}
}

// processShortVarDecl handles short variable declarations (e.g. x := expr).
func processShortVarDecl(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return
	}

	// Evaluate the right-hand side.
	var rhsType *typresolve.Type

	// If right is an expression_list with a single element, evaluate that element.
	if right.Type() == "expression_list" {
		if right.NamedChildCount() == 1 {
			rhsType = EvalExprType(ctx, right.NamedChild(0))
		} else {
			// Multiple RHS expressions: not handled as tuple yet.
			rhsType = EvalExprType(ctx, right.NamedChild(0))
		}
	} else {
		rhsType = EvalExprType(ctx, right)
	}

	if rhsType == nil {
		rhsType = typresolve.Unknown()
	}

	// Bind left-hand identifiers.
	bindLeftIdentifiers(ctx, left, rhsType)
}

// processVarSpec handles var declarations (e.g. var x int = expr).
func processVarSpec(ctx *ResolveContext, node *sitter.Node) {
	var varType *typresolve.Type

	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		varType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	} else {
		valueNode := node.ChildByFieldName("value")
		if valueNode != nil {
			if valueNode.Type() == "expression_list" && valueNode.NamedChildCount() > 0 {
				varType = EvalExprType(ctx, valueNode.NamedChild(0))
			} else {
				varType = EvalExprType(ctx, valueNode)
			}
		}
	}

	if varType == nil {
		varType = typresolve.Unknown()
	}

	// Bind all identifier children (the "name" field children).
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		if nameNode.Type() == "identifier" {
			name := nodeContent(nameNode, ctx.Content)
			if name != "_" {
				ctx.Scope.Bind(name, varType)
			}
		}
	}

	// Also check for multiple names in the node.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "identifier" {
			name := nodeContent(child, ctx.Content)
			if name != "_" {
				ctx.Scope.Bind(name, varType)
			}
		}
	}
}

// processConstSpec handles const declarations.
func processConstSpec(ctx *ResolveContext, node *sitter.Node) {
	var constType *typresolve.Type

	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		constType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	} else {
		valueNode := node.ChildByFieldName("value")
		if valueNode != nil {
			if valueNode.Type() == "expression_list" && valueNode.NamedChildCount() > 0 {
				constType = EvalExprType(ctx, valueNode.NamedChild(0))
			} else {
				constType = EvalExprType(ctx, valueNode)
			}
		}
	}

	if constType == nil {
		constType = typresolve.Unknown()
	}

	// Bind the name identifier.
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil && nameNode.Type() == "identifier" {
		name := nodeContent(nameNode, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, constType)
		}
	}
}

// processRangeClause handles range clauses in for statements.
func processRangeClause(ctx *ResolveContext, node *sitter.Node) {
	right := node.ChildByFieldName("right")
	if right == nil {
		return
	}

	containerType := EvalExprType(ctx, right)
	if containerType == nil {
		return
	}

	// Determine key and value types based on container type.
	var keyType, valueType *typresolve.Type

	switch containerType.Kind {
	case typresolve.KindSlice, typresolve.KindArray:
		keyType = typresolve.Builtin("int")
		if containerType.Elem != nil {
			valueType = containerType.Elem
		} else {
			valueType = typresolve.Unknown()
		}
	case typresolve.KindMap:
		if containerType.Key != nil {
			keyType = containerType.Key
		} else {
			keyType = typresolve.Unknown()
		}
		if containerType.Value != nil {
			valueType = containerType.Value
		} else {
			valueType = typresolve.Unknown()
		}
	case typresolve.KindChannel:
		valueType = typresolve.Unknown()
		if containerType.Elem != nil {
			valueType = containerType.Elem
		}
		keyType = valueType // For channels, only one value is received.
	case typresolve.KindBuiltin:
		if containerType.Name == "string" {
			keyType = typresolve.Builtin("int")
			valueType = typresolve.Builtin("rune")
		}
	default:
		keyType = typresolve.Unknown()
		valueType = typresolve.Unknown()
	}

	// Bind left-hand identifiers (first = key, second = value).
	left := node.ChildByFieldName("left")
	if left == nil {
		return
	}

	if left.Type() == "expression_list" {
		if left.NamedChildCount() > 0 {
			first := left.NamedChild(0)
			if first != nil && first.Type() == "identifier" {
				name := nodeContent(first, ctx.Content)
				if name != "_" && keyType != nil {
					ctx.Scope.Bind(name, keyType)
				}
			}
		}
		if left.NamedChildCount() > 1 {
			second := left.NamedChild(1)
			if second != nil && second.Type() == "identifier" {
				name := nodeContent(second, ctx.Content)
				if name != "_" && valueType != nil {
					ctx.Scope.Bind(name, valueType)
				}
			}
		}
	} else if left.Type() == "identifier" {
		name := nodeContent(left, ctx.Content)
		if name != "_" && keyType != nil {
			ctx.Scope.Bind(name, keyType)
		}
	}
}

// bindLeftIdentifiers binds left-hand identifiers from a short var
// declaration to the resolved right-hand type.
func bindLeftIdentifiers(ctx *ResolveContext, left *sitter.Node, rhsType *typresolve.Type) {
	if left.Type() == "expression_list" {
		count := int(left.NamedChildCount())
		for i := 0; i < count; i++ {
			child := left.NamedChild(int(i))
			if child == nil || child.Type() != "identifier" {
				continue
			}
			name := nodeContent(child, ctx.Content)
			if name == "_" {
				continue
			}

			if rhsType.Kind == typresolve.KindTuple && i < len(rhsType.Elements) {
				ctx.Scope.Bind(name, rhsType.Elements[i])
			} else if i == 0 {
				ctx.Scope.Bind(name, rhsType)
			} else {
				ctx.Scope.Bind(name, typresolve.Unknown())
			}
		}
	} else if left.Type() == "identifier" {
		name := nodeContent(left, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, rhsType)
		}
	}
}
