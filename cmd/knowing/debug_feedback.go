package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdDebugFeedback shows feedback records for a symbol or all symbols.
// Usage: knowing debug-feedback [-db path] [-symbol name] [-min-count N] [-top N]
func cmdDebugFeedback(args []string) error {
	fs := flag.NewFlagSet("debug-feedback", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	symbol := fs.String("symbol", "", "Filter by symbol name (substring match on qualified_name)")
	minCount := fs.Int("min-count", 1, "Minimum total feedback count to display")
	top := fs.Int("top", 50, "Maximum number of symbols to display")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Query feedback records with optional symbol filter.
	query := `SELECT f.symbol_hash,
		COALESCE(n.qualified_name, hex(f.symbol_hash)),
		SUM(CASE WHEN f.useful = 1 THEN 1 ELSE 0 END) as pos,
		SUM(CASE WHEN f.useful = 0 THEN 1 ELSE 0 END) as neg,
		COUNT(*) as total,
		COUNT(DISTINCT f.keyword_cluster) as clusters
		FROM feedback f
		LEFT JOIN nodes n ON f.symbol_hash = n.node_hash`

	var args2 []any
	if *symbol != "" {
		query += ` WHERE n.qualified_name LIKE ?`
		args2 = append(args2, "%"+*symbol+"%")
	}
	query += ` GROUP BY f.symbol_hash HAVING total >= ? ORDER BY total DESC LIMIT ?`
	args2 = append(args2, *minCount, *top)

	rows, err := st.DB().QueryContext(ctx, query, args2...)
	if err != nil {
		return fmt.Errorf("querying feedback: %w", err)
	}
	defer rows.Close()

	fmt.Printf("=== Feedback Records ===\n")
	fmt.Printf("Filter: symbol=%q  min-count=%d\n\n", *symbol, *minCount)
	fmt.Printf("%-50s  %4s  %4s  %5s  %5s  %8s\n", "SYMBOL", "+", "-", "TOTAL", "SCORE", "CLUSTERS")
	fmt.Printf("%-50s  %4s  %4s  %5s  %5s  %8s\n", "------", "--", "--", "-----", "-----", "--------")

	count := 0
	for rows.Next() {
		var hashBytes []byte
		var qn string
		var pos, neg, total, clusters int
		if err := rows.Scan(&hashBytes, &qn, &pos, &neg, &total, &clusters); err != nil {
			return err
		}

		// Shorten qualified name for display.
		display := terminalSymbol(qn)
		if len(display) > 50 {
			display = display[:47] + "..."
		}

		score := 0.0
		if total > 0 {
			score = float64(pos) / float64(total)
		}

		fmt.Printf("%-50s  %4d  %4d  %5d  %5.2f  %8d\n", display, pos, neg, total, score, clusters)
		count++
	}

	if count == 0 {
		fmt.Println("No feedback records found.")
	} else {
		fmt.Printf("\nShowing %d symbols\n", count)
	}

	// Show per-cluster breakdown if filtering a specific symbol.
	if *symbol != "" && count > 0 {
		fmt.Printf("\n=== Per-Cluster Breakdown ===\n")
		clusterRows, err := st.DB().QueryContext(ctx,
			`SELECT f.keyword_cluster,
				SUM(CASE WHEN f.useful = 1 THEN 1 ELSE 0 END),
				SUM(CASE WHEN f.useful = 0 THEN 1 ELSE 0 END),
				COUNT(*)
			FROM feedback f
			LEFT JOIN nodes n ON f.symbol_hash = n.node_hash
			WHERE n.qualified_name LIKE ?
			GROUP BY f.keyword_cluster
			ORDER BY COUNT(*) DESC`,
			"%"+*symbol+"%",
		)
		if err != nil {
			return err
		}
		defer clusterRows.Close()

		fmt.Printf("%-66s  %4s  %4s  %5s\n", "CLUSTER", "+", "-", "TOTAL")
		fmt.Printf("%-66s  %4s  %4s  %5s\n", "-------", "--", "--", "-----")
		for clusterRows.Next() {
			var clusterBytes []byte
			var pos, neg, total int
			if err := clusterRows.Scan(&clusterBytes, &pos, &neg, &total); err != nil {
				return err
			}
			clusterStr := "(global)"
			if len(clusterBytes) > 0 {
				var h types.Hash
				copy(h[:], clusterBytes)
				clusterStr = h.String()[:16] + "..."
			}
			fmt.Printf("%-66s  %4d  %4d  %5d\n", clusterStr, pos, neg, total)
		}
	}

	return nil
}
