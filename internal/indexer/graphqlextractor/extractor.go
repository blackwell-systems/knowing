// Package graphqlextractor provides an extractor for GraphQL schema files.
// It uses regex/line parsing to extract types, inputs, enums, interfaces,
// and operations from .graphql and .gql files.
package graphqlextractor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	provenance        = "ast_inferred"
	confidenceExplicit = 1.0
	confidenceInferred = 0.7
)

var (
	// Matches: type Foo, type Foo implements Bar & Baz, type Foo @directive {
	typeDefRE = regexp.MustCompile(`^\s*type\s+(\w+)(?:\s+implements\s+([\w\s&]+))?`)
	// Matches: input Foo {
	inputDefRE = regexp.MustCompile(`^\s*input\s+(\w+)`)
	// Matches: enum Foo {
	enumDefRE = regexp.MustCompile(`^\s*enum\s+(\w+)`)
	// Matches: interface Foo {
	interfaceDefRE = regexp.MustCompile(`^\s*interface\s+(\w+)`)
	// Matches: scalar Foo
	scalarDefRE = regexp.MustCompile(`^\s*scalar\s+(\w+)`)
	// Matches: union Foo = Bar | Baz
	unionDefRE = regexp.MustCompile(`^\s*union\s+(\w+)`)
	// Matches field definitions like: fieldName(args): ReturnType
	fieldRE = regexp.MustCompile(`^\s*(\w+)\s*(?:\([^)]*\))?\s*:\s*\[?\s*(\w+)`)
)

// operationTypes are the root query/mutation/subscription type names.
var operationTypes = map[string]bool{
	"Query":        true,
	"Mutation":     true,
	"Subscription": true,
}

// builtinScalars are types that should not generate reference edges.
var builtinScalars = map[string]bool{
	"String":  true,
	"Int":     true,
	"Float":   true,
	"Boolean": true,
	"ID":      true,
}

// GraphQLExtractor extracts nodes and edges from GraphQL schema files.
type GraphQLExtractor struct{}

// NewGraphQLExtractor creates a new GraphQLExtractor instance.
func NewGraphQLExtractor() *GraphQLExtractor {
	return &GraphQLExtractor{}
}

// Name returns the identifier for this extractor.
func (e *GraphQLExtractor) Name() string { return "graphql" }

// CanHandle returns true for .graphql and .gql files.
func (e *GraphQLExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".graphql" || ext == ".gql"
}

// Extract parses GraphQL schema definitions and produces nodes and edges.
func (e *GraphQLExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	// First pass: collect all defined type names for reference resolution.
	definedTypes := make(map[string]types.Hash)
	var pendingRefs []pendingRef

	scanner := bufio.NewScanner(bytes.NewReader(opts.Content))
	lineNum := 0
	var currentTypeName string
	var currentTypeHash types.Hash
	isOperationType := false
	inBlock := false
	braceDepth := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments.
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		// Track brace depth.
		braceDepth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")
		if braceDepth <= 0 {
			inBlock = false
			currentTypeName = ""
			braceDepth = 0
		}

		// Check for type definitions.
		if m := typeDefRE.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			kind := "type"
			if operationTypes[name] {
				isOperationType = true
			} else {
				isOperationType = false
			}
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, kind)
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, kind, name),
				Kind:          kind,
				Line:          lineNum,
			})
			definedTypes[name] = nodeHash
			currentTypeName = name
			currentTypeHash = nodeHash
			inBlock = true

			// Parse implements clause.
			if m[2] != "" {
				ifaces := strings.Split(m[2], "&")
				for _, iface := range ifaces {
					iface = strings.TrimSpace(iface)
					if iface == "" {
						continue
					}
					ifaceHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, iface, "interface")
					result.Edges = append(result.Edges, makeEdge(nodeHash, ifaceHash, "implements"))
				}
			}
			continue
		}

		if m := inputDefRE.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", name),
				Kind:          "type",
				Line:          lineNum,
			})
			definedTypes[name] = nodeHash
			currentTypeName = name
			currentTypeHash = nodeHash
			inBlock = true
			isOperationType = false
			continue
		}

		if m := enumDefRE.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", name),
				Kind:          "type",
				Line:          lineNum,
			})
			definedTypes[name] = nodeHash
			currentTypeName = name
			currentTypeHash = nodeHash
			inBlock = true
			isOperationType = false
			continue
		}

		if m := interfaceDefRE.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "interface")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "interface", name),
				Kind:          "interface",
				Line:          lineNum,
			})
			definedTypes[name] = nodeHash
			currentTypeName = name
			currentTypeHash = nodeHash
			inBlock = true
			isOperationType = false
			continue
		}

		if m := scalarDefRE.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", name),
				Kind:          "type",
				Line:          lineNum,
			})
			definedTypes[name] = nodeHash
			continue
		}

		if m := unionDefRE.FindStringSubmatch(trimmed); m != nil {
			name := m[1]
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "type")
			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "type", name),
				Kind:          "type",
				Line:          lineNum,
			})
			definedTypes[name] = nodeHash
			continue
		}

		// Parse fields inside a type block.
		if inBlock && currentTypeName != "" {
			if m := fieldRE.FindStringSubmatch(trimmed); m != nil {
				fieldName := m[1]
				fieldType := m[2]

				// If we are in an operation type (Query, Mutation, Subscription),
				// each field is an operation (function node).
				if isOperationType {
					opHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, fieldName, "function")
					result.Nodes = append(result.Nodes, types.Node{
						NodeHash:      opHash,
						FileHash:      opts.FileHash,
						QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "function", fieldName),
						Kind:          "function",
						Line:          lineNum,
					})
					// Reference from operation to return type.
					if !builtinScalars[fieldType] {
						pendingRefs = append(pendingRefs, pendingRef{
							sourceHash: opHash,
							targetName: fieldType,
						})
					}
				} else {
					// Reference from containing type to field type.
					if !builtinScalars[fieldType] {
						pendingRefs = append(pendingRefs, pendingRef{
							sourceHash: currentTypeHash,
							targetName: fieldType,
						})
					}
				}
			}
		}
	}

	// Resolve pending references.
	for _, ref := range pendingRefs {
		targetHash, ok := definedTypes[ref.targetName]
		if !ok {
			// Create a placeholder node for the referenced type.
			targetHash = types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ref.targetName, "type")
		}
		result.Edges = append(result.Edges, makeEdge(ref.sourceHash, targetHash, "references"))
	}

	return result, nil
}

type pendingRef struct {
	sourceHash types.Hash
	targetName string
}

// buildQN constructs a qualified name for a GraphQL node.
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
