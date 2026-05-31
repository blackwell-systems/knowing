package goresolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	golang "github.com/smacker/go-tree-sitter/golang"
)

// Compile-time check that GoResolver implements typresolve.Resolver.
var _ typresolve.Resolver = (*GoResolver)(nil)

// GoResolver implements typresolve.Resolver for Go. It uses tree-sitter
// AST walking and a shared type registry to resolve call expressions and
// type references without requiring an external LSP server.
type GoResolver struct {
	registry *typresolve.Registry
}

// NewGoResolver creates a new GoResolver.
func NewGoResolver() *GoResolver {
	return &GoResolver{}
}

// Language returns "go".
func (r *GoResolver) Language() string { return "go" }

// InitWorkspace builds the cross-file type registry from extracted
// definitions. Called once before any ResolveFile calls.
func (r *GoResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs, nil, nil)
	return nil
}

// ResolveFile resolves type references in a single Go file. Thread-safe
// after InitWorkspace completes (registry is read-only, all mutable state
// is stack-local).
func (r *GoResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// Get the tree-sitter root node from opts.ParsedTree.
	root, err := extractRoot(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build per-file import map.
	imports := BuildImportMap(root, opts.Content)

	// Determine package QN and repoURL from existing edges.
	pkgQN, repoURL := inferPackageInfo(opts.FilePath, opts.Edges)

	// Create per-file resolve context.
	rctx := &ResolveContext{
		Registry: r.registry,
		Scope:    typresolve.NewScope(nil),
		Imports:  imports,
		PkgQN:    pkgQN,
		Content:  opts.Content,
	}

	// Resolve calls.
	edges := ResolveCallsInFile(rctx, root, opts.FileHash, repoURL, opts.FilePath)

	return edges, nil
}

// extractRoot extracts a *sitter.Node root from opts.ParsedTree, falling
// back to re-parsing opts.Content if the tree is not directly available.
func extractRoot(ctx context.Context, opts typresolve.ResolveFileOpts) (*sitter.Node, error) {
	if opts.ParsedTree != nil {
		// Try direct *sitter.Node assertion.
		if root, ok := opts.ParsedTree.(*sitter.Node); ok {
			return root, nil
		}

		// Try interface with GetRoot method.
		type rootProvider interface {
			GetRoot() *sitter.Node
		}
		if rp, ok := opts.ParsedTree.(rootProvider); ok {
			if root := rp.GetRoot(); root != nil {
				return root, nil
			}
		}
	}

	// Fallback: re-parse from content.
	if len(opts.Content) == 0 {
		return nil, fmt.Errorf("no parsed tree and no content to parse for %s", opts.FilePath)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.FilePath, err)
	}
	// Note: tree ownership is transferred to caller. The tree-sitter Go
	// bindings handle cleanup via finalizers.
	return tree.RootNode(), nil
}

// inferPackageInfo extracts the package qualified name and repo URL from
// existing edges. Falls back to computing from the file path.
func inferPackageInfo(filePath string, edges []types.Edge) (pkgQN string, repoURL string) {
	// Try to extract from existing edge source qualified names.
	// Edge QNs have format "repoURL://pkgPath.symbolName".
	for _, e := range edges {
		// Use source hash indirectly: we don't have the QN in the edge.
		// Instead, check CallSiteFile which is the file path.
		if e.CallSiteFile != "" || e.EdgeType != "" {
			// We can't extract QN from the edge struct directly.
			// Fall through to path-based inference.
			break
		}
	}

	// Infer from file path.
	// Strip filename to get directory path as package path.
	pkgQN = inferPkgQNFromPath(filePath)
	return pkgQN, ""
}

// inferPkgQNFromPath infers a package qualified name from a file path.
// Uses the directory portion of the path.
func inferPkgQNFromPath(filePath string) string {
	// Find the last slash to get the directory.
	idx := strings.LastIndex(filePath, "/")
	if idx >= 0 {
		return filePath[:idx]
	}
	return filePath
}
