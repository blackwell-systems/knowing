package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdDebugSeeds shows the seed selection pipeline step by step for a given task.
// Usage: knowing debug-seeds -task "description" [-db path] <repo-path>
func cmdDebugSeeds(args []string) error {
	fs := flag.NewFlagSet("debug-seeds", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	task := fs.String("task", "", "Task description to debug (required)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *task == "" {
		return fmt.Errorf("usage: knowing debug-seeds -task \"description\" [-db path] <repo-path>")
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	repoPath, _ = filepath.Abs(repoPath)

	if *repoURL == "" {
		*repoURL = detectRepoURL(repoPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Count nodes to show graph size.
	repoHash := types.NewHash([]byte(*repoURL))
	files, _ := st.FilesByRepo(ctx, repoHash)
	var nodeCount int
	for _, f := range files {
		nodes, _ := st.NodesByFileHash(ctx, f.FileHash)
		nodeCount += len(nodes)
	}

	fmt.Printf("=== SEED DEBUG ===\n")
	fmt.Printf("Task: %q\n", *task)
	fmt.Printf("Repo: %s\n", *repoURL)
	fmt.Printf("Nodes: %d\n", nodeCount)
	fmt.Printf("\n")

	// Step 1: Extract keywords.
	ks := knowingctx.ExtractKeywordSetExported(*task)
	fmt.Printf("--- Step 1: Keywords ---\n")
	fmt.Printf("  Primary: %v\n", ks.Primary())
	fmt.Printf("  Components: %v\n", ks.Components)
	fmt.Printf("  Compounds: %v\n", ks.Compounds)
	fmt.Printf("\n")

	// Step 2: Extract path terms.
	pathTerms := knowingctx.ExtractPathTermsExported(*task)
	fmt.Printf("--- Step 2: Path Terms ---\n")
	fmt.Printf("  Terms: %v\n", pathTerms)
	fmt.Printf("\n")

	// Step 3: BM25 search (mirrors ForTask production path).
	fmt.Printf("--- Step 3: BM25 Results ---\n")
	ftsQuery := knowingctx.BuildFTSQueryExported(ks.Primary())
	fmt.Printf("  FTS Query: %s\n", ftsQuery)
	var bm25Nodes []types.Node
	if ftsQuery != "" {
		if nodes, err := st.SearchBM25Nodes(ctx, ftsQuery, 15); err == nil {
			bm25Nodes = nodes
		} else {
			fmt.Printf("  Error: %v\n", err)
		}
	}
	// Fallback decomposition: if primary query returned 0 results,
	// decompose compound keywords and retry (mirrors ForTask).
	if len(bm25Nodes) == 0 {
		decomposed := knowingctx.DecomposeCompoundsTargetedExported(ks.Primary())
		if decomposed == "" {
			decomposed = knowingctx.DecomposeCompoundsTargetedExported(ks.All())
		}
		if decomposed != "" {
			fmt.Printf("  (primary returned 0 results, decomposing compounds)\n")
			fmt.Printf("  Decomposed Query: %s\n", decomposed)
			if nodes, err := st.SearchBM25Nodes(ctx, decomposed, 15); err == nil {
				bm25Nodes = nodes
			} else {
				fmt.Printf("  Error: %v\n", err)
			}
		}
	}
	for i, n := range bm25Nodes {
		pkg := extractPkgFromQualified(n.QualifiedName)
		fmt.Printf("  [%2d] %s (pkg: %s)\n", i+1, terminalSymbol(n.QualifiedName), pkg)
	}
	fmt.Printf("\n")

	// Step 4: Show which BM25 results match path terms.
	if len(pathTerms) > 0 {
		fmt.Printf("--- Step 4: Path Boost Analysis ---\n")
		fmt.Printf("  Node count vs threshold: %d vs 50000 (boost %s)\n",
			nodeCount, map[bool]string{true: "ACTIVE", false: "INACTIVE"}[nodeCount > 50000])
		for i, n := range bm25Nodes {
			qnLower := strings.ToLower(n.QualifiedName)
			matches := []string{}
			for _, pt := range pathTerms {
				if strings.Contains(qnLower, strings.ToLower(pt)) {
					matches = append(matches, pt)
				}
			}
			boost := ""
			if len(matches) > 0 {
				boost = fmt.Sprintf(" BOOST(%s)", strings.Join(matches, ","))
			}
			fmt.Printf("  [%2d] %s%s\n", i+1, terminalSymbol(n.QualifiedName), boost)
			_ = i // suppress unused warning when bm25Nodes is empty
		}
		fmt.Printf("\n")
	}

	// Step 5: Run actual ForTask and show what it returns.
	fmt.Printf("--- Step 5: Final ForTask Top 10 ---\n")
	engine := knowingctx.NewContextEngine(st)
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: *task,
		TokenBudget:     5000,
		RepoURL:         *repoURL,
	})
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else if block != nil {
		for i, sym := range block.Symbols {
			if i >= 10 {
				break
			}
			fmt.Printf("  [%2d] %s (score: %.3f)\n", i+1, terminalSymbol(sym.Node.QualifiedName), sym.Score)
		}
		fmt.Printf("  Total symbols: %d\n", len(block.Symbols))
	}

	return nil
}

// terminalSymbol extracts the last meaningful part of a QualifiedName for display.
func terminalSymbol(qn string) string {
	// Strip repo prefix (everything before ://)
	if idx := strings.Index(qn, "://"); idx >= 0 {
		qn = qn[idx+3:]
	}
	// Strip file path (everything up to last file extension + dot)
	exts := []string{".go.", ".py.", ".ts.", ".tsx.", ".js.", ".rs.", ".java.", ".cs.", ".rb."}
	for _, ext := range exts {
		if idx := strings.LastIndex(qn, ext); idx >= 0 {
			return qn[idx+len(ext):]
		}
	}
	// Fallback: last slash component
	if idx := strings.LastIndex(qn, "/"); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}
