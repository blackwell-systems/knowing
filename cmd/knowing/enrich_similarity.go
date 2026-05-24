package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdEnrichSimilarity adds similarity edges to an existing database without
// re-indexing. Loads all nodes, computes pairwise Jaccard similarity within
// each package, and stores similar_to edges for pairs above threshold.
//
// Usage: knowing enrich-similarity [-db path] [-threshold 0.5]
func cmdEnrichSimilarity(args []string) error {
	fs := flag.NewFlagSet("enrich-similarity", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database")
	threshold := fs.Float64("threshold", 0.5, "Jaccard similarity threshold (0.0-1.0)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Load all nodes from the database.
	allNodes, err := st.NodesByName(ctx, "%")
	if err != nil {
		return fmt.Errorf("loading nodes: %w", err)
	}
	if len(allNodes) == 0 {
		return fmt.Errorf("no nodes in database (index first)")
	}

	fmt.Printf("Loaded %d nodes, computing similarity edges (threshold=%.2f)...\n", len(allNodes), *threshold)

	// Compute similarity edges.
	edges := indexer.ComputeSimilarityEdges(allNodes, *threshold)
	fmt.Printf("Generated %d similarity edges\n", len(edges))

	if len(edges) == 0 {
		return nil
	}

	// Store edges.
	if err := st.BatchPutEdges(ctx, edges); err != nil {
		return fmt.Errorf("storing edges: %w", err)
	}

	// Rebuild FTS to include new edges in search.
	st.RebuildFTS(ctx) //nolint:errcheck

	fmt.Printf("Done. %d similarity edges added to %s\n", len(edges), *dbPath)
	return nil
}
