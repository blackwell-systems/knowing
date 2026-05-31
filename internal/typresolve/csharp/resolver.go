package csresolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	csharp "github.com/smacker/go-tree-sitter/csharp"
)

// Compile-time check that CSharpResolver implements typresolve.Resolver.
var _ typresolve.Resolver = (*CSharpResolver)(nil)

// CSharpResolver implements typresolve.Resolver for C#. It uses tree-sitter
// AST walking and a shared type registry to resolve call expressions and
// type references without requiring an external LSP server.
type CSharpResolver struct {
	registry *typresolve.Registry
}

// NewCSharpResolver creates a new CSharpResolver.
func NewCSharpResolver() *CSharpResolver {
	return &CSharpResolver{}
}

// Language returns "csharp".
func (r *CSharpResolver) Language() string { return "csharp" }

// InitWorkspace builds the cross-file type registry from extracted
// definitions. Called once before any ResolveFile calls.
func (r *CSharpResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs)
	return nil
}

// ResolveFile resolves type references in a single C# file. Thread-safe
// after InitWorkspace completes (registry is read-only, all mutable state
// is stack-local).
func (r *CSharpResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// Get the tree-sitter root node from opts.ParsedTree.
	root, err := extractCSharpRoot(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build per-file using map.
	usings := BuildUsingMap(root, opts.Content)

	// Determine namespace/package info from existing edges.
	namespaceQN, repoURL := inferCSharpPackageInfo(opts.FilePath, opts.Edges)

	// Create per-file resolve context.
	rctx := &ResolveContext{
		Registry:       r.registry,
		Scope:          typresolve.NewScope(nil),
		Usings:         usings,
		NamespaceStack: nil,
		Content:        opts.Content,
		ModuleQN:       namespaceQN,
	}

	// Resolve calls.
	edges := ResolveCallsInFile(rctx, root, opts.FileHash, repoURL, opts.FilePath)

	return edges, nil
}

// extractCSharpRoot extracts a *sitter.Node root from opts.ParsedTree,
// falling back to re-parsing opts.Content if the tree is not directly available.
func extractCSharpRoot(ctx context.Context, opts typresolve.ResolveFileOpts) (*sitter.Node, error) {
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
	parser.SetLanguage(csharp.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.FilePath, err)
	}
	return tree.RootNode(), nil
}

// inferCSharpPackageInfo extracts the namespace and repo URL from existing edges.
func inferCSharpPackageInfo(filePath string, edges []types.Edge) (namespaceQN string, repoURL string) {
	// Try to infer from file path: strip filename to get directory path.
	idx := strings.LastIndex(filePath, "/")
	if idx >= 0 {
		namespaceQN = filePath[:idx]
	} else {
		namespaceQN = filePath
	}
	return namespaceQN, ""
}
