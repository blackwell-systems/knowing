// Package typresolve defines the Resolver interface for per-language
// in-process type resolution and the ResolverEnricher orchestrator that
// dispatches resolution concurrently across files. Language-specific
// resolvers (Go, Python, TypeScript, etc.) implement the Resolver
// interface; the enricher groups files by language, initializes each
// resolver's workspace, and fans out file resolution with bounded
// concurrency.
package typresolve

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ProvenanceResolverResolved is the provenance string for edges produced
// by in-process type resolution, distinct from "lsp_resolved" edges
// produced by the external LSP enricher.
const ProvenanceResolverResolved = "resolver_resolved"

// ResolverConfidence is the default confidence for resolver-produced edges.
const ResolverConfidence = 0.9

// Resolver is the contract that per-language in-process type resolvers
// implement. Language() identifies the resolver; InitWorkspace builds a
// cross-file type registry from extracted definitions (called once, after
// which the registry is read-only); ResolveFile resolves a single file
// and is safe for concurrent use.
type Resolver interface {
	// Language returns the language identifier (e.g. "go", "python").
	Language() string

	// InitWorkspace builds the cross-file type registry from all extracted
	// definitions. Called once before any ResolveFile calls. The registry
	// must be read-only after this returns.
	InitWorkspace(ctx context.Context, defs []ResolverDef) error

	// ResolveFile resolves type references in a single file. Thread-safe;
	// called concurrently across files. Returns edges with provenance
	// ProvenanceResolverResolved and confidence ResolverConfidence.
	ResolveFile(ctx context.Context, opts ResolveFileOpts) ([]types.Edge, error)
}

// ResolverDef contains definition metadata collected from extraction,
// used to build the per-language type registry in InitWorkspace.
type ResolverDef struct {
	QualifiedName string
	Kind          string // "function", "type", "method", etc.
	Signature     string
	FilePath      string
	PackagePath   string
}

// ResolveFileOpts contains all inputs for resolving a single file.
type ResolveFileOpts struct {
	FilePath   string
	FileHash   types.Hash
	Content    []byte
	ParsedTree types.ParsedTree
	Edges      []types.Edge
}

// FileResult wraps extraction output for a single file, used as input
// to the ResolverEnricher.
type FileResult struct {
	Path       string
	FileHash   types.Hash
	Content    []byte
	ParsedTree types.ParsedTree
	Edges      []types.Edge
	Language   string
}

// ResolverEnricher orchestrates per-language type resolution. It groups
// files by language, initializes each registered resolver, and resolves
// files concurrently using bounded concurrency with a single DB-writer
// goroutine.
type ResolverEnricher struct {
	store       types.GraphStore
	resolvers   map[string]Resolver
	concurrency int
}

// NewResolverEnricher creates a ResolverEnricher backed by the given store.
// If concurrency is 0 or negative, it defaults to 8.
func NewResolverEnricher(store types.GraphStore, concurrency int) *ResolverEnricher {
	if concurrency <= 0 {
		concurrency = 8
	}
	return &ResolverEnricher{
		store:       store,
		resolvers:   make(map[string]Resolver),
		concurrency: concurrency,
	}
}

// Register adds a language-specific resolver. Subsequent calls with the
// same Language() value overwrite the previous registration.
func (re *ResolverEnricher) Register(r Resolver) {
	re.resolvers[r.Language()] = r
}

// HasResolver reports whether a resolver is registered for the given language.
func (re *ResolverEnricher) HasResolver(language string) bool {
	_, ok := re.resolvers[language]
	return ok
}

// Run executes type resolution for all file results. It groups files by
// language, then for each language with a registered resolver:
//  1. Collects ResolverDef entries from the file results
//  2. Calls InitWorkspace with the collected definitions
//  3. Resolves files concurrently via a channel semaphore
//  4. Writes edges through a single writer goroutine calling store.PutEdge
//
// Errors in individual files or languages are logged but do not abort the
// entire run.
func (re *ResolverEnricher) Run(ctx context.Context, repoHash types.Hash, fileResults []FileResult) error {
	// Group files by language.
	byLang := make(map[string][]FileResult)
	for _, fr := range fileResults {
		if fr.Language == "" {
			continue
		}
		byLang[fr.Language] = append(byLang[fr.Language], fr)
	}

	for lang, files := range byLang {
		resolver, ok := re.resolvers[lang]
		if !ok {
			continue
		}

		if err := re.runLanguage(ctx, resolver, files); err != nil {
			log.Printf("typresolve: language %s: %v", lang, err)
		}
	}

	return nil
}

// runLanguage handles a single language: init workspace, resolve files
// concurrently, write edges via a single writer goroutine.
func (re *ResolverEnricher) runLanguage(ctx context.Context, resolver Resolver, files []FileResult) error {
	lang := resolver.Language()

	// Collect definitions from file results.
	defs := collectDefs(files)

	// Initialize workspace.
	if err := resolver.InitWorkspace(ctx, defs); err != nil {
		return fmt.Errorf("InitWorkspace failed: %w", err)
	}

	// Edge channel for the single writer goroutine.
	edgeCh := make(chan types.Edge, 64)

	// Writer goroutine: reads edges from channel and writes to store.
	var writerWg sync.WaitGroup
	var writeErrors int
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		for edge := range edgeCh {
			if err := re.store.PutEdge(ctx, edge); err != nil {
				writeErrors++
				if writeErrors <= 5 {
					log.Printf("typresolve: PutEdge error (%s): %v", lang, err)
				}
			}
		}
	}()

	// Resolve files concurrently with bounded concurrency.
	sem := make(chan struct{}, re.concurrency)
	var wg sync.WaitGroup
	var filesResolved, edgesProduced, fileErrors int64
	var mu sync.Mutex

	for _, fr := range files {
		select {
		case <-ctx.Done():
			break
		case sem <- struct{}{}:
		}

		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(fr FileResult) {
			defer func() {
				<-sem
				wg.Done()
			}()

			opts := ResolveFileOpts{
				FilePath:   fr.Path,
				FileHash:   fr.FileHash,
				Content:    fr.Content,
				ParsedTree: fr.ParsedTree,
				Edges:      fr.Edges,
			}

			edges, err := resolver.ResolveFile(ctx, opts)
			if err != nil {
				mu.Lock()
				fileErrors++
				mu.Unlock()
				log.Printf("typresolve: ResolveFile error (%s) %s: %v", lang, fr.Path, err)
				return
			}

			for _, e := range edges {
				edgeCh <- e
			}

			mu.Lock()
			filesResolved++
			edgesProduced += int64(len(edges))
			mu.Unlock()
		}(fr)
	}

	wg.Wait()
	close(edgeCh)
	writerWg.Wait()

	log.Printf("typresolve: %s: %d files resolved, %d edges produced, %d file errors, %d write errors",
		lang, filesResolved, edgesProduced, fileErrors, writeErrors)

	return nil
}

// collectDefs extracts ResolverDef entries from file results. For now,
// this uses a simple conversion: one def per unique (EdgeType, FilePath)
// pair from the file's edges. Language-specific resolvers will extract
// richer type information during InitWorkspace using the parsed tree and
// content directly.
func collectDefs(files []FileResult) []ResolverDef {
	var defs []ResolverDef
	seen := make(map[string]bool)
	for _, fr := range files {
		for _, e := range fr.Edges {
			key := e.EdgeType + ":" + fr.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			defs = append(defs, ResolverDef{
				Kind:     e.EdgeType,
				FilePath: fr.Path,
			})
		}
	}
	return defs
}
