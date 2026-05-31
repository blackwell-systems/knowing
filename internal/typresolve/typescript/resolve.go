package tsresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
)

// ResolveCallsInFile walks a TypeScript file's AST resolving call expressions,
// new expressions, and JSX elements, emitting edges. Uses the shared registry,
// per-file scope chain, and import map.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	if root == nil {
		return nil
	}

	var edges []types.Edge

	// Pass 1: Process top-level variable declarations into root scope.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "lexical_declaration", "variable_declaration":
			ProcessStatement(ctx, child)
		case "export_statement":
			// Check for exported variable declarations.
			for j := 0; j < int(child.NamedChildCount()); j++ {
				inner := child.NamedChild(j)
				if inner != nil && (inner.Type() == "lexical_declaration" || inner.Type() == "variable_declaration") {
					ProcessStatement(ctx, inner)
				}
			}
		}
	}

	// Pass 2: Process functions, classes, and other declarations.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}
		processTopLevel(ctx, child, &edges, fileHash, repoURL, filePath)
	}

	return edges
}

// processTopLevel handles a top-level AST node.
func processTopLevel(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	switch node.Type() {
	case "function_declaration":
		processFunctionDeclaration(ctx, node, edges, fileHash, repoURL, filePath)

	case "class_declaration":
		processClassDeclaration(ctx, node, edges, fileHash, repoURL, filePath)

	case "export_statement":
		// Unwrap export to inner declaration.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			inner := node.NamedChild(i)
			if inner == nil {
				continue
			}
			switch inner.Type() {
			case "function_declaration":
				processFunctionDeclaration(ctx, inner, edges, fileHash, repoURL, filePath)
			case "class_declaration":
				processClassDeclaration(ctx, inner, edges, fileHash, repoURL, filePath)
			case "lexical_declaration":
				processLexicalWithArrows(ctx, inner, edges, fileHash, repoURL, filePath)
			}
		}

	case "lexical_declaration":
		processLexicalWithArrows(ctx, node, edges, fileHash, repoURL, filePath)

	case "expression_statement":
		// Top-level expressions (e.g., IIFE, method calls).
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
			}
		}
	}
}

// processFunctionDeclaration handles a function_declaration node.
func processFunctionDeclaration(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := nodeText(nameNode, ctx.Content)

	oldFuncQN := ctx.EnclosingFuncQN
	ctx.EnclosingFuncQN = ctx.ModuleQN + "." + funcName

	// Push function scope.
	childScope := typresolve.NewScope(ctx.Scope)
	oldScope := ctx.Scope
	ctx.Scope = childScope

	// Bind parameters.
	bindFunctionParams(ctx, node)

	// Walk function body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop function scope.
	ctx.Scope = oldScope
	ctx.EnclosingFuncQN = oldFuncQN
}

// processClassDeclaration handles a class_declaration node.
func processClassDeclaration(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	className := nodeText(nameNode, ctx.Content)
	classQN := ctx.ModuleQN + "." + className

	oldClassQN := ctx.EnclosingClassQN
	ctx.EnclosingClassQN = classQN

	// Push class scope.
	classScope := typresolve.NewScope(ctx.Scope)
	oldScope := ctx.Scope
	ctx.Scope = classScope

	// Bind `this` to the class type.
	ctx.Scope.Bind("this", typresolve.Named(classQN))

	// Walk class body for method_definition nodes.
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		for i := 0; i < int(bodyNode.NamedChildCount()); i++ {
			member := bodyNode.NamedChild(i)
			if member == nil {
				continue
			}
			if member.Type() == "method_definition" {
				processMethodDefinition(ctx, member, classQN, edges, fileHash, repoURL, filePath)
			} else if member.Type() == "public_field_definition" || member.Type() == "field_definition" {
				// Field initializers may contain calls.
				valueNode := member.ChildByFieldName("value")
				if valueNode != nil {
					resolveCallsInNode(ctx, valueNode, edges, fileHash, repoURL, filePath)
				}
			}
		}
	}

	// Pop class scope.
	ctx.Scope = oldScope
	ctx.EnclosingClassQN = oldClassQN
}

// processMethodDefinition handles a method_definition inside a class body.
func processMethodDefinition(ctx *ResolveContext, node *sitter.Node, classQN string, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeText(nameNode, ctx.Content)

	oldFuncQN := ctx.EnclosingFuncQN
	ctx.EnclosingFuncQN = classQN + "." + methodName

	// Push method scope.
	methodScope := typresolve.NewScope(ctx.Scope)
	oldScope := ctx.Scope
	ctx.Scope = methodScope

	// Bind `this` to the class type.
	ctx.Scope.Bind("this", typresolve.Named(classQN))

	// Bind parameters (skip explicit `this: Type` parameter).
	bindMethodParams(ctx, node)

	// Walk method body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop method scope.
	ctx.Scope = oldScope
	ctx.EnclosingFuncQN = oldFuncQN
}

// processLexicalWithArrows handles lexical_declaration nodes that may
// contain arrow functions (e.g., const handler = () => { ... }).
func processLexicalWithArrows(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil || child.Type() != "variable_declarator" {
			continue
		}

		nameNode := child.ChildByFieldName("name")
		valueNode := child.ChildByFieldName("value")
		if nameNode == nil || valueNode == nil {
			continue
		}

		if valueNode.Type() == "arrow_function" || valueNode.Type() == "function" {
			varName := nodeText(nameNode, ctx.Content)

			oldFuncQN := ctx.EnclosingFuncQN
			ctx.EnclosingFuncQN = ctx.ModuleQN + "." + varName

			// Push function scope.
			fnScope := typresolve.NewScope(ctx.Scope)
			oldScope := ctx.Scope
			ctx.Scope = fnScope

			// Bind parameters for the arrow/function.
			bindArrowOrFunctionParams(ctx, valueNode)

			// Walk function body.
			body := valueNode.ChildByFieldName("body")
			if body != nil {
				resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
			}

			ctx.Scope = oldScope
			ctx.EnclosingFuncQN = oldFuncQN
		} else {
			// Regular variable with potential call expressions in initializer.
			resolveCallsInNode(ctx, valueNode, edges, fileHash, repoURL, filePath)
		}
	}
}

// resolveCallsInNode recursively walks an AST node resolving calls.
func resolveCallsInNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	if node == nil {
		return
	}

	// Process statement bindings (variable declarations, etc.).
	ProcessStatement(ctx, node)

	switch node.Type() {
	case "call_expression":
		resolveCallExpression(ctx, node, edges, fileHash, repoURL, filePath)

	case "new_expression":
		resolveNewExpression(ctx, node, edges, fileHash, repoURL, filePath)

	case "jsx_element":
		resolveJSXElement(ctx, node, edges, fileHash, repoURL, filePath)

	case "jsx_self_closing_element":
		resolveJSXSelfClosing(ctx, node, edges, fileHash, repoURL, filePath)
	}

	// Push scope for block nodes.
	pushScope := isBlockNode(node.Type())
	var oldScope *typresolve.Scope
	if pushScope {
		childScope := typresolve.NewScope(ctx.Scope)
		oldScope = ctx.Scope
		ctx.Scope = childScope
	}

	// Recurse into children, skipping nested function/class declarations
	// and arrow functions (handled separately by top-level pass or as
	// expression contexts).
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		childType := child.Type()
		if childType == "function_declaration" || childType == "class_declaration" {
			continue
		}
		// Arrow functions in non-top-level positions: walk their bodies
		// for call resolution within the current enclosing function context.
		if childType == "arrow_function" {
			arrowScope := typresolve.NewScope(ctx.Scope)
			origScope := ctx.Scope
			ctx.Scope = arrowScope
			bindArrowOrFunctionParams(ctx, child)
			body := child.ChildByFieldName("body")
			if body != nil {
				resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
			}
			ctx.Scope = origScope
			continue
		}
		resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
	}

	if pushScope {
		ctx.Scope = oldScope
	}
}

// resolveCallExpression resolves a call_expression and emits edges.
func resolveCallExpression(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return
	}

	switch fnNode.Type() {
	case "member_expression":
		resolveMemberCall(ctx, fnNode, edges, fileHash, repoURL, filePath)

	case "identifier":
		name := nodeText(fnNode, ctx.Content)
		if isBuiltinFunction(name) {
			return
		}
		resolveSimpleCall(ctx, name, edges, fileHash, repoURL, filePath)
	}
}

// resolveMemberCall resolves obj.method() calls.
func resolveMemberCall(ctx *ResolveContext, fnNode *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	object := fnNode.ChildByFieldName("object")
	property := fnNode.ChildByFieldName("property")
	if object == nil || property == nil {
		return
	}

	propName := nodeText(property, ctx.Content)
	objName := ""
	if object.Type() == "identifier" {
		objName = nodeText(object, ctx.Content)
	}

	// Check if object is a namespace import: ns.method().
	if objName != "" {
		if imp, ok := ResolveImport(ctx.Imports, objName); ok && imp.IsNamespace {
			modulePath := ResolveModulePath(imp.ModulePath, filePath)
			if modulePath == "" {
				modulePath = imp.ModulePath
			}
			funcQN := modulePath + "." + propName
			if f := ctx.Registry.LookupFunc(funcQN); f != nil {
				emitCallEdge(ctx, edges, fileHash, repoURL, filePath, f.QualifiedName, modulePath, propName, types.KindFunction)
				return
			}
		}
	}

	// Evaluate object type and look up method on receiver type.
	objType := EvalExprType(ctx, object)
	if objType != nil && objType.Kind == typresolve.KindNamed {
		if f := LookupMember(ctx.Registry, objType.Name, propName); f != nil {
			// Determine target module path from the receiver type QN.
			targetModulePath := extractModulePath(f.QualifiedName)
			targetName := f.ShortName
			emitCallEdge(ctx, edges, fileHash, repoURL, filePath, f.QualifiedName, targetModulePath, targetName, types.KindMethod)
			return
		}
	}

	// Builtin wrapper class fallback for primitives.
	if objType != nil && objType.Kind == typresolve.KindBuiltin {
		wrapper := BuiltinWrapperClass(objType.Name)
		if wrapper != "" {
			if f := LookupMember(ctx.Registry, wrapper, propName); f != nil {
				emitCallEdge(ctx, edges, fileHash, repoURL, filePath, f.QualifiedName, "", f.ShortName, types.KindMethod)
				return
			}
		}
	}
}

// resolveSimpleCall resolves a simple function call by identifier.
func resolveSimpleCall(ctx *ResolveContext, name string, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Check if name is in scope as a Named type (constructor-like call).
	if t := ctx.Scope.Lookup(name); t != nil && t.Kind == typresolve.KindNamed {
		// Check if it's a registered type (constructor call without new).
		if ctx.Registry.LookupType(t.Name) != nil {
			targetModulePath := extractModulePath(t.Name)
			emitCallEdge(ctx, edges, fileHash, repoURL, filePath, t.Name, targetModulePath, name, types.KindFunction)
			return
		}
	}

	// Check imports.
	if imp, ok := ResolveImport(ctx.Imports, name); ok {
		modulePath := ResolveModulePath(imp.ModulePath, filePath)
		if modulePath == "" {
			modulePath = imp.ModulePath
		}
		lookupName := imp.OriginalName
		if lookupName == "" {
			lookupName = name
		}
		funcQN := modulePath + "." + lookupName
		if f := ctx.Registry.LookupFunc(funcQN); f != nil {
			emitCallEdge(ctx, edges, fileHash, repoURL, filePath, f.QualifiedName, modulePath, lookupName, types.KindFunction)
			return
		}
	}

	// Module-local function.
	if f := ctx.Registry.LookupSymbol(ctx.ModuleQN, name); f != nil {
		emitCallEdge(ctx, edges, fileHash, repoURL, filePath, f.QualifiedName, ctx.ModuleQN, name, types.KindFunction)
		return
	}
}

// resolveNewExpression resolves `new MyClass()` constructor calls.
func resolveNewExpression(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	constructor := node.ChildByFieldName("constructor")
	if constructor == nil {
		return
	}

	if constructor.Type() != "identifier" {
		return
	}

	name := nodeText(constructor, ctx.Content)

	// Check imports.
	if imp, ok := ResolveImport(ctx.Imports, name); ok {
		modulePath := ResolveModulePath(imp.ModulePath, filePath)
		if modulePath == "" {
			modulePath = imp.ModulePath
		}
		lookupName := imp.OriginalName
		if lookupName == "" {
			lookupName = name
		}
		typeQN := modulePath + "." + lookupName
		if ctx.Registry.LookupType(typeQN) != nil {
			emitCallEdge(ctx, edges, fileHash, repoURL, filePath, typeQN, modulePath, lookupName, types.KindFunction)
			return
		}
	}

	// Module-local type.
	typeQN := ctx.ModuleQN + "." + name
	if ctx.Registry.LookupType(typeQN) != nil {
		emitCallEdge(ctx, edges, fileHash, repoURL, filePath, typeQN, ctx.ModuleQN, name, types.KindFunction)
		return
	}

	// Global type.
	if ctx.Registry.LookupType(name) != nil {
		emitCallEdge(ctx, edges, fileHash, repoURL, filePath, name, "", name, types.KindFunction)
	}
}

// resolveJSXElement resolves <Component> in JSX elements.
func resolveJSXElement(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Get opening element.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Type() == "jsx_opening_element" {
			resolveJSXTagName(ctx, child, edges, fileHash, repoURL, filePath)
			break
		}
	}
}

// resolveJSXSelfClosing resolves <Component /> in self-closing JSX elements.
func resolveJSXSelfClosing(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	resolveJSXTagName(ctx, node, edges, fileHash, repoURL, filePath)
}

// resolveJSXTagName resolves the component name in a JSX element.
func resolveJSXTagName(ctx *ResolveContext, element *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// The first named child is typically the tag name (identifier or member_expression).
	nameNode := element.ChildByFieldName("name")
	if nameNode == nil {
		// Fallback: first named child.
		if element.NamedChildCount() > 0 {
			nameNode = element.NamedChild(0)
		}
	}
	if nameNode == nil {
		return
	}

	name := nodeText(nameNode, ctx.Content)

	// Only resolve uppercase component names (lowercase are HTML intrinsics).
	if len(name) == 0 || name[0] < 'A' || name[0] > 'Z' {
		return
	}

	// Check imports.
	if imp, ok := ResolveImport(ctx.Imports, name); ok {
		modulePath := ResolveModulePath(imp.ModulePath, filePath)
		if modulePath == "" {
			modulePath = imp.ModulePath
		}
		lookupName := imp.OriginalName
		if lookupName == "" {
			lookupName = name
		}
		funcQN := modulePath + "." + lookupName
		emitCallEdge(ctx, edges, fileHash, repoURL, filePath, funcQN, modulePath, lookupName, types.KindFunction)
		return
	}

	// Module-local component.
	funcQN := ctx.ModuleQN + "." + name
	if ctx.Registry.LookupFunc(funcQN) != nil || ctx.Registry.LookupType(funcQN) != nil {
		emitCallEdge(ctx, edges, fileHash, repoURL, filePath, funcQN, ctx.ModuleQN, name, types.KindFunction)
	}
}

// emitCallEdge creates and appends a calls edge.
func emitCallEdge(ctx *ResolveContext, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, targetQN string, targetModulePath string, targetName string, targetKind string) {
	// Compute source hash from enclosing function.
	sourceFuncName := ctx.EnclosingFuncQN
	if sourceFuncName == "" {
		sourceFuncName = ctx.ModuleQN + ".<module>"
	}

	sourceModulePath := ctx.ModuleQN
	sourceShortName := lastSegment(sourceFuncName)

	sourceHash := types.ComputeNodeHash(repoURL, sourceModulePath, types.EmptyHash, sourceShortName, types.KindFunction)

	// Compute target hash.
	if targetModulePath == "" {
		targetModulePath = extractModulePath(targetQN)
	}
	targetShortName := targetName
	if targetShortName == "" {
		targetShortName = lastSegment(targetQN)
	}

	targetHash := types.ComputeNodeHash(repoURL, targetModulePath, types.EmptyHash, targetShortName, targetKind)

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, typresolve.ProvenanceResolverResolved)

	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.Calls,
		Confidence: typresolve.ResolverConfidence,
		Provenance: typresolve.ProvenanceResolverResolved,
	})
}

// bindFunctionParams binds parameter names into scope from a function_declaration.
func bindFunctionParams(ctx *ResolveContext, funcNode *sitter.Node) {
	params := funcNode.ChildByFieldName("parameters")
	if params == nil {
		return
	}
	bindParamList(ctx, params, false)
}

// bindMethodParams binds parameter names into scope from a method_definition,
// skipping an explicit `this: Type` parameter if present.
func bindMethodParams(ctx *ResolveContext, methodNode *sitter.Node) {
	params := methodNode.ChildByFieldName("parameters")
	if params == nil {
		return
	}
	bindParamList(ctx, params, true)
}

// bindArrowOrFunctionParams binds parameters from an arrow_function or function expression.
func bindArrowOrFunctionParams(ctx *ResolveContext, node *sitter.Node) {
	params := node.ChildByFieldName("parameters")
	if params != nil {
		bindParamList(ctx, params, false)
		return
	}
	// Single parameter arrow: (x) => ... or x => ...
	param := node.ChildByFieldName("parameter")
	if param != nil && param.Type() == "identifier" {
		ctx.Scope.Bind(nodeText(param, ctx.Content), typresolve.Unknown())
	}
}

// bindParamList binds parameters from a formal_parameters node.
func bindParamList(ctx *ResolveContext, params *sitter.Node, skipThis bool) {
	firstParam := true
	for i := 0; i < int(params.NamedChildCount()); i++ {
		child := params.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "required_parameter", "optional_parameter", "rest_parameter":
			// Check for explicit `this: Type` parameter.
			if skipThis && firstParam {
				firstParam = false
				nameNode := child.ChildByFieldName("pattern")
				if nameNode == nil {
					nameNode = child.ChildByFieldName("name")
				}
				if nameNode != nil && nodeText(nameNode, ctx.Content) == "this" {
					continue
				}
			}
			firstParam = false

			nameNode := child.ChildByFieldName("pattern")
			if nameNode == nil {
				nameNode = child.ChildByFieldName("name")
			}
			if nameNode == nil {
				continue
			}

			paramName := nodeText(nameNode, ctx.Content)
			var paramType *typresolve.Type

			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				paramType = ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Imports)
			}
			if paramType == nil {
				paramType = typresolve.Unknown()
			}

			if paramName != "" && paramName != "_" {
				ctx.Scope.Bind(paramName, paramType)
			}

		case "identifier":
			firstParam = false
			name := nodeText(child, ctx.Content)
			if name != "" && name != "_" {
				ctx.Scope.Bind(name, typresolve.Unknown())
			}
		}
	}
}

// extractModulePath extracts the module path from a qualified name.
// For "src/compiler/checker.doCheck", returns "src/compiler/checker".
// For "express.Router", returns "express".
func extractModulePath(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[:idx]
	}
	return qn
}

// isBlockNode returns true for AST node types that introduce a new scope.
func isBlockNode(nodeType string) bool {
	switch nodeType {
	case "statement_block", "if_statement", "else_clause",
		"for_statement", "for_in_statement", "for_of_statement",
		"while_statement", "do_statement",
		"switch_case", "switch_default",
		"try_statement", "catch_clause", "finally_clause":
		return true
	}
	return false
}

// isBuiltinFunction returns true for JavaScript builtins that should not
// produce call edges.
func isBuiltinFunction(name string) bool {
	switch name {
	case "console", "parseInt", "parseFloat", "isNaN", "isFinite",
		"encodeURI", "decodeURI", "encodeURIComponent", "decodeURIComponent",
		"setTimeout", "setInterval", "clearTimeout", "clearInterval",
		"require", "eval", "alert", "confirm", "prompt",
		"JSON", "Math", "Object", "Array", "String", "Number", "Boolean",
		"Symbol", "BigInt", "Date", "RegExp", "Error", "Promise",
		"Map", "Set", "WeakMap", "WeakSet", "Proxy", "Reflect":
		return true
	}
	return false
}
