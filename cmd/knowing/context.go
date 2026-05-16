package main

import (
	stdctx "context"
	"flag"
	"fmt"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdContext generates graph-aware context for a task description or set of
// changed files. It queries the knowledge graph, ranks symbols by blast radius,
// confidence, recency, and distance, then formats the output within a token budget.
func cmdContext(args []string) error {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	task := fs.String("task", "", "Task description for context generation")
	files := fs.String("files", "", "Comma-separated list of changed file paths")
	budget := fs.Int("budget", 50000, "Token budget")
	format := fs.String("format", "xml", "Output format (xml/markdown/json)")
	dbPath := fs.String("db", "knowing.db", "Path to SQLite database")
	repo := fs.String("repo", "", "Repository URL for file resolution")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Validate: either -task or -files must be provided, but not both.
	if *task == "" && *files == "" {
		return fmt.Errorf("either --task or --files must be specified")
	}
	if *task != "" && *files != "" {
		return fmt.Errorf("specify either --task or --files, not both")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	engine := knowingctx.NewContextEngine(st)
	ctx := stdctx.Background()

	var block *knowingctx.ContextBlock

	if *task != "" {
		block, err = engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: *task,
			TokenBudget:     *budget,
			Format:          *format,
		})
	} else {
		fileList := strings.Split(*files, ",")
		block, err = engine.ForFiles(ctx, knowingctx.FileOptions{
			Files:       fileList,
			RepoURL:     *repo,
			TokenBudget: *budget,
			Format:      *format,
		})
	}
	if err != nil {
		return fmt.Errorf("generating context: %w", err)
	}

	output, err := knowingctx.FormatContextBlock(block, *format)
	if err != nil {
		return fmt.Errorf("formatting context: %w", err)
	}

	fmt.Print(output)
	return nil
}
