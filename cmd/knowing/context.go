package main

import (
	stdctx "context"
	"flag"
	"fmt"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/wire"
)

// cmdContext generates graph-aware context for a task description or set of
// changed files. It queries the knowledge graph, ranks symbols by blast radius,
// confidence, recency, and distance, then formats the output within a token budget.
func cmdContext(args []string) error {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	task := fs.String("task", "", "Task description for context generation")
	files := fs.String("files", "", "Comma-separated list of changed file paths")
	budget := fs.Int("budget", 50000, "Token budget")
	format := fs.String("format", "xml", "Output format (gcf/gcb/json/xml/markdown)")
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
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

	var output string
	switch *format {
	case "gcf", "gcb", "json":
		tool := "context_for_task"
		if *task == "" {
			tool = "context_for_files"
		}
		payload, pErr := wire.FromContextBlock(ctx, block, tool, st)
		if pErr != nil {
			return fmt.Errorf("building wire payload: %w", pErr)
		}
		output, err = wire.EncodeWith(*format, payload)
	default:
		output, err = knowingctx.FormatContextBlock(block, *format)
	}
	if err != nil {
		return fmt.Errorf("formatting context: %w", err)
	}

	fmt.Print(output)
	return nil
}
