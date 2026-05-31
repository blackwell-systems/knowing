package tsresolve

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TypeScriptResolver implements typresolve.Resolver for TypeScript/JavaScript.
type TypeScriptResolver struct {
	registry *typresolve.Registry
}

// Compile-time interface check.
var _ typresolve.Resolver = (*TypeScriptResolver)(nil)

// NewTypeScriptResolver creates a new TypeScript resolver.
func NewTypeScriptResolver() *TypeScriptResolver {
	return &TypeScriptResolver{}
}

// Language returns "typescript".
func (r *TypeScriptResolver) Language() string { return "typescript" }

// InitWorkspace builds a Registry from TS definitions.
func (r *TypeScriptResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs)
	return nil
}

// ResolveFile walks the TypeScript AST and resolves calls and references.
func (r *TypeScriptResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// 1. Get tree-sitter root node.
	var root *sitter.Node
	switch v := opts.ParsedTree.(type) {
	case *sitter.Node:
		root = v
	default:
		// Re-parse as fallback.
		parser := sitter.NewParser()
		parser.SetLanguage(typescript.GetLanguage())
		tree, err := parser.ParseCtx(ctx, nil, opts.Content)
		if err != nil {
			return nil, fmt.Errorf("parse: %w", err)
		}
		defer tree.Close()
		root = tree.RootNode()
	}

	// 2. Build per-file import map.
	imports := BuildImportMap(root, opts.Content)

	// 3. Determine module QN from file path.
	moduleQN := inferModuleQN(opts.FilePath)

	// 4. Create ResolveContext.
	rctx := &ResolveContext{
		Registry: r.registry,
		Scope:    typresolve.NewScope(nil),
		Imports:  imports,
		ModuleQN: moduleQN,
		Content:  opts.Content,
	}

	// 5. Bind imports into scope.
	for name, imp := range imports {
		modulePath := ResolveModulePath(imp.ModulePath, opts.FilePath)
		if modulePath == "" {
			// Bare module specifier: use the module path directly.
			modulePath = imp.ModulePath
		}

		if imp.IsNamespace || imp.IsDefault {
			// Namespace or default import: bind as Named(modulePath).
			rctx.Scope.Bind(name, typresolve.Named(modulePath))
		} else {
			// Named import: try to resolve from registry.
			lookupName := imp.OriginalName
			if lookupName == "" {
				lookupName = name
			}
			fullQN := modulePath + "." + lookupName

			if f := r.registry.LookupFunc(fullQN); f != nil && f.Signature != nil {
				rctx.Scope.Bind(name, f.Signature)
			} else if r.registry.LookupType(fullQN) != nil {
				rctx.Scope.Bind(name, typresolve.Named(fullQN))
			} else {
				// Not found in registry; bind as Named for potential resolution.
				rctx.Scope.Bind(name, typresolve.Named(fullQN))
			}
		}
	}

	// 6. Resolve calls.
	repoURL := ""
	if len(opts.Edges) > 0 {
		// Extract repoURL from existing edge hashes if available.
		// For now, leave empty; hash computation uses moduleQN.
	}

	edges := ResolveCallsInFile(rctx, root, opts.FileHash, repoURL, opts.FilePath)

	return edges, nil
}

// inferModuleQN converts a file path to a forward-slash module path without
// extension. Mirrors computeQNamePrefix from tsextractor.
func inferModuleQN(filePath string) string {
	dir := filepath.Dir(filePath)
	if dir == "." {
		dir = ""
	}
	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if dir == "" {
		return base
	}
	return filepath.ToSlash(dir) + "/" + base
}
