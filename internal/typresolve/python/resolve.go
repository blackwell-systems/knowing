package pyresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// resolvedCall holds the result of resolving a call expression.
type resolvedCall struct {
	calleeQN string
	strategy string // "resolver_direct", "resolver_type_dispatch", "resolver_import"
}

// ResolveCallsInFile walks a Python file's AST resolving call expressions
// and emitting edges. Uses the shared registry, per-file scope chain, and
// import map.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	var edges []types.Edge

	// Pass 1: Process module-level assignments into root scope.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "assignment", "expression_statement":
			ProcessStatement(ctx, child)
		}
	}

	// Pass 2: Process function and class definitions.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "function_definition":
			processFuncDef(ctx, child, &edges, fileHash, repoURL, filePath)
		case "class_definition":
			processClassDef(ctx, child, &edges, fileHash, repoURL, filePath)
		case "decorated_definition":
			// Unwrap decorated definition.
			defNode := child.ChildByFieldName("definition")
			if defNode == nil {
				// Try last named child as fallback.
				if child.NamedChildCount() > 0 {
					defNode = child.NamedChild(int(child.NamedChildCount()) - 1)
				}
			}
			if defNode != nil {
				switch defNode.Type() {
				case "function_definition":
					processFuncDef(ctx, defNode, &edges, fileHash, repoURL, filePath)
				case "class_definition":
					processClassDef(ctx, defNode, &edges, fileHash, repoURL, filePath)
				}
			}
		}
	}

	return edges
}

// processFuncDef processes a top-level or nested function definition.
func processFuncDef(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := nodeText(nameNode, ctx.Content)

	// Build the function's qualified name.
	var funcQN string
	if ctx.EnclosingClassQN != "" {
		funcQN = ctx.EnclosingClassQN + "." + funcName
	} else {
		funcQN = ctx.ModuleQN + "." + funcName
	}

	origFuncQN := ctx.EnclosingFuncQN
	ctx.EnclosingFuncQN = funcQN

	// Push function scope.
	funcScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = funcScope

	// Check if this is a static method (no self/cls binding).
	isStaticMethod := isDecorated(node, "staticmethod", ctx.Content)

	// Bind parameters into scope.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindPythonParams(ctx, paramsNode, isStaticMethod)
	}

	// Walk function body resolving calls.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop scope.
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = origFuncQN
}

// processClassDef processes a class definition.
func processClassDef(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	className := nodeText(nameNode, ctx.Content)
	classQN := ctx.ModuleQN + "." + className

	origClassQN := ctx.EnclosingClassQN
	ctx.EnclosingClassQN = classQN

	// Push class scope.
	classScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = classScope

	// Process class body: look for methods and nested classes.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			if child == nil {
				continue
			}
			switch child.Type() {
			case "function_definition":
				processFuncDef(ctx, child, edges, fileHash, repoURL, filePath)
			case "class_definition":
				processClassDef(ctx, child, edges, fileHash, repoURL, filePath)
			case "decorated_definition":
				defNode := child.ChildByFieldName("definition")
				if defNode == nil && child.NamedChildCount() > 0 {
					defNode = child.NamedChild(int(child.NamedChildCount()) - 1)
				}
				if defNode != nil {
					switch defNode.Type() {
					case "function_definition":
						processFuncDef(ctx, defNode, edges, fileHash, repoURL, filePath)
					case "class_definition":
						processClassDef(ctx, defNode, edges, fileHash, repoURL, filePath)
					}
				}
			case "assignment", "expression_statement":
				ProcessStatement(ctx, child)
			}
		}
	}

	// Pop scope and restore.
	ctx.Scope = origScope
	ctx.EnclosingClassQN = origClassQN
}

// resolveCallsInNode recursively walks an AST node resolving call
// expressions and emitting edges.
func resolveCallsInNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	if node == nil {
		return
	}

	// Process statement for scope bindings.
	ProcessStatement(ctx, node)

	if node.Type() == "call" {
		if rc := resolveCallExpr(ctx, node); rc != nil {
			edge := buildCallEdge(ctx, node, rc, fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}
	}

	// if_statement gets special-case narrowing.
	if node.Type() == "if_statement" {
		walkIfStatement(ctx, node, edges, fileHash, repoURL, filePath)
		return
	}

	// Handle scope for comprehensions.
	needsPop := false
	switch node.Type() {
	case "list_comprehension", "dictionary_comprehension",
		"set_comprehension", "generator_expression":
		childScope := typresolve.NewScope(ctx.Scope)
		ctx.Scope = childScope
		needsPop = true

		// Bind for_in_clause variables.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "for_in_clause" {
				ProcessStatement(ctx, child)
			}
		}
	}

	// Skip recursion into nested function_definition, class_definition,
	// and lambda (processed by top-level pass or not at all).
	switch node.Type() {
	case "function_definition", "class_definition", "lambda", "decorated_definition":
		if needsPop {
			ctx.Scope = ctx.Scope.Parent()
		}
		return
	}

	// Recurse into children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil {
			resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
		}
	}

	if needsPop {
		ctx.Scope = ctx.Scope.Parent()
	}
}

// walkIfStatement handles if_statement with type narrowing in the consequence
// branch. Implements isinstance() and is-None/is-not-None guards.
func walkIfStatement(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	condNode := node.ChildByFieldName("condition")
	bodyNode := node.ChildByFieldName("consequence")
	altNode := node.ChildByFieldName("alternative")

	// Process walrus bindings in the condition (they leak to enclosing scope).
	bindWalrusIn(ctx, condNode)

	// Resolve calls in condition.
	resolveCallsInNode(ctx, condNode, edges, fileHash, repoURL, filePath)

	// Consequence branch: apply narrowing in a child scope.
	if bodyNode != nil {
		origScope := ctx.Scope
		ctx.Scope = typresolve.NewScope(origScope)

		// isinstance narrowing: if isinstance(x, T): -> bind x to T in body.
		if condNode != nil {
			if varName, narrowedType := matchIsinstance(ctx, condNode); varName != "" && narrowedType != nil {
				ctx.Scope.Bind(varName, narrowedType)
			}
		}

		// is-not-None narrowing: if x is not None: -> strip None from x's type.
		if condNode != nil {
			if varName, polarity := matchIsNone(ctx, condNode); varName != "" {
				if polarity == 1 { // "x is not None"
					current := ctx.Scope.Lookup(varName)
					if current != nil {
						ctx.Scope.Bind(varName, stripNone(current))
					}
				}
			}
		}

		resolveCallsInNode(ctx, bodyNode, edges, fileHash, repoURL, filePath)
		ctx.Scope = origScope
	}

	// Alternative branch (else/elif).
	if altNode != nil {
		resolveCallsInNode(ctx, altNode, edges, fileHash, repoURL, filePath)
	}

	// Early-return narrowing: if the body terminates and the condition is
	// "x is None: return" then x is non-None after the if.
	if bodyNode != nil && blockTerminates(bodyNode) {
		if condNode != nil {
			if varName, polarity := matchIsNone(ctx, condNode); varName != "" {
				if polarity == -1 { // "x is None" with body that returns
					current := ctx.Scope.Lookup(varName)
					if current != nil {
						ctx.Scope.Bind(varName, stripNone(current))
					}
				}
			}
		}
	}
}

// matchIsinstance detects isinstance(x, T) in a condition node.
// Returns (variable_name, narrowed_type) or ("", nil) if not matched.
func matchIsinstance(ctx *ResolveContext, condNode *sitter.Node) (string, *typresolve.Type) {
	if condNode.Type() != "call" {
		return "", nil
	}
	fnNode := condNode.ChildByFieldName("function")
	if fnNode == nil || fnNode.Type() != "identifier" {
		return "", nil
	}
	fname := nodeText(fnNode, ctx.Content)
	if fname != "isinstance" {
		return "", nil
	}
	argsNode := condNode.ChildByFieldName("arguments")
	if argsNode == nil || int(argsNode.NamedChildCount()) < 2 {
		return "", nil
	}
	varNode := argsNode.NamedChild(0)
	typeNode := argsNode.NamedChild(1)
	if varNode == nil || varNode.Type() != "identifier" {
		return "", nil
	}
	if typeNode == nil {
		return "", nil
	}
	varName := nodeText(varNode, ctx.Content)
	typeText := nodeText(typeNode, ctx.Content)
	if varName == "" || typeText == "" {
		return "", nil
	}
	narrowedType := ParseAnnotation(typeText, ctx.ModuleQN)
	return varName, narrowedType
}

// matchIsNone detects "x is None" or "x is not None" in a comparison_operator.
// Returns (variable_name, polarity):
//   - polarity = 1 means "x is not None" (positive narrow)
//   - polarity = -1 means "x is None" (negative narrow for early-return)
//   - polarity = 0 means no match
func matchIsNone(ctx *ResolveContext, condNode *sitter.Node) (string, int) {
	if condNode.Type() != "comparison_operator" {
		return "", 0
	}

	// Walk children to find the pattern "X is None" or "X is not None".
	childCount := int(condNode.ChildCount())
	var left, right *sitter.Node
	isOp := false
	isNotOp := false

	for i := 0; i < childCount; i++ {
		child := condNode.Child(i)
		if child == nil {
			continue
		}
		if child.IsNamed() {
			if left == nil {
				left = child
			} else if right == nil {
				right = child
			}
		} else {
			// Anonymous token: "is" or "is not"
			tok := nodeText(child, ctx.Content)
			if tok == "is" {
				isOp = true
			} else if tok == "is not" {
				isOp = true
				isNotOp = true
			}
		}
	}

	if !isOp || left == nil || right == nil {
		return "", 0
	}

	leftText := nodeText(left, ctx.Content)
	rightText := nodeText(right, ctx.Content)

	var varName string
	if rightText == "None" && left.Type() == "identifier" {
		varName = leftText
	} else if leftText == "None" && right.Type() == "identifier" {
		varName = rightText
	}

	if varName == "" {
		return "", 0
	}

	if isNotOp {
		return varName, 1 // x is not None
	}
	return varName, -1 // x is None
}

// stripNone removes None from an Optional type. If the type is Optional[T],
// returns T. Otherwise returns the type unchanged.
func stripNone(t *typresolve.Type) *typresolve.Type {
	if t == nil {
		return t
	}
	if t.Kind == typresolve.KindOptional && t.Elem != nil {
		return t.Elem
	}
	return t
}

// blockTerminates checks if a block ends with return, raise, break, or continue.
func blockTerminates(block *sitter.Node) bool {
	if block == nil {
		return false
	}
	count := int(block.NamedChildCount())
	if count == 0 {
		return false
	}
	last := block.NamedChild(count - 1)
	if last == nil {
		return false
	}
	switch last.Type() {
	case "return_statement", "raise_statement", "break_statement", "continue_statement":
		return true
	}
	return false
}

// bindWalrusIn walks a node tree looking for walrus expressions (named_expression)
// and binds their targets in the current scope.
func bindWalrusIn(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}
	if node.Type() == "named_expression" {
		nameNode := node.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = node.ChildByFieldName("left")
		}
		valueNode := node.ChildByFieldName("value")
		if valueNode == nil {
			valueNode = node.ChildByFieldName("right")
		}
		if nameNode != nil && nameNode.Type() == "identifier" && valueNode != nil {
			name := nodeText(nameNode, ctx.Content)
			if name != "" && name != "_" {
				valType := EvalExprType(ctx, valueNode)
				ctx.Scope.Bind(name, valType)
			}
		}
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil {
			bindWalrusIn(ctx, child)
		}
	}
}

// resolveCallExpr resolves a call expression node to a resolvedCall.
func resolveCallExpr(ctx *ResolveContext, node *sitter.Node) *resolvedCall {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return nil
	}

	switch fnNode.Type() {
	case "identifier":
		return resolveIdentifierCall(ctx, fnNode)
	case "attribute":
		return resolveAttributeCall(ctx, fnNode)
	default:
		return nil
	}
}

// resolveIdentifierCall resolves a simple identifier call (e.g. foo()).
func resolveIdentifierCall(ctx *ResolveContext, fnNode *sitter.Node) *resolvedCall {
	name := nodeText(fnNode, ctx.Content)

	// Skip builtins.
	if IsBuiltinFunc(name) {
		return nil
	}

	// Check scope for Named type (constructor call).
	if t := ctx.Scope.Lookup(name); t != nil {
		if t.Kind == typresolve.KindNamed {
			// Could be a constructor or a from-import function.
			// Check if the Named path is in the registry as a type.
			if ctx.Registry.LookupType(t.Name) != nil {
				return &resolvedCall{calleeQN: t.Name, strategy: "resolver_direct"}
			}
			// Check if it's a function via from-import.
			if ctx.Registry.LookupFunc(t.Name) != nil {
				return &resolvedCall{calleeQN: t.Name, strategy: "resolver_import"}
			}
			// Emit anyway as best-effort.
			return &resolvedCall{calleeQN: t.Name, strategy: "resolver_direct"}
		}
	}

	// Module-local function.
	calleeQN := ctx.ModuleQN + "." + name
	if ctx.Registry.LookupSymbol(ctx.ModuleQN, name) != nil {
		return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_direct"}
	}

	// Check registry for type (class constructor).
	if ctx.Registry.LookupType(calleeQN) != nil {
		return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_direct"}
	}

	// Builtins function.
	if ctx.Registry.LookupSymbol("builtins", name) != nil {
		return &resolvedCall{calleeQN: "builtins." + name, strategy: "resolver_direct"}
	}

	return nil
}

// resolveAttributeCall resolves an attribute call (e.g. obj.method()).
func resolveAttributeCall(ctx *ResolveContext, fnNode *sitter.Node) *resolvedCall {
	objNode := fnNode.ChildByFieldName("object")
	attrNode := fnNode.ChildByFieldName("attribute")
	if objNode == nil || attrNode == nil {
		return nil
	}

	attrName := nodeText(attrNode, ctx.Content)

	// Check if object is a module import.
	if objNode.Type() == "identifier" {
		objName := nodeText(objNode, ctx.Content)
		if info, ok := ResolveImport(ctx.Imports, objName); ok && !info.IsFromStyle {
			modulePath := info.ModulePath
			// Look up module.method in registry.
			calleeQN := modulePath + "." + attrName
			if ctx.Registry.LookupSymbol(modulePath, attrName) != nil {
				return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_import"}
			}
			// Could be a type constructor.
			if ctx.Registry.LookupType(calleeQN) != nil {
				return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_import"}
			}
			// Emit as best-effort.
			return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_import"}
		}
	}

	// super().method() resolution.
	if objNode.Type() == "call" {
		superFn := objNode.ChildByFieldName("function")
		if superFn != nil && superFn.Type() == "identifier" {
			superName := nodeText(superFn, ctx.Content)
			if superName == "super" && ctx.EnclosingClassQN != "" {
				rt := ctx.Registry.LookupType(ctx.EnclosingClassQN)
				if rt != nil {
					for _, baseQN := range rt.EmbeddedTypes {
						if m := LookupAttribute(ctx.Registry, baseQN, attrName); m != nil {
							return &resolvedCall{calleeQN: m.QualifiedName, strategy: "resolver_type_dispatch"}
						}
						// super().__init__ special case.
						if attrName == "__init__" {
							return &resolvedCall{calleeQN: baseQN + ".__init__", strategy: "resolver_type_dispatch"}
						}
					}
				}
			}
		}
	}

	// Evaluate object type for method dispatch.
	objType := EvalExprType(ctx, objNode)
	if objType == nil || objType.Kind == typresolve.KindUnknown {
		return nil
	}

	if objType.Kind == typresolve.KindNamed {
		typeQN := objType.Name
		// Look up method via LookupAttribute (follows MRO).
		if m := LookupAttribute(ctx.Registry, typeQN, attrName); m != nil {
			return &resolvedCall{calleeQN: m.QualifiedName, strategy: "resolver_type_dispatch"}
		}
		// Construct method QN even if not in registry.
		return &resolvedCall{calleeQN: typeQN + "." + attrName, strategy: "resolver_type_dispatch"}
	}

	if objType.Kind == typresolve.KindBuiltin {
		builtinQN := "builtins." + objType.Name
		if m := LookupAttribute(ctx.Registry, builtinQN, attrName); m != nil {
			return &resolvedCall{calleeQN: m.QualifiedName, strategy: "resolver_type_dispatch"}
		}
	}

	// Union/Optional type dispatch: try each Named member.
	if objType.Kind == typresolve.KindOptional && objType.Elem != nil {
		inner := objType.Elem
		if inner.Kind == typresolve.KindNamed {
			if m := LookupAttribute(ctx.Registry, inner.Name, attrName); m != nil {
				return &resolvedCall{calleeQN: m.QualifiedName, strategy: "resolver_type_dispatch"}
			}
			return &resolvedCall{calleeQN: inner.Name + "." + attrName, strategy: "resolver_type_dispatch"}
		}
	}

	return nil
}

// buildCallEdge creates a types.Edge from a resolved call.
func buildCallEdge(ctx *ResolveContext, callNode *sitter.Node, rc *resolvedCall, fileHash types.Hash, repoURL string, filePath string) types.Edge {
	// Compute source hash from the enclosing function.
	srcModulePath, srcFuncName, srcKind := splitEnclosingFunc(ctx.EnclosingFuncQN, ctx.ModuleQN)
	sourceHash := types.ComputeNodeHash(repoURL, srcModulePath, types.EmptyHash, srcFuncName, srcKind)

	// Compute target hash from the callee.
	tgtModulePath, tgtFuncName, tgtKind := splitQualifiedName(rc.calleeQN)
	targetHash := types.ComputeNodeHash(repoURL, tgtModulePath, types.EmptyHash, tgtFuncName, tgtKind)

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, typresolve.ProvenanceResolverResolved)

	return types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.Calls,
		Confidence:   typresolve.ResolverConfidence,
		Provenance:   typresolve.ProvenanceResolverResolved,
		CallSiteLine: int(callNode.StartPoint().Row) + 1, // 1-indexed
		CallSiteCol:  int(callNode.StartPoint().Column),   // 0-indexed
		CallSiteFile: filePath,
	}
}

// splitEnclosingFunc splits the enclosing function QN into module path,
// function name, and kind. Handles "mod.func" and "mod.Class.method".
func splitEnclosingFunc(funcQN string, moduleQN string) (modulePath string, funcName string, kind string) {
	if funcQN == "" {
		return moduleQN, "<module>", types.KindFunction
	}

	// Remove moduleQN prefix to get the rest.
	if len(funcQN) > len(moduleQN)+1 && strings.HasPrefix(funcQN, moduleQN+".") {
		rest := funcQN[len(moduleQN)+1:]
		// Check if it's a method (contains another dot): Class.method
		if strings.Contains(rest, ".") {
			return moduleQN, rest, types.KindMethod
		}
		return moduleQN, rest, types.KindFunction
	}

	return moduleQN, funcQN, types.KindFunction
}

// splitQualifiedName splits a callee qualified name into module path,
// symbol name, and kind.
func splitQualifiedName(qn string) (modulePath string, symbolName string, kind string) {
	lastDot := strings.LastIndex(qn, ".")
	secondLastDot := -1

	if lastDot > 0 {
		secondLastDot = strings.LastIndex(qn[:lastDot], ".")
	}

	if lastDot == -1 {
		return "", qn, types.KindFunction
	}

	if secondLastDot >= 0 {
		// Could be module.Class.method
		possibleClass := qn[secondLastDot+1 : lastDot]
		// Heuristic: class names start with uppercase in Python convention
		if len(possibleClass) > 0 && possibleClass[0] >= 'A' && possibleClass[0] <= 'Z' {
			return qn[:secondLastDot], qn[secondLastDot+1:], types.KindMethod
		}
	}

	return qn[:lastDot], qn[lastDot+1:], types.KindFunction
}

// bindPythonParams binds function parameters into the current scope.
func bindPythonParams(ctx *ResolveContext, paramsNode *sitter.Node, isStaticMethod bool) {
	firstParam := true
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		param := paramsNode.NamedChild(i)
		if param == nil {
			continue
		}

		switch param.Type() {
		case "identifier":
			// Simple parameter without annotation.
			name := nodeText(param, ctx.Content)
			if name == "_" {
				firstParam = false
				continue
			}

			if firstParam && !isStaticMethod && ctx.EnclosingClassQN != "" {
				// First param is self or cls: bind to class type.
				if name == "self" || name == "cls" {
					ctx.Scope.Bind(name, typresolve.Named(ctx.EnclosingClassQN))
					firstParam = false
					continue
				}
			}
			ctx.Scope.Bind(name, typresolve.Unknown())
			firstParam = false

		case "typed_parameter":
			// typed_parameter has identifier as first named child (not a field),
			// and "type" as a field child.
			nameNode := findIdentifierChild(param)
			typeNode := param.ChildByFieldName("type")
			if nameNode == nil {
				firstParam = false
				continue
			}
			name := nodeText(nameNode, ctx.Content)
			if name == "_" {
				firstParam = false
				continue
			}

			if firstParam && !isStaticMethod && ctx.EnclosingClassQN != "" {
				if name == "self" || name == "cls" {
					ctx.Scope.Bind(name, typresolve.Named(ctx.EnclosingClassQN))
					firstParam = false
					continue
				}
			}

			var paramType *typresolve.Type
			if typeNode != nil {
				annText := nodeText(typeNode, ctx.Content)
				paramType = ParseAnnotation(annText, ctx.ModuleQN)
			}
			if paramType == nil {
				paramType = typresolve.Unknown()
			}
			ctx.Scope.Bind(name, paramType)
			firstParam = false

		case "typed_default_parameter":
			nameNode := findIdentifierChild(param)
			typeNode := param.ChildByFieldName("type")
			if nameNode == nil {
				firstParam = false
				continue
			}
			name := nodeText(nameNode, ctx.Content)
			if name == "_" {
				firstParam = false
				continue
			}

			var paramType *typresolve.Type
			if typeNode != nil {
				annText := nodeText(typeNode, ctx.Content)
				paramType = ParseAnnotation(annText, ctx.ModuleQN)
			}
			if paramType == nil {
				paramType = typresolve.Unknown()
			}
			ctx.Scope.Bind(name, paramType)
			firstParam = false

		case "default_parameter":
			nameNode := param.ChildByFieldName("name")
			if nameNode == nil {
				firstParam = false
				continue
			}
			name := nodeText(nameNode, ctx.Content)
			if name != "_" {
				ctx.Scope.Bind(name, typresolve.Unknown())
			}
			firstParam = false

		case "list_splat_pattern", "dictionary_splat_pattern":
			// *args or **kwargs
			if param.NamedChildCount() > 0 {
				child := param.NamedChild(0)
				if child != nil && child.Type() == "identifier" {
					name := nodeText(child, ctx.Content)
					if name != "_" {
						ctx.Scope.Bind(name, typresolve.Unknown())
					}
				}
			}
			firstParam = false

		default:
			firstParam = false
		}
	}
}

// findIdentifierChild finds the first identifier child of a node.
// In tree-sitter Python, typed_parameter and typed_default_parameter
// have the parameter name as a regular child (not a named field).
func findIdentifierChild(node *sitter.Node) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Type() == "identifier" {
			return child
		}
	}
	return nil
}

// isDecorated checks if a function_definition's parent (or the node itself)
// has a specific decorator. Since we receive the inner function_definition
// (after unwrapping decorated_definition), we check the parent.
// The content parameter provides the source bytes needed for node text
// extraction.
func isDecorated(node *sitter.Node, decoratorName string, content []byte) bool {
	parent := node.Parent()
	if parent == nil || parent.Type() != "decorated_definition" {
		return false
	}
	for i := 0; i < int(parent.NamedChildCount()); i++ {
		child := parent.NamedChild(i)
		if child != nil && child.Type() == "decorator" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				inner := child.NamedChild(j)
				if inner != nil && inner.Type() == "identifier" {
					if inner.Content(content) == decoratorName {
						return true
					}
				}
			}
		}
	}
	return false
}
