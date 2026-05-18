package main

import (
	stdctx "context"
	"flag"
	"fmt"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdWhy explains why a symbol ranked where it did for a given task.
// Shows the full scoring breakdown: seed tier, RWR score, HITS authority,
// blast radius, confidence, recency, session boost, feedback weight.
func cmdWhy(args []string) error {
	fs := flag.NewFlagSet("why", flag.ExitOnError)
	task := fs.String("task", "", "Task description (required)")
	symbol := fs.String("symbol", "", "Symbol name to explain (required)")
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Accept positional args: knowing why <symbol> -task "..."
	if *symbol == "" && fs.NArg() > 0 {
		*symbol = fs.Arg(0)
	}

	if *task == "" {
		return fmt.Errorf("--task is required")
	}
	if *symbol == "" {
		return fmt.Errorf("symbol name is required (positional or --symbol)")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	engine := knowingctx.NewContextEngine(st)
	ctx := stdctx.Background()

	result, err := engine.ExplainSymbol(ctx, *task, *symbol)
	if err != nil {
		return err
	}

	printExplain(result)
	return nil
}

func printExplain(r *knowingctx.ExplainResult) {
	fmt.Printf("knowing why: %s\n", r.Symbol.QualifiedName)
	fmt.Printf("  Kind: %s\n", r.Symbol.Kind)
	if r.Symbol.Line > 0 {
		fmt.Printf("  Line: %d\n", r.Symbol.Line)
	}
	fmt.Println()

	if r.Rank < 0 {
		fmt.Printf("  NOT RANKED (not in top results for this task)\n")
		fmt.Printf("  Reason: not reached by seed matching or RWR walk\n")
		if r.RWRScore > 0 {
			fmt.Printf("  RWR score: %.4f (below 0.02 threshold)\n", r.RWRScore)
		}
		fmt.Printf("  Keywords extracted: %s\n", strings.Join(r.Keywords, ", "))
		return
	}

	fmt.Printf("  Rank: #%d of %d symbols\n", r.Rank, r.TotalSymbols)
	fmt.Printf("  Total score: %.4f\n", r.TotalScore)
	fmt.Println()

	// Seed discovery
	fmt.Println("  Discovery:")
	if r.IsSeed {
		fmt.Printf("    Seed: yes (direct keyword match)\n")
	} else {
		fmt.Printf("    Seed: no (reached via graph walk)\n")
	}
	fmt.Printf("    Channel: %s\n", r.SeedChannel)
	if r.SeedTier != "" {
		fmt.Printf("    Tier: %s\n", r.SeedTier)
	}
	if len(r.EquivMatches) > 0 {
		fmt.Printf("    Equivalence classes: %s\n", strings.Join(r.EquivMatches, ", "))
	}
	fmt.Printf("    Keywords: %s\n", strings.Join(r.Keywords, ", "))
	fmt.Println()

	// Score components
	fmt.Println("  Score breakdown:")
	fmt.Printf("    Blast radius:  %.4f  (caller proxy %d, max %d)\n",
		r.Components.BlastRadius, r.CallerProxy, r.MaxCallers)
	fmt.Printf("    Confidence:    %.4f\n", r.Components.Confidence)
	fmt.Printf("    Recency:       %.4f\n", r.Components.Recency)
	fmt.Printf("    Distance:      %.4f  (distance=%d)\n",
		r.Components.Distance, boolToInt(!r.IsSeed))
	if r.Components.Feedback != 0 {
		fmt.Printf("    Feedback:      %+.4f\n", r.Components.Feedback)
	}
	if r.Components.Session != 0 {
		fmt.Printf("    Session:       %.4f\n", r.Components.Session)
	}
	fmt.Println()

	// Graph signals
	fmt.Println("  Graph signals:")
	fmt.Printf("    RWR score:     %.4f\n", r.RWRScore)
	if r.HITSAuthority > 0 || r.HITSHub > 0 {
		fmt.Printf("    HITS authority: %.4f\n", r.HITSAuthority)
		fmt.Printf("    HITS hub:       %.4f\n", r.HITSHub)
		if r.HITSAdjust != 0 {
			fmt.Printf("    HITS adjust:    %+.4f\n", r.HITSAdjust)
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
