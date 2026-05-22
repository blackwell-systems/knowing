// Package rustextractor provides a tree-sitter based extractor for Rust files.
// It implements types.Extractor and produces declaration nodes and syntactic
// call/import edges without type resolution.
//
// This extractor handles Rust-specific constructs: function items, struct/enum
// definitions, trait definitions (mapped to "interface" kind), impl blocks with
// methods, use declarations, call expressions (including scoped identifiers),
// macro invocations, and HTTP route detection for Actix-web, Axum, and Rocket
// frameworks.
//
// All edges have provenance "ast_inferred" and confidence 0.7. The LSP
// enrichment pass can later upgrade confirmed edges.
package rustextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	extractorName = "treesitter-rust"
	provenance    = "ast_inferred"
	confidence    = 0.7
)

// RustExtractor implements types.Extractor for Rust files using tree-sitter
// AST parsing. It is AST-only with no type resolution.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type RustExtractor struct{}

// NewRustExtractor creates a new RustExtractor.
func NewRustExtractor() *RustExtractor {
	return &RustExtractor{}
}

// Name returns the extractor name.
func (e *RustExtractor) Name() string {
	return extractorName
}

// CanHandle returns true for .rs files that are not in the target/ directory.
func (e *RustExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".rs") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "target" {
			return false
		}
	}
	return true
}

// Extract parses the Rust file with tree-sitter and produces nodes for
// declarations and edges for calls, imports, and routes.
func (e *RustExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	// Compute a base path for qualified names.
	basePath := computeBasePath(opts)

	// Build Rust import map: maps imported type/function names to their source module path.
	// e.g., "use crate::core::resolver::FeatureResolver" -> rustImports["FeatureResolver"] = "src/cargo/core/resolver"
	rustImports := buildRustImportMap(root, opts)

	var nodes []types.Node
	var edges []types.Edge

	// Walk top-level children of the root node.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		n, ed := extractTopLevelWithImports(child, opts, basePath, rustImports)
		nodes = append(nodes, n...)
		edges = append(edges, ed...)
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

// computeBasePath determines the base path for qualified names by combining
// the module root directory name with the file path.
func computeBasePath(opts types.ExtractOptions) string {
	dir := filepath.Dir(opts.FilePath)
	if dir == "." {
		dir = ""
	}
	moduleName := filepath.Base(opts.ModuleRoot)
	if moduleName == "." || moduleName == "" {
		moduleName = "crate"
	}
	if dir == "" {
		return moduleName
	}
	return moduleName + "/" + filepath.ToSlash(dir)
}

// extractTopLevelWithImports is extractTopLevel with import resolution for call edges.
func extractTopLevelWithImports(node *sitter.Node, opts types.ExtractOptions, basePath string, rustImports map[string]string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	switch node.Type() {
	case "function_item":
		n, ed := extractFunctionItemWithImports(node, opts, basePath, "", rustImports)
		nodes = append(nodes, n...)
		edges = append(edges, ed...)

	case "impl_item":
		n, ed := extractImplItemWithImports(node, opts, basePath, rustImports)
		nodes = append(nodes, n...)
		edges = append(edges, ed...)

	case "struct_item":
		n := extractStructItem(node, opts, basePath)
		if n != nil {
			nodes = append(nodes, *n)
			deriveEdges := extractDeriveAttributes(node, opts, basePath, n.NodeHash)
			edges = append(edges, deriveEdges...)
		}

	case "enum_item":
		n := extractEnumItem(node, opts, basePath)
		if n != nil {
			nodes = append(nodes, *n)
			deriveEdges := extractDeriveAttributes(node, opts, basePath, n.NodeHash)
			edges = append(edges, deriveEdges...)
		}

	case "trait_item":
		n := extractTraitItem(node, opts, basePath)
		if n != nil {
			nodes = append(nodes, *n)
		}

	case "use_declaration":
		ed := extractUseDeclaration(node, opts, basePath)
		edges = append(edges, ed...)

	case "macro_invocation":
		ed := extractMacroInvocation(node, opts, basePath)
		edges = append(edges, ed...)
	}

	return nodes, edges
}

// extractTopLevel dispatches a top-level node to the appropriate handler.
func extractTopLevel(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	switch node.Type() {
	case "function_item":
		n, ed := extractFunctionItem(node, opts, basePath, "")
		nodes = append(nodes, n...)
		edges = append(edges, ed...)

	case "impl_item":
		n, ed := extractImplItem(node, opts, basePath)
		nodes = append(nodes, n...)
		edges = append(edges, ed...)

	case "struct_item":
		n := extractStructItem(node, opts, basePath)
		if n != nil {
			nodes = append(nodes, *n)
			// Emit decorates edges for derive macros and other attributes.
			deriveEdges := extractDeriveAttributes(node, opts, basePath, n.NodeHash)
			edges = append(edges, deriveEdges...)
		}

	case "enum_item":
		n := extractEnumItem(node, opts, basePath)
		if n != nil {
			nodes = append(nodes, *n)
			// Emit decorates edges for derive macros and other attributes.
			deriveEdges := extractDeriveAttributes(node, opts, basePath, n.NodeHash)
			edges = append(edges, deriveEdges...)
		}

	case "trait_item":
		n := extractTraitItem(node, opts, basePath)
		if n != nil {
			nodes = append(nodes, *n)
		}

	case "use_declaration":
		ed := extractUseDeclaration(node, opts, basePath)
		edges = append(edges, ed...)

	case "macro_invocation":
		ed := extractMacroInvocation(node, opts, basePath)
		edges = append(edges, ed...)
	}

	return nodes, edges
}

// extractFunctionItem extracts a function_item node. If implType is non-empty,
// the function is treated as a method within an impl block.
func extractFunctionItem(node *sitter.Node, opts types.ExtractOptions, basePath, implType string) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	kind := "function"
	var qname string
	if implType != "" {
		kind = "method"
		qname = fmt.Sprintf("%s://%s/%s.%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, implType, name)
	} else {
		qname = fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, name)
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, kind)
	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("fn %s()", name),
	}

	var nodes []types.Node
	nodes = append(nodes, n)

	// Check for route attributes on this function.
	routeNodes, routeEdges := extractRouteAttributes(node, opts, basePath, nodeHash)
	nodes = append(nodes, routeNodes...)

	// Extract call edges from the function body.
	body := node.ChildByFieldName("body")
	var edges []types.Edge
	edges = append(edges, routeEdges...)
	callEdges := extractCallEdgesFromBody(body, opts, basePath, nodeHash)
	edges = append(edges, callEdges...)

	return nodes, edges
}

// extractImplItem extracts an impl_item node, producing method nodes for
// each function_item in the impl body. If the impl block is a trait impl
// (impl Trait for Type), it also emits an "implements" edge.
func extractImplItem(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	// Get the impl target type.
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil, nil
	}
	implType := typeNode.Content(opts.Content)

	var nodes []types.Node
	var edges []types.Edge

	// Check for trait name: "impl Trait for Type" has a "trait" field.
	traitNode := node.ChildByFieldName("trait")
	if traitNode != nil {
		traitName := traitNode.Content(opts.Content)
		if traitName != "" {
			typeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, implType, "type")
			traitHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, traitName, "interface")
			edgeHash := types.ComputeEdgeHash(typeHash, traitHash, "implements", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: typeHash,
				TargetHash: traitHash,
				EdgeType:   "implements",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}

	// Walk the body to find function_item children.
	body := node.ChildByFieldName("body")
	if body == nil {
		return nodes, edges
	}

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() == "function_item" {
			n, ed := extractFunctionItem(child, opts, basePath, implType)
			nodes = append(nodes, n...)
			edges = append(edges, ed...)
		}
	}

	return nodes, edges
}

// extractStructItem extracts a struct_item node.
func extractStructItem(node *sitter.Node, opts types.ExtractOptions, basePath string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, "type")
	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, name),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("struct %s", name),
	}
}

// extractEnumItem extracts an enum_item node.
func extractEnumItem(node *sitter.Node, opts types.ExtractOptions, basePath string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, "type")
	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, name),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("enum %s", name),
	}
}

// extractTraitItem extracts a trait_item node. Traits map to "interface" kind
// in knowing's model.
func extractTraitItem(node *sitter.Node, opts types.ExtractOptions, basePath string) *types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, "interface")
	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, name),
		Kind:          "interface",
		Line:          line,
		Signature:     fmt.Sprintf("trait %s", name),
	}
}

// extractUseDeclaration extracts a use_declaration node, creating import edges.
func extractUseDeclaration(node *sitter.Node, opts types.ExtractOptions, basePath string) []types.Edge {
	argNode := node.ChildByFieldName("argument")
	if argNode == nil {
		return nil
	}

	usePath := argNode.Content(opts.Content)
	// Parse out the crate/module name (first segment before ::).
	crateName := usePath
	if idx := strings.Index(usePath, "::"); idx >= 0 {
		crateName = usePath[:idx]
	}

	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, filepath.Base(opts.FilePath), "file")
	targetHash := types.ComputeNodeHash(opts.RepoURL, crateName, types.EmptyHash, crateName, "package")

	edgeHash := types.ComputeEdgeHash(fileNodeHash, targetHash, "imports", provenance)
	return []types.Edge{
		{
			EdgeHash:   edgeHash,
			SourceHash: fileNodeHash,
			TargetHash: targetHash,
			EdgeType:   "imports",
			Confidence: confidence,
			Provenance: provenance,
		},
	}
}

// extractMacroInvocation extracts a macro_invocation node, creating a call edge.
func extractMacroInvocation(node *sitter.Node, opts types.ExtractOptions, basePath string) []types.Edge {
	macroNode := node.ChildByFieldName("macro")
	if macroNode == nil {
		return nil
	}
	macroName := macroNode.Content(opts.Content)

	// Source is a synthetic file node.
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, filepath.Base(opts.FilePath), "file")
	targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, macroName, "function")

	edgeHash := types.ComputeEdgeHash(fileNodeHash, targetHash, "calls", provenance)
	return []types.Edge{
		{
			EdgeHash:     edgeHash,
			SourceHash:   fileNodeHash,
			TargetHash:   targetHash,
			EdgeType:     "calls",
			Confidence:   confidence,
			Provenance:   provenance,
			CallSiteLine: int(node.StartPoint().Row) + 1,
			CallSiteCol:  int(node.StartPoint().Column),
			CallSiteFile: opts.FilePath,
		},
	}
}

// extractCallEdgesFromBody walks a function body for call_expression nodes
// and creates call edges.
func extractCallEdgesFromBody(body *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash) []types.Edge {
	if body == nil {
		return nil
	}
	var edges []types.Edge
	walkForCalls(body, opts, basePath, sourceHash, &edges)
	return edges
}

// walkForCalls recursively walks nodes looking for call_expression and
// macro_invocation nodes.
func walkForCalls(node *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "call_expression":
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			edge := resolveCallEdge(funcNode, opts, basePath, sourceHash)
			if edge != nil {
				edge.CallSiteLine = int(node.StartPoint().Row) + 1
				edge.CallSiteCol = int(node.StartPoint().Column)
				edge.CallSiteFile = opts.FilePath
				*edges = append(*edges, *edge)
			}
		}
	case "macro_invocation":
		macroNode := node.ChildByFieldName("macro")
		if macroNode != nil {
			macroName := macroNode.Content(opts.Content)
			targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, macroName, "function")
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
			// Emit throws edge for panic! and bail! macros.
			if macroName == "panic" || macroName == "bail" {
				errorName := macroName + "!"
				errTargetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, errorName, "error")
				errEdgeHash := types.ComputeEdgeHash(sourceHash, errTargetHash, "throws", provenance)
				*edges = append(*edges, types.Edge{
					EdgeHash:   errEdgeHash,
					SourceHash: sourceHash,
					TargetHash: errTargetHash,
					EdgeType:   "throws",
					Confidence: confidence,
					Provenance: provenance,
				})
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, basePath, sourceHash, edges)
	}
}

// resolveCallEdge creates an edge for a call expression's function node.
func resolveCallEdge(funcNode *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash) *types.Edge {
	return resolveCallEdgeWithImports(funcNode, opts, basePath, sourceHash, nil)
}

// resolveCallEdgeWithImports resolves a call expression to a call edge, using
// the import map to resolve cross-module targets when available.
func resolveCallEdgeWithImports(funcNode *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, rustImports map[string]string) *types.Edge {
	content := opts.Content
	var targetName string
	var firstName string

	switch funcNode.Type() {
	case "identifier":
		targetName = funcNode.Content(content)
		firstName = targetName
	case "scoped_identifier":
		// e.g., "FeatureResolver::new" -> firstName = "FeatureResolver"
		targetName = funcNode.Content(content)
		if idx := strings.Index(targetName, "::"); idx >= 0 {
			firstName = targetName[:idx]
		} else {
			firstName = targetName
		}
	case "field_expression":
		targetName = funcNode.Content(content)
		if idx := strings.Index(targetName, "."); idx >= 0 {
			firstName = targetName[:idx]
		} else {
			firstName = targetName
		}
	default:
		return nil
	}

	if targetName == "" {
		return nil
	}

	// Resolve through import map: if the first segment was imported from another
	// module, compute the target hash against that module's path.
	targetBasePath := basePath
	edgeProvenance := provenance
	edgeConfidence := confidence
	if rustImports != nil && firstName != "" {
		if srcPath, ok := rustImports[firstName]; ok {
			targetBasePath = srcPath
			edgeProvenance = "ast_resolved"
			edgeConfidence = 0.85
		}
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, targetBasePath, types.EmptyHash, targetName, "function")
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", edgeProvenance)

	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "calls",
		Confidence: edgeConfidence,
		Provenance: edgeProvenance,
	}
}

// extractFunctionItemWithImports is extractFunctionItem with import-resolved call edges.
func extractFunctionItemWithImports(node *sitter.Node, opts types.ExtractOptions, basePath, implType string, rustImports map[string]string) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	kind := "function"
	var qname string
	if implType != "" {
		kind = "method"
		qname = fmt.Sprintf("%s://%s/%s.%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, implType, name)
	} else {
		qname = fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, name)
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, kind)
	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("fn %s()", name),
	}

	var nodes []types.Node
	nodes = append(nodes, n)

	routeNodes, routeEdges := extractRouteAttributes(node, opts, basePath, nodeHash)
	nodes = append(nodes, routeNodes...)

	body := node.ChildByFieldName("body")
	var edges []types.Edge
	edges = append(edges, routeEdges...)
	callEdges := extractCallEdgesFromBodyWithImports(body, opts, basePath, nodeHash, rustImports)
	edges = append(edges, callEdges...)

	return nodes, edges
}

// extractImplItemWithImports is extractImplItem with import-resolved call edges.
func extractImplItemWithImports(node *sitter.Node, opts types.ExtractOptions, basePath string, rustImports map[string]string) ([]types.Node, []types.Edge) {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil, nil
	}
	implType := typeNode.Content(opts.Content)

	var nodes []types.Node
	var edges []types.Edge

	traitNode := node.ChildByFieldName("trait")
	if traitNode != nil {
		traitName := traitNode.Content(opts.Content)
		if traitName != "" {
			typeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, implType, "type")
			traitHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, traitName, "interface")
			edgeHash := types.ComputeEdgeHash(typeHash, traitHash, "implements", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: typeHash,
				TargetHash: traitHash,
				EdgeType:   "implements",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}

	body := node.ChildByFieldName("body")
	if body == nil {
		return nodes, edges
	}

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() == "function_item" {
			n, ed := extractFunctionItemWithImports(child, opts, basePath, implType, rustImports)
			nodes = append(nodes, n...)
			edges = append(edges, ed...)
		}
	}

	return nodes, edges
}

// extractCallEdgesFromBodyWithImports walks a function body for call edges with import resolution.
func extractCallEdgesFromBodyWithImports(body *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, rustImports map[string]string) []types.Edge {
	if body == nil {
		return nil
	}
	var edges []types.Edge
	walkForCallsWithImports(body, opts, basePath, sourceHash, rustImports, &edges)
	return edges
}

// walkForCallsWithImports recursively walks nodes looking for call_expression nodes
// with import resolution.
func walkForCallsWithImports(node *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, rustImports map[string]string, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "call_expression":
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			edge := resolveCallEdgeWithImports(funcNode, opts, basePath, sourceHash, rustImports)
			if edge != nil {
				edge.CallSiteLine = int(node.StartPoint().Row) + 1
				edge.CallSiteCol = int(node.StartPoint().Column)
				edge.CallSiteFile = opts.FilePath
				*edges = append(*edges, *edge)
			}
		}
	case "macro_invocation":
		macroNode := node.ChildByFieldName("macro")
		if macroNode != nil {
			macroName := macroNode.Content(opts.Content)
			targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, macroName, "function")
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
			if macroName == "panic" || macroName == "bail" {
				errorName := macroName + "!"
				errTargetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, errorName, "error")
				errEdgeHash := types.ComputeEdgeHash(sourceHash, errTargetHash, "throws", provenance)
				*edges = append(*edges, types.Edge{
					EdgeHash:   errEdgeHash,
					SourceHash: sourceHash,
					TargetHash: errTargetHash,
					EdgeType:   "throws",
					Confidence: confidence,
					Provenance: provenance,
				})
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCallsWithImports(node.Child(i), opts, basePath, sourceHash, rustImports, edges)
	}
}

// buildRustImportMap extracts all use declarations from a Rust AST and builds
// a map from imported type/function names to their source module path.
// Handles: use crate::module::Type, use crate::module::{Type1, Type2},
// use super::module::func_name, use module::Type as Alias
func buildRustImportMap(root *sitter.Node, opts types.ExtractOptions) map[string]string {
	imports := make(map[string]string)

	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil || node.Type() != "use_declaration" {
			continue
		}

		argNode := node.ChildByFieldName("argument")
		if argNode == nil {
			continue
		}

		usePath := argNode.Content(opts.Content)
		parseRustUsePath(usePath, opts, imports)
	}

	return imports
}

// parseRustUsePath parses a Rust use path and populates the import map.
// "crate::core::resolver::FeatureResolver" -> imports["FeatureResolver"] = "src/cargo/core/resolver"
// "crate::core::{Config, Workspace}" -> imports["Config"] = "src/cargo/core", imports["Workspace"] = "src/cargo/core"
// "super::helpers::resolve" -> imports["resolve"] = resolved parent module path
func parseRustUsePath(usePath string, opts types.ExtractOptions, imports map[string]string) {
	// Handle glob imports: "use crate::module::*" (skip, can't resolve individual names)
	if strings.HasSuffix(usePath, "::*") {
		return
	}

	// Handle group imports: "use crate::module::{A, B, C}"
	if idx := strings.Index(usePath, "::{"); idx >= 0 {
		prefix := usePath[:idx]
		modulePath := resolveRustModulePath(prefix, opts)
		// Extract names from the braces.
		braceContent := usePath[idx+3:]
		braceContent = strings.TrimSuffix(braceContent, "}")
		for _, name := range strings.Split(braceContent, ",") {
			name = strings.TrimSpace(name)
			// Handle "Type as Alias"
			if asIdx := strings.Index(name, " as "); asIdx >= 0 {
				alias := strings.TrimSpace(name[asIdx+4:])
				imports[alias] = modulePath
			} else if name != "" {
				// Handle nested path: "self::SubModule" (skip)
				if strings.Contains(name, "::") {
					continue
				}
				imports[name] = modulePath
			}
		}
		return
	}

	// Simple path: "crate::module::Type"
	lastSep := strings.LastIndex(usePath, "::")
	if lastSep < 0 {
		return
	}
	prefix := usePath[:lastSep]
	name := usePath[lastSep+2:]

	// Handle "Type as Alias"
	if asIdx := strings.Index(name, " as "); asIdx >= 0 {
		alias := strings.TrimSpace(name[asIdx+4:])
		imports[alias] = resolveRustModulePath(prefix, opts)
	} else {
		imports[name] = resolveRustModulePath(prefix, opts)
	}
}

// resolveRustModulePath converts a Rust module path prefix to a file-system relative path.
// "crate::core::resolver" -> "src/cargo/core/resolver" (relative to ModuleRoot)
// "super::helpers" -> parent directory + "helpers"
// "std::collections" -> "" (external, can't resolve)
func resolveRustModulePath(prefix string, opts types.ExtractOptions) string {
	parts := strings.Split(prefix, "::")

	if len(parts) == 0 {
		return ""
	}

	switch parts[0] {
	case "crate":
		// crate:: refers to the current crate root (typically src/).
		// "crate::core::resolver" -> "src/core/resolver" or "src/cargo/core/resolver"
		moduleParts := parts[1:]
		if len(moduleParts) == 0 {
			return ""
		}
		// Try with "src/" prefix (standard Rust layout).
		candidate := "src/" + strings.Join(moduleParts, "/")
		return filepath.ToSlash(candidate)

	case "super":
		// super:: refers to parent module.
		dir := filepath.Dir(opts.FilePath)
		for _, p := range parts[1:] {
			if p == "super" {
				dir = filepath.Dir(dir)
			} else {
				dir = filepath.Join(dir, p)
			}
		}
		return filepath.ToSlash(dir)

	case "self":
		// self:: refers to current module.
		dir := filepath.Dir(opts.FilePath)
		for _, p := range parts[1:] {
			dir = filepath.Join(dir, p)
		}
		return filepath.ToSlash(dir)

	default:
		// External crate (std, tokio, etc.) - can't resolve.
		return ""
	}
}

// routeAttrPatterns maps attribute names to HTTP methods for route detection
// across Actix-web, Axum, and Rocket frameworks.
var routeAttrPatterns = map[string]string{
	"get":    "GET",
	"post":   "POST",
	"put":    "PUT",
	"delete": "DELETE",
	"patch":  "PATCH",
	"head":   "HEAD",
}

// extractRouteAttributes checks a function_item for route attribute macros
// (e.g., #[get("/path")]) used by Actix-web and Rocket.
func extractRouteAttributes(funcNode *sitter.Node, opts types.ExtractOptions, basePath string, handlerHash types.Hash) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	// Look for attribute_item siblings before this function.
	// In tree-sitter Rust grammar, attributes are children of the parent,
	// preceding the function_item. But they can also be direct children
	// of the function_item node in some cases.
	// We need to check the parent's children that precede this node.
	parent := funcNode.Parent()
	if parent == nil {
		return nodes, edges
	}

	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == funcNode {
			break
		}
		if child.Type() == "attribute_item" {
			rn, re := tryExtractRouteAttribute(child, opts, basePath, handlerHash)
			nodes = append(nodes, rn...)
			edges = append(edges, re...)
		}
	}

	// Also check direct children of the function_item (some tree-sitter
	// versions nest attributes inside the function node).
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		child := funcNode.Child(i)
		if child.Type() == "attribute_item" {
			rn, re := tryExtractRouteAttribute(child, opts, basePath, handlerHash)
			nodes = append(nodes, rn...)
			edges = append(edges, re...)
		}
	}

	return nodes, edges
}

// tryExtractRouteAttribute checks if an attribute_item is a route attribute
// and extracts route_handler nodes and handles_route edges.
func tryExtractRouteAttribute(attrNode *sitter.Node, opts types.ExtractOptions, basePath string, handlerHash types.Hash) ([]types.Node, []types.Edge) {
	content := opts.Content
	attrText := attrNode.Content(content)

	// Match patterns like #[get("/path")] or #[post("/path")]
	for attrName, httpMethod := range routeAttrPatterns {
		prefix := "#[" + attrName + "(\""
		if !strings.HasPrefix(attrText, prefix) {
			continue
		}
		// Extract the route path.
		rest := attrText[len(prefix):]
		endIdx := strings.Index(rest, "\"")
		if endIdx < 0 {
			continue
		}
		routePath := rest[:endIdx]
		routePattern := httpMethod + " " + routePath

		// Create a route_handler node.
		qname := fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, routePattern)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, routePattern, "route_handler")

		var nodes []types.Node
		var edges []types.Edge

		nodes = append(nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qname,
			Kind:          "route_handler",
			Signature:     routePattern,
		})

		// Create a handles_route edge from the route node to the handler function.
		if handlerHash != types.EmptyHash {
			edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: nodeHash,
				TargetHash: handlerHash,
				EdgeType:   "handles_route",
				Confidence: confidence,
				Provenance: provenance,
			})
		}

		return nodes, edges
	}

	return nil, nil
}

// extractAxumRoutes detects Axum Router::new().route("/path", get(handler))
// patterns in the AST. This is called during body walking for call expressions
// matching .route() patterns.
func extractAxumRoute(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	content := opts.Content
	nodeText := node.Content(content)

	// Look for .route( calls with two arguments: path and handler.
	if !strings.Contains(nodeText, ".route(") {
		return nil, nil
	}

	argsNode := node.ChildByFieldName("arguments")
	if argsNode == nil {
		return nil, nil
	}

	// Extract the route path from the first string argument.
	routePath := extractFirstRustStringArg(argsNode, content)
	if routePath == "" {
		return nil, nil
	}

	// The second argument typically is get(handler), post(handler), etc.
	httpMethod, handlerName := extractAxumMethodAndHandler(argsNode, content)
	if httpMethod == "" {
		httpMethod = "ANY"
	}

	routePattern := httpMethod + " " + routePath
	qname := fmt.Sprintf("%s://%s/%s.%s", opts.RepoURL, opts.ModuleRoot, basePath, routePattern)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, routePattern, "route_handler")

	var nodes []types.Node
	var edges []types.Edge

	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "route_handler",
		Signature:     routePattern,
	})

	if handlerName != "" {
		handlerHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, handlerName, "function")
		edgeHash := types.ComputeEdgeHash(nodeHash, handlerHash, "handles_route", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: handlerHash,
			EdgeType:   "handles_route",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return nodes, edges
}

// extractFirstRustStringArg extracts the first string literal from an arguments node.
func extractFirstRustStringArg(argsNode *sitter.Node, content []byte) string {
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		if child.Type() == "string_literal" {
			val := child.Content(content)
			return strings.Trim(val, `"`)
		}
	}
	return ""
}

// extractAxumMethodAndHandler extracts the HTTP method and handler name from
// the second argument of an Axum .route() call. The second arg is typically
// get(handler_fn) or post(handler_fn).
func extractAxumMethodAndHandler(argsNode *sitter.Node, content []byte) (string, string) {
	argIdx := 0
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		if child.Type() == "," || child.Type() == "(" || child.Type() == ")" {
			continue
		}
		argIdx++
		if argIdx == 2 {
			// Expect a call_expression like get(handler).
			if child.Type() == "call_expression" {
				funcNode := child.ChildByFieldName("function")
				if funcNode != nil {
					methodName := funcNode.Content(content)
					httpMethod := strings.ToUpper(methodName)
					if _, ok := routeAttrPatterns[methodName]; ok {
						// Extract the handler name from the arguments.
						innerArgs := child.ChildByFieldName("arguments")
						if innerArgs != nil {
							for j := 0; j < int(innerArgs.ChildCount()); j++ {
								arg := innerArgs.Child(j)
								if arg.Type() == "identifier" {
									return httpMethod, arg.Content(content)
								}
							}
						}
						return httpMethod, ""
					}
				}
			}
			return "", ""
		}
	}
	return "", ""
}

// extractDeriveAttributes checks for #[derive(Trait1, Trait2)] attribute items
// on a struct or enum and emits "decorates" edges for each derived trait.
func extractDeriveAttributes(node *sitter.Node, opts types.ExtractOptions, basePath string, declHash types.Hash) []types.Edge {
	var edges []types.Edge
	parent := node.Parent()
	if parent == nil {
		return edges
	}

	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(i)
		if child == node {
			break
		}
		if child.Type() != "attribute_item" {
			continue
		}
		attrText := child.Content(opts.Content)
		// Match #[derive(Trait1, Trait2, ...)]
		if !strings.HasPrefix(attrText, "#[derive(") {
			continue
		}
		inner := attrText[len("#[derive("):]
		endIdx := strings.Index(inner, ")")
		if endIdx < 0 {
			continue
		}
		inner = inner[:endIdx]
		traits := strings.Split(inner, ",")
		for _, t := range traits {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, t, "function")
			edgeHash := types.ComputeEdgeHash(targetHash, declHash, "decorates", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: targetHash,
				TargetHash: declHash,
				EdgeType:   "decorates",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}
	return edges
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
