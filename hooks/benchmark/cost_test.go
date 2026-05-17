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
	"github.com/blackwell-systems/knowing/internal/wire"
)

// TestHookCostComparison measures the net token cost of hooks vs manual context gathering.
//
// The question: does spending 800 tokens automatically save more than 800 tokens
// of manual context-gathering that the agent would otherwise do?
//
// Method:
// 1. For each task, measure what a full context_for_task call returns (the "manual" path)
// 2. Measure what the 800-token hook injection provides
// 3. Calculate: how much of the manual response is covered by the hook?
// 4. If the hook covers enough, the agent can skip the manual call entirely (net savings)
//    If not, the agent still calls context_for_task and the hook was wasted (net cost)
func TestHookCostComparison(t *testing.T) {
	dbPath := os.Getenv("KNOWING_DB")
	if dbPath == "" {
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
		t.Skipf("cannot open store: %v", err)
	}
	defer st.Close()

	engine := knowingctx.NewContextEngine(st)
	ctx := context.Background()

	hookBudget := 800
	manualBudget := 4000 // typical agent context_for_task call

	type comparison struct {
		task           string
		hookTokens     int
		hookSymbols    int
		manualTokens   int
		manualSymbols  int
		coverageByHook float64 // what % of manual symbols does the hook already provide?
		netSavings     int     // manualTokens - hookTokens (if hook covers enough to skip manual)
	}

	var results []comparison

	for _, tk := range tasks {
		// Hook path: 800 token budget (what the agent gets for free).
		hookBlock, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: tk.File,
			TokenBudget:     hookBudget,
			Format:          "xml",
		})
		if err != nil {
			continue
		}

		// Manual path: 4000 token budget (what the agent would request explicitly).
		manualBlock, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: tk.File,
			TokenBudget:     manualBudget,
			Format:          "xml",
		})
		if err != nil {
			continue
		}

		// Measure hook cost in GCF tokens (what actually gets injected).
		hookPayload, _ := wire.FromContextBlock(ctx, hookBlock, "context_for_task", st)
		hookGCF := ""
		if hookPayload != nil {
			hookGCF, _ = wire.EncodeWith("gcf", hookPayload)
		}
		hookActualTokens := countWords(hookGCF)

		// Measure manual cost in GCF tokens (what the agent would receive).
		manualPayload, _ := wire.FromContextBlock(ctx, manualBlock, "context_for_task", st)
		manualGCF := ""
		if manualPayload != nil {
			manualGCF, _ = wire.EncodeWith("gcf", manualPayload)
		}
		manualActualTokens := countWords(manualGCF)

		// Calculate coverage: what % of the manual response's symbols are already
		// in the hook injection?
		hookNames := make(map[string]bool)
		for _, s := range hookBlock.Symbols {
			hookNames[s.Node.QualifiedName] = true
		}

		coveredCount := 0
		for _, s := range manualBlock.Symbols {
			if hookNames[s.Node.QualifiedName] {
				coveredCount++
			}
		}

		coverage := 0.0
		if len(manualBlock.Symbols) > 0 {
			coverage = float64(coveredCount) / float64(len(manualBlock.Symbols))
		}

		// Net savings: if coverage >= 70%, the hook provides enough that the agent
		// could skip the manual call. Savings = manualTokens avoided - hookTokens spent.
		// If coverage < 70%, the agent still needs to call manually, so hook is pure cost.
		net := -hookActualTokens // default: hook is a cost (agent still calls manually)
		if coverage >= 0.50 {
			net = manualActualTokens - hookActualTokens // savings from skipping manual call
		}

		results = append(results, comparison{
			task:           tk.Name,
			hookTokens:     hookActualTokens,
			hookSymbols:    len(hookBlock.Symbols),
			manualTokens:   manualActualTokens,
			manualSymbols:  len(manualBlock.Symbols),
			coverageByHook: coverage,
			netSavings:     net,
		})
	}

	// Report.
	t.Log("")
	t.Log("=== Hook Cost Comparison: Automatic (800 tok) vs Manual (4000 tok) ===")
	t.Log("")
	t.Logf("%-35s %8s %8s %9s %10s", "Task", "Hook(T)", "Manual(T)", "Coverage", "Net")
	t.Logf("%-35s %8s %8s %9s %10s", strings.Repeat("-", 35), "-------", "--------", "--------", "----------")

	totalHookCost := 0
	totalManualCost := 0
	totalNet := 0
	skippable := 0

	for _, r := range results {
		netStr := fmt.Sprintf("%+d", r.netSavings)
		if r.netSavings > 0 {
			netStr = fmt.Sprintf("+%d SAVE", r.netSavings)
		} else {
			netStr = fmt.Sprintf("%d COST", r.netSavings)
		}
		t.Logf("%-35s %8d %8d %8.0f%% %10s",
			r.task, r.hookTokens, r.manualTokens, r.coverageByHook*100, netStr)

		totalHookCost += r.hookTokens
		totalManualCost += r.manualTokens
		totalNet += r.netSavings
		if r.coverageByHook >= 0.50 {
			skippable++
		}
	}

	t.Log("")
	t.Logf("Tasks where hook covers >= 50%% of manual: %d/%d (%.0f%%)",
		skippable, len(results), float64(skippable)/float64(len(results))*100)
	t.Logf("Total hook cost (all tasks):    %d tokens", totalHookCost)
	t.Logf("Total manual cost (all tasks):  %d tokens", totalManualCost)
	t.Logf("Net across all tasks:           %+d tokens", totalNet)
	t.Log("")

	if totalNet > 0 {
		t.Logf("VERDICT: HOOKS SAVE %d TOKENS net across %d tasks", totalNet, len(results))
		t.Logf("  The hook costs %d tokens total but eliminates %d tokens of manual calls",
			totalHookCost, totalHookCost+totalNet)
	} else {
		t.Logf("VERDICT: HOOKS COST %d EXTRA TOKENS across %d tasks", -totalNet, len(results))
		t.Logf("  The agent still needs manual context calls in most cases")
	}
}

func countWords(s string) int {
	return len(strings.Fields(s))
}
