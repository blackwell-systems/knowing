// Package dockerfileextractor provides an extractor for Dockerfile files.
// It parses Dockerfile instructions line-by-line and produces nodes and edges
// representing images, stages, ports, and environment variables.
package dockerfileextractor

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
	provenance = "ast_inferred"
	confidence = 0.7
)

// DockerfileExtractor extracts nodes and edges from Dockerfiles.
type DockerfileExtractor struct{}

// NewDockerfileExtractor creates a new DockerfileExtractor instance.
func NewDockerfileExtractor() *DockerfileExtractor {
	return &DockerfileExtractor{}
}

// Name returns the identifier for this extractor.
func (e *DockerfileExtractor) Name() string { return "dockerfile" }

// CanHandle returns true for files named Dockerfile, Dockerfile.*, or *.dockerfile.
func (e *DockerfileExtractor) CanHandle(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if lower == "dockerfile" {
		return true
	}
	if strings.HasPrefix(lower, "dockerfile.") {
		return true
	}
	if strings.HasSuffix(lower, ".dockerfile") {
		return true
	}
	return false
}

// Extract parses Dockerfile instructions and produces nodes and edges.
func (e *DockerfileExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	// File-level node representing the Dockerfile itself.
	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, opts.FilePath, types.KindType)
	fileNode := types.Node{
		NodeHash:      fileNodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", filepath.Base(opts.FilePath)),
		Kind:          types.KindType,
		Line:          1,
	}
	result.Nodes = append(result.Nodes, fileNode)

	var currentStageHash types.Hash
	stageIndex := 0

	scanner := bufio.NewScanner(bytes.NewReader(opts.Content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Remove line continuations (trailing backslash).
		for strings.HasSuffix(line, "\\") && scanner.Scan() {
			lineNum++
			line = strings.TrimSuffix(line, "\\")
			line = strings.TrimSpace(line) + " " + strings.TrimSpace(scanner.Text())
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		instruction := strings.ToUpper(parts[0])

		switch instruction {
		case "FROM":
			if len(parts) < 2 {
				continue
			}
			imageRef := parts[1]

			// Check for "AS stageName".
			var stageName string
			for i, p := range parts {
				if strings.EqualFold(p, "AS") && i+1 < len(parts) {
					stageName = parts[i+1]
					break
				}
			}

			if stageName == "" {
				stageName = fmt.Sprintf("stage-%d", stageIndex)
			}
			stageIndex++

			// Create stage node.
			stageHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, stageName, types.KindType)
			currentStageHash = stageHash
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      stageHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", stageName),
				Kind:          types.KindType,
				Line:          lineNum,
			})

			// Create image node and depends_on edge.
			imageName := imageRef
			if idx := strings.Index(imageName, ":"); idx > 0 {
				imageName = imageName[:idx]
			}
			imageHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, imageRef, "image")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      imageHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "image", imageRef),
				Kind:          "image",
				Line:          lineNum,
			})
			result.Edges = append(result.Edges, makeEdge(stageHash, imageHash, "depends_on"))

			// Check for COPY --from=stage (this FROM might reference another stage).
			// Handled below in COPY case.

		case "EXPOSE":
			for _, portStr := range parts[1:] {
				// Strip protocol suffix like /tcp, /udp.
				port := strings.Split(portStr, "/")[0]
				portHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, port, "port")
				result.Nodes = append(result.Nodes, types.Node{
					NodeHash:      portHash,
					FileHash:      opts.FileHash,
					QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "port", port),
					Kind:          "port",
					Line:          lineNum,
				})
			}

		case "COPY", "ADD":
			// Check for --from=stage.
			for _, p := range parts[1:] {
				if strings.HasPrefix(p, "--from=") {
					fromStage := strings.TrimPrefix(p, "--from=")
					fromStageHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, fromStage, types.KindType)
					if !currentStageHash.IsZero() {
						result.Edges = append(result.Edges, makeEdge(currentStageHash, fromStageHash, "depends_on"))
					}
				}
			}

		case "ENV":
			if len(parts) < 2 {
				continue
			}
			// ENV KEY=VALUE or ENV KEY VALUE
			key := parts[1]
			if idx := strings.Index(key, "="); idx > 0 {
				key = key[:idx]
			}
			varHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, key, types.KindVar)
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      varHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "var", key),
				Kind:          types.KindVar,
				Line:          lineNum,
			})

		case "ARG":
			if len(parts) < 2 {
				continue
			}
			argName := parts[1]
			if idx := strings.Index(argName, "="); idx > 0 {
				argName = argName[:idx]
			}
			argHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, argName, types.KindVar)
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      argHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "var", argName),
				Kind:          types.KindVar,
				Line:          lineNum,
			})
		}
	}

	return result, nil
}

// buildQN constructs a qualified name for a Dockerfile node.
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
		Confidence: confidence,
		Provenance: provenance,
	}
}
