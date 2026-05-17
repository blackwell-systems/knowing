package cloudextractor

import (
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

// subExtractor is the internal interface each format parser implements.
type subExtractor interface {
	name() string
	canHandle(path string, content []byte) bool
	extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error)
}

// CloudExtractor delegates to format-specific sub-extractors.
type CloudExtractor struct {
	subs []subExtractor
}

// NewCloudExtractor creates a CloudExtractor with all registered sub-extractors.
func NewCloudExtractor() *CloudExtractor {
	return &CloudExtractor{
		subs: []subExtractor{
			&cfnExtractor{},
			&composeExtractor{},
			&actionsExtractor{},
			&serverlessExtractor{},
		},
	}
}

// Name returns the identifier for this extractor.
func (e *CloudExtractor) Name() string { return "cloud-yaml" }

// CanHandle returns true for all YAML files. Content-based filtering happens
// in Extract via the sub-extractors.
func (e *CloudExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// Extract delegates to the first sub-extractor that can handle the file content.
// If no sub-extractor matches, it returns an empty result.
func (e *CloudExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	for _, sub := range e.subs {
		if sub.canHandle(opts.FilePath, opts.Content) {
			return sub.extract(ctx, opts)
		}
	}
	return &types.ExtractResult{}, nil
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

// toStringMap converts a map[string]interface{} to map[string]string.
func toStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// makeEdge creates an edge with standard cloud extractor provenance and confidence.
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

// buildQN constructs a qualified name for a cloud resource node.
// Format: {repoURL}://{filePath}.{kind}.{name}
func buildQN(repoURL, filePath, kind, name string) string {
	return fmt.Sprintf("%s://%s.%s.%s", repoURL, filePath, kind, name)
}
