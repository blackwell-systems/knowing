package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
)

// Gortex implements benchtype.Adapter for the Gortex code graph engine.
type Gortex struct {
	indexed map[string]bool
	binary  string
}

func NewGortex() *Gortex {
	bin := "/tmp/gortex/gortex"
	return &Gortex{indexed: make(map[string]bool), binary: bin}
}

func (a *Gortex) Name() string { return "gortex" }

func (a *Gortex) Index(repoPath string) (int64, error) {
	if a.indexed[repoPath] {
		return 0, nil
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	cmd := exec.Command(a.binary, "index", absPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("gortex index: %s", string(output))
	}
	a.indexed[repoPath] = true
	return time.Since(start).Milliseconds(), nil
}

func (a *Gortex) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return benchtype.RetrievalResult{System: "gortex", TaskID: task.ID, Error: err.Error()}, nil
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.binary, "context",
		"--task", task.Description,
		"--index", absPath,
		"--format", "json",
		"--max-symbols", "10",
		"--token-budget", fmt.Sprintf("%d", tokenBudget),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return benchtype.RetrievalResult{System: "gortex", TaskID: task.ID, Error: fmt.Sprintf("gortex context: %s", string(output))}, nil
	}

	latency := time.Since(start).Milliseconds()

	// Parse gortex JSON response. Gortex emits structured log lines (zap JSON)
	// before the actual response. Find the last line that starts with '{' and
	// contains "relevant_symbols" (the actual response object).
	var resp gortexContextResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		// Split on newlines and find the response line (last JSON with relevant_symbols).
		lines := strings.Split(string(output), "\n")
		found := false
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			if strings.Contains(line, "relevant_symbols") || strings.Contains(line, "files_to_edit") {
				if err2 := json.Unmarshal([]byte(line), &resp); err2 == nil {
					found = true
					break
				}
			}
		}
		if !found {
			return benchtype.RetrievalResult{System: "gortex", TaskID: task.ID, Error: "json parse: no response object in output"}, nil
		}
	}

	var symbols []benchtype.RetrievedSymbol
	for i, s := range resp.RelevantSymbols {
		qn := s.FilePath + "." + s.Name
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: qn,
			Normalized:    normalize.Symbol(qn),
			Score:         1.0 / float64(i+1),
			Rank:          i + 1,
			Kind:          s.Kind,
		})
	}

	if len(symbols) > 20 {
		symbols = symbols[:20]
	}

	tokensUsed := len(output) / 4
	return benchtype.RetrievalResult{
		System:     "gortex",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: tokensUsed,
		LatencyMs:  latency,
	}, nil
}

func (a *Gortex) Close() error                                              { return nil }
func (a *Gortex) SupportsLearning() bool                                     { return false }
func (a *Gortex) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }
func (a *Gortex) Reset(repoPath string) error {
	delete(a.indexed, repoPath)
	return nil
}

type gortexContextResponse struct {
	FilesToEdit      []string       `json:"files_to_edit"`
	RelatedTestFiles []string       `json:"related_test_files"`
	RelevantSymbols  []gortexSymbol `json:"relevant_symbols"`
	Task             string         `json:"task"`
}

type gortexSymbol struct {
	FilePath  string `json:"file_path"`
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Signature string `json:"signature"`
	StartLine int    `json:"start_line"`
}
