package gotsextractor

import (
	"fmt"
	"net/url"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// goHTTPClientMethods maps Go HTTP client method names to whether they have a URL argument.
// Methods with true have a URL as their first string argument.
var goHTTPClientMethods = map[string]bool{
	"Get":  true,
	"Post": true,
	"Head": true,
}

// goHTTPClientNewRequest is the special-case method where the HTTP method is the
// first argument and the URL is the second argument.
const goHTTPClientNewRequest = "NewRequest"

// goHTTPClientPackages are Go import paths recognized as HTTP client packages.
var goHTTPClientPackages = map[string]bool{
	"net/http": true,
}

// ExtractGoEndpointEdges walks a function body for Go HTTP client call patterns
// and creates 'consumes_endpoint' edges.
func ExtractGoEndpointEdges(body *sitter.Node, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, imports map[string]string) ([]types.Node, []types.Edge) {
	if body == nil {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	seen := make(map[types.Hash]struct{})

	walkForGoEndpointCalls(body, opts, pkgPath, funcNodeHash, imports, &nodes, &edges, seen)
	return nodes, edges
}

// walkForGoEndpointCalls recursively walks nodes looking for Go HTTP client call patterns.
func walkForGoEndpointCalls(node *sitter.Node, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, imports map[string]string, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		httpMethod, urlPath := matchGoHTTPClientCall(node, opts.Content, imports)
		if urlPath != "" {
			endpointSig := httpMethod + " " + urlPath
			endpointHash := types.ComputeNodeHash(opts.RepoURL, "endpoints", types.EmptyHash, endpointSig, "endpoint")

			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(funcNodeHash, endpointHash, edgetype.ConsumesEndpoint, provenance)

			// Deduplicate by edge hash.
			if _, exists := seen[edgeHash]; !exists {
				seen[edgeHash] = struct{}{}

				endpointNode := types.Node{
					NodeHash:      endpointHash,
					FileHash:      types.EmptyHash,
					QualifiedName: fmt.Sprintf("%s://endpoints.%s", opts.RepoURL, endpointSig),
					Kind:          "endpoint",
					Signature:     endpointSig,
				}
				*nodes = append(*nodes, endpointNode)

				edge := types.Edge{
					EdgeHash:   edgeHash,
					SourceHash: funcNodeHash,
					TargetHash: endpointHash,
					EdgeType:   edgetype.ConsumesEndpoint,
					Confidence: 0.6,
					Provenance: provenance,
				}
				*edges = append(*edges, edge)
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForGoEndpointCalls(node.Child(i), opts, pkgPath, funcNodeHash, imports, nodes, edges, seen)
	}
}

// matchGoHTTPClientCall checks if a call_expression matches a Go HTTP client pattern.
// Returns the HTTP method and URL path if matched, or ("", "") if not.
func matchGoHTTPClientCall(callNode *sitter.Node, content []byte, imports map[string]string) (string, string) {
	funcNode := callNode.ChildByFieldName("function")
	if funcNode == nil || funcNode.Type() != "selector_expression" {
		return "", ""
	}

	operandNode := funcNode.ChildByFieldName("operand")
	fieldNode := funcNode.ChildByFieldName("field")
	if operandNode == nil || fieldNode == nil {
		return "", ""
	}

	methodName := fieldNode.Content(content)
	receiverName := operandNode.Content(content)

	// Check if the receiver is a known HTTP client package alias.
	isHTTPPkg := false
	if operandNode.Type() == "identifier" {
		if importPath, ok := imports[receiverName]; ok {
			if goHTTPClientPackages[importPath] {
				isHTTPPkg = true
			}
		}
		// Also accept bare "http" or "client" as receivers (common patterns).
		if receiverName == "http" || receiverName == "client" {
			isHTTPPkg = true
		}
	}

	if !isHTTPPkg {
		return "", ""
	}

	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return "", ""
	}

	// Handle NewRequest special case: first arg is method, second is URL.
	if methodName == goHTTPClientNewRequest {
		return matchNewRequestPattern(argsNode, content)
	}

	// Standard methods: Get, Post, Head.
	if !goHTTPClientMethods[methodName] {
		return "", ""
	}

	urlStr := extractGoFirstStringArg(argsNode, content)
	if urlStr == "" {
		return "", ""
	}

	path := goExtractURLPath(urlStr)
	if path == "" {
		return "", ""
	}

	httpMethod := deriveGoHTTPMethod(methodName)
	return httpMethod, path
}

// matchNewRequestPattern extracts HTTP method and URL from http.NewRequest("METHOD", "url", ...).
func matchNewRequestPattern(argsNode *sitter.Node, content []byte) (string, string) {
	args := collectGoStringArgs(argsNode, content, 2)
	if len(args) < 2 {
		return "", ""
	}

	httpMethod := strings.ToUpper(args[0])
	if httpMethod == "" {
		return "", ""
	}

	path := goExtractURLPath(args[1])
	if path == "" {
		return "", ""
	}

	return httpMethod, path
}

// collectGoStringArgs collects up to maxArgs string literal argument values from an argument list.
func collectGoStringArgs(argsNode *sitter.Node, content []byte, maxArgs int) []string {
	var results []string
	for i := 0; i < int(argsNode.ChildCount()) && len(results) < maxArgs; i++ {
		child := argsNode.Child(i)
		switch child.Type() {
		case "interpreted_string_literal":
			val := child.Content(content)
			val = strings.Trim(val, `"`)
			results = append(results, val)
		case "raw_string_literal":
			val := child.Content(content)
			val = strings.Trim(val, "`")
			results = append(results, val)
		}
	}
	return results
}

// extractGoFirstStringArg extracts the first string literal argument value.
func extractGoFirstStringArg(argsNode *sitter.Node, content []byte) string {
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		switch child.Type() {
		case "interpreted_string_literal":
			val := child.Content(content)
			return strings.Trim(val, `"`)
		case "raw_string_literal":
			val := child.Content(content)
			return strings.Trim(val, "`")
		}
	}
	return ""
}

// goExtractURLPath extracts the path portion from a URL string.
// "/api/users" -> "/api/users"
// "http://service/api/users" -> "/api/users"
// "https://api.example.com/v1/users" -> "/v1/users"
func goExtractURLPath(rawURL string) string {
	if strings.HasPrefix(rawURL, "/") {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Path == "" || u.Path == "/" {
		return ""
	}
	return u.Path
}

// deriveGoHTTPMethod maps a Go HTTP client method name to an HTTP method string.
func deriveGoHTTPMethod(methodName string) string {
	switch methodName {
	case "Get":
		return "GET"
	case "Post":
		return "POST"
	case "Head":
		return "HEAD"
	default:
		return "ANY"
	}
}
