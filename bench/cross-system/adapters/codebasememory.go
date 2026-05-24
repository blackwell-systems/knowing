package adapters

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
)

// CodebaseMemory implements benchtype.Adapter for codebase-memory-mcp (2.6K stars).
// Uses tree-sitter (155 grammars) + BM25 + label boost + semantic edges.
// CLI: `codebase-memory-mcp cli search_graph '{"query":"...","project":"..."}'`
type CodebaseMemory struct {
	projects map[string]string // repoPath -> project name
}

func NewCodebaseMemory() *CodebaseMemory {
	return &CodebaseMemory{projects: make(map[string]string)}
}

func (c *CodebaseMemory) Name() string { return "codebase-memory" }

func (c *CodebaseMemory) Index(repoPath string) (int64, error) {
	start := time.Now()

	absPath, err := absPath(repoPath)
	if err != nil {
		return 0, err
	}

	arg := fmt.Sprintf(`{"repo_path": %q}`, absPath)
	cmd := exec.Command(codebaseMemoryBin(), "cli", "index_repository", arg)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("index failed: %v: %s", err, output)
	}

	// Extract project name from response.
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "{") {
			var resp struct {
				Project string `json:"project"`
			}
			if json.Unmarshal([]byte(line), &resp) == nil && resp.Project != "" {
				c.projects[repoPath] = resp.Project
			}
		}
	}

	return time.Since(start).Milliseconds(), nil
}

func (c *CodebaseMemory) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	project := c.projects[repoPath]
	if project == "" {
		return benchtype.RetrievalResult{
			System: "codebase-memory",
			TaskID: task.ID,
			Error:  "repo not indexed",
		}, nil
	}

	start := time.Now()

	arg := fmt.Sprintf(`{"query": %q, "project": %q, "limit": 50}`, task.Description, project)
	cmd := exec.Command(codebaseMemoryBin(), "cli", "search_graph", arg)
	output, err := cmd.Output()
	if err != nil {
		return benchtype.RetrievalResult{
			System: "codebase-memory",
			TaskID: task.ID,
			Error:  fmt.Sprintf("search failed: %v", err),
		}, nil
	}

	latency := time.Since(start).Milliseconds()

	// Parse JSON (skip log lines).
	var jsonLine string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "{") && strings.Contains(line, "results") {
			jsonLine = line
			break
		}
	}
	if jsonLine == "" {
		// Try stderr+stdout combined.
		combined, _ := exec.Command(codebaseMemoryBin(), "cli", "search_graph", arg).CombinedOutput()
		for _, line := range strings.Split(string(combined), "\n") {
			if strings.HasPrefix(line, "{") && strings.Contains(line, "results") {
				jsonLine = line
				break
			}
		}
	}
	if jsonLine == "" {
		return benchtype.RetrievalResult{
			System: "codebase-memory",
			TaskID: task.ID,
			Error:  "no JSON in output",
		}, nil
	}

	var result cmResult
	if err := json.Unmarshal([]byte(jsonLine), &result); err != nil {
		return benchtype.RetrievalResult{
			System: "codebase-memory",
			TaskID: task.ID,
			Error:  fmt.Sprintf("parse error: %v", err),
		}, nil
	}

	var symbols []benchtype.RetrievedSymbol
	for i, r := range result.Results {
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: r.QualifiedName,
			Normalized:    normalize.Symbol(r.Name),
			Rank:          i + 1,
			Kind:          r.Label,
		})
	}

	tokenCount := len(jsonLine) / 4

	return benchtype.RetrievalResult{
		System:     "codebase-memory",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: tokenCount,
		LatencyMs:  latency,
	}, nil
}

func (c *CodebaseMemory) SupportsLearning() bool { return false }

func (c *CodebaseMemory) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }

func (c *CodebaseMemory) Reset(_ string) error { return nil }

func (c *CodebaseMemory) IsAvailable() bool {
	_, err := exec.LookPath("codebase-memory-mcp")
	return err == nil
}

func codebaseMemoryBin() string {
	path, err := exec.LookPath("codebase-memory-mcp")
	if err != nil {
		return "codebase-memory-mcp"
	}
	return path
}

type cmResult struct {
	Total   int        `json:"total"`
	Results []cmSymbol `json:"results"`
}

type cmSymbol struct {
	Name          string  `json:"name"`
	QualifiedName string  `json:"qualified_name"`
	Label         string  `json:"label"`
	FilePath      string  `json:"file_path"`
	Rank          float64 `json:"rank"`
}

func absPath(path string) (string, error) {
	cmd := exec.Command("realpath", path)
	out, err := cmd.Output()
	if err != nil {
		// Fallback for macOS (no realpath by default).
		cmd = exec.Command("python3", "-c", fmt.Sprintf("import os; print(os.path.abspath(%q))", path))
		out, err = cmd.Output()
		if err != nil {
			return path, nil
		}
	}
	return strings.TrimSpace(string(out)), nil
}
