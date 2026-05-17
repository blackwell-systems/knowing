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
	"github.com/blackwell-systems/knowing/internal/indexer/k8sextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/protoextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/rustextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/sqlextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/terraformextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	"github.com/blackwell-systems/knowing/internal/indexer/tsextractor"
	knowingmcp "github.com/blackwell-systems/knowing/internal/mcp"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// Version is the current version of the knowing CLI.
const Version = "knowing v0.1.0"

// defaultDB returns the default database path, checking KNOWING_DB env var first.
func defaultDB() string {
	if env := os.Getenv("KNOWING_DB"); env != "" {
		return env
	}
	return "knowing.db"
}

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
	case "ingest-scip":
		return cmdIngestSCIP(args[1:])
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
	fmt.Fprintln(os.Stderr, "  ingest-scip Import a SCIP index for external dependency symbols")
	fmt.Fprintln(os.Stderr, "  version  Print version information")
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
	registerAllExtractors(idx, *full)

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

	// Build export structure with community annotations.
	type exportCommunity struct {
		ID    int    `json:"id"`
		Label string `json:"label"`
		Size  int    `json:"size"`
	}
	type exportNode struct {
		NodeHash      string `json:"node_hash"`
		QualifiedName string `json:"qualified_name"`
		Kind          string `json:"kind"`
		Line          int    `json:"line"`
		Signature     string `json:"signature"`
		Community     int    `json:"community"`
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

	// Run Louvain to annotate nodes with community IDs.
	communityOf, communityLabels := computeExportCommunities(nodes, edges)

	// Build community summary (only communities with 3+ members).
	commSizes := make(map[int]int)
	for _, c := range communityOf {
		commSizes[c]++
	}
	var exportComms []exportCommunity
	significantComms := make(map[int]bool)
	for id, size := range commSizes {
		if size >= 3 {
			exportComms = append(exportComms, exportCommunity{
				ID:    id,
				Label: communityLabels[id],
				Size:  size,
			})
			significantComms[id] = true
		}
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
		export.Nodes = append(export.Nodes, exportNode{
			NodeHash:      fmt.Sprintf("%x", n.NodeHash),
			QualifiedName: n.QualifiedName,
			Kind:          n.Kind,
			Line:          n.Line,
			Signature:     n.Signature,
			Community:     communityOf[n.NodeHash],
		})
	}
	for _, e := range edges {
		srcComm := communityOf[e.SourceHash]
		tgtComm := communityOf[e.TargetHash]
		export.Edges = append(export.Edges, exportEdge{
			EdgeHash:       fmt.Sprintf("%x", e.EdgeHash),
			SourceHash:     fmt.Sprintf("%x", e.SourceHash),
			TargetHash:     fmt.Sprintf("%x", e.TargetHash),
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

// computeExportCommunities runs Louvain and returns community assignments + labels.
func computeExportCommunities(nodes []types.Node, edges []types.Edge) (map[types.Hash]int, map[int]string) {
	// Build undirected adjacency.
	nodeSet := make(map[types.Hash]bool, len(nodes))
	nodeHashes := make([]types.Hash, 0, len(nodes))
	nodeByHash := make(map[types.Hash]types.Node, len(nodes))
	for _, n := range nodes {
		nodeSet[n.NodeHash] = true
		nodeHashes = append(nodeHashes, n.NodeHash)
		nodeByHash[n.NodeHash] = n
	}

	type wEdge struct {
		target types.Hash
		weight float64
	}
	adj := make(map[types.Hash][]wEdge, len(nodes))
	for _, e := range edges {
		if nodeSet[e.SourceHash] && nodeSet[e.TargetHash] {
			adj[e.SourceHash] = append(adj[e.SourceHash], wEdge{e.TargetHash, 1.0})
			adj[e.TargetHash] = append(adj[e.TargetHash], wEdge{e.SourceHash, 1.0})
		}
	}

	// Louvain.
	communityOf := make(map[types.Hash]int, len(nodeHashes))
	for i, h := range nodeHashes {
		communityOf[h] = i
	}

	var twoM float64
	for _, neighbors := range adj {
		for _, e := range neighbors {
			twoM += e.weight
		}
	}
	if twoM == 0 {
		twoM = 1
	}
	m := twoM / 2.0

	ki := make(map[types.Hash]float64, len(nodeHashes))
	for _, h := range nodeHashes {
		for _, e := range adj[h] {
			ki[h] += e.weight
		}
	}

	sigmaTot := make(map[int]float64, len(nodeHashes))
	for _, h := range nodeHashes {
		sigmaTot[communityOf[h]] += ki[h]
	}

	for pass := 0; pass < 20; pass++ {
		moved := false
		for _, node := range nodeHashes {
			currentComm := communityOf[node]
			bestComm := currentComm
			bestGain := 0.0

			kiIn := make(map[int]float64)
			for _, e := range adj[node] {
				kiIn[communityOf[e.target]] += e.weight
			}

			sigCurr := sigmaTot[currentComm] - ki[node]
			kiInCurr := kiIn[currentComm]

			for c, w := range kiIn {
				if c == currentComm {
					continue
				}
				sigC := sigmaTot[c]
				gainAdd := w/m - (sigC*ki[node])/(2*m*m)
				gainRemove := kiInCurr/m - (sigCurr*ki[node])/(2*m*m)
				gain := gainAdd - gainRemove
				if gain > bestGain {
					bestGain = gain
					bestComm = c
				}
			}

			if bestComm != currentComm {
				sigmaTot[currentComm] -= ki[node]
				sigmaTot[bestComm] += ki[node]
				communityOf[node] = bestComm
				moved = true
			}
		}
		if !moved {
			break
		}
	}

	// Renumber and label.
	commIDs := make(map[int]int)
	nextID := 0
	for _, c := range communityOf {
		if _, ok := commIDs[c]; !ok {
			commIDs[c] = nextID
			nextID++
		}
	}
	for h := range communityOf {
		communityOf[h] = commIDs[communityOf[h]]
	}

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

	// Deduplicate labels: if multiple communities share a label, append (#2, #3, etc.)
	labelCounts := make(map[string]int)
	labelSeen := make(map[string]int)
	for _, label := range communityLabels {
		labelCounts[label]++
	}
	// Sort community IDs for deterministic numbering.
	commIDList := make([]int, 0, len(communityLabels))
	for c := range communityLabels {
		commIDList = append(commIDList, c)
	}
	sort.Ints(commIDList)
	for _, c := range commIDList {
		label := communityLabels[c]
		if labelCounts[label] > 1 {
			labelSeen[label]++
			communityLabels[c] = fmt.Sprintf("%s #%d", label, labelSeen[label])
		}
	}

	return communityOf, communityLabels
}

// exportDot renders the graph as a Graphviz DOT file with community subgraphs.
// Communities are detected via Louvain and rendered as cluster subgraphs.
// Cross-community edges are colored red to highlight architectural boundaries.
func exportDot(nodes []types.Node, edges []types.Edge) error {
	// Build node hash -> node map and adjacency for Louvain.
	nodeByHash := make(map[types.Hash]types.Node, len(nodes))
	nodeHashes := make([]types.Hash, 0, len(nodes))
	for _, n := range nodes {
		nodeByHash[n.NodeHash] = n
		nodeHashes = append(nodeHashes, n.NodeHash)
	}

	// Build adjacency list for Louvain (undirected).
	nodeSet := make(map[types.Hash]bool, len(nodes))
	for _, h := range nodeHashes {
		nodeSet[h] = true
	}

	type wEdge struct {
		target types.Hash
		weight float64
	}
	adj := make(map[types.Hash][]wEdge, len(nodes))
	for _, e := range edges {
		if nodeSet[e.SourceHash] && nodeSet[e.TargetHash] {
			adj[e.SourceHash] = append(adj[e.SourceHash], wEdge{e.TargetHash, 1.0})
			adj[e.TargetHash] = append(adj[e.TargetHash], wEdge{e.SourceHash, 1.0})
		}
	}

	// Run Louvain with correct modularity gain (same algorithm as communities MCP tool).
	communityOf := make(map[types.Hash]int, len(nodeHashes))
	for i, h := range nodeHashes {
		communityOf[h] = i
	}

	// Total weight (sum of all edge weights; adj is undirected so twoM = sum of all entries).
	var twoM float64
	for _, neighbors := range adj {
		for _, e := range neighbors {
			twoM += e.weight
		}
	}
	if twoM == 0 {
		twoM = 1
	}
	m := twoM / 2.0

	// Node strengths (weighted degree).
	ki := make(map[types.Hash]float64, len(nodeHashes))
	for _, h := range nodeHashes {
		for _, e := range adj[h] {
			ki[h] += e.weight
		}
	}

	// Sigma_tot per community.
	sigmaTot := make(map[int]float64, len(nodeHashes))
	for _, h := range nodeHashes {
		sigmaTot[communityOf[h]] += ki[h]
	}

	// Iterate.
	for pass := 0; pass < 20; pass++ {
		moved := false
		for _, node := range nodeHashes {
			currentComm := communityOf[node]
			bestComm := currentComm
			bestGain := 0.0

			kiIn := make(map[int]float64)
			for _, e := range adj[node] {
				kiIn[communityOf[e.target]] += e.weight
			}

			sigCurr := sigmaTot[currentComm] - ki[node]
			kiInCurr := kiIn[currentComm]

			for c, w := range kiIn {
				if c == currentComm {
					continue
				}
				sigC := sigmaTot[c]
				gainAdd := w/m - (sigC*ki[node])/(2*m*m)
				gainRemove := kiInCurr/m - (sigCurr*ki[node])/(2*m*m)
				gain := gainAdd - gainRemove
				if gain > bestGain {
					bestGain = gain
					bestComm = c
				}
			}

			if bestComm != currentComm {
				sigmaTot[currentComm] -= ki[node]
				sigmaTot[bestComm] += ki[node]
				communityOf[node] = bestComm
				moved = true
			}
		}
		if !moved {
			break
		}
	}

	// Renumber communities to 0..N-1.
	commIDs := make(map[int]int)
	nextID := 0
	for _, c := range communityOf {
		if _, ok := commIDs[c]; !ok {
			commIDs[c] = nextID
			nextID++
		}
	}
	for h := range communityOf {
		communityOf[h] = commIDs[communityOf[h]]
	}

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
		if err := enricher.Run(ctx, types.NewHash([]byte(*repoURL))); err != nil {
			fmt.Fprintf(os.Stderr, "enrichment warning: %v\n", err)
		}
		enricher.Close(ctx)
	}

	return nil
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
}

// Compile-time assertion that SQLiteStore implements GraphStore. This also
// ensures the types import is used.
var _ types.GraphStore = (*store.SQLiteStore)(nil)
