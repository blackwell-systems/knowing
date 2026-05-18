// Package makefileextractor provides an extractor for Makefile and .mk files.
// It parses make targets, dependencies, and include directives to produce
// nodes and edges representing the build graph.
package makefileextractor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	provenance        = "ast_inferred"
	confidenceExplicit = 1.0
	confidenceInferred = 0.7
)

// MakefileExtractor extracts nodes and edges from Makefiles.
type MakefileExtractor struct{}

// NewMakefileExtractor creates a new MakefileExtractor instance.
func NewMakefileExtractor() *MakefileExtractor {
	return &MakefileExtractor{}
}

// Name returns the identifier for this extractor.
func (e *MakefileExtractor) Name() string { return "makefile" }

// CanHandle returns true for files named Makefile, makefile, or *.mk.
func (e *MakefileExtractor) CanHandle(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if lower == "makefile" {
		return true
	}
	ext := filepath.Ext(path)
	return strings.ToLower(ext) == ".mk"
}

// Extract parses Makefile targets and produces nodes and edges.
func (e *MakefileExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	// Collect phony targets to exclude from dependency edges.
	phonyTargets := make(map[string]bool)

	// First pass: find .PHONY declarations.
	scanner := bufio.NewScanner(bytes.NewReader(opts.Content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".PHONY") {
			// .PHONY: target1 target2
			if idx := strings.Index(trimmed, ":"); idx >= 0 {
				targets := strings.Fields(trimmed[idx+1:])
				for _, t := range targets {
					phonyTargets[t] = true
				}
			}
		}
	}

	// Second pass: extract targets and dependencies.
	scanner = bufio.NewScanner(bytes.NewReader(opts.Content))
	lineNum := 0
	targetNodes := make(map[string]types.Hash)

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip recipe lines (starting with tab), comments, and empty lines.
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " \t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip .PHONY lines (already processed).
		if strings.HasPrefix(trimmed, ".PHONY") {
			continue
		}

		// Parse include directives.
		if strings.HasPrefix(trimmed, "include ") || strings.HasPrefix(trimmed, "-include ") {
			prefix := "include "
			if strings.HasPrefix(trimmed, "-include ") {
				prefix = "-include "
			}
			files := strings.Fields(strings.TrimPrefix(trimmed, prefix))
			fileNodeHash := ensureTargetNode(opts, targetNodes, result, opts.FilePath, lineNum)
			for _, f := range files {
				includeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, f, "type")
				result.Nodes = append(result.Nodes, types.Node{
					NodeHash:      includeHash,
					FileHash:      opts.FileHash,
					QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", f),
					Kind:          "type",
					Line:          lineNum,
				})
				result.Edges = append(result.Edges, makeEdge(fileNodeHash, includeHash, "imports"))
			}
			continue
		}

		// Skip variable assignments.
		if strings.Contains(trimmed, "=") && !strings.Contains(trimmed, ":") {
			continue
		}
		// Handle VAR := value (contains both : and =).
		if strings.Contains(trimmed, ":=") || strings.Contains(trimmed, "?=") || strings.Contains(trimmed, "+=") {
			continue
		}

		// Parse target: dependency1 dependency2
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx <= 0 {
			continue
		}

		// Skip pattern rules with % and double-colon rules.
		targetPart := strings.TrimSpace(trimmed[:colonIdx])
		if strings.Contains(targetPart, "%") {
			continue
		}

		// Multiple targets can be on one line.
		targets := strings.Fields(targetPart)
		depPart := strings.TrimSpace(trimmed[colonIdx+1:])
		// Remove trailing comments from dependency list.
		if ci := strings.Index(depPart, "#"); ci >= 0 {
			depPart = strings.TrimSpace(depPart[:ci])
		}
		deps := strings.Fields(depPart)

		for _, target := range targets {
			// Skip special targets.
			if strings.HasPrefix(target, ".") {
				continue
			}

			targetHash := ensureTargetNode(opts, targetNodes, result, target, lineNum)

			for _, dep := range deps {
				if strings.HasPrefix(dep, "|") {
					dep = strings.TrimPrefix(dep, "|")
				}
				if dep == "" || strings.HasPrefix(dep, "$") {
					continue
				}
				depHash := ensureTargetNode(opts, targetNodes, result, dep, 0)
				result.Edges = append(result.Edges, makeEdge(targetHash, depHash, "depends_on"))
			}
		}
	}

	return result, nil
}

// ensureTargetNode creates a target node if it does not already exist and returns its hash.
func ensureTargetNode(opts types.ExtractOptions, nodeMap map[string]types.Hash, result *types.ExtractResult, name string, line int) types.Hash {
	if h, ok := nodeMap[name]; ok {
		return h
	}
	h := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "action")
	nodeMap[name] = h
	result.Nodes = append(result.Nodes, types.Node{
		NodeHash:      h,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "action", name),
		Kind:          "action",
		Line:          line,
	})
	return h
}

// buildQN constructs a qualified name for a Makefile node.
func buildQN(repoURL, filePath, kind, name string) string {
	return fmt.Sprintf("%s://%s.%s.%s", repoURL, filePath, kind, name)
}

// makeEdge creates an edge with standard provenance and confidence.
func makeEdge(sourceHash, targetHash types.Hash, edgeType string) types.Edge {
	return types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance),
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgeType,
		Confidence: confidenceInferred,
		Provenance: provenance,
	}
}
