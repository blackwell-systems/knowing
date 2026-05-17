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
	"strings"
	"syscall"
	"time"

	"github.com/blackwell-systems/knowing/internal/daemon"
	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/goextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	knowingmcp "github.com/blackwell-systems/knowing/internal/mcp"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// Version is the current version of the knowing CLI.
const Version = "knowing v0.1.0"

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
		fmt.Println(Version)
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
		return cmdInit(args[1:])
	case "test-scope":
		return cmdTestScope(args[1:])
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: knowing <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  mcp      Run MCP server over stdio")
	fmt.Fprintln(os.Stderr, "  serve    Start the daemon with MCP server and file watching")
	fmt.Fprintln(os.Stderr, "  index    Index a repository")
	fmt.Fprintln(os.Stderr, "  query    Query the knowledge graph")
	fmt.Fprintln(os.Stderr, "  diff     Compute semantic diff between two snapshots")
	fmt.Fprintln(os.Stderr, "  context  Generate graph-aware context for a task or files")
	fmt.Fprintln(os.Stderr, "  export   Export the graph as JSON")
	fmt.Fprintln(os.Stderr, "  reindex  Clear and re-index a repository from scratch")
	fmt.Fprintln(os.Stderr, "  init     Generate CLAUDE.md with graph-derived project context")
	fmt.Fprintln(os.Stderr, "  test-scope  Compute affected tests from changed files")
	fmt.Fprintln(os.Stderr, "  version  Print version information")
}

// cmdServe starts the daemon: opens the database, creates the indexer with
// tree-sitter extractors, launches the MCP server, watches repos, and blocks
// until SIGINT/SIGTERM.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
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
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	tsExt, err := treesitter.NewTreeSitterExtractor("python")
	if err == nil {
		idx.Register(tsExt)
	}

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
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
	repoURL := fs.String("url", "", "Repository URL (e.g. github.com/org/repo)")
	commitHash := fs.String("commit", "HEAD", "Commit hash to record")
	full := fs.Bool("full", false, "Use full type resolution (go/packages) instead of fast tree-sitter extraction")
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

	// Register extractors.
	if *full {
		idx.Register(goextractor.NewGoExtractor())
	} else {
		idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	}
	tsExt, err := treesitter.NewTreeSitterExtractor("python")
	if err == nil {
		idx.Register(tsExt)
	}

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, *repoURL, repoPath, *commitHash)
	if err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	repoHash := types.NewHash([]byte(*repoURL))
	fmt.Printf("Indexed %s\n", repoPath)
	fmt.Printf("Repo:     %x\n", repoHash)
	fmt.Printf("Snapshot: %x\n", snap.SnapshotHash)
	fmt.Printf("Nodes: %d, Edges: %d\n", snap.NodeCount, snap.EdgeCount)

	if !*full {
		fmt.Println("Running LSP enrichment...")
		enricher := enrichment.NewEnricher(st, repoPath)
		if err := enricher.Run(ctx, repoHash); err != nil {
			fmt.Fprintf(os.Stderr, "enrichment warning: %v\n", err)
		}
		enricher.Close(ctx)
	}

	return nil
}

// cmdQuery searches the knowledge graph for nodes matching a qualified name
// prefix and prints each node with its outgoing edges.
func cmdQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
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
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
	format := fs.String("format", "json", "Output format (only json supported)")
	repoFilter := fs.String("repo", "", "Filter by repo URL")
	snapshotFilter := fs.String("snapshot", "", "Filter by snapshot hash")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *format != "json" {
		return fmt.Errorf("unsupported format: %s", *format)
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

	// Collect edges from each node.
	edgeSet := make(map[types.Hash]bool)
	var edges []types.Edge
	for _, n := range nodes {
		outgoing, err := st.EdgesFrom(ctx, n.NodeHash, "")
		if err != nil {
			return fmt.Errorf("querying edges from %x: %w", n.NodeHash, err)
		}
		for _, e := range outgoing {
			if !edgeSet[e.EdgeHash] {
				edgeSet[e.EdgeHash] = true
				edges = append(edges, e)
			}
		}
	}

	// Build export structure.
	type exportNode struct {
		NodeHash      string `json:"node_hash"`
		QualifiedName string `json:"qualified_name"`
		Kind          string `json:"kind"`
		Line          int    `json:"line"`
		Signature     string `json:"signature"`
	}
	type exportEdge struct {
		EdgeHash   string  `json:"edge_hash"`
		SourceHash string  `json:"source_hash"`
		TargetHash string  `json:"target_hash"`
		EdgeType   string  `json:"edge_type"`
		Confidence float64 `json:"confidence"`
		Provenance string  `json:"provenance"`
	}
	type exportMetadata struct {
		Repo       string `json:"repo"`
		Snapshot   string `json:"snapshot"`
		ExportedAt string `json:"exported_at"`
		NodeCount  int    `json:"node_count"`
		EdgeCount  int    `json:"edge_count"`
	}
	type exportData struct {
		Nodes    []exportNode   `json:"nodes"`
		Edges    []exportEdge   `json:"edges"`
		Metadata exportMetadata `json:"metadata"`
	}

	repoLabel := "all"
	if *repoFilter != "" {
		repoLabel = *repoFilter
	}
	snapLabel := "latest"
	if *snapshotFilter != "" {
		snapLabel = *snapshotFilter
	}

	export := exportData{
		Nodes: make([]exportNode, 0, len(nodes)),
		Edges: make([]exportEdge, 0, len(edges)),
		Metadata: exportMetadata{
			Repo:       repoLabel,
			Snapshot:   snapLabel,
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
			NodeCount:  len(nodes),
			EdgeCount:  len(edges),
		},
	}

	for _, n := range nodes {
		export.Nodes = append(export.Nodes, exportNode{
			NodeHash:      fmt.Sprintf("%x", n.NodeHash),
			QualifiedName: n.QualifiedName,
			Kind:          n.Kind,
			Line:          n.Line,
			Signature:     n.Signature,
		})
	}
	for _, e := range edges {
		export.Edges = append(export.Edges, exportEdge{
			EdgeHash:   fmt.Sprintf("%x", e.EdgeHash),
			SourceHash: fmt.Sprintf("%x", e.SourceHash),
			TargetHash: fmt.Sprintf("%x", e.TargetHash),
			EdgeType:   e.EdgeType,
			Confidence: e.Confidence,
			Provenance: e.Provenance,
		})
	}

	out, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// cmdReindex clears all nodes, edges, and edge events, then re-indexes the
// repository from scratch. This solves stale data and duplicate prefix issues.
func cmdReindex(args []string) error {
	fs := flag.NewFlagSet("reindex", flag.ExitOnError)
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
	repoURL := fs.String("url", "", "Repository URL (e.g. github.com/org/repo)")
	commitHash := fs.String("commit", "HEAD", "Commit hash to record")
	full := fs.Bool("full", false, "Use full type resolution (go/packages) instead of fast tree-sitter extraction")
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

	if *full {
		idx.Register(goextractor.NewGoExtractor())
	} else {
		idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	}
	tsExt, err := treesitter.NewTreeSitterExtractor("python")
	if err == nil {
		idx.Register(tsExt)
	}

	snap, err := idx.IndexRepo(ctx, *repoURL, repoPath, *commitHash)
	if err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	fmt.Printf("Reindexed: %d nodes, %d edges (previous data cleared)\n", snap.NodeCount, snap.EdgeCount)

	if !*full {
		fmt.Println("Running LSP enrichment...")
		enricher := enrichment.NewEnricher(st, repoPath)
		if err := enricher.Run(ctx, types.NewHash([]byte(*repoURL))); err != nil {
			fmt.Fprintf(os.Stderr, "enrichment warning: %v\n", err)
		}
		enricher.Close(ctx)
	}

	return nil
}

// Compile-time assertion that SQLiteStore implements GraphStore. This also
// ensures the types import is used.
var _ types.GraphStore = (*store.SQLiteStore)(nil)
