package rubyresolve

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/ruby"
)

// Compile-time check that RubyResolver implements typresolve.Resolver.
var _ typresolve.Resolver = (*RubyResolver)(nil)

// RubyResolver implements typresolve.Resolver for Ruby. It uses tree-sitter
// AST walking and a shared type registry to resolve call expressions and
// constant references without requiring an external LSP server.
type RubyResolver struct {
	registry *typresolve.Registry
}

// NewRubyResolver creates a new RubyResolver.
func NewRubyResolver() *RubyResolver {
	return &RubyResolver{}
}

// Language returns "ruby".
func (r *RubyResolver) Language() string { return "ruby" }

// InitWorkspace builds the cross-file type registry from extracted
// definitions. Called once before any ResolveFile calls.
func (r *RubyResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs)
	return nil
}

// ResolveFile resolves type references in a single Ruby file. Thread-safe
// after InitWorkspace completes (registry is read-only, all mutable state
// is stack-local).
func (r *RubyResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// Get the tree-sitter root node from opts.ParsedTree.
	root, err := extractRubyRoot(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build per-file require map.
	requires := BuildRequireMap(root, opts.Content, opts.FilePath)

	// Determine the file-level qualified name prefix (file path without extension).
	qnPrefix := strings.TrimSuffix(filepath.ToSlash(opts.FilePath), filepath.Ext(opts.FilePath))

	// Extract repo URL from existing edges (best effort).
	repoURL := ""

	// Create per-file resolve context.
	rctx := &ResolveContext{
		Registry:    r.registry,
		Scope:       typresolve.NewScope(nil),
		Requires:    requires,
		Nesting:     nil,
		CurrentFile: opts.FilePath,
		Content:     opts.Content,
	}

	// Resolve calls in the file.
	edges := ResolveCallsInFile(rctx, root, opts.FileHash, repoURL, opts.FilePath)

	// Set provenance and confidence on all returned edges.
	for i := range edges {
		edges[i].Provenance = typresolve.ProvenanceResolverResolved
		edges[i].Confidence = typresolve.ResolverConfidence
	}

	_ = qnPrefix // used internally by ResolveCallsInFile via filePath

	return edges, nil
}

// extractRubyRoot extracts a *sitter.Node root from opts.ParsedTree,
// falling back to re-parsing opts.Content with the Ruby grammar.
func extractRubyRoot(ctx context.Context, opts typresolve.ResolveFileOpts) (*sitter.Node, error) {
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
	parser.SetLanguage(ruby.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.FilePath, err)
	}
	return tree.RootNode(), nil
}
