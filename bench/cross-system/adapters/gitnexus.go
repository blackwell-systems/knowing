package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return 0, err
	}

	// Check if already indexed (has .gitnexus directory).
	if _, err := os.Stat(filepath.Join(absPath, ".gitnexus")); err == nil {
		a.indexed[repoPath] = true
		return 0, nil
	}

	// Not indexed and we won't attempt re-indexing during benchmark
	// (GitNexus takes >22 min on VS Code, >60 min on kubernetes).
	return 0, fmt.Errorf("gitnexus: repo not pre-indexed (no .gitnexus/ dir at %s)", absPath)
}

func (a *GitNexus) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	absPath, _ := filepath.Abs(repoPath)
	repoName := filepath.Base(absPath)

	start := time.Now()
	cmd := exec.Command("gitnexus", "query", task.Description, "--limit", "10", "-r", repoName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return benchtype.RetrievalResult{
			System: "gitnexus",
			TaskID: task.ID,
			Error:  fmt.Sprintf("gitnexus query failed: %s", strings.TrimSpace(string(output))),
		}, nil
	}

	latency := time.Since(start).Milliseconds()

	// Parse GitNexus JSON output (processes + definitions).
	symbols, tokensUsed := parseGitNexusOutput(output)

	return benchtype.RetrievalResult{
		System:     "gitnexus",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: tokensUsed,
		LatencyMs:  latency,
	}, nil
}

func (a *GitNexus) SupportsLearning() bool { return false }

func (a *GitNexus) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }

func (a *GitNexus) Reset(repoPath string) error {
	delete(a.indexed, repoPath)
	return nil
}

// parseGitNexusOutput extracts symbols from GitNexus query JSON response.
// Format: { "processes": [...], "process_symbols": [...], "definitions": [...] }
func parseGitNexusOutput(output []byte) ([]benchtype.RetrievedSymbol, int) {
	var resp struct {
		ProcessSymbols []struct {
			Name     string `json:"name"`
			FilePath string `json:"filePath"`
		} `json:"process_symbols"`
		Definitions []struct {
			Name     string `json:"name"`
			FilePath string `json:"filePath"`
		} `json:"definitions"`
	}

	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, 0
	}

	var symbols []benchtype.RetrievedSymbol
	seen := make(map[string]bool)
	rank := 1

	// Process symbols first (higher relevance from flow analysis).
	for _, ps := range resp.ProcessSymbols {
		if ps.Name == "" || seen[ps.Name] {
			continue
		}
		seen[ps.Name] = true
		qn := ps.FilePath + "." + ps.Name
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: qn,
			Normalized:    normalize.Symbol(qn),
			Score:         1.0 / float64(rank),
			Rank:          rank,
			Kind:          "function",
		})
		rank++
	}

	// Then definitions.
	for _, d := range resp.Definitions {
		if d.Name == "" || seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		qn := d.FilePath + "." + d.Name
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: qn,
			Normalized:    normalize.Symbol(qn),
			Score:         1.0 / float64(rank),
			Rank:          rank,
			Kind:          "function",
		})
		rank++
	}

	if len(symbols) > 20 {
		symbols = symbols[:20]
	}

	tokensUsed := len(output) / 4
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
