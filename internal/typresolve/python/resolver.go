package pyresolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	python "github.com/smacker/go-tree-sitter/python"
)

// Compile-time check that PythonResolver implements typresolve.Resolver.
var _ typresolve.Resolver = (*PythonResolver)(nil)

// PythonResolver implements typresolve.Resolver for Python. It uses
// tree-sitter AST walking and a shared type registry to resolve call
// expressions and type references without requiring an external LSP server.
type PythonResolver struct {
	registry *typresolve.Registry
}

// NewPythonResolver creates a new PythonResolver.
func NewPythonResolver() *PythonResolver {
	return &PythonResolver{}
}

// Language returns "python".
func (r *PythonResolver) Language() string { return "python" }

// InitWorkspace builds the cross-file type registry from extracted
// definitions. Called once before any ResolveFile calls.
func (r *PythonResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs)
	// Pre-register stdlib types and functions as a fallback layer.
	RegisterStdlib(r.registry)
	return nil
}

// ResolveFile resolves type references in a single Python file. Thread-safe
// after InitWorkspace completes (registry is read-only, all mutable state
// is stack-local).
func (r *PythonResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// Get the tree-sitter root node from opts.ParsedTree.
	root, err := extractPythonRoot(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build per-file import map.
	imports := BuildImportMap(root, opts.Content)

	// Determine module QN and repoURL from existing edges or file path.
	moduleQN, repoURL := inferModuleInfo(opts.FilePath, opts.Edges)

	// Create per-file resolve context.
	rctx := &ResolveContext{
		Registry: r.registry,
		Scope:    typresolve.NewScope(nil),
		Imports:  imports,
		ModuleQN: moduleQN,
		Content:  opts.Content,
	}

	// Bind imports into scope.
	for name, info := range imports {
		if info.IsFromStyle {
			// from X import Y: bind Y to Named(X.Y)
			rctx.Scope.Bind(name, typresolve.Named(info.ModulePath))
		} else {
			// import X: bind X as module (Named with module path)
			rctx.Scope.Bind(name, typresolve.Named(info.ModulePath))
		}
	}

	// Resolve calls in the file.
	edges := ResolveCallsInFile(rctx, root, opts.FileHash, repoURL, opts.FilePath)

	return edges, nil
}

// extractPythonRoot extracts a *sitter.Node root from opts.ParsedTree,
// falling back to re-parsing opts.Content if the tree is not directly
// available.
func extractPythonRoot(ctx context.Context, opts typresolve.ResolveFileOpts) (*sitter.Node, error) {
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
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.FilePath, err)
	}
	return tree.RootNode(), nil
}

// inferModuleInfo extracts the module qualified name and repo URL from
// existing edges or computes from the file path.
func inferModuleInfo(filePath string, edges []types.Edge) (moduleQN string, repoURL string) {
	// Try to extract from file path.
	moduleQN = inferModuleQNFromPath(filePath)
	return moduleQN, ""
}

// inferModuleQNFromPath converts a file path to a Python dotted module path.
// Examples:
//
//	"foo/bar/baz.py" -> "foo.bar.baz"
//	"foo/bar/__init__.py" -> "foo.bar"
//	"baz.py" -> "baz"
func inferModuleQNFromPath(filePath string) string {
	// Strip leading "./" or "/"
	p := filePath
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")

	// Remove .py extension
	if strings.HasSuffix(p, ".py") {
		p = p[:len(p)-3]
	}

	// Handle __init__ case
	if strings.HasSuffix(p, "/__init__") {
		p = p[:len(p)-len("/__init__")]
	} else if p == "__init__" {
		return ""
	}

	// Replace slashes with dots
	return strings.ReplaceAll(p, "/", ".")
}
