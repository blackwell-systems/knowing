package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/blackwell-systems/knowing/internal/diff"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdDiff compares two snapshots and prints the semantic diff. It opens the
// database, parses the two snapshot hashes from positional arguments, calls
// SemanticDiff, and renders the result in text or JSON format.
func cmdDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	format := fs.String("format", "text", "Output format: text or json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return fmt.Errorf("usage: knowing diff [flags] <old-ref> <new-ref>\n  refs: @latest, @prev, @first, @N, or 64-char hex hash")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	oldHash, err := resolveSnapshotRef(st, fs.Arg(0))
	if err != nil {
		return fmt.Errorf("resolving old ref: %w", err)
	}

	newHash, err := resolveSnapshotRef(st, fs.Arg(1))
	if err != nil {
		return fmt.Errorf("resolving new ref: %w", err)
	}

	ctx := context.Background()
	result, err := diff.SemanticDiff(ctx, st, oldHash, newHash)
	if err != nil {
		return fmt.Errorf("computing semantic diff: %w", err)
	}

	switch *format {
	case "json":
		out, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(out))
	case "text":
		printTextDiff(result)
	default:
		return fmt.Errorf("unsupported format: %s (use text or json)", *format)
	}

	return nil
}

// parseSnapshotHash decodes a hex string into a types.Hash. It returns an
// error if the hex is invalid or not exactly 32 bytes (64 hex characters).
func parseSnapshotHash(s string) (types.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return types.Hash{}, fmt.Errorf("invalid hex: %w", err)
	}
	if len(b) != 32 {
		return types.Hash{}, fmt.Errorf("expected 32 bytes (64 hex chars), got %d bytes", len(b))
	}
	var h types.Hash
	copy(h[:], b)
	return h, nil
}

// resolveSnapshotRef resolves a snapshot reference to a hash.
// Supports:
//   - @latest: most recent snapshot
//   - @first: oldest snapshot
//   - @N: Nth snapshot from latest (0 = latest, 1 = previous, etc.)
//   - @prev: alias for @1
//   - Raw 64-char hex hash
func resolveSnapshotRef(st *store.SQLiteStore, ref string) (types.Hash, error) {
	ctx := context.Background()

	if !strings.HasPrefix(ref, "@") {
		return parseSnapshotHash(ref)
	}

	name := ref[1:] // strip @

	db := st.DB()
	switch name {
	case "latest":
		var hashBytes []byte
		err := db.QueryRowContext(ctx, "SELECT snapshot_hash FROM snapshots ORDER BY timestamp DESC LIMIT 1").Scan(&hashBytes)
		if err != nil {
			return types.Hash{}, fmt.Errorf("no snapshots found")
		}
		var h types.Hash
		copy(h[:], hashBytes)
		return h, nil

	case "first":
		var hashBytes []byte
		err := db.QueryRowContext(ctx, "SELECT snapshot_hash FROM snapshots ORDER BY timestamp ASC LIMIT 1").Scan(&hashBytes)
		if err != nil {
			return types.Hash{}, fmt.Errorf("no snapshots found")
		}
		var h types.Hash
		copy(h[:], hashBytes)
		return h, nil

	case "prev":
		return resolveSnapshotRef(st, "@1")

	default:
		// Try numeric offset: @N means Nth from latest.
		offset, err := strconv.Atoi(name)
		if err != nil {
			return types.Hash{}, fmt.Errorf("unknown ref %q (use @latest, @first, @prev, @N, or a hex hash)", ref)
		}
		var hashBytes []byte
		err = db.QueryRowContext(ctx,
			"SELECT snapshot_hash FROM snapshots ORDER BY timestamp DESC LIMIT 1 OFFSET ?", offset).Scan(&hashBytes)
		if err != nil {
			return types.Hash{}, fmt.Errorf("snapshot @%d not found (only %d snapshots exist)", offset, countSnapshots(db))
		}
		var h types.Hash
		copy(h[:], hashBytes)
		return h, nil
	}
}

func countSnapshots(db interface{ QueryRowContext(context.Context, string, ...any) *sql.Row }) int {
	var count int
	_ = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM snapshots").Scan(&count)
	return count
}

// printTextDiff renders a SemanticDiffResult as human-readable text to stdout.
func printTextDiff(r *diff.SemanticDiffResult) {
	oldShort := shortHash(r.OldSnapshot)
	newShort := shortHash(r.NewSnapshot)

	fmt.Fprintf(os.Stdout, "Semantic Diff: %s -> %s\n\n", oldShort, newShort)

	if len(r.NodesAdded) > 0 {
		fmt.Fprintf(os.Stdout, "Nodes Added (%d):\n", len(r.NodesAdded))
		for _, n := range r.NodesAdded {
			if n.Line > 0 {
				fmt.Fprintf(os.Stdout, "  + %s (%s) [line %d]\n", n.QualifiedName, n.Kind, n.Line)
			} else {
				fmt.Fprintf(os.Stdout, "  + %s (%s)\n", n.QualifiedName, n.Kind)
			}
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(r.NodesRemoved) > 0 {
		fmt.Fprintf(os.Stdout, "Nodes Removed (%d):\n", len(r.NodesRemoved))
		for _, n := range r.NodesRemoved {
			if n.Line > 0 {
				fmt.Fprintf(os.Stdout, "  - %s (%s) [line %d]\n", n.QualifiedName, n.Kind, n.Line)
			} else {
				fmt.Fprintf(os.Stdout, "  - %s (%s)\n", n.QualifiedName, n.Kind)
			}
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(r.NodesModified) > 0 {
		fmt.Fprintf(os.Stdout, "Nodes Modified (%d):\n", len(r.NodesModified))
		for _, n := range r.NodesModified {
			fmt.Fprintf(os.Stdout, "  ~ %s (%s)\n", n.QualifiedName, n.Kind)
			fmt.Fprintf(os.Stdout, "    edges added: %d, edges removed: %d\n",
				len(n.EdgesAdded), len(n.EdgesRemoved))
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(r.EdgesAdded) > 0 {
		fmt.Fprintf(os.Stdout, "Edges Added (%d):\n", len(r.EdgesAdded))
		for _, e := range r.EdgesAdded {
			fmt.Fprintf(os.Stdout, "  %s -> %s [%s] (confidence: %.2f)\n",
				e.SourceName, e.TargetName, e.EdgeType, e.Confidence)
		}
		fmt.Fprintln(os.Stdout)
	}

	if len(r.EdgesRemoved) > 0 {
		fmt.Fprintf(os.Stdout, "Edges Removed (%d):\n", len(r.EdgesRemoved))
		for _, e := range r.EdgesRemoved {
			fmt.Fprintf(os.Stdout, "  %s -> %s [%s] (confidence: %.2f)\n",
				e.SourceName, e.TargetName, e.EdgeType, e.Confidence)
		}
		fmt.Fprintln(os.Stdout)
	}

	fmt.Fprintf(os.Stdout, "Summary: %d nodes added, %d removed, %d modified; %d edges added, %d removed\n",
		r.Summary.NodesAdded, r.Summary.NodesRemoved, r.Summary.NodesModified,
		r.Summary.EdgesAdded, r.Summary.EdgesRemoved)
}

// shortHash returns the first 8 characters of a hex hash string for display.
func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
