package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdDebugVocab shows learned vocabulary associations from the vocab_associations table.
// Usage: knowing debug-vocab [-db path] [-keyword filter] [-min-count N] [-task "description"]
func cmdDebugVocab(args []string) error {
	fs := flag.NewFlagSet("debug-vocab", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	keyword := fs.String("keyword", "", "Filter by keyword (empty = show all)")
	minCount := fs.Int("min-count", 1, "Minimum observation count to display")
	top := fs.Int("top", 50, "Maximum number of associations to display")
	task := fs.String("task", "", "Preview vocab filter for a task description (shows which keywords would be recorded)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Task filter preview mode: show which keywords pass/fail the vocab filter.
	if *task != "" {
		ks := knowingctx.ExtractKeywordSet(*task)
		all := ks.All()
		var worthy, filtered []string
		for _, kw := range all {
			lkw := strings.ToLower(kw)
			if knowingctx.IsVocabWorthy(lkw) {
				worthy = append(worthy, kw)
			} else {
				filtered = append(filtered, kw)
			}
		}
		fmt.Printf("=== Vocab Filter Preview ===\n")
		fmt.Printf("Task: %s\n\n", *task)
		fmt.Printf("Vocab-worthy (%d):\n", len(worthy))
		for _, kw := range worthy {
			fmt.Printf("  + %s\n", kw)
		}
		fmt.Printf("\nFiltered out (%d):\n", len(filtered))
		for _, kw := range filtered {
			fmt.Printf("  - %s\n", kw)
		}
		fmt.Printf("\nFilter rate: %d/%d keywords removed (%.0f%%)\n",
			len(filtered), len(all), float64(len(filtered))/float64(len(all))*100)
		return nil
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Query associations.
	var keywords []string
	if *keyword != "" {
		keywords = []string{*keyword}
	}

	// If no keyword filter, query all by fetching distinct keywords first.
	if len(keywords) == 0 {
		rows, err := st.DB().QueryContext(ctx,
			`SELECT DISTINCT keyword FROM vocab_associations WHERE count >= ? ORDER BY keyword`,
			*minCount,
		)
		if err != nil {
			return fmt.Errorf("querying keywords: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var kw string
			if err := rows.Scan(&kw); err != nil {
				return err
			}
			keywords = append(keywords, kw)
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}

	if len(keywords) == 0 {
		fmt.Println("No vocabulary associations found.")
		return nil
	}

	assocs, err := st.LearnedVocabAssociations(ctx, keywords, *minCount)
	if err != nil {
		return fmt.Errorf("querying associations: %w", err)
	}

	if len(assocs) == 0 {
		fmt.Println("No vocabulary associations found.")
		return nil
	}

	// Display.
	fmt.Printf("=== Learned Vocabulary Associations ===\n")
	fmt.Printf("Filter: keyword=%q  min-count=%d\n\n", *keyword, *minCount)
	fmt.Printf("%-20s  %-30s  %5s\n", "KEYWORD", "SYMBOL", "COUNT")
	fmt.Printf("%-20s  %-30s  %5s\n", "-------", "------", "-----")

	shown := 0
	for _, a := range assocs {
		if shown >= *top {
			fmt.Printf("\n... truncated at %d (use -top to show more)\n", *top)
			break
		}
		fmt.Printf("%-20s  %-30s  %5d\n", a.Keyword, a.SymbolName, a.Count)
		shown++
	}

	fmt.Printf("\nTotal: %d associations across %d keywords\n", len(assocs), len(keywords))
	return nil
}
