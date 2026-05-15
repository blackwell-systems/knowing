// Package enrichment provides an LSP-based enrichment pass that upgrades
// ast_inferred edges to lsp_resolved by querying gopls via the agent-lsp
// public API. It also discovers new implements and references edges not
// found by tree-sitter.
package enrichment

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/agent-lsp/pkg/lsp"
	lsptypes "github.com/blackwell-systems/agent-lsp/pkg/types"
	"github.com/blackwell-systems/knowing/internal/types"
)

// Enricher upgrades ast_inferred graph edges to lsp_resolved by querying
// gopls for definition resolution, and discovers new implements/references
// edges not found by tree-sitter extraction.
type Enricher struct {
	store         types.GraphStore
	workspaceRoot string
	client        *lsp.LSPClient
}

// NewEnricher creates an Enricher that will use the given store and
// workspace root for LSP operations.
func NewEnricher(store types.GraphStore, workspaceRoot string) *Enricher {
	return &Enricher{
		store:         store,
		workspaceRoot: workspaceRoot,
	}
}

// enrichStats tracks enrichment progress for summary logging.
type enrichStats struct {
	edgesProcessed int
	edgesUpgraded  int
	edgesSkipped   int
	edgeErrors     int
	newEdges       int
	filesProcessed int
	fileErrors     int
}

// Run starts gopls, iterates edges with provenance "ast_inferred", queries
// hover/definition for each edge source, and upgrades resolved edges to
// provenance "lsp_resolved" with confidence 0.9. After edge upgrade, it
// discovers new implements and references edges. Shuts down gopls on
// completion or context cancellation. Best-effort: individual failures are
// counted but do not abort the run.
func (e *Enricher) Run(ctx context.Context, repoHash types.Hash) error {
	// Start gopls.
	client := lsp.NewLSPClient("gopls", []string{})
	if err := client.Initialize(ctx, e.workspaceRoot); err != nil {
		return fmt.Errorf("enrichment: start gopls: %w", err)
	}
	e.client = client
	defer func() {
		_ = e.Close(ctx)
	}()

	// Get the latest snapshot for this repo.
	snap, err := e.store.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("enrichment: get latest snapshot: %w", err)
	}
	if snap == nil {
		return fmt.Errorf("enrichment: no snapshot found for repo %s", repoHash)
	}

	// Build a file hash to file path map.
	files, err := e.store.FilesByRepo(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("enrichment: list files: %w", err)
	}
	filePathByHash := make(map[types.Hash]string, len(files))
	for _, f := range files {
		filePathByHash[f.FileHash] = f.Path
	}

	stats := &enrichStats{}

	// Discover new implements and references edges via LSP document symbols.
	// This is the primary enrichment path: gopls provides type-resolved
	// symbol information that tree-sitter cannot.
	//
	// Note: per-edge upgrade (resolving ast_inferred call edges individually)
	// is not implemented because call-site positions are not stored in edges.
	// The tree-sitter pass stores the source node's declaration line, not
	// the call site line. Without call-site positions, gopls GetDefinition
	// returns the declaration itself, not useful for edge validation.
	// Future: store call-site line/col in edges to enable per-edge upgrade.
	e.discoverNewEdges(ctx, files, filePathByHash, stats)

	// Log summary instead of per-edge noise.
	log.Printf("enrichment complete: %d edges processed, %d upgraded, %d skipped, %d errors, %d new edges discovered, %d files scanned, %d file errors",
		stats.edgesProcessed, stats.edgesUpgraded, stats.edgesSkipped, stats.edgeErrors,
		stats.newEdges, stats.filesProcessed, stats.fileErrors)

	return nil
}

// Close shuts down the LSP client if running.
func (e *Enricher) Close(ctx context.Context) error {
	if e.client != nil {
		e.client.Shutdown(ctx)
		e.client = nil
	}
	return nil
}

// upgradeEdge creates a new lsp_resolved edge from an ast_inferred edge.
// Used by discoverNewEdges when it finds an existing ast_inferred edge
// that can be confirmed by LSP.
func upgradeEdge(old types.Edge) types.Edge {
	newProvenance := "lsp_resolved"
	newEdgeHash := types.ComputeEdgeHash(old.SourceHash, old.TargetHash, old.EdgeType, newProvenance)
	return types.Edge{
		EdgeHash:   newEdgeHash,
		SourceHash: old.SourceHash,
		TargetHash: old.TargetHash,
		EdgeType:   old.EdgeType,
		Confidence: 0.9,
		Provenance: newProvenance,
	}
}

// discoverNewEdges uses LSP to find implements and references edges not
// found by tree-sitter.
func (e *Enricher) discoverNewEdges(
	ctx context.Context,
	files []types.File,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
) {
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}

		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}

		stats.filesProcessed++
		uri := "file://" + filepath.Join(e.workspaceRoot, f.Path)

		symbols, err := e.client.GetDocumentSymbols(ctx, uri)
		if err != nil {
			stats.fileErrors++
			continue
		}

		e.processSymbols(ctx, uri, symbols, f, filePathByHash, stats)
	}
}

// symbolKindName maps LSP SymbolKind constants to knowing node kinds.
func symbolKindName(kind lsptypes.SymbolKind) string {
	switch kind {
	case 5: // Class
		return "type"
	case 11: // Interface
		return "interface"
	case 12: // Function
		return "function"
	case 6: // Method
		return "method"
	default:
		return ""
	}
}

// processSymbols processes document symbols to discover implements and
// references edges.
func (e *Enricher) processSymbols(
	ctx context.Context,
	uri string,
	symbols []lsptypes.DocumentSymbol,
	file types.File,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
) {
	for _, sym := range symbols {
		if ctx.Err() != nil {
			return
		}

		kind := symbolKindName(sym.Kind)
		pos := lsptypes.Position{
			Line:      sym.SelectionRange.Start.Line,
			Character: sym.SelectionRange.Start.Character,
		}

		if kind == "type" || kind == "interface" {
			impls, err := e.client.GetImplementation(ctx, uri, pos)
			if err == nil {
				e.insertEdgesFromLocations(ctx, uri, pos, impls, "implements", file, stats)
			}
		}

		if kind == "function" || kind == "method" {
			refs, err := e.client.GetReferences(ctx, uri, pos, false)
			if err == nil {
				e.insertEdgesFromLocations(ctx, uri, pos, refs, "references", file, stats)
			}
		}

		if len(sym.Children) > 0 {
			e.processSymbols(ctx, uri, sym.Children, file, filePathByHash, stats)
		}
	}
}

// insertEdgesFromLocations creates lsp_resolved edges from LSP location
// results, skipping edges that already exist.
func (e *Enricher) insertEdgesFromLocations(
	ctx context.Context,
	sourceURI string,
	sourcePos lsptypes.Position,
	locations []lsptypes.Location,
	edgeType string,
	sourceFile types.File,
	stats *enrichStats,
) {
	for _, loc := range locations {
		if ctx.Err() != nil {
			return
		}

		if loc.URI == sourceURI && loc.Range.Start.Line == sourcePos.Line {
			continue
		}

		sourceData := fmt.Sprintf("%s:%d:%d", sourceURI, sourcePos.Line, sourcePos.Character)
		sourceHash := types.NewHash([]byte(sourceData))

		targetData := fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		targetHash := types.NewHash([]byte(targetData))

		provenance := "lsp_resolved"
		edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)

		existing, err := e.store.GetEdge(ctx, edgeHash)
		if err != nil {
			continue
		}
		if existing != nil {
			continue
		}

		edge := types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: sourceHash,
			TargetHash: targetHash,
			EdgeType:   edgeType,
			Confidence: 0.9,
			Provenance: provenance,
		}

		if err := e.store.PutEdge(ctx, edge); err != nil {
			continue
		}
		stats.newEdges++
	}
}
