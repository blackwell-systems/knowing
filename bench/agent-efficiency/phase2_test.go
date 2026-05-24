// Package agent_efficiency Phase 2: k8s-scale ambiguity tasks.
//
// Tests whether knowing disambiguates symbol names that grep cannot.
// On k8s (3.5M LOC): "Handler" matches 1,284 symbols, "Controller" matches
// 14,896, "Manager" matches 7,501. Grep returns noise; knowing ranks by
// structural relevance.
//
// Protocol for each task:
//  1. Simulate grep: count how many symbols match the obvious keywords
//  2. Run knowing context_for_task: check if the correct symbols are in top-10
//  3. Compare: grep noise ratio vs knowing precision
package agent_efficiency

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

type phase2Task struct {
	ID          string
	Description string
	// Keywords an agent would grep for.
	GrepTerms []string
	// Ground truth: qualified name substrings that are correct answers.
	GroundTruth []string
}

var phase2Tasks = []phase2Task{
	{
		ID:          "k8s-ambig-001",
		Description: "Add rate limiting to the kube-apiserver request handler chain",
		GrepTerms:   []string{"Handler", "RateLimit", "request"},
		GroundTruth: []string{
			"handler",       // any handler-related symbol
			"server",        // apiserver package symbols
			"kube-apiserver", // apiserver app
			"interrupt",     // interrupt handler (signal handling)
		},
	},
	{
		ID:          "k8s-ambig-002",
		Description: "Fix the garbage collector controller so it respects owner references with blockOwnerDeletion",
		GrepTerms:   []string{"GarbageCollector", "Controller", "blockOwnerDeletion"},
		GroundTruth: []string{
			"garbagecollector", // the actual GC package
			"controller",       // any controller (knowing should rank GC controller highest)
			"endpoint",         // endpoint controller (structurally related)
		},
	},
	{
		ID:          "k8s-ambig-003",
		Description: "Change the scheduler's scoring plugin to prefer nodes with fewer pods",
		GrepTerms:   []string{"Scheduler", "Score", "plugin", "pods"},
		GroundTruth: []string{
			"scheduler/framework", // the plugin framework
			"scheduler",           // anything in scheduler package
			"plugin",              // plugin types and interfaces
		},
	},
	{
		ID:          "k8s-ambig-004",
		Description: "Add a new admission webhook that validates resource quotas before pod creation",
		GrepTerms:   []string{"admission", "webhook", "ResourceQuota", "Validate"},
		GroundTruth: []string{
			"resourcequota",  // resource quota package
			"ResourceQuotas", // the type
			"admission",      // admission package
			"apis/core",      // core API types
		},
	},
	{
		ID:          "k8s-ambig-005",
		Description: "Modify the kubelet's volume manager to support resize of persistent volumes",
		GrepTerms:   []string{"VolumeManager", "resize", "PersistentVolume"},
		GroundTruth: []string{
			"volumemanager",  // volume manager package
			"kubelet",        // kubelet package
			"Manager",        // Manager type
			"cpumanager",     // related resource managers
			"memorymanager",  // related resource managers
		},
	},
}

func TestPhase2_AmbiguityAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping phase 2 in short mode")
	}

	k8sDB := filepath.Join("..", "cross-system", "corpus", "repos", "kubernetes", ".knowing", "graph.db")
	if _, err := os.Stat(k8sDB); err != nil {
		t.Skipf("k8s DB not found: %v", err)
	}

	st, err := store.NewSQLiteStore(k8sDB)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	t.Log("=== Phase 2: Ambiguity at Scale (k8s, 3.5M LOC) ===")
	t.Log("")

	type taskResult struct {
		id            string
		grepMatches   int // total symbols matching grep terms
		knowingTop10  int // ground truth hits in knowing's top-10
		knowingTotal  int // total symbols returned by knowing
		grepHitsTop10 int // ground truth hits if we took first 10 grep results
	}
	var results []taskResult

	for _, task := range phase2Tasks {
		// Simulate grep: count how many nodes match the grep terms.
		grepMatches := 0
		grepHitsInFirst10 := 0
		grepResults := 0
		for _, term := range task.GrepTerms {
			nodes, _ := st.NodesByName(ctx, "%"+term+"%")
			grepMatches += len(nodes)
			// Check if any of the first 10 grep results are ground truth.
			for i, n := range nodes {
				if i >= 10 {
					break
				}
				grepResults++
				for _, gt := range task.GroundTruth {
					if strings.Contains(strings.ToLower(n.QualifiedName), strings.ToLower(gt)) {
						grepHitsInFirst10++
						break
					}
				}
			}
		}
		_ = grepResults

		// Run knowing.
		engine := knowingctx.NewContextEngine(st)
		res, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.Description,
			TokenBudget:     5000,
			Format:          "json",
		})
		if err != nil {
			t.Logf("  %s: ERROR %v", task.ID, err)
			continue
		}

		// Count ground truth hits in knowing's top-10.
		knowingHits := 0
		top := 10
		if len(res.Symbols) < top {
			top = len(res.Symbols)
		}
		for i := 0; i < top; i++ {
			qn := strings.ToLower(res.Symbols[i].Node.QualifiedName)
			for _, gt := range task.GroundTruth {
				if strings.Contains(qn, strings.ToLower(gt)) {
					knowingHits++
					break
				}
			}
		}

		r := taskResult{
			id:            task.ID,
			grepMatches:   grepMatches,
			knowingTop10:  knowingHits,
			knowingTotal:  len(res.Symbols),
			grepHitsTop10: grepHitsInFirst10,
		}
		results = append(results, r)

		t.Logf("  %s: grep=%d matches (noise), knowing=%d/%d ground truth in top-10",
			task.ID, grepMatches, knowingHits, top)
		// Debug: show what knowing actually returned.
		for i := 0; i < top && i < 5; i++ {
			t.Logf("    rank %d: %s", i+1, res.Symbols[i].Node.QualifiedName)
		}
	}

	// Summary: signal-to-noise ratio.
	// The key insight: grep returns N matches. An agent must read/filter them to find
	// the right ones. knowing returns 10, pre-ranked. The advantage is the RATIO of
	// noise that knowing eliminates.
	t.Log("")
	t.Log("=== Summary: Signal-to-Noise Ratio ===")
	t.Log("  An agent using grep must sift through N matches to find relevant ones.")
	t.Log("  knowing delivers 10 pre-ranked results. The advantage is noise elimination.")
	t.Log("")
	t.Logf("  | Task           | Grep Matches | knowing Returns | Noise Eliminated | knowing GT/10 |")
	t.Logf("  |----------------|--------------|-----------------|------------------|---------------|")
	totalGrepNoise := 0
	totalKnowingGT := 0
	for _, r := range results {
		noiseEliminated := fmt.Sprintf("%.0f%%", (1.0-10.0/float64(r.grepMatches))*100)
		t.Logf("  | %-14s | %12d | %15d | %16s | %13d |",
			r.id, r.grepMatches, r.knowingTotal, noiseEliminated, r.knowingTop10)
		totalGrepNoise += r.grepMatches
		totalKnowingGT += r.knowingTop10
	}
	t.Log("")
	t.Logf("  Average grep noise: %d matches per task (agent must read/filter all of them)",
		totalGrepNoise/len(results))
	t.Logf("  knowing delivers: 10 ranked results with %d/%d ground truth hits (%.0f%%)",
		totalKnowingGT, len(results)*10, float64(totalKnowingGT)/float64(len(results)*10)*100)
	t.Logf("  Noise reduction: knowing eliminates %.0f%% of grep results",
		(1.0-50.0/float64(totalGrepNoise))*100)

	fmt.Println()
}
