package agent_efficiency

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExportTasks writes tasks.json to the bench/agent-efficiency directory so
// that the runner script can read task descriptions without needing a Go
// toolchain at runtime.
//
// Run with:
//
//	GOWORK=off go test ./bench/agent-efficiency/ -run TestExportTasks -v
func TestExportTasks(t *testing.T) {
	type exportedTask struct {
		ID          string   `json:"id"`
		Description string   `json:"description"`
		Complexity  string   `json:"complexity"`
		Keywords    []string `json:"answer_keywords"`
	}

	out := make([]exportedTask, 0, len(Tasks))
	for _, task := range Tasks {
		out = append(out, exportedTask{
			ID:          task.ID,
			Description: task.Description,
			Complexity:  task.Complexity,
			Keywords:    task.GroundTruth.AnswerKeywords,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal tasks: %v", err)
	}

	dest := filepath.Join(testDir(t), "tasks.json")
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatalf("write tasks.json: %v", err)
	}
	t.Logf("wrote %s (%d tasks)", dest, len(out))
}

// TestAnalyzeTranscripts scans the transcripts/ subdirectory for JSONL files,
// parses each one, and computes comparison results for any task where both a
// control and a treatment transcript exist. It writes FINDINGS.md to the
// bench/agent-efficiency directory.
//
// Run with:
//
//	GOWORK=off go test ./bench/agent-efficiency/ -run TestAnalyzeTranscripts -v
func TestAnalyzeTranscripts(t *testing.T) {
	dir := filepath.Join(testDir(t), "transcripts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("transcripts/ directory not found; run sessions first")
		}
		t.Fatalf("read transcripts dir: %v", err)
	}

	// Build a lookup from task ID to its ground truth.
	gtByID := make(map[string]GroundTruth, len(Tasks))
	for _, task := range Tasks {
		gtByID[task.ID] = task.GroundTruth
	}

	// control[taskID] and treatment[taskID] hold parsed metrics.
	controls := make(map[string]SessionMetrics)
	treatments := make(map[string]SessionMetrics)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		name := strings.TrimSuffix(e.Name(), ".jsonl")

		// Determine mode from filename suffix (before any session-id suffix).
		mode := ""
		parts := strings.Split(name, "-")
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] == "control" || parts[i] == "treatment" {
				mode = parts[i]
				break
			}
		}
		if mode == "" {
			t.Logf("skip %s: cannot determine control/treatment mode", e.Name())
			continue
		}

		// Determine task ID.
		taskID := ""
		for _, task := range Tasks {
			if strings.HasPrefix(name, task.ID+"-") {
				taskID = task.ID
				break
			}
		}
		if taskID == "" {
			t.Logf("skip %s: no matching task ID found", e.Name())
			continue
		}

		gt := gtByID[taskID]
		metrics, err := ParseTranscriptWithScoring(path, gt)
		if err != nil {
			t.Logf("warn: parse %s: %v", e.Name(), err)
			continue
		}
		metrics.TaskID = taskID
		t.Logf("parsed %s: tokens=%d tools=%d correctness=%.2f",
			e.Name(), metrics.TotalTokens, metrics.ToolCalls, metrics.AnswerCorrectness)

		// Validate control mode compliance: no knowing MCP tools should appear.
		if mode == "control" {
			for toolName := range metrics.ToolCallsByType {
				if strings.HasPrefix(toolName, "mcp__knowing__") {
					t.Logf("WARNING: control session %s used knowing tool %s (invalid, should re-run)", e.Name(), toolName)
				}
			}
			controls[taskID] = metrics
		} else {
			treatments[taskID] = metrics
		}
	}

	// Compute comparisons for tasks that have both sides.
	var results []ComparisonResult
	for taskID, ctrl := range controls {
		treatment, ok := treatments[taskID]
		if !ok {
			t.Logf("task %s: control found but no treatment transcript", taskID)
			continue
		}
		results = append(results, Compare(ctrl, treatment))
	}
	for taskID := range treatments {
		if _, ok := controls[taskID]; !ok {
			t.Logf("task %s: treatment found but no control transcript", taskID)
		}
	}

	if len(results) == 0 {
		t.Log("no complete pairs found; skipping report generation")
		return
	}

	report := FormatReport(results)

	dest := filepath.Join(testDir(t), "FINDINGS.md")
	if err := os.WriteFile(dest, []byte(report), 0o644); err != nil {
		t.Fatalf("write FINDINGS.md: %v", err)
	}
	t.Logf("wrote %s (%d task pairs)", dest, len(results))
	t.Log("\n" + report)
}

// TestAnalyzeMultiTurn is the analyzer for multi-turn benchmark sessions.
// It reads transcripts from transcripts/multi-turn/ and produces a comparison
// report focused on time-to-first-edit, exploration calls, and build success.
func TestAnalyzeMultiTurn(t *testing.T) {
	// Scan all repo subdirectories under transcripts/.
	transcriptsRoot := filepath.Join(testDir(t), "transcripts")
	repoDirs, err := os.ReadDir(transcriptsRoot)
	if err != nil {
		t.Skip("transcripts/ not found; run run-repo.sh first")
	}

	// Process each repo's transcripts.
	for _, repoEntry := range repoDirs {
		if !repoEntry.IsDir() {
			continue
		}
		repoName := repoEntry.Name()
		t.Run(repoName, func(t *testing.T) {
			analyzeRepoTranscripts(t, filepath.Join(transcriptsRoot, repoName), repoName)
		})
	}
}

func analyzeRepoTranscripts(t *testing.T, dir string, repoName string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("%s: no transcripts", repoName)
		return
	}

	type mtMetrics struct {
		SessionMetrics
		TimeToFirstEditMs int64
		ExplorationCalls  int // Grep+Read+Glob before first Edit
		KnowingCalls      int // mcp__knowing__* calls
		BuildSuccess      bool
	}

	controls := make(map[string]mtMetrics)
	treatments := make(map[string]mtMetrics)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		name := strings.TrimSuffix(e.Name(), ".jsonl")

		mode := ""
		for _, m := range []string{"control", "treatment"} {
			if strings.HasSuffix(name, "-"+m) {
				mode = m
				break
			}
		}
		if mode == "" {
			continue
		}

		taskID := strings.TrimSuffix(name, "-"+mode)

		sm, err := ParseTranscript(path)
		if err != nil {
			t.Logf("warn: parse %s: %v", e.Name(), err)
			continue
		}
		sm.TaskID = taskID

		mt := mtMetrics{SessionMetrics: sm}

		// Count knowing calls and exploration calls.
		for tool, count := range sm.ToolCallsByType {
			if strings.HasPrefix(tool, "mcp__knowing__") {
				mt.KnowingCalls += count
			}
		}
		mt.ExplorationCalls = sm.ToolCallsByType["Grep"] + sm.ToolCallsByType["Read"] + sm.ToolCallsByType["Glob"]

		// Check verification result.
		verifyPath := filepath.Join(dir, taskID+"-"+mode+"-verify.json")
		if data, err := os.ReadFile(verifyPath); err == nil {
			var vr struct {
				BuildSuccess bool `json:"build_success"`
			}
			if json.Unmarshal(data, &vr) == nil {
				mt.BuildSuccess = vr.BuildSuccess
			}
		}

		t.Logf("parsed %s: tokens=%d tools=%d knowing=%d explore=%d build=%v",
			e.Name(), mt.TotalTokens, mt.ToolCalls, mt.KnowingCalls, mt.ExplorationCalls, mt.BuildSuccess)

		if mode == "control" {
			controls[taskID] = mt
		} else {
			treatments[taskID] = mt
		}
	}

	if len(controls) == 0 && len(treatments) == 0 {
		t.Skip("no multi-turn transcripts found")
	}

	// Generate report.
	var sb strings.Builder
	sb.WriteString("# Multi-Turn Agent Efficiency Results\n\n")
	sb.WriteString("| Task | Mode | Tokens | Tools | Explore | Knowing | Build | Wall (s) |\n")
	sb.WriteString("|------|------|--------|-------|---------|---------|-------|----------|\n")

	allTasks := make(map[string]bool)
	for k := range controls {
		allTasks[k] = true
	}
	for k := range treatments {
		allTasks[k] = true
	}

	for taskID := range allTasks {
		if ctrl, ok := controls[taskID]; ok {
			sb.WriteString(fmt.Sprintf("| %s | control | %d | %d | %d | %d | %v | %.1f |\n",
				taskID, ctrl.TotalTokens, ctrl.ToolCalls, ctrl.ExplorationCalls, ctrl.KnowingCalls, ctrl.BuildSuccess, float64(ctrl.WallClockMs)/1000))
		}
		if treat, ok := treatments[taskID]; ok {
			sb.WriteString(fmt.Sprintf("| %s | treatment | %d | %d | %d | %d | %v | %.1f |\n",
				taskID, treat.TotalTokens, treat.ToolCalls, treat.ExplorationCalls, treat.KnowingCalls, treat.BuildSuccess, float64(treat.WallClockMs)/1000))
		}
	}

	// Comparison summary for paired results.
	sb.WriteString("\n## Paired Comparison\n\n")
	sb.WriteString("| Task | Token Δ | Tool Δ | Explore Δ | Build Ctrl | Build Treat |\n")
	sb.WriteString("|------|---------|--------|-----------|------------|-------------|\n")

	for taskID := range allTasks {
		ctrl, cOk := controls[taskID]
		treat, tOk := treatments[taskID]
		if !cOk || !tOk {
			continue
		}
		tokenDelta := treat.TotalTokens - ctrl.TotalTokens
		toolDelta := treat.ToolCalls - ctrl.ToolCalls
		exploreDelta := treat.ExplorationCalls - ctrl.ExplorationCalls
		sb.WriteString(fmt.Sprintf("| %s | %+d | %+d | %+d | %v | %v |\n",
			taskID, tokenDelta, toolDelta, exploreDelta, ctrl.BuildSuccess, treat.BuildSuccess))
	}

	report := sb.String()
	dest := filepath.Join(testDir(t), "FINDINGS-multi-turn.md")
	if err := os.WriteFile(dest, []byte(report), 0o644); err != nil {
		t.Fatalf("write findings: %v", err)
	}
	t.Logf("wrote %s", dest)
	t.Log("\n" + report)
}

// testDir returns the directory containing this test file, which is the
// bench/agent-efficiency package root.
func testDir(t *testing.T) string {
	t.Helper()
	// os.Getwd() inside a test returns the package directory when run with
	// `go test ./bench/agent-efficiency/`.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
