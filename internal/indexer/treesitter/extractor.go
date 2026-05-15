// Package treesitter provides a tree-sitter based extractor for non-Go languages.
// Currently supports Python.
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
func (e *TreeSitterExtractor) walkNode(node *sitter.Node, opts types.ExtractOptions, classContext string, result *types.ExtractResult) {
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
