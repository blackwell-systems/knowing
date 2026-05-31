package typresolve

import (
	"context"
	"log"

	"github.com/blackwell-systems/knowing/internal/types"
)

// LspEnricher is the interface the existing enrichment.Enricher satisfies.
// Defined here rather than importing internal/enrichment to avoid circular
// dependencies. The enricher's Run method has the signature:
//
//	Run(ctx context.Context, repoHash types.Hash) error
type LspEnricher interface {
	Run(ctx context.Context, repoHash types.Hash) error
}

// LanguageStats reports the routing decision for a single language.
type LanguageStats struct {
	Language    string
	FileCount   int
	UseResolver bool
}

// RouteStats returns per-language routing decisions without executing.
// Useful for logging and testing.
func RouteStats(resolverEnricher *ResolverEnricher, fileResults []FileResult) []LanguageStats {
	counts := make(map[string]int)
	order := make([]string, 0)
	for _, fr := range fileResults {
		if fr.Language == "" {
			continue
		}
		if counts[fr.Language] == 0 {
			order = append(order, fr.Language)
		}
		counts[fr.Language]++
	}

	stats := make([]LanguageStats, 0, len(order))
	for _, lang := range order {
		stats = append(stats, LanguageStats{
			Language:    lang,
			FileCount:   counts[lang],
			UseResolver: resolverEnricher.HasResolver(lang),
		})
	}
	return stats
}

// RouteEnrichment dispatches file results to either the in-process resolver
// or the external LSP enricher based on which languages have registered
// resolvers. Languages with a resolver go through resolverEnricher.Run;
// languages without one fall back to lspEnricher.Run (if non-nil).
//
// Resolver errors do not prevent the LSP fallback from running. The first
// error encountered (resolver or LSP) is returned.
func RouteEnrichment(ctx context.Context, store types.GraphStore,
	workspaceRoot string, repoHash types.Hash,
	resolverEnricher *ResolverEnricher,
	lspEnricher LspEnricher,
	fileResults []FileResult) error {

	if len(fileResults) == 0 {
		return nil
	}

	// Partition files by language and routing decision.
	var resolverFiles []FileResult
	needsLSP := false
	stats := RouteStats(resolverEnricher, fileResults)

	for _, s := range stats {
		if s.UseResolver {
			log.Printf("typresolve: routing %s to in-process resolver (%d files)", s.Language, s.FileCount)
		} else {
			log.Printf("typresolve: routing %s to LSP fallback (%d files)", s.Language, s.FileCount)
			needsLSP = true
		}
	}

	// Collect files destined for the resolver.
	for _, fr := range fileResults {
		if fr.Language != "" && resolverEnricher.HasResolver(fr.Language) {
			resolverFiles = append(resolverFiles, fr)
		}
	}

	var firstErr error

	// Run resolver for languages that have one.
	if len(resolverFiles) > 0 {
		if err := resolverEnricher.Run(ctx, repoHash, resolverFiles); err != nil {
			log.Printf("typresolve: resolver error: %v", err)
			firstErr = err
		}
	}

	// Run LSP fallback for languages without a resolver.
	if needsLSP {
		if lspEnricher == nil {
			log.Printf("typresolve: LSP fallback needed but no LspEnricher provided, skipping")
		} else {
			if err := lspEnricher.Run(ctx, repoHash); err != nil {
				log.Printf("typresolve: LSP enricher error: %v", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	return firstErr
}
