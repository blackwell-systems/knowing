package javaresolve

import (
	"strings"
	"unicode"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveCallsInFile walks a Java file's AST resolving call expressions
// and emitting edges. Uses the shared registry, per-file scope chain,
// and import map.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	var edges []types.Edge

	// Walk top-level children for class/interface/enum declarations.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "class_declaration", "interface_declaration", "enum_declaration":
			processClassDecl(ctx, child, &edges, fileHash, repoURL, filePath)
		}
	}

	return edges
}

// processClassDecl processes a class, interface, or enum declaration.
func processClassDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	className := nodeContent(nameNode, ctx.Content)

	// Build qualified class name.
	var classQN string
	if ctx.EnclosingClassQN != "" {
		classQN = ctx.EnclosingClassQN + "." + className
	} else if ctx.PkgQN != "" {
		classQN = ctx.PkgQN + "." + className
	} else {
		classQN = className
	}

	// Save and set enclosing class.
	prevClassQN := ctx.EnclosingClassQN
	ctx.EnclosingClassQN = classQN

	// Push class scope.
	classScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = classScope

	// Walk the class body.
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		for i := 0; i < int(bodyNode.NamedChildCount()); i++ {
			child := bodyNode.NamedChild(i)
			if child == nil {
				continue
			}
			switch child.Type() {
			case "field_declaration":
				processFieldDecl(ctx, child)
			case "method_declaration":
				processMethodDecl(ctx, child, edges, fileHash, repoURL, filePath)
			case "constructor_declaration":
				processConstructorDecl(ctx, child, edges, fileHash, repoURL, filePath)
			case "class_declaration", "interface_declaration", "enum_declaration":
				processClassDecl(ctx, child, edges, fileHash, repoURL, filePath)
			}
		}
	}

	// Pop scope and restore enclosing class.
	ctx.Scope = origScope
	ctx.EnclosingClassQN = prevClassQN
}

// processFieldDecl processes a field declaration to bind field types into scope.
func processFieldDecl(ctx *ResolveContext, node *sitter.Node) {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	fieldType := ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)

	// Walk variable_declarator children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil || child.Type() != "variable_declarator" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nodeContent(nameNode, ctx.Content)
		ctx.Scope.Bind(name, fieldType)
	}
}

// processMethodDecl processes a method declaration.
func processMethodDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeContent(nameNode, ctx.Content)

	// Build enclosing func QN from class QN.
	prevFuncQN := ctx.EnclosingFuncQN
	if ctx.EnclosingClassQN != "" {
		ctx.EnclosingFuncQN = ctx.EnclosingClassQN + "." + methodName
	} else {
		ctx.EnclosingFuncQN = methodName
	}

	// Push method scope.
	methodScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = methodScope

	// Bind parameters into scope.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindJavaParams(ctx, paramsNode)
	}

	// Resolve calls in the method body.
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		resolveCallsInNode(ctx, bodyNode, edges, fileHash, repoURL, filePath)
	}

	// Pop scope.
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = prevFuncQN
}

// processConstructorDecl processes a constructor declaration.
func processConstructorDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Constructor QN uses <init> convention.
	prevFuncQN := ctx.EnclosingFuncQN
	if ctx.EnclosingClassQN != "" {
		ctx.EnclosingFuncQN = ctx.EnclosingClassQN + ".<init>"
	} else {
		ctx.EnclosingFuncQN = "<init>"
	}

	// Push constructor scope.
	ctorScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = ctorScope

	// Bind parameters into scope.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindJavaParams(ctx, paramsNode)
	}

	// Resolve calls in the constructor body.
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		resolveCallsInNode(ctx, bodyNode, edges, fileHash, repoURL, filePath)
	}

	// Pop scope.
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = prevFuncQN
}

// bindJavaParams binds formal parameters into the current scope.
func bindJavaParams(ctx *ResolveContext, paramsNode *sitter.Node) {
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		param := paramsNode.NamedChild(i)
		if param == nil || param.Type() != "formal_parameter" {
			// Also handle spread_parameter for varargs.
			if param != nil && param.Type() == "spread_parameter" {
				bindSpreadParam(ctx, param)
			}
			continue
		}

		typeNode := param.ChildByFieldName("type")
		nameNode := param.ChildByFieldName("name")
		if typeNode == nil || nameNode == nil {
			continue
		}

		paramType := ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
		name := nodeContent(nameNode, ctx.Content)
		ctx.Scope.Bind(name, paramType)
	}
}

// bindSpreadParam binds a varargs parameter (e.g., String... args).
func bindSpreadParam(ctx *ResolveContext, node *sitter.Node) {
	typeNode := node.ChildByFieldName("type")
	nameNode := node.ChildByFieldName("name")
	if typeNode == nil || nameNode == nil {
		return
	}

	elemType := ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
	paramType := typresolve.Slice(elemType)
	name := nodeContent(nameNode, ctx.Content)
	ctx.Scope.Bind(name, paramType)
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
	case "method_invocation":
		if edge, ok := resolveMethodInvocation(ctx, node, fileHash, repoURL, filePath); ok {
			*edges = append(*edges, edge)
		}

	case "object_creation_expression":
		if edge, ok := resolveObjectCreation(ctx, node, fileHash, repoURL, filePath); ok {
			*edges = append(*edges, edge)
		}
		// Process anonymous class body if present.
		processAnonymousClassBody(ctx, node, edges, fileHash, repoURL, filePath)

	case "method_reference":
		if edge, ok := resolveMethodReference(ctx, node, fileHash, repoURL, filePath); ok {
			*edges = append(*edges, edge)
		}

	case "lambda_expression":
		processLambdaExpression(ctx, node, edges, fileHash, repoURL, filePath)
	}

	// Handle scoped blocks: push scope, recurse, pop.
	needsPop := false
	switch node.Type() {
	case "block", "if_statement", "for_statement", "enhanced_for_statement",
		"while_statement", "do_statement", "switch_expression",
		"try_statement", "try_with_resources_statement":
		childScope := typresolve.NewScope(ctx.Scope)
		ctx.Scope = childScope
		needsPop = true
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

// resolveMethodInvocation resolves a method_invocation node and returns
// a call edge if successful.
func resolveMethodInvocation(ctx *ResolveContext, node *sitter.Node, fileHash types.Hash, repoURL string, filePath string) (types.Edge, bool) {
	nameNode := node.ChildByFieldName("name")
	objectNode := node.ChildByFieldName("object")

	if nameNode == nil {
		return types.Edge{}, false
	}

	methodName := nodeContent(nameNode, ctx.Content)

	// Skip builtin methods.
	if IsBuiltinFunc(methodName) {
		return types.Edge{}, false
	}

	var calleeQN string

	if objectNode == nil {
		// No receiver: look up on enclosing class first, then static imports, then package-local.
		if ctx.EnclosingClassQN != "" {
			if f := LookupFieldOrMethod(ctx.Registry, ctx.EnclosingClassQN, methodName); f != nil {
				calleeQN = f.QualifiedName
			}
		}
		// Static import resolution: check single static imports and wildcard static imports.
		if calleeQN == "" && ctx.ImportInfo != nil {
			if classQN, ok := ctx.ImportInfo.StaticMethods[methodName]; ok {
				if f := LookupFieldOrMethod(ctx.Registry, classQN, methodName); f != nil {
					calleeQN = f.QualifiedName
				} else {
					calleeQN = classQN + "." + methodName
				}
			}
			if calleeQN == "" {
				for _, classQN := range ctx.ImportInfo.StaticWildcardClasses {
					if f := LookupFieldOrMethod(ctx.Registry, classQN, methodName); f != nil {
						calleeQN = f.QualifiedName
						break
					}
				}
			}
		}
		if calleeQN == "" {
			// Package-local function.
			if f := ctx.Registry.LookupFunc(ctx.PkgQN + "." + methodName); f != nil {
				calleeQN = f.QualifiedName
			} else {
				// Construct a best-effort QN.
				if ctx.EnclosingClassQN != "" {
					calleeQN = ctx.EnclosingClassQN + "." + methodName
				} else {
					calleeQN = ctx.PkgQN + "." + methodName
				}
			}
		}
	} else {
		// Object is present.
		calleeQN = resolveObjectMethodCall(ctx, objectNode, methodName)
	}

	if calleeQN == "" {
		return types.Edge{}, false
	}

	edge := buildJavaCallEdge(ctx, node, calleeQN, fileHash, repoURL, filePath)
	return edge, true
}

// resolveObjectMethodCall resolves a method call on an object receiver.
func resolveObjectMethodCall(ctx *ResolveContext, objectNode *sitter.Node, methodName string) string {
	// If object is an identifier, check for import (static call pattern).
	if objectNode.Type() == "identifier" {
		objectName := nodeContent(objectNode, ctx.Content)

		// Check if it matches an import (uppercase first letter = class name).
		if len(objectName) > 0 && unicode.IsUpper(rune(objectName[0])) {
			if pkg, ok := ctx.Imports[objectName]; ok {
				classQN := pkg + "." + objectName
				if f := LookupFieldOrMethod(ctx.Registry, classQN, methodName); f != nil {
					return f.QualifiedName
				}
				return classQN + "." + methodName
			}
			// Try package-local class.
			classQN := ctx.PkgQN + "." + objectName
			if f := LookupFieldOrMethod(ctx.Registry, classQN, methodName); f != nil {
				return f.QualifiedName
			}
			return classQN + "." + methodName
		}
	}

	// Evaluate object type via EvalExprType.
	objType := EvalExprType(ctx, objectNode)
	if objType != nil && objType.Kind == typresolve.KindNamed {
		if f := LookupFieldOrMethod(ctx.Registry, objType.Name, methodName); f != nil {
			return f.QualifiedName
		}
		return objType.Name + "." + methodName
	}

	return ""
}

// resolveObjectCreation resolves an object_creation_expression (new ClassName())
// and returns a constructor call edge.
func resolveObjectCreation(ctx *ResolveContext, node *sitter.Node, fileHash types.Hash, repoURL string, filePath string) (types.Edge, bool) {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return types.Edge{}, false
	}

	// Extract the type name and qualify it.
	typeName := extractBaseTypeName(typeNode, ctx.Content)
	if typeName == "" {
		return types.Edge{}, false
	}

	// Skip builtin types.
	if IsBuiltinType(typeName) {
		return types.Edge{}, false
	}

	qualifiedType := qualifyTypeName(typeName, ctx.PkgQN, ctx.Imports)
	calleeQN := qualifiedType + ".<init>"

	edge := buildJavaCallEdge(ctx, node, calleeQN, fileHash, repoURL, filePath)
	return edge, true
}

// buildJavaCallEdge creates a types.Edge from a resolved Java call.
func buildJavaCallEdge(ctx *ResolveContext, callNode *sitter.Node, calleeQN string, fileHash types.Hash, repoURL string, filePath string) types.Edge {
	// Compute source hash from the enclosing function.
	srcPkgPath, srcFuncName, srcKind := splitEnclosingFunc(ctx.EnclosingFuncQN, ctx.PkgQN)
	sourceHash := types.ComputeNodeHash(repoURL, srcPkgPath, types.EmptyHash, srcFuncName, srcKind)

	// Compute target hash from the callee.
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

// splitEnclosingFunc splits the enclosing function QN into package path,
// function name, and kind. Java method QNs have format
// "com.example.ClassName.methodName". With pkgQN "com.example", this
// extracts "ClassName.methodName" as the function name.
func splitEnclosingFunc(funcQN string, pkgQN string) (pkgPath string, funcName string, kind string) {
	if pkgQN != "" && strings.HasPrefix(funcQN, pkgQN+".") {
		rest := funcQN[len(pkgQN)+1:]
		// If rest contains a dot, it is ClassName.methodName (a method).
		if strings.Contains(rest, ".") {
			return pkgQN, rest, types.KindMethod
		}
		return pkgQN, rest, types.KindFunction
	}
	return pkgQN, funcQN, types.KindFunction
}

// processAnonymousClassBody detects an anonymous class body in an
// object_creation_expression and recursively processes its methods.
// Anonymous classes in Java look like: new Interface() { void method() { ... } }
func processAnonymousClassBody(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Look for a class_body child (tree-sitter uses "class_body" for anonymous class bodies).
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		if child.Type() == "class_body" {
			// Process methods inside the anonymous class body.
			for j := 0; j < int(child.NamedChildCount()); j++ {
				member := child.NamedChild(j)
				if member == nil {
					continue
				}
				switch member.Type() {
				case "method_declaration":
					processMethodDecl(ctx, member, edges, fileHash, repoURL, filePath)
				case "constructor_declaration":
					processConstructorDecl(ctx, member, edges, fileHash, repoURL, filePath)
				case "field_declaration":
					processFieldDecl(ctx, member)
				}
			}
		}
	}
}

// resolveMethodReference resolves a method_reference node (e.g., String::valueOf,
// this::process) and returns a call edge.
func resolveMethodReference(ctx *ResolveContext, node *sitter.Node, fileHash types.Hash, repoURL string, filePath string) (types.Edge, bool) {
	// tree-sitter-java method_reference has children: object "::" name
	// The structure is: type_identifier/identifier "::" identifier
	var objectText string
	var methodName string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Type() {
		case "identifier", "type_identifier":
			if methodName == "" && objectText != "" {
				// This is the method name (after ::).
				methodName = nodeContent(child, ctx.Content)
			} else if objectText == "" {
				objectText = nodeContent(child, ctx.Content)
			}
		case "::":
			// Next identifier will be the method name.
			continue
		}
	}

	// tree-sitter may structure it differently; try field-based access.
	if objectText == "" || methodName == "" {
		// Try ChildByFieldName which some grammars use.
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			methodName = nodeContent(nameNode, ctx.Content)
		}
		// The first named child that's not the name is the object.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child != nameNode {
				objectText = nodeContent(child, ctx.Content)
				break
			}
		}
	}

	if objectText == "" || methodName == "" {
		return types.Edge{}, false
	}

	// Skip builtin methods.
	if IsBuiltinFunc(methodName) {
		return types.Edge{}, false
	}

	var calleeQN string

	// Resolve the class/object part.
	if objectText == "this" {
		if ctx.EnclosingClassQN != "" {
			if f := LookupFieldOrMethod(ctx.Registry, ctx.EnclosingClassQN, methodName); f != nil {
				calleeQN = f.QualifiedName
			} else {
				calleeQN = ctx.EnclosingClassQN + "." + methodName
			}
		}
	} else if objectText == "super" {
		rt := ctx.Registry.LookupType(ctx.EnclosingClassQN)
		if rt != nil && len(rt.EmbeddedTypes) > 0 {
			parent := rt.EmbeddedTypes[0]
			if f := LookupFieldOrMethod(ctx.Registry, parent, methodName); f != nil {
				calleeQN = f.QualifiedName
			} else {
				calleeQN = parent + "." + methodName
			}
		}
	} else if len(objectText) > 0 && unicode.IsUpper(rune(objectText[0])) {
		// Class reference: ClassName::method
		classQN := qualifyTypeName(objectText, ctx.PkgQN, ctx.Imports)
		if f := LookupFieldOrMethod(ctx.Registry, classQN, methodName); f != nil {
			calleeQN = f.QualifiedName
		} else {
			calleeQN = classQN + "." + methodName
		}
	} else {
		// Instance variable reference: try scope lookup.
		if t := ctx.Scope.Lookup(objectText); t != nil && t.Kind == typresolve.KindNamed {
			if f := LookupFieldOrMethod(ctx.Registry, t.Name, methodName); f != nil {
				calleeQN = f.QualifiedName
			} else {
				calleeQN = t.Name + "." + methodName
			}
		}
	}

	if calleeQN == "" {
		return types.Edge{}, false
	}

	edge := buildJavaCallEdge(ctx, node, calleeQN, fileHash, repoURL, filePath)
	return edge, true
}

// processLambdaExpression processes a lambda_expression node, binding
// parameters if their types can be inferred from context (functional interface).
func processLambdaExpression(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string) {
	// Push a lambda scope.
	lambdaScope := typresolve.NewScope(ctx.Scope)
	origScope := ctx.Scope
	ctx.Scope = lambdaScope

	// Try to bind lambda parameters. Java lambdas can have:
	// (Type param) -> body   (typed parameters)
	// (param) -> body        (inferred parameters)
	// param -> body          (single inferred parameter)
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindLambdaParams(ctx, paramsNode, node)
	} else {
		// Single parameter without parens: the first identifier child is the param.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil && child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				// Infer type from context (best-effort: bind as Unknown).
				paramType := inferLambdaParamType(ctx, node, 0)
				ctx.Scope.Bind(name, paramType)
				break
			}
		}
	}

	// Resolve calls in the lambda body.
	bodyNode := node.ChildByFieldName("body")
	if bodyNode != nil {
		resolveCallsInNode(ctx, bodyNode, edges, fileHash, repoURL, filePath)
	}

	ctx.Scope = origScope
}

// bindLambdaParams binds lambda parameters, attempting to infer types from
// the target functional interface context.
func bindLambdaParams(ctx *ResolveContext, paramsNode *sitter.Node, lambdaNode *sitter.Node) {
	// Check if parameters are typed (formal_parameter) or inferred (inferred_parameters).
	switch paramsNode.Type() {
	case "formal_parameters":
		// Typed lambda parameters: (String s, int x) -> ...
		bindJavaParams(ctx, paramsNode)
	case "inferred_parameters":
		// Inferred: (a, b) -> ...; try to infer from context.
		for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
			child := paramsNode.NamedChild(i)
			if child == nil || child.Type() != "identifier" {
				continue
			}
			name := nodeContent(child, ctx.Content)
			paramType := inferLambdaParamType(ctx, lambdaNode, i)
			ctx.Scope.Bind(name, paramType)
		}
	default:
		// Might be a single identifier or other structure.
		for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
			child := paramsNode.NamedChild(i)
			if child == nil {
				continue
			}
			if child.Type() == "formal_parameter" {
				typeNode := child.ChildByFieldName("type")
				nameNode := child.ChildByFieldName("name")
				if typeNode != nil && nameNode != nil {
					paramType := ParseTypeNode(typeNode, ctx.Content, ctx.PkgQN, ctx.Imports)
					name := nodeContent(nameNode, ctx.Content)
					ctx.Scope.Bind(name, paramType)
				}
			} else if child.Type() == "identifier" {
				name := nodeContent(child, ctx.Content)
				paramType := inferLambdaParamType(ctx, lambdaNode, i)
				ctx.Scope.Bind(name, paramType)
			}
		}
	}
}

// inferLambdaParamType infers the type of a lambda parameter at the given
// index by looking at the parent context (e.g., method argument position)
// to determine the target functional interface.
func inferLambdaParamType(ctx *ResolveContext, lambdaNode *sitter.Node, paramIdx int) *typresolve.Type {
	// Walk up to find the argument list context.
	parent := lambdaNode.Parent()
	if parent == nil {
		return typresolve.Unknown()
	}

	// If the lambda is an argument to a method, we could look up the method's
	// parameter type to find the functional interface. For now, use common
	// patterns.
	if parent.Type() == "argument_list" {
		grandparent := parent.Parent()
		if grandparent != nil && grandparent.Type() == "method_invocation" {
			nameNode := grandparent.ChildByFieldName("name")
			objectNode := grandparent.ChildByFieldName("object")
			if nameNode != nil {
				methodName := nodeContent(nameNode, ctx.Content)
				// Common stream operations that take Function<T,R> or Predicate<T>.
				switch methodName {
				case "map", "flatMap", "filter", "forEach", "peek",
					"reduce", "collect", "anyMatch", "allMatch", "noneMatch":
					// If the object is known to be a Stream or Collection,
					// we could infer element type. For now return Unknown.
					_ = objectNode
				}
			}
		}
	}

	return typresolve.Unknown()
}

// splitQualifiedName splits a callee qualified name into package path,
// symbol name, and kind. Handles Java QN formats like
// "com.example.service.UserService.processData".
func splitQualifiedName(qn string) (pkgPath string, symbolName string, kind string) {
	// Find the last two dots to separate package path from class.method.
	lastDot := strings.LastIndex(qn, ".")
	if lastDot == -1 {
		return "", qn, types.KindFunction
	}

	secondLastDot := strings.LastIndex(qn[:lastDot], ".")
	if secondLastDot == -1 {
		// Only one dot: "ClassName.method" or "pkg.ClassName"
		return qn[:lastDot], qn[lastDot+1:], types.KindFunction
	}

	// Check if the segment after secondLastDot starts with uppercase
	// (indicating a class name, making this a method reference).
	segAfterSecondDot := qn[secondLastDot+1 : lastDot]
	if len(segAfterSecondDot) > 0 && unicode.IsUpper(rune(segAfterSecondDot[0])) {
		// This is "pkg.ClassName.methodName"
		return qn[:secondLastDot], qn[secondLastDot+1:], types.KindMethod
	}

	// Otherwise "pkgPart.ClassName" or similar.
	return qn[:lastDot], qn[lastDot+1:], types.KindFunction
}
