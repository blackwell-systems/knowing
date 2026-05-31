package csresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// csEvalMaxDepth is the maximum recursion depth for expression evaluation,
// matching CS_EVAL_MAX_DEPTH from the C reference implementation.
const csEvalMaxDepth = 64

// ResolveContext holds per-file state for C# type resolution.
type ResolveContext struct {
	Registry         *typresolve.Registry
	Scope            *typresolve.Scope
	Usings           []UsingInfo
	NamespaceStack   []string
	Content          []byte
	EnclosingFuncQN  string
	EnclosingClassQN string
	EnclosingBaseQN  string
	ModuleQN         string
	EvalDepth        int
}

// nodeContent extracts the source text for a tree-sitter node.
func nodeContent(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a C# expression AST node using scope
// lookup, registry lookup, member access, invocation, and literal types.
// Port of cs_eval_expr_type from cs_lsp.c lines 716-884.
func EvalExprType(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	ctx.EvalDepth++
	if ctx.EvalDepth > csEvalMaxDepth {
		ctx.EvalDepth--
		return typresolve.Unknown()
	}
	defer func() { ctx.EvalDepth-- }()

	switch node.Type() {
	case "identifier":
		return evalIdentifier(ctx, node)

	case "this_expression", "this":
		if ctx.EnclosingClassQN != "" {
			return typresolve.Named(ctx.EnclosingClassQN)
		}
		return typresolve.Unknown()

	case "base_expression", "base":
		if ctx.EnclosingBaseQN != "" {
			return typresolve.Named(ctx.EnclosingBaseQN)
		}
		return typresolve.Unknown()

	case "invocation_expression":
		return evalInvocation(ctx, node)

	case "member_access_expression":
		return evalMemberAccess(ctx, node)

	case "conditional_access_expression":
		// x?.Method -- evaluate the expression part (child 0) then member
		return evalConditionalAccess(ctx, node)

	case "object_creation_expression":
		return evalObjectCreation(ctx, node)

	case "implicit_object_creation_expression":
		// target-typed new: `new() { ... }` -- type inferred from context
		return typresolve.Unknown()

	case "string_literal", "interpolated_string_expression",
		"verbatim_string_literal":
		return typresolve.Named("System.String")

	case "character_literal":
		return typresolve.Named("System.Char")

	case "integer_literal":
		return typresolve.Named("System.Int32")

	case "real_literal":
		return typresolve.Named("System.Double")

	case "boolean_literal":
		return typresolve.Named("System.Boolean")

	case "null_literal":
		return typresolve.Unknown()

	case "parenthesized_expression":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(int(i))
			if child.IsNamed() {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "cast_expression":
		return evalCast(ctx, node)

	case "as_expression":
		// (expr as Type) -> Type
		typeNode := node.ChildByFieldName("right")
		if typeNode == nil {
			// Try second named child
			if node.NamedChildCount() >= 2 {
				typeNode = node.NamedChild(1)
			}
		}
		if typeNode != nil {
			return parseTypeStub(typeNode, ctx.Content)
		}
		return typresolve.Unknown()

	case "is_expression":
		return typresolve.Named("System.Boolean")

	case "await_expression":
		return evalAwait(ctx, node)

	case "binary_expression":
		return evalBinary(ctx, node)

	case "assignment_expression":
		// Evaluate RHS
		right := node.ChildByFieldName("right")
		if right != nil {
			return EvalExprType(ctx, right)
		}
		return typresolve.Unknown()

	case "conditional_expression":
		// ternary: cond ? consequence : alternative -> consequence type
		consequence := node.ChildByFieldName("consequence")
		if consequence != nil {
			return EvalExprType(ctx, consequence)
		}
		return typresolve.Unknown()

	case "tuple_expression":
		return evalTuple(ctx, node)

	case "typeof_expression":
		return typresolve.Named("System.Type")

	case "sizeof_expression":
		return typresolve.Named("System.Int32")

	case "default_expression":
		// default(Type) -> Type
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() != "default" {
				return parseTypeStub(child, ctx.Content)
			}
		}
		return typresolve.Unknown()

	case "element_access_expression":
		return evalElementAccess(ctx, node)

	case "array_creation_expression":
		return evalArrayCreation(ctx, node)

	case "implicit_array_creation_expression":
		// new[] { ... } -- infer from first element
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.Type() == "initializer_expression" {
				if child.NamedChildCount() > 0 {
					elemType := EvalExprType(ctx, child.NamedChild(0))
					return typresolve.Slice(elemType)
				}
			}
		}
		return typresolve.Slice(typresolve.Unknown())

	case "lambda_expression", "anonymous_method_expression":
		return typresolve.Unknown()

	case "throw_expression":
		return typresolve.Unknown()

	case "checked_expression", "unchecked_expression":
		// Unwrap: checked(expr) -> eval inner
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "prefix_unary_expression", "postfix_unary_expression":
		return evalUnary(ctx, node)

	default:
		return typresolve.Unknown()
	}
}

// evalIdentifier resolves an identifier expression.
// 1. Scope lookup
// 2. Implicit this member lookup
// 3. Type name resolution (registry lookup)
func evalIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeContent(node, ctx.Content)

	// 1. Scope lookup.
	if t := ctx.Scope.Lookup(name); t != nil {
		return t
	}

	// 2. Implicit this member: look up as method/field on enclosing class.
	if ctx.EnclosingClassQN != "" {
		if m := lookupMethodStub(ctx.Registry, ctx.EnclosingClassQN, name); m != nil {
			if m.Signature != nil {
				return m.Signature
			}
		}
		// Check field
		if ft := lookupFieldStub(ctx.Registry, ctx.EnclosingClassQN, name); ft != nil {
			return ft
		}
	}

	// 3. Check if it is a registered type name.
	if t := ctx.Registry.LookupType(name); t != nil {
		return typresolve.Named(name)
	}

	// 4. Check as function in the module.
	if ctx.ModuleQN != "" {
		if f := ctx.Registry.LookupSymbol(ctx.ModuleQN, name); f != nil {
			if f.Signature != nil {
				return f.Signature
			}
		}
	}

	return typresolve.Unknown()
}

// evalInvocation resolves an invocation_expression.
// The function node can be an identifier, member_access, or conditional_access.
func evalInvocation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		// Try first named child
		if node.NamedChildCount() > 0 {
			fnNode = node.NamedChild(0)
		}
	}
	if fnNode == nil {
		return typresolve.Unknown()
	}

	// Check for builtin expression keywords.
	if fnNode.Type() == "identifier" {
		name := nodeContent(fnNode, ctx.Content)
		if isBuiltinFuncStub(name) {
			return evalBuiltinExprResult(name)
		}
	}

	// Evaluate function type.
	fnType := EvalExprType(ctx, fnNode)
	if fnType == nil {
		return typresolve.Unknown()
	}

	// If it is a KindFunc, extract return type.
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

	// If it is Named, could be a constructor call or type conversion.
	if fnType.Kind == typresolve.KindNamed {
		return fnType
	}

	return typresolve.Unknown()
}

// evalMemberAccess resolves a member_access_expression (e.g. obj.Member).
func evalMemberAccess(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	exprNode := node.ChildByFieldName("expression")
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return typresolve.Unknown()
	}

	memberName := nodeContent(nameNode, ctx.Content)
	// Strip generic type arguments from member name (e.g., Method<T> -> Method)
	if idx := strings.IndexByte(memberName, '<'); idx >= 0 {
		memberName = memberName[:idx]
	}

	// Handle `this.Member` and `base.Member` where this/base are unnamed tokens.
	if exprNode == nil {
		// Check for unnamed `this` or `base` as the first child.
		firstChild := node.Child(0)
		if firstChild != nil && !firstChild.IsNamed() {
			text := nodeContent(firstChild, ctx.Content)
			if text == "this" && ctx.EnclosingClassQN != "" {
				return lookupMemberOnType(ctx, ctx.EnclosingClassQN, memberName)
			}
			if text == "base" && ctx.EnclosingBaseQN != "" {
				return lookupMemberOnType(ctx, ctx.EnclosingBaseQN, memberName)
			}
		}
		return typresolve.Unknown()
	}

	// Evaluate the receiver expression.
	base := EvalExprType(ctx, exprNode)
	if base == nil || base.Kind == typresolve.KindUnknown {
		// If the expression is an identifier, try it as a type name for static access.
		if exprNode.Type() == "identifier" {
			typeName := nodeContent(exprNode, ctx.Content)
			if m := lookupMethodStub(ctx.Registry, typeName, memberName); m != nil {
				if m.Signature != nil {
					return m.Signature
				}
			}
			if ft := lookupFieldStub(ctx.Registry, typeName, memberName); ft != nil {
				return ft
			}
		}
		return typresolve.Unknown()
	}

	if base.Kind == typresolve.KindNamed {
		typeQN := base.Name
		// Look up method on the type.
		if m := lookupMethodStub(ctx.Registry, typeQN, memberName); m != nil {
			if m.Signature != nil {
				return m.Signature
			}
		}
		// Look up field on the type.
		if ft := lookupFieldStub(ctx.Registry, typeQN, memberName); ft != nil {
			return ft
		}
	}

	return typresolve.Unknown()
}

// evalConditionalAccess resolves x?.Member conditional access.
func evalConditionalAccess(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// First named child is the expression, second is the binding.
	if node.NamedChildCount() < 2 {
		return typresolve.Unknown()
	}
	exprNode := node.NamedChild(0)
	bindingNode := node.NamedChild(1)
	if exprNode == nil || bindingNode == nil {
		return typresolve.Unknown()
	}

	base := EvalExprType(ctx, exprNode)
	if base == nil || base.Kind != typresolve.KindNamed {
		return typresolve.Unknown()
	}

	// The binding is a member_binding_expression.
	if bindingNode.Type() == "member_binding_expression" {
		nameNode := bindingNode.ChildByFieldName("name")
		if nameNode == nil && bindingNode.NamedChildCount() > 0 {
			nameNode = bindingNode.NamedChild(0)
		}
		if nameNode != nil {
			memberName := nodeContent(nameNode, ctx.Content)
			if m := lookupMethodStub(ctx.Registry, base.Name, memberName); m != nil {
				if m.Signature != nil {
					return m.Signature
				}
			}
			if ft := lookupFieldStub(ctx.Registry, base.Name, memberName); ft != nil {
				return ft
			}
		}
	}

	return typresolve.Unknown()
}

// evalObjectCreation resolves `new Type(...)`.
func evalObjectCreation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		return parseTypeStub(typeNode, ctx.Content)
	}
	return typresolve.Unknown()
}

// evalCast resolves a cast_expression: (Type)expr -> Type.
func evalCast(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		return parseTypeStub(typeNode, ctx.Content)
	}
	return typresolve.Unknown()
}

// evalAwait resolves an await expression by evaluating the inner expression
// and unwrapping Task<T> to T.
func evalAwait(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// The inner expression is the first named child.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil {
			inner := EvalExprType(ctx, child)
			return unwrapTaskStub(inner)
		}
	}
	return typresolve.Unknown()
}

// evalBinary resolves a binary expression.
func evalBinary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// Check for comparison/logical operators.
	opNode := node.ChildByFieldName("operator")
	if opNode != nil {
		op := nodeContent(opNode, ctx.Content)
		switch op {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||", "is", "as":
			return typresolve.Named("System.Boolean")
		}
	} else {
		// Try to find operator by scanning unnamed children.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(int(i))
			if child != nil && !child.IsNamed() {
				op := nodeContent(child, ctx.Content)
				switch op {
				case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
					return typresolve.Named("System.Boolean")
				}
			}
		}
	}

	// Arithmetic/bitwise: return left operand type.
	left := node.ChildByFieldName("left")
	if left != nil {
		return EvalExprType(ctx, left)
	}
	// Fallback: first named child
	if node.NamedChildCount() > 0 {
		return EvalExprType(ctx, node.NamedChild(0))
	}
	return typresolve.Unknown()
}

// evalTuple resolves a tuple expression: (a, b, c).
func evalTuple(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	var elems []*typresolve.Type
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "argument" {
			// tuple arguments may have a nested expression
			if child.NamedChildCount() > 0 {
				elems = append(elems, EvalExprType(ctx, child.NamedChild(0)))
			} else {
				elems = append(elems, EvalExprType(ctx, child))
			}
		} else if child != nil {
			elems = append(elems, EvalExprType(ctx, child))
		}
	}
	if len(elems) == 0 {
		return typresolve.Unknown()
	}
	return typresolve.Tuple(elems)
}

// evalElementAccess resolves element_access_expression (e.g., dict[key], list[0]).
func evalElementAccess(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	exprNode := node.ChildByFieldName("expression")
	if exprNode == nil && node.NamedChildCount() > 0 {
		exprNode = node.NamedChild(0)
	}
	if exprNode == nil {
		return typresolve.Unknown()
	}

	base := EvalExprType(ctx, exprNode)
	if base == nil {
		return typresolve.Unknown()
	}

	switch base.Kind {
	case typresolve.KindSlice, typresolve.KindArray:
		if base.Elem != nil {
			return base.Elem
		}
	case typresolve.KindMap:
		if base.Value != nil {
			return base.Value
		}
	}

	return typresolve.Unknown()
}

// evalArrayCreation resolves `new Type[] { ... }` or `new Type[size]`.
func evalArrayCreation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		elemType := parseTypeStub(typeNode, ctx.Content)
		return typresolve.Slice(elemType)
	}
	return typresolve.Slice(typresolve.Unknown())
}

// evalUnary resolves prefix/postfix unary expressions.
func evalUnary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	operand := node.ChildByFieldName("operand")
	if operand == nil {
		// Try first named child
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil && child.IsNamed() {
				operand = child
				break
			}
		}
	}
	if operand == nil {
		return typresolve.Unknown()
	}

	// Check operator
	opNode := node.Child(0)
	if opNode != nil && !opNode.IsNamed() {
		op := nodeContent(opNode, ctx.Content)
		if op == "!" {
			return typresolve.Named("System.Boolean")
		}
	}

	return EvalExprType(ctx, operand)
}

// lookupMemberOnType is a helper that looks up a method or field on a type.
func lookupMemberOnType(ctx *ResolveContext, typeQN, memberName string) *typresolve.Type {
	if m := lookupMethodStub(ctx.Registry, typeQN, memberName); m != nil {
		if m.Signature != nil {
			return m.Signature
		}
	}
	if ft := lookupFieldStub(ctx.Registry, typeQN, memberName); ft != nil {
		return ft
	}
	return typresolve.Unknown()
}

// --- Integration wiring (wave 2) ---
// These functions delegate to the real implementations in methods.go,
// types.go, and builtins.go.

// lookupMethodStub delegates to LookupMethod (methods.go) with inheritance walking.
func lookupMethodStub(reg *typresolve.Registry, typeQN, methodName string) *typresolve.RegisteredFunc {
	return LookupMethod(reg, typeQN, methodName)
}

// lookupFieldStub delegates to LookupField (methods.go) with inheritance walking.
func lookupFieldStub(reg *typresolve.Registry, typeQN, fieldName string) *typresolve.Type {
	return LookupField(reg, typeQN, fieldName)
}

// parseTypeStub delegates to ParseTypeNode (types.go) with full type resolution.
// When called without a ResolveContext (from eval functions that only have
// node + content), uses empty namespace/usings and nil registry for basic parsing.
func parseTypeStub(node *sitter.Node, content []byte) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}
	result := ParseTypeNode(node, content, "", nil, nil)
	if result == nil {
		return typresolve.Unknown()
	}
	return result
}

// parseTypeWithContext delegates to ParseTypeNode with full resolution context.
func parseTypeWithContext(node *sitter.Node, ctx *ResolveContext) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}
	ns := ""
	if len(ctx.NamespaceStack) > 0 {
		ns = strings.Join(ctx.NamespaceStack, ".")
	}
	result := ParseTypeNode(node, ctx.Content, ns, ctx.Usings, ctx.Registry)
	if result == nil {
		return typresolve.Unknown()
	}
	return result
}

// isBuiltinFuncStub delegates to IsBuiltinFunc (builtins.go).
func isBuiltinFuncStub(name string) bool {
	return IsBuiltinFunc(name)
}

// evalBuiltinExprResult returns the result type for a C# builtin expression keyword.
func evalBuiltinExprResult(name string) *typresolve.Type {
	switch name {
	case "typeof":
		return typresolve.Named("System.Type")
	case "nameof":
		return typresolve.Named("System.String")
	case "sizeof":
		return typresolve.Named("System.Int32")
	case "default":
		return typresolve.Unknown()
	case "checked", "unchecked":
		return typresolve.Unknown()
	case "stackalloc":
		return typresolve.Unknown()
	}
	return typresolve.Unknown()
}

// unwrapTaskStub delegates to UnwrapTask (builtins.go).
func unwrapTaskStub(t *typresolve.Type) *typresolve.Type {
	if t == nil {
		return typresolve.Unknown()
	}
	return UnwrapTask(t)
}
