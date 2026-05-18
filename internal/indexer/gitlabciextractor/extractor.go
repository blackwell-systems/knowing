// Package gitlabciextractor provides an extractor for GitLab CI configuration
// files. It parses .gitlab-ci.yml and produces nodes and edges representing
// CI jobs, stages, and their dependencies.
package gitlabciextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	provenance        = "ast_inferred"
	confidenceExplicit = 1.0
	confidenceInferred = 0.7
)

// reservedKeys are top-level YAML keys that are GitLab CI directives, not jobs.
var reservedKeys = map[string]bool{
	"stages":    true,
	"variables": true,
	"image":     true,
	"services":  true,
	"before_script": true,
	"after_script":  true,
	"cache":     true,
	"include":   true,
	"default":   true,
	"workflow":  true,
	"pages":     false, // pages is a valid job name
}

// GitLabCIExtractor extracts nodes and edges from .gitlab-ci.yml files.
type GitLabCIExtractor struct{}

// NewGitLabCIExtractor creates a new GitLabCIExtractor instance.
func NewGitLabCIExtractor() *GitLabCIExtractor {
	return &GitLabCIExtractor{}
}

// Name returns the identifier for this extractor.
func (e *GitLabCIExtractor) Name() string { return "gitlab-ci" }

// CanHandle returns true for .gitlab-ci.yml files.
func (e *GitLabCIExtractor) CanHandle(path string) bool {
	base := filepath.Base(path)
	return strings.ToLower(base) == ".gitlab-ci.yml"
}

// Extract parses GitLab CI YAML and produces nodes and edges.
func (e *GitLabCIExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &raw); err != nil {
		return result, nil
	}

	// Track stage nodes for deduplication.
	stageNodes := make(map[string]types.Hash)
	jobNodes := make(map[string]types.Hash)

	// Extract stages if defined.
	if stagesRaw, ok := raw["stages"]; ok {
		if stages, ok := stagesRaw.([]interface{}); ok {
			for _, s := range stages {
				if stageName, ok := s.(string); ok {
					stageHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, stageName, "action")
					stageNodes[stageName] = stageHash
					result.Nodes = append(result.Nodes, types.Node{
						NodeHash:      stageHash,
						FileHash:      opts.FileHash,
						QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "action", stageName),
						Kind:          "action",
						Line:          1,
					})
				}
			}
		}
	}

	// Extract jobs.
	for key, value := range raw {
		// Skip reserved keys and hidden jobs (starting with .) that are templates.
		if reservedKeys[key] {
			continue
		}

		jobMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		// Create job node.
		jobHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, key, "job")
		jobNodes[key] = jobHash
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      jobHash,
			FileHash:      opts.FileHash,
			QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "job", key),
			Kind:          "job",
			Line:          1,
		})

		// Parse "needs" dependencies.
		if needsRaw, ok := jobMap["needs"]; ok {
			needs := parseStringList(needsRaw)
			for _, need := range needs {
				needHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, need, "job")
				result.Edges = append(result.Edges, makeEdge(jobHash, needHash, "depends_on"))
			}
		}

		// Parse "extends" (template inheritance).
		if extendsRaw, ok := jobMap["extends"]; ok {
			extends := parseStringList(extendsRaw)
			for _, ext := range extends {
				extHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ext, "job")
				result.Edges = append(result.Edges, makeEdge(jobHash, extHash, "extends"))
			}
		}

		// Parse "stage" assignment.
		if stageName, ok := jobMap["stage"].(string); ok {
			stageHash, exists := stageNodes[stageName]
			if !exists {
				stageHash = types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, stageName, "action")
				stageNodes[stageName] = stageHash
				result.Nodes = append(result.Nodes, types.Node{
					NodeHash:      stageHash,
					FileHash:      opts.FileHash,
					QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "action", stageName),
					Kind:          "action",
					Line:          1,
				})
			}
			result.Edges = append(result.Edges, makeEdge(jobHash, stageHash, "depends_on"))
		}

		// Parse "image" dependency.
		if imageRaw, ok := jobMap["image"]; ok {
			var imageName string
			switch v := imageRaw.(type) {
			case string:
				imageName = v
			case map[string]interface{}:
				if name, ok := v["name"].(string); ok {
					imageName = name
				}
			}
			if imageName != "" {
				imageHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, imageName, "image")
				result.Nodes = append(result.Nodes, types.Node{
					NodeHash:      imageHash,
					FileHash:      opts.FileHash,
					QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "image", imageName),
					Kind:          "image",
					Line:          1,
				})
				result.Edges = append(result.Edges, makeEdge(jobHash, imageHash, "depends_on"))
			}
		}
	}

	return result, nil
}

// parseStringList extracts a list of strings from a YAML value that can be
// a single string, a list of strings, or a list of maps with "job" keys
// (the expanded needs syntax).
func parseStringList(v interface{}) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		var result []string
		for _, item := range val {
			switch it := item.(type) {
			case string:
				result = append(result, it)
			case map[string]interface{}:
				// Expanded needs syntax: {job: "name", ...}
				if jobName, ok := it["job"].(string); ok {
					result = append(result, jobName)
				}
			}
		}
		return result
	}
	return nil
}

// buildQN constructs a qualified name for a GitLab CI node.
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
