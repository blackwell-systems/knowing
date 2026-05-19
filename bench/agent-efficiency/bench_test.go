package agent_efficiency

import (
	"encoding/json"
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

		if mode == "control" {
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
