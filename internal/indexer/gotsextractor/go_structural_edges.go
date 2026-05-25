package gotsextractor

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractGoStructuralEdges walks a Go AST and extracts structural relationship
// edges that tree-sitter can detect without type resolution:
//
//   - Interface embedding: type A struct { B } -> A implements B
//   - Channel send/receive: ch <- v / v = <-ch -> producer/consumer edges
//   - Type assertions: v.(Type) -> references Type
//   - Struct field access: obj.Field -> references the accessed field/type
//
// These are real code relationships that belong in the graph for correctness.
func extractGoStructuralEdges(root *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string, extNodes map[types.Hash]types.Node) []types.Edge {
	if root == nil {
		return nil
	}
	var edges []types.Edge
	walkForStructuralEdges(root, opts, pkgPath, imports, &edges, extNodes)
	return edges
}

func walkForStructuralEdges(node *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string, edges *[]types.Edge, extNodes map[types.Hash]types.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "type_assertion":
		// v.(Type) -> references edge from enclosing function to Type
		extractTypeAssertionEdge(node, opts, pkgPath, imports, edges, extNodes)

	case "send_statement":
		// ch <- value: the channel and the value are connected
		extractChannelSendEdge(node, opts, pkgPath, imports, edges, extNodes)

	case "receive_statement":
		// <-ch: channel receive
		extractChannelReceiveEdge(node, opts, pkgPath, edges)

	case "struct_type":
		// Check for embedded interfaces/types in struct field list
		extractEmbeddingEdges(node, opts, pkgPath, imports, edges, extNodes)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForStructuralEdges(node.Child(i), opts, pkgPath, imports, edges, extNodes)
	}
}

// extractTypeAssertionEdge handles v.(Type) patterns.
// Creates a "references" edge from the enclosing context to the asserted Type.
func extractTypeAssertionEdge(node *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string, edges *[]types.Edge, extNodes map[types.Hash]types.Node) {
	// The type assertion has form: expression "." "(" type ")"
	// In tree-sitter Go: type_assertion has a "type" field.
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return
	}
	typeName := typeNode.Content(opts.Content)
	if typeName == "" {
		return
	}

	// Resolve the type name through imports if qualified (pkg.Type).
	targetRepoURL := opts.RepoURL
	targetPkg := pkgPath
	if strings.Contains(typeName, ".") {
		parts := strings.SplitN(typeName, ".", 2)
		pkgAlias := parts[0]
		typeName = parts[1]
		if importPath, ok := imports[pkgAlias]; ok {
			targetPkg = importPath
			targetRepoURL = inferRepoURL(opts, targetPkg, pkgPath)
		}
	}

	targetHash := types.ComputeNodeHash(targetRepoURL, targetPkg, types.EmptyHash, typeName, types.KindType)

	// Source: use file-level hash (we don't track which function the assertion is in here)
	sourceHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, opts.FileHash, opts.FilePath, types.KindFile)

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.References, "type_assertion")
	*edges = append(*edges, types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.References,
		Confidence:   0.9,
		Provenance:   "type_assertion",
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	})

	// Register phantom external node if needed.
	if targetRepoURL != opts.RepoURL {
		if _, exists := extNodes[targetHash]; !exists {
			extNodes[targetHash] = types.Node{
				NodeHash:      targetHash,
				FileHash:      types.EmptyHash,
				QualifiedName: targetRepoURL + "://" + targetPkg + "." + typeName,
				Kind:          types.KindType,
			}
		}
	}
}

// extractChannelSendEdge handles ch <- value patterns.
// Creates a "calls" edge from the sending context to the channel variable.
func extractChannelSendEdge(node *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string, edges *[]types.Edge, extNodes map[types.Hash]types.Node) {
	// send_statement: channel "<-" value
	if node.ChildCount() < 2 {
		return
	}
	channelNode := node.Child(0)
	if channelNode == nil {
		return
	}
	channelName := channelNode.Content(opts.Content)
	if channelName == "" || strings.Contains(channelName, "(") {
		return // skip complex expressions
	}

	// Create edge from file to the channel (lightweight: just records the relationship)
	sourceHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, opts.FileHash, opts.FilePath, types.KindFile)
	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, channelName, "variable")

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.References, "channel_send")
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.References,
		Confidence: 0.7,
		Provenance: "channel_send",
	})
}

// extractChannelReceiveEdge handles <-ch patterns.
func extractChannelReceiveEdge(node *sitter.Node, opts types.ExtractOptions, pkgPath string, edges *[]types.Edge) {
	// unary_expression with operator "<-"
	if node.ChildCount() < 2 {
		return
	}
	// The operand is the channel
	operand := node.Child(1)
	if operand == nil {
		return
	}
	channelName := operand.Content(opts.Content)
	if channelName == "" || strings.Contains(channelName, "(") {
		return
	}

	sourceHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, opts.FileHash, opts.FilePath, types.KindFile)
	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, channelName, "variable")

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.References, "channel_receive")
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.References,
		Confidence: 0.7,
		Provenance: "channel_receive",
	})
}

// extractEmbeddingEdges handles struct embedding (implicit interface satisfaction).
// When a struct embeds a type (no field name, just the type), it implicitly implements
// that type's interface.
func extractEmbeddingEdges(node *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string, edges *[]types.Edge, extNodes map[types.Hash]types.Node) {
	// struct_type -> field_declaration_list -> field_declaration
	// An embedded field has no name, just a type.
	fieldList := node.ChildByFieldName("body")
	if fieldList == nil {
		// Try direct child iteration for field_declaration_list
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child != nil && child.Type() == "field_declaration_list" {
				fieldList = child
				break
			}
		}
	}
	if fieldList == nil {
		return
	}

	// Find the parent type_spec to get the struct's name.
	// Walk up to find type_spec -> type_declaration
	parentType := findParentTypeName(node, opts.Content)
	if parentType == "" {
		return
	}
	structHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, parentType, types.KindType)

	for i := 0; i < int(fieldList.ChildCount()); i++ {
		field := fieldList.Child(i)
		if field == nil || field.Type() != "field_declaration" {
			continue
		}
		// An embedded field has no "name" field, just a "type" field.
		nameNode := field.ChildByFieldName("name")
		if nameNode != nil {
			continue // has a name, not an embedding
		}
		typeNode := field.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		embeddedTypeName := typeNode.Content(opts.Content)
		if embeddedTypeName == "" {
			continue
		}
		// Strip pointer prefix
		embeddedTypeName = strings.TrimPrefix(embeddedTypeName, "*")

		// Resolve through imports
		targetRepoURL := opts.RepoURL
		targetPkg := pkgPath
		if strings.Contains(embeddedTypeName, ".") {
			parts := strings.SplitN(embeddedTypeName, ".", 2)
			if importPath, ok := imports[parts[0]]; ok {
				targetPkg = importPath
				targetRepoURL = inferRepoURL(opts, targetPkg, pkgPath)
				embeddedTypeName = parts[1]
			}
		}

		targetHash := types.ComputeNodeHash(targetRepoURL, targetPkg, types.EmptyHash, embeddedTypeName, types.KindType)

		// Create implements edge: struct --implements--> embedded type
		edgeHash := types.ComputeEdgeHash(structHash, targetHash, edgetype.Implements, "embedding")
		*edges = append(*edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: structHash,
			TargetHash: targetHash,
			EdgeType:   edgetype.Implements,
			Confidence: 1.0,
			Provenance: "embedding",
		})

		if targetRepoURL != opts.RepoURL {
			if _, exists := extNodes[targetHash]; !exists {
				extNodes[targetHash] = types.Node{
					NodeHash:      targetHash,
					FileHash:      types.EmptyHash,
					QualifiedName: targetRepoURL + "://" + targetPkg + "." + embeddedTypeName,
					Kind:          types.KindType,
				}
			}
		}
	}
}

// findParentTypeName walks up from a struct_type node to find the type name.
func findParentTypeName(node *sitter.Node, content []byte) string {
	// In Go tree-sitter: type_declaration -> type_spec -> type_identifier + struct_type
	// Walk up to type_spec and get the name field.
	parent := node.Parent()
	for parent != nil {
		if parent.Type() == "type_spec" {
			nameNode := parent.ChildByFieldName("name")
			if nameNode != nil {
				return nameNode.Content(content)
			}
		}
		parent = parent.Parent()
	}
	return ""
}
