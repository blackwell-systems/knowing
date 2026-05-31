package pyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveContext holds per-file state for Python type resolution.
type ResolveContext struct {
	Registry         *typresolve.Registry
	Scope            *typresolve.Scope
	Imports          map[string]ImportInfo // from imports.go (Agent A)
	ModuleQN         string               // current module qualified name
	Content          []byte               // source file content
	EnclosingFuncQN  string               // QN of enclosing function
	EnclosingClassQN string               // QN of enclosing class (for self/cls)
}

// nodeText extracts the text of a tree-sitter node from the source content.
func nodeText(node *sitter.Node, content []byte) string {
	return node.Content(content)
}

// EvalExprType evaluates the type of a Python expression AST node using
// scope lookup, registry lookup, import resolution, and attribute dispatch.
// This is the Python port of py_eval_expr_type from the C reference.
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
	case "tuple":
		return evalTuple(ctx, node)
	case "list":
		return evalList(ctx, node)
	case "dictionary":
		return evalDict(ctx, node)
	case "set":
		return evalSet(ctx, node)

	case "identifier":
		return evalIdentifier(ctx, node)

	case "attribute":
		return evalAttribute(ctx, node)

	case "call":
		return evalCall(ctx, node)

	case "named_expression":
		// Walrus operator: (name := expr) evaluates to the expr's type
		// and binds name in the enclosing scope.
		return evalWalrus(ctx, node)

	case "binary_operator":
		// Best-effort: return left operand type.
		left := node.ChildByFieldName("left")
		if left != nil {
			return EvalExprType(ctx, left)
		}
		return typresolve.Unknown()

	case "comparison_operator", "boolean_operator", "not_operator":
		return typresolve.Builtin("bool")

	case "conditional_expression":
		// a if cond else b: evaluate a, return it (simplified).
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "parenthesized_expression":
		// Unwrap parentheses.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "await", "await_expression":
		// Unwrap await (treat as identity for type purposes).
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(int(i))
			if child != nil {
				return EvalExprType(ctx, child)
			}
		}
		return typresolve.Unknown()

	case "subscript":
		return evalSubscript(ctx, node)

	default:
		return typresolve.Unknown()
	}
}

// evalTuple evaluates a tuple literal expression.
func evalTuple(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	count := int(node.NamedChildCount())
	if count == 0 {
		return typresolve.Builtin("tuple")
	}
	elems := make([]*typresolve.Type, 0, count)
	for i := 0; i < count; i++ {
		child := node.NamedChild(int(i))
		elems = append(elems, EvalExprType(ctx, child))
	}
	return typresolve.Tuple(elems)
}

// evalList evaluates a list literal expression.
func evalList(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node.NamedChildCount() == 0 {
		return typresolve.Builtin("list")
	}
	firstElem := node.NamedChild(0)
	elemType := EvalExprType(ctx, firstElem)
	return typresolve.Slice(elemType)
}

// evalDict evaluates a dictionary literal expression.
func evalDict(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node.NamedChildCount() == 0 {
		return typresolve.Builtin("dict")
	}
	// Find the first pair child.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child != nil && child.Type() == "pair" {
			keyNode := child.ChildByFieldName("key")
			valueNode := child.ChildByFieldName("value")
			keyType := typresolve.Unknown()
			valueType := typresolve.Unknown()
			if keyNode != nil {
				keyType = EvalExprType(ctx, keyNode)
			}
			if valueNode != nil {
				valueType = EvalExprType(ctx, valueNode)
			}
			return typresolve.Map(keyType, valueType)
		}
	}
	return typresolve.Builtin("dict")
}

// evalSet evaluates a set literal expression.
func evalSet(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	if node.NamedChildCount() == 0 {
		return typresolve.Builtin("set")
	}
	return typresolve.Named("builtins.set")
}

// evalIdentifier resolves an identifier expression in Python.
func evalIdentifier(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	name := nodeText(node, ctx.Content)

	// 1. Scope lookup.
	if t := ctx.Scope.Lookup(name); t != nil {
		return t
	}

	// 2. Check True/False/None.
	switch name {
	case "True", "False":
		return typresolve.Builtin("bool")
	case "None":
		return typresolve.Builtin("None")
	}

	// 3. Module-local function from registry.
	if f := ctx.Registry.LookupSymbol(ctx.ModuleQN, name); f != nil {
		if f.Signature != nil {
			return f.Signature
		}
	}

	// 4. Builtins fallback: look up as builtin function.
	if f := ctx.Registry.LookupSymbol("builtins", name); f != nil {
		if f.Signature != nil {
			return f.Signature
		}
	}

	// 5. Builtin type check.
	if t := ctx.Registry.LookupType("builtins." + name); t != nil {
		return typresolve.Named("builtins." + name)
	}

	// 6. Builtin type via helper.
	if t := ResolveBuiltinType(name); t != nil {
		return t
	}

	return typresolve.Unknown()
}

// evalAttribute resolves an attribute access expression (e.g. obj.attr).
func evalAttribute(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	objNode := node.ChildByFieldName("object")
	attrNode := node.ChildByFieldName("attribute")
	if objNode == nil || attrNode == nil {
		return typresolve.Unknown()
	}

	attrName := nodeText(attrNode, ctx.Content)

	// Check if the object is an import (module access).
	if objNode.Type() == "identifier" {
		objName := nodeText(objNode, ctx.Content)
		if info, ok := ResolveImport(ctx.Imports, objName); ok && !info.IsFromStyle {
			// This is a module access: e.g. os.path, django.db
			modulePath := info.ModulePath

			// Look up symbol in registry.
			if f := ctx.Registry.LookupSymbol(modulePath, attrName); f != nil {
				if f.Signature != nil {
					return f.Signature
				}
			}
			// Look up as type.
			typeQN := modulePath + "." + attrName
			if t := ctx.Registry.LookupType(typeQN); t != nil {
				return typresolve.Named(typeQN)
			}
			// Could be a sub-module; return as named.
			return typresolve.Named(typeQN)
		}
	}

	// Evaluate object type recursively.
	objType := EvalExprType(ctx, objNode)
	if objType == nil {
		return typresolve.Unknown()
	}

	switch objType.Kind {
	case typresolve.KindNamed:
		typeQN := objType.Name

		// Check if this is a module path from imports.
		if isModuleFromImports(ctx, typeQN) {
			if f := ctx.Registry.LookupSymbol(typeQN, attrName); f != nil {
				if f.Signature != nil {
					return f.Signature
				}
			}
			newQN := typeQN + "." + attrName
			if t := ctx.Registry.LookupType(newQN); t != nil {
				return typresolve.Named(newQN)
			}
			return typresolve.Named(newQN)
		}

		// Look up method via LookupAttribute (follows MRO).
		if m := LookupAttribute(ctx.Registry, typeQN, attrName); m != nil {
			if m.Signature != nil {
				return substituteSelf(m.Signature, typeQN)
			}
		}

		// Look up field.
		if ft := LookupField(ctx.Registry, typeQN, attrName); ft != nil {
			return ft
		}

	case typresolve.KindBuiltin:
		// Look up method on "builtins.<typename>".
		builtinQN := "builtins." + objType.Name
		if m := LookupAttribute(ctx.Registry, builtinQN, attrName); m != nil {
			if m.Signature != nil {
				return m.Signature
			}
		}

	case typresolve.KindOptional:
		// Optional[T] union dispatch: try T for attribute access.
		inner := objType.Elem
		if inner != nil && inner.Kind == typresolve.KindNamed {
			if m := LookupAttribute(ctx.Registry, inner.Name, attrName); m != nil {
				if m.Signature != nil {
					return substituteSelf(m.Signature, inner.Name)
				}
			}
			if ft := LookupField(ctx.Registry, inner.Name, attrName); ft != nil {
				return ft
			}
		}
	}

	return typresolve.Unknown()
}

// isModuleFromImports checks if a given qualified name corresponds to
// a module path in the import map.
func isModuleFromImports(ctx *ResolveContext, qn string) bool {
	for _, info := range ctx.Imports {
		if info.ModulePath == qn && !info.IsFromStyle {
			return true
		}
	}
	return false
}

// evalCall resolves a call expression in Python.
func evalCall(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return typresolve.Unknown()
	}

	switch fnNode.Type() {
	case "identifier":
		return evalCallIdentifier(ctx, fnNode)

	case "attribute":
		return evalCallAttribute(ctx, fnNode)
	}

	// Generic: evaluate function expression, extract return type.
	fnType := EvalExprType(ctx, fnNode)
	return extractReturnType(fnType)
}

// evalCallIdentifier resolves a call where the function is an identifier.
func evalCallIdentifier(ctx *ResolveContext, fnNode *sitter.Node) *typresolve.Type {
	name := nodeText(fnNode, ctx.Content)

	// a. Check if name is in scope as Named type (constructor call).
	if t := ctx.Scope.Lookup(name); t != nil {
		if t.Kind == typresolve.KindNamed {
			return t // Instance creation returns the type itself.
		}
		// If it's a function in scope, extract return type.
		return extractReturnType(t)
	}

	// b. Module-local function.
	if f := ctx.Registry.LookupSymbol(ctx.ModuleQN, name); f != nil {
		return extractFuncReturnType(f)
	}

	// c. Builtins function.
	if f := ctx.Registry.LookupSymbol("builtins", name); f != nil {
		return extractFuncReturnType(f)
	}

	// d. Builtin type constructor: int(), str(), etc.
	if IsBuiltinType(name) {
		return typresolve.Builtin(name)
	}

	// e. Check registry for type (class call = constructor).
	if t := ctx.Registry.LookupType(ctx.ModuleQN + "." + name); t != nil {
		return typresolve.Named(ctx.ModuleQN + "." + name)
	}

	return typresolve.Unknown()
}

// evalCallAttribute resolves a call where the function is an attribute access.
func evalCallAttribute(ctx *ResolveContext, fnNode *sitter.Node) *typresolve.Type {
	objNode := fnNode.ChildByFieldName("object")
	attrNode := fnNode.ChildByFieldName("attribute")
	if objNode == nil || attrNode == nil {
		return typresolve.Unknown()
	}

	attrName := nodeText(attrNode, ctx.Content)

	// Check if object is a module import.
	if objNode.Type() == "identifier" {
		objName := nodeText(objNode, ctx.Content)
		if info, ok := ResolveImport(ctx.Imports, objName); ok && !info.IsFromStyle {
			modulePath := info.ModulePath
			// Look up module.method in registry.
			if f := ctx.Registry.LookupSymbol(modulePath, attrName); f != nil {
				return extractFuncReturnType(f)
			}
			// Could be a type constructor.
			typeQN := modulePath + "." + attrName
			if ctx.Registry.LookupType(typeQN) != nil {
				return typresolve.Named(typeQN)
			}
		}
	}

	// super().method() resolution: resolve against the first base class.
	if objNode.Type() == "call" {
		if ret := evalSuperCall(ctx, objNode, attrName); ret != nil {
			return ret
		}
	}

	// Evaluate object type.
	objType := EvalExprType(ctx, objNode)
	if objType == nil {
		return typresolve.Unknown()
	}

	// Container method special cases: dict.items/keys/values/get with
	// parameterized types.
	if objType.Kind == typresolve.KindMap {
		if ret := evalDictMethodCall(objType, attrName); ret != nil {
			return ret
		}
	}

	if objType.Kind == typresolve.KindNamed {
		typeQN := objType.Name
		// Look up method via LookupAttribute.
		if m := LookupAttribute(ctx.Registry, typeQN, attrName); m != nil {
			ret := extractFuncReturnType(m)
			// typing.Self substitution: if return type is "Self", substitute
			// the receiver class.
			return substituteSelf(ret, typeQN)
		}
	}

	if objType.Kind == typresolve.KindBuiltin {
		builtinQN := "builtins." + objType.Name
		if m := LookupAttribute(ctx.Registry, builtinQN, attrName); m != nil {
			return extractFuncReturnType(m)
		}
	}

	// Union type dispatch: try each Named member; if exactly one resolves,
	// use that result.
	if objType.Kind == typresolve.KindOptional {
		// Optional[T] is Union[T, None]; try T.
		inner := objType.Elem
		if inner != nil && inner.Kind == typresolve.KindNamed {
			if m := LookupAttribute(ctx.Registry, inner.Name, attrName); m != nil {
				ret := extractFuncReturnType(m)
				return substituteSelf(ret, inner.Name)
			}
		}
	}

	return typresolve.Unknown()
}

// evalSuperCall resolves super().method() to the parent class method's return
// type. Returns nil if this is not a super() call pattern.
func evalSuperCall(ctx *ResolveContext, callNode *sitter.Node, attrName string) *typresolve.Type {
	fnNode := callNode.ChildByFieldName("function")
	if fnNode == nil || fnNode.Type() != "identifier" {
		return nil
	}
	name := nodeText(fnNode, ctx.Content)
	if name != "super" {
		return nil
	}
	if ctx.EnclosingClassQN == "" {
		return nil
	}

	// Find the enclosing class in the registry and walk its bases.
	rt := ctx.Registry.LookupType(ctx.EnclosingClassQN)
	if rt == nil || len(rt.EmbeddedTypes) == 0 {
		return nil
	}

	for _, baseQN := range rt.EmbeddedTypes {
		if m := LookupAttribute(ctx.Registry, baseQN, attrName); m != nil {
			ret := extractFuncReturnType(m)
			return substituteSelf(ret, ctx.EnclosingClassQN)
		}
		// Special case: super().__init__
		if attrName == "__init__" {
			initQN := baseQN + ".__init__"
			if f := ctx.Registry.LookupFunc(initQN); f != nil {
				return extractFuncReturnType(f)
			}
			// __init__ always returns None
			return typresolve.Builtin("None")
		}
	}
	return nil
}

// evalDictMethodCall handles dict.items/keys/values/get/pop returning
// parameterized types based on the map's key/value types.
func evalDictMethodCall(mapType *typresolve.Type, methodName string) *typresolve.Type {
	keyType := mapType.Key
	valType := mapType.Value
	if keyType == nil {
		keyType = typresolve.Unknown()
	}
	if valType == nil {
		valType = typresolve.Unknown()
	}

	switch methodName {
	case "items":
		// Returns an iterable of (K, V) tuples.
		return typresolve.Slice(typresolve.Tuple([]*typresolve.Type{keyType, valType}))
	case "keys":
		return typresolve.Slice(keyType)
	case "values":
		return typresolve.Slice(valType)
	case "get":
		// dict.get(k) -> Optional[V]
		return typresolve.Optional(valType)
	case "pop":
		return valType
	case "copy":
		return mapType
	case "setdefault":
		return valType
	}
	return nil
}

// substituteSelf replaces a "Self" named type with the receiver's qualified
// name. Handles direct Self, Optional[Self], etc.
func substituteSelf(t *typresolve.Type, receiverQN string) *typresolve.Type {
	if t == nil {
		return t
	}
	if t.Kind == typresolve.KindNamed {
		if t.Name == "Self" || t.Name == "typing.Self" || t.Name == "typing_extensions.Self" {
			return typresolve.Named(receiverQN)
		}
	}
	if t.Kind == typresolve.KindOptional && t.Elem != nil {
		inner := substituteSelf(t.Elem, receiverQN)
		if inner != t.Elem {
			return typresolve.Optional(inner)
		}
	}
	return t
}

// extractReturnType extracts the return type from a function type.
// For Python, functions typically have a single return type.
func extractReturnType(t *typresolve.Type) *typresolve.Type {
	if t == nil {
		return typresolve.Unknown()
	}
	if t.Kind == typresolve.KindFunc {
		if len(t.Returns) == 0 {
			return typresolve.Unknown()
		}
		if len(t.Returns) == 1 {
			return t.Returns[0]
		}
		return typresolve.Tuple(t.Returns)
	}
	// If it's a Named type, calling it is a constructor.
	if t.Kind == typresolve.KindNamed {
		return t
	}
	return typresolve.Unknown()
}

// extractFuncReturnType extracts the return type from a RegisteredFunc.
func extractFuncReturnType(f *typresolve.RegisteredFunc) *typresolve.Type {
	if f == nil || f.Signature == nil {
		return typresolve.Unknown()
	}
	return extractReturnType(f.Signature)
}

// evalWalrus evaluates a walrus operator (named_expression): name := expr.
// Binds the name in the enclosing scope and returns the expression type.
func evalWalrus(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		// Try "left" field as alternative.
		nameNode = node.ChildByFieldName("left")
	}
	valueNode := node.ChildByFieldName("value")
	if valueNode == nil {
		valueNode = node.ChildByFieldName("right")
	}
	if valueNode == nil {
		return typresolve.Unknown()
	}

	valType := EvalExprType(ctx, valueNode)

	// Bind in enclosing scope.
	if nameNode != nil && nameNode.Type() == "identifier" {
		name := nodeText(nameNode, ctx.Content)
		if name != "" && name != "_" {
			ctx.Scope.Bind(name, valType)
		}
	}

	return valType
}

// evalSubscript resolves a subscript expression (e.g. x[0], d["key"]).
func evalSubscript(ctx *ResolveContext, node *sitter.Node) *typresolve.Type {
	valueNode := node.ChildByFieldName("value")
	if valueNode == nil {
		return typresolve.Unknown()
	}

	containerType := EvalExprType(ctx, valueNode)
	if containerType == nil {
		return typresolve.Unknown()
	}

	switch containerType.Kind {
	case typresolve.KindMap:
		if containerType.Value != nil {
			return containerType.Value
		}
	case typresolve.KindSlice:
		if containerType.Elem != nil {
			return containerType.Elem
		}
	case typresolve.KindTuple:
		// Try to get the index for positional access.
		subscriptNode := node.ChildByFieldName("subscript")
		if subscriptNode != nil && subscriptNode.Type() == "integer" {
			idxText := nodeText(subscriptNode, ctx.Content)
			idx := 0
			for _, ch := range idxText {
				if ch >= '0' && ch <= '9' {
					idx = idx*10 + int(ch-'0')
				}
			}
			if idx < len(containerType.Elements) {
				return containerType.Elements[idx]
			}
		}
		// If we can't determine the index, return Unknown.
		return typresolve.Unknown()
	}

	return typresolve.Unknown()
}
