package csresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveCallsInFile walks a C# file's AST resolving call expressions and
// emitting edges. Uses the shared registry, per-file scope chain, using map,
// and namespace stack. Processes namespace declarations, class/struct/interface
// declarations, method/constructor declarations, invocations, and object creation.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	var edges []types.Edge
	resolveNode(ctx, root, &edges, fileHash, repoURL, filePath)
	return edges
}

// resolveNode recursively walks an AST node resolving declarations and calls.
func resolveNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "namespace_declaration":
		processNamespaceDecl(ctx, node, edges, fileHash, repoURL, filePath)
		return

	case "file_scoped_namespace_declaration":
		processFileScopedNamespace(ctx, node, edges, fileHash, repoURL, filePath)
		return

	case "class_declaration", "struct_declaration", "interface_declaration", "enum_declaration", "record_declaration":
		processTypeDecl(ctx, node, edges, fileHash, repoURL, filePath)
		return

	case "method_declaration", "constructor_declaration":
		processMethodDecl(ctx, node, edges, fileHash, repoURL, filePath)
		return

	case "invocation_expression":
		if rc := resolveInvocation(ctx, node); rc != nil {
			edge := buildCSharpCallEdge(ctx, node, rc, fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}

	case "object_creation_expression":
		if rc := resolveObjectCreation(ctx, node); rc != nil {
			edge := buildCSharpCallEdge(ctx, node, rc, fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}

	case "implicit_object_creation_expression":
		// Target-typed new: type inferred from context. Skip for now.
	}

	// Process statement for scope bindings (variable declarations, etc.).
	ProcessStatement(ctx, node)

	// Recurse into children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		resolveNode(ctx, child, edges, fileHash, repoURL, filePath)
	}
}

// processNamespaceDecl handles namespace_declaration nodes.
func processNamespaceDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	var nsName string
	if nameNode != nil {
		nsName = nodeContent(nameNode, ctx.Content)
	}

	// Push namespace.
	ctx.NamespaceStack = append(ctx.NamespaceStack, nsName)
	origModuleQN := ctx.ModuleQN
	ctx.ModuleQN = currentNamespace(ctx.NamespaceStack)

	// Recurse into namespace body.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Type() == "declaration_list" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				resolveNode(ctx, child.NamedChild(j), edges, fileHash, repoURL, filePath)
			}
		}
	}

	// Pop namespace.
	ctx.NamespaceStack = ctx.NamespaceStack[:len(ctx.NamespaceStack)-1]
	ctx.ModuleQN = origModuleQN
}

// processFileScopedNamespace handles file_scoped_namespace_declaration.
func processFileScopedNamespace(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	var nsName string
	if nameNode != nil {
		nsName = nodeContent(nameNode, ctx.Content)
	}

	ctx.NamespaceStack = append(ctx.NamespaceStack, nsName)
	ctx.ModuleQN = currentNamespace(ctx.NamespaceStack)

	// File-scoped namespace: all remaining children are in this namespace.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil {
			resolveNode(ctx, child, edges, fileHash, repoURL, filePath)
		}
	}

	ctx.NamespaceStack = ctx.NamespaceStack[:len(ctx.NamespaceStack)-1]
}

// processTypeDecl handles class/struct/interface/enum/record declarations.
func processTypeDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	typeName := nodeContent(nameNode, ctx.Content)

	// Build the full class QN.
	var classQN string
	ns := currentNamespace(ctx.NamespaceStack)
	if ctx.EnclosingClassQN != "" {
		// Nested type.
		classQN = ctx.EnclosingClassQN + "." + typeName
	} else if ns != "" {
		classQN = ns + "." + typeName
	} else {
		classQN = typeName
	}

	// Extract base class from base_list.
	var baseQN string
	baseList := node.ChildByFieldName("bases")
	if baseList == nil {
		// Try to find base_list among children.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "base_list" {
				baseList = child
				break
			}
		}
	}
	if baseList != nil && baseList.NamedChildCount() > 0 {
		firstBase := baseList.NamedChild(0)
		if firstBase != nil {
			baseName := nodeContent(firstBase, ctx.Content)
			baseName = strings.TrimSpace(baseName)
			// Strip generic args for base class QN.
			if idx := strings.IndexByte(baseName, '<'); idx >= 0 {
				baseName = baseName[:idx]
			}
			baseQN = ResolveTypeName(baseName, ns, ctx.Usings, ctx.Registry, classQN, ctx.ModuleQN)
		}
	}

	// Save and set enclosing class context.
	origClassQN := ctx.EnclosingClassQN
	origBaseQN := ctx.EnclosingBaseQN
	ctx.EnclosingClassQN = classQN
	ctx.EnclosingBaseQN = baseQN

	// Process declaration body.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && child.Type() == "declaration_list" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				resolveNode(ctx, child.NamedChild(j), edges, fileHash, repoURL, filePath)
			}
		}
	}

	// Restore enclosing class context.
	ctx.EnclosingClassQN = origClassQN
	ctx.EnclosingBaseQN = origBaseQN
}

// processMethodDecl handles method_declaration and constructor_declaration nodes.
func processMethodDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	var methodName string
	if nameNode != nil {
		methodName = nodeContent(nameNode, ctx.Content)
	} else if node.Type() == "constructor_declaration" {
		// Constructor name is the class name.
		methodName = ".ctor"
	}

	// Build enclosing function QN.
	origFuncQN := ctx.EnclosingFuncQN
	if ctx.EnclosingClassQN != "" {
		ctx.EnclosingFuncQN = ctx.EnclosingClassQN + "." + methodName
	} else {
		ns := currentNamespace(ctx.NamespaceStack)
		if ns != "" {
			ctx.EnclosingFuncQN = ns + "." + methodName
		} else {
			ctx.EnclosingFuncQN = methodName
		}
	}

	// Push method scope.
	origScope := ctx.Scope
	ctx.Scope = typresolve.NewScope(origScope)

	// Bind parameters into scope.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindCSharpParams(ctx, paramsNode)
	}

	// Process body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveMethodBody(ctx, body, edges, fileHash, repoURL, filePath)
	}

	// Also check for expression body (=>).
	exprBody := node.ChildByFieldName("expression_body")
	if exprBody == nil {
		// Try arrow_expression_clause child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "arrow_expression_clause" {
				exprBody = child
				break
			}
		}
	}
	if exprBody != nil {
		resolveMethodBody(ctx, exprBody, edges, fileHash, repoURL, filePath)
	}

	// Pop scope.
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = origFuncQN
}

// resolveMethodBody resolves calls within a method body node.
func resolveMethodBody(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	if node == nil {
		return
	}

	// Process statement for scope bindings.
	ProcessStatement(ctx, node)

	switch node.Type() {
	case "invocation_expression":
		if rc := resolveInvocation(ctx, node); rc != nil {
			edge := buildCSharpCallEdge(ctx, node, rc, fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}

	case "object_creation_expression":
		if rc := resolveObjectCreation(ctx, node); rc != nil {
			edge := buildCSharpCallEdge(ctx, node, rc, fileHash, repoURL, filePath)
			*edges = append(*edges, edge)
		}

	case "block":
		// New scope for blocks.
		origScope := ctx.Scope
		ctx.Scope = typresolve.NewScope(origScope)
		for i := 0; i < int(node.NamedChildCount()); i++ {
			resolveMethodBody(ctx, node.NamedChild(i), edges, fileHash, repoURL, filePath)
		}
		ctx.Scope = origScope
		return
	}

	// Recurse into children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		resolveMethodBody(ctx, node.NamedChild(i), edges, fileHash, repoURL, filePath)
	}
}

// resolveInvocation resolves an invocation_expression node to a callee QN.
func resolveInvocation(ctx *ResolveContext, node *sitter.Node) *csResolvedCall {
	fnNode := node.ChildByFieldName("function")
	if fnNode == nil {
		if node.NamedChildCount() > 0 {
			fnNode = node.NamedChild(0)
		}
	}
	if fnNode == nil {
		return nil
	}

	switch fnNode.Type() {
	case "identifier":
		name := nodeContent(fnNode, ctx.Content)
		if IsBuiltinFunc(name) {
			return nil
		}
		// Try as method on enclosing class (implicit this).
		if ctx.EnclosingClassQN != "" {
			if m := LookupMethod(ctx.Registry, ctx.EnclosingClassQN, name); m != nil {
				return &csResolvedCall{calleeQN: m.QualifiedName}
			}
		}
		// Try as function in module.
		ns := currentNamespace(ctx.NamespaceStack)
		if ns != "" {
			candidate := ns + "." + name
			if ctx.Registry.LookupFunc(candidate) != nil {
				return &csResolvedCall{calleeQN: candidate}
			}
		}
		// Try scope lookup for delegate invocations.
		return nil

	case "member_access_expression":
		return resolveMemberAccessCall(ctx, fnNode)

	case "conditional_access_expression":
		// x?.Method() - try to resolve the member binding.
		if fnNode.NamedChildCount() >= 2 {
			bindingNode := fnNode.NamedChild(1)
			if bindingNode != nil && bindingNode.Type() == "member_binding_expression" {
				exprNode := fnNode.NamedChild(0)
				if exprNode != nil {
					base := EvalExprType(ctx, exprNode)
					if base != nil && base.Kind == typresolve.KindNamed {
						nameNode := bindingNode.ChildByFieldName("name")
						if nameNode == nil && bindingNode.NamedChildCount() > 0 {
							nameNode = bindingNode.NamedChild(0)
						}
						if nameNode != nil {
							memberName := nodeContent(nameNode, ctx.Content)
							memberName = stripGenericSuffix(memberName)
							if m := LookupMethod(ctx.Registry, base.Name, memberName); m != nil {
								return &csResolvedCall{calleeQN: m.QualifiedName}
							}
						}
					}
				}
			}
		}
		return nil

	case "generic_name":
		// Method<T>() - strip generic args and resolve as identifier.
		name := nodeContent(fnNode, ctx.Content)
		name = stripGenericSuffix(name)
		if ctx.EnclosingClassQN != "" {
			if m := LookupMethod(ctx.Registry, ctx.EnclosingClassQN, name); m != nil {
				return &csResolvedCall{calleeQN: m.QualifiedName}
			}
		}
		return nil
	}

	return nil
}

// resolveMemberAccessCall resolves a member_access_expression used as a call target.
func resolveMemberAccessCall(ctx *ResolveContext, node *sitter.Node) *csResolvedCall {
	exprNode := node.ChildByFieldName("expression")
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	memberName := nodeContent(nameNode, ctx.Content)
	memberName = stripGenericSuffix(memberName)

	if exprNode == nil {
		return nil
	}

	// Evaluate the receiver type.
	base := EvalExprType(ctx, exprNode)

	if base != nil && base.Kind == typresolve.KindNamed {
		// Look up method on the receiver type.
		if m := LookupMethod(ctx.Registry, base.Name, memberName); m != nil {
			return &csResolvedCall{calleeQN: m.QualifiedName}
		}
		// Try extension method.
		if m := LookupExtensionMethod(ctx.Registry, base.Name, memberName); m != nil {
			return &csResolvedCall{calleeQN: m.QualifiedName}
		}
	}

	// If base is unknown, try treating the expression as a type name (static call).
	if exprNode.Type() == "identifier" || exprNode.Type() == "qualified_name" {
		typeName := nodeContent(exprNode, ctx.Content)
		// Resolve through usings.
		ns := currentNamespace(ctx.NamespaceStack)
		resolvedType := ResolveTypeName(typeName, ns, ctx.Usings, ctx.Registry, ctx.EnclosingClassQN, ctx.ModuleQN)
		if m := LookupMethod(ctx.Registry, resolvedType, memberName); m != nil {
			return &csResolvedCall{calleeQN: m.QualifiedName}
		}
		// Try as static method: TypeName.MethodName
		candidate := resolvedType + "." + memberName
		if ctx.Registry.LookupFunc(candidate) != nil {
			return &csResolvedCall{calleeQN: candidate}
		}
	}

	return nil
}

// resolveObjectCreation resolves a new Type(...) expression.
func resolveObjectCreation(ctx *ResolveContext, node *sitter.Node) *csResolvedCall {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil
	}

	typeName := nodeContent(typeNode, ctx.Content)
	typeName = stripGenericSuffix(typeName)

	// Resolve type name.
	ns := currentNamespace(ctx.NamespaceStack)
	resolvedType := ResolveTypeName(typeName, ns, ctx.Usings, ctx.Registry, ctx.EnclosingClassQN, ctx.ModuleQN)

	// Look up constructor.
	if m := LookupMethod(ctx.Registry, resolvedType, csShortName(resolvedType)); m != nil {
		return &csResolvedCall{calleeQN: m.QualifiedName}
	}
	// If no explicit constructor, the type itself is the target.
	if ctx.Registry.LookupType(resolvedType) != nil {
		return &csResolvedCall{calleeQN: resolvedType + "..ctor"}
	}

	return nil
}

// bindCSharpParams binds method parameters into the current scope.
func bindCSharpParams(ctx *ResolveContext, paramsNode *sitter.Node) {
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		param := paramsNode.NamedChild(i)
		if param == nil || param.Type() != "parameter" {
			continue
		}

		nameNode := param.ChildByFieldName("name")
		typeNode := param.ChildByFieldName("type")

		if nameNode == nil {
			continue
		}

		name := nodeContent(nameNode, ctx.Content)
		if name == "" {
			continue
		}

		var paramType *typresolve.Type
		if typeNode != nil {
			ns := currentNamespace(ctx.NamespaceStack)
			paramType = ParseTypeNode(typeNode, ctx.Content, ns, ctx.Usings, ctx.Registry)
		}
		if paramType == nil {
			paramType = typresolve.Unknown()
		}

		ctx.Scope.Bind(name, paramType)
	}
}

// csResolvedCall holds the result of resolving a C# call expression.
type csResolvedCall struct {
	calleeQN string
}

// buildCSharpCallEdge constructs a types.Edge for a resolved call.
func buildCSharpCallEdge(ctx *ResolveContext, callNode *sitter.Node, rc *csResolvedCall, fileHash types.Hash, repoURL string, filePath string) types.Edge {
	// Compute source hash from the enclosing function.
	srcPkgPath, srcFuncName, srcKind := splitCSEnclosingFunc(ctx.EnclosingFuncQN)
	sourceHash := types.ComputeNodeHash(repoURL, srcPkgPath, types.EmptyHash, srcFuncName, srcKind)

	// Compute target hash from the callee.
	tgtPkgPath, tgtFuncName, tgtKind := splitCSQualifiedName(rc.calleeQN)
	targetHash := types.ComputeNodeHash(repoURL, tgtPkgPath, types.EmptyHash, tgtFuncName, tgtKind)

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, typresolve.ProvenanceResolverResolved)

	return types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.Calls,
		Confidence:   typresolve.ResolverConfidence,
		Provenance:   typresolve.ProvenanceResolverResolved,
		CallSiteLine: int(callNode.StartPoint().Row) + 1, // 1-indexed
		CallSiteCol:  int(callNode.StartPoint().Column),  // 0-indexed
		CallSiteFile: filePath,
	}
}

// splitCSEnclosingFunc splits a C# enclosing function QN into namespace path,
// function name, and kind.
func splitCSEnclosingFunc(funcQN string) (pkgPath string, funcName string, kind string) {
	if funcQN == "" {
		return "", "", "function"
	}
	if idx := strings.LastIndex(funcQN, "."); idx >= 0 {
		pkgPath = funcQN[:idx]
		funcName = funcQN[idx+1:]
	} else {
		funcName = funcQN
	}
	kind = "method"
	return
}

// splitCSQualifiedName splits a C# qualified name into namespace/type path,
// symbol name, and kind.
func splitCSQualifiedName(qn string) (pkgPath string, symbolName string, kind string) {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		pkgPath = qn[:idx]
		symbolName = qn[idx+1:]
	} else {
		symbolName = qn
	}
	kind = "method"
	return
}

// currentNamespace returns the dot-joined namespace from the stack.
func currentNamespace(stack []string) string {
	if len(stack) == 0 {
		return ""
	}
	return strings.Join(stack, ".")
}

// stripGenericSuffix strips generic type arguments from a name (e.g., Method<T> -> Method).
func stripGenericSuffix(name string) string {
	if idx := strings.IndexByte(name, '<'); idx >= 0 {
		return name[:idx]
	}
	return name
}
