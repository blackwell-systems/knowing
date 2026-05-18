// Package packagejsonextractor provides an extractor for package.json files.
// It parses the JSON to produce nodes for the package name, scripts, and
// dependency edges to referenced packages.
package packagejsonextractor

import (
	"context"
	"encoding/json"
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

// PackageJSONExtractor extracts nodes and edges from package.json files.
type PackageJSONExtractor struct{}

// NewPackageJSONExtractor creates a new PackageJSONExtractor instance.
func NewPackageJSONExtractor() *PackageJSONExtractor {
	return &PackageJSONExtractor{}
}

// Name returns the identifier for this extractor.
func (e *PackageJSONExtractor) Name() string { return "package-json" }

// CanHandle returns true for files named package.json.
func (e *PackageJSONExtractor) CanHandle(path string) bool {
	base := filepath.Base(path)
	return strings.ToLower(base) == "package.json"
}

// Extract parses package.json and produces nodes and edges.
func (e *PackageJSONExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var pkg struct {
		Name            string            `json:"name"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(opts.Content, &pkg); err != nil {
		return result, nil
	}

	// Package name node.
	var pkgHash types.Hash
	if pkg.Name != "" {
		pkgHash = types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, pkg.Name, "type")
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      pkgHash,
			FileHash:      opts.FileHash,
			QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", pkg.Name),
			Kind:          "type",
			Line:          1,
		})
	}

	// Script nodes.
	for name := range pkg.Scripts {
		scriptHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "action")
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      scriptHash,
			FileHash:      opts.FileHash,
			QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "action", name),
			Kind:          "action",
			Line:          1,
		})
	}

	// Dependency edges.
	sourceHash := pkgHash
	if sourceHash.IsZero() {
		// If no package name, use a file-level node as source.
		sourceHash = types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, opts.FilePath, "type")
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      sourceHash,
			FileHash:      opts.FileHash,
			QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", opts.FilePath),
			Kind:          "type",
			Line:          1,
		})
	}

	addDeps := func(deps map[string]string) {
		for depName := range deps {
			depHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, depName, "type")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      depHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", depName),
				Kind:          "type",
				Line:          1,
			})
			result.Edges = append(result.Edges, makeEdge(sourceHash, depHash, "depends_on"))
		}
	}

	addDeps(pkg.Dependencies)
	addDeps(pkg.DevDependencies)

	return result, nil
}

// buildQN constructs a qualified name for a package.json node.
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
