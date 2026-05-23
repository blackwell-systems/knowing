package protoextractor

import (
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// goParser is a lazily-initialized tree-sitter parser for Go source code.
// The protoextractor normally parses .proto files; this Go parser is used
// exclusively by ExtractRPCEdges to detect gRPC patterns in Go source.
var (
	goParserOnce sync.Once
	goParser     *sitter.Parser
)

func getGoParser() *sitter.Parser {
	goParserOnce.Do(func() {
		goParser = sitter.NewParser()
		goParser.SetLanguage(golang.GetLanguage())
	})
	return goParser
}

// ExtractRPCEdges scans Go source for gRPC implementation and client patterns.
// It creates 'implements_rpc' edges from server structs to proto service nodes,
// and 'consumes_rpc' edges from client call sites to proto service nodes.
//
// Detection patterns:
//   - implements_rpc: struct embedding pb.Unimplemented*Server
//   - consumes_rpc: calls to pb.New*Client(...)
func ExtractRPCEdges(root *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string) ([]types.Node, []types.Edge) {
	src := opts.Content

	var nodes []types.Node
	var edges []types.Edge

	// Pattern 1: detect implements_rpc from struct embeddings
	implNodes, implEdges := extractImplementsRPC(root, src, opts, pkgPath, imports)
	nodes = append(nodes, implNodes...)
	edges = append(edges, implEdges...)

	// Pattern 2: detect consumes_rpc from client creation calls
	consumeNodes, consumeEdges := extractConsumesRPC(root, src, opts, pkgPath, imports)
	nodes = append(nodes, consumeNodes...)
	edges = append(edges, consumeEdges...)

	return nodes, edges
}

// extractImplementsRPC walks top-level type declarations to find structs
// that embed pb.Unimplemented*Server, indicating a gRPC server implementation.
func extractImplementsRPC(root *sitter.Node, src []byte, opts types.ExtractOptions, pkgPath string, imports map[string]string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if child.Type() != "type_declaration" {
			continue
		}

		// type_declaration can have multiple type_spec children (type block)
		for j := 0; j < int(child.NamedChildCount()); j++ {
			typeSpec := child.NamedChild(j)
			if typeSpec.Type() != "type_spec" {
				continue
			}

			structName, structType := extractTypeSpecParts(typeSpec, src)
			if structName == "" || structType == nil || structType.Type() != "struct_type" {
				continue
			}

			// Walk the struct's field declarations
			fieldList := findNamedChild(structType, "field_declaration_list")
			if fieldList == nil {
				continue
			}

			for k := 0; k < int(fieldList.NamedChildCount()); k++ {
				field := fieldList.NamedChild(k)
				if field.Type() != "field_declaration" {
					continue
				}

				// Look for embedded fields with selector_expression matching
				// *.Unimplemented*Server pattern
				alias, serviceName := extractUnimplementedServer(field, src)
				if serviceName == "" {
					continue
				}

				// Resolve the proto package path from imports
				protoPkgPath := resolveProtoPkgPath(alias, imports)
				if protoPkgPath == "" {
					continue
				}

				// Create the implements_rpc edge
				structHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, structName, types.KindType)
				protoServiceHash := types.NewHash([]byte(protoPkgPath + "://" + serviceName + "/service"))

				edgeProvenance := "ast_inferred"
				edgeHash := types.ComputeEdgeHash(structHash, protoServiceHash, edgetype.ImplementsRPC, edgeProvenance)

				edges = append(edges, types.Edge{
					EdgeHash:   edgeHash,
					SourceHash: structHash,
					TargetHash: protoServiceHash,
					EdgeType:   edgetype.ImplementsRPC,
					Confidence: 0.9,
					Provenance: edgeProvenance,
				})

				// Also emit the struct as a node so downstream can reference it
				nodes = append(nodes, types.Node{
					NodeHash:      structHash,
					FileHash:      opts.FileHash,
					QualifiedName: opts.RepoURL + "://" + pkgPath + "." + structName,
					Kind:          types.KindType,
					Line:          int(typeSpec.StartPoint().Row) + 1,
					Signature:     "type " + structName + " struct",
				})
			}
		}
	}

	return nodes, edges
}

// extractConsumesRPC walks function bodies for call expressions matching
// *.New*Client(...) pattern, indicating gRPC client creation.
func extractConsumesRPC(root *sitter.Node, src []byte, opts types.ExtractOptions, pkgPath string, imports map[string]string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	// Walk all function/method declarations
	walkFunctions(root, src, func(funcName string, funcNode *sitter.Node, body *sitter.Node) {
		funcHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, funcName, types.KindFunction)

		// Walk the body looking for call expressions
		walkCallExpressions(body, src, func(callNode *sitter.Node) {
			alias, serviceName := extractNewClientCall(callNode, src)
			if serviceName == "" {
				return
			}

			// Resolve the proto package path from imports
			protoPkgPath := resolveProtoPkgPath(alias, imports)
			if protoPkgPath == "" {
				return
			}

			protoServiceHash := types.NewHash([]byte(protoPkgPath + "://" + serviceName + "/service"))

			edgeProvenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(funcHash, protoServiceHash, edgetype.ConsumesRPC, edgeProvenance)

			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: funcHash,
				TargetHash: protoServiceHash,
				EdgeType:   edgetype.ConsumesRPC,
				Confidence: 0.8,
				Provenance: edgeProvenance,
			})
		})
	})

	return nodes, edges
}

// extractTypeSpecParts extracts the name and type body from a type_spec node.
func extractTypeSpecParts(typeSpec *sitter.Node, src []byte) (string, *sitter.Node) {
	var name string
	var typeBody *sitter.Node

	for i := 0; i < int(typeSpec.NamedChildCount()); i++ {
		child := typeSpec.NamedChild(i)
		switch child.Type() {
		case "type_identifier":
			name = child.Content(src)
		case "struct_type", "interface_type":
			typeBody = child
		}
	}
	return name, typeBody
}

// extractUnimplementedServer checks if a field_declaration is an embedded
// field matching *.Unimplemented*Server pattern. Returns the package alias
// and the extracted service name.
func extractUnimplementedServer(field *sitter.Node, src []byte) (alias, serviceName string) {
	// Look for a qualified_type or selector_expression in the field
	// Embedded fields in Go tree-sitter are field_declarations with
	// no field name, just a type.
	for i := 0; i < int(field.NamedChildCount()); i++ {
		child := field.NamedChild(i)
		switch child.Type() {
		case "qualified_type":
			// qualified_type has package and type_identifier children
			pkg, typeName := extractQualifiedType(child, src)
			if svc := parseUnimplementedServerName(typeName); svc != "" {
				return pkg, svc
			}
		case "selector_expression":
			// selector_expression: operand.field
			pkg, sel := extractSelectorExpr(child, src)
			if svc := parseUnimplementedServerName(sel); svc != "" {
				return pkg, svc
			}
		}
	}
	return "", ""
}

// parseUnimplementedServerName extracts the service name from a type like
// "UnimplementedUserServiceServer" -> "UserService".
func parseUnimplementedServerName(typeName string) string {
	if !strings.HasPrefix(typeName, "Unimplemented") || !strings.HasSuffix(typeName, "Server") {
		return ""
	}
	// Strip "Unimplemented" prefix and "Server" suffix
	name := strings.TrimPrefix(typeName, "Unimplemented")
	name = strings.TrimSuffix(name, "Server")
	if name == "" {
		return ""
	}
	return name
}

// extractNewClientCall checks if a call_expression matches *.New*Client(...)
// pattern. Returns the package alias and extracted service name.
func extractNewClientCall(callNode *sitter.Node, src []byte) (alias, serviceName string) {
	// The function part of a call_expression is its first named child
	funcExpr := callNode.NamedChild(0)
	if funcExpr == nil {
		return "", ""
	}

	if funcExpr.Type() != "selector_expression" {
		return "", ""
	}

	pkg, sel := extractSelectorExpr(funcExpr, src)
	if pkg == "" || sel == "" {
		return "", ""
	}

	// Check pattern: New*Client
	if !strings.HasPrefix(sel, "New") || !strings.HasSuffix(sel, "Client") {
		return "", ""
	}

	// Extract service name: strip "New" prefix and "Client" suffix
	name := strings.TrimPrefix(sel, "New")
	name = strings.TrimSuffix(name, "Client")
	if name == "" {
		return "", ""
	}

	return pkg, name
}

// extractQualifiedType extracts package and type from a qualified_type node.
func extractQualifiedType(node *sitter.Node, src []byte) (pkg, typeName string) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "package_identifier":
			pkg = child.Content(src)
		case "type_identifier":
			typeName = child.Content(src)
		}
	}
	return pkg, typeName
}

// extractSelectorExpr extracts the operand and field from a selector_expression.
func extractSelectorExpr(node *sitter.Node, src []byte) (operand, field string) {
	// selector_expression has operand (first child) and field_identifier (second named child)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		switch child.Type() {
		case "identifier", "type_identifier", "package_identifier":
			if operand == "" {
				operand = child.Content(src)
			}
		case "field_identifier":
			field = child.Content(src)
		}
	}
	return operand, field
}

// resolveProtoPkgPath resolves the Go import path for a given package alias
// from the imports map. The alias is the local name used in the source code.
func resolveProtoPkgPath(alias string, imports map[string]string) string {
	if path, ok := imports[alias]; ok {
		return path
	}
	return ""
}

// walkFunctions iterates over function and method declarations in the root,
// calling fn for each one with its name and body node.
func walkFunctions(root *sitter.Node, src []byte, fn func(funcName string, funcNode *sitter.Node, body *sitter.Node)) {
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		switch child.Type() {
		case "function_declaration":
			name := extractFuncDeclName(child, src)
			body := findNamedChild(child, "block")
			if name != "" && body != nil {
				fn(name, child, body)
			}
		case "method_declaration":
			name := extractMethodDeclName(child, src)
			body := findNamedChild(child, "block")
			if name != "" && body != nil {
				fn(name, child, body)
			}
		}
	}
}

// walkCallExpressions recursively walks a node tree looking for call_expression
// nodes and calls fn for each one found.
func walkCallExpressions(node *sitter.Node, src []byte, fn func(callNode *sitter.Node)) {
	if node == nil {
		return
	}
	if node.Type() == "call_expression" {
		fn(node)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkCallExpressions(node.NamedChild(i), src, fn)
	}
}

// extractFuncDeclName extracts the function name from a function_declaration.
func extractFuncDeclName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" {
			return child.Content(src)
		}
	}
	return ""
}

// extractMethodDeclName extracts the method name from a method_declaration.
func extractMethodDeclName(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "field_identifier" {
			return child.Content(src)
		}
	}
	return ""
}

// findNamedChild finds the first named child of the given type.
func findNamedChild(node *sitter.Node, childType string) *sitter.Node {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == childType {
			return child
		}
	}
	return nil
}
