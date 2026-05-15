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

// Run starts gopls, iterates edges with provenance "ast_inferred", queries
// hover/definition for each edge source, and upgrades resolved edges to
// provenance "lsp_resolved" with confidence 0.9. After edge upgrade, it
// discovers new implements and references edges. Shuts down gopls on
// completion or context cancellation. Best-effort: individual failures are
// logged but do not abort the run.
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

	// Collect all ast_inferred edges by iterating nodes reachable from repo files.
	astEdges, err := e.collectASTInferredEdges(ctx, repoHash, filePathByHash)
	if err != nil {
		return fmt.Errorf("enrichment: collect edges: %w", err)
	}

	// Upgrade ast_inferred edges via LSP.
	e.upgradeEdges(ctx, astEdges, filePathByHash)

	// Discover new edges via LSP.
	e.discoverNewEdges(ctx, files, filePathByHash)

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

// edgeWithSource pairs an edge with the source node for LSP lookups.
type edgeWithSource struct {
	edge types.Edge
	node types.Node
}

// collectASTInferredEdges finds all edges with provenance "ast_inferred"
// for nodes belonging to the given repo's files.
func (e *Enricher) collectASTInferredEdges(
	ctx context.Context,
	repoHash types.Hash,
	filePathByHash map[types.Hash]string,
) ([]edgeWithSource, error) {
	// Get all nodes for this repo by querying nodes whose FileHash is in our file set.
	// Since there's no NodesByFileHash, use NodesByName with repo URL prefix.
	repo, err := e.store.GetRepo(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	if repo == nil {
		return nil, fmt.Errorf("repo not found: %s", repoHash)
	}

	nodes, err := e.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	var result []edgeWithSource
	for _, node := range nodes {
		// Only process nodes belonging to files in this repo.
		if _, ok := filePathByHash[node.FileHash]; !ok {
			continue
		}

		edges, err := e.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			log.Printf("enrichment: edges from %s: %v", node.QualifiedName, err)
			continue
		}

		for _, edge := range edges {
			if edge.Provenance == "ast_inferred" {
				result = append(result, edgeWithSource{edge: edge, node: node})
			}
		}
	}

	return result, nil
}

// upgradeEdge creates a new lsp_resolved edge from an ast_inferred edge.
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

// upgradeEdges attempts to resolve each ast_inferred edge via LSP and
// upgrades successfully resolved edges.
func (e *Enricher) upgradeEdges(
	ctx context.Context,
	edges []edgeWithSource,
	filePathByHash map[types.Hash]string,
) {
	for _, ews := range edges {
		if ctx.Err() != nil {
			return
		}

		filePath, ok := filePathByHash[ews.node.FileHash]
		if !ok {
			log.Printf("enrichment: no file path for node %s", ews.node.QualifiedName)
			continue
		}

		uri := "file://" + filepath.Join(e.workspaceRoot, filePath)
		// Node.Line is 1-indexed; LSP Position.Line is 0-indexed.
		pos := lsptypes.Position{
			Line:      ews.node.Line - 1,
			Character: 0,
		}

		// Try to resolve the definition.
		locs, err := e.client.GetDefinition(ctx, uri, pos)
		if err != nil {
			log.Printf("enrichment: definition for %s: %v", ews.node.QualifiedName, err)
			continue
		}
		if len(locs) == 0 {
			// Could not resolve; leave edge unchanged.
			continue
		}

		// Definition resolved: delete old edge and insert upgraded edge.
		if err := e.store.DeleteEdge(ctx, ews.edge.EdgeHash); err != nil {
			log.Printf("enrichment: delete edge %s: %v", ews.edge.EdgeHash, err)
			continue
		}

		newEdge := upgradeEdge(ews.edge)
		if err := e.store.PutEdge(ctx, newEdge); err != nil {
			log.Printf("enrichment: put edge %s: %v", newEdge.EdgeHash, err)
			continue
		}
	}
}

// discoverNewEdges uses LSP to find implements and references edges not
// found by tree-sitter.
func (e *Enricher) discoverNewEdges(
	ctx context.Context,
	files []types.File,
	filePathByHash map[types.Hash]string,
) {
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}

		if !strings.HasSuffix(f.Path, ".go") {
			continue
		}

		uri := "file://" + filepath.Join(e.workspaceRoot, f.Path)

		symbols, err := e.client.GetDocumentSymbols(ctx, uri)
		if err != nil {
			log.Printf("enrichment: document symbols for %s: %v", f.Path, err)
			continue
		}

		e.processSymbols(ctx, uri, symbols, f, filePathByHash)
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

		// For type/interface nodes, query implementations.
		if kind == "type" || kind == "interface" {
			impls, err := e.client.GetImplementation(ctx, uri, pos)
			if err != nil {
				log.Printf("enrichment: implementations for %s: %v", sym.Name, err)
			} else {
				e.insertEdgesFromLocations(ctx, uri, pos, impls, "implements", file)
			}
		}

		// For function/method nodes, query references.
		if kind == "function" || kind == "method" {
			refs, err := e.client.GetReferences(ctx, uri, pos, false)
			if err != nil {
				log.Printf("enrichment: references for %s: %v", sym.Name, err)
			} else {
				e.insertEdgesFromLocations(ctx, uri, pos, refs, "references", file)
			}
		}

		// Recurse into children (e.g., methods inside a type).
		if len(sym.Children) > 0 {
			e.processSymbols(ctx, uri, sym.Children, file, filePathByHash)
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
) {
	for _, loc := range locations {
		if ctx.Err() != nil {
			return
		}

		// Skip self-references.
		if loc.URI == sourceURI && loc.Range.Start.Line == sourcePos.Line {
			continue
		}

		// Compute source and target hashes based on positions.
		// We use a synthetic hash since we don't have the full node identity.
		sourceData := fmt.Sprintf("%s:%d:%d", sourceURI, sourcePos.Line, sourcePos.Character)
		sourceHash := types.NewHash([]byte(sourceData))

		targetData := fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		targetHash := types.NewHash([]byte(targetData))

		provenance := "lsp_resolved"
		edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)

		// Check if edge already exists.
		existing, err := e.store.GetEdge(ctx, edgeHash)
		if err != nil {
			log.Printf("enrichment: check edge: %v", err)
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
			log.Printf("enrichment: put new edge: %v", err)
			continue
		}
	}
}
