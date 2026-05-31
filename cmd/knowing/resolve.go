package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	goresolve "github.com/blackwell-systems/knowing/internal/typresolve/go"
	rubyresolve "github.com/blackwell-systems/knowing/internal/typresolve/ruby"
	"github.com/blackwell-systems/knowing/internal/types"
)

// runInProcessResolver runs in-process resolvers on all supported languages.
// This produces "resolver_resolved" edges that complement or replace LSP enrichment.
func runInProcessResolver(ctx context.Context, st *store.SQLiteStore, repoPath string, repoHash types.Hash) error {
	if err := runGoResolver(ctx, st, repoPath, repoHash); err != nil {
		fmt.Fprintf(os.Stderr, "  Go resolver warning: %v\n", err)
	}
	if err := runRubyResolver(ctx, st, repoPath, repoHash); err != nil {
		fmt.Fprintf(os.Stderr, "  Ruby resolver warning: %v\n", err)
	}
	return nil
}

// runGoResolver runs the in-process Go resolver on Go files in the repo.
func runGoResolver(ctx context.Context, st *store.SQLiteStore, repoPath string, repoHash types.Hash) error {
	// Get all files for this repo.
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("query files: %w", err)
	}

	// Filter to Go files and build FileResults.
	var goFiles []typresolve.FileResult
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}
		if strings.HasSuffix(f.Path, "_test.go") {
			continue // skip test files for now
		}

		absPath := filepath.Join(repoPath, f.Path)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue // file may have been deleted since indexing
		}

		// Get nodes for this file to build ResolverDefs.
		nodes, err := st.NodesByFileHash(ctx, f.FileHash)
		if err != nil {
			continue
		}

		// Convert nodes to edges-style info for the resolver.
		// The resolver primarily needs the defs (via InitWorkspace) not per-file edges.
		goFiles = append(goFiles, typresolve.FileResult{
			Path:     f.Path,
			FileHash: f.FileHash,
			Content:  content,
			Language: "go",
		})
		_ = nodes // defs are collected separately
	}

	if len(goFiles) == 0 {
		return nil
	}

	// Build ResolverDefs from all Go nodes in the repo.
	allNodes, err := queryGoNodes(ctx, st, repoHash)
	if err != nil {
		return fmt.Errorf("query go nodes: %w", err)
	}

	// Create and run the resolver.
	re := typresolve.NewResolverEnricher(st, 8)
	re.Register(goresolve.NewGoResolver())

	// Build defs from nodes for InitWorkspace.
	// The resolver's Run method calls InitWorkspace internally, but it needs
	// FileResult.Edges to extract defs. Since we don't have per-file edges here,
	// we inject defs directly by calling InitWorkspace ourselves and then running.
	resolver := goresolve.NewGoResolver()
	defs := nodesToDefs(allNodes)
	if err := resolver.InitWorkspace(ctx, defs); err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}

	// Resolve files concurrently.
	fmt.Fprintf(os.Stderr, "  In-process Go resolver: %d files...\n", len(goFiles))

	var edgeCount int
	for _, fr := range goFiles {
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

	fmt.Fprintf(os.Stderr, "  In-process Go resolver: %d edges produced\n", edgeCount)
	return nil
}

// queryGoNodes returns all nodes from Go files in the repo.
func queryGoNodes(ctx context.Context, st *store.SQLiteStore, repoHash types.Hash) ([]types.Node, error) {
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return nil, err
	}

	var allNodes []types.Node
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".go") {
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

// runRubyResolver runs the in-process Ruby resolver on Ruby files in the repo.
func runRubyResolver(ctx context.Context, st *store.SQLiteStore, repoPath string, repoHash types.Hash) error {
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("query files: %w", err)
	}

	var rubyFiles []typresolve.FileResult
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".rb") {
			continue
		}
		absPath := filepath.Join(repoPath, f.Path)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		rubyFiles = append(rubyFiles, typresolve.FileResult{
			Path:     f.Path,
			FileHash: f.FileHash,
			Content:  content,
			Language: "ruby",
		})
	}

	if len(rubyFiles) == 0 {
		return nil
	}

	// Build defs from all Ruby nodes.
	allNodes, err := queryNodesByExtension(ctx, st, repoHash, ".rb")
	if err != nil {
		return fmt.Errorf("query ruby nodes: %w", err)
	}

	resolver := rubyresolve.NewRubyResolver()
	defs := nodesToDefs(allNodes)
	if err := resolver.InitWorkspace(ctx, defs); err != nil {
		return fmt.Errorf("init workspace: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  In-process Ruby resolver: %d files...\n", len(rubyFiles))

	var edgeCount int
	for _, fr := range rubyFiles {
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

	fmt.Fprintf(os.Stderr, "  In-process Ruby resolver: %d edges produced\n", edgeCount)
	return nil
}

// queryNodesByExtension returns all nodes from files with the given extension.
func queryNodesByExtension(ctx context.Context, st *store.SQLiteStore, repoHash types.Hash, ext string) ([]types.Node, error) {
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return nil, err
	}

	var allNodes []types.Node
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ext) {
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
