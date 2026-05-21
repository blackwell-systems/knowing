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

// GitNexus implements benchtype.Adapter for the GitNexus knowledge graph MCP server.
// Requires: `npm install -g gitnexus` (or local install)
type GitNexus struct {
	indexed map[string]bool
}

func NewGitNexus() *GitNexus {
	return &GitNexus{indexed: make(map[string]bool)}
}

func (a *GitNexus) Name() string { return "gitnexus" }

func (a *GitNexus) Index(repoPath string) (int64, error) {
	if a.indexed[repoPath] {
		return 0, nil
	}

	start := time.Now()
	cmd := exec.Command("gitnexus", "index", repoPath)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("gitnexus index failed: %w\n%s", err, output)
	}
	a.indexed[repoPath] = true
	return time.Since(start).Milliseconds(), nil
}

func (a *GitNexus) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	start := time.Now()

	// GitNexus exposes search via its MCP server or CLI.
	// We use the CLI search command which returns JSON.
	cmd := exec.Command("gitnexus", "search",
		"--format", "json",
		"--limit", "20",
		"--query", task.Description,
	)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return benchtype.RetrievalResult{
			System: "gitnexus",
			TaskID: task.ID,
			Error:  fmt.Sprintf("gitnexus search failed: %v", err),
		}, nil
	}

	latency := time.Since(start).Milliseconds()

	// Parse GitNexus JSON output
	symbols, tokensUsed := parseGitNexusOutput(output)

	return benchtype.RetrievalResult{
		System:     "gitnexus",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: tokensUsed,
		LatencyMs:  latency,
		RawOutput:  string(output),
	}, nil
}

func (a *GitNexus) SupportsLearning() bool { return false }

func (a *GitNexus) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }

func (a *GitNexus) Reset(repoPath string) error {
	delete(a.indexed, repoPath)
	return nil
}

// parseGitNexusOutput extracts symbols from GitNexus JSON response.
// The exact format depends on GitNexus version; this handles the common structure.
func parseGitNexusOutput(output []byte) ([]benchtype.RetrievedSymbol, int) {
	// GitNexus search returns an array of results with name, file, type fields.
	var results []struct {
		Name     string  `json:"name"`
		FilePath string  `json:"file_path"`
		Kind     string  `json:"type"`
		Score    float64 `json:"score"`
	}

	if err := json.Unmarshal(output, &results); err != nil {
		// Try alternate format (object with "results" key)
		var wrapper struct {
			Results []struct {
				Name     string  `json:"name"`
				FilePath string  `json:"file_path"`
				Kind     string  `json:"type"`
				Score    float64 `json:"score"`
			} `json:"results"`
		}
		if err2 := json.Unmarshal(output, &wrapper); err2 != nil {
			return nil, 0
		}
		for _, r := range wrapper.Results {
			results = append(results, struct {
				Name     string  `json:"name"`
				FilePath string  `json:"file_path"`
				Kind     string  `json:"type"`
				Score    float64 `json:"score"`
			}(r))
		}
	}

	symbols := make([]benchtype.RetrievedSymbol, 0, len(results))
	for i, r := range results {
		if r.Name == "" {
			continue
		}
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: r.Name,
			Normalized:    normalize.Symbol(r.Name),
			Score:         r.Score,
			Rank:          i + 1,
			FilePath:      r.FilePath,
			Kind:          r.Kind,
		})
	}

	// Estimate tokens from raw output length (~4 chars per token)
	tokensUsed := len(output) / 4
	if tokensUsed == 0 && len(symbols) > 0 {
		tokensUsed = len(symbols) * 20
	}

	// If output contains token_count field, prefer that
	var meta struct {
		TokenCount int `json:"token_count"`
	}
	if json.Unmarshal(output, &meta) == nil && meta.TokenCount > 0 {
		tokensUsed = meta.TokenCount
	}

	return symbols, tokensUsed
}

// IsAvailable checks if gitnexus is installed and accessible.
func (a *GitNexus) IsAvailable() bool {
	_, err := exec.LookPath("gitnexus")
	return err == nil
}

// gitNexusVersion returns the installed version or empty string.
func gitNexusVersion() string {
	output, err := exec.Command("gitnexus", "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
