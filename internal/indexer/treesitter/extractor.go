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
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/blackwell-systems/knowing/internal/types"
)

// TreeSitterExtractor implements types.Extractor using tree-sitter for AST parsing.
type TreeSitterExtractor struct {
	language string
	parser   *sitter.Parser
}

// NewTreeSitterExtractor creates a new tree-sitter extractor for the given language.
// Currently only "python" is supported.
func NewTreeSitterExtractor(language string) (*TreeSitterExtractor, error) {
	lang := strings.ToLower(language)
	if lang != "python" {
		return nil, fmt.Errorf("unsupported language: %s (only python is supported)", language)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())

	return &TreeSitterExtractor{
		language: lang,
		parser:   parser,
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
	tree, err := e.parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	result := &types.ExtractResult{}

	// Walk the AST and extract symbols.
	e.walkNode(root, opts, "", result)

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
// The classContext parameter tracks whether we are inside a class body;
// if non-empty, functions are treated as methods and calls are scoped
// to the class context.
func (e *TreeSitterExtractor) walkNode(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	if node == nil {
		return
	}

	// Match against Python grammar node type names. Tree-sitter node types
	// are strings defined by the language grammar (e.g., "function_definition"
	// for Python's "def foo():").
	switch node.Type() {
	case "function_definition":
		e.extractFunction(node, opts, classContext, result)
	case "class_definition":
		e.extractClass(node, opts, result)
	case "import_statement", "import_from_statement":
		e.extractImport(node, opts, classContext, result)
	case "call":
		e.extractCall(node, opts, classContext, result)
	default:
		// Recurse into children for non-extracted node types.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			e.walkNode(child, opts, classContext, result)
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

	// Walk function body for calls and imports.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			e.walkNode(body.Child(i), opts, classContext, result)
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

	// Walk class body with class context for methods.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			e.walkNode(body.Child(i), opts, className, result)
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
func (e *TreeSitterExtractor) extractCall(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		return
	}
	calledName := funcNode.Content(opts.Content)
	line := int(funcNode.StartPoint().Row) + 1

	// Source is the enclosing context (file or class).
	sourceQName := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
	if classContext != "" {
		sourceQName = fmt.Sprintf("%s.%s", sourceQName, classContext)
	}
	sourceHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, opts.FileHash, sourceQName, "module")

	// Target is the called function (may be unresolved).
	targetQName := e.qualifiedName(opts, classContext, calledName)
	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, targetQName, "function")
	_ = line // line is informational, not stored on edges

	provenance := "ast_resolved"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: provenance,
	}
	result.Edges = append(result.Edges, edge)
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
