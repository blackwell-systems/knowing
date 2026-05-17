package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/indexer/scipingest"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdIngestSCIP imports a SCIP index file into the knowing knowledge graph.
// It reads a .scip protobuf file produced by a SCIP-compatible indexer and
// creates nodes and edges for all symbols and references found in the index.
func cmdIngestSCIP(args []string) error {
	fs := flag.NewFlagSet("ingest-scip", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	file := fs.String("file", "", "Path to the .scip index file (required)")
	repo := fs.String("repo", "", "Repository URL to associate (e.g. github.com/org/repo) (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *file == "" {
		return fmt.Errorf("usage: knowing ingest-scip -file <path> -repo <url> [flags]\n  -file flag is required")
	}
	if *repo == "" {
		return fmt.Errorf("usage: knowing ingest-scip -file <path> -repo <url> [flags]\n  -repo flag is required")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ingester := scipingest.NewSCIPIngester(st)
	ctx := context.Background()
	result, err := ingester.IngestFile(ctx, scipingest.SCIPIngestOptions{
		FilePath: *file,
		RepoURL:  *repo,
	})
	if err != nil {
		return fmt.Errorf("ingesting SCIP index: %w", err)
	}

	fmt.Printf("Ingested %d nodes, %d edges from %d documents\n",
		result.NodesCreated, result.EdgesCreated, result.DocsProcessed)

	return nil
}
