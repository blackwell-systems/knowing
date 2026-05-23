// Package envextractor provides an extractor for environment variable files.
// It parses .env files and produces nodes for each KEY=VALUE definition.
package envextractor

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
	confidence = 1.0 // explicit declarations
)

// EnvExtractor extracts nodes from .env files.
type EnvExtractor struct{}

// NewEnvExtractor creates a new EnvExtractor instance.
func NewEnvExtractor() *EnvExtractor {
	return &EnvExtractor{}
}

// Name returns the identifier for this extractor.
func (e *EnvExtractor) Name() string { return "env" }

// CanHandle returns true for .env, .env.local, .env.production, .env.development,
// and .env.example files.
func (e *EnvExtractor) CanHandle(path string) bool {
	base := filepath.Base(path)
	lower := strings.ToLower(base)
	if lower == ".env" {
		return true
	}
	if strings.HasPrefix(lower, ".env.") {
		return true
	}
	return false
}

// Extract parses KEY=VALUE lines and produces variable nodes.
func (e *EnvExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	scanner := bufio.NewScanner(bytes.NewReader(opts.Content))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE or just KEY (export KEY=VALUE also supported).
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimPrefix(line, "export ")
			line = strings.TrimSpace(line)
		}

		// Find the key name.
		idx := strings.Index(line, "=")
		var key string
		if idx > 0 {
			key = strings.TrimSpace(line[:idx])
		} else {
			// Lines without = but with a valid identifier are treated as keys.
			key = strings.TrimSpace(line)
		}

		if key == "" {
			continue
		}

		// Skip keys with spaces (not valid env var names).
		if strings.ContainsAny(key, " \t") {
			continue
		}

		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, key, types.KindVar)
		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.var.%s", opts.RepoURL, opts.FilePath, key),
			Kind:          types.KindVar,
			Line:          lineNum,
		})
	}

	return result, nil
}
