package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/blackwell-systems/knowing/internal/diff"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdDiff compares two snapshots and prints the semantic diff. It opens the
// database, parses the two snapshot hashes from positional arguments, calls
// SemanticDiff, and renders the result in text or JSON format.
func cmdDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	dbPath := fs.String("db", "knowing.db", "Path to the SQLite database")
	format := fs.String("format", "text", "Output format: text or json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() != 2 {
		return fmt.Errorf("usage: knowing diff [flags] <old-snapshot-hash> <new-snapshot-hash>")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	oldHash, err := parseSnapshotHash(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("invalid old snapshot hash: %w", err)
	}

	newHash, err := parseSnapshotHash(fs.Arg(1))
	if err != nil {
		return fmt.Errorf("invalid new snapshot hash: %w", err)
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
