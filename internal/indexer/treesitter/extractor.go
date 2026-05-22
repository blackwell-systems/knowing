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
	case "class_definition":
		e.extractClass(node, opts, result)
	case "import_statement", "import_from_statement":
		e.extractImport(node, opts, classContext, result)
	case "call":
		e.extractCallWithImports(node, opts, classContext, funcHash, pyImports, result)
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

// extractFunction extracts a function definition node.
func (e *TreeSitterExtractor) extractFunction(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1 // 1-indexed

	kind := "function"
	if classContext != "" {
		kind = "method"
	}

	qname := e.qualifiedName(opts, classContext, name)
	n := e.makeNode(opts, qname, kind, line)
	result.Nodes = append(result.Nodes, n)

	// Emit decorates edges for decorators on this function/method.
	e.extractPythonDecoratorEdges(node, opts, n.NodeHash, result)

	// Walk function body for calls and imports, passing this function's
	// node hash so calls use it as edge source.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			e.walkNode(body.Child(i), opts, classContext, n.NodeHash, result)
		}
	}
}

// extractClass extracts a class definition and its methods.
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
	e.extractPythonBaseClasses(node, opts, n.NodeHash, result)

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
func (e *TreeSitterExtractor) extractImport(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	// Build an import node representing the imported module.
	importText := node.Content(opts.Content)
	moduleName := parseImportModule(importText)
	if moduleName == "" {
		return
	}

	// Create a node for the import target (the module being imported).
	targetQName := moduleName
	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, targetQName, "module")

	// Create an edge from the file-level context to the imported module.
	sourceQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
	if classContext != "" {
		sourceQName = fmt.Sprintf("%s.%s", sourceQName, classContext)
	}
	sourceHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, sourceQName, "module")

	provenance := "ast_resolved"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "imports", provenance)
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "imports",
		Confidence: 1.0,
		Provenance: provenance,
	}
	result.Edges = append(result.Edges, edge)
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
	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, targetQName, "function")

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

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)
	edge := types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   confidence,
		Provenance:   provenance,
		CallSiteLine: int(funcNode.StartPoint().Row) + 1,
		CallSiteCol:  int(funcNode.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
	result.Edges = append(result.Edges, edge)
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

	// Resolve the module path to a file path within this repo.
	// "flask.app" -> "src/flask/app.py" (convention: replace dots with /, append .py)
	// "flask" -> "src/flask/__init__.py" (package import)
	modulePath := resolveModuleToPath(srcModule, opts.ModuleRoot)
	if modulePath == "" {
		return e.qualifiedName(opts, classContext, calledName)
	}

	// Build the target qualified name using the resolved file path.
	// "Flask.before_request" with module "flask.app" -> "repoURL://moduleRoot/src/flask/app.py.Flask.before_request"
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

	// If no file found, still return the best guess (the .py variant).
	// The target hash may not match an actual node, but it creates an edge
	// that could be resolved later by the cross-repo resolver.
	return "src/" + relPath + ".py"
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
			edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
			result.Edges = append(result.Edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: sourceHash,
				TargetHash: targetHash,
				EdgeType:   "throws",
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
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, "route_handler")
	routeNode := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
		Kind:          "route_handler",
		Signature:     routeSig,
	}
	result.Nodes = append(result.Nodes, routeNode)

	// Create handles_route edge to the handler function.
	handlerQName := e.qualifiedName(opts, classContext, funcName)
	handlerHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, handlerQName, "function")
	edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", "ast_inferred")
	result.Edges = append(result.Edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: nodeHash,
		TargetHash: handlerHash,
		EdgeType:   "handles_route",
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

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, "route_handler")
	routeNode := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
		Kind:          "route_handler",
		Signature:     routeSig,
	}
	result.Nodes = append(result.Nodes, routeNode)

	// Edge to the view function.
	handlerQName := e.qualifiedName(opts, "", viewName)
	handlerHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, handlerQName, "function")
	edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", "ast_inferred")
	result.Edges = append(result.Edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: nodeHash,
		TargetHash: handlerHash,
		EdgeType:   "handles_route",
		Confidence: 0.7,
		Provenance: "ast_inferred",
	})
}

// extractPythonBaseClasses checks a class_definition for base classes in the
// superclasses/argument_list field and emits "extends" edges.
func (e *TreeSitterExtractor) extractPythonBaseClasses(classNode *sitter.Node, opts types.ExtractOptions, classHash types.Hash, result *types.ExtractResult) {
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
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, baseName, "type")
		provenance := "ast_inferred"
		edgeHash := types.ComputeEdgeHash(classHash, targetHash, "extends", provenance)
		result.Edges = append(result.Edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: classHash,
			TargetHash: targetHash,
			EdgeType:   "extends",
			Confidence: 0.7,
			Provenance: provenance,
		})
	}
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
			targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, decoratorName, "function")
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(targetHash, declHash, "decorates", provenance)
			result.Edges = append(result.Edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: targetHash,
				TargetHash: declHash,
				EdgeType:   "decorates",
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
