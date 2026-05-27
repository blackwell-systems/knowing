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
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	client        *lsp.LSPClient // legacy: kept for Close() compatibility; prefer passing client explicitly
	lspConfig     *LSPConfig     // multi-language server config (nil = auto-detect)
	concurrency   int            // max parallel LSP requests (default 8)
	symbolTimeout time.Duration  // per-symbol LSP call timeout (default: DefaultSymbolTimeout)
	writeMu       sync.Mutex     // serializes DB writes from concurrent discover workers
}

// NewEnricher creates an Enricher that will use the given store and
// workspace root for LSP operations. Auto-detects available language servers.
// The workspace root is resolved to an absolute path (required for LSP URIs).
func NewEnricher(store types.GraphStore, workspaceRoot string) *Enricher {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absRoot = workspaceRoot
	}
	return &Enricher{
		store:         store,
		workspaceRoot: absRoot,
		concurrency:   8,
		symbolTimeout: DefaultSymbolTimeout,
	}
}

// SetSymbolTimeout sets the per-symbol LSP call timeout.
func (e *Enricher) SetSymbolTimeout(d time.Duration) {
	e.symbolTimeout = d
}

// SetConcurrency sets the maximum number of parallel LSP requests.
func (e *Enricher) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	e.concurrency = n
}

// SetLSPConfig overrides auto-detection with an explicit server configuration.
func (e *Enricher) SetLSPConfig(cfg *LSPConfig) {
	e.lspConfig = cfg
}

// enrichStats tracks enrichment progress for summary logging.
// Fields are accessed atomically from concurrent workers.
type enrichStats struct {
	edgesProcessed atomic.Int64
	edgesUpgraded  atomic.Int64
	edgesSkipped   atomic.Int64
	edgeErrors     atomic.Int64
	newEdges       atomic.Int64
	filesProcessed atomic.Int64
	fileErrors     atomic.Int64
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

	// Create phantom external nodes for any edge targets that don't exist
	// in the local database. This covers edges created or retargeted during
	// enrichment (lsp_resolved calls to stdlib, new reference edges, etc.).
	if err := e.createPhantomNodes(ctx, repoHash); err != nil {
		log.Printf("enrichment: phantom nodes: %v", err)
	}

	return nil
}

// runForServer runs enrichment using a single language server. For Go servers
// with multiple modules (go.work), it delegates to runMultiModule which spawns
// one gopls per module. For all other cases, a single server instance is used.
func (e *Enricher) runForServer(ctx context.Context, serverCfg LSPServerConfig, repoHash types.Hash,
	files []types.File, filePathByHash map[types.Hash]string, fileFilter func(string) bool) {

	if len(serverCfg.Command) == 0 {
		return
	}

	// For Go language servers, check for multi-module workspace.
	if serverCfg.LanguageID == "go" {
		modules, err := DiscoverModules(e.workspaceRoot)
		if err != nil {
			log.Printf("enrichment: discover modules: %v", err)
			// Fall through to single-server path on error.
		} else if len(modules) > 1 {
			e.runMultiModule(ctx, serverCfg, repoHash, files, filePathByHash, fileFilter, modules)
			return
		}
	}

	// Single-server path: start one LSP instance for the workspace root.
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

	e.runForServerWithClient(ctx, client, serverCfg, repoHash, files, filePathByHash, fileFilter)
}

// runForServerWithClient runs the enrichment pipeline using a pre-created LSP client.
// This is the core logic extracted from runForServer to allow reuse in multi-module mode.
func (e *Enricher) runForServerWithClient(ctx context.Context, client *lsp.LSPClient,
	serverCfg LSPServerConfig, repoHash types.Hash, files []types.File,
	filePathByHash map[types.Hash]string, fileFilter func(string) bool) {

	stats := &enrichStats{}

	// Build a file filter that combines the caller's filter with the language extension filter.
	langFilter := func(path string) bool {
		if fileFilter != nil && !fileFilter(path) {
			return false
		}
		return serverCfg.matchesFile(path)
	}

	// Wait for the workspace to be indexed. Do NOT open all files upfront:
	// gopls reads from disk for workspace indexing. Sending 3K files via
	// didOpen floods stdin and wastes memory (50MB+ for large repos).
	log.Printf("enrichment: waiting for workspace ready (up to 300s)...")
	waitStart := time.Now()
	client.WaitForWorkspaceReadyTimeout(ctx, 300*time.Second)
	log.Printf("enrichment: workspace ready (progress tokens) after %s", time.Since(waitStart).Truncate(time.Millisecond))

	// Readiness probe: progress tokens clearing doesn't mean the server can
	// actually serve requests (gopls reports ready before finishing package
	// loading). Send a probe request with increasing timeouts until the server
	// responds. This prevents flooding the server with 55K+ requests that all
	// timeout while it's still loading.
	if probeFile := findProbeFile(files, filePathByHash, langFilter, e.workspaceRoot); probeFile != "" {
		for _, probeSec := range []int{5, 10, 30, 60, 120} {
			probeCtx, probeCancel := context.WithTimeout(ctx, time.Duration(probeSec)*time.Second)
			_, err := client.GetDefinition(probeCtx, probeFile, lsptypes.Position{Line: 1, Character: 0})
			probeCancel()
			if err == nil {
				log.Printf("enrichment: server responding after %s (probe %ds succeeded)", time.Since(waitStart).Truncate(time.Millisecond), probeSec)
				break
			}
			if probeSec == 120 {
				log.Printf("enrichment: server still not responding after all probes, proceeding anyway")
				break
			}
			log.Printf("enrichment: server not ready (probe %ds: %v), retrying with %ds...", probeSec, err, probeSec*2)
		}
	} else {
		log.Printf("enrichment: no probe file found, skipping readiness check")
	}

	log.Printf("enrichment: readiness probe complete, starting enrichment")

	// Phase 1: Upgrade ast_inferred call edges. GetDefinition does not
	// require didOpen; gopls resolves from its workspace index.
	e.upgradeCallEdges(ctx, client, repoHash, filePathByHash, stats, langFilter)

	// Phase 2: Discover new edges. GetDocumentSymbols requires didOpen, so
	// we open files in batches to limit memory pressure on the LSP server.
	e.discoverNewEdgesBatched(ctx, client, files, filePathByHash, stats, langFilter, serverCfg.LanguageID)

	// Log summary.
	log.Printf("enrichment complete (%s): %d edges processed, %d upgraded, %d skipped, %d errors, %d new edges discovered, %d files scanned, %d file errors",
		serverCfg.LanguageID,
		stats.edgesProcessed.Load(), stats.edgesUpgraded.Load(), stats.edgesSkipped.Load(), stats.edgeErrors.Load(),
		stats.newEdges.Load(), stats.filesProcessed.Load(), stats.fileErrors.Load())
}

// runMultiModule spawns gopls instances for multi-module Go workspaces.
// The root module (largest) is processed first solo, then remaining sub-modules
// are processed in parallel (up to 4 concurrent gopls instances) since they're
// typically small (200-500 files each).
// Progress is tracked so interrupted runs can resume.
func (e *Enricher) runMultiModule(ctx context.Context, serverCfg LSPServerConfig,
	repoHash types.Hash, files []types.File, filePathByHash map[types.Hash]string,
	fileFilter func(string) bool, modules []ModuleInfo) {

	progress, err := LoadProgress(e.workspaceRoot)
	if err != nil {
		log.Printf("enrichment: load progress: %v; starting fresh", err)
		progress = &EnrichProgress{
			Modules:   make(map[string]ModuleStatus),
			StartedAt: time.Now(),
		}
	}

	var enriched, skipped, errored atomic.Int64

	// Separate root module (workspace root) from sub-modules.
	// Root is processed solo first (large, memory-intensive).
	var rootModule *ModuleInfo
	var subModules []ModuleInfo
	for i := range modules {
		if filepath.Clean(modules[i].Dir) == filepath.Clean(e.workspaceRoot) {
			rootModule = &modules[i]
		} else {
			subModules = append(subModules, modules[i])
		}
	}

	// Process root module first (solo, to limit peak memory).
	if rootModule != nil {
		e.enrichModule(ctx, serverCfg, repoHash, files, fileFilter, *rootModule, progress, &enriched, &skipped, &errored, 1, len(modules))
	}

	// Process sub-modules in parallel (4 concurrent gopls instances).
	// Sub-modules are small (~200-500 files), so 4 simultaneous gopls
	// instances use ~800MB total (vs 1.2GB for the root alone).
	const moduleParallelism = 4
	if len(subModules) > 0 {
		log.Printf("enrichment: processing %d sub-modules (%d parallel)", len(subModules), moduleParallelism)

		modSem := make(chan struct{}, moduleParallelism)
		var modWg sync.WaitGroup

		for i := range subModules {
			if ctx.Err() != nil {
				break
			}

			modWg.Add(1)
			modSem <- struct{}{}

			go func(mod ModuleInfo, idx int) {
				defer modWg.Done()
				defer func() { <-modSem }()

				if ctx.Err() != nil {
					return
				}

				// idx+2 because root is [1/N]
				e.enrichModule(ctx, serverCfg, repoHash, files, fileFilter, mod, progress, &enriched, &skipped, &errored, idx+2, len(modules))
			}(subModules[i], i)
		}

		modWg.Wait()
	}

	log.Printf("enrichment: multi-module summary: %d enriched, %d skipped (already complete), %d errored (of %d modules)",
		enriched.Load(), skipped.Load(), errored.Load(), len(modules))
}

// enrichModule processes a single module: starts gopls, runs enrichment, shuts down.
// Thread-safe: can be called from multiple goroutines (progress uses internal sync).
func (e *Enricher) enrichModule(ctx context.Context, serverCfg LSPServerConfig,
	repoHash types.Hash, files []types.File, fileFilter func(string) bool,
	module ModuleInfo, progress *EnrichProgress,
	enriched, skipped, errored *atomic.Int64, position, total int) {

	// Resume support: skip already-completed modules.
	if progress.IsComplete(module.Name) {
		skipped.Add(1)
		return
	}

	moduleFiles := FilesForModule(files, module, e.workspaceRoot)
	if len(moduleFiles) == 0 {
		progress.MarkModule(module.Name, nil)
		skipped.Add(1)
		return
	}

	log.Printf("enrichment: module [%d/%d] %s (%d files)", position, total, module.Name, len(moduleFiles))

	// Build module-scoped filePathByHash.
	moduleFilePathByHash := make(map[types.Hash]string, len(moduleFiles))
	for _, f := range moduleFiles {
		moduleFilePathByHash[f.FileHash] = f.Path
	}

	// Start a gopls instance for this module's directory.
	args := []string{}
	if len(serverCfg.Command) > 1 {
		args = serverCfg.Command[1:]
	}
	client := lsp.NewLSPClient(serverCfg.Command[0], args)
	if err := client.Initialize(ctx, module.Dir); err != nil {
		log.Printf("enrichment: start gopls for module %s: %v", module.Name, err)
		progress.MarkModule(module.Name, fmt.Errorf("gopls start: %w", err))
		errored.Add(1)
		_ = SaveProgress(e.workspaceRoot, progress)
		return
	}

	// Build a module-scoped file filter.
	moduleLangFilter := func(path string) bool {
		if fileFilter != nil && !fileFilter(path) {
			return false
		}
		return serverCfg.matchesFile(path)
	}

	e.runForServerWithClient(ctx, client, serverCfg, repoHash, moduleFiles, moduleFilePathByHash, moduleLangFilter)

	// Shut down this module's gopls.
	client.Shutdown(ctx)

	progress.MarkModule(module.Name, nil)
	enriched.Add(1)

	if err := SaveProgress(e.workspaceRoot, progress); err != nil {
		log.Printf("enrichment: save progress: %v", err)
	}
}

// Close shuts down the LSP client if running.
func (e *Enricher) Close(ctx context.Context) error {
	if e.client != nil {
		e.client.Shutdown(ctx)
		e.client = nil
	}
	return nil
}

// createPhantomNodes scans all edges in the repo and creates phantom external
// nodes for any targets or sources that don't exist in the database. This
// ensures the graph is complete after enrichment: every edge has both endpoints.
// Phantom nodes have Kind="external", FileHash=EmptyHash, and QualifiedName
// derived from the edge context where possible.
func (e *Enricher) createPhantomNodes(ctx context.Context, repoHash types.Hash) error {
	repo, err := e.store.GetRepo(ctx, repoHash)
	if err != nil || repo == nil {
		return err
	}

	nodes, err := e.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return err
	}

	nodeSet := make(map[types.Hash]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n.NodeHash] = true
	}

	// Scan all edges from all nodes and also check dangling edges
	// (edges whose source is not in our node set, created by discoverNewEdges).
	created := 0
	seen := make(map[types.Hash]bool)

	createPhantom := func(hash types.Hash, label string) {
		if nodeSet[hash] || seen[hash] {
			return
		}
		seen[hash] = true
		phantom := types.Node{
			NodeHash:      hash,
			FileHash:      types.EmptyHash,
			QualifiedName: fmt.Sprintf("external://%s", label),
			Kind:          "external",
		}
		if err := e.store.PutNode(ctx, phantom); err == nil {
			nodeSet[hash] = true
			created++
		}
	}

	// Check targets from known source nodes.
	for _, node := range nodes {
		edges, err := e.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			continue
		}
		for _, edge := range edges {
			createPhantom(edge.TargetHash, edge.EdgeType+".target")
		}
	}

	// Check dangling edges (source outside our node set).
	danglingEdges, err := e.store.DanglingEdges(ctx)
	if err == nil {
		for _, edge := range danglingEdges {
			createPhantom(edge.SourceHash, edge.EdgeType+".source")
			createPhantom(edge.TargetHash, edge.EdgeType+".target")
		}
	}

	if created > 0 {
		log.Printf("enrichment: created %d phantom external nodes", created)
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

// edgeWorkItem is a unit of work for concurrent edge upgrade.
type edgeWorkItem struct {
	edge types.Edge
	uri  string
	pos  lsptypes.Position
}

// edgeResolveResult is the result of a parallel LSP resolution.
type edgeResolveResult struct {
	original   types.Edge
	targetHash types.Hash // retargeted hash (may equal original)
}

// upgradeCallEdges finds ast_inferred edges with call-site positions and
// queries the language server's GetDefinition at those positions to confirm targets.
// Successfully resolved edges are upgraded to lsp_resolved with confidence 0.9.
// Edges that already have an lsp_resolved counterpart are skipped.
// LSP calls are made concurrently (bounded by e.concurrency); DB writes are
// serialized through a single writer goroutine to avoid SQLite lock contention.
func (e *Enricher) upgradeCallEdges(
	ctx context.Context,
	client *lsp.LSPClient,
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

	// Collect all edges that need upgrading.
	var workItems []edgeWorkItem
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

			// Skip if an lsp_resolved edge already exists for this source/target/type.
			resolvedHash := types.ComputeEdgeHash(edge.SourceHash, edge.TargetHash, edge.EdgeType, "lsp_resolved")
			if existing, err := e.store.GetEdge(ctx, resolvedHash); err == nil && existing != nil {
				stats.edgesSkipped.Add(1)
				continue
			}

			uri := "file://" + filepath.Join(e.workspaceRoot, edge.CallSiteFile)
			pos := lsptypes.Position{
				Line:      edge.CallSiteLine - 1,
				Character: edge.CallSiteCol,
			}
			workItems = append(workItems, edgeWorkItem{edge: edge, uri: uri, pos: pos})
		}
	}

	if len(workItems) == 0 {
		return
	}

	log.Printf("enrichment: upgrading %d call edges (%d concurrent)", len(workItems), e.concurrency)

	// Channel for resolved results (LSP workers -> DB writer).
	results := make(chan edgeResolveResult, e.concurrency*2)

	// Single DB writer goroutine: serializes all mutations.
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		for res := range results {
			if err := e.store.DeleteEdge(ctx, res.original.EdgeHash); err != nil {
				stats.edgeErrors.Add(1)
				continue
			}

			retargeted := res.original
			retargeted.TargetHash = res.targetHash

			upgraded := upgradeEdge(retargeted)
			upgraded.CallSiteLine = res.original.CallSiteLine
			upgraded.CallSiteCol = res.original.CallSiteCol
			upgraded.CallSiteFile = res.original.CallSiteFile
			if err := e.store.PutEdge(ctx, upgraded); err != nil {
				stats.edgeErrors.Add(1)
				continue
			}
			stats.edgesUpgraded.Add(1)
		}
	}()

	// Two-phase LSP resolution:
	// Phase A: warm up the language server with a single sequential request
	// using a long timeout (300s). This lets gopls/tsserver finish loading the
	// workspace before we flood it with parallel requests.
	// Phase B: blast through remaining edges with high concurrency (64 workers)
	// and a short timeout (5s). The server is warm so responses are fast.
	if len(workItems) > 0 {
		// Warm up the language server by opening a file first. gopls uses lazy
		// loading: it won't load a package until a file from that package is
		// opened via textDocument/didOpen. Without this, GetDefinition returns
		// "no package metadata" instantly because the package was never loaded.
		log.Printf("enrichment: warming up server (opening file + retrying, up to 300s)...")
		warmStart := time.Now()
		warmDeadline := warmStart.Add(300 * time.Second)
		item := workItems[0]

		// Open the file to trigger package loading.
		if content, err := os.ReadFile(strings.TrimPrefix(item.uri, "file://")); err == nil {
			_ = client.OpenDocument(ctx, item.uri, string(content), "go")
			log.Printf("enrichment: opened %s to trigger package loading", item.uri)
		}

		warmPos := lsptypes.Position{Line: 5, Character: 0} // safe position near top of file
		for time.Now().Before(warmDeadline) {
			warmCtx, warmCancel := context.WithTimeout(ctx, 30*time.Second)
			_, warmErr := client.GetDefinition(warmCtx, item.uri, warmPos)
			warmCancel()
			if warmErr == nil {
				log.Printf("enrichment: server warmed up in %s, switching to 64 concurrent workers",
					time.Since(warmStart).Truncate(time.Millisecond))
				break
			}
			if ctx.Err() != nil {
				break
			}
			log.Printf("enrichment: server loading (%v), retrying in 5s...", warmErr)
			time.Sleep(5 * time.Second)
		}
	}

	postWarmupConcurrency := 128
	if e.concurrency > postWarmupConcurrency {
		postWarmupConcurrency = e.concurrency
	}
	sem := make(chan struct{}, postWarmupConcurrency)
	var wg sync.WaitGroup

	for i := range workItems {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, item edgeWorkItem) {
			defer wg.Done()
			defer func() { <-sem }()

			if ctx.Err() != nil {
				return
			}

			stats.edgesProcessed.Add(1)

			timeout := 30 * time.Second

			var locs []lsptypes.Location
			err := WithSymbolTimeout(ctx, timeout, func(tctx context.Context) error {
				var innerErr error
				locs, innerErr = client.GetDefinition(tctx, item.uri, item.pos)
				return innerErr
			})
			if errors.Is(err, ErrSymbolTimeout) {
				stats.edgeErrors.Add(1)
				return
			}
			if err != nil {
				stats.edgeErrors.Add(1)
				return
			}
			if len(locs) == 0 {
				stats.edgesSkipped.Add(1)
				return
			}

			// Retarget the edge if LSP resolves to a known node.
			targetHash := item.edge.TargetHash
			if defNode := e.resolveDefinitionToNode(ctx, locs[0], repoHash); defNode != nil {
				targetHash = defNode.NodeHash
			}

			results <- edgeResolveResult{original: item.edge, targetHash: targetHash}
		}(i, workItems[i])
	}

	wg.Wait()
	close(results)
	writerWg.Wait()
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


// isTestFile returns true for test files across common languages.
// findProbeFile returns the first file path (as a file:// URI) that matches the
// language filter. Used as a readiness probe target for the LSP server.
func findProbeFile(files []types.File, filePathByHash map[types.Hash]string, langFilter func(string) bool, workspaceRoot string) string {
	for _, f := range files {
		path, ok := filePathByHash[f.FileHash]
		if !ok || !langFilter(path) {
			continue
		}
		// Resolve relative paths against workspace root, not cwd.
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspaceRoot, path)
		}
		return "file://" + path
	}
	return ""
}

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

// discoverNewEdgesBatched opens files in batches, queries document symbols,
// then closes the batch before opening the next. This limits memory pressure
// on the LSP server (e.g., gopls on k8s with 3K files and 900MB+ memory).
// Within each batch, files are processed concurrently.
func (e *Enricher) discoverNewEdgesBatched(
	ctx context.Context,
	client *lsp.LSPClient,
	files []types.File,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
	fileFilter func(string) bool,
	languageID string,
) {
	// Collect files that pass the filter.
	var eligible []types.File
	for _, f := range files {
		if fileFilter != nil && !fileFilter(f.Path) {
			continue
		}
		if isTestFile(f.Path) {
			continue
		}
		eligible = append(eligible, f)
	}

	if len(eligible) == 0 {
		return
	}

	// Process in batches of 50 files. Each batch: open -> query -> close.
	const batchSize = 50
	log.Printf("enrichment: discovering edges in %d files (%d concurrent, batch=%d)", len(eligible), e.concurrency, batchSize)

	for batchStart := 0; batchStart < len(eligible); batchStart += batchSize {
		if ctx.Err() != nil {
			break
		}

		batchEnd := batchStart + batchSize
		if batchEnd > len(eligible) {
			batchEnd = len(eligible)
		}
		batch := eligible[batchStart:batchEnd]

		// Open this batch of files.
		for _, f := range batch {
			if ctx.Err() != nil {
				break
			}
			absPath := filepath.Join(e.workspaceRoot, f.Path)
			content, err := os.ReadFile(absPath)
			if err != nil {
				continue
			}
			uri := "file://" + absPath
			_ = client.OpenDocument(ctx, uri, string(content), languageID)
		}

		// Query symbols concurrently within the batch.
		sem := make(chan struct{}, e.concurrency)
		var wg sync.WaitGroup

		for i := range batch {
			if ctx.Err() != nil {
				break
			}

			wg.Add(1)
			sem <- struct{}{}

			go func(f types.File) {
				defer wg.Done()
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}

				stats.filesProcessed.Add(1)
				uri := "file://" + filepath.Join(e.workspaceRoot, f.Path)

				symbols, err := client.GetDocumentSymbols(ctx, uri)
				if err != nil {
					stats.fileErrors.Add(1)
					return
				}

				e.processSymbolsWithClient(ctx, client, uri, symbols, f, filePathByHash, stats)
			}(batch[i])
		}

		wg.Wait()

		// Close this batch of files to release LSP server memory.
		for _, f := range batch {
			uri := "file://" + filepath.Join(e.workspaceRoot, f.Path)
			_ = client.CloseDocument(ctx, uri)
		}
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
// references edges. This is a legacy wrapper that uses e.client.
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

// processSymbolsWithClient processes document symbols using an explicit client
// and per-symbol timeout.
func (e *Enricher) processSymbolsWithClient(
	ctx context.Context,
	client *lsp.LSPClient,
	uri string,
	symbols []lsptypes.DocumentSymbol,
	file types.File,
	filePathByHash map[types.Hash]string,
	stats *enrichStats,
) {
	var sourceLines []string
	absPath := strings.TrimPrefix(uri, "file://")
	if data, err := os.ReadFile(absPath); err == nil {
		sourceLines = strings.Split(string(data), "\n")
	}

	e.processSymbolsWithSourceAndClient(ctx, client, uri, symbols, file, filePathByHash, stats, sourceLines)
}

// processSymbolsWithSource is the recursive implementation of processSymbols
// that carries parsed source lines for position correction. Uses e.client (legacy path).
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

// processSymbolsWithSourceAndClient is the recursive implementation that uses
// an explicit client and wraps LSP calls with WithSymbolTimeout.
func (e *Enricher) processSymbolsWithSourceAndClient(
	ctx context.Context,
	client *lsp.LSPClient,
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
			var impls []lsptypes.Location
			err := WithSymbolTimeout(ctx, e.symbolTimeout, func(tctx context.Context) error {
				var innerErr error
				impls, innerErr = client.GetImplementation(tctx, uri, pos)
				return innerErr
			})
			if err == nil {
				e.insertEdgesFromLocations(ctx, uri, pos, impls, "implements", file, stats)
			}
		}

		if kind == "function" || kind == "method" {
			var refs []lsptypes.Location
			err := WithSymbolTimeout(ctx, e.symbolTimeout, func(tctx context.Context) error {
				var innerErr error
				refs, innerErr = client.GetReferences(tctx, uri, pos, false)
				return innerErr
			})
			if err == nil {
				e.insertEdgesFromLocations(ctx, uri, pos, refs, "references", file, stats)
			}
		}

		if len(sym.Children) > 0 {
			e.processSymbolsWithSourceAndClient(ctx, client, uri, sym.Children, file, filePathByHash, stats, sourceLines)
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
// DB writes are serialized via e.writeMu to avoid SQLite lock contention
// when called from concurrent discover workers.
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

		e.writeMu.Lock()
		existing, err := e.store.GetEdge(ctx, edgeHash)
		if err != nil {
			e.writeMu.Unlock()
			continue
		}
		if existing != nil {
			e.writeMu.Unlock()
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
			e.writeMu.Unlock()
			continue
		}
		e.writeMu.Unlock()
		stats.newEdges.Add(1)
	}
}
