// Package goextractor provides a Go language extractor that uses
// golang.org/x/tools/go/packages for full type resolution.
//
// Unlike the tree-sitter extractor (gotsextractor), this extractor loads
// full type information via the Go compiler, enabling precise resolution of:
//   - Cross-package function calls (via TypesInfo.Uses)
//   - Interface implementations (via go/types.Implements)
//   - Non-call identifier references (variable reads, type references)
//
// The tradeoff is speed: loading packages requires invoking the Go compiler,
// which is significantly slower than tree-sitter parsing. Use the -full flag
// on "knowing index" to select this extractor.
//
// All edges produced by this extractor have provenance "ast_resolved" and
// confidence 1.0 because type resolution confirms the target.
package goextractor

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	gotypes "go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/blackwell-systems/knowing/internal/types"
)

// GoExtractor extracts nodes and edges from Go source files using the
// go/packages loader for full type resolution.
type GoExtractor struct{}

// NewGoExtractor creates a new GoExtractor.
func NewGoExtractor() *GoExtractor {
	return &GoExtractor{}
}

// Name returns the extractor name.
func (g *GoExtractor) Name() string {
	return "go"
}

// CanHandle returns true for .go files that are not test files and not
// in the vendor directory.
func (g *GoExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	// Reject vendor paths.
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "vendor" {
			return false
		}
	}
	return true
}

// Extract parses the Go file and produces nodes for each declared symbol
// and edges for calls, imports, implements, and references.
func (g *GoExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	// Determine the package directory from the file path.
	absPath := filepath.Join(opts.ModuleRoot, opts.FilePath)
	dir := filepath.Dir(absPath)

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedModule,
		Dir:     dir,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	if len(pkgs) == 0 {
		return &types.ExtractResult{}, nil
	}

	pkg := pkgs[0]
	return g.extractFromPackage(opts, pkg)
}

// ExtractWithPackage extracts nodes and edges from a Go file using a
// pre-loaded package. This avoids the per-file packages.Load overhead.
// The pkg parameter must contain Syntax, TypesInfo, and Fset for the
// target file.
func (g *GoExtractor) ExtractWithPackage(
	ctx context.Context,
	opts types.ExtractOptions,
	pkg *packages.Package,
) (*types.ExtractResult, error) {
	return g.extractFromPackage(opts, pkg)
}

// extractFromPackage contains the shared extraction logic used by both
// Extract and ExtractWithPackage. It finds the target AST file by base
// name, walks declarations for nodes, and extracts all edge types.
func (g *GoExtractor) extractFromPackage(
	opts types.ExtractOptions,
	pkg *packages.Package,
) (*types.ExtractResult, error) {
	// Find the AST file matching our target.
	baseName := filepath.Base(opts.FilePath)
	var targetFile *ast.File
	targetFset := pkg.Fset
	for _, f := range pkg.Syntax {
		pos := targetFset.Position(f.Pos())
		if filepath.Base(pos.Filename) == baseName {
			targetFile = f
			break
		}
	}

	if targetFile == nil {
		return &types.ExtractResult{}, nil
	}

	pkgPath := pkg.PkgPath

	var nodes []types.Node
	var edges []types.Edge

	// Walk AST declarations. ast.Inspect visits every node in the tree;
	// we only care about top-level FuncDecl and GenDecl nodes.
	ast.Inspect(targetFile, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			node := g.funcDeclNode(opts, pkgPath, targetFset, decl)
			nodes = append(nodes, node)

			// Analyze function body for call edges.
			if decl.Body != nil {
				callEdges := g.extractCallEdges(opts, pkgPath, node.NodeHash, decl.Body, pkg)
				edges = append(edges, callEdges...)
			}

		case *ast.GenDecl:
			genNodes := g.genDeclNodes(opts, pkgPath, targetFset, decl)
			nodes = append(nodes, genNodes...)
		}
		return true
	})

	// Add import edges from a synthetic "file" node to each imported package.
	// We use a synthetic file-level node (kind="file") as the source rather
	// than creating N edges from every function in the file, keeping the
	// graph manageable while still recording the dependency.
	for _, imp := range targetFile.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		impRepoURL := resolveTargetRepoURL(opts, impPath, pkg)
		impHash := types.ComputeNodeHash(impRepoURL, impPath, types.EmptyHash, impPath, "package")

		fileNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, filepath.Base(opts.FilePath), "file")
		provenance := "ast_resolved"
		edgeHash := types.ComputeEdgeHash(fileNodeHash, impHash, "imports", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: fileNodeHash,
			TargetHash: impHash,
			EdgeType:   "imports",
			Confidence: 1.0,
			Provenance: provenance,
		})
	}

	// Extract implements edges: concrete types that satisfy interfaces.
	implEdges := g.extractImplementsEdges(opts, pkgPath, pkg)
	edges = append(edges, implEdges...)

	// Extract references edges: non-call identifier usages.
	refEdges := g.extractReferenceEdges(opts, pkgPath, targetFile, pkg)
	edges = append(edges, refEdges...)

	// Sort nodes deterministically.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].QualifiedName != nodes[j].QualifiedName {
			return nodes[i].QualifiedName < nodes[j].QualifiedName
		}
		return nodes[i].Kind < nodes[j].Kind
	})

	// Sort edges deterministically.
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

	// Deduplicate edges.
	edges = deduplicateEdges(edges)

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// funcDeclNode creates a Node for a function or method declaration.
func (g *GoExtractor) funcDeclNode(opts types.ExtractOptions, pkgPath string, fset *token.FileSet, decl *ast.FuncDecl) types.Node {
	kind := "function"
	name := decl.Name.Name
	qualifiedName := fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name)

	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		kind = "method"
		recvType := receiverTypeName(decl.Recv.List[0].Type)
		qualifiedName = fmt.Sprintf("%s://%s.%s.%s", opts.RepoURL, pkgPath, recvType, name)
	}

	sig := formatFuncSignature(decl)
	pos := fset.Position(decl.Pos())

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, kind)

	return types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qualifiedName,
		Kind:          kind,
		Line:          pos.Line,
		Signature:     sig,
	}
}

// genDeclNodes creates Nodes for type, const, and var declarations.
func (g *GoExtractor) genDeclNodes(opts types.ExtractOptions, pkgPath string, fset *token.FileSet, decl *ast.GenDecl) []types.Node {
	var nodes []types.Node

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			kind := "type"
			if _, ok := s.Type.(*ast.InterfaceType); ok {
				kind = "interface"
			}
			name := s.Name.Name
			qualifiedName := fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name)
			pos := fset.Position(s.Pos())
			nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, kind)
			nodes = append(nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qualifiedName,
				Kind:          kind,
				Line:          pos.Line,
				Signature:     fmt.Sprintf("type %s", name),
			})

		case *ast.ValueSpec:
			kind := "var"
			if decl.Tok == token.CONST {
				kind = "const"
			}
			for _, ident := range s.Names {
				if ident.Name == "_" {
					continue
				}
				name := ident.Name
				qualifiedName := fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name)
				pos := fset.Position(ident.Pos())
				nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, kind)
				nodes = append(nodes, types.Node{
					NodeHash:      nodeHash,
					FileHash:      opts.FileHash,
					QualifiedName: qualifiedName,
					Kind:          kind,
					Line:          pos.Line,
					Signature:     fmt.Sprintf("%s %s", kind, name),
				})
			}
		}
	}

	return nodes
}

// extractCallEdges analyzes a function body for call expressions and
// creates edges to the called functions.
func (g *GoExtractor) extractCallEdges(opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, body *ast.BlockStmt, pkg *packages.Package) []types.Edge {
	var edges []types.Edge

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var targetName, targetKind, targetPkg string

		switch fn := call.Fun.(type) {
		case *ast.Ident:
			// Local or builtin function call.
			targetName = fn.Name
			targetKind = "function"
			targetPkg = pkgPath

		case *ast.SelectorExpr:
			// Qualified call: either pkg.Func or receiver.Method.
			targetName = fn.Sel.Name
			targetKind = "function"
			if ident, ok := fn.X.(*ast.Ident); ok {
				// Use TypesInfo to distinguish package qualifiers from method
				// receivers. If the identifier resolves to a PkgName, this is
				// a cross-package call; otherwise it is a method call on a local
				// variable and we keep the target in the same package.
				if pkg.TypesInfo != nil {
					if obj, exists := pkg.TypesInfo.Uses[ident]; exists {
						if pkgName, isPkg := obj.(*gotypes.PkgName); isPkg {
							targetPkg = pkgName.Imported().Path()
						} else {
							targetKind = "method"
							targetPkg = pkgPath
						}
					} else {
						targetPkg = ident.Name
					}
				} else {
					targetPkg = ident.Name
				}
			}
		default:
			return true
		}

		if targetName == "" {
			return true
		}

		targetRepoURL := resolveTargetRepoURL(opts, targetPkg, pkg)
		targetHash := types.ComputeNodeHash(targetRepoURL, targetPkg, types.EmptyHash, targetName, targetKind)
		provenance := "ast_resolved"
		edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: sourceHash,
			TargetHash: targetHash,
			EdgeType:   "calls",
			Confidence: 1.0,
			Provenance: "ast_resolved",
		})

		return true
	})

	return edges
}

// extractImplementsEdges detects concrete types that satisfy interfaces
// declared in the same package and emits "implements" edges.
func (g *GoExtractor) extractImplementsEdges(opts types.ExtractOptions, pkgPath string, pkg *packages.Package) []types.Edge {
	if pkg.TypesInfo == nil || pkg.Types == nil {
		return nil
	}

	// Collect all named interfaces and concrete types in the package scope.
	// We only check within the same package; cross-package implements edges
	// are discovered later by the LSP enricher.
	scope := pkg.Types.Scope()
	var ifaces []*gotypes.Named
	var concretes []*gotypes.Named

	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		named, ok := obj.Type().(*gotypes.Named)
		if !ok {
			continue
		}
		if gotypes.IsInterface(named) {
			ifaces = append(ifaces, named)
		} else {
			concretes = append(concretes, named)
		}
	}

	var edges []types.Edge
	for _, concrete := range concretes {
		for _, iface := range ifaces {
			ifaceType := iface.Underlying().(*gotypes.Interface)
			// Check both T and *T because pointer receivers satisfy interfaces
			// that value receivers do not.
			if gotypes.Implements(concrete, ifaceType) || gotypes.Implements(gotypes.NewPointer(concrete), ifaceType) {
				concreteHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, concrete.Obj().Name(), "type")
				ifaceHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, iface.Obj().Name(), "interface")
				provenance := "ast_resolved"
				edgeHash := types.ComputeEdgeHash(concreteHash, ifaceHash, "implements", provenance)
				edges = append(edges, types.Edge{
					EdgeHash:   edgeHash,
					SourceHash: concreteHash,
					TargetHash: ifaceHash,
					EdgeType:   "implements",
					Confidence: 1.0,
					Provenance: provenance,
				})
			}
		}
	}
	return edges
}

// extractReferenceEdges emits "references" edges for non-call identifier usages
// in the target file (e.g., reading a variable, referencing a type in a signature).
func (g *GoExtractor) extractReferenceEdges(opts types.ExtractOptions, pkgPath string, file *ast.File, pkg *packages.Package) []types.Edge {
	if pkg.TypesInfo == nil {
		return nil
	}

	// Collect call-expression function identifiers so we can exclude them
	// from "references" edges. Call targets already get "calls" edges;
	// emitting a "references" edge for the same identifier would be redundant.
	callIdents := make(map[*ast.Ident]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			callIdents[fn] = true
		case *ast.SelectorExpr:
			callIdents[fn.Sel] = true
		}
		return true
	})

	seen := make(map[types.Hash]struct{})
	var edges []types.Edge

	for ident, obj := range pkg.TypesInfo.Uses {
		if callIdents[ident] {
			continue
		}
		// Skip package names and builtins.
		if _, isPkg := obj.(*gotypes.PkgName); isPkg {
			continue
		}
		if obj.Pkg() == nil {
			continue // builtin
		}

		targetName := obj.Name()
		targetKind := "var"
		switch obj.(type) {
		case *gotypes.Func:
			targetKind = "function"
		case *gotypes.TypeName:
			targetKind = "type"
		case *gotypes.Const:
			targetKind = "const"
		}

		targetPkg := pkgPath
		if obj.Pkg() != nil && obj.Pkg().Path() != pkg.PkgPath {
			targetPkg = obj.Pkg().Path()
		}

		// Use a synthetic file-level source for reference edges.
		fileNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, filepath.Base(opts.FilePath), "file")
		targetRepoURL := resolveTargetRepoURL(opts, targetPkg, pkg)
		targetHash := types.ComputeNodeHash(targetRepoURL, targetPkg, types.EmptyHash, targetName, targetKind)
		provenance := "ast_resolved"
		edgeHash := types.ComputeEdgeHash(fileNodeHash, targetHash, "references", provenance)

		if _, exists := seen[edgeHash]; exists {
			continue
		}
		seen[edgeHash] = struct{}{}

		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: fileNodeHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: 1.0,
			Provenance: provenance,
		})
	}

	return edges
}

// receiverTypeName extracts the type name from a method receiver.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	default:
		return "Unknown"
	}
}

// formatFuncSignature builds a human-readable signature string.
func formatFuncSignature(decl *ast.FuncDecl) string {
	var sb strings.Builder
	sb.WriteString("func ")
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		recvType := receiverTypeName(decl.Recv.List[0].Type)
		sb.WriteString("(")
		sb.WriteString(recvType)
		sb.WriteString(") ")
	}
	sb.WriteString(decl.Name.Name)
	sb.WriteString("()")
	return sb.String()
}

// resolveTargetRepoURL determines the correct repo URL for a cross-repo
// call target. The resolution strategy has three tiers:
//
//  1. Local package: if targetPkg is within the same Go module, use opts.RepoURL.
//  2. Known external: if the imported package has Module info, look up its module
//     path in opts.ModuleToRepoURL (populated from the repos table). This ensures
//     edges point to the repo URL actually stored in the database.
//  3. Heuristic fallback: infer the repo URL from the import path structure
//     (e.g., "github.com/org/repo/pkg" -> "github.com/org/repo").
func resolveTargetRepoURL(opts types.ExtractOptions, targetPkg string, pkg *packages.Package) string {
	// Local package: same module as the source.
	if pkg.Module != nil && strings.HasPrefix(targetPkg, pkg.Module.Path) {
		return opts.RepoURL
	}

	// External package: try to get module path from the imported package.
	if importedPkg, ok := pkg.Imports[targetPkg]; ok && importedPkg.Module != nil {
		modulePath := importedPkg.Module.Path
		// Check if we have a stored repo URL for this module path.
		if opts.ModuleToRepoURL != nil {
			if repoURL, ok := opts.ModuleToRepoURL[modulePath]; ok {
				return repoURL
			}
		}
		return modulePath
	}

	// Heuristic fallback: infer from import path structure.
	return inferRepoURL(targetPkg)
}

// inferRepoURL extracts the probable repository URL from a Go import path.
// For "github.com/org/repo/pkg/sub" it returns "github.com/org/repo".
// For stdlib paths (no dots in first segment) it returns "stdlib".
func inferRepoURL(importPath string) string {
	parts := strings.Split(importPath, "/")
	if len(parts) == 0 {
		return importPath
	}
	// stdlib: no dots in first segment (e.g., "fmt", "net/http")
	if !strings.Contains(parts[0], ".") {
		return "stdlib"
	}
	// github.com, gitlab.com, bitbucket.org: first 3 segments
	if len(parts) >= 3 {
		return strings.Join(parts[:3], "/")
	}
	// golang.org/x/foo or similar with fewer segments
	if len(parts) >= 2 {
		return strings.Join(parts[:2], "/")
	}
	return importPath
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
