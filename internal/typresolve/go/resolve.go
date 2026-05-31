package goresolve

import (
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

// ResolveCallsInFile walks a Go file's AST resolving call expressions and
// emitting edges. Uses the shared registry, per-file scope chain, and
// import map. This is the Go port of go_lsp_process_file from the C
// reference implementation.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	var edges []types.Edge

	// Pass 1: Process package-level var/const declarations into root scope.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "var_declaration", "const_declaration":
			ProcessStatement(ctx, child)
		}
	}

	// Pass 2: Process function and method declarations.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "function_declaration":
			processFuncDecl(ctx, child, &edges, fileHash, repoURL, filePath)
		case "method_declaration":
			processMethodDecl(ctx, child, &edges, fileHash, repoURL, filePath)
		}
	}

	return edges
}

// processFuncDecl processes a function declaration node.
func processFuncDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := nodeContent(nameNode, ctx.Content)
	ctx.EnclosingFuncQN = ctx.PkgQN + "." + funcName

	// Push function scope.
	funcScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = funcScope

	// Bind parameters into scope.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindParams(ctx, paramsNode)
	}

	// Bind named return values into scope.
	resultNode := node.ChildByFieldName("result")
	if resultNode != nil {
		bindReturnValues(ctx, resultNode)
	}

	// Resolve calls in the function body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop scope.
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = ""
}

// processMethodDecl processes a method declaration node.
func processMethodDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeContent(nameNode, ctx.Content)

	// Extract receiver type to build qualified name.
	receiverNode := node.ChildByFieldName("receiver")
	receiverTypeName := ""
	if receiverNode != nil {
		receiverTypeName = extractReceiverTypeName(receiverNode, ctx.Content)
	}

	if receiverTypeName != "" {
		ctx.EnclosingFuncQN = ctx.PkgQN + "." + receiverTypeName + "." + methodName
	} else {
		ctx.EnclosingFuncQN = ctx.PkgQN + "." + methodName
	}

	// Push function scope.
	funcScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = funcScope

	// Bind receiver into scope.
	if receiverNode != nil {
		bindReceiver(ctx, receiverNode)
	}

	// Bind parameters into scope.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindParams(ctx, paramsNode)
	}

	// Bind named return values into scope.
	resultNode := node.ChildByFieldName("result")
	if resultNode != nil {
		bindReturnValues(ctx, resultNode)
	}

	// Resolve calls in the method body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Pop scope.
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

	if node.Type() == "call_expression" {
		if rc := resolveCallExpr(ctx, node); rc != nil {
			edge := buildCallEdge(ctx, node, rc, fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}
	}

	// Gap 3: Handle select statement with channel receive binding.
	if node.Type() == "select_statement" {
		resolveSelectStatement(ctx, node, edges, fileHash, repoURL, filePath)
		return
	}

	// Handle scoped blocks.
	needsPop := false
	switch node.Type() {
	case "block", "if_statement", "for_statement", "switch_statement":
		childScope := typresolve.NewScope(ctx.Scope)
		ctx.Scope = childScope
		needsPop = true
	}

	// Handle for-range clause.
	if node.Type() == "for_statement" {
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "range_clause" {
				ProcessStatement(ctx, child)
			}
		}
	}

	// Handle if statement initializer.
	if node.Type() == "if_statement" {
		initNode := node.ChildByFieldName("initializer")
		if initNode != nil {
			ProcessStatement(ctx, initNode)
		}
	}

	// Handle switch statement initializer.
	if node.Type() == "switch_statement" {
		initNode := node.ChildByFieldName("initializer")
		if initNode != nil {
			ProcessStatement(ctx, initNode)
		}
	}

	// Handle type switch: per-case scope narrowing.
	if node.Type() == "type_switch_statement" {
		resolveTypeSwitchStatement(ctx, node, edges, fileHash, repoURL, filePath)
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

// resolveTypeSwitchStatement handles type_switch_statement with per-case
// scope narrowing.
func resolveTypeSwitchStatement(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Find the type switch value and alias.
	var switchVarName string
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Type() == "type_switch_statement" || child.Type() == "short_var_declaration" {
			// Look for x := expr.(type) pattern.
			left := child.ChildByFieldName("left")
			if left != nil && left.Type() == "expression_list" && left.NamedChildCount() > 0 {
				first := left.NamedChild(0)
				if first != nil && first.Type() == "identifier" {
					switchVarName = nodeContent(first, ctx.Content)
				}
			}
		}
	}

	// Process each case clause with type narrowing.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "type_case_clause":
			caseScope := typresolve.NewScope(ctx.Scope)
			origScope := ctx.Scope
			ctx.Scope = caseScope

			// Narrow the switch variable to the case type.
			if switchVarName != "" {
				typeListNode := child.ChildByFieldName("type")
				if typeListNode != nil {
					caseType := ParseTypeNode(typeListNode, ctx.Content, ctx.PkgQN, ctx.Imports)
					if caseType != nil {
						ctx.Scope.Bind(switchVarName, caseType)
					}
				}
			}

			// Resolve calls in the case body.
			for j := 0; j < int(child.NamedChildCount()); j++ {
				caseChild := child.NamedChild(j)
				if caseChild != nil {
					resolveCallsInNode(ctx, caseChild, edges, fileHash, repoURL, filePath)
				}
			}

			ctx.Scope = origScope

		case "default_case":
			for j := 0; j < int(child.NamedChildCount()); j++ {
				caseChild := child.NamedChild(j)
				if caseChild != nil {
					resolveCallsInNode(ctx, caseChild, edges, fileHash, repoURL, filePath)
				}
			}
		}
	}
}

// resolveCallExpr resolves a call_expression node to a resolvedCall.
func resolveCallExpr(ctx *ResolveContext, node *sitter.Node) *resolvedCall {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		return nil
	}

	switch fnNode.Type() {
	case "selector_expression":
		return resolveSelectorCall(ctx, fnNode)
	case "identifier":
		return resolveIdentifierCall(ctx, fnNode)
	default:
		return nil
	}
}

// resolveSelectorCall resolves a selector expression call (e.g. pkg.Func or obj.Method).
func resolveSelectorCall(ctx *ResolveContext, fnNode *sitter.Node) *resolvedCall {
	operand := fnNode.ChildByFieldName("operand")
	field := fnNode.ChildByFieldName("field")
	if operand == nil || field == nil {
		return nil
	}

	fieldName := nodeContent(field, ctx.Content)

	// Check if operand is an import alias.
	if operand.Type() == "identifier" {
		alias := nodeContent(operand, ctx.Content)
		if pkgQN, ok := ResolveImport(ctx.Imports, alias); ok {
			calleeQN := pkgQN + "." + fieldName
			if ctx.Registry.LookupSymbol(pkgQN, fieldName) != nil {
				return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_import"}
			}
			// Even if not in registry, emit the edge with the resolved QN.
			return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_import"}
		}
	}

	// Evaluate operand type for method dispatch.
	base := EvalExprType(ctx, operand)
	if base == nil || base.Kind == typresolve.KindUnknown {
		// Heuristic fallback: use operand text as type name for the edge.
		// This produces edges like "varName.MethodName" which create phantom
		// nodes but maintain graph connectivity. Lower precision than typed
		// resolution, but critical for coverage.
		operandText := nodeContent(operand, ctx.Content)
		if operandText != "" && operandText != "_" {
			heuristicQN := operandText + "." + fieldName
			return &resolvedCall{calleeQN: heuristicQN, strategy: "resolver_heuristic"}
		}
		return nil
	}

	// Auto-deref pointer.
	if base.Kind == typresolve.KindPointer {
		base = base.Deref()
	}

	if base.Kind == typresolve.KindNamed {
		typeQN := base.Name
		// Look up method on the type.
		if m := LookupFieldOrMethod(ctx.Registry, typeQN, fieldName); m != nil {
			return &resolvedCall{calleeQN: m.QualifiedName, strategy: "resolver_type_dispatch"}
		}

		// Gap 2: Interface satisfaction resolution.
		// If the receiver type is an interface, try to find a single
		// concrete implementer and resolve to its method.
		rt := ctx.Registry.LookupType(typeQN)
		if rt != nil && rt.IsInterface && len(rt.MethodNames) > 0 {
			if concreteQN := findSoleImplementer(ctx.Registry, rt); concreteQN != "" {
				concreteMethod := ctx.Registry.LookupMethod(concreteQN, fieldName)
				if concreteMethod != nil {
					return &resolvedCall{calleeQN: concreteMethod.QualifiedName, strategy: "resolver_interface_resolve"}
				}
			}
			// Fallback: emit as interface dispatch.
			return &resolvedCall{calleeQN: typeQN + "." + fieldName, strategy: "resolver_interface_dispatch"}
		}

		// Construct method QN even if not found in registry.
		return &resolvedCall{calleeQN: typeQN + "." + fieldName, strategy: "resolver_type_dispatch"}
	}

	return nil
}

// resolveIdentifierCall resolves a simple identifier call (e.g. myFunc()).
func resolveIdentifierCall(ctx *ResolveContext, fnNode *sitter.Node) *resolvedCall {
	name := nodeContent(fnNode, ctx.Content)

	// Skip builtins.
	if IsBuiltinFunc(name) {
		return nil
	}

	// Look up as package-local function.
	calleeQN := ctx.PkgQN + "." + name
	if ctx.Registry.LookupSymbol(ctx.PkgQN, name) != nil {
		return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_direct"}
	}

	// Emit edge even if not in registry (might be in same package but
	// not yet registered).
	return &resolvedCall{calleeQN: calleeQN, strategy: "resolver_direct"}
}

// buildCallEdge creates a types.Edge from a resolved call.
func buildCallEdge(ctx *ResolveContext, callNode *sitter.Node, rc *resolvedCall, fileHash types.Hash, repoURL string, filePath string) types.Edge {
	// Compute source hash from the enclosing function.
	srcPkgPath, srcFuncName, srcKind := splitEnclosingFunc(ctx.EnclosingFuncQN, ctx.PkgQN)
	sourceHash := types.ComputeNodeHash(repoURL, srcPkgPath, types.EmptyHash, srcFuncName, srcKind)

	// Compute target hash from the callee.
	tgtPkgPath, tgtFuncName, tgtKind := splitQualifiedName(rc.calleeQN)
	targetHash := types.ComputeNodeHash(repoURL, tgtPkgPath, types.EmptyHash, tgtFuncName, tgtKind)

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, typresolve.ProvenanceResolverResolved)

	confidence := typresolve.ResolverConfidence
	if rc.strategy == "resolver_heuristic" {
		confidence = 0.6 // lower confidence for heuristic edges
	}

	return types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.Calls,
		Confidence:   confidence,
		Provenance:   typresolve.ProvenanceResolverResolved,
		CallSiteLine: int(callNode.StartPoint().Row) + 1, // 1-indexed
		CallSiteCol:  int(callNode.StartPoint().Column),   // 0-indexed
		CallSiteFile: filePath,
	}
}

// splitEnclosingFunc splits the enclosing function QN into package path,
// function name, and kind. Handles both "pkg.Func" and "pkg.Type.Method".
func splitEnclosingFunc(funcQN string, pkgQN string) (pkgPath string, funcName string, kind string) {
	// Remove pkgQN prefix to get the rest.
	if len(funcQN) > len(pkgQN)+1 {
		rest := funcQN[len(pkgQN)+1:] // after "pkgQN."
		// Check if it's a method (contains another dot).
		for i := 0; i < len(rest); i++ {
			if rest[i] == '.' {
				// It's a method: Type.Method
				return pkgQN, rest, types.KindMethod
			}
		}
		return pkgQN, rest, types.KindFunction
	}
	return pkgQN, funcQN, types.KindFunction
}

// splitQualifiedName splits a callee qualified name into package path,
// symbol name, and kind. Handles formats like "pkg.Func" and
// "pkg.Type.Method".
func splitQualifiedName(qn string) (pkgPath string, symbolName string, kind string) {
	// Find the first "." to separate package from symbol.
	// QNs can be like "fmt.Println" or "net/http.Handler.ServeHTTP"
	// We need to find the package boundary.

	// Look for the last segment(s) after the package path.
	// Strategy: the package path doesn't contain uppercase letters at the
	// boundary, so find the first dot followed by an uppercase letter.
	lastDot := -1
	secondLastDot := -1
	for i := len(qn) - 1; i >= 0; i-- {
		if qn[i] == '.' {
			if lastDot == -1 {
				lastDot = i
			} else if secondLastDot == -1 {
				secondLastDot = i
				break
			}
		}
	}

	if lastDot == -1 {
		// No dots: just a name.
		return "", qn, types.KindFunction
	}

	if secondLastDot >= 0 {
		// Two dots: could be pkg.Type.Method
		pkgPath = qn[:secondLastDot]
		symbolName = qn[secondLastDot+1:]
		return pkgPath, symbolName, types.KindMethod
	}

	// One dot: pkg.Func
	return qn[:lastDot], qn[lastDot+1:], types.KindFunction
}

// bindParams binds function parameters into the current scope.
func bindParams(ctx *ResolveContext, paramsNode *sitter.Node) {
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		paramDecl := paramsNode.NamedChild(i)
		if paramDecl == nil || paramDecl.Type() != "parameter_declaration" {
			continue
		}

		typeNode := paramDecl.ChildByFieldName("type")
		var paramType *typresolve.Type
		if typeNode != nil {
			// Handle variadic parameters: ...T becomes []T.
			if typeNode.Type() == "variadic_parameter_declaration" || nodeContent(typeNode, ctx.Content) == "..." {
				// Look for the actual type after "...".
				for j := 0; j < int(paramDecl.ChildCount()); j++ {
					child := paramDecl.Child(j)
					if child != nil && child.Type() != "identifier" && child.Type() != "..." {
						inner := ParseTypeNode(child, ctx.Content, ctx.PkgQN, ctx.Imports)
						if inner != nil {
							paramType = typresolve.Slice(inner)
							break
						}
					}
				}
				if paramType == nil {
					paramType = typresolve.Unknown()
				}
			} else {
				paramType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
			}
		}
		if paramType == nil {
			paramType = typresolve.Unknown()
		}

		// Bind each identifier in the parameter declaration.
		for j := 0; j < int(paramDecl.ChildCount()); j++ {
			child := paramDecl.Child(j)
			if child != nil && child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				if name != "_" {
					ctx.Scope.Bind(name, paramType)
				}
			}
		}
	}
}

// bindReturnValues binds named return values into the current scope.
func bindReturnValues(ctx *ResolveContext, resultNode *sitter.Node) {
	if resultNode.Type() != "parameter_list" {
		return
	}

	for i := 0; i < int(resultNode.NamedChildCount()); i++ {
		paramDecl := resultNode.NamedChild(i)
		if paramDecl == nil || paramDecl.Type() != "parameter_declaration" {
			continue
		}

		typeNode := paramDecl.ChildByFieldName("type")
		var retType *typresolve.Type
		if typeNode != nil {
			retType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
		}
		if retType == nil {
			retType = typresolve.Unknown()
		}

		// Bind named return identifiers.
		for j := 0; j < int(paramDecl.ChildCount()); j++ {
			child := paramDecl.Child(j)
			if child != nil && child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				if name != "_" {
					ctx.Scope.Bind(name, retType)
				}
			}
		}
	}
}

// bindReceiver binds the method receiver into the current scope.
func bindReceiver(ctx *ResolveContext, receiverNode *sitter.Node) {
	// receiver is a parameter_list with one parameter_declaration.
	for i := 0; i < int(receiverNode.NamedChildCount()); i++ {
		paramDecl := receiverNode.NamedChild(i)
		if paramDecl == nil || paramDecl.Type() != "parameter_declaration" {
			continue
		}

		typeNode := paramDecl.ChildByFieldName("type")
		var recvType *typresolve.Type
		if typeNode != nil {
			recvType = ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
		}
		if recvType == nil {
			recvType = typresolve.Unknown()
		}

		// Bind the receiver name.
		for j := 0; j < int(paramDecl.ChildCount()); j++ {
			child := paramDecl.Child(j)
			if child != nil && child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				if name != "_" {
					ctx.Scope.Bind(name, recvType)
					return
				}
			}
		}
	}
}

// extractReceiverTypeName extracts the type name from a receiver
// parameter_list node. Returns the bare type name (without pointer).
func extractReceiverTypeName(receiverNode *sitter.Node, content []byte) string {
	for i := 0; i < int(receiverNode.NamedChildCount()); i++ {
		paramDecl := receiverNode.NamedChild(i)
		if paramDecl == nil || paramDecl.Type() != "parameter_declaration" {
			continue
		}

		typeNode := paramDecl.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}

		return extractTypeName(typeNode, content)
	}
	return ""
}

// resolveSelectStatement handles select statements with per-case scope
// and channel receive variable binding.
//
// Gap 3: For each communication_case, detects receive_statement nodes,
// evaluates the channel type, extracts the element type, and binds
// the received variable(s) into the case scope.
func resolveSelectStatement(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}

		if child.Type() != "communication_case" && child.Type() != "default_case" {
			continue
		}

		// Push case scope.
		caseScope := typresolve.NewScope(ctx.Scope)
		origScope := ctx.Scope
		ctx.Scope = caseScope

		// Process children of the communication_case.
		for j := 0; j < int(child.NamedChildCount()); j++ {
			caseChild := child.NamedChild(j)
			if caseChild == nil {
				continue
			}

			if caseChild.Type() == "receive_statement" {
				// receive_statement: left := <-right
				// The right field is the channel expression.
				left := caseChild.ChildByFieldName("left")
				right := caseChild.ChildByFieldName("right")
				if right != nil {
					rightType := EvalExprType(ctx, right)
					// Extract element type from channel.
					recvType := rightType
					if rightType != nil && rightType.Kind == typresolve.KindChannel && rightType.Elem != nil {
						recvType = rightType.Elem
					}
					if recvType == nil {
						recvType = typresolve.Unknown()
					}

					// Bind left-hand variables.
					if left != nil {
						if left.Type() == "expression_list" {
							for k := 0; k < int(left.NamedChildCount()); k++ {
								varNode := left.NamedChild(k)
								if varNode == nil || varNode.Type() != "identifier" {
									continue
								}
								varName := nodeContent(varNode, ctx.Content)
								if varName == "_" {
									continue
								}
								if k == 0 {
									ctx.Scope.Bind(varName, recvType)
								} else {
									// Second variable is the ok bool.
									ctx.Scope.Bind(varName, typresolve.Builtin("bool"))
								}
							}
						} else if left.Type() == "identifier" {
							varName := nodeContent(left, ctx.Content)
							if varName != "_" {
								ctx.Scope.Bind(varName, recvType)
							}
						}
					}
				}
			} else {
				// Body statements: recurse for call resolution.
				resolveCallsInNode(ctx, caseChild, edges, fileHash, repoURL, filePath)
			}
		}

		// Pop case scope.
		ctx.Scope = origScope
	}
}

// findSoleImplementer scans all registered types to find a single
// concrete type that structurally satisfies the given interface.
// Returns the qualified name of the sole implementer, or empty string
// if zero or multiple implementers are found.
//
// Gap 2: Interface satisfaction resolution. When a method is called on
// an interface type and exactly one concrete type implements all the
// interface's methods, we resolve to that concrete type's method.
func findSoleImplementer(reg *typresolve.Registry, iface *typresolve.RegisteredType) string {
	if len(iface.MethodNames) == 0 {
		return ""
	}

	// We need to iterate all types. Since we cannot modify the typresolve
	// package, we use a workaround: check each method name combination.
	// The registry's method map has keys "receiverQN.methodName". We scan
	// the first interface method to find candidate types, then verify the
	// rest.

	// Unfortunately, we cannot iterate the registry's internal maps from
	// outside the package. Instead, we maintain a package-level type list
	// populated during BuildRegistry. See registeredTypeQNs below.
	var soleImpl string
	implCount := 0

	for _, candidateQN := range registeredTypeQNs {
		// Skip interfaces and aliases.
		cand := reg.LookupType(candidateQN)
		if cand == nil || cand.IsInterface || cand.AliasOf != "" {
			continue
		}

		// Check if candidate has all interface methods.
		satisfies := true
		for _, methodName := range iface.MethodNames {
			if reg.LookupMethod(candidateQN, methodName) == nil {
				satisfies = false
				break
			}
		}
		if satisfies {
			soleImpl = candidateQN
			implCount++
			if implCount > 1 {
				return "" // Multiple implementers: cannot resolve.
			}
		}
	}

	if implCount == 1 {
		return soleImpl
	}
	return ""
}

// registeredTypeQNs tracks all type qualified names registered during
// BuildRegistry. Used by findSoleImplementer for interface satisfaction
// scanning. Set by InitWorkspace via BuildRegistry.
var registeredTypeQNs []string

// extractTypeName extracts the bare type name from a type node,
// stripping pointers and parentheses.
func extractTypeName(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "type_identifier":
		return node.Content(content)
	case "pointer_type":
		if node.NamedChildCount() > 0 {
			return extractTypeName(node.NamedChild(0), content)
		}
	case "parenthesized_type":
		if node.NamedChildCount() > 0 {
			return extractTypeName(node.NamedChild(0), content)
		}
	case "generic_type":
		typeChild := node.ChildByFieldName("type")
		if typeChild != nil {
			return extractTypeName(typeChild, content)
		}
	}
	return ""
}
