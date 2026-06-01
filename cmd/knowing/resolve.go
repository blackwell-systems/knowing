package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	csharpresolve "github.com/blackwell-systems/knowing/internal/typresolve/csharp"
	goresolve "github.com/blackwell-systems/knowing/internal/typresolve/go"
	javaresolve "github.com/blackwell-systems/knowing/internal/typresolve/java"
	pythonresolve "github.com/blackwell-systems/knowing/internal/typresolve/python"
	rubyresolve "github.com/blackwell-systems/knowing/internal/typresolve/ruby"
	rustresolve "github.com/blackwell-systems/knowing/internal/typresolve/rust"
	tsresolve "github.com/blackwell-systems/knowing/internal/typresolve/typescript"
	"github.com/blackwell-systems/knowing/internal/types"
)

// resolverSpec defines a language resolver and its file extensions.
type resolverSpec struct {
	name       string
	extensions []string
	factory    func() typresolve.Resolver
}

// allResolvers returns the full set of in-process language resolvers.
func allResolvers() []resolverSpec {
	return []resolverSpec{
		{"Go", []string{".go"}, func() typresolve.Resolver { return goresolve.NewGoResolver() }},
		{"Ruby", []string{".rb"}, func() typresolve.Resolver { return rubyresolve.NewRubyResolver() }},
		{"Python", []string{".py"}, func() typresolve.Resolver { return pythonresolve.NewPythonResolver() }},
		{"TypeScript", []string{".ts", ".tsx"}, func() typresolve.Resolver { return tsresolve.NewTypeScriptResolver() }},
		{"Java", []string{".java"}, func() typresolve.Resolver { return javaresolve.NewJavaResolver() }},
		{"C#", []string{".cs"}, func() typresolve.Resolver { return csharpresolve.NewCSharpResolver() }},
		{"Rust", []string{".rs"}, func() typresolve.Resolver { return rustresolve.NewRustResolver() }},
	}
}

// runInProcessResolver runs in-process resolvers on all supported languages.
// This produces "resolver_resolved" edges that complement or replace LSP enrichment.
func runInProcessResolver(ctx context.Context, st *store.SQLiteStore, repoPath string, repoHash types.Hash) error {
	for _, spec := range allResolvers() {
		if err := runLanguageResolver(ctx, st, repoPath, repoHash, spec); err != nil {
			fmt.Fprintf(os.Stderr, "  %s resolver warning: %v\n", spec.name, err)
		}
	}
	return nil
}

// runLanguageResolver runs a single language resolver on matching files.
func runLanguageResolver(ctx context.Context, st *store.SQLiteStore, repoPath string, repoHash types.Hash, spec resolverSpec) error {
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("query files: %w", err)
	}

	// Collect matching files and read their content.
	var langFiles []typresolve.FileResult
	for _, f := range files {
		if !hasExtension(f.Path, spec.extensions) {
			continue
		}
		// Skip test files for Go (convention: _test.go).
		if strings.HasSuffix(f.Path, "_test.go") {
			continue
		}
		absPath := filepath.Join(repoPath, f.Path)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue // file may have been deleted since indexing
		}
		langFiles = append(langFiles, typresolve.FileResult{
			Path:     f.Path,
			FileHash: f.FileHash,
			Content:  content,
			Language: strings.ToLower(spec.name),
		})
	}

	if len(langFiles) == 0 {
		return nil
	}

	// Build defs from all nodes of this language.
	allNodes, err := queryNodesByExtensions(ctx, st, repoHash, spec.extensions)
	if err != nil {
		return fmt.Errorf("query %s nodes: %w", spec.name, err)
	}

	resolver := spec.factory()
	defs := nodesToDefs(allNodes)
	if err := resolver.InitWorkspace(ctx, defs); err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  In-process %s resolver: %d files...\n", spec.name, len(langFiles))

	var edgeCount int
	for _, fr := range langFiles {
		opts := typresolve.ResolveFileOpts{
			FilePath: fr.Path,
			FileHash: fr.FileHash,
			Content:  fr.Content,
		}
		edges, err := resolver.ResolveFile(ctx, opts)
		if err != nil {
			continue
		}
		for _, e := range edges {
			if putErr := st.PutEdge(ctx, e); putErr == nil {
				edgeCount++
			}
		}
	}

	fmt.Fprintf(os.Stderr, "  In-process %s resolver: %d edges produced\n", spec.name, edgeCount)
	return nil
}

// hasExtension checks if path ends with any of the given extensions.
func hasExtension(path string, exts []string) bool {
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// queryNodesByExtensions returns all nodes from files matching any of the given extensions.
func queryNodesByExtensions(ctx context.Context, st *store.SQLiteStore, repoHash types.Hash, exts []string) ([]types.Node, error) {
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return nil, err
	}

	var allNodes []types.Node
	for _, f := range files {
		if !hasExtension(f.Path, exts) {
			continue
		}
		nodes, err := st.NodesByFileHash(ctx, f.FileHash)
		if err != nil {
			continue
		}
		allNodes = append(allNodes, nodes...)
	}
	return allNodes, nil
}

// nodesToDefs converts stored nodes into ResolverDefs for the type resolver.
func nodesToDefs(nodes []types.Node) []typresolve.ResolverDef {
	defs := make([]typresolve.ResolverDef, 0, len(nodes))
	for _, n := range nodes {
		kind := "function"
		switch n.Kind {
		case "type", "struct", "interface", "class":
			kind = "type"
		case "method":
			kind = "method"
		}
		defs = append(defs, typresolve.ResolverDef{
			QualifiedName: n.QualifiedName,
			Kind:          kind,
			Signature:     n.Signature,
			FilePath:      "", // not available from stored node
		})
	}
	return defs
}
