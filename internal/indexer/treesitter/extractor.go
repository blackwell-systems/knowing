// Package treesitter provides a tree-sitter based extractor for non-Go languages.
// Currently supports Python.
//
// The extractor walks the tree-sitter AST to find function_definition,
// class_definition, import_statement, and call nodes (using Python grammar
// node type names). Qualified names follow the convention:
//
//	{repoURL}://{moduleRoot}/{filePath}.{ClassName}.{SymbolName}
//
// All edges have provenance "ast_resolved" and confidence 1.0, though
// cross-module call targets may be unresolved (dangling) if the target
// module is not indexed.
package treesitter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/resolve"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TreeSitterExtractor implements types.Extractor using tree-sitter for AST parsing.
// Thread-safe: each Extract call creates its own parser.
type TreeSitterExtractor struct {
	language string
}

// NewTreeSitterExtractor creates a new tree-sitter extractor for the given language.
// Currently only "python" is supported.
func NewTreeSitterExtractor(language string) (*TreeSitterExtractor, error) {
	lang := strings.ToLower(language)
	if lang != "python" {
		return nil, fmt.Errorf("unsupported language: %s (only python is supported)", language)
	}

	return &TreeSitterExtractor{
		language: lang,
	}, nil
}

// Name returns the extractor name.
func (e *TreeSitterExtractor) Name() string {
	return "treesitter-python"
}

// CanHandle returns true if the file path is a Python file.
func (e *TreeSitterExtractor) CanHandle(path string) bool {
	return filepath.Ext(path) == ".py"
}

// Extract parses the given Python source and returns nodes and edges.
func (e *TreeSitterExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	result := &types.ExtractResult{}

	// Build Python import map: maps imported names to their source module.
	// e.g., "from flask import Flask" -> pyImports["Flask"] = "flask"
	// e.g., "from flask.app import Flask" -> pyImports["Flask"] = "flask.app"
	// e.g., "import os" -> pyImports["os"] = "os"
	pyImports := buildPythonImportMap(root, opts.Content)

	// Walk the AST and extract symbols. The empty funcHash means top-level
	// calls use a synthetic module-level source; calls inside functions use
	// the function's node hash.
	e.walkNodeWithImports(root, opts, "", types.EmptyHash, pyImports, result)

	// Detect Python web framework routes (Flask, FastAPI, Django).
	e.extractPythonRoutes(root, opts, result)

	// Same-file method resolution: for each call edge whose target doesn't match
	// a known node in this file, check if a method with the same terminal name
	// exists among the extracted nodes. This handles patterns like:
	//   for op in self.operations: op.database_forwards(...)
	// where we can't determine the type of 'op' but can see that a method named
	// 'database_forwards' exists in the same module.
	resolveIntraFileCallsByName(opts, result)

	// Sort nodes deterministically by qualified name then kind.
	sort.Slice(result.Nodes, func(i, j int) bool {
		if result.Nodes[i].QualifiedName != result.Nodes[j].QualifiedName {
			return result.Nodes[i].QualifiedName < result.Nodes[j].QualifiedName
		}
		return result.Nodes[i].Kind < result.Nodes[j].Kind
	})

	// Sort edges deterministically by source, target, type.
	sort.Slice(result.Edges, func(i, j int) bool {
		si, sj := result.Edges[i], result.Edges[j]
		if si.SourceHash != sj.SourceHash {
			return si.SourceHash.String() < sj.SourceHash.String()
		}
		if si.TargetHash != sj.TargetHash {
			return si.TargetHash.String() < sj.TargetHash.String()
		}
		return si.EdgeType < sj.EdgeType
	})

	return result, nil
}

// walkNode recursively walks the tree-sitter AST and extracts symbols.
// classContext tracks whether we are inside a class body (for method scoping).
// funcHash is the node hash of the enclosing function/method; calls inside a
// function use this as their edge source so the enricher can find and upgrade them.
func (e *TreeSitterExtractor) walkNode(node *sitter.Node, opts types.ExtractOptions, classContext string, funcHash types.Hash, result *types.ExtractResult) {
	e.walkNodeWithImports(node, opts, classContext, funcHash, nil, result)
}

func (e *TreeSitterExtractor) walkNodeWithImports(node *sitter.Node, opts types.ExtractOptions, classContext string, funcHash types.Hash, pyImports map[string]string, result *types.ExtractResult) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "function_definition":
		e.extractFunction(node, opts, classContext, result)
		// Extract type_hint_of edges from parameter type annotations.
		e.extractTypeHints(node, opts, classContext, pyImports, result)
		// Extract reads_env and executes_process edges.
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			fnName := e.qualifiedName(opts, classContext, nameNode.Content(opts.Content))
			fnHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, fnName, types.KindFunction)
			envNodes, envEdges := extractEnvReadEdges(node, opts, classContext, fnHash)
			result.Nodes = append(result.Nodes, envNodes...)
			result.Edges = append(result.Edges, envEdges...)
			procNodes, procEdges := extractProcessExecEdges(node, opts, classContext, fnHash)
			result.Nodes = append(result.Nodes, procNodes...)
			result.Edges = append(result.Edges, procEdges...)
		}
		// Also walk the body with imports for nested calls inside nested functions.
		body := node.ChildByFieldName("body")
		if body != nil {
			nameNode := node.ChildByFieldName("name")
			nestedFuncHash := funcHash
			if nameNode != nil {
				nestedName := e.qualifiedName(opts, classContext, nameNode.Content(opts.Content))
				nestedFuncHash = types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, nestedName, types.KindFunction)
			}
			for i := 0; i < int(body.ChildCount()); i++ {
				e.walkNodeWithImports(body.Child(i), opts, classContext, nestedFuncHash, pyImports, result)
			}
		}
	case "class_definition":
		e.extractClassWithImports(node, opts, pyImports, result)
	case "import_statement", "import_from_statement":
		e.extractImport(node, opts, classContext, result)
	case "call":
		e.extractCallWithImports(node, opts, classContext, funcHash, pyImports, result)
		// Continue walking call arguments to extract nested calls, callbacks,
		// and lambda references. e.g., map(process, items) or app.route()(handler)
		args := node.ChildByFieldName("arguments")
		if args != nil {
			for i := 0; i < int(args.ChildCount()); i++ {
				e.walkNodeWithImports(args.Child(i), opts, classContext, funcHash, pyImports, result)
			}
		}
	case "lambda":
		// Walk lambda body for calls.
		body := node.ChildByFieldName("body")
		if body != nil {
			e.walkNodeWithImports(body, opts, classContext, funcHash, pyImports, result)
		}
	case "raise_statement":
		e.extractRaise(node, opts, classContext, result)
	default:
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			e.walkNodeWithImports(child, opts, classContext, funcHash, pyImports, result)
		}
	}
}

// qualifiedName builds a qualified name following the convention:
// {repoURL}://{modulePath}/{filePath}.{ClassName}.{SymbolName}
func (e *TreeSitterExtractor) qualifiedName(opts types.ExtractOptions, classContext, symbolName string) string {
	base := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
	if classContext != "" {
		return fmt.Sprintf("%s.%s.%s", base, classContext, symbolName)
	}
	return fmt.Sprintf("%s.%s", base, symbolName)
}

func (e *TreeSitterExtractor) makeNode(opts types.ExtractOptions, qualifiedName, kind string, line int) types.Node {
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, qualifiedName, kind)
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qualifiedName,
		Kind:          kind,
		Line:          line,
	}
}

// extractPythonDocstring extracts the docstring from a Python function or class body.
// In Python, a docstring is the first statement of a function/class if it's a string literal.
// Returns the docstring content (without quotes), or empty string if none.
func extractPythonDocstring(body *sitter.Node, content []byte) string {
	if body == nil || body.ChildCount() == 0 {
		return ""
	}
	first := body.Child(0)
	if first == nil {
		return ""
	}
	// The first statement should be an expression_statement containing a string.
	if first.Type() != "expression_statement" {
		return ""
	}
	if first.ChildCount() == 0 {
		return ""
	}
	strNode := first.Child(0)
	if strNode == nil {
		return ""
	}
	if strNode.Type() != "string" && strNode.Type() != "concatenated_string" {
		return ""
	}
	raw := strNode.Content(content)
	// Strip triple quotes or single quotes.
	doc := strings.TrimPrefix(raw, `"""`)
	doc = strings.TrimSuffix(doc, `"""`)
	doc = strings.TrimPrefix(doc, `'''`)
	doc = strings.TrimSuffix(doc, `'''`)
	doc = strings.TrimPrefix(doc, `"`)
	doc = strings.TrimSuffix(doc, `"`)
	doc = strings.TrimPrefix(doc, `'`)
	doc = strings.TrimSuffix(doc, `'`)
	doc = strings.TrimSpace(doc)
	// Cap at 500 chars to avoid bloating FTS with huge docstrings.
	if len(doc) > 500 {
		doc = doc[:500]
	}
	return doc
}

// extractFunction extracts a function definition node.
func (e *TreeSitterExtractor) extractFunction(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1 // 1-indexed

	kind := types.KindFunction
	if classContext != "" {
		kind = types.KindMethod
	}

	qname := e.qualifiedName(opts, classContext, name)
	n := e.makeNode(opts, qname, kind, line)

	// Extract Python docstring from function body.
	body := node.ChildByFieldName("body")
	n.Doc = extractPythonDocstring(body, opts.Content)

	result.Nodes = append(result.Nodes, n)

	// Extract field access edges for methods (classContext != "").
	if classContext != "" {
		fieldEdges := extractFieldAccessEdges(node, opts, classContext, n.NodeHash)
		result.Edges = append(result.Edges, fieldEdges...)
	}

	// Emit decorates edges for decorators on this function/method.
	e.extractPythonDecoratorEdges(node, opts, n.NodeHash, result)

	// Walk function body for calls and imports, passing this function's
	// node hash so calls use it as edge source.
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			e.walkNode(body.Child(i), opts, classContext, n.NodeHash, result)
		}
	}
}

// extractTypeHints creates type_hint_of edges from a function to the types
// referenced in its parameter annotations and return type annotation.
// Example: `def process(cache: BaseCache) -> bool:` creates edges to BaseCache.
func (e *TreeSitterExtractor) extractTypeHints(funcNode *sitter.Node, opts types.ExtractOptions, classContext string, pyImports map[string]string, result *types.ExtractResult) {
	nameNode := funcNode.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	funcName := nameNode.Content(opts.Content)
	kind := types.KindFunction
	if classContext != "" {
		kind = types.KindMethod
	}
	funcQN := e.qualifiedName(opts, classContext, funcName)
	funcHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, funcQN, kind)

	// Walk parameters for type annotations.
	params := funcNode.ChildByFieldName("parameters")
	if params == nil {
		return
	}
	seen := make(map[string]bool)
	for i := 0; i < int(params.ChildCount()); i++ {
		param := params.Child(i)
		// Python typed parameters have type "typed_parameter" or "typed_default_parameter"
		if param.Type() != "typed_parameter" && param.Type() != "typed_default_parameter" {
			continue
		}
		// The type annotation is in the "type" field.
		typeNode := param.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		typeName := extractTypeName(typeNode, opts.Content)
		if typeName == "" || seen[typeName] {
			continue
		}
		seen[typeName] = true
		e.emitTypeHintEdge(opts, funcHash, typeName, classContext, pyImports, result)
	}

	// Return type annotation.
	returnType := funcNode.ChildByFieldName("return_type")
	if returnType != nil {
		typeName := extractTypeName(returnType, opts.Content)
		if typeName != "" && !seen[typeName] {
			seen[typeName] = true
			e.emitTypeHintEdge(opts, funcHash, typeName, classContext, pyImports, result)
		}
	}
}

// extractTypeName gets the base type name from a type annotation node.
// Handles: simple identifiers, subscripts (Generic[T] -> Generic), attributes (mod.Type -> Type).
func extractTypeName(node *sitter.Node, content []byte) string {
	switch node.Type() {
	case "identifier":
		name := node.Content(content)
		// Skip builtins that are never useful as graph targets.
		switch name {
		case "int", "str", "float", "bool", "bytes", "None", "Any", "object", "type", "list", "dict", "tuple", "set":
			return ""
		}
		return name
	case "subscript":
		// Generic[T] -> extract "Generic" (the base type).
		value := node.ChildByFieldName("value")
		if value != nil {
			return extractTypeName(value, content)
		}
	case "attribute":
		// module.Type -> extract "Type" (terminal name) for import resolution.
		// Full dotted name for qualified resolution.
		return node.Content(content)
	case "none":
		return ""
	}
	// For other node types, try the raw content if it looks like an identifier.
	text := node.Content(content)
	if len(text) > 0 && len(text) < 50 && text[0] >= 'A' && text[0] <= 'Z' {
		return text
	}
	return ""
}

// emitTypeHintEdge resolves a type name and creates a type_hint_of edge.
func (e *TreeSitterExtractor) emitTypeHintEdge(opts types.ExtractOptions, funcHash types.Hash, typeName string, classContext string, pyImports map[string]string, result *types.ExtractResult) {
	// Resolve through import map (same as call resolution).
	targetQN := e.resolveCallTarget(opts, classContext, typeName, pyImports)
	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, targetQN, "type")

	edgeHash := types.ComputeEdgeHash(funcHash, targetHash, edgetype.TypeHintOf, "ast_inferred")
	result.Edges = append(result.Edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: funcHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.TypeHintOf,
		Confidence: 0.7,
		Provenance: "ast_inferred",
	})
}

// extractClass extracts a class definition and its methods.
func (e *TreeSitterExtractor) extractClassWithImports(node *sitter.Node, opts types.ExtractOptions, pyImports map[string]string, result *types.ExtractResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	className := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, "", className)
	n := e.makeNode(opts, qname, "type", line)

	// Extract Python docstring from class body.
	classBody := node.ChildByFieldName("body")
	n.Doc = extractPythonDocstring(classBody, opts.Content)

	result.Nodes = append(result.Nodes, n)

	// Emit extends edges with import-resolved target hashes.
	e.extractPythonBaseClasses(node, opts, n.NodeHash, pyImports, result)

	// Extract class field declarations as field nodes.
	fieldNodes := extractClassFieldNodes(node, opts, className)
	result.Nodes = append(result.Nodes, fieldNodes...)

	// Emit decorates edges for decorators on this class.
	e.extractPythonDecoratorEdges(node, opts, n.NodeHash, result)

	// Walk class body for method definitions.
	body := node.ChildByFieldName("body")
	if body == nil {
		return
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		switch child.Type() {
		case "function_definition":
			e.extractFunction(child, opts, className, result)
		case "decorated_definition":
			for j := 0; j < int(child.ChildCount()); j++ {
				inner := child.Child(j)
				if inner.Type() == "function_definition" {
					e.extractFunction(inner, opts, className, result)
				}
			}
		}
	}
}

func (e *TreeSitterExtractor) extractClass(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	className := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, "", className)
	n := e.makeNode(opts, qname, "type", line)
	result.Nodes = append(result.Nodes, n)

	// Emit extends edges for base classes (argument_list after class name).
	e.extractPythonBaseClasses(node, opts, n.NodeHash, nil, result)

	// Emit decorates edges for decorators on this class.
	e.extractPythonDecoratorEdges(node, opts, n.NodeHash, result)

	// Walk class body with class context for methods.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			e.walkNode(body.Child(i), opts, className, types.EmptyHash, result)
		}
	}
}

// extractImport extracts import statements and creates edges.
// For internal imports (same repo), resolves the module path to an actual file and
// creates import edges that point to real file/module nodes (enabling RWR traversal).
func (e *TreeSitterExtractor) extractImport(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	importText := node.Content(opts.Content)
	moduleName := parseImportModule(importText)
	if moduleName == "" {
		return
	}

	// Determine source hash (the importing file).
	sourceQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
	if classContext != "" {
		sourceQName = fmt.Sprintf("%s.%s", sourceQName, classContext)
	}
	sourceHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, sourceQName, "module")

	// Try to resolve as an internal module first (file exists on disk).
	// This creates import edges that match real nodes, enabling RWR to traverse
	// from importing file to imported module's symbols.
	targetRepoURL := opts.RepoURL
	modulePath := resolveModuleToPath(moduleName, opts.ModuleRoot)
	fullPath := filepath.Join(opts.ModuleRoot, modulePath)
	isInternal := false
	if _, err := os.Stat(fullPath); err == nil {
		isInternal = true
	}

	if !isInternal {
		// External import: use external repo URL if detectable.
		if extURL := resolve.InferExternalRepoURL(moduleName, "", resolve.PythonConfig); extURL != "" {
			targetRepoURL = extURL
		}
	}

	var targetHash types.Hash
	if isInternal {
		// Internal import: compute hash that matches the file node created during extraction.
		// File nodes are hashed as: ComputeNodeHash(repoURL, filePath, EmptyHash, basename, KindFile)
		// But we also want to match type/class nodes in that file.
		// Use the file's qualified name format to match what the extractor produces.
		targetQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, modulePath)
		targetHash = types.ComputeNodeHash(targetRepoURL, opts.ModuleRoot, types.EmptyHash, targetQName, "module")
	} else {
		targetHash = types.ComputeNodeHash(targetRepoURL, opts.ModuleRoot, types.EmptyHash, moduleName, "module")
	}

	provenance := "ast_resolved"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Imports, provenance)
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.Imports,
		Confidence: 1.0,
		Provenance: provenance,
	}
	result.Edges = append(result.Edges, edge)

	// For "from X import Y" statements, also create edges to the imported symbols.
	// If the import is "from django.db.migrations import operations", create an
	// edge to the operations package. If "from operations import base", resolve
	// base as either a submodule (operations/base.py) or a symbol.
	if node.Type() == "import_from_statement" {
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			var importedName string
			switch child.Type() {
			case "dotted_name":
				moduleNode := node.ChildByFieldName("module_name")
				if child == moduleNode {
					continue // skip the source module itself
				}
				importedName = child.Content(opts.Content)
			case "aliased_import":
				nameNode := child.ChildByFieldName("name")
				if nameNode != nil {
					importedName = nameNode.Content(opts.Content)
				}
			default:
				continue
			}
			if importedName == "" {
				continue
			}

			// Try to resolve importedName as a submodule of moduleName.
			subModulePath := resolveModuleToPath(moduleName+"."+importedName, opts.ModuleRoot)
			subFullPath := filepath.Join(opts.ModuleRoot, subModulePath)
			if _, err := os.Stat(subFullPath); err == nil {
				// importedName is a submodule file. Create edge to that file's types.
				subTargetQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, subModulePath)
				subTargetHash := types.ComputeNodeHash(targetRepoURL, opts.ModuleRoot, types.EmptyHash, subTargetQName, "module")
				subEdgeHash := types.ComputeEdgeHash(sourceHash, subTargetHash, edgetype.Imports, provenance)
				result.Edges = append(result.Edges, types.Edge{
					EdgeHash:   subEdgeHash,
					SourceHash: sourceHash,
					TargetHash: subTargetHash,
					EdgeType:   edgetype.Imports,
					Confidence: 1.0,
					Provenance: provenance,
				})
			}
		}
	}
}

// extractCall extracts function call expressions and creates call edges.
func (e *TreeSitterExtractor) extractCall(node *sitter.Node, opts types.ExtractOptions, classContext string, funcHash types.Hash, result *types.ExtractResult) {
	e.extractCallWithImports(node, opts, classContext, funcHash, nil, result)
}

// extractCallWithImports extracts function call expressions and creates call edges.
// When pyImports is provided, it resolves call targets through the import map so
// that cross-file calls (e.g., Flask.before_request -> flask.app.Flask.before_request)
// produce edges pointing to the correct target node.
func (e *TreeSitterExtractor) extractCallWithImports(node *sitter.Node, opts types.ExtractOptions, classContext string, funcHash types.Hash, pyImports map[string]string, result *types.ExtractResult) {
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		return
	}
	calledName := funcNode.Content(opts.Content)

	// Source: use the enclosing function's node hash when available (enables
	// the enricher to find this edge via NodesByName). Fall back to a synthetic
	// module-level hash for top-level calls outside any function.
	var sourceHash types.Hash
	if funcHash != types.EmptyHash {
		sourceHash = funcHash
	} else {
		sourceQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
		if classContext != "" {
			sourceQName = fmt.Sprintf("%s.%s", sourceQName, classContext)
		}
		sourceHash = types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, sourceQName, "module")
	}

	// Resolve target through import map when available.
	// For "Flask.before_request()", calledName is "Flask.before_request".
	// If "Flask" is in pyImports as coming from "flask.app", we resolve
	// the target to the definition file rather than the current file.
	targetQName := e.resolveCallTarget(opts, classContext, calledName, pyImports)
	targetRepoURL := opts.RepoURL
	// Use external repo URL when the call resolves to an external package.
	if pyImports != nil {
		firstName := strings.Split(calledName, ".")[0]
		if srcModule, ok := pyImports[firstName]; ok {
			if extURL := resolve.InferExternalRepoURL(srcModule, "", resolve.PythonConfig); extURL != "" {
				targetRepoURL = extURL
			}
		}
	}
	targetHash := types.ComputeNodeHash(targetRepoURL, opts.ModuleRoot, types.EmptyHash, targetQName, types.KindFunction)

	provenance := "ast_inferred"
	confidence := 0.7
	// Higher confidence when resolved through imports (we know the source module).
	if pyImports != nil {
		firstName := strings.Split(calledName, ".")[0]
		if _, ok := pyImports[firstName]; ok {
			provenance = "ast_resolved"
			confidence = 0.85
		}
	}

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, provenance)
	edge := types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.Calls,
		Confidence:   confidence,
		Provenance:   provenance,
		CallSiteLine: int(funcNode.StartPoint().Row) + 1,
		CallSiteCol:  int(funcNode.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
	result.Edges = append(result.Edges, edge)

	// Callback registration detection: when calling .connect(), .register(),
	// .before_request(), .add_url_rule(), .on(), etc., the first argument is
	// a callback function that gets invoked later. Create an edge from the
	// registrar object to the callback so RWR can walk registrar -> callback.
	e.extractCallbackRegistration(node, opts, classContext, calledName, targetHash, pyImports, result)
}

// callbackMethods is the set of method names that register callbacks.
// When obj.method(handler) is called, we create an edge from obj to handler.
var callbackMethods = map[string]bool{
	"connect":          true, // Django signals: post_save.connect(handler)
	"disconnect":       true, // Django signals: post_save.disconnect(handler)
	"register":         true, // Generic registration patterns
	"register_handler": true,
	"add_handler":      true,
	"before_request":   true, // Flask: app.before_request(func)
	"after_request":    true, // Flask: app.after_request(func)
	"teardown_request": true, // Flask: app.teardown_request(func)
	"before_app_request":   true,
	"after_app_request":    true,
	"teardown_appcontext":  true,
	"teardown_app_request": true,
	"errorhandler":     true, // Flask: app.errorhandler(404)(func)
	"add_url_rule":     true, // Flask: app.add_url_rule(rule, view_func=func)
	"on":               true, // Node.js EventEmitter: emitter.on('event', handler)
	"addEventListener": true, // DOM/Browser
	"subscribe":        true, // Observable/PubSub patterns
	"use":              true, // Express middleware: app.use(middleware)
	"add_middleware":   true, // Starlette/FastAPI
}

// extractCallbackRegistration detects registration calls and creates edges
// from the registrar to the callback argument.
func (e *TreeSitterExtractor) extractCallbackRegistration(node *sitter.Node, opts types.ExtractOptions, classContext, calledName string, registrarHash types.Hash, pyImports map[string]string, result *types.ExtractResult) {
	// Extract the terminal method name from "obj.method" or just "method".
	parts := strings.Split(calledName, ".")
	methodName := parts[len(parts)-1]
	if !callbackMethods[methodName] {
		return
	}

	// Find the arguments node in the call expression.
	argsNode := node.ChildByFieldName("arguments")
	if argsNode == nil {
		return
	}

	// The first non-keyword argument is typically the callback.
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		arg := argsNode.Child(i)
		if arg == nil {
			continue
		}
		// Skip parentheses, commas, keyword arguments.
		argType := arg.Type()
		if argType == "(" || argType == ")" || argType == "," || argType == "keyword_argument" {
			continue
		}

		// The argument content is the callback reference.
		callbackName := arg.Content(opts.Content)
		if callbackName == "" || strings.HasPrefix(callbackName, "\"") || strings.HasPrefix(callbackName, "'") {
			continue // skip string literals (event names like 'click')
		}
		// Skip numeric literals and None/True/False.
		if callbackName == "None" || callbackName == "True" || callbackName == "False" {
			continue
		}
		if len(callbackName) > 0 && callbackName[0] >= '0' && callbackName[0] <= '9' {
			continue
		}

		// Resolve the callback to a qualified name.
		callbackQName := e.resolveCallTarget(opts, classContext, callbackName, pyImports)
		callbackHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, callbackQName, types.KindFunction)

		// Create edge: registrar --calls--> callback
		cbEdgeHash := types.ComputeEdgeHash(registrarHash, callbackHash, edgetype.Calls, "callback_registration")
		result.Edges = append(result.Edges, types.Edge{
			EdgeHash:   cbEdgeHash,
			SourceHash: registrarHash,
			TargetHash: callbackHash,
			EdgeType:   edgetype.Calls,
			Confidence: 0.8,
			Provenance: "callback_registration",
		})
		break // only process the first callback argument
	}
}

// resolveIntraFileCallsByName is a placeholder for future same-file method resolution.
// The full implementation requires type inference (knowing what type a variable holds)
// which is what LSP enrichment (pyright/gopls) provides. Without type info, we can't
// reliably determine that `op.database_forwards()` calls `Operation.database_forwards`
// rather than some other class's `database_forwards` method.
//
// The existing contains + member_of + inherits edges handle the structural connections.
// For dynamic dispatch resolution, use `knowing index` with enrichment enabled.
func resolveIntraFileCallsByName(_ types.ExtractOptions, _ *types.ExtractResult) {
	// No-op: deferred to LSP enrichment.
}

// resolveCallTarget resolves a called name to its qualified target using the
// import map. For "Flask.before_request", if "Flask" was imported from "flask.app",
// the target becomes "repoURL://moduleRoot/src/flask/app.py.Flask.before_request".
func (e *TreeSitterExtractor) resolveCallTarget(opts types.ExtractOptions, classContext, calledName string, pyImports map[string]string) string {
	if pyImports == nil || calledName == "" {
		return e.qualifiedName(opts, classContext, calledName)
	}

	// Split "Flask.before_request" into ["Flask", "before_request"]
	parts := strings.Split(calledName, ".")
	firstName := parts[0]

	srcModule, ok := pyImports[firstName]
	if !ok {
		// Not an imported name; use local resolution.
		return e.qualifiedName(opts, classContext, calledName)
	}

	// Handle "from X import Y" where Y might be a submodule (file) rather than a symbol.
	// Example: "from django.db.migrations.operations import base"
	//   -> srcModule = "django.db.migrations.operations", firstName = "base"
	//   -> base is a submodule: operations/base.py
	//   -> call "base.Operation.state_forwards()" should resolve to operations/base.py.Operation.state_forwards
	//
	// Strategy: try resolving as submodule first (srcModule/firstName.py exists?),
	// then fall back to symbol in the module (srcModule.py.firstName).

	// Try 1: firstName is a submodule file (from package import submodule).
	subModulePath := resolveModuleToPath(srcModule+"."+firstName, opts.ModuleRoot)
	if subModulePath != "" {
		// Check if this path actually exists on disk.
		fullSubPath := filepath.Join(opts.ModuleRoot, subModulePath)
		if _, err := os.Stat(fullSubPath); err == nil {
			// firstName is a submodule. The remaining parts are the symbol path.
			if len(parts) > 1 {
				symbolPart := strings.Join(parts[1:], ".")
				return fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, subModulePath, symbolPart)
			}
			// Just the module itself (no symbol access after import).
			return fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, subModulePath)
		}
	}

	// Try 2: firstName is a symbol in the module (standard case).
	// "Flask.before_request" with module "flask.app" -> "repoURL://moduleRoot/src/flask/app.py.Flask.before_request"
	modulePath := resolveModuleToPath(srcModule, opts.ModuleRoot)
	if modulePath == "" {
		return e.qualifiedName(opts, classContext, calledName)
	}

	symbolPart := strings.Join(parts, ".")
	return fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, modulePath, symbolPart)
}

// resolveModuleToPath converts a Python module path to a file path.
// "flask.app" -> "src/flask/app.py" (tries common source layouts)
// "flask" -> "src/flask/__init__.py"
// Returns empty string if resolution fails.
func resolveModuleToPath(modulePath, moduleRoot string) string {
	// Convert dots to path separators.
	relPath := strings.ReplaceAll(modulePath, ".", "/")

	// Try common Python source layouts.
	candidates := []string{
		"src/" + relPath + ".py",
		relPath + ".py",
		"src/" + relPath + "/__init__.py",
		relPath + "/__init__.py",
	}

	for _, candidate := range candidates {
		fullPath := filepath.Join(moduleRoot, candidate)
		if _, err := os.Stat(fullPath); err == nil {
			return candidate
		}
	}

	// No matching file found. Return empty to signal unresolvable.
	// The in-process Python resolver handles these cases with full
	// cross-file type resolution.
	return ""
}

// buildPythonImportMap extracts all import statements from a Python AST and
// builds a map from imported names to their source modules.
func buildPythonImportMap(root *sitter.Node, content []byte) map[string]string {
	imports := make(map[string]string)

	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		switch node.Type() {
		case "import_from_statement":
			// "from flask.app import Flask, request_ctx"
			// module_name is "flask.app", imported names are "Flask", "request_ctx"
			moduleNode := node.ChildByFieldName("module_name")
			if moduleNode == nil {
				continue
			}
			moduleName := moduleNode.Content(content)

			// Collect all imported names.
			for j := 0; j < int(node.ChildCount()); j++ {
				child := node.Child(j)
				if child == nil {
					continue
				}
				if child.Type() == "dotted_name" && child != moduleNode {
					imports[child.Content(content)] = moduleName
				} else if child.Type() == "aliased_import" {
					// "from X import Y as Z" -> map Z to module X
					alias := child.ChildByFieldName("alias")
					name := child.ChildByFieldName("name")
					if alias != nil {
						imports[alias.Content(content)] = moduleName
					} else if name != nil {
						imports[name.Content(content)] = moduleName
					}
				}
			}

		case "import_statement":
			// "import flask" or "import flask as f"
			for j := 0; j < int(node.ChildCount()); j++ {
				child := node.Child(j)
				if child == nil {
					continue
				}
				if child.Type() == "dotted_name" {
					name := child.Content(content)
					imports[name] = name
				} else if child.Type() == "aliased_import" {
					alias := child.ChildByFieldName("alias")
					name := child.ChildByFieldName("name")
					if alias != nil && name != nil {
						imports[alias.Content(content)] = name.Content(content)
					} else if name != nil {
						imports[name.Content(content)] = name.Content(content)
					}
				}
			}
		}
	}

	return imports
}

// extractRaise extracts a throws edge from a raise_statement node.
// It looks for the raised type from "raise ExceptionType(...)" patterns.
func (e *TreeSitterExtractor) extractRaise(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	// Build the source hash from the enclosing context.
	sourceQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
	if classContext != "" {
		sourceQName = fmt.Sprintf("%s.%s", sourceQName, classContext)
	}
	sourceHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, sourceQName, "module")

	// Look for the raised type. Patterns:
	// raise ValueError("msg") -> call node with function=identifier("ValueError")
	// raise ValueError -> identifier("ValueError")
	// raise -> bare re-raise (skip)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		var errorType string

		switch child.Type() {
		case "call":
			funcNode := child.ChildByFieldName("function")
			if funcNode != nil {
				errorType = funcNode.Content(opts.Content)
			}
		case "identifier":
			errorType = child.Content(opts.Content)
		}

		if errorType != "" {
			targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, errorType, "error")
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Throws, provenance)
			result.Edges = append(result.Edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: sourceHash,
				TargetHash: targetHash,
				EdgeType:   edgetype.Throws,
				Confidence: 0.7,
				Provenance: provenance,
			})
			return
		}
	}
}

// parseImportModule extracts the module name from an import statement string.
func parseImportModule(importText string) string {
	// Handle "import foo", "import foo.bar", "from foo import bar", "from foo.bar import baz"
	importText = strings.TrimSpace(importText)

	if strings.HasPrefix(importText, "from ") {
		parts := strings.Fields(importText)
		if len(parts) >= 2 {
			return parts[1]
		}
		return ""
	}

	if strings.HasPrefix(importText, "import ") {
		parts := strings.Fields(importText)
		if len(parts) >= 2 {
			// Handle "import foo as bar" => "foo"
			return parts[1]
		}
		return ""
	}

	return ""
}

// pythonRouteDecorators maps decorator method names to HTTP methods.
// Shared by Flask and FastAPI (both use @app.get, @app.post, etc.)
var pythonRouteDecorators = map[string]string{
	"get":     "GET",
	"post":    "POST",
	"put":     "PUT",
	"delete":  "DELETE",
	"patch":   "PATCH",
	"route":   "ANY",
	"head":    "HEAD",
	"options": "OPTIONS",
}

// extractPythonRoutes detects Flask/FastAPI/Django route patterns and creates
// route_handler nodes with handles_route edges.
func (e *TreeSitterExtractor) extractPythonRoutes(root *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	// Walk top-level looking for decorated function definitions.
	// Pattern: @app.get("/path") or @router.post("/path") followed by def handler():
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "decorated_definition" {
			e.tryExtractPythonDecoratedRoute(child, opts, "", result)
		}
		// Also check inside class bodies (Django class-based views, FastAPI routers).
		if child.Type() == "class_definition" {
			nameNode := child.ChildByFieldName("name")
			className := ""
			if nameNode != nil {
				className = nameNode.Content(opts.Content)
			}
			body := child.ChildByFieldName("body")
			if body != nil {
				for j := 0; j < int(body.ChildCount()); j++ {
					member := body.Child(j)
					if member.Type() == "decorated_definition" {
						e.tryExtractPythonDecoratedRoute(member, opts, className, result)
					}
				}
			}
		}
	}

	// Django urls.py: path('url/', view_func) calls.
	if strings.Contains(opts.FilePath, "urls") {
		e.extractDjangoURLPatterns(root, opts, result)
	}
}

// tryExtractPythonDecoratedRoute checks if a decorated_definition has a route decorator.
func (e *TreeSitterExtractor) tryExtractPythonDecoratedRoute(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	// A decorated_definition has decorator children followed by the definition.
	var decoratorText string
	var funcName string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "decorator":
			decoratorText = child.Content(opts.Content)
		case "function_definition":
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				funcName = nameNode.Content(opts.Content)
			}
		}
	}

	if decoratorText == "" || funcName == "" {
		return
	}

	// Match @app.get("/path"), @router.post("/path"), @bp.route("/path")
	httpMethod, routePath := parsePythonRouteDecorator(decoratorText)
	if httpMethod == "" {
		return
	}

	routeSig := httpMethod + " " + routePath
	qnamePrefix := fmt.Sprintf("%s/%s", opts.ModuleRoot, opts.FilePath)

	// Create route_handler node.
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, types.KindRoute)
	routeNode := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
		Kind:          types.KindRoute,
		Signature:     routeSig,
	}
	result.Nodes = append(result.Nodes, routeNode)

	// Create handles_route edge to the handler function.
	handlerQName := e.qualifiedName(opts, classContext, funcName)
	handlerHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, handlerQName, types.KindFunction)
	edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, edgetype.HandlesRoute, "ast_inferred")
	result.Edges = append(result.Edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: nodeHash,
		TargetHash: handlerHash,
		EdgeType:   edgetype.HandlesRoute,
		Confidence: 0.7,
		Provenance: "ast_inferred",
	})
}

// parsePythonRouteDecorator parses @app.get("/users") style decorators.
// Returns the HTTP method and route path, or empty strings if not a route decorator.
func parsePythonRouteDecorator(text string) (string, string) {
	// Remove leading @
	text = strings.TrimPrefix(text, "@")

	// Find the method name: everything after the last dot, before the paren.
	// e.g., "app.get('/users')" -> method="get", or "router.post('/items')" -> method="post"
	parenIdx := strings.Index(text, "(")
	if parenIdx < 0 {
		return "", ""
	}
	prefix := text[:parenIdx]
	dotIdx := strings.LastIndex(prefix, ".")
	if dotIdx < 0 {
		return "", ""
	}
	methodName := prefix[dotIdx+1:]

	httpMethod, ok := pythonRouteDecorators[methodName]
	if !ok {
		return "", ""
	}

	// Extract the route path from the first string argument.
	argPart := text[parenIdx+1:]
	endParen := strings.LastIndex(argPart, ")")
	if endParen >= 0 {
		argPart = argPart[:endParen]
	}

	// Find first quoted string.
	routePath := extractQuotedString(argPart)
	if routePath == "" {
		return "", ""
	}

	return httpMethod, routePath
}

// extractDjangoURLPatterns extracts route patterns from Django urls.py files.
// Looks for path('url/', view_func) or re_path(r'^url/$', view_func) calls.
func (e *TreeSitterExtractor) extractDjangoURLPatterns(root *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	// Walk looking for call expressions where the function is "path" or "re_path".
	e.walkForDjangoURLs(root, opts, result)
}

func (e *TreeSitterExtractor) walkForDjangoURLs(node *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	if node == nil {
		return
	}
	if node.Type() == "call" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			funcName := funcNode.Content(opts.Content)
			if funcName == "path" || funcName == "re_path" {
				e.tryExtractDjangoPath(node, opts, result)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		e.walkForDjangoURLs(node.Child(i), opts, result)
	}
}

func (e *TreeSitterExtractor) tryExtractDjangoPath(callNode *sitter.Node, opts types.ExtractOptions, result *types.ExtractResult) {
	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return
	}

	// First arg is the URL pattern, second is the view.
	var urlPattern, viewName string
	argIdx := 0
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		t := child.Type()
		if t == "," || t == "(" || t == ")" {
			continue
		}
		argIdx++
		if argIdx == 1 {
			// URL pattern (string).
			text := child.Content(opts.Content)
			urlPattern = strings.Trim(text, `"'`)
		} else if argIdx == 2 {
			// View function or class.
			viewName = child.Content(opts.Content)
			break
		}
	}

	if urlPattern == "" || viewName == "" {
		return
	}

	if !strings.HasPrefix(urlPattern, "/") {
		urlPattern = "/" + urlPattern
	}

	routeSig := "ANY " + urlPattern
	qnamePrefix := fmt.Sprintf("%s/%s", opts.ModuleRoot, opts.FilePath)

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, types.KindRoute)
	routeNode := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
		Kind:          types.KindRoute,
		Signature:     routeSig,
	}
	result.Nodes = append(result.Nodes, routeNode)

	// Edge to the view function.
	handlerQName := e.qualifiedName(opts, "", viewName)
	handlerHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, handlerQName, types.KindFunction)
	edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, edgetype.HandlesRoute, "ast_inferred")
	result.Edges = append(result.Edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: nodeHash,
		TargetHash: handlerHash,
		EdgeType:   edgetype.HandlesRoute,
		Confidence: 0.7,
		Provenance: "ast_inferred",
	})
}

// extractPythonBaseClasses checks a class_definition for base classes in the
// superclasses/argument_list field and emits "extends" edges.
// Uses pyImports to resolve base class names to their actual definition file,
// ensuring the target hash matches the actual class node's hash.
func (e *TreeSitterExtractor) extractPythonBaseClasses(classNode *sitter.Node, opts types.ExtractOptions, classHash types.Hash, pyImports map[string]string, result *types.ExtractResult) {
	// In Python tree-sitter grammar, base classes appear in the "superclasses"
	// field (an argument_list node).
	superclasses := classNode.ChildByFieldName("superclasses")
	if superclasses == nil {
		return
	}
	for i := 0; i < int(superclasses.ChildCount()); i++ {
		child := superclasses.Child(i)
		// Skip punctuation: parens, commas.
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}
		// Skip keyword arguments like metaclass=ABCMeta.
		if child.Type() == "keyword_argument" {
			continue
		}
		baseName := child.Content(opts.Content)
		if baseName == "" {
			continue
		}

		// Skip Python builtins: they don't add reachability within a repo
		// and produce dangling edges (no corresponding node in the graph).
		if isPythonBuiltinClass(baseName) {
			continue
		}

		// Resolve base class through import map to compute the SAME hash
		// that makeNode produces when the target class is indexed.
		// makeNode uses: ComputeNodeHash(repoURL, moduleRoot, _, qualifiedName, kind)
		// where qualifiedName is "repoURL://moduleRoot/filepath.ClassName"
		targetQName := resolveBaseClassQName(baseName, opts, pyImports)
		if targetQName == "" {
			continue // unresolvable (dotted path through unresolved module)
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, targetQName, types.KindType)

		provenance := "ast_inferred"
		confidence := 0.7
		if pyImports != nil {
			if _, ok := pyImports[baseName]; ok {
				provenance = "ast_resolved"
				confidence = 0.85
			}
		}

		edgeHash := types.ComputeEdgeHash(classHash, targetHash, edgetype.Extends, provenance)
		result.Edges = append(result.Edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: classHash,
			TargetHash: targetHash,
			EdgeType:   edgetype.Extends,
			Confidence: confidence,
			Provenance: provenance,
		})
	}
}

// resolveBaseClassQName builds the full qualified name for a base class,
// matching the format that makeNode/qualifiedName produces when the target
// class is indexed. Format: "repoURL://moduleRoot/filepath.ClassName"
func resolveBaseClassQName(className string, opts types.ExtractOptions, pyImports map[string]string) string {
	if pyImports == nil {
		// No import map: assume defined in same file.
		return fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath, className)
	}

	// Direct match: className is imported directly (e.g., "from x import MyClass").
	srcModule, ok := pyImports[className]
	if ok {
		modulePath := resolveModuleToPath(srcModule, opts.ModuleRoot)
		if modulePath == "" {
			return "" // module path unresolvable
		}
		return fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, modulePath, className)
	}

	// Dotted path: className is "module.ClassName" (e.g., "validators.RegexValidator").
	// Split into module alias and class name, look up the module alias.
	if dotIdx := strings.Index(className, "."); dotIdx > 0 {
		moduleAlias := className[:dotIdx]
		actualClass := className[dotIdx+1:]
		if srcModule, ok := pyImports[moduleAlias]; ok {
			modulePath := resolveModuleToPath(srcModule, opts.ModuleRoot)
			if modulePath == "" {
				return "" // module path unresolvable
			}
			return fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, modulePath, actualClass)
		}
		// Dotted path but module alias not in imports: unresolvable.
		return ""
	}

	// Not imported and not a dotted path: assume defined in same file.
	// This is correct for same-file inheritance (e.g. class FooError(BarError):
	// where both are in the same file). May produce dangling edges for
	// star-imported classes, but those are handled by the Python resolver.
	return fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath, className)
}

// isPythonBuiltinClass returns true for Python builtin types that should not
// produce extends edges (they have no corresponding node in the graph).
func isPythonBuiltinClass(name string) bool {
	switch name {
	case "object", "type",
		"Exception", "BaseException",
		"ValueError", "TypeError", "KeyError", "IndexError", "AttributeError",
		"RuntimeError", "NotImplementedError", "StopIteration", "StopAsyncIteration",
		"OSError", "IOError", "FileNotFoundError", "PermissionError",
		"ImportError", "ModuleNotFoundError",
		"LookupError", "ArithmeticError", "BufferError",
		"EOFError", "MemoryError", "NameError", "ReferenceError",
		"SyntaxError", "SystemError", "UnicodeError", "UnicodeDecodeError", "UnicodeEncodeError",
		"ConnectionError", "TimeoutError", "BrokenPipeError",
		"OverflowError", "ZeroDivisionError", "FloatingPointError",
		"AssertionError", "RecursionError", "BlockingIOError",
		"ChildProcessError", "ConnectionAbortedError", "ConnectionRefusedError", "ConnectionResetError",
		"FileExistsError", "InterruptedError", "IsADirectoryError", "NotADirectoryError",
		"ProcessLookupError", "UnboundLocalError",
		"Warning", "UserWarning", "DeprecationWarning", "PendingDeprecationWarning",
		"RuntimeWarning", "SyntaxWarning", "ResourceWarning", "FutureWarning",
		"UnicodeWarning", "BytesWarning",
		"dict", "list", "set", "frozenset", "tuple",
		"str", "bytes", "bytearray", "memoryview",
		"int", "float", "complex", "bool",
		"property", "staticmethod", "classmethod",
		"super", "enumerate", "filter", "map", "zip", "range", "reversed",
		"ABC":
		return true
	}
	return false
}

// extractPythonDecoratorEdges checks for decorator nodes that precede a
// function or class definition (inside a decorated_definition parent) and
// emits "decorates" edges from the decorator to the declaration.
func (e *TreeSitterExtractor) extractPythonDecoratorEdges(declNode *sitter.Node, opts types.ExtractOptions, declHash types.Hash, result *types.ExtractResult) {
	parent := declNode.Parent()
	if parent == nil || parent.Type() != "decorated_definition" {
		return
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == declNode {
			break
		}
		if child.Type() == "decorator" {
			decoratorText := child.Content(opts.Content)
			decoratorName := parsePythonDecoratorName(decoratorText)
			if decoratorName == "" {
				continue
			}
			targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, decoratorName, types.KindFunction)
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(targetHash, declHash, edgetype.Decorates, provenance)
			result.Edges = append(result.Edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: targetHash,
				TargetHash: declHash,
				EdgeType:   edgetype.Decorates,
				Confidence: 0.7,
				Provenance: provenance,
			})
		}
	}
}

// parsePythonDecoratorName extracts the decorator function name from text
// like "@staticmethod" or "@app.route('/path')". Returns the name portion
// before any arguments.
func parsePythonDecoratorName(text string) string {
	text = strings.TrimPrefix(text, "@")
	if idx := strings.Index(text, "("); idx > 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	return text
}

// extractQuotedString finds the first single or double quoted string in text.
func extractQuotedString(text string) string {
	for _, q := range []byte{'"', '\''} {
		start := strings.IndexByte(text, q)
		if start >= 0 {
			end := strings.IndexByte(text[start+1:], q)
			if end >= 0 {
				return text[start+1 : start+1+end]
			}
		}
	}
	return ""
}
