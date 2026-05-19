// Package enrichment provides an LSP-based enrichment pass that upgrades
// ast_inferred edges to lsp_resolved by querying language servers via the
// agent-lsp public API. It also discovers new implements and references
// edges not found by tree-sitter.
//
// The enrichment pipeline works in three phases per language server:
//
//  1. Open files: source files matching the server's extensions are sent
//     via textDocument/didOpen so the server has full workspace knowledge.
//     Files must be opened before any queries because most servers index lazily.
//
//  2. Upgrade call edges: for each ast_inferred edge that has call-site
//     position data, query GetDefinition at (file, line, col). If the
//     server returns a location, the edge is confirmed and upgraded to
//     lsp_resolved with confidence 0.9. The original ast_inferred edge
//     is deleted first because provenance is part of the edge hash.
//
//  3. Discover new edges: for each opened file, retrieve document symbols
//     and query GetImplementation (for types/interfaces) and GetReferences
//     (for functions/methods) to find edges that tree-sitter missed.
//
// Supported language servers are auto-detected (gopls, typescript-language-server,
// pylsp/pyright, rust-analyzer, jdtls, OmniSharp) or configured via knowing-lsp.json.
//
// All operations are best-effort: individual failures are counted but do
// not abort the run. The enricher is designed to run in the background
// after each index run.
package enrichment

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackwell-systems/agent-lsp/pkg/lsp"
	lsptypes "github.com/blackwell-systems/agent-lsp/pkg/types"
	"github.com/blackwell-systems/knowing/internal/roster"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// Enricher upgrades ast_inferred graph edges to lsp_resolved by querying
// language servers for definition resolution, and discovers new implements/references
// edges not found by tree-sitter extraction. Supports multiple languages via LSPConfig.
type Enricher struct {
	store         types.GraphStore
	workspaceRoot string
	client        *lsp.LSPClient
	lspConfig     *LSPConfig // multi-language server config (nil = auto-detect)
}

// NewEnricher creates an Enricher that will use the given store and
// workspace root for LSP operations. Auto-detects available language servers.
func NewEnricher(store types.GraphStore, workspaceRoot string) *Enricher {
	return &Enricher{
		store:         store,
		workspaceRoot: workspaceRoot,
	}
}

// SetLSPConfig overrides auto-detection with an explicit server configuration.
func (e *Enricher) SetLSPConfig(cfg *LSPConfig) {
	e.lspConfig = cfg
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
	return e.runFiltered(ctx, repoHash, nil)
}

// RunScoped runs enrichment only for edges originating from the given file
// paths. If changedFiles is empty or nil, it falls back to full Run behavior.
func (e *Enricher) RunScoped(ctx context.Context, repoHash types.Hash, changedFiles []string) error {
	if len(changedFiles) == 0 {
		return e.runFiltered(ctx, repoHash, nil)
	}
	changedSet := make(map[string]struct{}, len(changedFiles))
	for _, f := range changedFiles {
		changedSet[f] = struct{}{}
	}
	fileFilter := func(path string) bool {
		_, ok := changedSet[path]
		return ok
	}
	return e.runFiltered(ctx, repoHash, fileFilter)
}

// runFiltered is the core enrichment logic shared by Run and RunScoped.
// When fileFilter is nil, all files are processed (full enrichment).
// When non-nil, only files passing the filter are processed.
// Iterates over all configured/detected language servers.
func (e *Enricher) runFiltered(ctx context.Context, repoHash types.Hash, fileFilter func(string) bool) error {
	// Determine which language servers to use.
	cfg := e.lspConfig
	if cfg == nil {
		cfg = DetectLSPServers(e.workspaceRoot)
	}
	if len(cfg.Servers) == 0 {
		log.Printf("enrichment: no language servers detected for %s", e.workspaceRoot)
		return nil
	}

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

	// Run enrichment for each detected language server.
	for _, serverCfg := range cfg.Servers {
		if ctx.Err() != nil {
			break
		}
		e.runForServer(ctx, serverCfg, repoHash, files, filePathByHash, fileFilter)
	}

	return nil
}

// runForServer runs enrichment using a single language server.
func (e *Enricher) runForServer(ctx context.Context, serverCfg LSPServerConfig, repoHash types.Hash,
	files []types.File, filePathByHash map[types.Hash]string, fileFilter func(string) bool) {

	if len(serverCfg.Command) == 0 {
		return
	}

	// Start the language server.
	args := []string{}
	if len(serverCfg.Command) > 1 {
		args = serverCfg.Command[1:]
	}
	client := lsp.NewLSPClient(serverCfg.Command[0], args)
	if err := client.Initialize(ctx, e.workspaceRoot); err != nil {
		log.Printf("enrichment: start %s: %v", serverCfg.Command[0], err)
		return
	}
	e.client = client
	defer func() {
		_ = e.Close(ctx)
	}()

	stats := &enrichStats{}

	// Build a file filter that combines the caller's filter with the language extension filter.
	langFilter := func(path string) bool {
		if fileFilter != nil && !fileFilter(path) {
			return false
		}
		return serverCfg.matchesFile(path)
	}

	// Open files for this language.
	e.openFilesForLanguage(ctx, files, langFilter, serverCfg.LanguageID)

	// Wait for the workspace to be indexed. Some servers (jdtls) import the
	// project asynchronously via $/progress and return empty results until
	// indexing completes. The 120s timeout accommodates large Gradle/Maven
	// projects. Servers that index synchronously (gopls, pyright, tsserver)
	// return immediately since they have no active progress tokens.
	client.WaitForWorkspaceReadyTimeout(ctx, 120*time.Second)

	// Upgrade ast_inferred call edges that have call-site positions.
	e.upgradeCallEdges(ctx, repoHash, filePathByHash, stats, langFilter)

	// Discover new implements and references edges via LSP document symbols.
	e.discoverNewEdges(ctx, files, filePathByHash, stats, langFilter)

	// Close all opened documents.
	e.closeFilesForLanguage(ctx, files, langFilter)

	// Log summary.
	log.Printf("enrichment complete (%s): %d edges processed, %d upgraded, %d skipped, %d errors, %d new edges discovered, %d files scanned, %d file errors",
		serverCfg.LanguageID,
		stats.edgesProcessed, stats.edgesUpgraded, stats.edgesSkipped, stats.edgeErrors,
		stats.newEdges, stats.filesProcessed, stats.fileErrors)
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

// upgradeCallEdges finds ast_inferred edges with call-site positions and
// queries the language server's GetDefinition at those positions to confirm targets.
// Successfully resolved edges are upgraded to lsp_resolved with confidence 0.9.
func (e *Enricher) upgradeCallEdges(
	ctx context.Context,
	repoHash types.Hash,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
	fileFilter func(string) bool,
) {
	repo, err := e.store.GetRepo(ctx, repoHash)
	if err != nil || repo == nil {
		return
	}

	nodes, err := e.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return
	}

	for _, node := range nodes {
		if ctx.Err() != nil {
			return
		}
		if _, ok := filePathByHash[node.FileHash]; !ok {
			continue
		}

		edges, err := e.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			continue
		}

		for _, edge := range edges {
			if edge.Provenance != "ast_inferred" {
				continue
			}
			if edge.CallSiteLine == 0 || edge.CallSiteFile == "" {
				continue
			}
			if fileFilter != nil && !fileFilter(edge.CallSiteFile) {
				continue
			}

			stats.edgesProcessed++

			uri := "file://" + filepath.Join(e.workspaceRoot, edge.CallSiteFile)
			// Convert from knowing's 1-indexed lines to LSP's 0-indexed lines.
			// Column is already 0-indexed (matching tree-sitter and LSP conventions).
			pos := lsptypes.Position{
				Line:      edge.CallSiteLine - 1,
				Character: edge.CallSiteCol,
			}

			locs, err := e.client.GetDefinition(ctx, uri, pos)
			if err != nil {
				stats.edgeErrors++
				continue
			}
			if len(locs) == 0 {
				stats.edgesSkipped++
				continue
			}

			// gopls confirmed a definition exists at this call site.
			// Try to retarget the edge to the correct node hash by matching
			// the definition location to a node in the database. This fixes
			// cross-repo method call edges where tree-sitter could not
			// determine the correct target package or kind.
			retargetedEdge := edge
			if defNode := e.resolveDefinitionToNode(ctx, locs[0], repoHash); defNode != nil {
				retargetedEdge.TargetHash = defNode.NodeHash
			}

			// Delete the old ast_inferred edge and create a new lsp_resolved
			// edge. We must delete first because the provenance string is part
			// of the edge hash; changing provenance produces a new hash.
			if err := e.store.DeleteEdge(ctx, edge.EdgeHash); err != nil {
				stats.edgeErrors++
				continue
			}

			upgraded := upgradeEdge(retargetedEdge)
			upgraded.CallSiteLine = edge.CallSiteLine
			upgraded.CallSiteCol = edge.CallSiteCol
			upgraded.CallSiteFile = edge.CallSiteFile
			if err := e.store.PutEdge(ctx, upgraded); err != nil {
				stats.edgeErrors++
				continue
			}
			stats.edgesUpgraded++
		}
	}
}

// resolveDefinitionToNode tries to match an LSP definition location to a node
// in the database. It converts the LSP URI to a workspace-relative path, looks
// up nodes at that file, and returns the best match by line number. This allows
// the enricher to retarget edges whose tree-sitter-generated target hashes are
// wrong (e.g., method calls with incorrect package or kind).
//
// For cross-repo definitions (where the file is outside the current workspace),
// the method checks the global roster to find the owning repo and queries that
// repo's database.
//
// Returns nil if no matching node is found (the edge will keep its original
// target hash).
func (e *Enricher) resolveDefinitionToNode(ctx context.Context, loc lsptypes.Location, repoHash types.Hash) *types.Node {
	absPath := strings.TrimPrefix(loc.URI, "file://")
	defLine := loc.Range.Start.Line + 1

	// Try the local workspace first.
	if strings.HasPrefix(absPath, e.workspaceRoot) {
		relPath := strings.TrimPrefix(absPath, e.workspaceRoot)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath != "" {
			if node := e.findNodeByFileLine(ctx, e.store, repoHash, relPath, defLine); node != nil {
				return node
			}
		}
	}

	// Definition is outside the workspace (cross-repo). Check the roster to
	// find which repo owns this file and query its database.
	return e.resolveDefinitionViaRoster(ctx, absPath, defLine)
}

// resolveDefinitionViaRoster checks the global roster to find which repo
// contains the given absolute file path, then queries that repo's database
// for a node at the specified line.
func (e *Enricher) resolveDefinitionViaRoster(ctx context.Context, absPath string, defLine int) *types.Node {
	r, err := roster.Load()
	if err != nil || r == nil {
		return nil
	}

	for _, entry := range r.Repos {
		if entry.Path == "" || entry.DB == "" {
			continue
		}
		// Check if the file is under this repo's path.
		if !strings.HasPrefix(absPath, entry.Path) {
			continue
		}
		relPath := strings.TrimPrefix(absPath, entry.Path)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" {
			continue
		}

		// Open the repo's database and look for the node.
		otherStore, err := store.NewSQLiteStore(entry.DB)
		if err != nil {
			continue
		}

		// Find the repo hash in this database.
		otherRepoHash := types.NewHash([]byte(entry.URL))
		node := e.findNodeByFileLine(ctx, otherStore, otherRepoHash, relPath, defLine)
		otherStore.Close()
		if node != nil {
			return node
		}
	}
	return nil
}

// findNodeByFileLine looks up nodes in a store by file path and returns
// the node closest to the given line number (within a 2-line tolerance).
func (e *Enricher) findNodeByFileLine(ctx context.Context, st types.GraphStore, repoHash types.Hash, relPath string, defLine int) *types.Node {
	nodes, err := st.NodesByFilePath(ctx, repoHash, relPath)
	if err != nil || len(nodes) == 0 {
		return nil
	}

	var best *types.Node
	bestDist := int(^uint(0) >> 1) // max int
	for i := range nodes {
		dist := defLine - nodes[i].Line
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			best = &nodes[i]
		}
	}

	// Only accept matches within 2 lines (LSP may point to the keyword line
	// while the node's Line field points to the name identifier).
	if best != nil && bestDist <= 2 {
		return best
	}
	return nil
}

// openFilesForLanguage opens files matching the filter via textDocument/didOpen.
// Language-agnostic: works for any language server.
func (e *Enricher) openFilesForLanguage(ctx context.Context, files []types.File, filter func(string) bool, languageID string) {
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}
		if !filter(f.Path) {
			continue
		}
		// Skip test files for all languages.
		if isTestFile(f.Path) {
			continue
		}
		absPath := filepath.Join(e.workspaceRoot, f.Path)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		uri := "file://" + absPath
		_ = e.client.OpenDocument(ctx, uri, string(content), languageID)
	}
}

// closeFilesForLanguage closes files matching the filter.
func (e *Enricher) closeFilesForLanguage(ctx context.Context, files []types.File, filter func(string) bool) {
	for _, f := range files {
		if filter(f.Path) && !isTestFile(f.Path) {
			uri := "file://" + filepath.Join(e.workspaceRoot, f.Path)
			_ = e.client.CloseDocument(ctx, uri)
		}
	}
}

// isTestFile returns true for test files across common languages.
func isTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go") ||
		strings.HasSuffix(path, ".test.ts") ||
		strings.HasSuffix(path, ".test.js") ||
		strings.HasSuffix(path, ".spec.ts") ||
		strings.HasSuffix(path, ".spec.js") ||
		strings.HasSuffix(path, "_test.py") ||
		strings.HasSuffix(path, "_test.rs") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/__tests__/")
}

// discoverNewEdges uses LSP to find implements and references edges not
// found by tree-sitter. Assumes files are already opened via openFilesForLanguage.
// The fileFilter is the combined language+scope filter from runForServer,
// so it already handles extension matching and test file exclusion.
func (e *Enricher) discoverNewEdges(
	ctx context.Context,
	files []types.File,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
	fileFilter func(string) bool,
) {
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}
		if fileFilter != nil && !fileFilter(f.Path) {
			continue
		}
		// Skip test files (redundant when called from runForServer since
		// openFilesForLanguage already skips them, but defensive for direct callers).
		if isTestFile(f.Path) {
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

// symbolKindName maps LSP SymbolKind numeric constants to knowing's node
// kind strings. Only types that we want to discover new edges for are
// mapped; all others return "" and are skipped.
func symbolKindName(kind lsptypes.SymbolKind) string {
	switch kind {
	case 5: // LSP SymbolKind.Class
		return "type"
	case 11: // LSP SymbolKind.Interface
		return "interface"
	case 12: // LSP SymbolKind.Function
		return "function"
	case 6: // LSP SymbolKind.Method
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
	// Read source lines for name-position correction. Some language servers
	// (e.g. pyright) set SelectionRange.Start to the keyword (def/class)
	// rather than the symbol name, causing GetReferences to return nothing.
	var sourceLines []string
	absPath := strings.TrimPrefix(uri, "file://")
	if data, err := os.ReadFile(absPath); err == nil {
		sourceLines = strings.Split(string(data), "\n")
	}

	e.processSymbolsWithSource(ctx, uri, symbols, file, filePathByHash, stats, sourceLines)
}

// processSymbolsWithSource is the recursive implementation of processSymbols
// that carries parsed source lines for position correction.
func (e *Enricher) processSymbolsWithSource(
	ctx context.Context,
	uri string,
	symbols []lsptypes.DocumentSymbol,
	file types.File,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
	sourceLines []string,
) {
	for _, sym := range symbols {
		if ctx.Err() != nil {
			return
		}

		kind := symbolKindName(sym.Kind)
		pos := resolveNamePosition(sym, sourceLines)

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
			e.processSymbolsWithSource(ctx, uri, sym.Children, file, filePathByHash, stats, sourceLines)
		}
	}
}

// resolveNamePosition returns the LSP position of the symbol's name on its
// declaration line. Some language servers (pyright, pylsp) set the
// SelectionRange to the keyword (class, def, async def) rather than the
// identifier. When the symbol name appears on the SelectionRange line at a
// different column, we use that column instead.
func resolveNamePosition(sym lsptypes.DocumentSymbol, sourceLines []string) lsptypes.Position {
	line := sym.SelectionRange.Start.Line
	col := sym.SelectionRange.Start.Character

	if len(sym.Name) > 0 && line < len(sourceLines) {
		lineText := sourceLines[line]
		idx := strings.Index(lineText, sym.Name)
		if idx >= 0 && idx != col {
			col = idx
		}
	}

	return lsptypes.Position{
		Line:      line,
		Character: col,
	}
}

// insertEdgesFromLocations creates lsp_resolved edges from LSP location
// results, skipping edges that already exist. Source and target hashes are
// computed from the LSP URIs and positions (not from qualified names),
// because we may not have a matching Node record for every location.
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
