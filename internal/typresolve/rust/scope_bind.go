package rustresolve

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// ProcessStatement processes a Rust statement node to bind variables into the
// current scope. Handles let bindings, assignments, for loops, if-let,
// while-let, and match arm bindings.
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "let_declaration":
		processLetDeclaration(ctx, node)

	case "assignment_expression":
		processAssignment(ctx, node)

	case "compound_assignment_expr":
		// No new bindings created; skip.

	case "for_expression":
		processForExpression(ctx, node)

	case "if_let_expression":
		processIfLetExpression(ctx, node)

	case "while_let_expression":
		processIfLetExpression(ctx, node) // Same pattern as if-let

	case "match_arm":
		processMatchArm(ctx, node)
	}
}

func processLetDeclaration(ctx *ResolveContext, node *sitter.Node) {
	patternNode := node.ChildByFieldName("pattern")
	typeNode := node.ChildByFieldName("type")
	valueNode := node.ChildByFieldName("value")

	// Determine the type
	var resolvedType *typresolve.Type
	if typeNode != nil {
		resolvedType = ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Uses)
	} else if valueNode != nil {
		resolvedType = EvalExprType(ctx, valueNode)
	} else {
		resolvedType = typresolve.Unknown()
	}

	if patternNode != nil {
		bindPattern(ctx, patternNode, resolvedType)
	}
}

func processAssignment(ctx *ResolveContext, node *sitter.Node) {
	leftNode := node.ChildByFieldName("left")
	rightNode := node.ChildByFieldName("right")

	if leftNode == nil || rightNode == nil {
		return
	}

	if leftNode.Type() == "identifier" {
		name := nodeContent(leftNode, ctx.Content)
		t := EvalExprType(ctx, rightNode)
		ctx.Scope.Bind(name, t)
	}
	// field_expression on left side: skip (field mutation)
}

func processForExpression(ctx *ResolveContext, node *sitter.Node) {
	patternNode := node.ChildByFieldName("pattern")
	if patternNode == nil {
		return
	}

	// Bind loop variable to Unknown (element type not inferrable without
	// generics/iterator resolution)
	bindPattern(ctx, patternNode, typresolve.Unknown())
}

func processIfLetExpression(ctx *ResolveContext, node *sitter.Node) {
	patternNode := node.ChildByFieldName("pattern")
	valueNode := node.ChildByFieldName("value")

	if patternNode == nil || valueNode == nil {
		return
	}

	valueType := EvalExprType(ctx, valueNode)

	// If pattern is tuple_struct_pattern (e.g., Some(x))
	if patternNode.Type() == "tuple_struct_pattern" {
		// Get inner binding
		innerType := typresolve.Unknown()
		if valueType.Kind == typresolve.KindOptional && valueType.Elem != nil {
			innerType = valueType.Elem
		} else if valueType.Kind == typresolve.KindNamed && valueType.Elem != nil {
			innerType = valueType.Elem
		}

		// Find identifier children inside the tuple struct pattern
		for i := 0; i < int(patternNode.ChildCount()); i++ {
			child := patternNode.Child(i)
			if child.Type() == "identifier" {
				// Skip the constructor name (first identifier is pattern name like "Some")
				name := nodeContent(child, ctx.Content)
				if name != "Some" && name != "Ok" && name != "Err" && name != "None" {
					ctx.Scope.Bind(name, innerType)
				}
			}
		}
	} else {
		bindPattern(ctx, patternNode, valueType)
	}
}

func processMatchArm(ctx *ResolveContext, node *sitter.Node) {
	patternNode := node.ChildByFieldName("pattern")
	if patternNode == nil {
		return
	}

	// For match arms, bind pattern identifiers to Unknown since we don't
	// track the scrutinee type through the match expression here
	bindPattern(ctx, patternNode, typresolve.Unknown())
}

// bindPattern binds identifiers in a pattern to the given type.
func bindPattern(ctx *ResolveContext, pattern *sitter.Node, resolvedType *typresolve.Type) {
	if pattern == nil {
		return
	}

	switch pattern.Type() {
	case "identifier":
		name := nodeContent(pattern, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, resolvedType)
		}

	case "mutable_specifier":
		// let mut x = ...; the identifier is a sibling, not a child
		// This case happens when the pattern itself is the mutable_specifier node
		// which shouldn't normally occur as the top pattern.

	case "mut_pattern":
		// let mut x = ... (pattern wraps mutable_specifier + identifier)
		for i := 0; i < int(pattern.ChildCount()); i++ {
			child := pattern.Child(i)
			if child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				if name != "_" {
					ctx.Scope.Bind(name, resolvedType)
				}
				return
			}
		}

	case "tuple_pattern":
		// let (a, b) = ...
		if resolvedType.Kind == typresolve.KindTuple && len(resolvedType.Elements) > 0 {
			idx := 0
			for i := 0; i < int(pattern.ChildCount()); i++ {
				child := pattern.Child(i)
				if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
					continue
				}
				elemType := typresolve.Unknown()
				if idx < len(resolvedType.Elements) {
					elemType = resolvedType.Elements[idx]
				}
				bindPattern(ctx, child, elemType)
				idx++
			}
		} else {
			// Bind all to Unknown
			for i := 0; i < int(pattern.ChildCount()); i++ {
				child := pattern.Child(i)
				if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
					continue
				}
				bindPattern(ctx, child, typresolve.Unknown())
			}
		}

	case "struct_pattern":
		// let Foo { x, y } = ...
		for i := 0; i < int(pattern.ChildCount()); i++ {
			child := pattern.Child(i)
			if child.Type() == "field_pattern" || child.Type() == "identifier" {
				if child.Type() == "identifier" {
					name := nodeContent(child, ctx.Content)
					if name != "_" {
						ctx.Scope.Bind(name, typresolve.Unknown())
					}
				} else {
					// field_pattern: find the identifier
					for j := 0; j < int(child.ChildCount()); j++ {
						fc := child.Child(j)
						if fc.Type() == "identifier" {
							name := nodeContent(fc, ctx.Content)
							if name != "_" {
								ctx.Scope.Bind(name, typresolve.Unknown())
							}
						}
					}
				}
			}
		}

	case "ref_pattern":
		// let ref x = ... -> bind x to Ref(valueType)
		for i := 0; i < int(pattern.ChildCount()); i++ {
			child := pattern.Child(i)
			if child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				if name != "_" {
					ctx.Scope.Bind(name, typresolve.Ref(resolvedType))
				}
				return
			}
		}

	case "_":
		// Wildcard: skip

	case "tuple_struct_pattern":
		// Destructuring like Some(x), Ok(val)
		for i := 0; i < int(pattern.ChildCount()); i++ {
			child := pattern.Child(i)
			if child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				// Skip constructor names
				if name != "Some" && name != "Ok" && name != "Err" && name != "None" && name != "_" {
					ctx.Scope.Bind(name, typresolve.Unknown())
				}
			}
		}
	}
}
