package rubyresolve

import (
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveCallsInFile walks a Ruby file's AST resolving call expressions
// and emitting edges. Uses the shared registry, per-file scope chain,
// require map, and constant resolution.
func ResolveCallsInFile(ctx *ResolveContext, root *sitter.Node, fileHash types.Hash, repoURL string, filePath string) []types.Edge {
	var edges []types.Edge

	qnPrefix := strings.TrimSuffix(filepath.ToSlash(filePath), filepath.Ext(filePath))

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "class":
			resolveClass(ctx, child, &edges, fileHash, repoURL, filePath, qnPrefix)

		case "module":
			resolveModule(ctx, child, &edges, fileHash, repoURL, filePath, qnPrefix)

		case "method":
			resolveMethodDecl(ctx, child, &edges, fileHash, repoURL, filePath, qnPrefix)

		case "call":
			resolveCallNode(ctx, child, &edges, fileHash, repoURL, filePath, qnPrefix)

		default:
			// Walk into other top-level nodes for nested calls.
			resolveCallsInNode(ctx, child, &edges, fileHash, repoURL, filePath, qnPrefix)
		}
	}

	return edges
}

// resolveClass processes a class node: push nesting, walk body, pop nesting.
func resolveClass(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}

	className := nodeText(nameNode, ctx.Content)
	if nameNode.Type() == "scope_resolution" {
		className = ParseScopeResolution(nameNode, ctx.Content)
	}

	ctx.Nesting = append(ctx.Nesting, className)
	defer func() { ctx.Nesting = ctx.Nesting[:len(ctx.Nesting)-1] }()

	// Walk class body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveClassBody(ctx, body, edges, fileHash, repoURL, filePath, qnPrefix)
	}
}

// resolveModule processes a module node: push nesting, walk body, pop nesting.
func resolveModule(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}

	moduleName := nodeText(nameNode, ctx.Content)
	if nameNode.Type() == "scope_resolution" {
		moduleName = ParseScopeResolution(nameNode, ctx.Content)
	}

	ctx.Nesting = append(ctx.Nesting, moduleName)
	defer func() { ctx.Nesting = ctx.Nesting[:len(ctx.Nesting)-1] }()

	body := node.ChildByFieldName("body")
	if body != nil {
		resolveClassBody(ctx, body, edges, fileHash, repoURL, filePath, qnPrefix)
	}
}

// resolveClassBody walks the body of a class or module, handling methods,
// nested classes/modules, and call expressions.
func resolveClassBody(ctx *ResolveContext, body *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "method":
			resolveMethodDecl(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)

		case "singleton_method":
			resolveMethodDecl(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)

		case "class":
			resolveClass(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)

		case "module":
			resolveModule(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)

		case "call":
			resolveCallNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)

		default:
			resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
		}
	}
}

// resolveMethodDecl processes a method (def) node.
func resolveMethodDecl(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nodeText(nameNode, ctx.Content)

	// Build enclosing function QN.
	if len(ctx.Nesting) > 0 {
		ctx.EnclosingFuncQN = qnPrefix + "." + strings.Join(ctx.Nesting, ".") + "." + methodName
	} else {
		ctx.EnclosingFuncQN = qnPrefix + "." + methodName
	}

	// Push method scope.
	origScope := ctx.Scope
	ctx.Scope = typresolve.NewScope(origScope)

	// Bind method parameters.
	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode != nil {
		bindRubyParams(ctx, paramsNode)
	}

	// Walk method body.
	body := node.ChildByFieldName("body")
	if body != nil {
		resolveCallsInNode(ctx, body, edges, fileHash, repoURL, filePath, qnPrefix)
	}

	// Pop scope.
	ctx.Scope = origScope
	ctx.EnclosingFuncQN = ""
}

// resolveCallsInNode recursively walks an AST node resolving call
// expressions and emitting edges.
func resolveCallsInNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	if node == nil {
		return
	}

	// Process statement for scope bindings (assignments, etc.).
	ProcessStatement(ctx, node)

	switch node.Type() {
	case "call":
		resolveCallNode(ctx, node, edges, fileHash, repoURL, filePath, qnPrefix)
		return

	case "method":
		// Nested method definition.
		resolveMethodDecl(ctx, node, edges, fileHash, repoURL, filePath, qnPrefix)
		return

	case "class":
		resolveClass(ctx, node, edges, fileHash, repoURL, filePath, qnPrefix)
		return

	case "module":
		resolveModule(ctx, node, edges, fileHash, repoURL, filePath, qnPrefix)
		return

	case "do_block", "block":
		origScope := ctx.Scope
		ctx.Scope = typresolve.NewScope(origScope)

		// Bind block parameters if present.
		blockParams := node.ChildByFieldName("parameters")
		if blockParams != nil {
			bindRubyParams(ctx, blockParams)
		}

		// Recurse into block body.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
			}
		}

		ctx.Scope = origScope
		return

	case "if", "unless", "while", "until", "begin":
		origScope := ctx.Scope
		ctx.Scope = typresolve.NewScope(origScope)

		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
			}
		}

		ctx.Scope = origScope
		return

	case "case":
		resolveCaseNode(ctx, node, edges, fileHash, repoURL, filePath, qnPrefix)
		return
	}

	// Recurse into children for all other node types.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil {
			resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
		}
	}
}

// resolveCaseNode handles case/when with per-when scopes.
func resolveCaseNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}

		if child.Type() == "when" {
			origScope := ctx.Scope
			ctx.Scope = typresolve.NewScope(origScope)

			for j := 0; j < int(child.NamedChildCount()); j++ {
				whenChild := child.NamedChild(j)
				if whenChild != nil {
					resolveCallsInNode(ctx, whenChild, edges, fileHash, repoURL, filePath, qnPrefix)
				}
			}

			ctx.Scope = origScope
		} else {
			resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
		}
	}
}

// resolveCallNode handles a call node and emits edges.
func resolveCallNode(ctx *ResolveContext, node *sitter.Node, edges *[]types.Edge, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) {
	methodNode := node.ChildByFieldName("method")
	receiver := node.ChildByFieldName("receiver")

	if methodNode == nil {
		// Recurse into arguments/blocks in case they contain calls.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
			}
		}
		return
	}

	methodName := nodeText(methodNode, ctx.Content)

	var calleeQN string
	var resolved bool

	if receiver != nil {
		calleeQN, resolved = resolveReceiverCall(ctx, receiver, methodName, qnPrefix)
	} else {
		calleeQN, resolved = resolveBareCall(ctx, methodName, qnPrefix)
	}

	_ = resolved

	// Only emit edge if we have a callee QN and it's not a builtin.
	if calleeQN != "" {
		edge := buildRubyCallEdge(ctx, node, calleeQN, fileHash, repoURL, filePath, qnPrefix)
		*edges = append(*edges, edge)
	}

	// Recurse into arguments and blocks for nested calls.
	argsNode := node.ChildByFieldName("arguments")
	if argsNode != nil {
		resolveCallsInNode(ctx, argsNode, edges, fileHash, repoURL, filePath, qnPrefix)
	}
	blockNode := node.ChildByFieldName("block")
	if blockNode != nil {
		resolveCallsInNode(ctx, blockNode, edges, fileHash, repoURL, filePath, qnPrefix)
	}

	// Check for do_block children.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child != nil && (child.Type() == "do_block" || child.Type() == "block") {
			resolveCallsInNode(ctx, child, edges, fileHash, repoURL, filePath, qnPrefix)
		}
	}
}

// resolveReceiverCall resolves a call with a receiver (obj.method, Class.new).
func resolveReceiverCall(ctx *ResolveContext, receiver *sitter.Node, methodName string, qnPrefix string) (string, bool) {
	// Class.new -> resolve to Class#initialize.
	if receiver.Type() == "constant" && methodName == "new" {
		constName := nodeText(receiver, ctx.Content)
		resolved := ResolveConstant(constName, ctx.Nesting)

		// Look up initialize method.
		if f := ResolveNew(ctx.Registry, resolved); f != nil {
			return f.QualifiedName, true
		}

		// Fall back: emit edge to ClassName.initialize.
		return resolved + ".initialize", false
	}

	// Evaluate receiver type.
	recvType := EvalExprType(ctx, receiver)

	if recvType.Kind == typresolve.KindNamed {
		typeQN := recvType.Name

		// Look up method on the type.
		if f := LookupAttribute(ctx.Registry, typeQN, methodName); f != nil {
			return f.QualifiedName, true
		}

		// Check attr_accessor generated methods.
		if f := LookupAttrAccessor(ctx.Registry, typeQN, methodName); f != nil {
			return f.QualifiedName, true
		}

		// Fall back: construct heuristic QN.
		return typeQN + "." + methodName, false
	}

	// Unresolved receiver: use receiver text + method name.
	recvText := nodeText(receiver, ctx.Content)
	return recvText + "." + methodName, false
}

// resolveBareCall resolves a call without a receiver (bare method call).
func resolveBareCall(ctx *ResolveContext, methodName string, qnPrefix string) (string, bool) {
	// Skip builtins.
	if IsBuiltinFunc(methodName) {
		return "", false
	}

	// Look up in current class context (implicit self).
	if len(ctx.Nesting) > 0 {
		currentClassQN := strings.Join(ctx.Nesting, "::")

		if f := LookupAttribute(ctx.Registry, currentClassQN, methodName); f != nil {
			return f.QualifiedName, true
		}

		// Also try with qnPrefix.
		fullClassQN := qnPrefix + "." + strings.Join(ctx.Nesting, ".")
		if f := LookupAttribute(ctx.Registry, fullClassQN, methodName); f != nil {
			return f.QualifiedName, true
		}
	}

	// Look up as top-level function.
	if f := ctx.Registry.LookupFunc(methodName); f != nil {
		return f.QualifiedName, true
	}

	// Try with qnPrefix.
	fullQN := qnPrefix + "." + methodName
	if f := ctx.Registry.LookupFunc(fullQN); f != nil {
		return f.QualifiedName, true
	}

	// Fall back to method name with prefix.
	return fullQN, false
}

// buildRubyCallEdge creates a types.Edge from a resolved Ruby call.
func buildRubyCallEdge(ctx *ResolveContext, callNode *sitter.Node, calleeQN string, fileHash types.Hash, repoURL string, filePath string, qnPrefix string) types.Edge {
	// Compute source hash from the enclosing function/method.
	srcQN := ctx.EnclosingFuncQN
	if srcQN == "" {
		// Top-level call: use the file as source.
		srcQN = qnPrefix
	}

	srcPkgPath, srcSymbol, srcKind := splitRubyQN(srcQN, qnPrefix)
	sourceHash := types.ComputeNodeHash(repoURL, srcPkgPath, types.EmptyHash, srcSymbol, srcKind)

	// Compute target hash from the callee QN.
	tgtPkgPath, tgtSymbol, tgtKind := splitRubyQN(calleeQN, "")
	targetHash := types.ComputeNodeHash(repoURL, tgtPkgPath, types.EmptyHash, tgtSymbol, tgtKind)

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

// splitRubyQN splits a Ruby qualified name into package path, symbol name,
// and kind. Handles Ruby QN formats like "filePath.ClassName.methodName"
// and "filePath.functionName".
func splitRubyQN(qn string, defaultPkgPath string) (pkgPath string, symbolName string, kind string) {
	// Count dots to determine structure.
	lastDot := strings.LastIndex(qn, ".")
	if lastDot == -1 {
		// No dots: just a name.
		return defaultPkgPath, qn, types.KindFunction
	}

	secondLastDot := strings.LastIndex(qn[:lastDot], ".")
	if secondLastDot >= 0 {
		// Two or more dots: could be pkg.Class.method.
		pkgPath = qn[:secondLastDot]
		symbolName = qn[secondLastDot+1:]
		return pkgPath, symbolName, types.KindMethod
	}

	// One dot: pkg.func or Class.method.
	return qn[:lastDot], qn[lastDot+1:], types.KindFunction
}

// bindRubyParams binds method/block parameters into the current scope as Unknown types.
func bindRubyParams(ctx *ResolveContext, paramsNode *sitter.Node) {
	for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
		child := paramsNode.NamedChild(i)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "identifier":
			name := nodeText(child, ctx.Content)
			ctx.Scope.Bind(name, typresolve.Unknown())

		case "optional_parameter", "keyword_parameter":
			// First child is the parameter name.
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, ctx.Content)
				ctx.Scope.Bind(name, typresolve.Unknown())
			} else if child.NamedChildCount() > 0 {
				first := child.NamedChild(0)
				if first.Type() == "identifier" {
					name := nodeText(first, ctx.Content)
					ctx.Scope.Bind(name, typresolve.Unknown())
				}
			}

		case "splat_parameter", "hash_splat_parameter", "block_parameter":
			// These have a name child or first named child is identifier.
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name := nodeText(nameNode, ctx.Content)
				ctx.Scope.Bind(name, typresolve.Unknown())
			} else if child.NamedChildCount() > 0 {
				first := child.NamedChild(0)
				if first.Type() == "identifier" {
					name := nodeText(first, ctx.Content)
					ctx.Scope.Bind(name, typresolve.Unknown())
				}
			}
		}
	}
}
