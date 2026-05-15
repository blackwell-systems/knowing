// Package main is the entry point for the knowing CLI.
// It wires all internal packages together and dispatches subcommands.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/blackwell-systems/knowing/internal/daemon"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/goextractor"
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
	default:
		printUsage()
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: knowing <subcommand> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Subcommands:")
	fmt.Fprintln(os.Stderr, "  serve    Start the daemon with MCP server and file watching")
	fmt.Fprintln(os.Stderr, "  index    Index a repository")
	fmt.Fprintln(os.Stderr, "  query    Query the knowledge graph")
	fmt.Fprintln(os.Stderr, "  version  Print version information")
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
	addr := fs.String("addr", ":8080", "HTTP address for MCP server")
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

	// Register extractors.
	idx.Register(goextractor.NewGoExtractor())
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
		Store: st,
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string) error {
			_, err := idx.IndexRepo(ctx, repoURL, repoPath, commitHash)
			return err
		},
		MCPAddr:   *addr,
		MCPServer: mcpServer,
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

func cmdIndex(args []string) error {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
	repoURL := fs.String("url", "", "Repository URL (e.g. github.com/org/repo)")
	commitHash := fs.String("commit", "HEAD", "Commit hash to record")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing index [flags] <repo-path>")
	}
	repoPath := fs.Arg(0)

	if *repoURL == "" {
		*repoURL = repoPath
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
	idx.Register(goextractor.NewGoExtractor())
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
	return nil
}

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

// Ensure types import is used (referenced by daemon.DaemonConfig.Store field type).
var _ types.GraphStore = (*store.SQLiteStore)(nil)
