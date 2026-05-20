// Package tsextractor provides a tree-sitter based extractor for TypeScript
// and JavaScript files. It implements types.Extractor and produces declaration
// nodes and syntactic call/import edges without type resolution.
//
// Supported file extensions: .ts, .tsx, .js, .jsx
// Excluded: files under node_modules/ directories
//
// Node types extracted:
//   - function_declaration -> "function" nodes
//   - class_declaration -> "type" nodes with nested "method" nodes
//   - interface_declaration -> "interface" nodes
//   - arrow functions assigned to variables -> "function" nodes
//   - import_statement and require() calls -> "imports" edges
//   - call_expression -> "calls" edges with call-site positions
//   - Express.js route registrations -> "route_handler" nodes and "handles_route" edges
//
// All edges use provenance "ast_inferred" and confidence 0.7, since without
// type resolution cross-module calls are heuristic.
package tsextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/blackwell-systems/knowing/internal/types"
)

// TypeScriptExtractor implements types.Extractor for TypeScript and JavaScript
// files using tree-sitter AST parsing.
type TypeScriptExtractor struct {
	parser *sitter.Parser
}

// NewTypeScriptExtractor creates a new TypeScriptExtractor with a tree-sitter
// parser configured for TypeScript.
func NewTypeScriptExtractor() *TypeScriptExtractor {
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	return &TypeScriptExtractor{parser: parser}
}

// Name returns the extractor name.
func (e *TypeScriptExtractor) Name() string {
	return "treesitter-typescript"
}

// CanHandle returns true for .ts, .tsx, .js, .jsx files that are not in
// node_modules/ directories.
func (e *TypeScriptExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx":
		// ok
	default:
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "node_modules" {
			return false
		}
	}
	return true
}

// Extract parses the TypeScript/JavaScript file with tree-sitter and produces
// nodes for declarations and edges for calls and imports.
func (e *TypeScriptExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	tree, err := e.parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	// Compute the base qualified name prefix.
	qnamePrefix := computeQNamePrefix(opts)

	// Collect import sources to detect framework usage.
	importSources := collectImportSources(root, opts.Content)
	hasExpress := importSources["express"] || importSources["fastify"] ||
		importSources["hono"] || importSources["@hono/hono"] ||
		importSources["@nestjs/common"] || importSources["next"]

	var nodes []types.Node
	var edges []types.Edge

	// File-level node hash for import edges.
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, filepath.Base(opts.FilePath), "file")

	// Walk top-level children.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		e.extractNode(child, opts, qnamePrefix, "", fileNodeHash, hasExpress, &nodes, &edges)
	}

	// NestJS: detect @Controller('prefix') + @Get/@Post decorators on methods.
	if importSources["@nestjs/common"] {
		extractNestJSRoutes(root, opts, qnamePrefix, &nodes, &edges)
	}

	// Next.js App Router: detect exported GET/POST/PUT/DELETE/PATCH functions
	// in route.ts/route.js files.
	if isNextJSRouteFile(opts.FilePath) {
		extractNextJSRoutes(root, opts, qnamePrefix, &nodes, &edges)
	}

	// Sort nodes by QualifiedName then Kind.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].QualifiedName != nodes[j].QualifiedName {
			return nodes[i].QualifiedName < nodes[j].QualifiedName
		}
		return nodes[i].Kind < nodes[j].Kind
	})

	// Sort edges by SourceHash, TargetHash, EdgeType.
	sort.Slice(edges, func(i, j int) bool {
		si, sj := edges[i], edges[j]
		if si.SourceHash != sj.SourceHash {
			return si.SourceHash.String() < sj.SourceHash.String()
		}
		if si.TargetHash != sj.TargetHash {
			return si.TargetHash.String() < sj.TargetHash.String()
		}
		return si.EdgeType < sj.EdgeType
	})

	// Deduplicate edges by EdgeHash.
	edges = deduplicateEdges(edges)

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// computeQNamePrefix builds the qualified name prefix from repo URL and file path.
// Format: {moduleRoot}/{filePath} (without extension).
func computeQNamePrefix(opts types.ExtractOptions) string {
	// Use the directory of the file as the "package" equivalent.
	dir := filepath.Dir(opts.FilePath)
	if dir == "." {
		dir = ""
	}
	// Strip the file extension to get a clean module path.
	base := strings.TrimSuffix(filepath.Base(opts.FilePath), filepath.Ext(opts.FilePath))
	if dir == "" {
		return base
	}
	return filepath.ToSlash(dir) + "/" + base
}

// collectImportSources walks top-level nodes to find import sources.
// Returns a set of module names that are imported.
func collectImportSources(root *sitter.Node, content []byte) map[string]bool {
	sources := make(map[string]bool)
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "import_statement" {
			src := child.ChildByFieldName("source")
			if src != nil {
				modName := strings.Trim(src.Content(content), `"'`)
				sources[modName] = true
			}
		}
	}
	return sources
}

// extractNode dispatches extraction based on tree-sitter node type.
func (e *TypeScriptExtractor) extractNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	fileNodeHash types.Hash,
	hasExpress bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "function_declaration":
		n := extractFuncDecl(node, opts, qnamePrefix, className)
		*nodes = append(*nodes, n)
		// Extract call edges from the function body.
		body := node.ChildByFieldName("body")
		extractCallEdgesFromBody(body, opts, qnamePrefix, n.NodeHash, hasExpress, nodes, edges)
		// Extract endpoint edges (consumes_endpoint) from HTTP client calls.
		epNodes, epEdges := ExtractEndpointEdges(body, opts, qnamePrefix, n.NodeHash)
		*nodes = append(*nodes, epNodes...)
		*edges = append(*edges, epEdges...)

	case "class_declaration":
		n := extractClassDecl(node, opts, qnamePrefix)
		*nodes = append(*nodes, n)

		// Emit extends edge if the class has an extends clause.
		extractExtendsClause(node, opts, qnamePrefix, n.NodeHash, edges)

		// Emit implements edges if the class has an implements clause.
		extractImplementsClause(node, opts, qnamePrefix, n.NodeHash, edges)

		// Emit decorates edges for decorators on this class.
		extractTSDecoratorEdges(node, opts, qnamePrefix, n.NodeHash, edges)

		// Walk class body for methods.
		body := node.ChildByFieldName("body")
		nameNode := node.ChildByFieldName("name")
		clsName := ""
		if nameNode != nil {
			clsName = nameNode.Content(opts.Content)
		}
		if body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child.Type() == "method_definition" {
					m := extractMethodDef(child, opts, qnamePrefix, clsName)
					*nodes = append(*nodes, m)

					// Emit overrides edge if the method has an override modifier.
					extractOverrideEdge(child, opts, qnamePrefix, clsName, m.NodeHash, edges)

					// Emit decorates edges for decorators on this method.
					extractTSDecoratorEdges(child, opts, qnamePrefix, m.NodeHash, edges)

					mBody := child.ChildByFieldName("body")
					extractCallEdgesFromBody(mBody, opts, qnamePrefix, m.NodeHash, hasExpress, nodes, edges)
					// Extract endpoint edges (consumes_endpoint) from HTTP client calls.
					mEpNodes, mEpEdges := ExtractEndpointEdges(mBody, opts, qnamePrefix, m.NodeHash)
					*nodes = append(*nodes, mEpNodes...)
					*edges = append(*edges, mEpEdges...)
				}
			}
		}

	case "interface_declaration":
		n := extractInterfaceDecl(node, opts, qnamePrefix)
		*nodes = append(*nodes, n)

	case "import_statement":
		importEdges := extractImportEdges(node, opts, qnamePrefix, fileNodeHash)
		*edges = append(*edges, importEdges...)

	case "lexical_declaration", "variable_declaration":
		// Check for arrow functions assigned to variables and require() calls.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "variable_declarator" {
				e.extractVariableDeclarator(child, opts, qnamePrefix, className, fileNodeHash, hasExpress, nodes, edges)
			}
		}

	case "expression_statement":
		// Check for top-level call expressions (e.g., route registrations).
		expr := node.ChildByFieldName("expression")
		if expr == nil {
			// Some expression_statements don't use the "expression" field name.
			for i := 0; i < int(node.ChildCount()); i++ {
				ch := node.Child(i)
				if ch.Type() == "call_expression" {
					expr = ch
					break
				}
			}
		}
		if expr != nil && expr.Type() == "call_expression" {
			handleTopLevelCallExpression(expr, opts, qnamePrefix, fileNodeHash, hasExpress, nodes, edges)
		}
	}
}

// extractVariableDeclarator handles variable declarators, extracting arrow
// functions and require() calls.
func (e *TypeScriptExtractor) extractVariableDeclarator(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	fileNodeHash types.Hash,
	hasExpress bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	nameNode := node.ChildByFieldName("name")
	valueNode := node.ChildByFieldName("value")
	if nameNode == nil || valueNode == nil {
		return
	}

	name := nameNode.Content(opts.Content)

	switch valueNode.Type() {
	case "arrow_function":
		n := makeArrowFuncNode(nameNode, opts, qnamePrefix, className, name)
		*nodes = append(*nodes, n)
		body := valueNode.ChildByFieldName("body")
		extractCallEdgesFromBody(body, opts, qnamePrefix, n.NodeHash, hasExpress, nodes, edges)
		// Extract endpoint edges (consumes_endpoint) from HTTP client calls.
		arrowEpNodes, arrowEpEdges := ExtractEndpointEdges(body, opts, qnamePrefix, n.NodeHash)
		*nodes = append(*nodes, arrowEpNodes...)
		*edges = append(*edges, arrowEpEdges...)

	case "call_expression":
		// Check for require() calls.
		funcNode := valueNode.ChildByFieldName("function")
		if funcNode != nil && funcNode.Type() == "identifier" && funcNode.Content(opts.Content) == "require" {
			argsNode := valueNode.ChildByFieldName("arguments")
			if argsNode != nil {
				modPath := extractFirstStringArgTS(argsNode, opts.Content)
				if modPath != "" {
					edge := makeImportEdge(opts, qnamePrefix, fileNodeHash, modPath)
					*edges = append(*edges, edge)
				}
			}
		}
	}
}

// extractFuncDecl creates a Node for a function declaration.
func extractFuncDecl(node *sitter.Node, opts types.ExtractOptions, qnamePrefix, className string) types.Node {
	nameNode := node.ChildByFieldName("name")
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	var qname string
	if className != "" {
		qname = fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, qnamePrefix, className, name)
	} else {
		qname = fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, name)
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, name, "function")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "function",
		Line:          line,
		Signature:     fmt.Sprintf("function %s()", name),
	}
}

// extractClassDecl creates a Node for a class declaration.
func extractClassDecl(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string) types.Node {
	nameNode := node.ChildByFieldName("name")
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, name, "type")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, name),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("class %s", name),
	}
}

// extractMethodDef creates a Node for a method definition inside a class.
func extractMethodDef(node *sitter.Node, opts types.ExtractOptions, qnamePrefix, className string) types.Node {
	nameNode := node.ChildByFieldName("name")
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, name, "method")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, qnamePrefix, className, name),
		Kind:          "method",
		Line:          line,
		Signature:     fmt.Sprintf("%s.%s()", className, name),
	}
}

// extractInterfaceDecl creates a Node for an interface declaration.
func extractInterfaceDecl(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string) types.Node {
	nameNode := node.ChildByFieldName("name")
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, name, "interface")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, name),
		Kind:          "interface",
		Line:          line,
		Signature:     fmt.Sprintf("interface %s", name),
	}
}

// makeArrowFuncNode creates a Node for an arrow function assigned to a variable.
func makeArrowFuncNode(nameNode *sitter.Node, opts types.ExtractOptions, qnamePrefix, className, name string) types.Node {
	line := int(nameNode.StartPoint().Row) + 1

	var qname string
	if className != "" {
		qname = fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, qnamePrefix, className, name)
	} else {
		qname = fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, name)
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, name, "function")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "function",
		Line:          line,
		Signature:     fmt.Sprintf("const %s = () => {}", name),
	}
}

// extractImportEdges creates import edges from an import_statement node.
func extractImportEdges(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string, fileNodeHash types.Hash) []types.Edge {
	src := node.ChildByFieldName("source")
	if src == nil {
		return nil
	}
	modPath := strings.Trim(src.Content(opts.Content), `"'`)
	if modPath == "" {
		return nil
	}

	edge := makeImportEdge(opts, qnamePrefix, fileNodeHash, modPath)
	return []types.Edge{edge}
}

// makeImportEdge creates a single import edge.
func makeImportEdge(opts types.ExtractOptions, qnamePrefix string, fileNodeHash types.Hash, modPath string) types.Edge {
	targetHash := types.ComputeNodeHash(opts.RepoURL, modPath, types.EmptyHash, modPath, "module")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(fileNodeHash, targetHash, "imports", provenance)
	return types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: fileNodeHash,
		TargetHash: targetHash,
		EdgeType:   "imports",
		Confidence: 0.7,
		Provenance: provenance,
	}
}

// extractCallEdgesFromBody recursively walks a node tree looking for call
// expressions and extracts call edges.
func extractCallEdgesFromBody(
	body *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	sourceHash types.Hash,
	hasExpress bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if body == nil {
		return
	}
	walkForCalls(body, opts, qnamePrefix, sourceHash, hasExpress, nodes, edges)
}

// walkForCalls recursively walks nodes looking for call_expression nodes.
func walkForCalls(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	sourceHash types.Hash,
	hasExpress bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if node == nil {
		return
	}
	switch node.Type() {
	case "call_expression":
		// Check for Express.js route registrations.
		if hasExpress {
			if rn, re := tryExtractExpressRoute(node, opts, qnamePrefix); rn != nil {
				*nodes = append(*nodes, *rn)
				if re != nil {
					*edges = append(*edges, *re)
				}
			}
		}

		// Extract call edge.
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			edge := resolveCallEdge(funcNode, node, opts, qnamePrefix, sourceHash)
			if edge != nil {
				*edges = append(*edges, *edge)
			}
		}

	case "throw_statement":
		// Extract throws edge: throw new ErrorType(...)
		if throwEdge := extractTSThrowEdge(node, opts, qnamePrefix, sourceHash); throwEdge != nil {
			*edges = append(*edges, *throwEdge)
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, qnamePrefix, sourceHash, hasExpress, nodes, edges)
	}
}

// handleTopLevelCallExpression handles call expressions at the top level
// (e.g., route registrations outside functions).
func handleTopLevelCallExpression(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	fileNodeHash types.Hash,
	hasExpress bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if hasExpress {
		if rn, re := tryExtractExpressRoute(node, opts, qnamePrefix); rn != nil {
			*nodes = append(*nodes, *rn)
			if re != nil {
				*edges = append(*edges, *re)
			}
		}
	}

	// Also extract as a call edge.
	funcNode := node.ChildByFieldName("function")
	if funcNode != nil {
		edge := resolveCallEdge(funcNode, node, opts, qnamePrefix, fileNodeHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}
	}
}

// extractTSThrowEdge extracts a throws edge from a throw_statement node.
// It looks for patterns like "throw new ErrorType(...)" and emits a throws
// edge from the enclosing function to the error type.
func extractTSThrowEdge(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash) *types.Edge {
	// A throw_statement typically has a child expression.
	// Look for "new_expression" patterns: throw new TypeError(...)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "new_expression" {
			constructorNode := child.ChildByFieldName("constructor")
			if constructorNode != nil {
				errorType := constructorNode.Content(opts.Content)
				if errorType != "" {
					targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, errorType, "error")
					provenance := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
					return &types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: sourceHash,
						TargetHash: targetHash,
						EdgeType:   "throws",
						Confidence: 0.7,
						Provenance: provenance,
					}
				}
			}
		}
	}
	// Fallback: if no new_expression, use the thrown expression text as the error name.
	// e.g., "throw error" or "throw 'message'"
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			errorName := child.Content(opts.Content)
			targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, errorName, "error")
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
			return &types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: sourceHash,
				TargetHash: targetHash,
				EdgeType:   "throws",
				Confidence: 0.7,
				Provenance: provenance,
			}
		}
	}
	return nil
}

// resolveCallEdge creates an edge for a call expression.
func resolveCallEdge(funcNode, callNode *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash) *types.Edge {
	content := opts.Content
	var targetName string

	switch funcNode.Type() {
	case "identifier":
		targetName = funcNode.Content(content)
	case "member_expression":
		// object.method -> use "object.method" as target name.
		obj := funcNode.ChildByFieldName("object")
		prop := funcNode.ChildByFieldName("property")
		if obj != nil && prop != nil {
			targetName = obj.Content(content) + "." + prop.Content(content)
		}
	default:
		return nil
	}

	if targetName == "" {
		return nil
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, targetName, "function")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   0.7,
		Provenance:   provenance,
		CallSiteLine: int(callNode.StartPoint().Row) + 1,
		CallSiteCol:  int(callNode.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// routeRegistrationMethods is the set of HTTP method names used by Express.js,
// Fastify, Hono, and other frameworks that share the app.method('/path', handler) pattern.
var routeRegistrationMethods = map[string]bool{
	"get":    true,
	"post":   true,
	"put":    true,
	"delete": true,
	"patch":  true,
	"all":    true,
	"use":    true,
	"head":   true,
	"options": true,
}

// tryExtractRoute checks if a call_expression is a framework route registration
// (Express.js, Fastify, Hono, or similar) and returns a route_handler node and
// handles_route edge if so.
func tryExtractExpressRoute(callNode *sitter.Node, opts types.ExtractOptions, qnamePrefix string) (*types.Node, *types.Edge) {
	funcNode := callNode.ChildByFieldName("function")
	if funcNode == nil || funcNode.Type() != "member_expression" {
		return nil, nil
	}

	prop := funcNode.ChildByFieldName("property")
	if prop == nil {
		return nil, nil
	}
	methodName := prop.Content(opts.Content)
	if !routeRegistrationMethods[methodName] {
		return nil, nil
	}

	// Get arguments to extract route pattern.
	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return nil, nil
	}

	routePattern := extractFirstStringArgTS(argsNode, opts.Content)
	if routePattern == "" {
		return nil, nil
	}

	httpMethod := deriveHTTPMethod(methodName)
	routeSig := httpMethod + " " + routePattern

	// Create route_handler node.
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, "route_handler")
	routeNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
		Kind:          "route_handler",
		Signature:     routeSig,
	}

	// Try to get the handler reference from the second argument.
	handlerName := extractSecondArgNameTS(argsNode, opts.Content)
	if handlerName == "" {
		return routeNode, nil
	}

	handlerHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, handlerName, "function")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", provenance)
	routeEdge := &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: nodeHash,
		TargetHash: handlerHash,
		EdgeType:   "handles_route",
		Confidence: 0.7,
		Provenance: provenance,
	}

	return routeNode, routeEdge
}

// deriveHTTPMethod maps an Express method name to an HTTP method string.
func deriveHTTPMethod(methodName string) string {
	switch strings.ToLower(methodName) {
	case "get":
		return "GET"
	case "post":
		return "POST"
	case "put":
		return "PUT"
	case "delete":
		return "DELETE"
	case "patch":
		return "PATCH"
	case "all":
		return "ALL"
	case "use":
		return "USE"
	default:
		return "ANY"
	}
}

// extractFirstStringArgTS extracts the string value from the first string
// argument in an arguments node. Handles both quoted strings.
func extractFirstStringArgTS(argsNode *sitter.Node, content []byte) string {
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		if child.Type() == "string" {
			val := child.Content(content)
			return strings.Trim(val, `"'`)
		}
	}
	return ""
}

// extractSecondArgNameTS returns the identifier name of the second non-punctuation
// argument, if it is a simple identifier.
func extractSecondArgNameTS(argsNode *sitter.Node, content []byte) string {
	argIdx := 0
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		// Skip punctuation.
		t := child.Type()
		if t == "," || t == "(" || t == ")" {
			continue
		}
		argIdx++
		if argIdx == 2 {
			if child.Type() == "identifier" {
				return child.Content(content)
			}
			return ""
		}
	}
	return ""
}

// nestJSHTTPDecorators maps NestJS route decorator names to HTTP methods.
var nestJSHTTPDecorators = map[string]string{
	"Get":     "GET",
	"Post":    "POST",
	"Put":     "PUT",
	"Delete":  "DELETE",
	"Patch":   "PATCH",
	"Head":    "HEAD",
	"Options": "OPTIONS",
	"All":     "ALL",
}

// extractNestJSRoutes finds @Controller('prefix') classes with @Get/@Post method decorators.
func extractNestJSRoutes(root *sitter.Node, opts types.ExtractOptions, qnamePrefix string, nodes *[]types.Node, edges *[]types.Edge) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "class_declaration" {
			continue
		}

		// Check for @Controller decorator with route prefix.
		controllerPrefix := extractNestControllerPrefix(child, opts.Content)
		if controllerPrefix == "" {
			continue
		}

		// Walk class body for decorated methods.
		body := child.ChildByFieldName("body")
		if body == nil {
			continue
		}

		className := ""
		nameNode := child.ChildByFieldName("name")
		if nameNode != nil {
			className = nameNode.Content(opts.Content)
		}

		for j := 0; j < int(body.ChildCount()); j++ {
			method := body.Child(j)
			if method.Type() != "method_definition" {
				continue
			}

			httpMethod, routePath := extractNestMethodDecorator(method, opts.Content)
			if httpMethod == "" {
				continue
			}

			fullRoute := controllerPrefix + routePath
			routeSig := httpMethod + " " + fullRoute

			methodName := ""
			mNameNode := method.ChildByFieldName("name")
			if mNameNode != nil {
				methodName = mNameNode.Content(opts.Content)
			}

			// Create route_handler node.
			nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, "route_handler")
			routeNode := types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
				Kind:          "route_handler",
				Signature:     routeSig,
			}
			*nodes = append(*nodes, routeNode)

			// Create handles_route edge from route to handler method.
			if methodName != "" && className != "" {
				handlerQName := className + "." + methodName
				handlerHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, handlerQName, "method")
				edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", "ast_inferred")
				*edges = append(*edges, types.Edge{
					EdgeHash:   edgeHash,
					SourceHash: nodeHash,
					TargetHash: handlerHash,
					EdgeType:   "handles_route",
					Confidence: 0.7,
					Provenance: "ast_inferred",
				})
			}
		}
	}
}

// extractNestControllerPrefix extracts the route prefix from @Controller('prefix').
func extractNestControllerPrefix(classNode *sitter.Node, content []byte) string {
	// Look for decorators on the class (preceding siblings or decorator field).
	// tree-sitter puts decorators as preceding siblings of the class declaration.
	parent := classNode.Parent()
	if parent == nil {
		return ""
	}

	// Find the decorator that precedes this class node.
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == classNode {
			break
		}
		if child.Type() == "decorator" {
			text := child.Content(content)
			if strings.HasPrefix(text, "@Controller(") {
				// Extract the string argument.
				prefix := extractDecoratorStringArg(text)
				if prefix != "" {
					return prefix
				}
				return "/"
			}
		}
	}
	return ""
}

// extractNestMethodDecorator checks if a method has a NestJS HTTP decorator.
// Returns the HTTP method and sub-route path.
func extractNestMethodDecorator(methodNode *sitter.Node, content []byte) (string, string) {
	parent := methodNode.Parent()
	if parent == nil {
		return "", ""
	}

	// Find decorators preceding this method in the class body.
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == methodNode {
			break
		}
		if child.Type() == "decorator" {
			text := child.Content(content)
			for decorName, httpMethod := range nestJSHTTPDecorators {
				if strings.HasPrefix(text, "@"+decorName+"(") || text == "@"+decorName {
					path := extractDecoratorStringArg(text)
					return httpMethod, path
				}
			}
		}
	}
	return "", ""
}

// extractDecoratorStringArg extracts a string literal from a decorator call.
// e.g., @Get('/users') -> "/users", @Controller('api') -> "/api"
func extractDecoratorStringArg(decoratorText string) string {
	start := strings.Index(decoratorText, "(")
	end := strings.LastIndex(decoratorText, ")")
	if start < 0 || end <= start {
		return ""
	}
	arg := strings.TrimSpace(decoratorText[start+1 : end])
	arg = strings.Trim(arg, `"'`)
	if arg == "" {
		return ""
	}
	if !strings.HasPrefix(arg, "/") {
		arg = "/" + arg
	}
	return arg
}

// nextJSHTTPMethods are function names that Next.js App Router treats as route handlers.
var nextJSHTTPMethods = map[string]string{
	"GET":     "GET",
	"POST":    "POST",
	"PUT":     "PUT",
	"DELETE":  "DELETE",
	"PATCH":   "PATCH",
	"HEAD":    "HEAD",
	"OPTIONS": "OPTIONS",
}

// isNextJSRouteFile returns true if the file path matches Next.js App Router conventions.
func isNextJSRouteFile(filePath string) bool {
	base := filepath.Base(filePath)
	return base == "route.ts" || base == "route.js" || base == "route.tsx" || base == "route.jsx"
}

// extractNextJSRoutes creates route_handler nodes for exported HTTP method functions
// in Next.js App Router route files. The route path is derived from the file's directory.
func extractNextJSRoutes(root *sitter.Node, opts types.ExtractOptions, qnamePrefix string, nodes *[]types.Node, edges *[]types.Edge) {
	// Derive route path from file directory.
	// e.g., "app/api/users/route.ts" -> "/api/users"
	routePath := deriveNextJSRoutePath(opts.FilePath)

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)

		// Look for: export async function GET(...) or export function POST(...)
		if child.Type() == "export_statement" {
			for j := 0; j < int(child.ChildCount()); j++ {
				decl := child.Child(j)
				if decl.Type() == "function_declaration" {
					nameNode := decl.ChildByFieldName("name")
					if nameNode == nil {
						continue
					}
					funcName := nameNode.Content(opts.Content)
					httpMethod, isRoute := nextJSHTTPMethods[funcName]
					if !isRoute {
						continue
					}

					routeSig := httpMethod + " " + routePath
					nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, "route_handler")
					routeNode := types.Node{
						NodeHash:      nodeHash,
						FileHash:      opts.FileHash,
						QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
						Kind:          "route_handler",
						Signature:     routeSig,
					}
					*nodes = append(*nodes, routeNode)

					// Edge from route to the handler function.
					handlerHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, funcName, "function")
					edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", "ast_inferred")
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: nodeHash,
						TargetHash: handlerHash,
						EdgeType:   "handles_route",
						Confidence: 0.7,
						Provenance: "ast_inferred",
					})
				}
			}
		}
	}
}

// deriveNextJSRoutePath extracts the route path from a Next.js file path.
// "app/api/users/route.ts" -> "/api/users"
// "src/app/api/auth/[id]/route.ts" -> "/api/auth/[id]"
func deriveNextJSRoutePath(filePath string) string {
	dir := filepath.Dir(filePath)
	parts := strings.Split(filepath.ToSlash(dir), "/")

	// Find "app" directory and take everything after it.
	appIdx := -1
	for i, p := range parts {
		if p == "app" {
			appIdx = i
			break
		}
	}
	if appIdx < 0 || appIdx >= len(parts)-1 {
		return "/" + filepath.Base(filepath.Dir(filePath))
	}

	routeParts := parts[appIdx+1:]
	return "/" + strings.Join(routeParts, "/")
}

// extractExtendsClause checks a class_declaration for an extends_clause child
// and emits an "extends" edge from the class to the superclass.
func extractExtendsClause(classNode *sitter.Node, opts types.ExtractOptions, qnamePrefix string, classHash types.Hash, edges *[]types.Edge) {
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() == "extends_clause" {
			// The extends clause contains type references as children.
			for j := 0; j < int(child.ChildCount()); j++ {
				typeRef := child.Child(j)
				if typeRef.Type() == "identifier" || typeRef.Type() == "type_identifier" || typeRef.Type() == "member_expression" {
					superName := typeRef.Content(opts.Content)
					targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, superName, "type")
					prov := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(classHash, targetHash, "extends", prov)
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: classHash,
						TargetHash: targetHash,
						EdgeType:   "extends",
						Confidence: 0.7,
						Provenance: prov,
					})
					break // Only one superclass in TS/JS
				}
			}
		}
	}
}

// extractImplementsClause checks a class_declaration for an implements_clause
// child and emits "implements" edges from the class to each interface.
func extractImplementsClause(classNode *sitter.Node, opts types.ExtractOptions, qnamePrefix string, classHash types.Hash, edges *[]types.Edge) {
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() == "implements_clause" {
			for j := 0; j < int(child.ChildCount()); j++ {
				typeRef := child.Child(j)
				if typeRef.Type() == "identifier" || typeRef.Type() == "type_identifier" || typeRef.Type() == "generic_type" {
					ifaceName := typeRef.Content(opts.Content)
					// Strip generic parameters for the target name.
					if idx := strings.Index(ifaceName, "<"); idx > 0 {
						ifaceName = ifaceName[:idx]
					}
					targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, ifaceName, "interface")
					prov := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(classHash, targetHash, "implements", prov)
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: classHash,
						TargetHash: targetHash,
						EdgeType:   "implements",
						Confidence: 0.7,
						Provenance: prov,
					})
				}
			}
		}
	}
}

// extractOverrideEdge checks if a method_definition has an "override" modifier
// and emits an "overrides" edge from the method to the parent class method.
func extractOverrideEdge(methodNode *sitter.Node, opts types.ExtractOptions, qnamePrefix, className string, methodHash types.Hash, edges *[]types.Edge) {
	// In tree-sitter TypeScript, the override keyword appears as a child of
	// the method_definition or inside an accessibility_modifier.
	hasOverride := false
	for i := 0; i < int(methodNode.ChildCount()); i++ {
		child := methodNode.Child(i)
		if child.Type() == "override_modifier" || child.Content(opts.Content) == "override" {
			hasOverride = true
			break
		}
	}
	if !hasOverride {
		return
	}

	nameNode := methodNode.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	methodName := nameNode.Content(opts.Content)

	// The target is the parent class method (best-effort: use className context).
	targetName := "super." + methodName
	targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, targetName, "method")
	prov := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(methodHash, targetHash, "overrides", prov)
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: methodHash,
		TargetHash: targetHash,
		EdgeType:   "overrides",
		Confidence: 0.7,
		Provenance: prov,
	})
}

// extractTSDecoratorEdges looks for decorator nodes that precede a declaration
// and emits "decorates" edges from the decorator to the declaration.
func extractTSDecoratorEdges(declNode *sitter.Node, opts types.ExtractOptions, qnamePrefix string, declHash types.Hash, edges *[]types.Edge) {
	parent := declNode.Parent()
	if parent == nil {
		return
	}
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == declNode {
			break
		}
		if child.Type() == "decorator" {
			decoratorText := child.Content(opts.Content)
			decoratorName := parseTSDecoratorName(decoratorText)
			if decoratorName == "" {
				continue
			}
			targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, decoratorName, "function")
			prov := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(targetHash, declHash, "decorates", prov)
			*edges = append(*edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: targetHash,
				TargetHash: declHash,
				EdgeType:   "decorates",
				Confidence: 0.7,
				Provenance: prov,
			})
		}
	}
}

// parseTSDecoratorName extracts the decorator function name from a decorator
// text like "@Component" or "@Injectable()".
func parseTSDecoratorName(text string) string {
	text = strings.TrimPrefix(text, "@")
	if idx := strings.Index(text, "("); idx > 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return text
}

// deduplicateEdges removes duplicate edges based on EdgeHash.
func deduplicateEdges(edges []types.Edge) []types.Edge {
	if len(edges) <= 1 {
		return edges
	}
	seen := make(map[types.Hash]struct{}, len(edges))
	result := make([]types.Edge, 0, len(edges))
	for _, e := range edges {
		if _, exists := seen[e.EdgeHash]; !exists {
			seen[e.EdgeHash] = struct{}{}
			result = append(result, e)
		}
	}
	return result
}
