package gotsextractor

import (
	"fmt"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// ExtractDocumentsEdges creates 'documents' edges for declarations that have
// doc comments. For each declaration node with a non-empty Doc field, creates
// a synthetic "doc_comment" node and a 'documents' edge from it to the declaration.
// Parameters:
//
//	root: tree-sitter root node of the file (unused, kept for interface consistency)
//	opts: standard ExtractOptions
//	pkgPath: resolved Go package path
//	declNodes: nodes already extracted by Extract (with Doc field populated)
//
// Returns: (commentNodes []types.Node, edges []types.Edge)
func ExtractDocumentsEdges(_ *sitter.Node, opts types.ExtractOptions, pkgPath string, declNodes []types.Node) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for _, node := range declNodes {
		if node.Doc == "" {
			continue
		}

		// Create a synthetic doc_comment node.
		commentName := "doc:" + node.QualifiedName
		commentHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, commentName, "doc_comment")
		commentNode := types.Node{
			NodeHash:      commentHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, commentName),
			Kind:          "doc_comment",
			Line:          node.Line - 1, // comment precedes the declaration
			Doc:           node.Doc,       // store the comment text
		}
		nodes = append(nodes, commentNode)

		// Create a 'documents' edge from comment to declaration.
		provenance := "ast_inferred"
		edgeHash := types.ComputeEdgeHash(commentHash, node.NodeHash, edgetype.Documents, provenance)
		edge := types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: commentHash,
			TargetHash: node.NodeHash,
			EdgeType:   edgetype.Documents,
			Confidence: 0.9,
			Provenance: provenance,
		}
		edges = append(edges, edge)
	}

	return nodes, edges
}
