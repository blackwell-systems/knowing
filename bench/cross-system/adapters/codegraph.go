package adapters

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
)

// CodeGraph implements benchtype.Adapter for colbymchenry/codegraph (19K stars).
// Uses tree-sitter + FTS5 + heuristic scoring. CLI: `codegraph context <task> --format json`.
type CodeGraph struct{}

func NewCodeGraph() *CodeGraph { return &CodeGraph{} }

func (c *CodeGraph) Name() string { return "codegraph" }

func (c *CodeGraph) Index(repoPath string) (int64, error) {
	start := time.Now()

	// Check if already indexed.
	dbPath := filepath.Join(repoPath, ".codegraph", "codegraph.db")
	if fileExists(dbPath) {
		return 0, nil
	}

	cmd := exec.Command("codegraph", "init", "-i", repoPath)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("codegraph init failed: %v: %s", err, output)
	}
	return time.Since(start).Milliseconds(), nil
}

func (c *CodeGraph) Retrieve(repoPath string, task benchtype.Task, tokenBudget int) (benchtype.RetrievalResult, error) {
	start := time.Now()

	// codegraph context "<task>" --path <repo> --format json --max-nodes 50 --no-code
	// We request max-nodes 50 to get enough symbols for P@10 measurement.
	cmd := exec.Command("codegraph", "context", task.Description,
		"--path", repoPath,
		"--format", "json",
		"--max-nodes", "50",
		"--max-code", "20",
	)
	output, err := cmd.Output()
	if err != nil {
		return benchtype.RetrievalResult{
			System: "codegraph",
			TaskID: task.ID,
			Error:  fmt.Sprintf("codegraph context failed: %v", err),
		}, nil
	}

	latency := time.Since(start).Milliseconds()

	var result codegraphResult
	if err := json.Unmarshal(output, &result); err != nil {
		return benchtype.RetrievalResult{
			System: "codegraph",
			TaskID: task.ID,
			Error:  fmt.Sprintf("parse error: %v (output: %.200s)", err, string(output)),
		}, nil
	}

	// Collect symbols from entry points + code blocks (deduplicated, ranked).
	seen := make(map[string]bool)
	var symbols []benchtype.RetrievedSymbol
	rank := 1

	// Entry points are highest priority (codegraph's top-ranked results).
	for _, ep := range result.EntryPoints {
		name := ep.QualifiedName
		if name == "" {
			name = ep.Name
		}
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: name,
			Normalized:    normalize.Symbol(name),
			Rank:          rank,
		})
		rank++
	}

	// Code blocks contribute additional symbols (BFS expansion from entry points).
	for _, cb := range result.CodeBlocks {
		name := cb.NodeName
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		// Build a qualified-ish name from file path + node name for matching.
		qn := name
		if cb.FilePath != "" {
			qn = cb.FilePath + "." + name
		}
		seen[strings.ToLower(name)] = true
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: qn,
			Normalized:    normalize.Symbol(name),
			Rank:          rank,
		})
		rank++
	}

	// Subgraph nodes (if any) fill remaining slots.
	for _, node := range result.Subgraph.Nodes {
		name := node.QualifiedName
		if name == "" {
			name = node.Name
		}
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		symbols = append(symbols, benchtype.RetrievedSymbol{
			QualifiedName: name,
			Normalized:    normalize.Symbol(name),
			Rank:          rank,
		})
		rank++
	}

	// Estimate tokens from output size.
	tokenCount := len(output) / 4

	return benchtype.RetrievalResult{
		System:     "codegraph",
		TaskID:     task.ID,
		Symbols:    symbols,
		TokensUsed: tokenCount,
		LatencyMs:  latency,
	}, nil
}

func (c *CodeGraph) SupportsLearning() bool { return false }

func (c *CodeGraph) RecordFeedback(_ string, _ benchtype.Task, _ []string) error { return nil }

func (c *CodeGraph) Reset(_ string) error { return nil }

func (c *CodeGraph) IsAvailable() bool {
	_, err := exec.LookPath("codegraph")
	return err == nil
}

// codegraphResult maps the JSON output from `codegraph context --format json`.
type codegraphResult struct {
	EntryPoints []codegraphNode `json:"entryPoints"`
	Subgraph    struct {
		Nodes map[string]codegraphNode `json:"nodes"`
	} `json:"subgraph"`
	CodeBlocks []codegraphCodeBlock `json:"codeBlocks"`
}

type codegraphNode struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	FilePath      string `json:"filePath"`
}

type codegraphCodeBlock struct {
	FilePath  string `json:"filePath"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	NodeName  string `json:"nodeName"`
	NodeKind  string `json:"nodeKind"`
}

func fileExists(path string) bool {
	cmd := exec.Command("test", "-f", path)
	return cmd.Run() == nil
}
