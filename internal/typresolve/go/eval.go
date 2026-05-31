package goresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveContext holds per-file state for type resolution.
type ResolveContext struct {
	Registry        *typresolve.Registry
	Scope           *typresolve.Scope
	Imports         map[string]string // alias -> package path
	PkgQN           string           // current package qualified name
	Content         []byte           // source file content
	EnclosingFuncQN string           // QN of the current function being resolved
}

// nodeContent extracts the source text for a tree-sitter node.
func nodeContent(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a Go expression AST node using scope
// lookup, registry lookup, import resolution, and method dispatch. This is
// the Go port of go_eval_expr_type from the C reference implementation.
func EvalExprType(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	switch node.Type() {
	case "identifier":
		return evalIdentifier(ctx, node)

	case "selector_expression":
		return evalSelector(ctx, node)

	case "call_expression":
		return evalCall(ctx, node)

	case "composite_literal":
		return evalCompositeLiteral(ctx, node)

	case "unary_expression":
		return evalUnary(ctx, node)

	case "index_expression":
		return evalIndex(ctx, node)

	case "type_assertion_expression":
		return evalTypeAssertion(ctx, node)

	case "parenthesized_expression":
		// Unwrap parentheses.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(int(i))
			if child.IsNamed() {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "binary_expression":
		return evalBinary(ctx, node)

	case "slice_expression":
		// Slice of a value has the same type as the value.
		operand := node.ChildByFieldName("operand")
		return EvalExprType(ctx, operand)

	case "interpreted_string_literal", "raw_string_literal":
		return typresolve.Builtin("string")

	case "int_literal":
		return typresolve.Builtin("int")

	case "float_literal":
		return typresolve.Builtin("float64")

	case "true", "false":
		return typresolve.Builtin("bool")

	case "nil":
		return typresolve.Unknown()

	case "rune_literal":
		return typresolve.Builtin("rune")

	case "func_literal":
		return evalFuncLiteral(ctx, node)

	default:
		return typresolve.Unknown()
	}
}

// evalIdentifier resolves an identifier expression.
func evalIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeContent(node, ctx.Content)

	// 1. Scope lookup.
	if t := ctx.Scope.Lookup(name); t != nil {
		return t
	}

	// 2. Package-level function from registry.
	if f := ctx.Registry.LookupSymbol(ctx.PkgQN, name); f != nil {
		if f.Signature != nil {
			return f.Signature
		}
	}

	// 3. Builtin type.
	if t := ResolveBuiltinType(name); t != nil {
		return t
	}

	// 4. Registered named type in current package.
	if t := ctx.Registry.LookupType(ctx.PkgQN + "." + name); t != nil {
		return typresolve.Named(ctx.PkgQN + "." + name)
	}

	return typresolve.Unknown()
}

// evalSelector resolves a selector expression (e.g. pkg.Symbol or obj.Method).
func evalSelector(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	operand := node.ChildByFieldName("operand")
	field := node.ChildByFieldName("field")
	if operand == nil || field == nil {
		return typresolve.Unknown()
	}

	fieldName := nodeContent(field, ctx.Content)

	// Check if operand is an import alias.
	if operand.Type() == "identifier" {
		alias := nodeContent(operand, ctx.Content)
		if pkgQN, ok := ResolveImport(ctx.Imports, alias); ok {
			// Look up as function in the imported package.
			if f := ctx.Registry.LookupSymbol(pkgQN, fieldName); f != nil {
				if f.Signature != nil {
					return f.Signature
				}
			}
			// Look up as type in the imported package.
			typeQN := pkgQN + "." + fieldName
			if t := ctx.Registry.LookupType(typeQN); t != nil {
				return typresolve.Named(typeQN)
			}
			return typresolve.Unknown()
		}
	}

	// Evaluate operand type recursively.
	base := EvalExprType(ctx, operand)
	if base == nil {
		return typresolve.Unknown()
	}

	// Auto-deref pointers.
	if base.Kind == typresolve.KindPointer {
		base = base.Deref()
	}

	if base.Kind == typresolve.KindNamed {
		typeQN := base.Name

		// Look up method.
		if m := LookupFieldOrMethod(ctx.Registry, typeQN, fieldName); m != nil {
			if m.Signature != nil {
				return m.Signature
			}
		}

		// Look up field.
		if ft := LookupField(ctx.Registry, typeQN, fieldName); ft != nil {
			return ft
		}
	}

	return typresolve.Unknown()
}

// evalCall resolves a call expression.
func evalCall(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	fnNode := node.ChildByFieldName("function")
	argsNode := node.ChildByFieldName("arguments")
	if fnNode == nil {
		return typresolve.Unknown()
	}

	// Check for builtin function call.
	if fnNode.Type() == "identifier" {
		name := nodeContent(fnNode, ctx.Content)
		if IsBuiltinFunc(name) {
			evalExpr := func(n *sitter.Node) *typresolve.Type {
				return EvalExprType(ctx, n)
			}
			return EvalBuiltinCall(name, argsNode, ctx.Content, ctx.PkgQN, ctx.Imports, evalExpr)
		}
	}

	// Check if function node is a type conversion (type node used as function).
	switch fnNode.Type() {
	case "slice_type", "map_type", "array_type", "channel_type",
		"pointer_type", "function_type", "struct_type", "interface_type",
		"parenthesized_type":
		return ParseTypeNode(fnNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	}

	// Evaluate function type recursively.
	fnType := EvalExprType(ctx, fnNode)
	if fnType == nil {
		return typresolve.Unknown()
	}

	// If function type is KindFunc with returns, return first return type
	// (or Tuple for multi-return).
	if fnType.Kind == typresolve.KindFunc {
		switch len(fnType.Returns) {
		case 0:
			return typresolve.Unknown()
		case 1:
			return fnType.Returns[0]
		default:
			return typresolve.Tuple(fnType.Returns)
		}
	}

	// If function type is KindNamed, this is a type conversion.
	if fnType.Kind == typresolve.KindNamed {
		return fnType
	}

	return typresolve.Unknown()
}

// evalCompositeLiteral resolves a composite literal (e.g. MyType{...}).
func evalCompositeLiteral(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return typresolve.Unknown()
	}
	return ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
}

// evalUnary resolves a unary expression.
func evalUnary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// Find the operator (first unnamed child or check content).
	operand := node.ChildByFieldName("operand")
	if operand == nil {
		return typresolve.Unknown()
	}

	// The operator is the first child (unnamed).
	opNode := node.Child(0)
	if opNode == nil {
		return typresolve.Unknown()
	}
	op := nodeContent(opNode, ctx.Content)

	switch op {
	case "&":
		inner := EvalExprType(ctx, operand)
		return typresolve.Pointer(inner)
	case "*":
		inner := EvalExprType(ctx, operand)
		return inner.Deref()
	case "<-":
		inner := EvalExprType(ctx, operand)
		if inner != nil && inner.Kind == typresolve.KindChannel {
			if inner.Elem != nil {
				return inner.Elem
			}
		}
		return typresolve.Unknown()
	case "!":
		return typresolve.Builtin("bool")
	default:
		return EvalExprType(ctx, operand)
	}
}

// evalIndex resolves an index expression (e.g. m[key] or xs[0]).
func evalIndex(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	operand := node.ChildByFieldName("operand")
	if operand == nil {
		return typresolve.Unknown()
	}

	base := EvalExprType(ctx, operand)
	if base == nil {
		return typresolve.Unknown()
	}

	switch base.Kind {
	case typresolve.KindMap:
		if base.Value != nil {
			return base.Value
		}
	case typresolve.KindSlice, typresolve.KindArray:
		if base.Elem != nil {
			return base.Elem
		}
	}

	return typresolve.Unknown()
}

// evalTypeAssertion resolves a type assertion expression (e.g. x.(T)).
func evalTypeAssertion(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return typresolve.Unknown()
	}
	return ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
}

// evalBinary resolves a binary expression.
func evalBinary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	opNode := node.ChildByFieldName("operator")
	if opNode != nil {
		op := nodeContent(opNode, ctx.Content)
		switch op {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return typresolve.Builtin("bool")
		}
	}

	// For arithmetic/bitwise operators, return left operand type.
	left := node.ChildByFieldName("left")
	if left != nil {
		return EvalExprType(ctx, left)
	}
	return typresolve.Unknown()
}

// evalFuncLiteral resolves a function literal (closure).
func evalFuncLiteral(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// Push child scope.
	childScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = childScope

	var params []typresolve.Param
	var returns []*typresolve.Type

	// Parse parameters.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		params = parseParamList(ctx, paramsNode)
		// Bind parameters into scope.
		for _, p := range params {
			if p.Name != "" && p.Name != "_" {
				ctx.Scope.Bind(p.Name, p.Type)
			}
		}
	}

	// Parse return type.
	resultNode := node.ChildByFieldName("result")
	if resultNode != nil {
		if resultNode.Type() == "parameter_list" {
			// Multiple returns.
			for i := 0; i < int(resultNode.NamedChildCount()); i++ {
				paramDecl := resultNode.NamedChild(int(i))
				if paramDecl != nil {
					typeNode := paramDecl.ChildByFieldName("type")
					if typeNode != nil {
						returns = append(returns, ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports))
					}
				}
			}
		} else {
			// Single return type.
			returns = append(returns, ParseTypeNode(resultNode, ctx.Content, ctx.PkgQN, ctx.Imports))
		}
	}

	// Walk closure body to resolve calls.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkBlock(ctx, body)
	}

	// Restore scope before returning type.
	ctx.Scope = origScope

	return typresolve.Func(params, returns)
}

// parseParamList extracts parameters from a parameter_list node.
func parseParamList(ctx *ResolveContext, node *sitter.Node) []typresolve.Param {
	var params []typresolve.Param
	for i := 0; i < int(node.NamedChildCount()); i++ {
		paramDecl := node.NamedChild(int(i))
		if paramDecl == nil || paramDecl.Type() != "parameter_declaration" {
			continue
		}
		typeNode := paramDecl.ChildByFieldName("type")
		var paramType *typresolve.Type
		if typeNode != nil {
			paramType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
		} else {
			paramType = typresolve.Unknown()
		}

		// Collect all identifier children as parameter names.
		nameFound := false
		for j := 0; j < int(paramDecl.ChildCount()); j++ {
			child := paramDecl.Child(int(j))
			if child.Type() == "identifier" {
				params = append(params, typresolve.Param{
					Name: nodeContent(child, ctx.Content),
					Type: paramType,
				})
				nameFound = true
			}
		}
		// Unnamed parameter (type only).
		if !nameFound {
			params = append(params, typresolve.Param{Type: paramType})
		}
	}
	return params
}

// walkBlock walks a block node, processing statements for scope binding.
func walkBlock(ctx *ResolveContext, block *sitter.Node) {
	for i := 0; i < int(block.NamedChildCount()); i++ {
		child := block.NamedChild(int(i))
		if child == nil {
			continue
		}
		ProcessStatement(ctx, child)
		// Recurse into nested blocks (if/for/etc.).
		walkBlockChildren(ctx, child)
	}
}

// walkBlockChildren recurses into child nodes that may contain blocks.
func walkBlockChildren(ctx *ResolveContext, node *sitter.Node) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}
		switch child.Type() {
		case "block":
			walkBlock(ctx, child)
		case "if_statement", "for_statement", "switch_statement",
			"select_statement", "case_clause", "default_clause":
			walkBlockChildren(ctx, child)
		}
	}
}
