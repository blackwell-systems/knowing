package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdRebuildFTS rebuilds the FTS5 search index without reindexing.
// Useful after enrichment (to exclude phantom nodes from search) or
// after any change to the FTS tokenization logic.
func cmdRebuildFTS(args []string) error {
	fs := flag.NewFlagSet("rebuild-fts", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	// Count before.
	var before int
	_ = st.DB().QueryRow("SELECT COUNT(*) FROM nodes_fts_content").Scan(&before)

	fmt.Fprintf(os.Stderr, "Rebuilding FTS index for %s...\n", *dbPath)
	start := time.Now()
	if err := st.RebuildFTS(context.Background()); err != nil {
		return fmt.Errorf("rebuild fts: %w", err)
	}

	// Count after.
	var after int
	_ = st.DB().QueryRow("SELECT COUNT(*) FROM nodes_fts_content").Scan(&after)

	fmt.Fprintf(os.Stderr, "FTS rebuilt in %s: %d -> %d entries",
		time.Since(start).Truncate(time.Millisecond), before, after)
	if before != after {
		fmt.Fprintf(os.Stderr, " (%d phantom nodes excluded)", before-after)
	}
	fmt.Fprintln(os.Stderr)
	return nil
}
