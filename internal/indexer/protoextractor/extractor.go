// Package protoextractor provides a tree-sitter based extractor for Protocol
// Buffer (.proto) files. It implements types.Extractor and produces nodes for
// service, message, enum, and RPC declarations, plus edges for message
// references in RPC methods, field types, and imports.
//
// Provenance is "ast_inferred" and confidence is 0.7.
package protoextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/protobuf"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	provenance = "ast_inferred"
	confidence = 0.7
)

// ProtoExtractor implements types.Extractor for Protocol Buffer files using
// tree-sitter AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type ProtoExtractor struct{}

// NewProtoExtractor creates a new ProtoExtractor.
func NewProtoExtractor() *ProtoExtractor {
	return &ProtoExtractor{}
}

// Name returns the extractor name.
func (e *ProtoExtractor) Name() string {
	return "treesitter-proto"
}

// CanHandle returns true for .proto files not in vendor/ directories.
func (e *ProtoExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".proto") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "vendor" || p == ".git" {
			return false
		}
	}
	return true
}

// Extract parses the .proto file with tree-sitter and produces nodes for
// declarations and edges for references.
func (e *ProtoExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(protobuf.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	src := opts.Content

	var nodes []types.Node
	var edges []types.Edge

	// Extract the package name for qualified naming.
	pkgName := extractPackageName(root, src)

	// Walk top-level declarations.
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		nodeType := child.Type()

		switch nodeType {
		case "message":
			name := extractDeclName(child, "message_name", src)
			if name == "" {
				continue
			}
			qname := qualifiedName(opts.RepoURL, pkgName, name)
			line := int(child.StartPoint().Row) + 1

			nodeHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + name + "/type"))
			nodes = append(nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qname,
				Kind:          "type",
				Line:          line,
				Signature:     "message " + name,
			})

			// Extract field type references.
			fieldEdges := extractFieldReferences(child, src, opts, pkgName, nodeHash)
			edges = append(edges, fieldEdges...)

		case "enum":
			name := extractDeclName(child, "enum_name", src)
			if name == "" {
				continue
			}
			qname := qualifiedName(opts.RepoURL, pkgName, name)
			line := int(child.StartPoint().Row) + 1

			nodeHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + name + "/type"))
			nodes = append(nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qname,
				Kind:          "type",
				Line:          line,
				Signature:     "enum " + name,
			})

		case "service":
			name := extractDeclName(child, "service_name", src)
			if name == "" {
				continue
			}
			qname := qualifiedName(opts.RepoURL, pkgName, name)
			line := int(child.StartPoint().Row) + 1

			serviceHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + name + "/service"))
			nodes = append(nodes, types.Node{
				NodeHash:      serviceHash,
				FileHash:      opts.FileHash,
				QualifiedName: qname,
				Kind:          "service",
				Line:          line,
				Signature:     "service " + name,
			})

			// Extract RPC methods.
			rpcNodes, rpcEdges := extractRPCs(child, src, opts, pkgName, name, serviceHash)
			nodes = append(nodes, rpcNodes...)
			edges = append(edges, rpcEdges...)

		case "import":
			importPath := extractImportPath(child, src)
			if importPath != "" {
				sourceHash := opts.FileHash
				targetHash := types.NewHash([]byte(importPath))
				edgeHash := types.NewHash([]byte(fmt.Sprintf("%x%x%s%s", sourceHash, targetHash, "imports", provenance)))
				edges = append(edges, types.Edge{
					EdgeHash:   edgeHash,
					SourceHash: sourceHash,
					TargetHash: targetHash,
					EdgeType:   "imports",
					Confidence: confidence,
					Provenance: provenance,
				})
			}
		}
	}

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// extractPackageName finds the package declaration in the proto file.
func extractPackageName(root *sitter.Node, src []byte) string {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() == "package" {
			// Package name is in a full_ident child.
			for j := 0; j < int(child.NamedChildCount()); j++ {
				pkg := child.NamedChild(j)
				if pkg.Type() == "full_ident" {
					return pkg.Content(src)
				}
			}
		}
	}
	return ""
}

// extractDeclName extracts the identifier from a named declaration node.
// The name is in a child of the given type (e.g., "message_name", "service_name").
func extractDeclName(node *sitter.Node, nameType string, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == nameType {
			// The actual identifier is a child of the name node.
			for j := 0; j < int(child.NamedChildCount()); j++ {
				id := child.NamedChild(j)
				if id.Type() == "identifier" {
					return id.Content(src)
				}
			}
			return child.Content(src)
		}
	}
	return ""
}

// extractRPCs extracts RPC method declarations from a service node.
func extractRPCs(serviceNode *sitter.Node, src []byte, opts types.ExtractOptions, pkgName, serviceName string, serviceHash types.Hash) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for i := 0; i < int(serviceNode.NamedChildCount()); i++ {
		child := serviceNode.NamedChild(i)
		if child.Type() != "rpc" {
			continue
		}

		// RPC name is in rpc_name child.
		rpcName := ""
		var msgTypes []string

		for j := 0; j < int(child.NamedChildCount()); j++ {
			rpcChild := child.NamedChild(j)
			switch rpcChild.Type() {
			case "rpc_name":
				rpcName = rpcChild.Content(src)
			case "message_or_enum_type":
				msgTypes = append(msgTypes, rpcChild.Content(src))
			}
		}

		if rpcName == "" {
			continue
		}

		reqType := ""
		respType := ""
		if len(msgTypes) >= 1 {
			reqType = msgTypes[0]
		}
		if len(msgTypes) >= 2 {
			respType = msgTypes[1]
		}

		qname := qualifiedName(opts.RepoURL, pkgName, serviceName+"."+rpcName)
		line := int(child.StartPoint().Row) + 1

		sig := fmt.Sprintf("rpc %s(%s) returns (%s)", rpcName, reqType, respType)
		rpcHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + serviceName + "." + rpcName + "/function"))

		nodes = append(nodes, types.Node{
			NodeHash:      rpcHash,
			FileHash:      opts.FileHash,
			QualifiedName: qname,
			Kind:          "function",
			Line:          line,
			Signature:     sig,
		})

		// Edge: service -> rpc (calls).
		callEdgeHash := types.NewHash([]byte(fmt.Sprintf("%x%x%s%s", serviceHash, rpcHash, "calls", provenance)))
		edges = append(edges, types.Edge{
			EdgeHash:   callEdgeHash,
			SourceHash: serviceHash,
			TargetHash: rpcHash,
			EdgeType:   "calls",
			Confidence: confidence,
			Provenance: provenance,
		})

		// Edge: rpc -> request message type (references).
		if reqType != "" {
			targetHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + reqType + "/type"))
			edgeHash := types.NewHash([]byte(fmt.Sprintf("%x%x%s%s-req", rpcHash, targetHash, "references", provenance)))
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: rpcHash,
				TargetHash: targetHash,
				EdgeType:   "references",
				Confidence: confidence,
				Provenance: provenance,
			})
		}

		// Edge: rpc -> response message type (references).
		if respType != "" && respType != reqType {
			targetHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + respType + "/type"))
			edgeHash := types.NewHash([]byte(fmt.Sprintf("%x%x%s%s-resp", rpcHash, targetHash, "references", provenance)))
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: rpcHash,
				TargetHash: targetHash,
				EdgeType:   "references",
				Confidence: confidence,
				Provenance: provenance,
			})
		}
	}

	return nodes, edges
}

// extractFieldReferences extracts edges from message field types to other messages.
func extractFieldReferences(msgNode *sitter.Node, src []byte, opts types.ExtractOptions, pkgName string, msgHash types.Hash) []types.Edge {
	var edges []types.Edge

	// Find the message_body child.
	var body *sitter.Node
	for i := 0; i < int(msgNode.NamedChildCount()); i++ {
		child := msgNode.NamedChild(i)
		if child.Type() == "message_body" {
			body = child
			break
		}
	}
	if body == nil {
		return edges
	}

	seen := make(map[string]bool)
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if child.Type() != "field" {
			continue
		}

		// Extract the type from the field. Fields look like: "Type name = N;"
		// The type is a message_or_enum_type or a scalar.
		fieldType := extractFieldType(child, src)
		if fieldType == "" || isScalarType(fieldType) || seen[fieldType] {
			continue
		}
		seen[fieldType] = true

		targetHash := types.NewHash([]byte(opts.RepoURL + "://" + pkgName + "." + fieldType + "/type"))
		edgeHash := types.NewHash([]byte(fmt.Sprintf("%x%x%s%s", msgHash, targetHash, "references", provenance)))
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: msgHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return edges
}

// extractFieldType gets the type name from a field node.
func extractFieldType(fieldNode *sitter.Node, src []byte) string {
	for i := 0; i < int(fieldNode.NamedChildCount()); i++ {
		child := fieldNode.NamedChild(i)
		if child.Type() == "message_or_enum_type" || child.Type() == "type" {
			return child.Content(src)
		}
	}
	// Fallback: parse the field text to extract the type.
	// Fields are "type name = N;" or "repeated type name = N;".
	text := fieldNode.Content(src)
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "repeated ")
	text = strings.TrimPrefix(text, "optional ")
	parts := strings.Fields(text)
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

// extractImportPath extracts the path string from an import statement.
func extractImportPath(importNode *sitter.Node, src []byte) string {
	// Walk all children (including unnamed) to find the string literal.
	for i := 0; i < int(importNode.ChildCount()); i++ {
		child := importNode.Child(i)
		content := child.Content(src)
		if strings.HasPrefix(content, "\"") && strings.HasSuffix(content, "\"") {
			return strings.Trim(content, "\"")
		}
	}
	return ""
}

// qualifiedName constructs a qualified name in the knowing format.
func qualifiedName(repoURL, pkgName, name string) string {
	if pkgName != "" {
		return repoURL + "://" + pkgName + "." + name
	}
	return repoURL + "://" + name
}

// isScalarType returns true for protobuf scalar types that don't generate edges.
var scalarTypes = map[string]bool{
	"double": true, "float": true, "int32": true, "int64": true,
	"uint32": true, "uint64": true, "sint32": true, "sint64": true,
	"fixed32": true, "fixed64": true, "sfixed32": true, "sfixed64": true,
	"bool": true, "string": true, "bytes": true,
}

func isScalarType(t string) bool {
	return scalarTypes[t]
}
