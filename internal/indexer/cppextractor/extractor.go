// Package cppextractor provides a tree-sitter based extractor for C and C++
// files. It implements types.Extractor and produces declaration nodes and
// syntactic call/include edges without type resolution.
//
// Supported file extensions: .cpp, .cc, .cxx, .c++, .hpp, .hh, .hxx, .h++, .h, .c
// Excluded: files under build/, cmake-build-*/, .cache/, third_party/,
// third-party/, external/, vendor/, _deps/ directories.
//
// Node types extracted:
//   - function_definition -> "function" (top-level) or "method" (inside class)
//   - class_specifier / struct_specifier -> "type" with nested method/field nodes
//   - namespace_definition -> "type"
//   - enum_specifier -> "type" with "const" enumerator children
//   - type_definition -> "type" (typedef)
//   - using_declaration -> "type"
//   - field_declaration -> "field"
//   - preproc_def -> "const" (macro definitions)
//
// Edge types extracted:
//   - call_expression -> "calls" with call-site positions
//   - base class in class_specifier -> "extends"
//   - class body containment -> "contains" + "member_of"
//   - preproc_include -> "includes"
//
// All edges use provenance "ast_inferred" and confidence 0.7 for cross-file
// calls, and "ast_resolved" with confidence 0.85 for same-file and structural
// edges. The LSP enrichment pass can later upgrade confirmed edges.
package cppextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/cpp"

	"github.com/blackwell-systems/knowing/internal/indexer/docextract"
	"github.com/blackwell-systems/knowing/internal/types"
)

// CppExtractor implements types.Extractor for C and C++ files using
// tree-sitter AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type CppExtractor struct {
	compileDB *CompileDB // loaded once, nil if unavailable
}

// NewCppExtractor creates a new CppExtractor without a compilation database.
func NewCppExtractor() *CppExtractor {
	return &CppExtractor{}
}

// NewCppExtractorWithCompileDB creates a new CppExtractor with a pre-loaded
// compilation database for cross-file include resolution.
func NewCppExtractorWithCompileDB(db *CompileDB) *CppExtractor {
	return &CppExtractor{compileDB: db}
}

// Name returns the extractor name.
func (e *CppExtractor) Name() string {
	return "treesitter-cpp"
}

// excludedDirs is the set of directory names to skip during extraction.
var excludedDirs = map[string]bool{
	"build":          true,
	".cache":         true,
	".cmake":         true,
	"third_party":    true,
	"third-party":    true,
	"external":       true,
	"vendor":         true,
	"_deps":          true,
	"cmake-builds":   true,
	"__pycache__":    true,
	"node_modules":   true,
	".git":           true,
}

// CanHandle returns true for .cpp, .cc, .cxx, .c++, .hpp, .hh, .hxx, .h++,
// .h, .c files that are not in excluded directories.
func (e *CppExtractor) CanHandle(path string) bool {
	if !hasCppExtension(path) {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if excludedDirs[p] {
			return false
		}
		// Match cmake-build-* pattern
		if strings.HasPrefix(p, "cmake-build-") {
			return false
		}
	}
	return true
}

// hasCppExtension checks if the file path has a C/C++ extension.
func hasCppExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".cpp", ".cc", ".cxx", ".c++",
		".hpp", ".hh", ".hxx", ".h++",
		".h", ".c":
		return true
	default:
		return false
	}
}

// Extract parses the C/C++ file with tree-sitter and produces nodes for
// declarations and edges for calls, includes, inheritance, and containment.
func (e *CppExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(cpp.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	basePath := computeBasePath(opts)

	var compileEntry *CompileEntry
	if e.compileDB != nil {
		compileEntry = e.compileDB.Lookup(opts.FilePath)
	}

	// Extract all symbols and edges in one walk.
	nodes, edges := extractSymbols(root, opts, basePath, compileEntry)

	// Build template table from extracted type nodes for instantiation resolution.
	tmplTable := buildTemplateTableFromNodes(nodes)

	// Extract instantiation edges from explicit template_instantiation nodes.
	instantiationEdges := extractExplicitInstantiations(root, opts, basePath, tmplTable)
	edges = append(edges, instantiationEdges...)

	// Walk function bodies for template usage (template_type in declarations).
	usageEdges := extractTemplateUsageEdges(root, opts, basePath, nodes, tmplTable)
	edges = append(edges, usageEdges...)

	// Extract include edges from preprocessor directives.
	includeEdges := extractIncludeEdges(root, opts, types.EmptyHash, basePath, compileEntry)
	edges = append(edges, includeEdges...)

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

// computeBasePath builds the qualified name prefix from repo URL and file path.
// Format: {moduleRoot}/{filePath} (without extension).
func computeBasePath(opts types.ExtractOptions) string {
	dir := filepath.Dir(opts.FilePath)
	if dir == "." {
		dir = ""
	}
	base := strings.TrimSuffix(filepath.Base(opts.FilePath), filepath.Ext(opts.FilePath))
	if dir == "" {
		return base
	}
	return filepath.ToSlash(dir) + "/" + base
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

// makeFuncQName builds a qualified name for a function or method.
func makeFuncQName(opts types.ExtractOptions, basePath, className, funcName string) string {
	if className != "" {
		return fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, basePath, className, funcName)
	}
	return fmt.Sprintf("%s://%s.%s", opts.RepoURL, basePath, funcName)
}

// makeTypeQName builds a qualified name for a type (class, struct, enum, namespace).
func makeTypeQName(opts types.ExtractOptions, basePath, typeName string) string {
	return fmt.Sprintf("%s://%s.%s", opts.RepoURL, basePath, typeName)
}

// makeFieldQName builds a qualified name for a field.
func makeFieldQName(opts types.ExtractOptions, basePath, className, fieldName string) string {
	if className != "" {
		return fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, basePath, className, fieldName)
	}
	return fmt.Sprintf("%s://%s.%s", opts.RepoURL, basePath, fieldName)
}

// makeEnumValueQName builds a qualified name for an enum value.
func makeEnumValueQName(opts types.ExtractOptions, basePath, enumName, valueName string) string {
	return fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, basePath, enumName, valueName)
}

// makeMacroQName builds a qualified name for a macro definition.
func makeMacroQName(opts types.ExtractOptions, basePath, macroName string) string {
	return fmt.Sprintf("%s://%s.%s", opts.RepoURL, basePath, macroName)
}

// extractDoc extracts documentation comments preceding a node.
func extractDoc(node *sitter.Node, content []byte) string {
	return docextract.FromPrecedingComments(node, content, 500)
}

// findChildByType returns the first child of the given type, or nil.
func findChildByType(node *sitter.Node, typeName string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == typeName {
			return child
		}
	}
	return nil
}

// buildTemplateTableFromNodes builds a TemplateTable from extracted nodes.
// Any type node whose signature starts with "template<" is registered as a template.
func buildTemplateTableFromNodes(nodes []types.Node) *TemplateTable {
	tmplTable := NewTemplateTable()
	for _, n := range nodes {
		if n.Kind == types.KindType && strings.HasPrefix(n.Signature, "template<") {
			// Extract the base name from the qualified name.
			name := extractSimpleName(n.QualifiedName)
			if name != "" {
				tmplTable.Register(name, n.NodeHash)
			}
		}
	}
	return tmplTable
}

// extractExplicitInstantiations walks the root looking for template_instantiation
// nodes and creates instantiates edges.
func extractExplicitInstantiations(root *sitter.Node, opts types.ExtractOptions, basePath string, tmplTable *TemplateTable) []types.Edge {
	var edges []types.Edge
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "template_instantiation" {
			e := extractTemplateInstantiationEdges(child, opts, basePath, tmplTable)
			edges = append(edges, e...)
		}
	}
	return edges
}

// extractTemplateUsageEdges walks the AST looking for template_type usage in
// declarations and creates instantiates edges from enclosing functions to templates.
func extractTemplateUsageEdges(root *sitter.Node, opts types.ExtractOptions, basePath string, nodes []types.Node, tmplTable *TemplateTable) []types.Edge {
	var edges []types.Edge

	// Build a map from function/method name to node hash for enclosing scope resolution.
	fnMap := make(map[string]types.Hash)
	for _, n := range nodes {
		if n.Kind == types.KindFunction || n.Kind == types.KindMethod {
			name := extractSimpleName(n.QualifiedName)
			if name != "" {
				fnMap[name] = n.NodeHash
			}
		}
	}

	// Walk the AST for function definitions and check their bodies for template usage.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		edges = walkTopLevelForTemplateUsage(child, opts, basePath, fnMap, tmplTable)
	}

	return edges
}

// walkTopLevelForTemplateUsage handles top-level function definitions and
// class definitions, checking bodies for template_type usage.
func walkTopLevelForTemplateUsage(node *sitter.Node, opts types.ExtractOptions, basePath string, fnMap map[string]types.Hash, tmplTable *TemplateTable) []types.Edge {
	if node == nil {
		return nil
	}

	var edges []types.Edge

	switch node.Type() {
	case "function_definition":
		edges = extractTemplateUsageFromBody(node, opts, basePath, fnMap, tmplTable)
	case "class_specifier", "struct_specifier":
		// Check methods inside class bodies.
		body := node.ChildByFieldName("body")
		if body == nil {
			body = findChildByType(node, "field_declaration_list")
		}
		if body != nil {
			edges = walkClassBodyForTemplateUsage(body, opts, basePath, fnMap, tmplTable)
		}
	case "namespace_definition":
		body := node.ChildByFieldName("body")
		if body == nil {
			body = findChildByType(node, "declaration_list")
		}
		if body != nil {
			for j := 0; j < int(body.ChildCount()); j++ {
				child := body.Child(j)
				edges = append(edges, walkTopLevelForTemplateUsage(child, opts, basePath, fnMap, tmplTable)...)
			}
		}
	case "template_declaration":
		inner := findInnerDeclaration(node)
		if inner != nil {
			edges = walkTopLevelForTemplateUsage(inner, opts, basePath, fnMap, tmplTable)
		}
	case "linkage_specification":
		declList := findChildByType(node, "declaration_list")
		if declList != nil {
			for j := 0; j < int(declList.ChildCount()); j++ {
				child := declList.Child(j)
				edges = append(edges, walkTopLevelForTemplateUsage(child, opts, basePath, fnMap, tmplTable)...)
			}
		}
	}

	return edges
}

// extractTemplateUsageFromBody extracts instantiates edges from a function's body.
func extractTemplateUsageFromBody(funcNode *sitter.Node, opts types.ExtractOptions, basePath string, fnMap map[string]types.Hash, tmplTable *TemplateTable) []types.Edge {
	// Get function name to find its hash.
	decl := funcNode.ChildByFieldName("declarator")
	if decl == nil {
		return nil
	}
	name := getDeclaratorName(decl, opts.Content)
	if name == "" {
		return nil
	}

	fnHash, ok := fnMap[name]
	if !ok {
		return nil
	}

	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	return walkForInstantiations(body, opts, basePath, fnHash, tmplTable)
}

// walkClassBodyForTemplateUsage checks method bodies inside a class for template usage.
func walkClassBodyForTemplateUsage(body *sitter.Node, opts types.ExtractOptions, basePath string, fnMap map[string]types.Hash, tmplTable *TemplateTable) []types.Edge {
	var edges []types.Edge

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		switch child.Type() {
		case "function_definition":
			edges = append(edges, extractTemplateUsageFromBody(child, opts, basePath, fnMap, tmplTable)...)
		case "template_declaration":
			inner := findInnerDeclaration(child)
			if inner != nil && inner.Type() == "function_definition" {
				edges = append(edges, extractTemplateUsageFromBody(inner, opts, basePath, fnMap, tmplTable)...)
			}
		}
	}

	return edges
}
