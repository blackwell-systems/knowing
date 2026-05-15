// Package gotsextractor provides a Go language extractor using tree-sitter
// for fast AST-only parsing. It implements types.Extractor and produces
// declaration nodes and syntactic call/import edges without type resolution.
package gotsextractor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/blackwell-systems/knowing/internal/types"
)

// openFile is a package-level variable so tests can override it.
var openFile = func(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

// GoTreeSitterExtractor implements types.Extractor for Go files using
// tree-sitter AST parsing. It is the fast-path extractor: AST-only,
// no type resolution, completes in milliseconds per file.
type GoTreeSitterExtractor struct {
	parser *sitter.Parser
}

// NewGoTreeSitterExtractor creates a new GoTreeSitterExtractor with a
// tree-sitter parser configured for Go.
func NewGoTreeSitterExtractor() *GoTreeSitterExtractor {
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	return &GoTreeSitterExtractor{parser: parser}
}

// Name returns the extractor name.
func (e *GoTreeSitterExtractor) Name() string {
	return "go-treesitter"
}

// CanHandle returns true for .go files that are not test files and not
// in the vendor directory. Same logic as GoExtractor.
func (e *GoTreeSitterExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	if strings.HasSuffix(path, "_test.go") {
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

// Extract parses the Go file with tree-sitter and produces nodes for
// declarations and edges for calls and imports.
func (e *GoTreeSitterExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	tree, err := e.parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	// Determine the package path.
	pkgPath, err := computePkgPath(root, opts)
	if err != nil {
		return nil, fmt.Errorf("compute package path: %w", err)
	}

	// Build import alias map for resolving selector expressions.
	imports := buildImportMap(root, opts.Content)

	var nodes []types.Node
	var edges []types.Edge

	// Walk top-level declarations.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_declaration":
			node := extractFuncDecl(child, opts, pkgPath)
			nodes = append(nodes, node)
			callEdges := extractCallEdges(child.ChildByFieldName("body"), opts, pkgPath, node.NodeHash, imports)
			edges = append(edges, callEdges...)

		case "method_declaration":
			node := extractMethodDecl(child, opts, pkgPath)
			nodes = append(nodes, node)
			callEdges := extractCallEdges(child.ChildByFieldName("body"), opts, pkgPath, node.NodeHash, imports)
			edges = append(edges, callEdges...)

		case "type_declaration":
			typeNodes := extractTypeDecl(child, opts, pkgPath)
			nodes = append(nodes, typeNodes...)

		case "const_declaration":
			constNodes := extractValueDecl(child, opts, pkgPath, "const")
			nodes = append(nodes, constNodes...)

		case "var_declaration":
			varNodes := extractValueDecl(child, opts, pkgPath, "var")
			nodes = append(nodes, varNodes...)

		case "import_declaration":
			importEdges := extractImportEdges(child, opts, pkgPath)
			edges = append(edges, importEdges...)
		}
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

// computePkgPath determines the full package path by reading the package
// clause from the AST and the module path from go.mod.
func computePkgPath(root *sitter.Node, opts types.ExtractOptions) (string, error) {
	// Get the directory portion of the file path relative to module root.
	relDir := filepath.Dir(opts.FilePath)
	if relDir == "." {
		relDir = ""
	}

	// Read the module path from go.mod.
	modulePath, err := readModulePath(opts.ModuleRoot)
	if err != nil {
		return "", err
	}

	if relDir == "" {
		return modulePath, nil
	}
	return modulePath + "/" + filepath.ToSlash(relDir), nil
}

// readModulePath reads the module path from go.mod in the given directory.
func readModulePath(moduleRoot string) (string, error) {
	gomodPath := filepath.Join(moduleRoot, "go.mod")
	f, err := openFile(gomodPath)
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

// buildImportMap builds a mapping from import alias to full import path.
// For imports without an explicit alias, the alias is the last path segment.
func buildImportMap(root *sitter.Node, content []byte) map[string]string {
	imports := make(map[string]string)
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "import_declaration" {
			continue
		}
		for j := 0; j < int(child.ChildCount()); j++ {
			spec := child.Child(j)
			if spec.Type() == "import_spec" {
				addImportSpec(spec, content, imports)
			} else if spec.Type() == "import_spec_list" {
				for k := 0; k < int(spec.ChildCount()); k++ {
					item := spec.Child(k)
					if item.Type() == "import_spec" {
						addImportSpec(item, content, imports)
					}
				}
			}
		}
	}
	return imports
}

// addImportSpec extracts one import spec into the imports map.
func addImportSpec(spec *sitter.Node, content []byte, imports map[string]string) {
	pathNode := spec.ChildByFieldName("path")
	if pathNode == nil {
		return
	}
	importPath := strings.Trim(pathNode.Content(content), `"`)

	nameNode := spec.ChildByFieldName("name")
	if nameNode != nil {
		alias := nameNode.Content(content)
		if alias != "." && alias != "_" {
			imports[alias] = importPath
		}
	} else {
		// Default alias is the last path segment.
		parts := strings.Split(importPath, "/")
		alias := parts[len(parts)-1]
		imports[alias] = importPath
	}
}

// extractFuncDecl creates a Node for a function declaration.
func extractFuncDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath string) types.Node {
	nameNode := node.ChildByFieldName("name")
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, "function")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name),
		Kind:          "function",
		Line:          line,
		Signature:     fmt.Sprintf("func %s()", name),
	}
}

// extractMethodDecl creates a Node for a method declaration.
func extractMethodDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath string) types.Node {
	nameNode := node.ChildByFieldName("name")
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	receiverType := extractReceiverType(node.ChildByFieldName("receiver"), opts.Content)

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, "method")
	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, pkgPath, receiverType, name),
		Kind:          "method",
		Line:          line,
		Signature:     fmt.Sprintf("func (%s) %s()", receiverType, name),
	}
}

// extractReceiverType extracts the type name from a method receiver
// parameter list node.
func extractReceiverType(receiver *sitter.Node, content []byte) string {
	if receiver == nil {
		return "Unknown"
	}
	// receiver is a parameter_list containing parameter_declaration(s)
	for i := 0; i < int(receiver.ChildCount()); i++ {
		child := receiver.Child(i)
		if child.Type() == "parameter_declaration" {
			return extractTypeName(child.ChildByFieldName("type"), content)
		}
	}
	return "Unknown"
}

// extractTypeName gets the base type name from a type node, stripping
// pointer indirection.
func extractTypeName(typeNode *sitter.Node, content []byte) string {
	if typeNode == nil {
		return "Unknown"
	}
	switch typeNode.Type() {
	case "pointer_type":
		// *T -> T
		for i := 0; i < int(typeNode.ChildCount()); i++ {
			child := typeNode.Child(i)
			if child.Type() != "*" {
				return extractTypeName(child, content)
			}
		}
		return "Unknown"
	case "type_identifier":
		return typeNode.Content(content)
	case "generic_type":
		// Generic[T] -> Generic
		for i := 0; i < int(typeNode.ChildCount()); i++ {
			child := typeNode.Child(i)
			if child.Type() == "type_identifier" {
				return child.Content(content)
			}
		}
		return typeNode.Content(content)
	default:
		return typeNode.Content(content)
	}
}

// extractTypeDecl creates Nodes for type declarations (type specs).
func extractTypeDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath string) []types.Node {
	var nodes []types.Node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "type_spec" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(opts.Content)
		line := int(nameNode.StartPoint().Row) + 1

		kind := "type"
		typeBody := child.ChildByFieldName("type")
		if typeBody != nil && typeBody.Type() == "interface_type" {
			kind = "interface"
		}

		nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, kind)
		nodes = append(nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name),
			Kind:          kind,
			Line:          line,
			Signature:     fmt.Sprintf("type %s", name),
		})
	}
	return nodes
}

// extractValueDecl creates Nodes for const or var declarations.
func extractValueDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath, kind string) []types.Node {
	var nodes []types.Node
	specType := "const_spec"
	if kind == "var" {
		specType = "var_spec"
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != specType {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(opts.Content)
		if name == "_" {
			continue
		}
		line := int(nameNode.StartPoint().Row) + 1

		nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, kind)
		nodes = append(nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name),
			Kind:          kind,
			Line:          line,
			Signature:     fmt.Sprintf("%s %s", kind, name),
		})
	}
	return nodes
}

// extractCallEdges walks a function/method body for call_expression nodes
// and creates call edges.
func extractCallEdges(body *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string) []types.Edge {
	if body == nil {
		return nil
	}
	var edges []types.Edge
	walkForCalls(body, opts, pkgPath, sourceHash, imports, &edges)
	return edges
}

// walkForCalls recursively walks nodes looking for call_expression nodes.
func walkForCalls(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string, edges *[]types.Edge) {
	if node == nil {
		return
	}
	if node.Type() == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			edge := resolveCallEdge(funcNode, opts, pkgPath, sourceHash, imports)
			if edge != nil {
				// Store call-site position for LSP enrichment.
				edge.CallSiteLine = int(node.StartPoint().Row) + 1 // tree-sitter is 0-indexed, we store 1-indexed
				edge.CallSiteCol = int(node.StartPoint().Column)
				edge.CallSiteFile = opts.FilePath
				*edges = append(*edges, *edge)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, pkgPath, sourceHash, imports, edges)
	}
}

// resolveCallEdge creates an edge for a call expression's function node.
func resolveCallEdge(funcNode *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string) *types.Edge {
	content := opts.Content
	var targetName, targetKind, targetPkg string

	switch funcNode.Type() {
	case "identifier":
		// Local function call.
		targetName = funcNode.Content(content)
		targetKind = "function"
		targetPkg = pkgPath

	case "selector_expression":
		// Qualified call: pkg.Func or receiver.Method
		operandNode := funcNode.ChildByFieldName("operand")
		fieldNode := funcNode.ChildByFieldName("field")
		if operandNode == nil || fieldNode == nil {
			return nil
		}
		targetName = fieldNode.Content(content)
		targetKind = "function"

		if operandNode.Type() == "identifier" {
			operandName := operandNode.Content(content)
			// Look up in import map to resolve to full package path.
			if importPath, ok := imports[operandName]; ok {
				targetPkg = importPath
			} else {
				targetPkg = operandName
			}
		} else {
			return nil
		}

	default:
		return nil
	}

	if targetName == "" {
		return nil
	}

	targetRepoURL := inferRepoURL(opts, targetPkg, pkgPath)
	targetHash := types.ComputeNodeHash(targetRepoURL, targetPkg, types.EmptyHash, targetName, targetKind)
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "calls",
		Confidence: 0.7,
		Provenance: provenance,
	}
}

// extractImportEdges creates import edges for an import declaration.
func extractImportEdges(node *sitter.Node, opts types.ExtractOptions, pkgPath string) []types.Edge {
	var edges []types.Edge
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, filepath.Base(opts.FilePath), "file")

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "import_spec" {
			edge := makeImportEdge(child, opts, pkgPath, fileNodeHash)
			if edge != nil {
				edges = append(edges, *edge)
			}
		} else if child.Type() == "import_spec_list" {
			for j := 0; j < int(child.ChildCount()); j++ {
				spec := child.Child(j)
				if spec.Type() == "import_spec" {
					edge := makeImportEdge(spec, opts, pkgPath, fileNodeHash)
					if edge != nil {
						edges = append(edges, *edge)
					}
				}
			}
		}
	}
	return edges
}

// makeImportEdge creates a single import edge from an import_spec node.
func makeImportEdge(spec *sitter.Node, opts types.ExtractOptions, pkgPath string, fileNodeHash types.Hash) *types.Edge {
	pathNode := spec.ChildByFieldName("path")
	if pathNode == nil {
		return nil
	}
	impPath := strings.Trim(pathNode.Content(opts.Content), `"`)

	impRepoURL := inferRepoURL(opts, impPath, pkgPath)
	impHash := types.ComputeNodeHash(impRepoURL, impPath, types.EmptyHash, impPath, "package")

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(fileNodeHash, impHash, "imports", provenance)
	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: fileNodeHash,
		TargetHash: impHash,
		EdgeType:   "imports",
		Confidence: 0.7,
		Provenance: provenance,
	}
}

// inferRepoURL determines the repo URL for a target package. If the
// target package starts with the same module prefix as opts.RepoURL,
// it is local. Otherwise, use heuristic inference from the import path.
func inferRepoURL(opts types.ExtractOptions, targetPkg, localPkg string) string {
	// If it matches a known module mapping, use that.
	if opts.ModuleToRepoURL != nil {
		for modulePath, repoURL := range opts.ModuleToRepoURL {
			if strings.HasPrefix(targetPkg, modulePath) {
				return repoURL
			}
		}
	}

	// If it shares a common prefix with the local package, assume same repo.
	// This is a heuristic since we do not have module info from go/packages.
	if localPkg != "" {
		localParts := strings.Split(localPkg, "/")
		targetParts := strings.Split(targetPkg, "/")
		if len(localParts) >= 3 && len(targetParts) >= 3 &&
			localParts[0] == targetParts[0] && localParts[1] == targetParts[1] && localParts[2] == targetParts[2] {
			return opts.RepoURL
		}
	}

	// Stdlib check: no dots in first segment.
	parts := strings.Split(targetPkg, "/")
	if len(parts) > 0 && !strings.Contains(parts[0], ".") {
		return "stdlib"
	}

	// External: first 3 segments.
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "/")
	}
	return targetPkg
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
