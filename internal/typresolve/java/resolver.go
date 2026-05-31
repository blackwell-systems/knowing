package javaresolve

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	java "github.com/smacker/go-tree-sitter/java"
)

// Compile-time check that JavaResolver implements typresolve.Resolver.
var _ typresolve.Resolver = (*JavaResolver)(nil)

// JavaResolver implements typresolve.Resolver for Java. It uses tree-sitter
// AST walking and a shared type registry to resolve call expressions and
// type references without requiring an external LSP server.
type JavaResolver struct {
	registry *typresolve.Registry
}

// NewJavaResolver creates a new JavaResolver.
func NewJavaResolver() *JavaResolver {
	return &JavaResolver{}
}

// Language returns "java".
func (r *JavaResolver) Language() string { return "java" }

// InitWorkspace builds the cross-file type registry from extracted
// definitions. Called once before any ResolveFile calls. Sets the stdlib
// registry as fallback so java.lang, java.util, java.io types resolve
// without explicit definitions.
func (r *JavaResolver) InitWorkspace(ctx context.Context, defs []typresolve.ResolverDef) error {
	r.registry = BuildRegistry(defs)
	r.registry.SetFallback(GetStdlibRegistry())
	return nil
}

// ResolveFile resolves type references in a single Java file. Thread-safe
// after InitWorkspace completes (registry is read-only, all mutable state
// is stack-local).
func (r *JavaResolver) ResolveFile(ctx context.Context, opts typresolve.ResolveFileOpts) ([]types.Edge, error) {
	if r.registry == nil {
		return nil, fmt.Errorf("resolver not initialized: call InitWorkspace first")
	}

	// Get the tree-sitter root node from opts.ParsedTree.
	root, err := extractRoot(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Extract package qualified name.
	pkgQN := ExtractPackage(root, opts.Content)
	if pkgQN == "" {
		pkgQN = inferPkgQNFromPath(opts.FilePath)
	}

	// Build per-file import info (includes wildcards and static imports).
	importInfo := BuildImportInfo(root, opts.Content)

	// Determine repoURL from existing edges.
	repoURL := inferRepoURL(opts.Edges)

	// Create per-file resolve context.
	rctx := &ResolveContext{
		Registry:   r.registry,
		Scope:      typresolve.NewScope(nil),
		Imports:    importInfo.Regular,
		ImportInfo: importInfo,
		PkgQN:      pkgQN,
		Content:    opts.Content,
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
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", opts.FilePath, err)
	}
	return tree.RootNode(), nil
}

// inferPkgQNFromPath infers a Java package qualified name from a file path.
// Converts path separators to dots and strips the .java extension.
// For "src/main/java/com/example/service/UserService.java", this attempts
// to extract "com.example.service".
func inferPkgQNFromPath(filePath string) string {
	// Strip .java extension.
	path := strings.TrimSuffix(filePath, ".java")

	// Find the last slash to get directory.
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return ""
	}
	dir := path[:idx]

	// Try to find standard Java source root markers.
	for _, marker := range []string{"/src/main/java/", "/src/test/java/", "/src/"} {
		if markerIdx := strings.Index(dir, marker); markerIdx >= 0 {
			pkgPath := dir[markerIdx+len(marker):]
			return strings.ReplaceAll(pkgPath, "/", ".")
		}
	}

	// Fallback: convert directory separators to dots.
	return strings.ReplaceAll(dir, "/", ".")
}

// inferRepoURL extracts the repo URL from existing edges if available.
func inferRepoURL(edges []types.Edge) string {
	// Edges don't directly contain the repo URL, but we can try to
	// extract it from CallSiteFile or other fields.
	// For now, return empty string (same pattern as Go resolver).
	return ""
}
