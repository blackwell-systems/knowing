package tsresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ProcessStatement processes a TypeScript statement node to bind variables
// into the current scope. Handles variable_declarator (const/let/var with
// type annotations or initializer inference), for_in_statement, for_of_statement,
// destructuring patterns (object and array), catch_clause bindings,
// lexical_declaration, variable_declaration, and assignment_expression.
//
// This is the TS port of ts_process_statement from the C reference
// implementation (lines 1694-1927).
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "variable_declarator":
		processVariableDeclarator(ctx, node)

	case "lexical_declaration", "variable_declaration":
		// Walk children for variable_declarator nodes.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "variable_declarator" {
				processVariableDeclarator(ctx, child)
			}
		}

	case "for_in_statement":
		processForIn(ctx, node)

	case "for_of_statement":
		processForOf(ctx, node)

	case "catch_clause":
		processCatchClause(ctx, node)

	case "assignment_expression":
		processAssignment(ctx, node)
	}
}

// processVariableDeclarator handles a variable_declarator node
// (e.g. const x: number = 42, const {a, b} = obj, const [a, b] = arr).
func processVariableDeclarator(ctx *ResolveContext, node *sitter.Node) {
	nameNode := node.ChildByFieldName("name")
	valueNode := node.ChildByFieldName("value")

	// Determine the type: annotation wins over RHS inference.
	var varType *typresolve.Type

	// Check for type annotation.
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		varType = ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Imports)
	} else if valueNode != nil {
		varType = EvalExprType(ctx, valueNode)
	}

	if varType == nil {
		varType = typresolve.Unknown()
	}

	if nameNode == nil {
		return
	}

	switch nameNode.Type() {
	case "identifier":
		name := nodeText(nameNode, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, varType)
		}

	case "object_pattern":
		bindObjectPattern(ctx, nameNode, varType)

	case "array_pattern":
		bindArrayPattern(ctx, nameNode, varType)
	}
}

// bindObjectPattern binds variables from an object destructuring pattern.
// For each property in the pattern, if the RHS type has known fields,
// bind to the field type. Otherwise bind to Unknown.
func bindObjectPattern(ctx *ResolveContext, pattern *sitter.Node, rhsType *typresolve.Type) {
	for i := 0; i < int(pattern.NamedChildCount()); i++ {
		child := pattern.NamedChild(int(i))
		if child == nil {
			continue
		}

		var bindName string

		switch child.Type() {
		case "shorthand_property_identifier_pattern":
			// const { a } = obj -> a is both property name and variable name.
			bindName = nodeText(child, ctx.Content)

		case "pair_pattern":
			// const { a: b } = obj -> a is property, b is variable.
			valueChild := child.ChildByFieldName("value")
			if valueChild != nil && valueChild.Type() == "identifier" {
				bindName = nodeText(valueChild, ctx.Content)
			}

		case "identifier":
			bindName = nodeText(child, ctx.Content)

		case "rest_pattern":
			// const { ...rest } = obj
			for j := 0; j < int(child.NamedChildCount()); j++ {
				inner := child.NamedChild(int(j))
				if inner != nil && inner.Type() == "identifier" {
					name := nodeText(inner, ctx.Content)
					if name != "_" {
						ctx.Scope.Bind(name, typresolve.Unknown())
					}
				}
			}
			continue
		}

		if bindName != "" && bindName != "_" {
			// Try to find field type from RHS type.
			fieldType := typresolve.Unknown()
			if rhsType != nil && rhsType.Kind == typresolve.KindNamed {
				if ft := LookupField(ctx.Registry, rhsType.Name, bindName); ft != nil {
					fieldType = ft
				}
			}
			ctx.Scope.Bind(bindName, fieldType)
		}
	}
}

// bindArrayPattern binds variables from an array destructuring pattern.
// If RHS is Tuple, bind each var to the corresponding element.
// If RHS is Slice, bind each var to the element type.
// Otherwise bind to Unknown.
func bindArrayPattern(ctx *ResolveContext, pattern *sitter.Node, rhsType *typresolve.Type) {
	for i := 0; i < int(pattern.NamedChildCount()); i++ {
		child := pattern.NamedChild(int(i))
		if child == nil {
			continue
		}

		var bindName string
		if child.Type() == "identifier" {
			bindName = nodeText(child, ctx.Content)
		} else if child.Type() == "rest_pattern" {
			// const [a, ...rest] = arr
			for j := 0; j < int(child.NamedChildCount()); j++ {
				inner := child.NamedChild(int(j))
				if inner != nil && inner.Type() == "identifier" {
					name := nodeText(inner, ctx.Content)
					if name != "_" {
						// Rest of an array is still the same array type.
						ctx.Scope.Bind(name, rhsType)
					}
				}
			}
			continue
		}

		if bindName == "" || bindName == "_" {
			continue
		}

		var elemType *typresolve.Type
		if rhsType != nil {
			switch rhsType.Kind {
			case typresolve.KindTuple:
				if i < len(rhsType.Elements) {
					elemType = rhsType.Elements[i]
				}
			case typresolve.KindSlice, typresolve.KindArray:
				if rhsType.Elem != nil {
					elemType = rhsType.Elem
				}
			}
		}

		if elemType == nil {
			elemType = typresolve.Unknown()
		}
		ctx.Scope.Bind(bindName, elemType)
	}
}

// processForIn handles for...in statements.
// for (const k in obj) -> k is Builtin("string")
func processForIn(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	if left == nil {
		return
	}

	// Bind left variable to string (for...in yields keys as strings).
	bindForLoopVar(ctx, left, typresolve.Builtin("string"))
}

// processForOf handles for...of statements.
// for (const x of items) -> x is element type of items
func processForOf(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil {
		return
	}

	elemType := typresolve.Unknown()
	if right != nil {
		iterableType := EvalExprType(ctx, right)
		if iterableType != nil {
			switch iterableType.Kind {
			case typresolve.KindSlice, typresolve.KindArray:
				if iterableType.Elem != nil {
					elemType = iterableType.Elem
				}
			}
		}
	}

	bindForLoopVar(ctx, left, elemType)
}

// bindForLoopVar binds the variable(s) in a for loop's left-hand side.
func bindForLoopVar(ctx *ResolveContext, left *sitter.Node, varType *typresolve.Type) {
	switch left.Type() {
	case "identifier":
		name := nodeText(left, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, varType)
		}
	case "lexical_declaration", "variable_declaration":
		// const x of ..., let x in ...
		for i := 0; i < int(left.NamedChildCount()); i++ {
			child := left.NamedChild(int(i))
			if child != nil && child.Type() == "variable_declarator" {
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil && nameNode.Type() == "identifier" {
					name := nodeText(nameNode, ctx.Content)
					if name != "_" {
						ctx.Scope.Bind(name, varType)
					}
				}
			}
		}
	}
}

// processCatchClause handles catch clause bindings.
// catch (e) -> e is Unknown (TS catch parameter is unknown by default).
func processCatchClause(ctx *ResolveContext, node *sitter.Node) {
	// The catch parameter is in the "parameter" field.
	param := node.ChildByFieldName("parameter")
	if param == nil {
		return
	}

	if param.Type() == "identifier" {
		name := nodeText(param, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, typresolve.Unknown())
		}
	}
}

// processAssignment handles assignment expressions for existing variables.
func processAssignment(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return
	}

	// Only rebind if left is an identifier already in scope.
	if left.Type() == "identifier" {
		name := nodeText(left, ctx.Content)
		if ctx.Scope.Lookup(name) != nil {
			rhsType := EvalExprType(ctx, right)
			if rhsType != nil {
				ctx.Scope.Bind(name, rhsType)
			}
		}
	}
}
