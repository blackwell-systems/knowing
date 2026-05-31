package rubyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveContext holds per-file state for type resolution.
type ResolveContext struct {
	Registry        *typresolve.Registry
	Scope           *typresolve.Scope
	Requires        map[string]string // local binding -> module path
	Nesting         []string          // current module/class nesting stack
	CurrentFile     string            // current file path
	Content         []byte            // source file content
	EnclosingFuncQN string            // QN of the current method being resolved
}

// nodeText extracts the text of a tree-sitter node from the source content.
func nodeText(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a Ruby expression AST node using
// scope lookup, registry lookup, constant resolution, and method dispatch.
func EvalExprType(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	nodeType := node.Type()

	switch nodeType {
	// Literals
	case "string", "string_array", "heredoc_body":
		return typresolve.Named("Ruby::String")
	case "integer":
		return typresolve.Named("Ruby::Integer")
	case "float":
		return typresolve.Named("Ruby::Float")
	case "symbol", "simple_symbol":
		return typresolve.Named("Ruby::Symbol")
	case "true":
		return typresolve.Named("Ruby::TrueClass")
	case "false":
		return typresolve.Named("Ruby::FalseClass")
	case "nil":
		return typresolve.Named("Ruby::NilClass")
	case "array":
		return typresolve.Named("Ruby::Array")
	case "hash":
		return typresolve.Named("Ruby::Hash")
	case "regex":
		return typresolve.Named("Ruby::Regexp")
	case "range":
		return typresolve.Named("Ruby::Range")
	case "lambda":
		return typresolve.Named("Ruby::Proc")

	case "identifier":
		return evalIdentifier(ctx, node)

	case "constant":
		return evalConstant(ctx, node)

	case "scope_resolution":
		return evalScopeResolution(ctx, node)

	case "call":
		return evalCall(ctx, node)

	case "parenthesized_statements":
		return evalParenthesized(ctx, node)

	case "conditional", "if", "unless":
		return evalConditional(ctx, node)

	case "binary":
		return evalBinary(ctx, node)

	case "unary":
		return evalUnary(ctx, node)

	case "element_reference":
		return evalElementReference(ctx, node)

	case "assignment":
		return evalAssignmentExpr(ctx, node)

	case "method":
		// Method definition as expression returns a symbol in Ruby.
		return typresolve.Named("Ruby::Symbol")

	default:
		return typresolve.Unknown()
	}
}

// evalIdentifier resolves an identifier through scope lookup, builtins,
// and local method lookup.
func evalIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeText(node, ctx.Content)

	// Check scope first.
	if t := ctx.Scope.Lookup(name); t != nil && !t.IsUnknown() {
		return t
	}

	// Check builtin function.
	if IsBuiltinFunc(name) {
		return typresolve.Unknown()
	}

	// Check local method in current class/module.
	if len(ctx.Nesting) > 0 {
		currentClassQN := ctx.Nesting[len(ctx.Nesting)-1]
		if f := ctx.Registry.LookupMethod(currentClassQN, name); f != nil {
			if f.Signature != nil && len(f.Signature.Returns) > 0 {
				return f.Signature.Returns[0]
			}
			return typresolve.Unknown()
		}
	}

	return typresolve.Unknown()
}

// evalConstant resolves a Ruby constant reference (ClassName, ModuleName).
func evalConstant(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeText(node, ctx.Content)

	// Check builtin type first.
	if bt := ResolveBuiltinType(name); bt != nil {
		return bt
	}

	// Resolve constant in nesting context.
	resolved := ResolveConstant(name, ctx.Nesting)

	// Look up in registry.
	if ctx.Registry.LookupType(resolved) != nil {
		return typresolve.Named(resolved)
	}

	return typresolve.Named(name)
}

// evalScopeResolution handles A::B::C scope resolution nodes.
func evalScopeResolution(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	qn := ParseScopeResolution(node, ctx.Content)

	if ctx.Registry.LookupType(qn) != nil {
		return typresolve.Named(qn)
	}

	return typresolve.Named(qn)
}

// evalCall handles method calls (both bare and with receiver).
func evalCall(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	methodNode := node.ChildByFieldName("method")
	receiver := node.ChildByFieldName("receiver")
	args := node.ChildByFieldName("arguments")

	var methodName string
	if methodNode != nil {
		methodName = nodeText(methodNode, ctx.Content)
	}

	if receiver == nil {
		return evalBareCall(ctx, methodName, args)
	}

	return evalReceiverCall(ctx, receiver, methodName)
}

// evalBareCall handles calls without a receiver (puts, require, local methods).
func evalBareCall(ctx *ResolveContext, methodName string, args *sitter.Node) *typresolve.Type {
	// Check builtin function.
	if IsBuiltinFunc(methodName) {
		return EvalBuiltinCall(methodName, args, ctx.Content, nil)
	}

	// Check method in current class.
	if len(ctx.Nesting) > 0 {
		currentClassQN := ctx.Nesting[len(ctx.Nesting)-1]
		if f := LookupAttribute(ctx.Registry, currentClassQN, methodName); f != nil {
			if f.Signature != nil && len(f.Signature.Returns) > 0 {
				return f.Signature.Returns[0]
			}
			return typresolve.Unknown()
		}
	}

	// Check package-level function.
	if f := ctx.Registry.LookupFunc(methodName); f != nil {
		if f.Signature != nil && len(f.Signature.Returns) > 0 {
			return f.Signature.Returns[0]
		}
		return typresolve.Unknown()
	}

	return typresolve.Unknown()
}

// evalReceiverCall handles calls with a receiver (obj.method, Class.new).
func evalReceiverCall(ctx *ResolveContext, receiver *sitter.Node, methodName string) *typresolve.Type {
	// Check for Class.new (constructor).
	if receiver.Type() == "constant" && methodName == "new" {
		constName := nodeText(receiver, ctx.Content)
		if bt := ResolveBuiltinType(constName); bt != nil {
			return bt
		}
		resolved := ResolveConstant(constName, ctx.Nesting)
		return typresolve.Named(resolved)
	}

	// Evaluate receiver type recursively.
	recvType := EvalExprType(ctx, receiver)

	// If receiver is a named type, try method lookup.
	if recvType.Kind == typresolve.KindNamed {
		typeQN := recvType.Name
		if f := LookupAttribute(ctx.Registry, typeQN, methodName); f != nil {
			if f.Signature != nil && len(f.Signature.Returns) > 0 {
				return f.Signature.Returns[0]
			}
		}
	}

	// Handle common Ruby method return types by convention.
	return rubyMethodReturnType(methodName, recvType)
}

// rubyMethodReturnType returns the conventional return type for well-known
// Ruby method names. Returns Unknown if the method is not recognized.
func rubyMethodReturnType(methodName string, recvType *typresolve.Type) *typresolve.Type {
	switch methodName {
	// String conversions
	case "to_s", "inspect", "to_str":
		return typresolve.Named("Ruby::String")

	// Integer conversions
	case "to_i", "to_int":
		return typresolve.Named("Ruby::Integer")

	// Float conversion
	case "to_f":
		return typresolve.Named("Ruby::Float")

	// Array conversions
	case "to_a", "to_ary", "entries":
		return typresolve.Named("Ruby::Array")

	// Hash conversion
	case "to_h":
		return typresolve.Named("Ruby::Hash")

	// Symbol conversion
	case "to_sym":
		return typresolve.Named("Ruby::Symbol")

	// Rational/Complex
	case "to_r":
		return typresolve.Named("Ruby::Rational")
	case "to_c":
		return typresolve.Named("Ruby::Complex")

	// Boolean predicates
	case "nil?", "empty?", "include?", "respond_to?", "is_a?", "kind_of?",
		"instance_of?", "frozen?", "equal?", "eql?", "==", "!=", "===":
		return typresolve.Named("Ruby::TrueClass")

	// Integer-returning methods
	case "size", "length", "count", "index", "rindex", "object_id", "hash":
		return typresolve.Named("Ruby::Integer")

	// Class method
	case "class":
		return typresolve.Named("Ruby::Class")

	// Self-returning methods
	case "freeze", "dup", "clone", "tap", "then", "yield_self":
		return recvType

	// Collection methods returning Array
	case "map", "collect", "select", "filter", "reject", "flat_map",
		"sort", "sort_by", "min_by", "max_by", "group_by", "each_with_object":
		return typresolve.Named("Ruby::Array")

	// Enumeration (returns self for chaining)
	case "each", "each_with_index", "each_slice", "each_cons":
		return recvType

	// Element access (type unknown without generics)
	case "first", "last", "min", "max", "sample", "find", "detect":
		return typresolve.Unknown()

	// Hash-specific
	case "keys", "values":
		return typresolve.Named("Ruby::Array")
	case "merge", "update":
		return recvType

	default:
		return typresolve.Unknown()
	}
}

// evalParenthesized evaluates a parenthesized expression by returning
// the type of the last child expression (Ruby returns last expression value).
func evalParenthesized(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	count := int(node.NamedChildCount())
	if count == 0 {
		return typresolve.Unknown()
	}
	lastChild := node.NamedChild(count - 1)
	return EvalExprType(ctx, lastChild)
}

// evalConditional evaluates if/unless/conditional by returning the
// consequence branch type.
func evalConditional(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	consequence := node.ChildByFieldName("consequence")
	if consequence != nil {
		return EvalExprType(ctx, consequence)
	}
	// Try first named child after condition for inline if.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "then" {
			// Evaluate first child of then block.
			if child.NamedChildCount() > 0 {
				return EvalExprType(ctx, child.NamedChild(0))
			}
		}
	}
	return typresolve.Unknown()
}

// evalBinary evaluates binary operations.
func evalBinary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// Get operator (middle child).
	if node.ChildCount() < 3 {
		return typresolve.Unknown()
	}

	op := nodeText(node.Child(1), ctx.Content)
	left := node.ChildByFieldName("left")
	if left == nil && node.NamedChildCount() >= 1 {
		left = node.NamedChild(0)
	}

	switch op {
	// Comparison operators
	case "==", "!=", "<", ">", "<=", ">=", "===":
		return typresolve.Named("Ruby::TrueClass")
	case "<=>":
		return typresolve.Named("Ruby::Integer")

	// Arithmetic operators: return left operand type
	case "+", "-", "*", "/", "%", "**":
		if left != nil {
			leftType := EvalExprType(ctx, left)
			if leftType.Kind == typresolve.KindNamed && leftType.Name == "Ruby::String" {
				return typresolve.Named("Ruby::String")
			}
			return leftType
		}
		return typresolve.Unknown()

	// Logical operators: could be either branch
	case "&&", "||", "and", "or":
		return typresolve.Unknown()

	default:
		return typresolve.Unknown()
	}
}

// evalUnary evaluates unary operations.
func evalUnary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node.ChildCount() < 2 {
		return typresolve.Unknown()
	}

	op := nodeText(node.Child(0), ctx.Content)

	switch op {
	case "!", "not":
		return typresolve.Named("Ruby::TrueClass")
	case "-", "+":
		// Return operand type.
		if node.NamedChildCount() > 0 {
			return EvalExprType(ctx, node.NamedChild(0))
		}
		return typresolve.Unknown()
	default:
		return typresolve.Unknown()
	}
}

// evalElementReference evaluates array/hash indexing (obj[key]).
func evalElementReference(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node.NamedChildCount() == 0 {
		return typresolve.Unknown()
	}

	recvType := EvalExprType(ctx, node.NamedChild(0))
	if recvType.Kind == typresolve.KindNamed {
		switch recvType.Name {
		case "Ruby::String":
			return typresolve.Named("Ruby::String")
		}
	}
	// Array/Hash indexing: element type unknown without generics.
	return typresolve.Unknown()
}

// evalAssignmentExpr evaluates assignment as an expression (returns the
// right-hand side type for chained assignments).
func evalAssignmentExpr(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node.NamedChildCount() < 2 {
		return typresolve.Unknown()
	}
	// Right side is the last named child.
	right := node.NamedChild(int(node.NamedChildCount()) - 1)
	return EvalExprType(ctx, right)
}
