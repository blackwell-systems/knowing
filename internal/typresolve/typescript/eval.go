package tsresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

const maxEvalDepth = 64

// ResolveContext holds per-file state for TypeScript type resolution.
type ResolveContext struct {
	Registry         *typresolve.Registry
	Scope            *typresolve.Scope
	Imports          map[string]ImportInfo // from imports.go (Agent A)
	ModuleQN         string               // current module qualified name
	Content          []byte               // source file content
	EnclosingFuncQN  string               // QN of enclosing function
	EnclosingClassQN string               // QN of enclosing class (for this/super)
}

// nodeText extracts the source text for a tree-sitter node.
func nodeText(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a TypeScript expression AST node using
// scope lookup, registry lookup, import resolution, member dispatch, and
// async unwrapping. This is the TS port of ts_eval_expr_type from the C
// reference implementation.
func EvalExprType(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}
	return evalExprTypeInner(ctx, node, 0)
}

// evalExprTypeInner is the recursive helper with depth tracking to prevent
// infinite recursion (max 64, matching C reference TS_LSP_MAX_EVAL_DEPTH).
func evalExprTypeInner(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	if node == nil || depth >= maxEvalDepth {
		return typresolve.Unknown()
	}

	switch node.Type() {
	case "identifier":
		return evalIdentifier(ctx, node, depth)

	case "member_expression":
		return evalMemberExpression(ctx, node, depth)

	case "call_expression":
		return evalCallExpression(ctx, node, depth)

	case "new_expression":
		return evalNewExpression(ctx, node, depth)

	case "await_expression":
		return evalAwaitExpression(ctx, node, depth)

	case "as_expression", "satisfies_expression":
		return evalTypeAssertion(ctx, node)

	case "non_null_expression":
		// x! strips null/undefined conceptually, return operand type.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() != "!" {
				return evalExprTypeInner(ctx, child, depth+1)
			}
		}
		return typresolve.Unknown()

	case "parenthesized_expression":
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil {
				return evalExprTypeInner(ctx, child, depth+1)
			}
		}
		return typresolve.Unknown()

	case "template_string", "template_substitution":
		return typresolve.Builtin("string")

	case "string", "string_fragment":
		return typresolve.Builtin("string")

	case "number":
		return typresolve.Builtin("number")

	case "true", "false":
		return typresolve.Builtin("boolean")

	case "null":
		return typresolve.Builtin("null")

	case "undefined":
		return typresolve.Builtin("undefined")

	case "regex":
		return typresolve.Named("RegExp")

	case "array":
		return evalArrayLiteral(ctx, node, depth)

	case "object":
		return typresolve.Unknown()

	case "binary_expression":
		return evalBinaryExpression(ctx, node, depth)

	case "unary_expression":
		return evalUnaryExpression(ctx, node, depth)

	case "ternary_expression":
		return evalTernaryExpression(ctx, node, depth)

	case "arrow_function":
		return evalArrowFunction(ctx, node, depth)

	case "subscript_expression":
		return evalSubscriptExpression(ctx, node, depth)

	case "type_assertion":
		return evalLegacyTypeAssertion(ctx, node)

	case "this":
		if ctx.EnclosingClassQN != "" {
			return typresolve.Named(ctx.EnclosingClassQN)
		}
		return typresolve.Unknown()

	case "super":
		return evalSuper(ctx)

	default:
		// Check if it's a literal type via LiteralType helper.
		if t := LiteralType(node.Type()); t != nil {
			return t
		}
		return typresolve.Unknown()
	}
}

// evalIdentifier resolves a TypeScript identifier expression.
func evalIdentifier(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	name := nodeText(node, ctx.Content)

	// 1. Scope lookup.
	if t := ctx.Scope.Lookup(name); t != nil {
		return t
	}

	// 2. Special values.
	switch name {
	case "true", "false":
		return typresolve.Builtin("boolean")
	case "null":
		return typresolve.Builtin("null")
	case "undefined":
		return typresolve.Builtin("undefined")
	case "NaN", "Infinity":
		return typresolve.Builtin("number")
	}

	// 3. Imports.
	if imp, ok := ResolveImport(ctx.Imports, name); ok {
		modulePath := ResolveModulePath(imp.ModulePath, "")

		// Namespace import: return Named(modulePath).
		if imp.IsNamespace {
			return typresolve.Named(modulePath)
		}

		// Look up original name in registry.
		lookupName := name
		if imp.OriginalName != "" && imp.OriginalName != name {
			lookupName = imp.OriginalName
		}

		// Check as function.
		if f := ctx.Registry.LookupFunc(modulePath + "." + lookupName); f != nil {
			if f.Signature != nil {
				return f.Signature
			}
		}

		// Check as type.
		if t := ctx.Registry.LookupType(modulePath + "." + lookupName); t != nil {
			return typresolve.Named(modulePath + "." + lookupName)
		}
	}

	// 4. Module-local function.
	if f := ctx.Registry.LookupSymbol(ctx.ModuleQN, name); f != nil {
		if f.Signature != nil {
			return f.Signature
		}
	}

	// 5. Module-local type.
	if t := ctx.Registry.LookupType(ctx.ModuleQN + "." + name); t != nil {
		return typresolve.Named(ctx.ModuleQN + "." + name)
	}

	// 6. Global/stdlib.
	if t := ctx.Registry.LookupType(name); t != nil {
		return typresolve.Named(name)
	}
	if f := ctx.Registry.LookupFunc(name); f != nil {
		if f.Signature != nil {
			return f.Signature
		}
	}

	return typresolve.Unknown()
}

// evalMemberExpression resolves a member expression (obj.prop or obj?.prop).
func evalMemberExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	object := node.ChildByFieldName("object")
	property := node.ChildByFieldName("property")
	if object == nil || property == nil {
		return typresolve.Unknown()
	}

	propName := nodeText(property, ctx.Content)

	// Check if object is a namespace import.
	if object.Type() == "identifier" {
		objName := nodeText(object, ctx.Content)
		if imp, ok := ResolveImport(ctx.Imports, objName); ok && imp.IsNamespace {
			modulePath := ResolveModulePath(imp.ModulePath, "")
			// Look up as function.
			if f := ctx.Registry.LookupFunc(modulePath + "." + propName); f != nil {
				if f.Signature != nil {
					return f.Signature
				}
			}
			// Look up as type.
			if t := ctx.Registry.LookupType(modulePath + "." + propName); t != nil {
				return typresolve.Named(modulePath + "." + propName)
			}
			return typresolve.Unknown()
		}
	}

	// Evaluate object type recursively.
	objType := evalExprTypeInner(ctx, object, depth+1)
	if objType == nil {
		return typresolve.Unknown()
	}

	if objType.Kind == typresolve.KindNamed {
		// Look up member on the type.
		if t := LookupMemberType(ctx.Registry, objType.Name, propName); t != nil {
			return t
		}
	}

	if objType.Kind == typresolve.KindBuiltin {
		wrapper := BuiltinWrapperClass(objType.Name)
		if wrapper != "" {
			if t := LookupMemberType(ctx.Registry, wrapper, propName); t != nil {
				return t
			}
		}
	}

	return typresolve.Unknown()
}

// evalCallExpression resolves a call expression.
func evalCallExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return typresolve.Unknown()
	}

	// Check for builtin type constructors: Number(), String(), Boolean().
	if fnNode.Type() == "identifier" {
		name := nodeText(fnNode, ctx.Content)
		switch name {
		case "Number":
			return typresolve.Builtin("number")
		case "String":
			return typresolve.Builtin("string")
		case "Boolean":
			return typresolve.Builtin("boolean")
		}

		// Check if it's a class name (constructor call without new).
		typeQN := ctx.ModuleQN + "." + name
		if t := ctx.Registry.LookupType(typeQN); t != nil {
			return typresolve.Named(typeQN)
		}
	}

	// Evaluate function type recursively.
	fnType := evalExprTypeInner(ctx, fnNode, depth+1)
	if fnType == nil {
		return typresolve.Unknown()
	}

	// If function type is KindFunc with returns, return first return type.
	if fnType.Kind == typresolve.KindFunc {
		if len(fnType.Returns) > 0 {
			return fnType.Returns[0]
		}
		return typresolve.Unknown()
	}

	// If function type is Named, this is a type conversion/constructor call.
	if fnType.Kind == typresolve.KindNamed {
		return fnType
	}

	return typresolve.Unknown()
}

// evalNewExpression resolves a new expression (new MyClass()).
func evalNewExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	constructor := node.ChildByFieldName("constructor")
	if constructor == nil {
		return typresolve.Unknown()
	}

	// If constructor is an identifier, look up the class.
	if constructor.Type() == "identifier" {
		name := nodeText(constructor, ctx.Content)

		// Check imports.
		if imp, ok := ResolveImport(ctx.Imports, name); ok {
			modulePath := ResolveModulePath(imp.ModulePath, "")
			lookupName := name
			if imp.OriginalName != "" && imp.OriginalName != name {
				lookupName = imp.OriginalName
			}
			typeQN := modulePath + "." + lookupName
			if ctx.Registry.LookupType(typeQN) != nil {
				return typresolve.Named(typeQN)
			}
		}

		// Check module-local type.
		typeQN := ctx.ModuleQN + "." + name
		if ctx.Registry.LookupType(typeQN) != nil {
			return typresolve.Named(typeQN)
		}

		// Check global type.
		if ctx.Registry.LookupType(name) != nil {
			return typresolve.Named(name)
		}

		// Even if not in registry, return Named for the class.
		return typresolve.Named(ctx.ModuleQN + "." + name)
	}

	// Evaluate constructor type.
	ctorType := evalExprTypeInner(ctx, constructor, depth+1)
	if ctorType != nil && ctorType.Kind == typresolve.KindNamed {
		return ctorType
	}

	return typresolve.Unknown()
}

// evalAwaitExpression resolves an await expression by unwrapping Promise.
func evalAwaitExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	// The operand is the first named child.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil {
			operandType := evalExprTypeInner(ctx, child, depth+1)
			return UnwrapPromise(operandType)
		}
	}
	return typresolve.Unknown()
}

// evalTypeAssertion resolves as_expression and satisfies_expression.
func evalTypeAssertion(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// The type is in the second named child or the "type" field.
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		return ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Imports)
	}
	// Fallback: last named child is often the type.
	count := int(node.NamedChildCount())
	if count >= 2 {
		typeChild := node.NamedChild(count - 1)
		if typeChild != nil {
			return ParseTypeNode(typeChild, ctx.Content, ctx.ModuleQN, ctx.Imports)
		}
	}
	return typresolve.Unknown()
}

// evalLegacyTypeAssertion resolves <Type>expr syntax.
func evalLegacyTypeAssertion(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// The type child comes first in <Type>expr.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil {
			return ParseTypeNode(child, ctx.Content, ctx.ModuleQN, ctx.Imports)
		}
	}
	return typresolve.Unknown()
}

// evalArrayLiteral resolves an array literal [1, 2, 3].
func evalArrayLiteral(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	if node.NamedChildCount() > 0 {
		first := node.NamedChild(0)
		if first != nil {
			elemType := evalExprTypeInner(ctx, first, depth+1)
			return typresolve.Slice(elemType)
		}
	}
	return typresolve.Slice(typresolve.Unknown())
}

// evalBinaryExpression resolves a binary expression.
func evalBinaryExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	// Get operator text. In tree-sitter TS, operator is typically an unnamed child.
	// We check children for operator text.
	var op string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(int(i))
		if child != nil && !child.IsNamed() {
			text := nodeText(child, ctx.Content)
			switch text {
			case "==", "!=", "<", ">", "<=", ">=", "===", "!==", "instanceof", "in":
				return typresolve.Builtin("boolean")
			case "+":
				op = "+"
			default:
				if op == "" {
					op = text
				}
			}
		}
	}

	if op == "+" {
		// If either operand is string, result is string.
		left := node.ChildByFieldName("left")
		right := node.ChildByFieldName("right")
		if left != nil {
			leftType := evalExprTypeInner(ctx, left, depth+1)
			if leftType != nil && leftType.Kind == typresolve.KindBuiltin && leftType.Name == "string" {
				return typresolve.Builtin("string")
			}
		}
		if right != nil {
			rightType := evalExprTypeInner(ctx, right, depth+1)
			if rightType != nil && rightType.Kind == typresolve.KindBuiltin && rightType.Name == "string" {
				return typresolve.Builtin("string")
			}
		}
		// Default: return left operand type.
		if left != nil {
			return evalExprTypeInner(ctx, left, depth+1)
		}
	}

	// Other arithmetic: return left operand type.
	left := node.ChildByFieldName("left")
	if left != nil {
		return evalExprTypeInner(ctx, left, depth+1)
	}
	return typresolve.Unknown()
}

// evalUnaryExpression resolves a unary expression.
func evalUnaryExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	// Find operator (first unnamed child).
	var op string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(int(i))
		if child != nil && !child.IsNamed() {
			op = nodeText(child, ctx.Content)
			break
		}
	}

	switch op {
	case "typeof":
		return typresolve.Builtin("string")
	case "!":
		return typresolve.Builtin("boolean")
	case "-", "+", "~":
		return typresolve.Builtin("number")
	case "void":
		return typresolve.Builtin("undefined")
	default:
		// Evaluate operand and return its type.
		operand := node.ChildByFieldName("argument")
		if operand != nil {
			return evalExprTypeInner(ctx, operand, depth+1)
		}
		// Fallback: first named child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil {
				return evalExprTypeInner(ctx, child, depth+1)
			}
		}
		return typresolve.Unknown()
	}
}

// evalTernaryExpression resolves a ternary/conditional expression (a ? b : c).
func evalTernaryExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	// Return the consequence (second named child).
	consequence := node.ChildByFieldName("consequence")
	if consequence != nil {
		return evalExprTypeInner(ctx, consequence, depth+1)
	}
	// Fallback: second named child.
	if node.NamedChildCount() >= 2 {
		child := node.NamedChild(1)
		if child != nil {
			return evalExprTypeInner(ctx, child, depth+1)
		}
	}
	return typresolve.Unknown()
}

// evalArrowFunction resolves an arrow function expression.
func evalArrowFunction(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	var params []typresolve.Param
	var returns []*typresolve.Type

	// Check for return type annotation.
	returnTypeNode := node.ChildByFieldName("return_type")
	if returnTypeNode != nil {
		retType := ParseTypeNode(returnTypeNode, ctx.Content, ctx.ModuleQN, ctx.Imports)
		if retType != nil {
			returns = append(returns, retType)
		}
	}

	// Parse parameters.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		params = parseArrowParams(ctx, paramsNode)
	} else {
		// Single parameter without parens (e.g. x => x + 1).
		paramNode := node.ChildByFieldName("parameter")
		if paramNode != nil && paramNode.Type() == "identifier" {
			params = append(params, typresolve.Param{
				Name: nodeText(paramNode, ctx.Content),
				Type: typresolve.Unknown(),
			})
		}
	}

	// Push scope and bind params (do NOT walk body; resolve.go handles it).
	childScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = childScope

	for _, p := range params {
		if p.Name != "" {
			ctx.Scope.Bind(p.Name, p.Type)
		}
	}

	// If no return type annotation, evaluate body expression.
	if len(returns) == 0 {
		body := node.ChildByFieldName("body")
		if body != nil && body.Type() != "statement_block" {
			retType := evalExprTypeInner(ctx, body, depth+1)
			if retType != nil {
				returns = append(returns, retType)
			}
		}
	}

	ctx.Scope = origScope

	return typresolve.Func(params, returns)
}

// parseArrowParams extracts parameters from an arrow function's formal_parameters node.
func parseArrowParams(ctx *ResolveContext, node *sitter.Node) []typresolve.Param {
	var params []typresolve.Param
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}
		switch child.Type() {
		case "required_parameter", "optional_parameter":
			nameNode := child.ChildByFieldName("pattern")
			if nameNode == nil {
				nameNode = child.ChildByFieldName("name")
			}
			var paramName string
			if nameNode != nil {
				paramName = nodeText(nameNode, ctx.Content)
			}
			typeNode := child.ChildByFieldName("type")
			var paramType *typresolve.Type
			if typeNode != nil {
				paramType = ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Imports)
			} else {
				paramType = typresolve.Unknown()
			}
			params = append(params, typresolve.Param{Name: paramName, Type: paramType})
		case "identifier":
			params = append(params, typresolve.Param{
				Name: nodeText(child, ctx.Content),
				Type: typresolve.Unknown(),
			})
		}
	}
	return params
}

// evalSubscriptExpression resolves arr[0], map["key"], etc.
func evalSubscriptExpression(ctx *ResolveContext, node *sitter.Node, depth int) *typresolve.Type {
	object := node.ChildByFieldName("object")
	if object == nil {
		return typresolve.Unknown()
	}

	objType := evalExprTypeInner(ctx, object, depth+1)
	if objType == nil {
		return typresolve.Unknown()
	}

	switch objType.Kind {
	case typresolve.KindSlice, typresolve.KindArray:
		if objType.Elem != nil {
			return objType.Elem
		}
	case typresolve.KindMap:
		if objType.Value != nil {
			return objType.Value
		}
	case typresolve.KindTuple:
		// If index is a numeric literal, return corresponding element.
		index := node.ChildByFieldName("index")
		if index != nil && index.Type() == "number" {
			text := nodeText(index, ctx.Content)
			if idx := parseSimpleInt(text); idx >= 0 && idx < len(objType.Elements) {
				return objType.Elements[idx]
			}
		}
		// Default: return first element if available.
		if len(objType.Elements) > 0 {
			return objType.Elements[0]
		}
	}

	return typresolve.Unknown()
}

// parseSimpleInt parses a simple non-negative integer from a string.
// Returns -1 on failure.
func parseSimpleInt(s string) int {
	if len(s) == 0 {
		return -1
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// evalSuper resolves the super keyword.
func evalSuper(ctx *ResolveContext) *typresolve.Type {
	if ctx.EnclosingClassQN == "" {
		return typresolve.Unknown()
	}

	// Look up the enclosing class to find its parent (EmbeddedTypes[0]).
	if t := ctx.Registry.LookupType(ctx.EnclosingClassQN); t != nil {
		if len(t.EmbeddedTypes) > 0 {
			return typresolve.Named(t.EmbeddedTypes[0])
		}
	}
	return typresolve.Unknown()
}
