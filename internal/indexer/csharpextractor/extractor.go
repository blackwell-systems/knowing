// Package csharpextractor provides a tree-sitter based extractor for C#.
//
// It implements types.Extractor and produces declaration nodes and syntactic
// call/import edges for C# source files. The extractor handles classes,
// structs, interfaces, enums, methods, constructors, using directives,
// invocation expressions, object creation, and ASP.NET route attributes.
//
// All edges have provenance "ast_inferred" and confidence 0.7, consistent
// with other tree-sitter extractors in this codebase.
package csharpextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/csharp"

	"github.com/blackwell-systems/knowing/internal/types"
)

// CSharpExtractor implements types.Extractor for C# files using tree-sitter
// AST parsing.
type CSharpExtractor struct {
	parser *sitter.Parser
}

// NewCSharpExtractor creates a new CSharpExtractor with a tree-sitter parser
// configured for C#.
func NewCSharpExtractor() *CSharpExtractor {
	parser := sitter.NewParser()
	parser.SetLanguage(csharp.GetLanguage())
	return &CSharpExtractor{parser: parser}
}

// Name returns the extractor name.
func (e *CSharpExtractor) Name() string {
	return "treesitter-csharp"
}

// CanHandle returns true for .cs files, excluding files in bin/ and obj/
// directories (.NET build output).
func (e *CSharpExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".cs") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "bin" || p == "obj" {
			return false
		}
	}
	return true
}

// Extract parses the C# file with tree-sitter and produces nodes for
// declarations and edges for calls, imports, and routes.
func (e *CSharpExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	if len(opts.Content) == 0 {
		return &types.ExtractResult{}, nil
	}

	tree, err := e.parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	var nodes []types.Node
	var edges []types.Edge

	// Walk the AST recursively.
	e.walkNode(root, opts, "", &nodes, &edges)

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

// walkNode recursively walks the tree-sitter AST, extracting nodes and edges.
// parentContext is the qualified name prefix from enclosing class/struct/interface.
func (e *CSharpExtractor) walkNode(node *sitter.Node, opts types.ExtractOptions, parentContext string, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "using_directive":
		edge := e.extractUsingDirective(node, opts)
		if edge != nil {
			*edges = append(*edges, *edge)
		}
		return

	case "class_declaration":
		n, classRoutes := e.extractClassDecl(node, opts, parentContext, nodes, edges)
		if n != nil {
			*nodes = append(*nodes, *n)
			// Process route handlers from class-level route attributes.
			for _, rn := range classRoutes {
				*nodes = append(*nodes, rn)
			}
		}
		return

	case "interface_declaration":
		n := e.extractInterfaceDecl(node, opts, parentContext)
		if n != nil {
			*nodes = append(*nodes, *n)
		}
		// Walk children for nested types.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "declaration_list" {
				e.walkChildren(child, opts, e.qualifiedContext(parentContext, n), nodes, edges)
			}
		}
		return

	case "struct_declaration":
		n := e.extractStructDecl(node, opts, parentContext)
		if n != nil {
			*nodes = append(*nodes, *n)
		}
		// Walk body for methods.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "declaration_list" {
				e.walkChildren(child, opts, e.qualifiedContext(parentContext, n), nodes, edges)
			}
		}
		return

	case "enum_declaration":
		n := e.extractEnumDecl(node, opts, parentContext)
		if n != nil {
			*nodes = append(*nodes, *n)
		}
		return

	case "method_declaration":
		n, routeNodes, routeEdges := e.extractMethodDecl(node, opts, parentContext)
		if n != nil {
			*nodes = append(*nodes, *n)
			*nodes = append(*nodes, routeNodes...)
			*edges = append(*edges, routeEdges...)
		}
		// Walk body for call edges.
		body := node.ChildByFieldName("body")
		if body != nil && n != nil {
			e.walkForCalls(body, opts, parentContext, n.NodeHash, edges)
		}
		return

	case "constructor_declaration":
		n := e.extractConstructorDecl(node, opts, parentContext)
		if n != nil {
			*nodes = append(*nodes, *n)
		}
		// Walk body for call edges.
		body := node.ChildByFieldName("body")
		if body != nil && n != nil {
			e.walkForCalls(body, opts, parentContext, n.NodeHash, edges)
		}
		return

	case "invocation_expression":
		// Handled inside walkForCalls from method/constructor bodies.
		return

	case "object_creation_expression":
		// Handled inside walkForCalls from method/constructor bodies.
		return
	}

	// Default: recurse into children.
	for i := 0; i < int(node.ChildCount()); i++ {
		e.walkNode(node.Child(i), opts, parentContext, nodes, edges)
	}
}

// walkChildren walks all children of a node.
func (e *CSharpExtractor) walkChildren(node *sitter.Node, opts types.ExtractOptions, parentContext string, nodes *[]types.Node, edges *[]types.Edge) {
	for i := 0; i < int(node.ChildCount()); i++ {
		e.walkNode(node.Child(i), opts, parentContext, nodes, edges)
	}
}

// qualifiedContext builds a qualified name prefix from a parent context and node.
func (e *CSharpExtractor) qualifiedContext(parentContext string, n *types.Node) string {
	if n == nil {
		return parentContext
	}
	// Extract the simple name from the qualified name (last segment after the last dot).
	qname := n.QualifiedName
	if idx := strings.LastIndex(qname, "."); idx >= 0 {
		simpleName := qname[idx+1:]
		if parentContext != "" {
			return parentContext + "." + simpleName
		}
		return simpleName
	}
	return parentContext
}

// qualifiedName builds a fully qualified name.
func (e *CSharpExtractor) qualifiedName(opts types.ExtractOptions, parentContext, name string) string {
	base := fmt.Sprintf("%s://%s/%s", opts.RepoURL, opts.ModuleRoot, opts.FilePath)
	if parentContext != "" {
		return base + "." + parentContext + "." + name
	}
	return base + "." + name
}

// extractUsingDirective extracts a using directive as an import edge.
func (e *CSharpExtractor) extractUsingDirective(node *sitter.Node, opts types.ExtractOptions) *types.Edge {
	// The using directive has a child that is the namespace name (qualified_name or identifier).
	nameContent := e.extractUsingName(node, opts.Content)
	if nameContent == "" {
		return nil
	}

	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, filepath.Base(opts.FilePath), "file")
	targetHash := types.ComputeNodeHash(opts.RepoURL, nameContent, types.EmptyHash, nameContent, "package")

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(fileNodeHash, targetHash, "imports", provenance)

	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: fileNodeHash,
		TargetHash: targetHash,
		EdgeType:   "imports",
		Confidence: 0.7,
		Provenance: provenance,
	}
}

// extractUsingName extracts the namespace name from a using_directive node.
func (e *CSharpExtractor) extractUsingName(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "qualified_name", "identifier_name", "identifier":
			return child.Content(content)
		}
	}
	// Fallback: grab the text between "using" and ";".
	text := node.Content(content)
	text = strings.TrimPrefix(text, "using ")
	text = strings.TrimPrefix(text, "static ")
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)
	if text != "" && text != "using" {
		return text
	}
	return ""
}

// extractClassDecl extracts a class declaration node and walks its body.
// Returns the class node and any route_handler nodes from class-level attributes.
func (e *CSharpExtractor) extractClassDecl(node *sitter.Node, opts types.ExtractOptions, parentContext string, nodes *[]types.Node, edges *[]types.Edge) (*types.Node, []types.Node) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, parentContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")

	classNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("class %s", name),
	}

	// Emit extends/implements edges from base_list.
	e.extractCSharpBaseList(node, opts, nodeHash, edges)

	// Emit decorates edges for attributes on the class.
	e.extractCSharpAttributeEdges(node, opts, nodeHash, edges)

	// Check for class-level route attributes (e.g., [Route("api/[controller]")]).
	var routeNodes []types.Node
	classRoute := e.extractClassRouteAttribute(node, opts, parentContext, name)
	if classRoute != nil {
		routeNodes = append(routeNodes, *classRoute)
	}

	// Walk the class body for methods, constructors, nested types.
	ctx := parentContext
	if ctx != "" {
		ctx = ctx + "." + name
	} else {
		ctx = name
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "declaration_list" {
			e.walkChildren(child, opts, ctx, nodes, edges)
		}
	}

	return classNode, routeNodes
}

// extractInterfaceDecl extracts an interface declaration node.
func (e *CSharpExtractor) extractInterfaceDecl(node *sitter.Node, opts types.ExtractOptions, parentContext string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, parentContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "interface")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "interface",
		Line:          line,
		Signature:     fmt.Sprintf("interface %s", name),
	}
}

// extractStructDecl extracts a struct declaration node.
func (e *CSharpExtractor) extractStructDecl(node *sitter.Node, opts types.ExtractOptions, parentContext string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, parentContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("struct %s", name),
	}
}

// extractEnumDecl extracts an enum declaration node.
func (e *CSharpExtractor) extractEnumDecl(node *sitter.Node, opts types.ExtractOptions, parentContext string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, parentContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("enum %s", name),
	}
}

// extractMethodDecl extracts a method declaration node and any route attributes.
func (e *CSharpExtractor) extractMethodDecl(node *sitter.Node, opts types.ExtractOptions, parentContext string) (*types.Node, []types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, parentContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "method")

	methodNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "method",
		Line:          line,
		Signature:     fmt.Sprintf("method %s", name),
	}

	var semanticEdges []types.Edge

	// Emit overrides edge if the method has the "override" modifier.
	e.extractCSharpOverrideEdge(node, opts, name, nodeHash, &semanticEdges)

	// Emit decorates edges for attributes on this method.
	e.extractCSharpAttributeEdges(node, opts, nodeHash, &semanticEdges)

	// Check for ASP.NET route attributes on this method.
	routeNodes, routeEdges := e.extractMethodRouteAttributes(node, opts, parentContext, nodeHash)
	routeEdges = append(routeEdges, semanticEdges...)

	return methodNode, routeNodes, routeEdges
}

// extractConstructorDecl extracts a constructor declaration node.
func (e *CSharpExtractor) extractConstructorDecl(node *sitter.Node, opts types.ExtractOptions, parentContext string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := e.qualifiedName(opts, parentContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "method")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "method",
		Line:          line,
		Signature:     fmt.Sprintf("constructor %s", name),
	}
}

// walkForCalls recursively walks a subtree looking for invocation and object
// creation expressions, creating call edges.
func (e *CSharpExtractor) walkForCalls(node *sitter.Node, opts types.ExtractOptions, parentContext string, sourceHash types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "invocation_expression":
		edge := e.extractInvocationEdge(node, opts, parentContext, sourceHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}

	case "object_creation_expression":
		edge := e.extractObjectCreationEdge(node, opts, parentContext, sourceHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}

	case "throw_statement", "throw_expression":
		// TODO: extract throws edge to exception type
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		e.walkForCalls(node.Child(i), opts, parentContext, sourceHash, edges)
	}
}

// extractInvocationEdge creates a call edge for an invocation_expression.
func (e *CSharpExtractor) extractInvocationEdge(node *sitter.Node, opts types.ExtractOptions, parentContext string, sourceHash types.Hash) *types.Edge {
	// The function child is the first child of invocation_expression.
	funcNode := node.ChildByFieldName("function")
	if funcNode == nil {
		// Fallback: first child might be the function expression.
		if node.ChildCount() > 0 {
			funcNode = node.Child(0)
		}
		if funcNode == nil {
			return nil
		}
	}

	var targetName string

	switch funcNode.Type() {
	case "member_access_expression":
		// object.Method() - extract the method name.
		nameNode := funcNode.ChildByFieldName("name")
		if nameNode != nil {
			targetName = nameNode.Content(opts.Content)
		}
	case "identifier_name", "identifier":
		targetName = funcNode.Content(opts.Content)
	case "member_binding_expression":
		// ?.Method() - conditional access.
		nameNode := funcNode.ChildByFieldName("name")
		if nameNode != nil {
			targetName = nameNode.Content(opts.Content)
		}
	default:
		// Try to extract text as-is for simple cases.
		targetName = funcNode.Content(opts.Content)
		// Limit to reasonable identifier-like strings.
		if strings.ContainsAny(targetName, " \t\n(){}[]") {
			return nil
		}
	}

	if targetName == "" {
		return nil
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, targetName, "method")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   0.7,
		Provenance:   provenance,
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// extractObjectCreationEdge creates a call edge for new T() expressions.
func (e *CSharpExtractor) extractObjectCreationEdge(node *sitter.Node, opts types.ExtractOptions, parentContext string, sourceHash types.Hash) *types.Edge {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil
	}

	typeName := typeNode.Content(opts.Content)
	if typeName == "" {
		return nil
	}

	// Object creation calls the constructor.
	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, typeName, "method")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   0.7,
		Provenance:   provenance,
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// extractThrowEdge extracts a throws edge from a throw_statement or
// throw_expression node. It looks for "new ExceptionType(...)" patterns
// within the throw expression.
func (e *CSharpExtractor) extractThrowEdge(node *sitter.Node, opts types.ExtractOptions, parentContext string, sourceHash types.Hash) *types.Edge {
	// Walk children looking for object_creation_expression: throw new ExceptionType(...)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "object_creation_expression" {
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				exceptionType := typeNode.Content(opts.Content)
				if exceptionType != "" {
					targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, exceptionType, "error")
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
		// Also handle: throw identifier; (re-throwing a variable)
		if child.Type() == "identifier" || child.Type() == "identifier_name" {
			name := child.Content(opts.Content)
			if name != "" && name != "throw" {
				targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "error")
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
	return nil
}

// ASP.NET route attribute names that indicate HTTP method handlers.
var aspNetRouteAttributes = map[string]string{
	"HttpGet":    "GET",
	"HttpPost":   "POST",
	"HttpPut":    "PUT",
	"HttpDelete": "DELETE",
	"HttpPatch":  "PATCH",
}

// extractMethodRouteAttributes checks for ASP.NET route attributes on a method.
func (e *CSharpExtractor) extractMethodRouteAttributes(methodNode *sitter.Node, opts types.ExtractOptions, parentContext string, methodHash types.Hash) ([]types.Node, []types.Edge) {
	var routeNodes []types.Node
	var routeEdges []types.Edge

	// Look for attribute_list siblings that precede the method.
	// In tree-sitter C# grammar, attributes are children of the method's parent
	// or siblings. We need to check the method node's children for attribute_list.
	attrs := e.findAttributeLists(methodNode, opts.Content)

	for _, attr := range attrs {
		attrName := attr.name
		httpMethod, isHTTP := aspNetRouteAttributes[attrName]
		if !isHTTP {
			continue
		}

		routePattern := attr.argument
		if routePattern == "" {
			routePattern = ""
		}
		routeStr := httpMethod + " " + routePattern

		qname := e.qualifiedName(opts, parentContext, routeStr)
		routeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, routeStr, "route_handler")

		routeNodes = append(routeNodes, types.Node{
			NodeHash:      routeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qname,
			Kind:          "route_handler",
			Signature:     routeStr,
		})

		provenance := "ast_inferred"
		edgeHash := types.ComputeEdgeHash(routeHash, methodHash, "handles_route", provenance)
		routeEdges = append(routeEdges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: routeHash,
			TargetHash: methodHash,
			EdgeType:   "handles_route",
			Confidence: 0.7,
			Provenance: provenance,
		})
	}

	return routeNodes, routeEdges
}

// extractClassRouteAttribute checks for [Route("...")] or [ApiController] on a class.
func (e *CSharpExtractor) extractClassRouteAttribute(classNode *sitter.Node, opts types.ExtractOptions, parentContext, className string) *types.Node {
	attrs := e.findAttributeLists(classNode, opts.Content)

	for _, attr := range attrs {
		if attr.name == "Route" && attr.argument != "" {
			routeStr := "ROUTE " + attr.argument
			qname := e.qualifiedName(opts, parentContext, routeStr)
			routeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, routeStr, "route_handler")

			return &types.Node{
				NodeHash:      routeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qname,
				Kind:          "route_handler",
				Signature:     routeStr,
			}
		}
	}
	return nil
}

// attributeInfo holds parsed attribute data.
type attributeInfo struct {
	name     string
	argument string
}

// findAttributeLists walks the children of a node to find attribute_list nodes
// and extracts attribute names and arguments.
func (e *CSharpExtractor) findAttributeLists(node *sitter.Node, content []byte) []attributeInfo {
	var attrs []attributeInfo

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "attribute_list" {
			continue
		}
		// Each attribute_list contains attribute nodes.
		for j := 0; j < int(child.ChildCount()); j++ {
			attrNode := child.Child(j)
			if attrNode.Type() != "attribute" {
				continue
			}
			nameNode := attrNode.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			attrName := nameNode.Content(content)

			// Extract the first string argument if present.
			var argument string
			argsNode := attrNode.ChildByFieldName("arguments")
			if argsNode == nil {
				// Try looking for attribute_argument_list as a child.
				for k := 0; k < int(attrNode.ChildCount()); k++ {
					c := attrNode.Child(k)
					if c.Type() == "attribute_argument_list" {
						argsNode = c
						break
					}
				}
			}
			if argsNode != nil {
				argument = e.extractFirstStringArgument(argsNode, content)
			}

			attrs = append(attrs, attributeInfo{name: attrName, argument: argument})
		}
	}

	return attrs
}

// extractFirstStringArgument extracts the first string literal from an
// attribute argument list.
func (e *CSharpExtractor) extractFirstStringArgument(argsNode *sitter.Node, content []byte) string {
	// Walk children recursively looking for string_literal_expression or string_literal.
	return e.findFirstString(argsNode, content)
}

// findFirstString recursively searches for a string literal node.
func (e *CSharpExtractor) findFirstString(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	switch node.Type() {
	case "string_literal_expression", "string_literal", "interpolated_string_expression":
		text := node.Content(content)
		// Strip quotes.
		text = strings.TrimPrefix(text, "\"")
		text = strings.TrimPrefix(text, "@\"")
		text = strings.TrimPrefix(text, "$\"")
		text = strings.TrimSuffix(text, "\"")
		return text
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		if s := e.findFirstString(node.Child(i), content); s != "" {
			return s
		}
	}
	return ""
}

// extractCSharpBaseList checks a class_declaration for a base_list child and
// emits "extends" or "implements" edges. In C#, the first type in the base list
// is the superclass (if it is a class), and the rest are interfaces. Without
// type resolution, we use a heuristic: names starting with "I" followed by an
// uppercase letter are treated as interfaces; everything else is treated as a
// superclass.
func (e *CSharpExtractor) extractCSharpBaseList(classNode *sitter.Node, opts types.ExtractOptions, classHash types.Hash, edges *[]types.Edge) {
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() != "base_list" {
			continue
		}
		first := true
		for j := 0; j < int(child.ChildCount()); j++ {
			baseRef := child.Child(j)
			// Skip punctuation (colon, comma).
			if baseRef.Type() == ":" || baseRef.Type() == "," {
				continue
			}
			baseName := baseRef.Content(opts.Content)
			if baseName == "" {
				continue
			}
			// Strip generic parameters.
			if idx := strings.Index(baseName, "<"); idx > 0 {
				baseName = baseName[:idx]
			}

			// Heuristic: names starting with I + uppercase letter are interfaces.
			isInterface := len(baseName) >= 2 && baseName[0] == 'I' && baseName[1] >= 'A' && baseName[1] <= 'Z'

			if first && !isInterface {
				// First non-interface entry is the superclass.
				targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, baseName, "type")
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
				first = false
				continue
			}

			targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, baseName, "interface")
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
			first = false
		}
	}
}

// extractCSharpOverrideEdge checks if a method has the "override" modifier
// and emits an "overrides" edge.
func (e *CSharpExtractor) extractCSharpOverrideEdge(methodNode *sitter.Node, opts types.ExtractOptions, methodName string, methodHash types.Hash, edges *[]types.Edge) {
	hasOverride := false
	for i := 0; i < int(methodNode.ChildCount()); i++ {
		child := methodNode.Child(i)
		if child.Type() == "modifier" && child.Content(opts.Content) == "override" {
			hasOverride = true
			break
		}
	}
	if !hasOverride {
		return
	}

	targetName := "base." + methodName
	targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, targetName, "method")
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

// extractCSharpAttributeEdges emits "decorates" edges for attribute_list
// entries on a class or method declaration.
func (e *CSharpExtractor) extractCSharpAttributeEdges(node *sitter.Node, opts types.ExtractOptions, declHash types.Hash, edges *[]types.Edge) {
	attrs := e.findAttributeLists(node, opts.Content)
	for _, attr := range attrs {
		attrName := attr.name
		if attrName == "" {
			continue
		}
		// Skip ASP.NET route attributes (handled separately as route_handler edges).
		if _, isRoute := aspNetRouteAttributes[attrName]; isRoute {
			continue
		}
		if attrName == "Route" || attrName == "ApiController" {
			continue
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, attrName, "function")
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
