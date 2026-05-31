package rustresolve

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"
)

// Compile-time check that RustResolver implements typresolve.Resolver.
var _ typresolve.Resolver = (*RustResolver)(nil)

// RustResolver implements typresolve.Resolver for Rust. It uses tree-sitter
// AST walking and a shared type registry to resolve call expressions and
// type references without requiring an external LSP server.
type RustResolver struct {
	registry *typresolve.Registry
}

// NewRustResolver creates a new RustResolver.
func NewRustResolver() *RustResolver {
	return &RustResolver{}
}

// Language returns "rust".
func (r *RustResolver) Language() string { return "rust" }

// InitWorkspace builds the cross-file type registry from extracted
// definitions. Called once before any ResolveFile calls.
func (r *RustResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs)
	return nil
}

// ResolveFile resolves type references in a single Rust file. Thread-safe
// after InitWorkspace completes (registry is read-only, all mutable state
// is stack-local).
func (r *RustResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// Get the tree-sitter root node from opts.ParsedTree.
	root, err := extractRustRoot(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Build per-file use map.
	uses := BuildUseMap(root, opts.Content, opts.FilePath)

	// Build glob imports list.
	globImports := BuildGlobImports(root, opts.Content, opts.FilePath)

	// Expand glob imports: scan registry for matching entries.
	expandGlobImports(r.registry, globImports, uses)

	// Infer module QN from file path.
	moduleQN := InferModuleQN(opts.FilePath)

	// Extract repo URL from existing edges (best-effort).
	repoURL := ""

	// Create per-file resolve context.
	rctx := &ResolveContext{
		Registry:    r.registry,
		Scope:       typresolve.NewScope(nil),
		Uses:        uses,
		GlobImports: globImports,
		ModuleQN:    moduleQN,
		ImplType:    "",
		Content:     opts.Content,
		TraitBounds: nil,
	}

	// Resolve calls.
	edges := ResolveCallsInFile(rctx, root, opts.FileHash, repoURL, opts.FilePath)

	return edges, nil
}

// expandGlobImports scans the registry for types and functions whose qualified
// name begins with one of the glob prefixes, and adds them to the uses map.
// This enables `use module::*` to make symbols available by their short names.
func expandGlobImports(reg *typresolve.Registry, globs []string, uses map[string]string) {
	if len(globs) == 0 {
		return
	}

	// We cannot iterate the registry directly (private maps), but we can
	// check known stdlib prefixes and common patterns. For glob imports from
	// std modules, register the well-known symbols.
	for _, prefix := range globs {
		// Check well-known std prelude entries.
		if prefix == "std::prelude" || prefix == "std::prelude::v1" ||
			prefix == "std::prelude::rust_2021" || prefix == "std::prelude::rust_2024" {
			// Prelude is implicitly imported; add common entries.
			preludeEntries := []struct{ short, full string }{
				{"Vec", "std::Vec"}, {"String", "std::String"},
				{"Option", "std::Option"}, {"Result", "std::Result"},
				{"Box", "std::Box"}, {"Some", "std::Option"},
				{"None", "std::Option"}, {"Ok", "std::Result"},
				{"Err", "std::Result"}, {"Clone", "std::Clone"},
				{"Default", "std::Default"}, {"Iterator", "std::Iterator"},
			}
			for _, e := range preludeEntries {
				if _, exists := uses[e.short]; !exists {
					uses[e.short] = e.full
				}
			}
			continue
		}

		// For other prefixes, register the prefix itself so that
		// identifier resolution can try prefix + "::" + name.
		// Store the glob prefix in uses with a special glob marker.
		uses["__glob__"+prefix] = prefix
	}
}

// extractRustRoot extracts a *sitter.Node root from opts.ParsedTree, falling
// back to re-parsing opts.Content if the tree is not directly available.
func extractRustRoot(ctx context.Context, opts typresolve.ResolveFileOpts) (*sitter.Node, error) {
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
			if r := rp.GetRoot(); r != nil {
				return r, nil
			}
		}
	}

	// Fallback: re-parse from content.
	if len(opts.Content) == 0 {
		return nil, fmt.Errorf("no parsed tree and no content for %s", opts.FilePath)
	}

	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.FilePath, err)
	}
	return tree.RootNode(), nil
}
