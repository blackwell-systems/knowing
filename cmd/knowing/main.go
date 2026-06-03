// Package main is the entry point for the knowing CLI.
//
// The CLI provides three subcommands:
//   - serve: starts the daemon with file watching, reindexing, and MCP server
//   - index: one-shot indexing of a repository with optional LSP enrichment
//   - query: search the knowledge graph by symbol name prefix
//
// The serve command wires together the full pipeline: SQLite store, snapshot
// manager, indexer (with tree-sitter and optional Python extractors), MCP
// server, and daemon (with git watching and background enrichment).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/blackwell-systems/knowing/internal/daemon"
	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/csharpextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/cssextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/goextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/javaextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/cloudextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/dockerfileextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/envextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/eventextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/gitlabciextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/graphqlextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/helmextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/k8sextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/makefileextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/packagejsonextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/schemaextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/protoextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/rubyextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/rustextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/sqlextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/terraformextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	"github.com/blackwell-systems/knowing/internal/indexer/tsextractor"
	"github.com/blackwell-systems/knowing/internal/community"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	knowingmcp "github.com/blackwell-systems/knowing/internal/mcp"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// Build-time variables set by goreleaser ldflags.
var (
	Version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "version":
		fmt.Printf("knowing %s (commit: %s, built: %s)\n", Version, commit, date)
		return nil
	case "serve":
		return cmdServe(args[1:])
	case "index":
		return cmdIndex(args[1:])
	case "query":
		return cmdQuery(args[1:])
	case "export":
		return cmdExport(args[1:])
	case "diff":
		return cmdDiff(args[1:])
	case "context":
		return cmdContext(args[1:])
	case "mcp":
		return cmdMCP(args[1:])
	case "reindex":
		return cmdReindex(args[1:])
	case "init":
		return cmdSetup(args[1:])
	case "test-scope":
		return cmdTestScope(args[1:])
	case "ingest-scip":
		return cmdIngestSCIP(args[1:])
	case "why":
		return cmdWhy(args[1:])
	case "watch":
		return cmdWatch(args[1:])
	case "fsck":
		return cmdFsck(args[1:])
	case "rebuild-fts":
		return cmdRebuildFTS(args[1:])
	case "prove":
		return cmdProve(args[1:])
	case "verify":
		return cmdVerify(args[1:])
	case "prove-absent":
		return cmdProveAbsent(args[1:])
	case "audit":
		return cmdAudit(args[1:])
	case "audit-diff":
		return cmdAuditDiff(args[1:])
	case "audit-supply-chain":
		return cmdAuditSupplyChain(args[1:])
	case "enrich":
		return cmdEnrich(args[1:])
	case "enrich-similarity":
		return cmdEnrichSimilarity(args[1:])
	case "add":
		return cmdAdd(args[1:])
	case "remove":
		return cmdRemove(args[1:])
	case "daemon":
		return cmdDaemon(args[1:])
	case "list":
		return cmdList(args[1:])
	case "stats":
		return cmdStats(args[1:])
	case "reset":
		return cmdReset(args[1:])
	case "vacuum":
		return cmdVacuum(args[1:])
	case "stale":
		return cmdStale(args[1:])
	case "debug-seeds":
		return cmdDebugSeeds(args[1:])
	case "debug-fts":
		return cmdDebugFTS(args[1:])
	case "debug-walk":
		return cmdDebugWalk(args[1:])
	case "debug-vocab":
		return cmdDebugVocab(args[1:])
	case "debug-feedback":
		return cmdDebugFeedback(args[1:])
	case "debug-equiv":
		return cmdDebugEquiv(args[1:])
	case "debug-pack":
		return cmdDebugPack(args[1:])
	case "debug-rwr-cache":
		return cmdDebugRWRCache(args[1:])
	case "bench-task":
		return cmdBenchTask(args[1:])
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: knowing <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Repository management:")
	fmt.Fprintln(os.Stderr, "  add         Register and index a repository")
	fmt.Fprintln(os.Stderr, "  remove      Remove a repository from the roster (-purge to delete DB)")
	fmt.Fprintln(os.Stderr, "  list        List all tracked repositories")
	fmt.Fprintln(os.Stderr, "  reset       Delete all graph data for a repo (keep DB file)")
	fmt.Fprintln(os.Stderr, "  vacuum      Compact the database after deletions")
	fmt.Fprintln(os.Stderr, "  init        Set up knowing (index, enrich, configure MCP)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Indexing:")
	fmt.Fprintln(os.Stderr, "  index       Index a repository")
	fmt.Fprintln(os.Stderr, "  reindex     Clear and re-index from scratch")
	fmt.Fprintln(os.Stderr, "  enrich      Run LSP enrichment on indexed symbols")
	fmt.Fprintln(os.Stderr, "  ingest-scip Import a SCIP index for external dependencies")
	fmt.Fprintln(os.Stderr, "  watch       Watch for file changes and re-index on save")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Querying:")
	fmt.Fprintln(os.Stderr, "  query       Search the knowledge graph by symbol name")
	fmt.Fprintln(os.Stderr, "  context     Generate graph-ranked context for a task or files")
	fmt.Fprintln(os.Stderr, "  test-scope  Compute affected tests from changed files")
	fmt.Fprintln(os.Stderr, "  why         Explain why a symbol ranked where it did")
	fmt.Fprintln(os.Stderr, "  diff        Compute semantic diff between two snapshots (@latest, @prev, @N)")
	fmt.Fprintln(os.Stderr, "  stale       Report stale nodes/edges from changed files")
	fmt.Fprintln(os.Stderr, "  export      Export the graph as JSON")
	fmt.Fprintln(os.Stderr, "  stats       Show cumulative graph statistics and feedback")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Integrity and proofs:")
	fmt.Fprintln(os.Stderr, "  fsck        Verify graph integrity (hashes, references, chain)")
	fmt.Fprintln(os.Stderr, "  prove       Generate a Merkle proof that a relationship exists")
	fmt.Fprintln(os.Stderr, "  prove-absent  Prove a relationship does NOT exist")
	fmt.Fprintln(os.Stderr, "  verify      Verify a Merkle proof offline (no database needed)")
	fmt.Fprintln(os.Stderr, "  audit       Generate a structured compliance report")
	fmt.Fprintln(os.Stderr, "  audit-diff  Compare two audit snapshots with classification (@latest, @prev, @N)")
	fmt.Fprintln(os.Stderr, "  audit-supply-chain  Detect suspicious supply chain patterns in new code")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Server:")
	fmt.Fprintln(os.Stderr, "  mcp         Run MCP server over stdio")
	fmt.Fprintln(os.Stderr, "  serve       Start daemon with MCP server and file watching")
	fmt.Fprintln(os.Stderr, "  daemon      Manage daemon lifecycle (start/stop/status/restart)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  version     Print version information")
}

// cmdServe starts the daemon: opens the database, creates the indexer with
// tree-sitter extractors, launches the MCP server, watches repos, and blocks
// until SIGINT/SIGTERM.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	addr := fs.String("addr", ":8080", "HTTP address for MCP server")
	traceEnabled := fs.Bool("trace", false, "Enable runtime trace ingestion")
	traceEndpoint := fs.String("trace-endpoint", "localhost:4317", "OTLP gRPC endpoint for trace ingestion")
	traceBatchSize := fs.Int("trace-batch-size", 1000, "Number of spans per batch")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repos := fs.Args()

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)

	// Register extractors (tree-sitter fast path by default).
	registerAllExtractors(idx, false)

	mcpServer := knowingmcp.NewServer(st)

	// Wire the real index function into MCP handlers so index_repo tool works.
	knowingmcp.SetIndexFunc(func(ctx context.Context, repoURL, repoPath, commitHash string) error {
		_, err := idx.IndexRepo(ctx, repoURL, repoPath, commitHash)
		return err
	})

	d := daemon.NewDaemon(daemon.DaemonConfig{
		Store:  st,
		DBPath: *dbPath,
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
			if len(changedFiles) > 0 {
				return idx.IndexFilesIncremental(ctx, repoURL, repoPath, commitHash, changedFiles)
			}
			_, err := idx.IndexRepo(ctx, repoURL, repoPath, commitHash)
			return err
		},
		MCPAddr:   *addr,
		MCPServer: mcpServer,
		TraceConfig: func() *daemon.TraceIngestConfig {
			if !*traceEnabled {
				return nil
			}
			return &daemon.TraceIngestConfig{
				Enabled:       true,
				OTLPEndpoint:  *traceEndpoint,
				BatchSize:     *traceBatchSize,
				BatchInterval: 10 * time.Second,
			}
		}(),
		EnrichFunc: func(ctx context.Context, repoHash types.Hash, workspaceRoot string, changedFiles []string) error {
			enricher := enrichment.NewEnricher(st, workspaceRoot)
			if len(changedFiles) > 0 {
				return enricher.RunScoped(ctx, repoHash, changedFiles)
			}
			return enricher.Run(ctx, repoHash)
		},
	})

	// Watch requested repos.
	for _, repo := range repos {
		if err := d.WatchRepo(repo); err != nil {
			return fmt.Errorf("watching repo %s: %w", repo, err)
		}
	}

	// Start daemon with signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	if err := d.Start(ctx); err != nil {
		return fmt.Errorf("daemon: %w", err)
	}

	return d.Stop()
}

// cmdIndex performs a one-shot index of a repository. By default it uses
// the fast tree-sitter extractor; pass -full to use the Go packages extractor
// with full type resolution. After indexing, it runs LSP enrichment (unless
// -full was used, since the full extractor already produces high-confidence edges).
func cmdIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (e.g. github.com/org/repo)")
	commitHash := fs.String("commit", "HEAD", "Commit hash to record")
	filesFlag := fs.String("files", "", "Comma-separated list of changed files (relative paths) for incremental index")
	full := fs.Bool("full", false, "Use full type resolution (go/packages) instead of fast tree-sitter extraction")
	skipBlame := fs.Bool("skip-blame", false, "Skip git blame authorship extraction (faster, no authored_by edges)")
	noEnrich := fs.Bool("no-enrich", false, "Skip LSP enrichment (faster, edges stay at 0.7 confidence)")
	enrichConcurrency := fs.Int("enrich-concurrency", 8, "Number of parallel LSP requests during enrichment")
	workers := fs.Int("workers", 0, "Number of parallel extraction workers (default: 8)")
	edgeTypesFlag := fs.String("edge-types", "", "Comma-separated edge types to keep (ablation filter; empty = all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing index [flags] <repo-path>")
	}
	repoPath := fs.Arg(0)

	// Resolve repo URL to a stable canonical form.
	// Priority: explicit --url flag > go.mod module path > absolute filesystem path.
	if *repoURL == "" {
		// Try to read module path from go.mod in the repo.
		if modData, err := os.ReadFile(repoPath + "/go.mod"); err == nil {
			for _, line := range strings.Split(string(modData), "\n") {
				if strings.HasPrefix(line, "module ") {
					*repoURL = strings.TrimSpace(strings.TrimPrefix(line, "module "))
					break
				}
			}
		}
		// Fallback: resolve to absolute path for consistency.
		if *repoURL == "" {
			if abs, err := filepath.Abs(repoPath); err == nil {
				*repoURL = abs
			} else {
				*repoURL = repoPath
			}
		}
	}

	// Resolve HEAD commit hash if not explicitly provided.
	if *commitHash == "HEAD" {
		if resolved, err := daemon.GitHeadCommit(repoPath); err == nil {
			*commitHash = resolved
		}
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.SkipBlame = *skipBlame
	if *workers > 0 {
		idx.Concurrency = *workers
	}
	if *edgeTypesFlag != "" {
		idx.EdgeTypes = make(map[string]bool)
		for _, et := range strings.Split(*edgeTypesFlag, ",") {
			et = strings.TrimSpace(et)
			if et != "" {
				idx.EdgeTypes[et] = true
			}
		}
	}

	// Register extractors.
	registerAllExtractors(idx, *full)

	ctx := context.Background()
	repoHash := types.NewHash([]byte(*repoURL))

	// Incremental mode: only reindex specified files.
	if *filesFlag != "" {
		files := strings.Split(*filesFlag, ",")
		fmt.Fprintf(os.Stderr, "Incremental index: %d files in %s\n", len(files), repoPath)
		indexStart := time.Now()
		if err := idx.IndexFilesIncremental(ctx, *repoURL, repoPath, *commitHash, files); err != nil {
			return fmt.Errorf("incremental indexing: %w", err)
		}
		fmt.Printf("Indexed %d files in %s\n", len(files), time.Since(indexStart).Truncate(time.Millisecond))

		// Rebuild FTS for affected packages.
		if err := st.RebuildFTS(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "FTS rebuild warning: %v\n", err)
		}
		return nil
	}

	fmt.Fprintf(os.Stderr, "Indexing %s (%s)...\n", repoPath, *repoURL)
	indexStart := time.Now()
	snap, err := idx.IndexRepo(ctx, *repoURL, repoPath, *commitHash)
	if err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	fmt.Printf("Indexed %s in %s\n", repoPath, time.Since(indexStart).Truncate(time.Millisecond))
	fmt.Printf("Repo:     %x\n", repoHash)
	fmt.Printf("Snapshot: %x\n", snap.SnapshotHash)
	fmt.Printf("Nodes: %d, Edges: %d\n", snap.NodeCount, snap.EdgeCount)

	// Build adjacency cache for fast RWR queries. Compact binary format:
	// 65 bytes/edge, so 500K edges = ~32MB base64. Above that, BFS falls
	// back to per-node indexed queries.
	if snap.EdgeCount <= 500000 {
		if err := knowingctx.BuildAdjacencyCache(ctx, st); err != nil {
			fmt.Fprintf(os.Stderr, "adjacency cache warning: %v\n", err)
		}
	}

	if !*full && !*noEnrich {
		// Run in-process Go resolver (fast, no external dependencies).
		if err := runInProcessResolver(ctx, st, repoPath, repoHash); err != nil {
			fmt.Fprintf(os.Stderr, "  in-process resolver warning: %v\n", err)
		}

		fmt.Println("Running LSP enrichment...")
		enricher := enrichment.NewEnricher(st, repoPath)
		enricher.SetConcurrency(*enrichConcurrency)
		if err := enricher.Run(ctx, repoHash); err != nil {
			fmt.Fprintf(os.Stderr, "enrichment warning: %v\n", err)
		}
		enricher.Close(ctx)
	}

	// Note: embeddings are off by default (confirmed neutral on cold-start
	// benchmarks, session 23). Use --embeddings flag to enable if needed.

	return nil
}

// cmdQuery searches the knowledge graph for nodes matching a qualified name
// prefix and prints each node with its outgoing edges.
func cmdQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing query [flags] <symbol-prefix>")
	}
	prefix := fs.Arg(0)

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()
	nodes, err := st.NodesByName(ctx, prefix)
	if err != nil {
		return fmt.Errorf("querying nodes: %w", err)
	}

	if len(nodes) == 0 {
		fmt.Println("No nodes found.")
		return nil
	}

	for _, n := range nodes {
		fmt.Printf("%s (%s) [%x]\n", n.QualifiedName, n.Kind, n.NodeHash)

		edges, err := st.EdgesFrom(ctx, n.NodeHash, "")
		if err != nil {
			return fmt.Errorf("querying edges: %w", err)
		}
		for _, e := range edges {
			fmt.Printf("  -> %x [%s]\n", e.TargetHash, e.EdgeType)
		}
	}
	return nil
}

// cmdExport exports the knowledge graph as JSON to stdout. Supports filtering
// by repo URL and snapshot hash.
func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	format := fs.String("format", "json", "Output format (only json supported)")
	repoFilter := fs.String("repo", "", "Filter by repo URL")
	snapshotFilter := fs.String("snapshot", "", "Filter by snapshot hash")
	algoFlag := fs.String("algorithm", community.Default, "Community detection algorithm (louvain, louvain-fine, label-propagation)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *format != "json" && *format != "dot" {
		return fmt.Errorf("unsupported format: %s (use json or dot)", *format)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Collect nodes. If repo filter is specified, find nodes by repo prefix.
	// Otherwise, get all nodes.
	var nodes []types.Node
	if *repoFilter != "" {
		// Find nodes associated with the repo by querying with empty prefix
		// (all nodes), then filtering by repo. This is a simple approach;
		// a future optimization could add a NodesByRepo store method.
		allNodes, err := st.NodesByName(ctx, "")
		if err != nil {
			return fmt.Errorf("querying nodes: %w", err)
		}
		// Get files for this repo to filter nodes.
		repoHash := types.NewHash([]byte(*repoFilter))
		files, err := st.FilesByRepo(ctx, repoHash)
		if err != nil {
			return fmt.Errorf("querying files: %w", err)
		}
		fileSet := make(map[types.Hash]bool, len(files))
		for _, f := range files {
			fileSet[f.FileHash] = true
		}
		for _, n := range allNodes {
			if fileSet[n.FileHash] {
				nodes = append(nodes, n)
			}
		}
	} else {
		nodes, err = st.NodesByName(ctx, "")
		if err != nil {
			return fmt.Errorf("querying nodes: %w", err)
		}
	}

	// Collect edges from each node, filtering to edges where both endpoints
	// are in the exported node set (prevents cross-repo orphan edges).
	nodeHashSet := make(map[types.Hash]bool, len(nodes))
	for _, n := range nodes {
		nodeHashSet[n.NodeHash] = true
	}
	edgeSet := make(map[types.Hash]bool)
	var edges []types.Edge
	for _, n := range nodes {
		outgoing, err := st.EdgesFrom(ctx, n.NodeHash, "")
		if err != nil {
			return fmt.Errorf("querying edges from %x: %w", n.NodeHash, err)
		}
		for _, e := range outgoing {
			if !edgeSet[e.EdgeHash] && nodeHashSet[e.TargetHash] {
				edgeSet[e.EdgeHash] = true
				edges = append(edges, e)
			}
		}
	}

	// Build export structure with community annotations.
	type exportCommunity struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
		Size  int    `json:"size"`
	}
	type exportNode struct {
		NodeHash      string  `json:"node_hash"`
		QualifiedName string  `json:"qualified_name"`
		Kind          string  `json:"kind"`
		Line          int     `json:"line"`
		Signature     string  `json:"signature"`
		Community     int     `json:"community"`
		LastAuthor    string  `json:"last_author,omitempty"`
		LastCommitAt  int64   `json:"last_commit_at,omitempty"`
		CoveragePct   float64 `json:"coverage_pct"`
		Doc           string  `json:"doc,omitempty"`
	}
	type exportEdge struct {
		EdgeHash       string  `json:"edge_hash"`
		SourceHash     string  `json:"source_hash"`
		TargetHash     string  `json:"target_hash"`
		EdgeType       string  `json:"edge_type"`
		Confidence     float64 `json:"confidence"`
		Provenance     string  `json:"provenance"`
		CrossCommunity bool    `json:"cross_community"`
	}
	type exportMetadata struct {
		Repo           string `json:"repo"`
		Snapshot       string `json:"snapshot"`
		ExportedAt     string `json:"exported_at"`
		NodeCount      int    `json:"node_count"`
		EdgeCount      int    `json:"edge_count"`
		CommunityCount int    `json:"community_count"`
	}
	type exportData struct {
		Nodes       []exportNode      `json:"nodes"`
		Edges       []exportEdge      `json:"edges"`
		Communities []exportCommunity `json:"communities"`
		Metadata    exportMetadata    `json:"metadata"`
	}

	repoLabel := "all"
	if *repoFilter != "" {
		repoLabel = *repoFilter
	}
	snapLabel := "latest"
	if *snapshotFilter != "" {
		snapLabel = *snapshotFilter
	}

	if *format == "dot" {
		return exportDot(nodes, edges)
	}

	// Run community detection to annotate nodes with community IDs.
	communityOf, communityLabels := computeExportCommunities(nodes, edges, *algoFlag)

	// Build community summary. Communities smaller than minSize are merged into "other".
	const minCommunitySize = 10
	commSizes := make(map[int]int)
	for _, c := range communityOf {
		commSizes[c]++
	}
	// Assign small communities to an "other" bucket.
	otherID := -1
	otherSize := 0
	for h, c := range communityOf {
		if commSizes[c] < minCommunitySize {
			communityOf[h] = otherID
			otherSize++
		}
	}
	var exportComms []exportCommunity
	significantComms := make(map[int]bool)
	for id, size := range commSizes {
		if size >= minCommunitySize {
			exportComms = append(exportComms, exportCommunity{
				ID:    id,
				Label: communityLabels[id],
				Size:  size,
			})
			significantComms[id] = true
		}
	}
	if otherSize > 0 {
		exportComms = append(exportComms, exportCommunity{
			ID:    otherID,
			Label: "other",
			Size:  otherSize,
		})
		significantComms[otherID] = true
	}
	sort.Slice(exportComms, func(i, j int) bool {
		return exportComms[i].Size > exportComms[j].Size
	})
	// Nodes in singleton communities get assigned to community -1 (ungrouped).
	for h, c := range communityOf {
		if !significantComms[c] {
			communityOf[h] = -1
		}
	}

	export := exportData{
		Nodes:       make([]exportNode, 0, len(nodes)),
		Edges:       make([]exportEdge, 0, len(edges)),
		Communities: exportComms,
		Metadata: exportMetadata{
			Repo:           repoLabel,
			Snapshot:       snapLabel,
			ExportedAt:     time.Now().UTC().Format(time.RFC3339),
			NodeCount:      len(nodes),
			EdgeCount:      len(edges),
			CommunityCount: len(exportComms),
		},
	}

	for _, n := range nodes {
		en := exportNode{
			NodeHash:      n.NodeHash.String(),
			QualifiedName: n.QualifiedName,
			Kind:          n.Kind,
			Line:          n.Line,
			Signature:     n.Signature,
			Community:     communityOf[n.NodeHash],
			LastAuthor:    n.LastAuthor,
			LastCommitAt:  n.LastCommitAt,
			Doc:           n.Doc,
			CoveragePct:   n.CoveragePct,
		}
		export.Nodes = append(export.Nodes, en)
	}
	for _, e := range edges {
		srcComm := communityOf[e.SourceHash]
		tgtComm := communityOf[e.TargetHash]
		export.Edges = append(export.Edges, exportEdge{
			EdgeHash:       e.EdgeHash.String(),
			SourceHash:     e.SourceHash.String(),
			TargetHash:     e.TargetHash.String(),
			EdgeType:       e.EdgeType,
			Confidence:     e.Confidence,
			Provenance:     e.Provenance,
			CrossCommunity: srcComm != tgtComm,
		})
	}

	out, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// computeExportCommunities runs community detection and returns community assignments + labels.
// The algoName parameter selects the algorithm (e.g. "louvain", "louvain-fine", "label-propagation").
// If algoName is empty or not found, defaults to community.Default.
func computeExportCommunities(nodes []types.Node, edges []types.Edge, algoName string) (map[types.Hash]int, map[int]string) {
	// Build undirected adjacency.
	nodeSet := make(map[types.Hash]bool, len(nodes))
	nodeHashes := make([]types.Hash, 0, len(nodes))
	nodeByHash := make(map[types.Hash]types.Node, len(nodes))
	for _, n := range nodes {
		nodeSet[n.NodeHash] = true
		nodeHashes = append(nodeHashes, n.NodeHash)
		nodeByHash[n.NodeHash] = n
	}

	adj := make(map[types.Hash][]community.WeightedEdge, len(nodes))
	for _, e := range edges {
		if nodeSet[e.SourceHash] && nodeSet[e.TargetHash] {
			adj[e.SourceHash] = append(adj[e.SourceHash], community.WeightedEdge{Target: e.TargetHash, Weight: 1.0})
			adj[e.TargetHash] = append(adj[e.TargetHash], community.WeightedEdge{Target: e.SourceHash, Weight: 1.0})
		}
	}

	// Select and run algorithm.
	algo := community.Get(algoName)
	if algo == nil {
		algo = community.Get(community.Default)
	}
	g := &community.Graph{
		Nodes:   nodeHashes,
		Adj:     adj,
		NodeSet: nodeSet,
	}
	communityOf := algo.Detect(g)

	// Label by dominant package.
	communityLabels := make(map[int]string)
	pkgCounts := make(map[int]map[string]int)
	for h, c := range communityOf {
		if pkgCounts[c] == nil {
			pkgCounts[c] = make(map[string]int)
		}
		pkg := extractPkgFromQualified(nodeByHash[h].QualifiedName)
		if pkg != "" {
			pkgCounts[c][pkg]++
		}
	}
	for c, counts := range pkgCounts {
		bestPkg := ""
		bestCount := 0
		for pkg, count := range counts {
			if count > bestCount {
				bestCount = count
				bestPkg = pkg
			}
		}
		if bestPkg != "" {
			communityLabels[c] = bestPkg
		} else {
			communityLabels[c] = fmt.Sprintf("community_%d", c)
		}
	}

	// Deduplicate labels: when multiple communities share a package name,
	// use the highest-degree symbol as a differentiator (e.g., "server (MCPServer)")
	// instead of opaque numbering like "server #14".
	labelCounts := make(map[string]int)
	for _, label := range communityLabels {
		labelCounts[label]++
	}

	// For communities with duplicate labels, find their top symbol by edge count.
	commTopSymbol := make(map[int]string)
	edgeCount := make(map[types.Hash]int)
	for _, e := range edges {
		edgeCount[e.SourceHash]++
		edgeCount[e.TargetHash]++
	}
	for h, c := range communityOf {
		if labelCounts[communityLabels[c]] <= 1 {
			continue
		}
		node := nodeByHash[h]
		shortName := node.QualifiedName
		if dot := strings.LastIndex(shortName, "."); dot >= 0 {
			shortName = shortName[dot+1:]
		}
		// Prefer non-test symbols as differentiators.
		isTest := strings.HasPrefix(shortName, "Test") || strings.HasPrefix(shortName, "test_")
		existing, hasExisting := commTopSymbol[c]
		existingIsTest := strings.HasPrefix(existing, "Test") || strings.HasPrefix(existing, "test_")
		if !hasExisting || (!isTest && existingIsTest) || (!isTest && !existingIsTest && edgeCount[h] > edgeCount[types.NewHash([]byte(existing))]) {
			commTopSymbol[c] = shortName
		}
	}

	commIDList := make([]int, 0, len(communityLabels))
	for c := range communityLabels {
		commIDList = append(commIDList, c)
	}
	sort.Ints(commIDList)
	for _, c := range commIDList {
		label := communityLabels[c]
		if labelCounts[label] > 1 {
			if topSym, ok := commTopSymbol[c]; ok && topSym != "" {
				communityLabels[c] = fmt.Sprintf("%s (%s)", label, topSym)
			}
		}
	}

	return communityOf, communityLabels
}

// exportDot renders the graph as a Graphviz DOT file with community subgraphs.
// Communities are detected via the community package and rendered as cluster subgraphs.
// Cross-community edges are colored red to highlight architectural boundaries.
func exportDot(nodes []types.Node, edges []types.Edge) error {
	// Build node hash -> node map and adjacency for community detection.
	nodeByHash := make(map[types.Hash]types.Node, len(nodes))
	nodeHashes := make([]types.Hash, 0, len(nodes))
	for _, n := range nodes {
		nodeByHash[n.NodeHash] = n
		nodeHashes = append(nodeHashes, n.NodeHash)
	}

	nodeSet := make(map[types.Hash]bool, len(nodes))
	for _, h := range nodeHashes {
		nodeSet[h] = true
	}

	adj := make(map[types.Hash][]community.WeightedEdge, len(nodes))
	for _, e := range edges {
		if nodeSet[e.SourceHash] && nodeSet[e.TargetHash] {
			adj[e.SourceHash] = append(adj[e.SourceHash], community.WeightedEdge{Target: e.TargetHash, Weight: 1.0})
			adj[e.TargetHash] = append(adj[e.TargetHash], community.WeightedEdge{Target: e.SourceHash, Weight: 1.0})
		}
	}

	// Run community detection using louvain-fine (low resolution for package-level clusters).
	algo := community.Get("louvain-fine")
	g := &community.Graph{
		Nodes:   nodeHashes,
		Adj:     adj,
		NodeSet: nodeSet,
	}
	communityOf := algo.Detect(g)

	// Group nodes by community, keep only communities with 5+ members.
	// Cap at 20 communities (merge smallest into the largest).
	allCommunities := make(map[int][]types.Hash)
	for h, c := range communityOf {
		allCommunities[c] = append(allCommunities[c], h)
	}
	communities := make(map[int][]types.Hash)
	for c, members := range allCommunities {
		if len(members) >= 5 {
			communities[c] = members
		}
	}
	// If still too many, keep only the 20 largest.
	if len(communities) > 20 {
		type commSize struct {
			id   int
			size int
		}
		var sorted []commSize
		for id, members := range communities {
			sorted = append(sorted, commSize{id, len(members)})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].size > sorted[j].size
		})
		kept := make(map[int]bool)
		for i := 0; i < 20 && i < len(sorted); i++ {
			kept[sorted[i].id] = true
		}
		for id := range communities {
			if !kept[id] {
				delete(communities, id)
			}
		}
	}

	// Find dominant package for each community (for labeling).
	communityLabel := make(map[int]string)
	for cID, members := range communities {
		pkgCount := make(map[string]int)
		for _, h := range members {
			n := nodeByHash[h]
			pkg := extractPkgFromQualified(n.QualifiedName)
			if pkg != "" {
				pkgCount[pkg]++
			}
		}
		bestPkg := ""
		bestCount := 0
		for pkg, count := range pkgCount {
			if count > bestCount {
				bestCount = count
				bestPkg = pkg
			}
		}
		if bestPkg != "" {
			communityLabel[cID] = bestPkg
		} else {
			communityLabel[cID] = fmt.Sprintf("cluster_%d", cID)
		}
	}

	// Emit DOT.
	fmt.Println("digraph knowing {")
	fmt.Println("  rankdir=LR;")
	fmt.Println("  node [shape=box, style=filled, fontsize=10];")
	fmt.Println("  edge [fontsize=8];")
	fmt.Println("")

	// Color palette for communities.
	colors := []string{
		"#E8F5E9", "#E3F2FD", "#FFF3E0", "#F3E5F5",
		"#E0F7FA", "#FBE9E7", "#F1F8E9", "#EDE7F6",
		"#E8EAF6", "#FFF8E1", "#E0F2F1", "#FCE4EC",
		"#ECEFF1", "#F9FBE7", "#E1F5FE", "#FFF9C4",
	}

	// Render each community as a subgraph cluster.
	for cID, members := range communities {
		color := colors[cID%len(colors)]
		label := communityLabel[cID]
		fmt.Printf("  subgraph cluster_%d {\n", cID)
		fmt.Printf("    label=%q;\n", label)
		fmt.Printf("    style=filled;\n")
		fmt.Printf("    color=%q;\n", color)
		fmt.Printf("    fontsize=12;\n")
		fmt.Println("")

		for _, h := range members {
			n := nodeByHash[h]
			nodeID := fmt.Sprintf("n%x", h[:4])
			shortName := shortSymbolName(n.QualifiedName)
			shape := "box"
			if n.Kind == "type" {
				shape = "ellipse"
			} else if n.Kind == "service" {
				shape = "hexagon"
			}
			fmt.Printf("    %s [label=%q, shape=%s];\n", nodeID, shortName, shape)
		}
		fmt.Println("  }")
		fmt.Println("")
	}

	// Render edges. Cross-community edges are red.
	for _, e := range edges {
		if !nodeSet[e.SourceHash] || !nodeSet[e.TargetHash] {
			continue
		}
		srcID := fmt.Sprintf("n%x", e.SourceHash[:4])
		tgtID := fmt.Sprintf("n%x", e.TargetHash[:4])
		srcComm := communityOf[e.SourceHash]
		tgtComm := communityOf[e.TargetHash]

		attrs := ""
		if srcComm != tgtComm {
			attrs = " [color=red, style=bold]"
		}
		fmt.Printf("  %s -> %s%s;\n", srcID, tgtID, attrs)
	}

	fmt.Println("}")
	return nil
}

// extractPkgFromQualified extracts the package path from a qualified name.
func extractPkgFromQualified(qname string) string {
	idx := strings.Index(qname, "://")
	if idx < 0 {
		return ""
	}
	rest := qname[idx+3:]
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return rest
	}
	pkg := rest[:lastDot]
	// Trim to last path component for readability.
	lastSlash := strings.LastIndex(pkg, "/")
	if lastSlash >= 0 {
		return pkg[lastSlash+1:]
	}
	return pkg
}

// shortSymbolName extracts just the symbol name from a qualified name.
func shortSymbolName(qname string) string {
	lastDot := strings.LastIndex(qname, ".")
	if lastDot >= 0 {
		return qname[lastDot+1:]
	}
	return qname
}

// cmdReindex clears all nodes, edges, and edge events, then re-indexes the
// repository from scratch. This solves stale data and duplicate prefix issues.
func cmdReindex(args []string) error {
	fs := flag.NewFlagSet("reindex", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (e.g. github.com/org/repo)")
	commitHash := fs.String("commit", "HEAD", "Commit hash to record")
	full := fs.Bool("full", false, "Use full type resolution (go/packages) instead of fast tree-sitter extraction")
	reindexEnrichConcurrency := fs.Int("enrich-concurrency", 8, "Number of parallel LSP requests during enrichment")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing reindex [flags] <repo-path>")
	}
	repoPath := fs.Arg(0)

	// Resolve repo URL (same logic as cmdIndex).
	if *repoURL == "" {
		if modData, err := os.ReadFile(repoPath + "/go.mod"); err == nil {
			for _, line := range strings.Split(string(modData), "\n") {
				if strings.HasPrefix(line, "module ") {
					*repoURL = strings.TrimSpace(strings.TrimPrefix(line, "module "))
					break
				}
			}
		}
		if *repoURL == "" {
			if abs, err := filepath.Abs(repoPath); err == nil {
				*repoURL = abs
			} else {
				*repoURL = repoPath
			}
		}
	}

	// Resolve HEAD commit hash if not explicitly provided.
	if *commitHash == "HEAD" {
		if resolved, err := daemon.GitHeadCommit(repoPath); err == nil {
			*commitHash = resolved
		}
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	// Clear all existing graph data.
	ctx := context.Background()
	if err := st.TruncateGraph(ctx); err != nil {
		return fmt.Errorf("clearing database: %w", err)
	}

	// Re-run the full index pipeline.
	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)

	registerAllExtractors(idx, *full)

	snap, err := idx.IndexRepo(ctx, *repoURL, repoPath, *commitHash)
	if err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	fmt.Printf("Reindexed: %d nodes, %d edges (previous data cleared)\n", snap.NodeCount, snap.EdgeCount)

	if !*full {
		fmt.Println("Running LSP enrichment...")
		enricher := enrichment.NewEnricher(st, repoPath)
		enricher.SetConcurrency(*reindexEnrichConcurrency)
		if err := enricher.Run(ctx, types.NewHash([]byte(*repoURL))); err != nil {
			fmt.Fprintf(os.Stderr, "enrichment warning: %v\n", err)
		}
		enricher.Close(ctx)
	}

	return nil
}

// cmdStats shows cumulative graph statistics and feedback metrics.
func cmdStats(args []string) error {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	jsonOutput := fs.Bool("json", false, "Output as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()
	db := st.DB()

	// Node count.
	var nodeCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes").Scan(&nodeCount)

	// Edge count.
	var edgeCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edges").Scan(&edgeCount)

	// Snapshot count.
	var snapshotCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM snapshots").Scan(&snapshotCount)

	// Feedback stats.
	var feedbackTotal int64
	var feedbackUseful int64
	var feedbackNotUseful int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback").Scan(&feedbackTotal)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback WHERE useful = 1").Scan(&feedbackUseful)
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback WHERE useful = 0").Scan(&feedbackNotUseful)

	// Unique symbols with feedback.
	var feedbackSymbols int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT symbol_hash) FROM feedback").Scan(&feedbackSymbols)

	// Feedback with neighborhood_root (merkleized).
	var merkleizedFeedback int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM feedback WHERE neighborhood_root IS NOT NULL AND length(neighborhood_root) = 32").Scan(&merkleizedFeedback)

	// File count.
	var fileCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files").Scan(&fileCount)

	// Repo count.
	var repoCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM repos").Scan(&repoCount)

	// Community count (if table exists).
	var communityCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT community_id) FROM community_assignments").Scan(&communityCount)

	// Graph notes count.
	var notesCount int64
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graph_notes").Scan(&notesCount)

	if *jsonOutput {
		fmt.Printf(`{"nodes":%d,"edges":%d,"files":%d,"repos":%d,"snapshots":%d,"communities":%d,"graph_notes":%d,"feedback":{"total":%d,"useful":%d,"not_useful":%d,"symbols_with_feedback":%d,"merkleized":%d,"usefulness_rate":"%.1f%%"}}`,
			nodeCount, edgeCount, fileCount, repoCount, snapshotCount, communityCount, notesCount,
			feedbackTotal, feedbackUseful, feedbackNotUseful, feedbackSymbols, merkleizedFeedback,
			usefulnessRate(feedbackUseful, feedbackTotal))
		fmt.Println()
		return nil
	}

	fmt.Println("knowing stats")
	fmt.Println("=============")
	fmt.Println()
	fmt.Println("Graph")
	fmt.Printf("  Repos:        %d\n", repoCount)
	fmt.Printf("  Nodes:        %d\n", nodeCount)
	fmt.Printf("  Edges:        %d\n", edgeCount)
	fmt.Printf("  Files:        %d\n", fileCount)
	fmt.Printf("  Snapshots:    %d\n", snapshotCount)
	fmt.Printf("  Communities:  %d\n", communityCount)
	fmt.Printf("  Graph notes:  %d\n", notesCount)
	fmt.Println()
	fmt.Println("Feedback")
	fmt.Printf("  Total:        %d\n", feedbackTotal)
	fmt.Printf("  Useful:       %d\n", feedbackUseful)
	fmt.Printf("  Not useful:   %d\n", feedbackNotUseful)
	fmt.Printf("  Symbols:      %d (unique symbols with feedback)\n", feedbackSymbols)
	fmt.Printf("  Merkleized:   %d (with neighborhood_root for expiration)\n", merkleizedFeedback)
	if feedbackTotal > 0 {
		fmt.Printf("  Usefulness:   %.1f%%\n", usefulnessRate(feedbackUseful, feedbackTotal))
	}

	return nil
}

func usefulnessRate(useful, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(useful) / float64(total) * 100
}

// cmdReset deletes all graph data for a repository without removing the DB file.
func cmdReset(args []string) error {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()
	if err := st.TruncateGraph(ctx); err != nil {
		return fmt.Errorf("truncating graph: %w", err)
	}

	fmt.Println("Graph data deleted. Database file preserved.")
	fmt.Println("Run 'knowing index' to re-index.")
	return nil
}

// cmdVacuum compacts the database file after deletions.
func cmdVacuum(args []string) error {
	fs := flag.NewFlagSet("vacuum", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	// Get size before.
	info, _ := os.Stat(*dbPath)
	sizeBefore := info.Size()

	_, err = st.DB().ExecContext(context.Background(), "VACUUM")
	if err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}

	// Get size after.
	info, _ = os.Stat(*dbPath)
	sizeAfter := info.Size()

	saved := sizeBefore - sizeAfter
	fmt.Printf("Vacuumed %s\n", *dbPath)
	fmt.Printf("  Before: %s\n", formatBytes(sizeBefore))
	fmt.Printf("  After:  %s\n", formatBytes(sizeAfter))
	if saved > 0 {
		fmt.Printf("  Saved:  %s (%.1f%%)\n", formatBytes(saved), float64(saved)/float64(sizeBefore)*100)
	}
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}

// registerAllExtractors registers all language extractors with the indexer.
// If fullGo is true, uses the full go/packages extractor; otherwise uses
// tree-sitter for Go (faster but less type info).
func registerAllExtractors(idx *indexer.Indexer, fullGo bool) {
	// Go extractor (full type resolution or fast tree-sitter).
	if fullGo {
		idx.Register(goextractor.NewGoExtractor())
	} else {
		idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	}

	// Python (tree-sitter multi-language).
	if tsExt, err := treesitter.NewTreeSitterExtractor("python"); err == nil {
		idx.Register(tsExt)
	}

	// TypeScript/JavaScript.
	idx.Register(tsextractor.NewTypeScriptExtractor())

	// Rust.
	idx.Register(rustextractor.NewRustExtractor())

	// Java.
	idx.Register(javaextractor.NewJavaExtractor())

	// Ruby.
	idx.Register(rubyextractor.NewRubyExtractor())

	// C#.
	idx.Register(csharpextractor.NewCSharpExtractor())

	// Terraform HCL.
	idx.Register(terraformextractor.NewTerraformExtractor())

	// SQL.
	idx.Register(sqlextractor.NewSQLExtractor())

	// Kubernetes YAML.
	idx.Register(k8sextractor.NewK8sExtractor())

	// Cloud infrastructure YAML (CloudFormation, Docker Compose, GitHub Actions, Serverless).
	idx.Register(cloudextractor.NewCloudExtractor())

	// CSS/SCSS.
	idx.Register(cssextractor.NewCSSExtractor())

	// Protocol Buffers.
	idx.Register(protoextractor.NewProtoExtractor())

	// Event/MQ patterns (Kafka, NATS, SQS, RabbitMQ).
	// Handles same file extensions as language extractors; multi-dispatch
	// ensures both run on the same file.
	idx.Register(eventextractor.NewEventExtractor())

	// OpenAPI/JSON Schema.
	idx.Register(schemaextractor.NewSchemaExtractor())

	// Dockerfile.
	idx.Register(dockerfileextractor.NewDockerfileExtractor())

	// GraphQL schema.
	idx.Register(graphqlextractor.NewGraphQLExtractor())

	// Environment variable files (.env).
	idx.Register(envextractor.NewEnvExtractor())

	// Makefile.
	idx.Register(makefileextractor.NewMakefileExtractor())

	// Helm charts (Chart.yaml, values.yaml, templates).
	idx.Register(helmextractor.NewHelmExtractor())

	// GitLab CI (.gitlab-ci.yml).
	idx.Register(gitlabciextractor.NewGitLabCIExtractor())

	// package.json (Node.js).
	idx.Register(packagejsonextractor.NewPackageJSONExtractor())
}

// Compile-time assertion that SQLiteStore implements GraphStore. This also
// ensures the types import is used.
var _ types.GraphStore = (*store.SQLiteStore)(nil)
