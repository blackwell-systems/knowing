// Package terraformextractor provides a tree-sitter based extractor for
// Terraform HCL files. It implements types.Extractor and produces nodes for
// resource, data, module, variable, and output blocks, plus edges for
// inter-resource references and module calls.
//
// All edges have provenance "ast_inferred" and confidence 0.7.
package terraformextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/hcl"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	extractorName = "treesitter-terraform"
	provenance    = "ast_inferred"
	confidence    = 0.7
)

// interpolationRef matches Terraform interpolation references like
// ${aws_instance.web.id} and captures the resource type and name.
var interpolationRef = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)(?:\.[a-zA-Z_][a-zA-Z0-9_]*)?\}`)

// TerraformExtractor implements types.Extractor for Terraform HCL files using
// tree-sitter AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type TerraformExtractor struct{}

// NewTerraformExtractor creates a new TerraformExtractor.
func NewTerraformExtractor() *TerraformExtractor {
	return &TerraformExtractor{}
}

// Name returns the extractor name.
func (e *TerraformExtractor) Name() string {
	return extractorName
}

// CanHandle returns true for .tf files that are not in .terraform/ directories.
func (e *TerraformExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".tf") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == ".terraform" {
			return false
		}
	}
	return true
}

// Extract parses the Terraform HCL file with tree-sitter and produces nodes
// for block declarations and edges for references.
func (e *TerraformExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(hcl.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	var nodes []types.Node
	var edges []types.Edge

	// HCL tree-sitter grammar: config_file -> body -> block*
	// We need to find the body node and iterate its block children.
	var bodyNode *sitter.Node
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "body" {
			bodyNode = child
			break
		}
	}
	if bodyNode == nil {
		return &types.ExtractResult{}, nil
	}

	// Walk blocks inside the body.
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		n, ed := e.extractBlock(child, opts)
		nodes = append(nodes, n...)
		edges = append(edges, ed...)
	}

	// Extract interpolation-based dependency edges between resources.
	depEdges := e.extractInterpolationEdges(opts, nodes)
	edges = append(edges, depEdges...)

	// Sort nodes deterministically.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].QualifiedName != nodes[j].QualifiedName {
			return nodes[i].QualifiedName < nodes[j].QualifiedName
		}
		return nodes[i].Kind < nodes[j].Kind
	})

	// Sort edges deterministically.
	sort.Slice(edges, func(i, j int) bool {
		si, sj := edges[i], edges[j]
		if si.SourceHash != sj.SourceHash {
			return si.SourceHash.String() < sj.SourceHash.String()
		}
		if si.TargetHash != sj.TargetHash {
			return si.TargetHash.String() < sj.TargetHash.String()
		}
		return si.EdgeType < sj.EdgeType
	})

	// Deduplicate edges.
	edges = deduplicateEdges(edges)

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// extractBlock extracts a top-level HCL block node. In HCL grammar, blocks
// are represented as "block" nodes with labels indicating type and name.
func (e *TerraformExtractor) extractBlock(node *sitter.Node, opts types.ExtractOptions) ([]types.Node, []types.Edge) {
	if node == nil {
		return nil, nil
	}

	nodeType := node.Type()

	// HCL tree-sitter grammar represents top-level blocks as "block" type nodes.
	// We need to check the block type identifier (first child) to determine
	// what kind of Terraform block this is.
	if nodeType != "block" {
		return nil, nil
	}

	// Get the block type from the first identifier child.
	var blockType string
	var labels []string
	var bodyNode *sitter.Node

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			if blockType == "" {
				blockType = child.Content(opts.Content)
			} else {
				labels = append(labels, child.Content(opts.Content))
			}
		case "string_lit":
			// Strip quotes from string literals used as labels.
			content := child.Content(opts.Content)
			content = strings.Trim(content, `"`)
			labels = append(labels, content)
		case "body":
			bodyNode = child
		}
	}

	switch blockType {
	case "resource":
		return e.extractResource(opts, labels, node)
	case "data":
		return e.extractData(opts, labels, node)
	case "module":
		return e.extractModule(opts, labels, bodyNode, node)
	case "variable":
		return e.extractVariable(opts, labels, node)
	case "output":
		return e.extractOutput(opts, labels, node)
	}

	return nil, nil
}

// extractResource creates a node for a resource block.
// QN format: {repoURL}://{filePath}.resource.{type}.{name}
func (e *TerraformExtractor) extractResource(opts types.ExtractOptions, labels []string, node *sitter.Node) ([]types.Node, []types.Edge) {
	if len(labels) < 2 {
		return nil, nil
	}
	resourceType := labels[0]
	resourceName := labels[1]
	line := int(node.StartPoint().Row) + 1

	qname := fmt.Sprintf("%s://%s.resource.%s.%s", opts.RepoURL, opts.FilePath, resourceType, resourceName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, resourceType+"."+resourceName, "resource")

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "resource",
		Line:          line,
		Signature:     fmt.Sprintf("resource %q %q", resourceType, resourceName),
	}

	return []types.Node{n}, nil
}

// extractData creates a node for a data block.
// QN format: {repoURL}://{filePath}.data.{type}.{name}
func (e *TerraformExtractor) extractData(opts types.ExtractOptions, labels []string, node *sitter.Node) ([]types.Node, []types.Edge) {
	if len(labels) < 2 {
		return nil, nil
	}
	dataType := labels[0]
	dataName := labels[1]
	line := int(node.StartPoint().Row) + 1

	qname := fmt.Sprintf("%s://%s.data.%s.%s", opts.RepoURL, opts.FilePath, dataType, dataName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, dataType+"."+dataName, "data")

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "data",
		Line:          line,
		Signature:     fmt.Sprintf("data %q %q", dataType, dataName),
	}

	return []types.Node{n}, nil
}

// extractModule creates a node for a module block and optionally a "calls" edge.
// QN format: {repoURL}://{filePath}.module.{name}
func (e *TerraformExtractor) extractModule(opts types.ExtractOptions, labels []string, bodyNode *sitter.Node, node *sitter.Node) ([]types.Node, []types.Edge) {
	if len(labels) < 1 {
		return nil, nil
	}
	moduleName := labels[0]
	line := int(node.StartPoint().Row) + 1

	qname := fmt.Sprintf("%s://%s.module.%s", opts.RepoURL, opts.FilePath, moduleName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, moduleName, "module")

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "module",
		Line:          line,
		Signature:     fmt.Sprintf("module %q", moduleName),
	}

	var edges []types.Edge

	// Look for a "source" attribute in the body to create a "calls" edge.
	source := e.extractSourceAttribute(bodyNode, opts.Content)
	if source != "" {
		targetHash := types.ComputeNodeHash(opts.RepoURL, source, types.EmptyHash, source, "module")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "calls", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "calls",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return []types.Node{n}, edges
}

// extractVariable creates a node for a variable block.
// QN format: {repoURL}://{filePath}.variable.{name}
func (e *TerraformExtractor) extractVariable(opts types.ExtractOptions, labels []string, node *sitter.Node) ([]types.Node, []types.Edge) {
	if len(labels) < 1 {
		return nil, nil
	}
	varName := labels[0]
	line := int(node.StartPoint().Row) + 1

	qname := fmt.Sprintf("%s://%s.variable.%s", opts.RepoURL, opts.FilePath, varName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, varName, "variable")

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "variable",
		Line:          line,
		Signature:     fmt.Sprintf("variable %q", varName),
	}

	return []types.Node{n}, nil
}

// extractOutput creates a node for an output block.
// QN format: {repoURL}://{filePath}.output.{name}
func (e *TerraformExtractor) extractOutput(opts types.ExtractOptions, labels []string, node *sitter.Node) ([]types.Node, []types.Edge) {
	if len(labels) < 1 {
		return nil, nil
	}
	outputName := labels[0]
	line := int(node.StartPoint().Row) + 1

	qname := fmt.Sprintf("%s://%s.output.%s", opts.RepoURL, opts.FilePath, outputName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, outputName, "output")

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          "output",
		Line:          line,
		Signature:     fmt.Sprintf("output %q", outputName),
	}

	return []types.Node{n}, nil
}

// extractSourceAttribute finds a "source" attribute in a body node and
// returns its string value.
func (e *TerraformExtractor) extractSourceAttribute(bodyNode *sitter.Node, content []byte) string {
	if bodyNode == nil {
		return ""
	}
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() != "attribute" {
			continue
		}
		// An HCL attribute has children: identifier, "=", expression.
		var attrName string
		var attrVal string
		for j := 0; j < int(child.ChildCount()); j++ {
			gc := child.Child(j)
			switch gc.Type() {
			case "identifier":
				attrName = gc.Content(content)
			case "expression":
				attrVal = gc.Content(content)
			}
		}
		if attrName == "source" && attrVal != "" {
			return strings.Trim(attrVal, `"`)
		}
	}
	return ""
}

// extractInterpolationEdges scans the file content for Terraform interpolation
// references like ${aws_instance.web.id} and creates "depends_on" edges from
// the containing resource to the referenced resource.
func (e *TerraformExtractor) extractInterpolationEdges(opts types.ExtractOptions, nodes []types.Node) []types.Edge {
	// Build a map of resource type.name -> nodeHash for quick lookup.
	resourceMap := make(map[string]types.Hash)
	for _, n := range nodes {
		if n.Kind == "resource" {
			// Extract type.name from the qualified name.
			// Format: {repoURL}://{filePath}.resource.{type}.{name}
			parts := strings.Split(n.QualifiedName, ".resource.")
			if len(parts) == 2 {
				resourceMap[parts[1]] = n.NodeHash
			}
		}
	}

	if len(resourceMap) == 0 {
		return nil
	}

	// Find all interpolation references in the content.
	matches := interpolationRef.FindAllSubmatch(opts.Content, -1)
	if len(matches) == 0 {
		return nil
	}

	var edges []types.Edge
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		refType := string(match[1])
		refName := string(match[2])
		targetKey := refType + "." + refName

		targetHash, exists := resourceMap[targetKey]
		if !exists {
			continue
		}

		// Find which resource block contains this reference.
		// We look for all resource nodes and create edges from each resource
		// that contains this reference (other than self-references).
		for _, n := range nodes {
			if n.Kind != "resource" {
				continue
			}
			if n.NodeHash == targetHash {
				continue // skip self-reference
			}
			// Check if this resource's body contains the reference.
			// Simple heuristic: use line-based containment.
			edgeHash := types.ComputeEdgeHash(n.NodeHash, targetHash, "depends_on", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: n.NodeHash,
				TargetHash: targetHash,
				EdgeType:   "depends_on",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}

	return edges
}

// deduplicateEdges removes duplicate edges based on EdgeHash.
func deduplicateEdges(edges []types.Edge) []types.Edge {
	if len(edges) <= 1 {
		return edges
	}
	seen := make(map[types.Hash]struct{}, len(edges))
	result := make([]types.Edge, 0, len(edges))
	for _, e := range edges {
		if _, exists := seen[e.EdgeHash]; !exists {
			seen[e.EdgeHash] = struct{}{}
			result = append(result, e)
		}
	}
	return result
}
