package rustresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// stdMacros are standard library macros that don't produce meaningful call edges.
var stdMacros = map[string]bool{
	"println": true, "eprintln": true, "print": true, "eprint": true,
	"format": true, "format_args": true, "write": true, "writeln": true,
	"vec": true, "dbg": true, "panic": true,
	"todo": true, "unimplemented": true, "unreachable": true,
	"assert": true, "assert_eq": true, "assert_ne": true,
	"debug_assert": true, "debug_assert_eq": true, "debug_assert_ne": true,
	"cfg": true, "env": true, "option_env": true,
	"include": true, "include_str": true, "include_bytes": true,
	"concat": true, "stringify": true,
	"file": true, "line": true, "column": true, "module_path": true,
	"compile_error": true, "matches": true, "thread_local": true,
}

// ResolveCallsInFile walks a Rust file's AST resolving call expressions,
// method calls, macro invocations, and path expressions, then emitting call edges.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	var edges []types.Edge

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "function_item":
			processFunctionItem(ctx, child, &edges, fileHash, repoURL, filePath)

		case "impl_item":
			processImplItem(ctx, child, &edges, fileHash, repoURL, filePath)

		case "macro_invocation":
			// Top-level macro invocations.
			ctx.EnclosingFuncQN = ctx.ModuleQN
			resolveMacroInvocation(ctx, child, &edges, fileHash, repoURL, filePath)
			ctx.EnclosingFuncQN = ""
		}
	}

	return edges
}

// processFunctionItem handles a top-level function_item node.
func processFunctionItem(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := nodeContent(nameNode, ctx.Content)
	ctx.EnclosingFuncQN = ctx.ModuleQN + "." + funcName

	// Push function scope.
	funcScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = funcScope

	// Extract and bind generic trait bounds.
	origBounds := ctx.TraitBounds
	ctx.TraitBounds = extractTraitBounds(ctx, node)

	// Bind function parameters (including trait-bounded params).
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindRustParams(ctx, paramsNode)
	}

	// Resolve calls in the function body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop scope and trait bounds.
	ctx.TraitBounds = origBounds
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = ""
}

// extractTraitBounds parses type_parameters on a function_item to extract
// trait bounds like T: Handler. Returns a map from type param name to trait QN.
func extractTraitBounds(ctx *ResolveContext, funcNode *sitter.Node) map[string]string {
	bounds := make(map[string]string)

	// Look for type_parameters child node.
	var typeParamsNode *sitter.Node
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		child := funcNode.Child(i)
		if child != nil && child.Type() == "type_parameters" {
			typeParamsNode = child
			break
		}
	}
	if typeParamsNode == nil {
		return bounds
	}

	// Walk type_parameters looking for constrained_type_parameter nodes.
	for i := 0; i < int(typeParamsNode.ChildCount()); i++ {
		child := typeParamsNode.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "constrained_type_parameter" {
			// constrained_type_parameter has a "left" (the type param name)
			// and bounds (the trait(s)).
			leftNode := child.ChildByFieldName("left")
			boundsNode := child.ChildByFieldName("bounds")
			if leftNode == nil {
				continue
			}
			paramName := nodeContent(leftNode, ctx.Content)
			if boundsNode != nil {
				// Get the first trait bound.
				traitName := extractFirstTraitBound(ctx, boundsNode)
				if traitName != "" {
					bounds[paramName] = traitName
				}
			}
		}
	}

	// Also check where_clause for bounds.
	var whereNode *sitter.Node
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		child := funcNode.Child(i)
		if child != nil && child.Type() == "where_clause" {
			whereNode = child
			break
		}
	}
	if whereNode != nil {
		for i := 0; i < int(whereNode.ChildCount()); i++ {
			child := whereNode.Child(i)
			if child != nil && child.Type() == "where_predicate" {
				leftNode := child.ChildByFieldName("left")
				boundsNode := child.ChildByFieldName("bounds")
				if leftNode == nil || boundsNode == nil {
					continue
				}
				paramName := nodeContent(leftNode, ctx.Content)
				traitName := extractFirstTraitBound(ctx, boundsNode)
				if traitName != "" {
					bounds[paramName] = traitName
				}
			}
		}
	}

	return bounds
}

// extractFirstTraitBound extracts the first trait name from a trait_bounds node.
func extractFirstTraitBound(ctx *ResolveContext, boundsNode *sitter.Node) string {
	for i := 0; i < int(boundsNode.ChildCount()); i++ {
		child := boundsNode.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "type_identifier":
			name := nodeContent(child, ctx.Content)
			// Resolve through uses map.
			if path, ok := ctx.Uses[name]; ok {
				return convertToQN(path + "::" + name)
			}
			return ctx.ModuleQN + "." + name
		case "scoped_type_identifier":
			fullPath := nodeContent(child, ctx.Content)
			parts := strings.Split(fullPath, "::")
			if len(parts) > 0 {
				if resolved, ok := ctx.Uses[parts[0]]; ok {
					parts[0] = resolved
					return convertToQN(strings.Join(parts, "::"))
				}
			}
			return convertToQN(fullPath)
		case "generic_type":
			// e.g., Iterator<Item = String>
			baseNode := child.ChildByFieldName("type")
			if baseNode == nil && child.ChildCount() > 0 {
				baseNode = child.Child(0)
			}
			if baseNode != nil {
				name := nodeContent(baseNode, ctx.Content)
				if path, ok := ctx.Uses[name]; ok {
					return convertToQN(path + "::" + name)
				}
				return ctx.ModuleQN + "." + name
			}
		}
	}
	return ""
}

// processImplItem handles an impl_item node (impl Type { ... } or impl Trait for Type { ... }).
func processImplItem(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Get the impl target type.
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	typeName := nodeContent(typeNode, ctx.Content)

	// Resolve the type to a qualified name.
	implTypeQN := resolveTypeQN(ctx, typeName)
	origImplType := ctx.ImplType
	ctx.ImplType = implTypeQN

	// Walk the body's children looking for function_item (methods).
	body := node.ChildByFieldName("body")
	if body == nil {
		ctx.ImplType = origImplType
		return
	}

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child == nil {
			continue
		}

		if child.Type() == "function_item" {
			processMethodInImpl(ctx, child, implTypeQN, edges, fileHash, repoURL, filePath)
		}
	}

	ctx.ImplType = origImplType
}

// processMethodInImpl handles a method defined within an impl block.
func processMethodInImpl(ctx *ResolveContext, node *sitter.Node, implTypeQN string, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeContent(nameNode, ctx.Content)
	ctx.EnclosingFuncQN = implTypeQN + "." + methodName

	// Push method scope.
	funcScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = funcScope

	// Extract and bind generic trait bounds for this method.
	origBounds := ctx.TraitBounds
	ctx.TraitBounds = extractTraitBounds(ctx, node)

	// Bind self parameter to Named(implTypeQN).
	ctx.Scope.Bind("self", typresolve.Named(implTypeQN))

	// Bind other parameters.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindRustParams(ctx, paramsNode)
	}

	// Resolve calls in the method body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop scope and trait bounds.
	ctx.TraitBounds = origBounds
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = ""
}

// resolveCallsInNode recursively walks an AST node resolving call
// expressions and emitting edges.
func resolveCallsInNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	if node == nil {
		return
	}

	// Process statement for scope bindings.
	ProcessStatement(ctx, node)

	switch node.Type() {
	case "call_expression":
		resolveCallExpression(ctx, node, edges, fileHash, repoURL, filePath)

	case "macro_invocation":
		resolveMacroInvocation(ctx, node, edges, fileHash, repoURL, filePath)

	case "function_item":
		// Nested function: push new scope and recurse.
		processNestedFunction(ctx, node, edges, fileHash, repoURL, filePath)
		return

	case "block":
		// Push scope for blocks.
		childScope := typresolve.NewScope(ctx.Scope)
		origScope := ctx.Scope
		ctx.Scope = childScope
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
			}
		}
		ctx.Scope = origScope
		return

	case "if_expression", "while_expression", "loop_expression", "for_expression":
		childScope := typresolve.NewScope(ctx.Scope)
		origScope := ctx.Scope
		ctx.Scope = childScope
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
			}
		}
		ctx.Scope = origScope
		return

	case "if_let_expression", "while_let_expression":
		childScope := typresolve.NewScope(ctx.Scope)
		origScope := ctx.Scope
		ctx.Scope = childScope
		// Process the let pattern binding.
		ProcessStatement(ctx, node)
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
			}
		}
		ctx.Scope = origScope
		return

	case "match_expression":
		// Process the scrutinee normally, then each arm with its own scope.
		scrutinee := node.ChildByFieldName("value")
		if scrutinee != nil {
			resolveCallsInNode(ctx, scrutinee, edges, fileHash, repoURL, filePath)
		}
		body := node.ChildByFieldName("body")
		if body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				arm := body.Child(i)
				if arm != nil && arm.Type() == "match_arm" {
					armScope := typresolve.NewScope(ctx.Scope)
					origScope := ctx.Scope
					ctx.Scope = armScope
					ProcessStatement(ctx, arm)
					for j := 0; j < int(arm.ChildCount()); j++ {
						armChild := arm.Child(j)
						if armChild != nil {
							resolveCallsInNode(ctx, armChild, edges, fileHash, repoURL, filePath)
						}
					}
					ctx.Scope = origScope
				}
			}
		}
		return

	case "closure_expression":
		childScope := typresolve.NewScope(ctx.Scope)
		origScope := ctx.Scope
		ctx.Scope = childScope
		// Bind closure parameters.
		paramsNode := node.ChildByFieldName("parameters")
		if paramsNode != nil {
			bindRustParams(ctx, paramsNode)
		}
		body := node.ChildByFieldName("body")
		if body != nil {
			resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
		}
		ctx.Scope = origScope
		return
	}

	// Recurse into children for all other node types.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child != nil {
			resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath)
		}
	}
}

// resolveCallExpression handles a call_expression node.
func resolveCallExpression(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		return
	}

	switch funcNode.Type() {
	case "scoped_identifier":
		// Type::method() or module::function() style calls.
		resolveCalleeQN := resolveScopedCall(ctx, funcNode)
		if resolveCalleeQN != "" {
			edge := buildCallEdge(ctx, node, resolveCalleeQN, "resolver_scoped", fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}

	case "identifier":
		// Bare function call: func_name()
		name := nodeContent(funcNode, ctx.Content)
		if IsBuiltinFunc(name) {
			return
		}
		calleeQN := resolveIdentCall(ctx, name)
		if calleeQN != "" {
			edge := buildCallEdge(ctx, node, calleeQN, "resolver_direct", fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}

	case "field_expression":
		// Method call: obj.method()
		resolveMethodCall(ctx, node, funcNode, edges, fileHash, repoURL, filePath)
	}
}

// resolveScopedCall resolves a scoped_identifier call like Type::method() or module::func().
// Also handles turbofish: collect::<Vec<String>>() by stripping type_arguments.
func resolveScopedCall(ctx *ResolveContext, funcNode *sitter.Node) string {
	fullPath := nodeContent(funcNode, ctx.Content)
	// Strip turbofish type arguments for resolution.
	fullPath = stripTurbofish(fullPath)
	parts := strings.Split(fullPath, "::")

	if len(parts) == 0 {
		return ""
	}

	// Try to resolve through uses map.
	if resolved, ok := ctx.Uses[parts[0]]; ok {
		// Replace first segment with resolved path.
		resolvedParts := append(strings.Split(resolved, "::"), parts[1:]...)
		resolvedPath := strings.Join(resolvedParts, "::")
		// Convert :: to . for QN format.
		return convertToQN(resolvedPath)
	}

	// Check if first segment is a known type in the registry.
	if len(parts) == 2 {
		typeName := parts[0]
		// Try local type.
		localTypeQN := ctx.ModuleQN + "." + typeName
		if ctx.Registry.LookupType(localTypeQN) != nil {
			return localTypeQN + "." + parts[1]
		}
		// Try uses-resolved type.
		if path, ok := ctx.Uses[typeName]; ok {
			return convertToQN(path) + "." + parts[1]
		}
		// Try as-is in registry.
		if ctx.Registry.LookupType(typeName) != nil {
			return typeName + "." + parts[1]
		}
	}

	// Look up as function in registry.
	funcQN := convertToQN(fullPath)
	if ctx.Registry.LookupFunc(funcQN) != nil {
		return funcQN
	}

	// Default: convert path to QN format.
	return funcQN
}

// resolveIdentCall resolves a bare identifier function call.
func resolveIdentCall(ctx *ResolveContext, name string) string {
	// Look up in current module context.
	localQN := ctx.ModuleQN + "." + name
	if ctx.Registry.LookupFunc(localQN) != nil {
		return localQN
	}

	// Look up in uses map.
	if path, ok := ctx.Uses[name]; ok {
		return convertToQN(path) + "." + name
	}

	// Try glob import prefixes: check if any glob module has this function.
	for _, prefix := range ctx.GlobImports {
		globQN := convertToQN(prefix+"::"+name) + "." + name
		if ctx.Registry.LookupFunc(globQN) != nil {
			return globQN
		}
		// Try simpler form: prefix.name
		simpleQN := convertToQN(prefix) + "." + name
		if ctx.Registry.LookupFunc(simpleQN) != nil {
			return simpleQN
		}
	}

	// Emit with local QN as best effort.
	return localQN
}

// resolveMethodCall resolves a method call like obj.method().
func resolveMethodCall(ctx *ResolveContext, callNode *sitter.Node, funcNode *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	operandNode := funcNode.ChildByFieldName("value")
	fieldNode := funcNode.ChildByFieldName("field")
	if operandNode == nil || fieldNode == nil {
		return
	}

	methodName := nodeContent(fieldNode, ctx.Content)

	// Evaluate receiver type.
	receiverType := EvalExprType(ctx, operandNode)
	baseType := DerefToBase(receiverType)

	if baseType != nil && baseType.Kind == typresolve.KindNamed {
		typeQN := baseType.Name

		// Check if the type is a trait-bounded type parameter.
		if ctx.TraitBounds != nil {
			if traitQN, ok := ctx.TraitBounds[typeQN]; ok {
				// Look up method on the bound trait.
				if m := LookupMethod(ctx.Registry, traitQN, methodName); m != nil {
					edge := buildCallEdge(ctx, callNode, m.QualifiedName, "resolver_trait_bound", fileHash, repoURL, filePath)
					*edges = append(*edges, edge)
					return
				}
				// Heuristic: use trait + method.
				calleeQN := traitQN + "." + methodName
				edge := buildCallEdge(ctx, callNode, calleeQN, "resolver_trait_bound", fileHash, repoURL, filePath)
				*edges = append(*edges, edge)
				return
			}
		}

		// Look up method via registry (including trait dispatch).
		if m := LookupMethod(ctx.Registry, typeQN, methodName); m != nil {
			edge := buildCallEdge(ctx, callNode, m.QualifiedName, "resolver_type_dispatch", fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
			return
		}
		// Heuristic: emit edge using receiver type + method.
		calleeQN := typeQN + "." + methodName
		edge := buildCallEdge(ctx, callNode, calleeQN, "resolver_type_dispatch", fileHash, repoURL, filePath)
		*edges = append(*edges, edge)
		return
	}

	// Check if the receiver is a type parameter with trait bounds (KindTypeParam).
	if baseType != nil && baseType.Kind == typresolve.KindTypeParam && ctx.TraitBounds != nil {
		if traitQN, ok := ctx.TraitBounds[baseType.Name]; ok {
			if m := LookupMethod(ctx.Registry, traitQN, methodName); m != nil {
				edge := buildCallEdge(ctx, callNode, m.QualifiedName, "resolver_trait_bound", fileHash, repoURL, filePath)
				*edges = append(*edges, edge)
				return
			}
			calleeQN := traitQN + "." + methodName
			edge := buildCallEdge(ctx, callNode, calleeQN, "resolver_trait_bound", fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
			return
		}
	}

	// Cannot resolve receiver type; skip.
}

// resolveMacroInvocation handles a macro_invocation node.
func resolveMacroInvocation(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Get macro name.
	macroNode := node.ChildByFieldName("macro")
	if macroNode == nil {
		if node.ChildCount() > 0 {
			macroNode = node.Child(0)
		}
	}
	if macroNode == nil {
		return
	}

	macroName := nodeContent(macroNode, ctx.Content)
	macroName = strings.TrimSuffix(macroName, "!")

	// Skip standard macros.
	if stdMacros[macroName] {
		return
	}

	// For user-defined macros: emit call edge.
	calleeQN := ctx.ModuleQN + "." + macroName
	if path, ok := ctx.Uses[macroName]; ok {
		calleeQN = convertToQN(path) + "." + macroName
	}

	edge := buildCallEdge(ctx, node, calleeQN, "resolver_macro", fileHash, repoURL, filePath)
	*edges = append(*edges, edge)
}

// processNestedFunction handles a nested function_item.
func processNestedFunction(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := nodeContent(nameNode, ctx.Content)

	origFuncQN := ctx.EnclosingFuncQN
	ctx.EnclosingFuncQN = ctx.ModuleQN + "." + funcName

	funcScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = funcScope

	// Extract trait bounds for nested function.
	origBounds := ctx.TraitBounds
	ctx.TraitBounds = extractTraitBounds(ctx, node)

	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindRustParams(ctx, paramsNode)
	}

	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	ctx.TraitBounds = origBounds
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = origFuncQN
}

// buildCallEdge creates a types.Edge from a resolved call.
func buildCallEdge(ctx *ResolveContext, callNode *sitter.Node, calleeQN string, strategy string, fileHash types.Hash, repoURL string, filePath string) types.Edge {
	// Source: enclosing function.
	srcPkgPath, srcFuncName, srcKind := splitQualifiedName(ctx.EnclosingFuncQN)
	sourceHash := types.ComputeNodeHash(repoURL, srcPkgPath, types.EmptyHash, srcFuncName, srcKind)

	// Target: callee.
	tgtPkgPath, tgtFuncName, tgtKind := splitQualifiedName(calleeQN)
	targetHash := types.ComputeNodeHash(repoURL, tgtPkgPath, types.EmptyHash, tgtFuncName, tgtKind)

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, typresolve.ProvenanceResolverResolved)

	return types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.Calls,
		Confidence:   typresolve.ResolverConfidence,
		Provenance:   typresolve.ProvenanceResolverResolved,
		CallSiteLine: int(callNode.StartPoint().Row) + 1,
		CallSiteCol:  int(callNode.StartPoint().Column),
		CallSiteFile: filePath,
	}
}

// splitQualifiedName splits a qualified name into module path, symbol name, and kind.
// Handles Rust QN formats where "." separates components:
// "module.Function" -> ("module", "Function", "function")
// "module.Type.Method" -> ("module", "Type.Method", "method")
func splitQualifiedName(qn string) (modulePath string, symbolName string, kind string) {
	lastDot := strings.LastIndex(qn, ".")
	if lastDot == -1 {
		return "", qn, types.KindFunction
	}
	secondLastDot := strings.LastIndex(qn[:lastDot], ".")
	if secondLastDot >= 0 {
		// Two dots: module.Type.Method
		return qn[:secondLastDot], qn[secondLastDot+1:], types.KindMethod
	}
	// One dot: module.Function
	return qn[:lastDot], qn[lastDot+1:], types.KindFunction
}

// convertToQN converts a Rust path with :: separators to the dot-based QN format
// used internally. E.g., "crate::module::Type" -> "crate::module.Type"
// Only the last :: separator before the symbol becomes a dot.
func convertToQN(path string) string {
	// The QN format uses :: for module hierarchy and . for the symbol boundary.
	// Convert the last :: to a dot.
	lastSep := strings.LastIndex(path, "::")
	if lastSep < 0 {
		return path
	}
	return path[:lastSep] + "." + path[lastSep+2:]
}

// resolveTypeQN resolves a type name to its qualified name using the context.
func resolveTypeQN(ctx *ResolveContext, typeName string) string {
	// Check uses map.
	if path, ok := ctx.Uses[typeName]; ok {
		return convertToQN(path + "::" + typeName)
	}
	// Default to module-local.
	return ctx.ModuleQN + "." + typeName
}

// bindRustParams binds function/method parameters into the current scope.
func bindRustParams(ctx *ResolveContext, paramsNode *sitter.Node) {
	for i := 0; i < int(paramsNode.ChildCount()); i++ {
		child := paramsNode.Child(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "parameter":
			patternNode := child.ChildByFieldName("pattern")
			typeNode := child.ChildByFieldName("type")
			if patternNode != nil {
				var paramType *typresolve.Type
				if typeNode != nil {
					paramType = ParseTypeNode(typeNode, ctx.Content, ctx.ModuleQN, ctx.Uses)
					// If the type is a type param with a known trait bound,
					// bind it as Named(paramName) so trait-bound dispatch works.
					if paramType.Kind == typresolve.KindNamed && ctx.TraitBounds != nil {
						if _, hasBound := ctx.TraitBounds[paramType.Name]; hasBound {
							// Keep as Named with the type param name.
							// resolveMethodCall will look up the trait bound.
						}
					}
				} else {
					paramType = typresolve.Unknown()
				}
				name := nodeContent(patternNode, ctx.Content)
				if name != "_" && name != "self" && name != "&self" && name != "&mut self" {
					ctx.Scope.Bind(name, paramType)
				}
			}

		case "self_parameter":
			// self is already bound by processMethodInImpl.
			continue
		}
	}
}
