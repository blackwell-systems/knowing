package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdDebugFTS runs a raw FTS5 query against the node index and shows results.
// Usage: knowing debug-fts -query "provider OR schema" [-db path] [-limit N]
func cmdDebugFTS(args []string) error {
	fs := flag.NewFlagSet("debug-fts", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	query := fs.String("query", "", "FTS5 query string (required)")
	limit := fs.Int("limit", 30, "Max results to show")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *query == "" {
		return fmt.Errorf("usage: knowing debug-fts -query \"term1 OR term2\" [-db path] [-limit N]")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	fmt.Printf("=== FTS5 DEBUG ===\n")
	fmt.Printf("Query: %s\n", *query)
	fmt.Printf("Limit: %d\n", *limit)
	fmt.Printf("\n")

	nodes, err := st.SearchBM25Nodes(ctx, *query, *limit)
	if err != nil {
		return fmt.Errorf("FTS5 error: %w", err)
	}

	fmt.Printf("--- Results (%d) ---\n", len(nodes))
	for i, n := range nodes {
		// Extract package from qualified name for context.
		pkg := extractPkgFromQualified(n.QualifiedName)
		symbol := terminalSymbol(n.QualifiedName)
		fmt.Printf("  [%2d] %s\n", i+1, symbol)
		fmt.Printf("       pkg: %s\n", pkg)
		fmt.Printf("       kind: %s  qn: %s\n", n.Kind, truncate(n.QualifiedName, 120))
		if n.Signature != "" {
			fmt.Printf("       sig: %s\n", truncate(n.Signature, 100))
		}
		fmt.Printf("\n")
	}

	if len(nodes) == 0 {
		fmt.Printf("  (no results)\n")
		fmt.Printf("\n  Tips:\n")
		fmt.Printf("  - Check FTS5 syntax: terms joined with OR, phrases in quotes\n")
		fmt.Printf("  - Column targets: symbol_name:\"term\" or file_path:\"term\"\n")
		fmt.Printf("  - Prefix matching: term* (matches term, terms, terminal, etc.)\n")
	}

	return nil
}

// truncate shortens a string with ellipsis if longer than max.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// extractPkgFromQualified is defined in debug_seeds.go
