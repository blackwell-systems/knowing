// Package helmextractor provides an extractor for Helm chart files.
// It handles Chart.yaml (chart metadata and dependencies), values.yaml
// (top-level configuration keys), and template files (Kubernetes resources).
package helmextractor

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

// HelmExtractor extracts nodes and edges from Helm chart files.
type HelmExtractor struct{}

// NewHelmExtractor creates a new HelmExtractor instance.
func NewHelmExtractor() *HelmExtractor {
	return &HelmExtractor{}
}

// Name returns the identifier for this extractor.
func (e *HelmExtractor) Name() string { return "helm" }

// CanHandle returns true for Chart.yaml, values.yaml, or YAML files inside
// a templates/ directory. This is a simplified check; it does not verify
// the presence of Chart.yaml in an ancestor directory for templates.
func (e *HelmExtractor) CanHandle(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	ext := strings.ToLower(filepath.Ext(path))

	// Chart.yaml or values.yaml at any level.
	if lower == "chart.yaml" || lower == "chart.yml" {
		return true
	}
	if lower == "values.yaml" || lower == "values.yml" {
		return true
	}

	// Templates directory YAML files.
	if (ext == ".yaml" || ext == ".yml") && isInTemplatesDir(path) {
		return true
	}

	return false
}

// isInTemplatesDir checks if the path is inside a templates/ directory.
func isInTemplatesDir(path string) bool {
	normalized := filepath.ToSlash(path)
	parts := strings.Split(normalized, "/")
	for _, part := range parts {
		if strings.ToLower(part) == "templates" {
			return true
		}
	}
	return false
}

// Extract parses Helm chart files and produces nodes and edges.
func (e *HelmExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	base := strings.ToLower(filepath.Base(opts.FilePath))

	switch {
	case base == "chart.yaml" || base == "chart.yml":
		return e.extractChart(opts)
	case base == "values.yaml" || base == "values.yml":
		return e.extractValues(opts)
	default:
		return e.extractTemplate(opts)
	}
}

// extractChart parses Chart.yaml and produces nodes for the chart and dependency edges.
func (e *HelmExtractor) extractChart(opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var chart struct {
		Name         string `yaml:"name"`
		Version      string `yaml:"version"`
		Dependencies []struct {
			Name       string `yaml:"name"`
			Version    string `yaml:"version"`
			Repository string `yaml:"repository"`
		} `yaml:"dependencies"`
	}
	if err := yaml.Unmarshal(opts.Content, &chart); err != nil {
		return result, nil
	}

	if chart.Name == "" {
		return result, nil
	}

	// Chart node.
	chartHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, chart.Name, types.KindType)
	result.Nodes = append(result.Nodes, types.Node{
		NodeHash:      chartHash,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", chart.Name),
		Kind:          types.KindType,
		Line:          1,
	})

	// Dependency edges.
	for _, dep := range chart.Dependencies {
		if dep.Name == "" {
			continue
		}
		depHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, dep.Name, types.KindType)
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      depHash,
			FileHash:      opts.FileHash,
			QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", dep.Name),
			Kind:          types.KindType,
			Line:          1,
		})
		result.Edges = append(result.Edges, makeEdge(chartHash, depHash, "depends_on"))
	}

	return result, nil
}

// extractValues parses values.yaml and produces nodes for top-level keys.
func (e *HelmExtractor) extractValues(opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var values map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &values); err != nil {
		return result, nil
	}

	line := 1
	for key := range values {
		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, key, types.KindVar)
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "var", key),
			Kind:          types.KindVar,
			Line:          line,
		})
		line++
	}

	return result, nil
}

// extractTemplate parses a Helm template YAML file and extracts basic resource info.
// Since Helm templates contain Go template directives, we do best-effort YAML parsing.
func (e *HelmExtractor) extractTemplate(opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &raw); err != nil {
		// Template files often contain Go template syntax that breaks YAML parsing.
		return result, nil
	}

	kind := getString(raw, "kind")
	if kind == "" {
		return result, nil
	}

	// Extract resource name from metadata.
	name := ""
	if meta, ok := raw["metadata"].(map[string]interface{}); ok {
		name = getString(meta, "name")
	}
	if name == "" {
		name = filepath.Base(opts.FilePath)
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, strings.ToLower(kind))
	result.Nodes = append(result.Nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, strings.ToLower(kind), name),
		Kind:          strings.ToLower(kind),
		Line:          1,
	})

	return result, nil
}

// getString safely extracts a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// buildQN constructs a qualified name for a Helm node.
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
