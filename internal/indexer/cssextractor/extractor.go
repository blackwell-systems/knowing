// Package cssextractor provides a tree-sitter based extractor for CSS and SCSS
// files. It implements types.Extractor and produces nodes for class selectors,
// ID selectors, and custom properties, as well as edges for @import rules and
// var() references.
//
// Supported file extensions: .css, .scss
// Excluded: files under node_modules/ directories
//
// Node types extracted:
//   - class_selector (.classname)
//   - id_selector (#idname)
//   - custom_property (--varname definitions)
//
// Edge types extracted:
//   - imports (@import rules)
//   - depends_on (var(--name) references to custom properties)
//
// All edges use provenance "ast_inferred" and confidence 0.7.
package cssextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"

	"github.com/blackwell-systems/knowing/internal/types"
)

// CSSExtractor implements types.Extractor for CSS and SCSS files using
// tree-sitter AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type CSSExtractor struct{}

// NewCSSExtractor creates a new CSSExtractor.
func NewCSSExtractor() *CSSExtractor {
	return &CSSExtractor{}
}

// Name returns the extractor name.
func (e *CSSExtractor) Name() string {
	return "treesitter-css"
}

// CanHandle returns true for .css and .scss files that are not in
// node_modules/ directories.
func (e *CSSExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".css", ".scss":
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

// Extract parses the CSS file with tree-sitter and produces nodes for
// selectors and custom properties, and edges for imports and var() references.
func (e *CSSExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(css.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	var nodes []types.Node
	var edges []types.Edge

	// File-level node hash for import edges.
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, filepath.Base(opts.FilePath), "file")

	// Track custom property definitions for var() resolution.
	customProps := make(map[string]types.Hash)

	// First pass: collect all nodes (class selectors, ID selectors, custom properties).
	walkCSS(root, opts, &nodes, customProps)

	// Second pass: collect import edges and var() dependency edges.
	extractCSSEdges(root, opts, fileNodeHash, customProps, &edges)

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// walkCSS recursively walks the tree-sitter AST to extract CSS nodes.
func walkCSS(node *sitter.Node, opts types.ExtractOptions, nodes *[]types.Node, customProps map[string]types.Hash) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "class_selector":
		n := extractClassSelector(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
		}

	case "id_selector":
		n := extractIDSelector(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
		}

	case "declaration":
		// Check if this is a custom property definition (--varname: value).
		n := extractCustomProperty(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
			// Extract the property name for var() resolution.
			propName := findPropertyName(node, opts.Content)
			if propName != "" {
				customProps[propName] = n.NodeHash
			}
		}
	}

	// Recurse into children.
	for i := 0; i < int(node.ChildCount()); i++ {
		walkCSS(node.Child(i), opts, nodes, customProps)
	}
}

// extractClassSelector creates a Node for a CSS class selector (.classname).
func extractClassSelector(node *sitter.Node, opts types.ExtractOptions) *types.Node {
	// The class_selector node content is ".classname"; extract the name without the dot.
	content := node.Content(opts.Content)
	name := strings.TrimPrefix(content, ".")
	if name == "" {
		return nil
	}

	line := int(node.StartPoint().Row) + 1
	qname := fmt.Sprintf("%s://%s.class.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "class_selector")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "class_selector",
		Line:          line,
		Signature:     "." + name,
	}
}

// extractIDSelector creates a Node for a CSS ID selector (#idname).
func extractIDSelector(node *sitter.Node, opts types.ExtractOptions) *types.Node {
	// The id_selector node content is "#idname"; extract the name without the hash.
	content := node.Content(opts.Content)
	name := strings.TrimPrefix(content, "#")
	if name == "" {
		return nil
	}

	line := int(node.StartPoint().Row) + 1
	qname := fmt.Sprintf("%s://%s.id.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "id_selector")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "id_selector",
		Line:          line,
		Signature:     "#" + name,
	}
}

// findPropertyName extracts the property name text from a declaration node.
// In the CSS tree-sitter grammar, the property name is a child node of type "property_name".
func findPropertyName(node *sitter.Node, content []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "property_name" {
			return child.Content(content)
		}
	}
	return ""
}

// extractCustomProperty creates a Node for a custom property definition (--varname: value).
func extractCustomProperty(node *sitter.Node, opts types.ExtractOptions) *types.Node {
	name := findPropertyName(node, opts.Content)
	if name == "" || !strings.HasPrefix(name, "--") {
		return nil
	}

	line := int(node.StartPoint().Row) + 1
	qname := fmt.Sprintf("%s://%s.property.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "custom_property")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "custom_property",
		Line:          line,
		Signature:     name,
	}
}

// extractCSSEdges walks the AST to find @import rules and var() references.
func extractCSSEdges(node *sitter.Node, opts types.ExtractOptions, fileNodeHash types.Hash, customProps map[string]types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "import_statement":
		edge := extractImportEdge(node, opts, fileNodeHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}

	case "rule_set":
		// Look for var() references in declarations within this rule set.
		extractVarRefs(node, opts, customProps, edges)
	}

	// Recurse into children (but not into rule_set children again for var refs,
	// since extractVarRefs handles that internally).
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if node.Type() == "rule_set" && child.Type() == "block" {
			// Already handled by extractVarRefs.
			continue
		}
		extractCSSEdges(child, opts, fileNodeHash, customProps, edges)
	}
}

// extractImportEdge creates an edge for an @import rule.
func extractImportEdge(node *sitter.Node, opts types.ExtractOptions, fileNodeHash types.Hash) *types.Edge {
	// Find the string value in the import statement.
	var importPath string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "string_value":
			importPath = child.Content(opts.Content)
			importPath = strings.Trim(importPath, `"'`)
		case "call_expression":
			// url("path")
			for j := 0; j < int(child.ChildCount()); j++ {
				arg := child.Child(j)
				if arg.Type() == "arguments" {
					for k := 0; k < int(arg.ChildCount()); k++ {
						a := arg.Child(k)
						if a.Type() == "string_value" {
							importPath = a.Content(opts.Content)
							importPath = strings.Trim(importPath, `"'`)
						}
					}
				}
			}
		}
	}

	if importPath == "" {
		return nil
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, importPath, types.EmptyHash, importPath, "module")
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

// extractVarRefs looks for var(--name) references within a rule set and creates
// depends_on edges from the rule's selector to the custom property definition.
func extractVarRefs(ruleSet *sitter.Node, opts types.ExtractOptions, customProps map[string]types.Hash, edges *[]types.Edge) {
	// Get the selector for this rule set to use as the source.
	var selectorHash types.Hash
	var hasSelectorHash bool

	// Find selectors node.
	for i := 0; i < int(ruleSet.ChildCount()); i++ {
		child := ruleSet.Child(i)
		if child.Type() == "selectors" {
			// Use the first selector as the source.
			for j := 0; j < int(child.ChildCount()); j++ {
				sel := child.Child(j)
				if sel.Type() == "," || sel.Type() == "comment" {
					continue
				}
				// Find class or ID selector within this selector.
				selectorHash, hasSelectorHash = findSelectorHash(sel, opts)
				if hasSelectorHash {
					break
				}
			}
			break
		}
	}

	if !hasSelectorHash {
		return
	}

	// Walk the block looking for var() references in declaration values.
	for i := 0; i < int(ruleSet.ChildCount()); i++ {
		child := ruleSet.Child(i)
		if child.Type() == "block" {
			walkBlockForVarRefs(child, opts, selectorHash, customProps, edges)
			break
		}
	}
}

// findSelectorHash finds a class_selector or id_selector within a selector node
// and returns its hash.
func findSelectorHash(node *sitter.Node, opts types.ExtractOptions) (types.Hash, bool) {
	if node == nil {
		return types.EmptyHash, false
	}

	switch node.Type() {
	case "class_selector":
		content := node.Content(opts.Content)
		name := strings.TrimPrefix(content, ".")
		if name != "" {
			hash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "class_selector")
			return hash, true
		}
	case "id_selector":
		content := node.Content(opts.Content)
		name := strings.TrimPrefix(content, "#")
		if name != "" {
			hash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "id_selector")
			return hash, true
		}
	}

	// Recurse into children.
	for i := 0; i < int(node.ChildCount()); i++ {
		hash, found := findSelectorHash(node.Child(i), opts)
		if found {
			return hash, true
		}
	}

	return types.EmptyHash, false
}

// walkBlockForVarRefs walks a CSS block looking for var() call expressions.
func walkBlockForVarRefs(node *sitter.Node, opts types.ExtractOptions, selectorHash types.Hash, customProps map[string]types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}

	// Look for call_expression nodes with function_name "var".
	if node.Type() == "call_expression" {
		var funcName string
		var argValue string
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "function_name" {
				funcName = child.Content(opts.Content)
			} else if child.Type() == "arguments" {
				// Find the plain_value inside arguments.
				for j := 0; j < int(child.ChildCount()); j++ {
					arg := child.Child(j)
					if arg.Type() == "plain_value" {
						argValue = arg.Content(opts.Content)
						break
					}
				}
			}
		}
		if funcName == "var" && strings.HasPrefix(argValue, "--") {
			if targetHash, ok := customProps[argValue]; ok {
				provenance := "ast_inferred"
				edgeHash := types.ComputeEdgeHash(selectorHash, targetHash, "depends_on", provenance)
				*edges = append(*edges, types.Edge{
					EdgeHash:   edgeHash,
					SourceHash: selectorHash,
					TargetHash: targetHash,
					EdgeType:   "depends_on",
					Confidence: 0.7,
					Provenance: provenance,
				})
			}
		}
	}

	// Recurse.
	for i := 0; i < int(node.ChildCount()); i++ {
		walkBlockForVarRefs(node.Child(i), opts, selectorHash, customProps, edges)
	}
}
