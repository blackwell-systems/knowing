// Package gotsextractor provides a Go language extractor using tree-sitter
// for fast AST-only parsing. It implements types.Extractor and produces
// declaration nodes and syntactic call/import edges without type resolution.
//
// This is the default (fast-path) extractor for Go files. It completes in
// milliseconds per file because it uses tree-sitter's incremental parser
// rather than the Go compiler. The tradeoff is lower-confidence edges:
// without type information, cross-package calls are resolved heuristically
// from import aliases, so edges have provenance "ast_inferred" and
// confidence 0.7. The LSP enrichment pass (internal/enrichment) later
// upgrades confirmed edges to "lsp_resolved" with confidence 0.9.
//
// Call-site positions (line, column, file) are recorded on every call edge
// so the enricher can query gopls GetDefinition at the exact location.
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

// CanHandle returns true for .go files not in the vendor directory.
// Test files (_test.go) are included so test-scope can trace call edges.
func (e *GoTreeSitterExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".go") {
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

	// Walk top-level children of the root node. Tree-sitter represents Go
	// source as a flat list of top-level declarations under the root. Each
	// child's Type() string corresponds to a Go grammar rule name.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_declaration":
			node := extractFuncDecl(child, opts, pkgPath)
			nodes = append(nodes, node)
			body := child.ChildByFieldName("body")
			callEdges := extractCallEdges(body, opts, pkgPath, node.NodeHash, imports)
			edges = append(edges, callEdges...)
			// Extract throws edges for error returns and panics.
			throwsEdges := extractGoThrowsEdges(body, opts, pkgPath, node.NodeHash, imports)
			edges = append(edges, throwsEdges...)
			// Extract HTTP route registrations from function bodies.
			routes := extractRouteSymbols(body, opts, pkgPath, imports)
			rn, re := routeSymbolsToNodesAndEdges(routes, opts, pkgPath)
			nodes = append(nodes, rn...)
			edges = append(edges, re...)

		case "method_declaration":
			node := extractMethodDecl(child, opts, pkgPath)
			nodes = append(nodes, node)
			body := child.ChildByFieldName("body")
			callEdges := extractCallEdges(body, opts, pkgPath, node.NodeHash, imports)
			edges = append(edges, callEdges...)
			// Extract throws edges for error returns and panics.
			throwsEdges := extractGoThrowsEdges(body, opts, pkgPath, node.NodeHash, imports)
			edges = append(edges, throwsEdges...)
			// Extract HTTP route registrations from method bodies.
			routes := extractRouteSymbols(body, opts, pkgPath, imports)
			rn, re := routeSymbolsToNodesAndEdges(routes, opts, pkgPath)
			nodes = append(nodes, rn...)
			edges = append(edges, re...)

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

	// Create a synthetic file node for import edge sources. Import edges use
	// a file-level node as their source (since imports belong to the file, not
	// a function). This node must be stored so the edge's source is not dangling.
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, filepath.Base(opts.FilePath), "file")
	hasImportEdges := false
	for _, e := range edges {
		if e.EdgeType == "imports" {
			hasImportEdges = true
			break
		}
	}
	if hasImportEdges {
		nodes = append(nodes, types.Node{
			NodeHash:      fileNodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, filepath.Base(opts.FilePath)),
			Kind:          "file",
			Line:          1,
		})
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

// computePkgPath determines the full Go package path by combining the module
// path from go.mod with the file's directory relative to the module root.
// For example, if the module is "github.com/org/repo" and the file is at
// "internal/store/sqlite.go", the package path is "github.com/org/repo/internal/store".
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
		Doc:           extractDocComment(node, opts.Content),
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
		Doc:           extractDocComment(node, opts.Content),
	}
}

// extractReceiverType extracts the type name from a method receiver
// parameter list node. In tree-sitter's Go grammar, the receiver is
// a "parameter_list" containing one "parameter_declaration" whose "type"
// field is the receiver type (possibly wrapped in pointer_type).
func extractReceiverType(receiver *sitter.Node, content []byte) string {
	if receiver == nil {
		return "Unknown"
	}
	for i := 0; i < int(receiver.ChildCount()); i++ {
		child := receiver.Child(i)
		if child.Type() == "parameter_declaration" {
			return extractTypeName(child.ChildByFieldName("type"), content)
		}
	}
	return "Unknown"
}

// extractTypeName gets the base type name from a tree-sitter type node,
// stripping pointer indirection and generic type parameters. This handles
// the common patterns in Go receiver types: T, *T, and Generic[T].
func extractTypeName(typeNode *sitter.Node, content []byte) string {
	if typeNode == nil {
		return "Unknown"
	}
	switch typeNode.Type() {
	case "pointer_type":
		// *T -> T: skip the "*" child and recurse into the inner type.
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
			Doc:           extractDocComment(child, opts.Content),
		})
	}
	return nodes
}

// extractDocComment extracts the documentation comment preceding a declaration node.
// Works language-agnostically by looking at the previous sibling(s) for comment nodes.
// Returns the first 200 characters of the combined comment text, stripped of comment markers.
func extractDocComment(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	// Collect comment nodes immediately preceding this declaration.
	// In tree-sitter, comments are sibling nodes at the same level.
	var commentLines []string
	prev := node.PrevSibling()
	for prev != nil {
		nodeType := prev.Type()
		if nodeType == "comment" {
			text := prev.Content(content)
			// Strip Go comment markers: "// " or "/* ... */"
			text = strings.TrimPrefix(text, "//")
			text = strings.TrimPrefix(text, "/*")
			text = strings.TrimSuffix(text, "*/")
			text = strings.TrimSpace(text)
			if text != "" {
				commentLines = append([]string{text}, commentLines...) // prepend (reversed order)
			}
			prev = prev.PrevSibling()
		} else {
			break // stop at first non-comment sibling
		}
	}

	if len(commentLines) == 0 {
		return ""
	}

	doc := strings.Join(commentLines, " ")
	// Cap at 200 chars to avoid bloating the DB.
	if len(doc) > 200 {
		doc = doc[:200]
	}
	return doc
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
				// Store the call-site position so the LSP enricher can query
				// gopls GetDefinition at exactly this location. For selector
				// expressions (pkg.Func or receiver.Method), use the field
				// (method/function name) position rather than the operand
				// position. This ensures gopls resolves the called symbol,
				// not the receiver variable.
				posNode := callSitePositionNode(funcNode)
				edge.CallSiteLine = int(posNode.StartPoint().Row) + 1
				edge.CallSiteCol = int(posNode.StartPoint().Column)
				edge.CallSiteFile = opts.FilePath
				*edges = append(*edges, *edge)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, pkgPath, sourceHash, imports, edges)
	}
}

// extractGoThrowsEdges walks a function/method body looking for error return
// patterns (fmt.Errorf, errors.New) and panic() calls. For each, it emits a
// "throws" edge from the function to a synthetic error type node.
func extractGoThrowsEdges(body *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string) []types.Edge {
	if body == nil {
		return nil
	}
	var edges []types.Edge
	walkForThrows(body, opts, pkgPath, sourceHash, imports, &edges)
	return edges
}

// walkForThrows recursively walks nodes looking for error-returning patterns
// and panic calls in Go function bodies.
func walkForThrows(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "call_expression":
		// Detect panic(...) calls.
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil && funcNode.Type() == "identifier" && funcNode.Content(opts.Content) == "panic" {
			errorName := "panic"
			targetHash := types.ComputeNodeHash(opts.RepoURL, "builtin", types.EmptyHash, errorName, "error")
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
			*edges = append(*edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: sourceHash,
				TargetHash: targetHash,
				EdgeType:   "throws",
				Confidence: 0.7,
				Provenance: provenance,
			})
		}
		// Detect fmt.Errorf(...) and errors.New(...) inside return statements.
		// These are handled at the return_statement level below.

	case "return_statement":
		// Walk the return statement's children looking for error-constructing calls.
		walkReturnForErrors(node, opts, pkgPath, sourceHash, imports, edges)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForThrows(node.Child(i), opts, pkgPath, sourceHash, imports, edges)
	}
}

// walkReturnForErrors walks a return statement looking for fmt.Errorf or
// errors.New call expressions and emits throws edges for them.
func walkReturnForErrors(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string, edges *[]types.Edge) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil && funcNode.Type() == "selector_expression" {
			operandNode := funcNode.ChildByFieldName("operand")
			fieldNode := funcNode.ChildByFieldName("field")
			if operandNode != nil && fieldNode != nil {
				pkg := operandNode.Content(opts.Content)
				fn := fieldNode.Content(opts.Content)

				var errorName string
				if fn == "Errorf" {
					// Resolve the package alias to see if it maps to "fmt".
					if importPath, ok := imports[pkg]; ok && importPath == "fmt" {
						errorName = "fmt.Errorf"
					} else if pkg == "fmt" {
						errorName = "fmt.Errorf"
					}
				} else if fn == "New" {
					if importPath, ok := imports[pkg]; ok && importPath == "errors" {
						errorName = "errors.New"
					} else if pkg == "errors" {
						errorName = "errors.New"
					}
				}

				if errorName != "" {
					targetHash := types.ComputeNodeHash(opts.RepoURL, "errors", types.EmptyHash, errorName, "error")
					provenance := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: sourceHash,
						TargetHash: targetHash,
						EdgeType:   "throws",
						Confidence: 0.7,
						Provenance: provenance,
					})
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkReturnForErrors(node.Child(i), opts, pkgPath, sourceHash, imports, edges)
	}
}

// callSitePositionNode returns the tree-sitter node whose position should be
// used as the call-site location for LSP enrichment. For selector expressions,
// this is the "field" child (the method/function name) because gopls resolves
// the symbol at that position. For plain identifiers, the node itself is used.
//
// Example: for `p.registry.Register(entity)`, the call_expression's function
// is the selector `p.registry.Register`. Using the expression's start position
// (column of `p`) would cause gopls to resolve the variable `p` rather than
// the method `Register`. By using the field's position, gopls correctly
// resolves `Register` to its definition in the target package.
func callSitePositionNode(funcNode *sitter.Node) *sitter.Node {
	if funcNode.Type() == "selector_expression" {
		field := funcNode.ChildByFieldName("field")
		if field != nil {
			return field
		}
	}
	return funcNode
}

// resolveCallEdge creates an edge for a call expression's function node.
//
// For selector expressions (pkg.Func or receiver.Method), the function
// distinguishes between package-qualified calls and method calls:
//   - If the operand is an import alias, this is a package function call
//     (kind="function", package=the import path).
//   - If the operand is a local variable (not in imports), this is a method
//     call (kind="method", package=current package or inferred from type).
//   - If the operand is itself a selector expression (chained access like
//     p.registry.Method()), we extract the method name and treat it as a
//     method call with lower confidence.
func resolveCallEdge(funcNode *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, imports map[string]string) *types.Edge {
	content := opts.Content
	var targetName, targetKind, targetPkg string
	confidence := 0.7

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

		switch operandNode.Type() {
		case "identifier":
			operandName := operandNode.Content(content)
			// Look up in import map to resolve to full package path.
			if importPath, ok := imports[operandName]; ok {
				// Package-qualified call: modulea.NewRegistry()
				targetKind = "function"
				targetPkg = importPath
			} else {
				// Method call on a local variable: r.Register(), e.QualifiedName()
				// The operand is not an import alias, so this is a method call.
				targetKind = "method"
				targetPkg = pkgPath
				confidence = 0.5
			}

		case "selector_expression":
			// Chained selector: p.registry.Method()
			// The innermost field is the receiver variable, and the outer field
			// is the method name. We treat this as a method call. We cannot
			// determine the target package without type information, so we use
			// the current package and lower confidence.
			targetKind = "method"
			targetPkg = pkgPath
			confidence = 0.4

			// Try to resolve through the import map if the chain root is an
			// imported package. For example: modulea.SomeType.Method() would
			// have the root identifier as an import alias. Walk down to find it.
			rootOperand := operandNode
			for rootOperand != nil && rootOperand.Type() == "selector_expression" {
				rootOperand = rootOperand.ChildByFieldName("operand")
			}
			if rootOperand != nil && rootOperand.Type() == "identifier" {
				rootName := rootOperand.Content(content)
				if importPath, ok := imports[rootName]; ok {
					targetPkg = importPath
					confidence = 0.5
				}
			}

		default:
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
		Confidence: confidence,
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

// inferRepoURL determines the repo URL for a target package using heuristics
// (since tree-sitter does not provide Go module information).
//
// Resolution strategy:
//  1. Check opts.ModuleToRepoURL for an exact module-path prefix match.
//     This uses the map built from all indexed repos' go.mod files.
//  2. If the target and local package share the first 3 path segments
//     (e.g., "github.com/org/repo"), assume they are in the same repo.
//  3. Stdlib check: if the first segment has no dots (e.g., "fmt", "net"),
//     return "stdlib".
//  4. External: take the first 3 segments as the repo URL
//     (e.g., "github.com/org/repo").
func inferRepoURL(opts types.ExtractOptions, targetPkg, localPkg string) string {
	// Tier 1: known module mapping from indexed repos.
	if opts.ModuleToRepoURL != nil {
		for modulePath, repoURL := range opts.ModuleToRepoURL {
			if strings.HasPrefix(targetPkg, modulePath) {
				return repoURL
			}
		}
	}

	// Tier 2: common prefix heuristic (same first 3 path segments = same repo).
	if localPkg != "" {
		localParts := strings.Split(localPkg, "/")
		targetParts := strings.Split(targetPkg, "/")
		if len(localParts) >= 3 && len(targetParts) >= 3 &&
			localParts[0] == targetParts[0] && localParts[1] == targetParts[1] && localParts[2] == targetParts[2] {
			return opts.RepoURL
		}
	}

	// Tier 3: stdlib (no dots in first segment means standard library).
	parts := strings.Split(targetPkg, "/")
	if len(parts) > 0 && !strings.Contains(parts[0], ".") {
		return "stdlib"
	}

	// Tier 4: external package; first 3 segments form the repo URL.
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "/")
	}
	return targetPkg
}

// routeSymbol represents an HTTP route handler registration detected in source.
type routeSymbol struct {
	ServiceName  string
	RoutePattern string
	HandlerHash  types.Hash
	MappingType  string // "http_route"
}

// httpRouterPackages maps known HTTP router import paths to a set of method
// names that register route handlers. The first string argument of these calls
// is the route pattern, and the second is the handler function reference.
var httpRouterPackages = map[string]map[string]bool{
	"net/http": {
		"HandleFunc": true,
		"Handle":     true,
	},
	"github.com/go-chi/chi": {
		"Get":    true,
		"Post":   true,
		"Put":    true,
		"Delete": true,
		"Patch":  true,
	},
	"github.com/go-chi/chi/v5": {
		"Get":    true,
		"Post":   true,
		"Put":    true,
		"Delete": true,
		"Patch":  true,
	},
	"github.com/gin-gonic/gin": {
		"GET":    true,
		"POST":   true,
		"PUT":    true,
		"DELETE": true,
		"PATCH":  true,
	},
	"github.com/labstack/echo": {
		"GET":    true,
		"POST":   true,
		"PUT":    true,
		"DELETE": true,
		"PATCH":  true,
	},
	"github.com/labstack/echo/v4": {
		"GET":    true,
		"POST":   true,
		"PUT":    true,
		"DELETE": true,
		"PATCH":  true,
	},
	"github.com/gorilla/mux": {
		"HandleFunc": true,
		"Handle":     true,
	},
}

// allRouteMethodNames is the union of all method names across all router
// packages. Used as a fast pre-filter before checking the import map.
var allRouteMethodNames map[string]bool

func init() {
	allRouteMethodNames = make(map[string]bool)
	for _, methods := range httpRouterPackages {
		for m := range methods {
			allRouteMethodNames[m] = true
		}
	}
}

// extractRouteSymbols walks a function body for HTTP route registration
// patterns and returns detected route symbols.
func extractRouteSymbols(body *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string) []routeSymbol {
	if body == nil {
		return nil
	}
	var routes []routeSymbol
	walkForRoutes(body, opts, pkgPath, imports, &routes)
	return routes
}

// walkForRoutes recursively walks nodes looking for HTTP route registration
// call expressions (e.g., http.HandleFunc("/path", handler)).
func walkForRoutes(node *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string, routes *[]routeSymbol) {
	if node == nil {
		return
	}
	if node.Type() == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil && funcNode.Type() == "selector_expression" {
			if rs := tryExtractRoute(funcNode, node, opts, pkgPath, imports); rs != nil {
				*routes = append(*routes, *rs)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkForRoutes(node.Child(i), opts, pkgPath, imports, routes)
	}
}

// tryExtractRoute checks if a selector_expression call is an HTTP route
// registration and extracts the route symbol if so.
func tryExtractRoute(funcNode, callNode *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string) *routeSymbol {
	content := opts.Content

	fieldNode := funcNode.ChildByFieldName("field")
	if fieldNode == nil {
		return nil
	}
	methodName := fieldNode.Content(content)

	// Fast pre-filter: skip if method name is not a known route method.
	if !allRouteMethodNames[methodName] {
		return nil
	}

	operandNode := funcNode.ChildByFieldName("operand")
	if operandNode == nil {
		return nil
	}

	// Resolve the operand to an import path.
	var importPath string
	if operandNode.Type() == "identifier" {
		alias := operandNode.Content(content)
		if ip, ok := imports[alias]; ok {
			importPath = ip
		}
	}

	// If not resolved via import map, it could be a local variable (e.g., r := chi.NewRouter()).
	// For local variables, we check if the method name is in any router package's method set.
	// This is a heuristic: if the method name matches AND the file imports a router package,
	// we treat it as a route registration.
	if importPath == "" {
		importPath = inferRouterPackageFromContext(methodName, imports)
	}

	if importPath == "" {
		return nil
	}

	// Check if this import path is a known router package with this method.
	methods, ok := httpRouterPackages[importPath]
	if !ok || !methods[methodName] {
		return nil
	}

	// Extract the route pattern from the first argument.
	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return nil
	}
	routePattern := extractFirstStringArg(argsNode, content)
	if routePattern == "" {
		return nil
	}

	// Extract the handler reference from the second argument for hash computation.
	handlerName := extractSecondArgName(argsNode, content)

	// Compute the handler hash. If the handler is a simple identifier, hash it
	// as a function in the current package.
	var handlerHash types.Hash
	if handlerName != "" {
		handlerHash = types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, handlerName, "function")
	}

	// Derive HTTP method from the router method name.
	httpMethod := deriveHTTPMethod(methodName)

	return &routeSymbol{
		ServiceName:  pkgPath,
		RoutePattern: httpMethod + " " + routePattern,
		HandlerHash:  handlerHash,
		MappingType:  "http_route",
	}
}

// inferRouterPackageFromContext checks if any imported package is a known
// router package that supports the given method name. This handles cases
// where the router is stored in a local variable (e.g., r := chi.NewRouter()).
func inferRouterPackageFromContext(methodName string, imports map[string]string) string {
	for _, importPath := range imports {
		if methods, ok := httpRouterPackages[importPath]; ok && methods[methodName] {
			return importPath
		}
	}
	return ""
}

// extractFirstStringArg extracts the string value from the first argument in
// an argument_list node. Returns empty string if the first arg is not a string literal.
func extractFirstStringArg(argsNode *sitter.Node, content []byte) string {
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		if child.Type() == "interpreted_string_literal" || child.Type() == "raw_string_literal" {
			val := child.Content(content)
			return strings.Trim(val, `"` + "`")
		}
	}
	return ""
}

// extractSecondArgName returns the identifier name of the second argument,
// if it is a simple identifier. Otherwise returns empty string.
func extractSecondArgName(argsNode *sitter.Node, content []byte) string {
	argIdx := 0
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		// Skip punctuation (commas, parens).
		if child.Type() == "," || child.Type() == "(" || child.Type() == ")" {
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

// deriveHTTPMethod maps a router method name to an HTTP method string.
func deriveHTTPMethod(methodName string) string {
	switch methodName {
	case "Get", "GET":
		return "GET"
	case "Post", "POST":
		return "POST"
	case "Put", "PUT":
		return "PUT"
	case "Delete", "DELETE":
		return "DELETE"
	case "Patch", "PATCH":
		return "PATCH"
	case "HandleFunc", "Handle":
		// For HandleFunc/Handle, we don't know the HTTP method.
		return "ANY"
	default:
		return "ANY"
	}
}

// routeSymbolsToNodesAndEdges converts detected route symbols into graph nodes
// and edges. Each route symbol becomes a "route_handler" node and a
// "handles_route" edge pointing to the handler function node.
func routeSymbolsToNodesAndEdges(routes []routeSymbol, opts types.ExtractOptions, pkgPath string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for _, rs := range routes {
		// Create a route_handler node whose QualifiedName encodes the method and path.
		qname := fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, rs.RoutePattern)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, rs.RoutePattern, "route_handler")

		nodes = append(nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qname,
			Kind:          "route_handler",
			Signature:     rs.RoutePattern,
		})

		// If we have a handler hash, create an edge from the route node to the handler.
		if rs.HandlerHash != types.EmptyHash {
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(nodeHash, rs.HandlerHash, "handles_route", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: nodeHash,
				TargetHash: rs.HandlerHash,
				EdgeType:   "handles_route",
				Confidence: 0.7,
				Provenance: provenance,
			})
		}
	}

	return nodes, edges
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
