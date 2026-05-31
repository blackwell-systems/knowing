package rustresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// ResolveContext holds per-file state for type resolution.
type ResolveContext struct {
	Registry        *typresolve.Registry
	Scope           *typresolve.Scope
	Uses            map[string]string // short name -> module path
	ModuleQN        string            // current module qualified name
	ImplType        string            // current impl block type QN (empty if not in impl)
	Content         []byte            // source file content
	EnclosingFuncQN string            // QN of the current function being resolved
}

// nodeContent extracts the source text for a tree-sitter node.
func nodeContent(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a Rust expression AST node.
func EvalExprType(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	switch node.Type() {
	case "identifier":
		return evalIdentifier(ctx, node)

	case "self":
		// self keyword: look up in scope
		if t := ctx.Scope.Lookup("self"); t != nil && !t.IsUnknown() {
			return t
		}
		return typresolve.Unknown()

	case "scoped_identifier":
		return evalScopedIdentifier(ctx, node)

	case "call_expression":
		return evalCallExpression(ctx, node)

	case "macro_invocation":
		return evalMacroInvocation(ctx, node)

	case "field_expression":
		return evalFieldExpression(ctx, node)

	case "reference_expression":
		// &expr or &mut expr
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "&" && child.Type() != "mutable_specifier" {
				inner := EvalExprType(ctx, child)
				return typresolve.Ref(inner)
			}
		}
		return typresolve.Ref(typresolve.Unknown())

	case "dereference_expression":
		// *expr
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "*" {
				inner := EvalExprType(ctx, child)
				if inner.Kind == typresolve.KindReference || inner.Kind == typresolve.KindPointer {
					return inner.Elem
				}
				return typresolve.Unknown()
			}
		}
		return typresolve.Unknown()

	case "try_expression":
		// expr?
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "?" {
				inner := EvalExprType(ctx, child)
				if inner.Kind == typresolve.KindNamed && (strings.Contains(inner.Name, "Result") || strings.Contains(inner.Name, "Option")) && inner.Elem != nil {
					return inner.Elem
				}
				if inner.Kind == typresolve.KindOptional && inner.Elem != nil {
					return inner.Elem
				}
				return typresolve.Unknown()
			}
		}
		return typresolve.Unknown()

	case "string_literal", "raw_string_literal":
		return typresolve.Builtin("str")

	case "char_literal":
		return typresolve.Builtin("char")

	case "integer_literal":
		return typresolve.Builtin("i32")

	case "float_literal":
		return typresolve.Builtin("f64")

	case "boolean_literal", "true", "false":
		return typresolve.Builtin("bool")

	case "array_expression":
		return typresolve.Slice(typresolve.Unknown())

	case "tuple_expression":
		var elems []*typresolve.Type
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "(" && child.Type() != ")" && child.Type() != "," {
				elems = append(elems, EvalExprType(ctx, child))
			}
		}
		return typresolve.Tuple(elems)

	case "struct_expression":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			name := nodeContent(nameNode, ctx.Content)
			if path, ok := ctx.Uses[name]; ok {
				return typresolve.Named(path)
			}
			if ctx.Registry.LookupType(ctx.ModuleQN+"::"+name) != nil {
				return typresolve.Named(ctx.ModuleQN + "::" + name)
			}
			return typresolve.Named(name)
		}
		return typresolve.Unknown()

	case "unary_expression":
		op := ""
		var operand *sitter.Node
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "!" || child.Type() == "-" {
				op = child.Type()
			} else {
				operand = child
			}
		}
		if operand == nil {
			return typresolve.Unknown()
		}
		operandType := EvalExprType(ctx, operand)
		if op == "!" && operandType.Kind == typresolve.KindBuiltin && operandType.Name == "bool" {
			return typresolve.Builtin("bool")
		}
		if op == "!" {
			return typresolve.Builtin("bool")
		}
		return operandType

	case "binary_expression":
		return evalBinaryExpression(ctx, node)

	case "index_expression":
		// expr[index]
		operand := node.ChildByFieldName("value")
		if operand == nil && node.ChildCount() > 0 {
			operand = node.Child(0)
		}
		if operand != nil {
			t := EvalExprType(ctx, operand)
			if (t.Kind == typresolve.KindSlice || t.Kind == typresolve.KindArray) && t.Elem != nil {
				return t.Elem
			}
		}
		return typresolve.Unknown()

	case "type_cast_expression":
		// expr as Type
		typeNode := node.ChildByFieldName("type")
		if typeNode != nil {
			return ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Uses)
		}
		return typresolve.Unknown()

	case "closure_expression":
		return typresolve.Func(nil, nil)

	case "if_expression":
		// Evaluate consequence block
		consequence := node.ChildByFieldName("consequence")
		if consequence != nil {
			return evalBlockLastExpr(ctx, consequence)
		}
		return typresolve.Unknown()

	case "match_expression":
		// Evaluate first arm's body
		body := node.ChildByFieldName("body")
		if body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				arm := body.Child(i)
				if arm.Type() == "match_arm" {
					valueNode := arm.ChildByFieldName("value")
					if valueNode != nil {
						return EvalExprType(ctx, valueNode)
					}
				}
			}
		}
		return typresolve.Unknown()

	case "block":
		return evalBlockLastExpr(ctx, node)

	case "parenthesized_expression":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "(" && child.Type() != ")" {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "await_expression":
		return typresolve.Unknown()

	case "range_expression":
		return typresolve.Named("std::ops::Range")

	default:
		return typresolve.Unknown()
	}
}

func evalIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeContent(node, ctx.Content)

	// Scope lookup
	if t := ctx.Scope.Lookup(name); t != nil && !t.IsUnknown() {
		return t
	}

	// Builtin type check
	if IsBuiltinType(name) {
		return ResolveBuiltinType(name)
	}

	// Enum variant constructors
	switch name {
	case "Some", "None":
		return typresolve.Named("std::Option")
	case "Ok", "Err":
		return typresolve.Named("std::Result")
	}

	// Uses map
	if path, ok := ctx.Uses[name]; ok {
		return typresolve.Named(path)
	}

	return typresolve.Unknown()
}

func evalScopedIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	fullPath := nodeContent(node, ctx.Content)
	parts := strings.Split(fullPath, "::")

	if len(parts) == 0 {
		return typresolve.Unknown()
	}

	// If first segment is in uses map, resolve through import
	if resolved, ok := ctx.Uses[parts[0]]; ok {
		parts[0] = resolved
		resolvedPath := strings.Join(parts, "::")
		if ctx.Registry.LookupType(resolvedPath) != nil {
			return typresolve.Named(resolvedPath)
		}
		return typresolve.Named(resolvedPath)
	}

	// If first segment is a known type and second is method/variant
	if len(parts) == 2 {
		typeQN := parts[0]
		if ctx.Registry.LookupType(typeQN) != nil {
			return typresolve.Named(typeQN)
		}
		// Try with module prefix
		fullQN := ctx.ModuleQN + "::" + typeQN
		if ctx.Registry.LookupType(fullQN) != nil {
			return typresolve.Named(fullQN)
		}
	}

	// Look up full path in registry
	if ctx.Registry.LookupType(fullPath) != nil {
		return typresolve.Named(fullPath)
	}

	// Best effort
	return typresolve.Named(fullPath)
}

func evalCallExpression(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		return typresolve.Unknown()
	}

	switch funcNode.Type() {
	case "identifier":
		name := nodeContent(funcNode, ctx.Content)
		if IsBuiltinFunc(name) {
			return typresolve.Unknown()
		}
		// Look up in scope
		if t := ctx.Scope.Lookup(name); t != nil {
			if t.Kind == typresolve.KindFunc && len(t.Returns) > 0 {
				return t.Returns[0]
			}
			if t.Kind == typresolve.KindNamed {
				return t
			}
		}
		return typresolve.Unknown()

	case "scoped_identifier":
		fullPath := nodeContent(funcNode, ctx.Content)
		parts := strings.Split(fullPath, "::")
		if len(parts) >= 2 {
			lastPart := parts[len(parts)-1]
			typeParts := parts[:len(parts)-1]
			typePath := strings.Join(typeParts, "::")

			// Type::new, Type::from, Type::default patterns
			if lastPart == "new" || lastPart == "from" || lastPart == "default" {
				// Resolve the type
				if resolved, ok := ctx.Uses[typePath]; ok {
					return typresolve.Named(resolved)
				}
				if ctx.Registry.LookupType(typePath) != nil {
					return typresolve.Named(typePath)
				}
				fullQN := ctx.ModuleQN + "::" + typePath
				if ctx.Registry.LookupType(fullQN) != nil {
					return typresolve.Named(fullQN)
				}
				return typresolve.Named(typePath)
			}

			// module::func lookup
			if resolved, ok := ctx.Uses[parts[0]]; ok {
				parts[0] = resolved
				funcQN := strings.Join(parts, "::")
				if f := ctx.Registry.LookupFunc(funcQN); f != nil && f.Signature != nil && len(f.Signature.Returns) > 0 {
					return f.Signature.Returns[0]
				}
			}
		}
		return typresolve.Unknown()

	case "field_expression":
		// Method call: obj.method(args)
		operandNode := funcNode.ChildByFieldName("value")
		fieldNode := funcNode.ChildByFieldName("field")
		if operandNode == nil || fieldNode == nil {
			return typresolve.Unknown()
		}
		receiverType := EvalExprType(ctx, operandNode)
		baseType := DerefToBase(receiverType)
		if baseType.Kind == typresolve.KindNamed {
			methodName := nodeContent(fieldNode, ctx.Content)
			if m := LookupMethod(ctx.Registry, baseType.Name, methodName); m != nil {
				if m.Signature != nil && len(m.Signature.Returns) > 0 {
					return m.Signature.Returns[0]
				}
			}
		}
		return typresolve.Unknown()

	default:
		// Evaluate function type recursively
		funcType := EvalExprType(ctx, funcNode)
		if funcType.Kind == typresolve.KindFunc && len(funcType.Returns) > 0 {
			return funcType.Returns[0]
		}
		if funcType.Kind == typresolve.KindNamed {
			return funcType
		}
		return typresolve.Unknown()
	}
}

func evalMacroInvocation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	macroNode := node.ChildByFieldName("macro")
	if macroNode == nil {
		// Try first child
		if node.ChildCount() > 0 {
			macroNode = node.Child(0)
		}
	}
	if macroNode != nil {
		macroName := nodeContent(macroNode, ctx.Content)
		// Strip trailing ! if present
		macroName = strings.TrimSuffix(macroName, "!")
		return EvalMacroReturnType(macroName)
	}
	return typresolve.Unknown()
}

func evalFieldExpression(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	operandNode := node.ChildByFieldName("value")
	fieldNode := node.ChildByFieldName("field")
	if operandNode == nil || fieldNode == nil {
		return typresolve.Unknown()
	}

	baseType := EvalExprType(ctx, operandNode)
	derefed := DerefToBase(baseType)
	if derefed.Kind == typresolve.KindNamed {
		fieldName := nodeContent(fieldNode, ctx.Content)
		if ft := LookupField(ctx.Registry, derefed.Name, fieldName); ft != nil {
			return ft
		}
	}
	return typresolve.Unknown()
}

func evalBinaryExpression(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// Find operator
	var op string
	var leftNode *sitter.Node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "==", "!=", "<", ">", "<=", ">=":
			return typresolve.Builtin("bool")
		case "&&", "||":
			return typresolve.Builtin("bool")
		case "+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>":
			op = child.Type()
		default:
			if leftNode == nil {
				leftNode = child
			}
		}
	}
	_ = op
	if leftNode != nil {
		return EvalExprType(ctx, leftNode)
	}
	return typresolve.Unknown()
}

func evalBlockLastExpr(ctx *ResolveContext, block *sitter.Node) *typresolve.Type {
	if block == nil {
		return typresolve.Unknown()
	}
	// Find last non-brace child
	var lastExpr *sitter.Node
	for i := 0; i < int(block.ChildCount()); i++ {
		child := block.Child(i)
		if child.Type() != "{" && child.Type() != "}" {
			lastExpr = child
		}
	}
	if lastExpr != nil {
		return EvalExprType(ctx, lastExpr)
	}
	return typresolve.Unknown()
}
