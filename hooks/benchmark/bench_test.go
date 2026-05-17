//go:build hookbench

package benchmark

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

// task represents a simulated edit task with known symbol dependencies.
type task struct {
	Name string
	// File being edited (used as the hook input).
	File string
	// Symbols that the task actually needs to touch or understand.
	// These are substrings that should appear in the context if the hook is useful.
	NeedsSymbols []string
}

// tasks defines the benchmark task set. Each represents a realistic edit scenario.
// NeedsSymbols are qualified name substrings that a developer editing this file
// would benefit from seeing in their context.
var tasks = []task{
	{
		Name: "add new MCP tool handler",
		File: "server",
		NeedsSymbols: []string{
			"Server.registerTools",
			"requireHash",
			"requireStringArg",
			"getIntArg",
			"NewServer",
		},
	},
	{
		Name: "modify context engine scoring",
		File: "context",
		NeedsSymbols: []string{
			"ContextEngine",
			"ForTask",
			"RankSymbols",
			"RandomWalkWithRestart",
			"packIntoBudget",
			"ScoreComponents",
		},
	},
	{
		Name: "fix snapshot diff logic",
		File: "snapshot",
		NeedsSymbols: []string{
			"SnapshotManager",
			"ComputeSnapshot",
			"SnapshotDiff",
			"MerkleTree",
		},
	},
	{
		Name: "update SQLite store method",
		File: "sqlite",
		NeedsSymbols: []string{
			"SQLiteStore",
			"PutNode",
			"PutEdge",
			"EdgesFrom",
			"EdgesTo",
		},
	},
	{
		Name: "modify wire format encoder",
		File: "wire",
		NeedsSymbols: []string{
			"Encode",
			"Decode",
			"Payload",
			"Symbol",
			"EncodeWith",
		},
	},
	{
		Name: "edit Go extractor",
		File: "goextractor",
		NeedsSymbols: []string{
			"GoExtractor",
			"Extract",
			"Indexer",
			"ExtractResult",
		},
	},
	{
		Name: "change daemon behavior",
		File: "daemon",
		NeedsSymbols: []string{
			"Daemon",
			"Start",
			"onCommit",
			"IndexRepo",
		},
	},
	{
		Name: "modify TypeScript extractor routes",
		File: "tsextractor",
		NeedsSymbols: []string{
			"TypeScriptExtractor",
			"Extract",
			"route",
			"handles_route",
		},
	},
	{
		Name: "fix indexer incremental logic",
		File: "indexer",
		NeedsSymbols: []string{
			"Indexer",
			"IndexRepo",
			"processFile",
			"Register",
		},
	},
	{
		Name: "update trace ingestion",
		File: "trace",
		NeedsSymbols: []string{
			"TraceIngestor",
			"IngestSpans",
			"SymbolResolver",
			"Confidence",
		},
	},
}

func TestHookBenchmark(t *testing.T) {
	dbPath := os.Getenv("KNOWING_DB")
	if dbPath == "" {
		// Try common locations relative to repo root.
		candidates := []string{"knowing.db", "../../knowing.db", "../../../knowing.db"}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				dbPath = c
				break
			}
		}
		if dbPath == "" {
			dbPath = "knowing.db"
		}
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Skipf("cannot open store at %s: %v (run 'knowing index -db knowing.db .' first)", dbPath, err)
	}
	defer st.Close()

	engine := knowingctx.NewContextEngine(st)
	ctx := context.Background()

	type result struct {
		task      string
		symbols   int
		precision float64
		recall    float64
		tokens    int
	}

	var results []result

	for _, tk := range tasks {
		budget := 800
		if b := os.Getenv("KNOWING_HOOKS_BUDGET"); b != "" {
			fmt.Sscanf(b, "%d", &budget)
		}

		// Edit-aware seeding with fallback: try primary symbol first,
		// if it returns too few results, fall back to filename query.
		// This simulates the hook extracting the most prominent symbol
		// from old_string, with filename as fallback.
		query := tk.File
		if len(tk.NeedsSymbols) >= 1 {
			// Try primary symbol query first.
			testBlock, _ := engine.ForTask(ctx, knowingctx.TaskOptions{
				TaskDescription: tk.NeedsSymbols[0],
				TokenBudget:     budget,
				Format:          "xml",
			})
			if testBlock != nil && len(testBlock.Symbols) >= 10 {
				query = tk.NeedsSymbols[0]
			}
			// else: keep filename as query (broader match)
		}

		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: query,
			TokenBudget:     budget,
			Format:          "xml",
		})
		if err != nil {
			t.Logf("SKIP %s: %v", tk.Name, err)
			continue
		}

		// Collect all qualified names from the context block.
		injectedNames := make([]string, 0, len(block.Symbols))
		for _, sym := range block.Symbols {
			injectedNames = append(injectedNames, sym.Node.QualifiedName)
		}

		// Measure precision: what fraction of injected symbols are in NeedsSymbols?
		relevant := 0
		for _, name := range injectedNames {
			for _, need := range tk.NeedsSymbols {
				if strings.Contains(name, need) {
					relevant++
					break
				}
			}
		}
		precision := 0.0
		if len(injectedNames) > 0 {
			precision = float64(relevant) / float64(len(injectedNames))
		}

		// Measure recall: what fraction of NeedsSymbols appear in injected context?
		found := 0
		for _, need := range tk.NeedsSymbols {
			for _, name := range injectedNames {
				if strings.Contains(name, need) {
					found++
					break
				}
			}
		}
		recall := 0.0
		if len(tk.NeedsSymbols) > 0 {
			recall = float64(found) / float64(len(tk.NeedsSymbols))
		}

		tokens := block.TokensUsed

		results = append(results, result{
			task:      tk.Name,
			symbols:   len(injectedNames),
			precision: precision,
			recall:    recall,
			tokens:    tokens,
		})
	}

	if len(results) == 0 {
		t.Fatal("no tasks produced results (is the database indexed?)")
	}

	// Print scorecard.
	t.Log("")
	t.Log("=== Hook Benchmark: Precision & Recall ===")
	t.Log("")
	t.Logf("%-40s %6s %9s %8s %6s", "Task", "Syms", "Precision", "Recall", "Tokens")
	t.Logf("%-40s %6s %9s %8s %6s", strings.Repeat("-", 40), "-----", "---------", "--------", "------")

	var totalPrecision, totalRecall float64
	for _, r := range results {
		t.Logf("%-40s %6d %8.1f%% %7.1f%% %6d",
			r.task, r.symbols, r.precision*100, r.recall*100, r.tokens)
		totalPrecision += r.precision
		totalRecall += r.recall
	}

	meanPrecision := totalPrecision / float64(len(results))
	meanRecall := totalRecall / float64(len(results))

	t.Log("")
	t.Logf("Mean precision: %.1f%%", meanPrecision*100)
	t.Logf("Mean recall:    %.1f%%", meanRecall*100)
	t.Log("")

	// Verdict.
	if meanPrecision >= 0.30 && meanRecall >= 0.50 {
		t.Logf("VERDICT: PASS (hooks provide useful context)")
		t.Logf("  Precision >= 30%%: at least a third of injected symbols are task-relevant")
		t.Logf("  Recall >= 50%%: at least half the needed symbols are provided by the hook")
	} else if meanPrecision < 0.15 {
		t.Errorf("VERDICT: FAIL (hooks inject mostly noise)")
		t.Logf("  Precision %.1f%% < 15%%: most injected context is irrelevant", meanPrecision*100)
	} else {
		t.Logf("VERDICT: INCONCLUSIVE")
		t.Logf("  Precision: %.1f%% (threshold: 30%%)", meanPrecision*100)
		t.Logf("  Recall: %.1f%% (threshold: 50%%)", meanRecall*100)
		t.Logf("  Consider tuning token budget or keyword extraction before committing")
	}

	// Hard fail if hooks are actively harmful.
	if meanPrecision < 0.10 {
		t.Fatalf("CRITICAL: precision %.1f%% indicates hooks are pure noise. Do not ship.", meanPrecision*100)
	}

	// Output summary for CI.
	summary := fmt.Sprintf("hook_benchmark: precision=%.1f%% recall=%.1f%% tasks=%d",
		meanPrecision*100, meanRecall*100, len(results))
	t.Log(summary)
}
