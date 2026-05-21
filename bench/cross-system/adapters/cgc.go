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

// CGC implements benchtype.Adapter for CodeGraphContext.
// Requires: `pip install codegraphcontext` or local clone.
// CGC indexes into KuzuDB (default) and exposes search via CLI or MCP.
type CGC struct {
	indexed map[string]bool
}

func NewCGC() *CGC {
	return &CGC{indexed: make(map[string]bool)}
}

func (a *CGC) Name() string { return "cgc" }

func (a *CGC) Index(repoPath string) (int64, error) {
	if a.indexed[repoPath] {
		return 0, nil
	}

	start := time.Now()
	cmd := exec.Command("cgc", "index", "--path", repoPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("cgc index failed: %w\n%s", err, output)
	}
	a.indexed[repoPath] = true
	return time.Since(start).Milliseconds(), nil
}

func (a *CGC) Retrieve(repoPath string, task benchtype.Task, _ int) (benchtype.RetrievalResult, error) {
	start := time.Now()

	// CGC search via CLI (returns JSON)
	cmd := exec.Command("cgc", "search",
		"--path", repoPath,
		"--format", "json",
		"--limit", "20",
		task.Description,
	)
	output, err := cmd.Output()
	if err != nil {
		return benchtype.RetrievalResult{
			System: "cgc",
			TaskID: task.ID,
			Error:  fmt.Sprintf("cgc search failed: %v", err),
		}, nil
	}

	latency := time.Since(start).Milliseconds()
	symbols, tokensUsed := parseCGCOutput(output)

	return benchtype.RetrievalResult{
		System:     "cgc",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: tokensUsed,
		LatencyMs:  latency,
		RawOutput:  string(output),
	}, nil
}

func (a *CGC) SupportsLearning() bool { return false }

func (a *CGC) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }

func (a *CGC) Reset(repoPath string) error {
	delete(a.indexed, repoPath)
	return nil
}

func parseCGCOutput(output []byte) ([]benchtype.RetrievedSymbol, int) {
	// CGC returns results as JSON array with name, file, type fields
	var results []struct {
		Name     string  `json:"name"`
		QN       string  `json:"qualified_name"`
		FilePath string  `json:"file"`
		Kind     string  `json:"type"`
		Score    float64 `json:"relevance"`
	}

	if err := json.Unmarshal(output, &results); err != nil {
		// Try wrapper format
		var wrapper struct {
			Results json.RawMessage `json:"results"`
		}
		if err2 := json.Unmarshal(output, &wrapper); err2 != nil {
			return nil, 0
		}
		_ = json.Unmarshal(wrapper.Results, &results)
	}

	symbols := make([]benchtype.RetrievedSymbol, 0, len(results))
	for i, r := range results {
		name := r.QN
		if name == "" {
			name = r.Name
		}
		if name == "" {
			continue
		}
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: name,
			Normalized:    normalize.Symbol(name),
			Score:         r.Score,
			Rank:          i + 1,
			FilePath:      r.FilePath,
			Kind:          r.Kind,
		})
	}

	// Estimate tokens
	tokensUsed := len(output) / 4
	if tokensUsed == 0 && len(symbols) > 0 {
		tokensUsed = len(symbols) * 20
	}

	return symbols, tokensUsed
}

// IsAvailable checks if cgc is installed.
func (a *CGC) IsAvailable() bool {
	_, err := exec.LookPath("cgc")
	return err == nil
}

// cgcVersion returns the installed version or empty string.
func cgcVersion() string {
	output, err := exec.Command("cgc", "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
