package javaresolve

import (
	"unicode"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveContext holds per-file state for Java type resolution.
type ResolveContext struct {
	Registry         *typresolve.Registry
	Scope            *typresolve.Scope
	Imports          map[string]string // className -> packagePath
	PkgQN            string           // current package qualified name (e.g., "org.apache.kafka.clients")
	Content          []byte           // source file content
	EnclosingFuncQN  string           // QN of enclosing method
	EnclosingClassQN string           // QN of enclosing class (for this/super resolution)
}

// nodeContent extracts the source text for a tree-sitter node.
func nodeContent(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a Java expression AST node using scope
// lookup, registry lookup, import resolution, and method dispatch.
func EvalExprType(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	nodeType := node.Type()

	// Check for literal types first.
	if lt := LiteralType(nodeType); lt != nil {
		return lt
	}

	switch nodeType {
	case "identifier":
		return evalIdentifier(ctx, node)

	case "this":
		return evalThis(ctx)

	case "super":
		return evalSuper(ctx)

	case "field_access":
		return evalFieldAccess(ctx, node)

	case "method_invocation":
		return evalMethodInvocation(ctx, node)

	case "object_creation_expression":
		return evalObjectCreation(ctx, node)

	case "cast_expression":
		return evalCast(ctx, node)

	case "ternary_expression", "conditional_expression":
		// Return the type of the true branch.
		consequence := node.ChildByFieldName("consequence")
		if consequence != nil {
			return EvalExprType(ctx, consequence)
		}
		return typresolve.Unknown()

	case "parenthesized_expression":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(int(i))
			if child.IsNamed() {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "array_creation_expression":
		return evalArrayCreation(ctx, node)

	case "array_access":
		return evalArrayAccess(ctx, node)

	case "binary_expression":
		return evalBinary(ctx, node)

	case "unary_expression":
		operand := node.ChildByFieldName("operand")
		if operand != nil {
			return EvalExprType(ctx, operand)
		}
		return typresolve.Unknown()

	case "instanceof_expression":
		return typresolve.Builtin("boolean")

	case "lambda_expression":
		return typresolve.Unknown()

	case "method_reference":
		return typresolve.Unknown()

	default:
		return typresolve.Unknown()
	}
}

// evalIdentifier resolves an identifier via scope, registry, imports, and builtins.
func evalIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeContent(node, ctx.Content)

	// 1. Scope lookup.
	if t := ctx.Scope.Lookup(name); t != nil {
		return t
	}

	// 2. Package-local type check.
	if ctx.Registry.LookupType(ctx.PkgQN+"."+name) != nil {
		return typresolve.Named(ctx.PkgQN + "." + name)
	}

	// 3. Import check.
	if pkg, ok := ctx.Imports[name]; ok {
		qn := pkg + "." + name
		if ctx.Registry.LookupType(qn) != nil {
			return typresolve.Named(qn)
		}
	}

	// 4. Builtin type check.
	if bt := ResolveBuiltinType(name); bt != nil {
		return bt
	}

	return typresolve.Unknown()
}

// evalThis resolves the "this" keyword to the enclosing class type.
func evalThis(ctx *ResolveContext) *typresolve.Type {
	if ctx.EnclosingClassQN != "" {
		return typresolve.Named(ctx.EnclosingClassQN)
	}
	return typresolve.Unknown()
}

// evalSuper resolves the "super" keyword to the superclass type.
func evalSuper(ctx *ResolveContext) *typresolve.Type {
	if ctx.EnclosingClassQN == "" {
		return typresolve.Unknown()
	}
	rt := ctx.Registry.LookupType(ctx.EnclosingClassQN)
	if rt == nil || len(rt.EmbeddedTypes) == 0 {
		return typresolve.Unknown()
	}
	return typresolve.Named(rt.EmbeddedTypes[0])
}

// evalFieldAccess handles field_access nodes (e.g., obj.field, ClassName.CONSTANT).
func evalFieldAccess(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	objectNode := node.ChildByFieldName("object")
	fieldNode := node.ChildByFieldName("field")
	if objectNode == nil || fieldNode == nil {
		return typresolve.Unknown()
	}

	fieldName := nodeContent(fieldNode, ctx.Content)

	// If object is an identifier, check for import or static class reference.
	if objectNode.Type() == "identifier" {
		objectName := nodeContent(objectNode, ctx.Content)

		// Check imports.
		if pkg, ok := ctx.Imports[objectName]; ok {
			if f := ctx.Registry.LookupSymbol(pkg+"."+objectName, fieldName); f != nil {
				return extractFuncReturnType(f)
			}
			if ft := LookupField(ctx.Registry, pkg+"."+objectName, fieldName); ft != nil {
				return ft
			}
		}

		// Check if uppercase (class name) for static field access.
		if len(objectName) > 0 && unicode.IsUpper(rune(objectName[0])) {
			// Try package-local.
			classQN := ctx.PkgQN + "." + objectName
			if ft := LookupField(ctx.Registry, classQN, fieldName); ft != nil {
				return ft
			}
			if f := LookupFieldOrMethod(ctx.Registry, classQN, fieldName); f != nil {
				return extractFuncReturnType(f)
			}
		}
	}

	// Evaluate the object type recursively.
	objType := EvalExprType(ctx, objectNode)
	if objType.Kind == typresolve.KindNamed {
		// Look up method first, then field.
		if f := LookupFieldOrMethod(ctx.Registry, objType.Name, fieldName); f != nil {
			return extractFuncReturnType(f)
		}
		if ft := LookupField(ctx.Registry, objType.Name, fieldName); ft != nil {
			return ft
		}
	}

	return typresolve.Unknown()
}

// evalMethodInvocation handles method_invocation nodes.
func evalMethodInvocation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	nameNode := node.ChildByFieldName("name")
	objectNode := node.ChildByFieldName("object")

	if nameNode == nil {
		return typresolve.Unknown()
	}

	methodName := nodeContent(nameNode, ctx.Content)

	// Skip builtin methods.
	if IsBuiltinFunc(methodName) {
		return typresolve.Unknown()
	}

	if objectNode == nil {
		// Simple method call (no receiver): check enclosing class or package-local.
		if ctx.EnclosingClassQN != "" {
			if f := LookupFieldOrMethod(ctx.Registry, ctx.EnclosingClassQN, methodName); f != nil {
				return extractFuncReturnType(f)
			}
		}
		// Package-local function lookup.
		if f := ctx.Registry.LookupFunc(ctx.PkgQN + "." + methodName); f != nil {
			return extractFuncReturnType(f)
		}
		return typresolve.Unknown()
	}

	// Object is present: check if it is an import alias.
	if objectNode.Type() == "identifier" {
		objectName := nodeContent(objectNode, ctx.Content)
		if pkg, ok := ctx.Imports[objectName]; ok {
			classQN := pkg + "." + objectName
			if f := LookupFieldOrMethod(ctx.Registry, classQN, methodName); f != nil {
				return extractFuncReturnType(f)
			}
			if f := ctx.Registry.LookupSymbol(classQN, methodName); f != nil {
				return extractFuncReturnType(f)
			}
		}
	}

	// Evaluate object type and look up method.
	objType := EvalExprType(ctx, objectNode)
	if objType.Kind == typresolve.KindNamed {
		if f := LookupFieldOrMethod(ctx.Registry, objType.Name, methodName); f != nil {
			return extractFuncReturnType(f)
		}
	}

	return typresolve.Unknown()
}

// evalObjectCreation handles object_creation_expression nodes (new ClassName()).
func evalObjectCreation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return typresolve.Unknown()
	}
	return ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
}

// evalCast handles cast_expression nodes ((Type) expr).
func evalCast(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return typresolve.Unknown()
	}
	return ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
}

// evalArrayCreation handles array_creation_expression nodes (new int[10]).
func evalArrayCreation(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return typresolve.Unknown()
	}
	elemType := ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	return typresolve.Slice(elemType)
}

// evalArrayAccess handles array_access nodes (arr[0]).
func evalArrayAccess(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	arrayNode := node.ChildByFieldName("array")
	if arrayNode == nil {
		return typresolve.Unknown()
	}
	arrType := EvalExprType(ctx, arrayNode)
	if arrType == nil {
		return typresolve.Unknown()
	}
	switch arrType.Kind {
	case typresolve.KindSlice, typresolve.KindArray:
		if arrType.Elem != nil {
			return arrType.Elem
		}
	case typresolve.KindMap:
		if arrType.Value != nil {
			return arrType.Value
		}
	}
	return typresolve.Unknown()
}

// evalBinary handles binary_expression nodes.
func evalBinary(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	// Check the operator for comparison/logical.
	op := ""
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(int(i))
		if !child.IsNamed() {
			text := nodeContent(child, ctx.Content)
			switch text {
			case "==", "!=", "<", ">", "<=", ">=", "&&", "||", "instanceof":
				return typresolve.Builtin("boolean")
			default:
				op = text
			}
		}
	}
	_ = op

	// Non-comparison: return left operand type.
	left := node.ChildByFieldName("left")
	if left != nil {
		return EvalExprType(ctx, left)
	}
	return typresolve.Unknown()
}

// extractFuncReturnType extracts the return type from a RegisteredFunc.
// Returns the first return type if the signature has KindFunc with Returns.
func extractFuncReturnType(f *typresolve.RegisteredFunc) *typresolve.Type {
	if f == nil || f.Signature == nil {
		return typresolve.Unknown()
	}
	if f.Signature.Kind == typresolve.KindFunc && len(f.Signature.Returns) > 0 {
		return f.Signature.Returns[0]
	}
	return typresolve.Unknown()
}
