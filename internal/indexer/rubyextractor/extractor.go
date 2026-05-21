// Package rubyextractor provides a tree-sitter based extractor for Ruby files.
// It implements types.Extractor and produces declaration nodes and syntactic
// call/import edges without type resolution.
//
// Supported file extensions: .rb
// Excluded: files under vendor/ directories
//
// Node types extracted:
//   - class -> "type" nodes with nested "method" nodes
//   - module -> "type" nodes
//   - method (def) -> "function" (top-level) or "method" (inside class/module)
//   - singleton_method (def self.foo) -> "method" nodes
//   - require/require_relative -> "imports" edges
//   - call/method_call -> "calls" edges
//   - class Foo < Bar -> "extends" edge
//   - include/extend ModuleName -> "implements" edges
//   - Rails-style class-level method calls -> "decorates" edges
//   - Rails route patterns in config/routes.rb -> "handles_route" edges
//
// All edges use provenance "ast_inferred" and confidence 0.7, since without
// type resolution cross-module calls are heuristic.
package rubyextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/ruby"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	extractorName = "treesitter-ruby"
	provenance    = "ast_inferred"
	confidence    = 0.7
)

// railsDecorators is the set of class-level method calls that act as decorators
// in Rails and common Ruby DSLs.
var railsDecorators = map[string]bool{
	// Attribute accessors
	"attr_accessor": true,
	"attr_reader":   true,
	"attr_writer":   true,
	// ActiveRecord associations
	"has_many":                   true,
	"has_one":                    true,
	"belongs_to":                 true,
	"has_and_belongs_to_many":    true,
	// ActiveRecord callbacks
	"before_action":              true,
	"after_action":               true,
	"around_action":              true,
	"before_save":                true,
	"after_save":                 true,
	"before_create":              true,
	"after_create":               true,
	"before_update":              true,
	"after_update":               true,
	"before_destroy":             true,
	"after_destroy":              true,
	"before_validation":          true,
	"after_validation":           true,
	"after_commit":               true,
	"after_initialize":           true,
	"after_find":                 true,
	// Validations
	"validates":                  true,
	"validate":                   true,
	// Scopes
	"scope":                      true,
}

// routeMethods maps Rails route DSL method names to HTTP methods.
var routeMethods = map[string]string{
	"get":       "GET",
	"post":      "POST",
	"put":       "PUT",
	"patch":     "PATCH",
	"delete":    "DELETE",
	"resources": "ANY",
	"resource":  "ANY",
	"namespace": "NAMESPACE",
	"root":      "GET",
	"match":     "ANY",
}

// RubyExtractor implements types.Extractor for Ruby files using tree-sitter
// AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type RubyExtractor struct{}

// NewRubyExtractor creates a new RubyExtractor.
func NewRubyExtractor() *RubyExtractor {
	return &RubyExtractor{}
}

// Name returns the extractor name.
func (e *RubyExtractor) Name() string {
	return extractorName
}

// CanHandle returns true for .rb files that are not in vendor/ directories.
func (e *RubyExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".rb") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "vendor" {
			return false
		}
	}
	return true
}

// Extract parses the Ruby file with tree-sitter and produces nodes for
// declarations and edges for calls and imports.
func (e *RubyExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(ruby.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	qnamePrefix := computeQNamePrefix(opts)

	var nodes []types.Node
	var edges []types.Edge

	// File-level node hash for import edges.
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, filepath.Base(opts.FilePath), "file")

	// Detect if this is a routes file.
	isRoutesFile := strings.HasSuffix(opts.FilePath, "config/routes.rb") ||
		strings.HasSuffix(opts.FilePath, "routes.rb")

	// Walk top-level children.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		extractNode(child, opts, qnamePrefix, "", "", fileNodeHash, isRoutesFile, &nodes, &edges)
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

// computeQNamePrefix builds the qualified name prefix from the file path.
// For Ruby, we use the file path without extension as the "package" equivalent.
// e.g., "app/models/user.rb" -> "app/models/user"
func computeQNamePrefix(opts types.ExtractOptions) string {
	return strings.TrimSuffix(filepath.ToSlash(opts.FilePath), filepath.Ext(opts.FilePath))
}

// extractNode dispatches extraction based on tree-sitter node type.
func extractNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	moduleName string,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "class":
		extractClassNode(node, opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)

	case "module":
		extractModuleNode(node, opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)

	case "method":
		extractMethodNode(node, opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)

	case "singleton_method":
		extractSingletonMethodNode(node, opts, qnamePrefix, className, moduleName, nodes, edges)

	case "call":
		handleCallNode(node, opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)

	case "program", "body_statement":
		// Walk children of program or body_statement nodes.
		for i := 0; i < int(node.ChildCount()); i++ {
			extractNode(node.Child(i), opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)
		}
	}
}

// extractClassNode handles class declarations, including superclass extraction
// and walking the class body for methods and decorators.
func extractClassNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	parentClass string,
	moduleName string,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedClass := buildQualifiedName(moduleName, parentClass, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, qualifiedClass, "type")

	*nodes = append(*nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, qualifiedClass),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("class %s", name),
	})

	// Extract superclass ("extends" edge) from the superclass field.
	// The superclass node contains "< ClassName" with the constant as a child.
	superNode := node.ChildByFieldName("superclass")
	if superNode != nil {
		superName := extractConstantFromSuperclass(superNode, opts.Content)
		if superName != "" {
			targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, superName, "type")
			edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "extends", provenance)
			*edges = append(*edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: nodeHash,
				TargetHash: targetHash,
				EdgeType:   "extends",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}

	// Walk class body for methods, includes, extends, and decorator calls.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkClassBody(body, opts, qnamePrefix, qualifiedClass, moduleName, nodeHash, fileNodeHash, isRoutesFile, nodes, edges)
	}
}

// extractModuleNode handles module declarations.
func extractModuleNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	parentClass string,
	moduleName string,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedModule := buildQualifiedName(moduleName, "", name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, qualifiedModule, "type")

	*nodes = append(*nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, qualifiedModule),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("module %s", name),
	})

	// Walk module body.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkClassBody(body, opts, qnamePrefix, "", qualifiedModule, nodeHash, fileNodeHash, isRoutesFile, nodes, edges)
	}
}

// extractMethodNode handles regular method definitions (def foo).
func extractMethodNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	moduleName string,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	// Determine kind based on whether we are inside a class/module.
	kind := "function"
	container := className
	if container == "" {
		container = moduleName
	}
	if container != "" {
		kind = "method"
	}

	var qname string
	if container != "" {
		qname = fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, qnamePrefix, container, name)
	} else {
		qname = fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, name)
	}

	var sigName string
	if container != "" {
		sigName = container + "#" + name
	} else {
		sigName = name
	}

	hashName := name
	if container != "" {
		hashName = container + "." + name
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, hashName, kind)

	*nodes = append(*nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("def %s", sigName),
	})

	// Walk method body for call edges.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkForCalls(body, opts, qnamePrefix, nodeHash, isRoutesFile, nodes, edges)
	}
}

// extractSingletonMethodNode handles singleton method definitions (def self.foo).
func extractSingletonMethodNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	moduleName string,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	container := className
	if container == "" {
		container = moduleName
	}

	var qname string
	if container != "" {
		qname = fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, qnamePrefix, container, name)
	} else {
		qname = fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, name)
	}

	hashName := name
	if container != "" {
		hashName = container + "." + name
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, hashName, "method")

	*nodes = append(*nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "method",
		Line:          line,
		Signature:     fmt.Sprintf("def self.%s", name),
	})

	// Walk body for call edges.
	body := node.ChildByFieldName("body")
	if body != nil {
		walkForCalls(body, opts, qnamePrefix, nodeHash, false, nodes, edges)
	}
}

// handleCallNode handles top-level or class-level call nodes. This covers:
//   - require/require_relative (import edges)
//   - include/extend (implements edges)
//   - Rails decorator calls (decorates edges)
//   - Rails route definitions (handles_route edges)
//   - General call edges
func handleCallNode(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	moduleName string,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	methodNode := node.ChildByFieldName("method")
	if methodNode == nil {
		return
	}
	methodName := methodNode.Content(opts.Content)

	// Determine the class/module hash for decorator and implements edges.
	container := className
	if container == "" {
		container = moduleName
	}
	var containerHash types.Hash
	if container != "" {
		containerHash = types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, container, "type")
	}

	switch methodName {
	case "require", "require_relative":
		extractRequireEdge(node, opts, qnamePrefix, fileNodeHash, edges)
		return

	case "include", "extend":
		if container != "" {
			extractIncludeExtendEdge(node, opts, qnamePrefix, containerHash, edges)
		}
		return
	}

	// Check for Rails decorator calls at class body level.
	if container != "" && railsDecorators[methodName] {
		extractDecoratorEdge(methodName, opts, qnamePrefix, containerHash, edges)
		return
	}

	// Check for Rails route definitions.
	if isRoutesFile {
		if httpMethod, ok := routeMethods[methodName]; ok {
			extractRouteEdge(node, opts, qnamePrefix, methodName, httpMethod, fileNodeHash, nodes, edges)
			return
		}
	}

	// General call edge from file or container.
	sourceHash := fileNodeHash
	if container != "" {
		sourceHash = containerHash
	}
	extractSingleCallEdge(node, opts, qnamePrefix, sourceHash, edges)

	// Walk into block arguments (do...end or { }) for nested calls.
	// This is important for routes files where route calls are inside a
	// Rails.application.routes.draw do...end block.
	block := node.ChildByFieldName("block")
	if block != nil {
		walkBlock(block, opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)
	}
}

// walkBlock walks the body of a do_block or block looking for extractable nodes.
func walkBlock(
	block *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	moduleName string,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if block == nil {
		return
	}
	body := block.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			extractNode(body.Child(i), opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)
		}
	}
}

// walkClassBody walks the body of a class or module, extracting methods,
// includes, decorator calls, and nested classes/modules.
func walkClassBody(
	body *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	className string,
	moduleName string,
	containerHash types.Hash,
	fileNodeHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if body == nil {
		return
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		extractNode(child, opts, qnamePrefix, className, moduleName, fileNodeHash, isRoutesFile, nodes, edges)
	}
}

// extractRequireEdge extracts an import edge from a require or require_relative call.
func extractRequireEdge(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	fileNodeHash types.Hash,
	edges *[]types.Edge,
) {
	arg := extractFirstStringArg(node, opts.Content)
	if arg == "" {
		return
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, arg, types.EmptyHash, arg, "module")
	edgeHash := types.ComputeEdgeHash(fileNodeHash, targetHash, "imports", provenance)
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: fileNodeHash,
		TargetHash: targetHash,
		EdgeType:   "imports",
		Confidence: confidence,
		Provenance: provenance,
	})
}

// extractIncludeExtendEdge extracts an "implements" edge from an include or
// extend call within a class or module body.
func extractIncludeExtendEdge(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	classHash types.Hash,
	edges *[]types.Edge,
) {
	args := node.ChildByFieldName("arguments")
	if args == nil {
		return
	}
	// Walk arguments to find identifiers (module names).
	for i := 0; i < int(args.ChildCount()); i++ {
		arg := args.Child(i)
		if arg.Type() == "constant" || arg.Type() == "scope_resolution" || arg.Type() == "identifier" {
			modName := arg.Content(opts.Content)
			if modName == "" {
				continue
			}
			targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, modName, "type")
			edgeHash := types.ComputeEdgeHash(classHash, targetHash, "implements", provenance)
			*edges = append(*edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: classHash,
				TargetHash: targetHash,
				EdgeType:   "implements",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}
}

// extractDecoratorEdge creates a "decorates" edge for a Rails-style class-level
// method call (e.g., has_many, validates, before_action).
func extractDecoratorEdge(
	decoratorName string,
	opts types.ExtractOptions,
	qnamePrefix string,
	classHash types.Hash,
	edges *[]types.Edge,
) {
	targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, decoratorName, "function")
	edgeHash := types.ComputeEdgeHash(targetHash, classHash, "decorates", provenance)
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: targetHash,
		TargetHash: classHash,
		EdgeType:   "decorates",
		Confidence: confidence,
		Provenance: provenance,
	})
}

// extractRouteEdge extracts a route_handler node and handles_route edge from
// a Rails route definition.
func extractRouteEdge(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	methodName string,
	httpMethod string,
	fileNodeHash types.Hash,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	// For resources/namespace, the argument is typically a symbol.
	// For get/post/etc, the argument is a string path.
	arg := extractFirstArgText(node, opts.Content)
	if arg == "" {
		return
	}

	routeSig := httpMethod + " " + arg
	routeNodeHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, routeSig, "route_handler")

	*nodes = append(*nodes, types.Node{
		NodeHash:      routeNodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, qnamePrefix, routeSig),
		Kind:          "route_handler",
		Signature:     routeSig,
	})

	// Create handles_route edge from route to file.
	edgeHash := types.ComputeEdgeHash(routeNodeHash, fileNodeHash, "handles_route", provenance)
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: routeNodeHash,
		TargetHash: fileNodeHash,
		EdgeType:   "handles_route",
		Confidence: confidence,
		Provenance: provenance,
	})
}

// walkForCalls recursively walks nodes looking for call expressions.
// In Ruby, bare method calls (no receiver, no parens) appear as plain
// identifiers in the AST, so we also emit call edges for identifiers
// found in method bodies.
func walkForCalls(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	sourceHash types.Hash,
	isRoutesFile bool,
	nodes *[]types.Node,
	edges *[]types.Edge,
) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "call":
		extractSingleCallEdge(node, opts, qnamePrefix, sourceHash, edges)
	case "identifier":
		// Bare method calls in Ruby (e.g., perform_task) appear as identifiers.
		// Emit a call edge for them. Skip Ruby keywords and common locals.
		name := node.Content(opts.Content)
		if name != "" && !rubyKeyword(name) {
			targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, name, "function")
			edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)
			*edges = append(*edges, types.Edge{
				EdgeHash:     edgeHash,
				SourceHash:   sourceHash,
				TargetHash:   targetHash,
				EdgeType:     "calls",
				Confidence:   confidence,
				Provenance:   provenance,
				CallSiteLine: int(node.StartPoint().Row) + 1,
				CallSiteCol:  int(node.StartPoint().Column),
				CallSiteFile: opts.FilePath,
			})
		}
		return // identifiers have no children
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, qnamePrefix, sourceHash, isRoutesFile, nodes, edges)
	}
}

// rubyKeyword returns true for Ruby keywords and built-in names that should
// not be treated as method calls when encountered as bare identifiers.
func rubyKeyword(name string) bool {
	switch name {
	case "self", "nil", "true", "false", "end", "do", "if", "else", "elsif",
		"unless", "while", "until", "for", "in", "begin", "rescue", "ensure",
		"raise", "return", "yield", "super", "then", "and", "or", "not":
		return true
	}
	return false
}

// extractSingleCallEdge creates a call edge for a call node.
func extractSingleCallEdge(
	node *sitter.Node,
	opts types.ExtractOptions,
	qnamePrefix string,
	sourceHash types.Hash,
	edges *[]types.Edge,
) {
	methodNode := node.ChildByFieldName("method")
	if methodNode == nil {
		return
	}
	methodName := methodNode.Content(opts.Content)
	if methodName == "" {
		return
	}

	// Check if there is a receiver.
	receiver := node.ChildByFieldName("receiver")
	targetName := methodName
	if receiver != nil {
		receiverName := receiver.Content(opts.Content)
		if receiverName != "" {
			targetName = receiverName + "." + methodName
		}
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, targetName, "function")
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	*edges = append(*edges, types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   confidence,
		Provenance:   provenance,
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	})
}

// extractFirstStringArg extracts the first string literal argument from a call node.
// Handles both 'single' and "double" quoted strings.
func extractFirstStringArg(node *sitter.Node, content []byte) string {
	args := node.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child.Type() == "string" {
			val := child.Content(content)
			return strings.Trim(val, `"'`)
		}
	}
	return ""
}

// extractFirstArgText extracts the text of the first meaningful argument from
// a call node. This handles both string literals and symbol literals (:name).
func extractFirstArgText(node *sitter.Node, content []byte) string {
	args := node.ChildByFieldName("arguments")
	if args == nil {
		return ""
	}
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		switch child.Type() {
		case "string":
			val := child.Content(content)
			return strings.Trim(val, `"'`)
		case "simple_symbol":
			val := child.Content(content)
			return strings.TrimPrefix(val, ":")
		case "symbol":
			val := child.Content(content)
			return strings.TrimPrefix(val, ":")
		}
	}
	return ""
}

// extractConstantFromSuperclass extracts the class name from a superclass node.
// In Ruby's tree-sitter grammar, the superclass field contains "< ClassName"
// with the actual class name as a constant or scope_resolution child node.
func extractConstantFromSuperclass(superNode *sitter.Node, content []byte) string {
	for i := 0; i < int(superNode.ChildCount()); i++ {
		child := superNode.Child(i)
		switch child.Type() {
		case "constant", "scope_resolution":
			return child.Content(content)
		}
	}
	return ""
}

// buildQualifiedName constructs a qualified name from module, class, and symbol
// components, joining non-empty parts with "::".
func buildQualifiedName(moduleName, className, name string) string {
	var parts []string
	if moduleName != "" {
		parts = append(parts, moduleName)
	}
	if className != "" {
		parts = append(parts, className)
	}
	parts = append(parts, name)
	return strings.Join(parts, "::")
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
