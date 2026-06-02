package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdDebugEquiv shows which equivalence classes match a task description.
// Usage: knowing debug-equiv -task "description" [-db path] <repo-path>
func cmdDebugEquiv(args []string) error {
	fs := flag.NewFlagSet("debug-equiv", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	task := fs.String("task", "", "Task description to match (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *task == "" {
		return fmt.Errorf("usage: knowing debug-equiv -task \"description\" [-db path] [repo-path]")
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	repoPath, _ = filepath.Abs(repoPath)

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	fmt.Printf("=== Equivalence Class Debug ===\n")
	fmt.Printf("Task: %q\n\n", *task)

	// Source 1: Hand-curated (seed + universal + language).
	allClasses := append(knowingctx.SeedEquivalenceClassesExported(), knowingctx.UniversalEquivalenceClassesExported()...)
	allClasses = append(allClasses, knowingctx.LanguageEquivalenceClassesExported()...)

	repoLang := knowingctx.DetectRepoLanguageExported(ctx, st)
	matches := knowingctx.MatchEquivalenceClassesLangExported(*task, allClasses, repoLang)

	fmt.Printf("--- Source 1: Hand-curated (%d matches) ---\n", len(matches))
	for _, m := range matches {
		phrases := m.Class.Phrases
		if len(phrases) > 5 {
			phrases = phrases[:5]
		}
		targets := m.Targets
		if len(targets) > 10 {
			targets = targets[:10]
		}
		fmt.Printf("  [%s] %s (weight=%.1f, lang=%q)\n", m.Class.Source, m.Class.Concept, m.Class.Weight, m.Class.Lang)
		fmt.Printf("    Phrases: %s\n", strings.Join(phrases, ", "))
		fmt.Printf("    Targets: %s\n", strings.Join(targets, ", "))
	}

	// Source 2: Graph-derived aliases (info only, needs ForTask for full resolution).
	fmt.Printf("\n--- Source 2: Graph-derived ---\n")
	fmt.Printf("  (Graph aliases are derived at query time from top tiered results.\n")
	fmt.Printf("   Run `knowing debug-seeds` to see which seeds feed graph alias generation.)\n")

	// Source 3: Learned vocab associations.
	ks := knowingctx.ExtractKeywordSetExported(*task)
	keywords := ks.All()
	lowerKeywords := make([]string, len(keywords))
	for i, kw := range keywords {
		lowerKeywords[i] = strings.ToLower(kw)
	}

	assocs, err := st.LearnedVocabAssociations(ctx, lowerKeywords, 2)
	fmt.Printf("\n--- Source 3: Learned vocab (%d confirmed associations) ---\n", len(assocs))
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if len(assocs) == 0 {
		display := lowerKeywords
		if len(display) > 5 {
			display = display[:5]
		}
		fmt.Printf("  (No learned associations with count >= 2 for keywords: %v)\n", display)
	} else {
		for _, a := range assocs {
			fmt.Printf("  %q -> %s (count=%d)\n", a.Keyword, a.SymbolName, a.Count)
		}
	}

	// Summary.
	fmt.Printf("\n--- Keywords extracted ---\n")
	fmt.Printf("  Primary: %v\n", ks.Primary())
	allDisplay := keywords
	if len(allDisplay) > 15 {
		allDisplay = allDisplay[:15]
	}
	fmt.Printf("  All: %v\n", allDisplay)
	fmt.Printf("  Repo language: %s\n", repoLang)

	return nil
}
