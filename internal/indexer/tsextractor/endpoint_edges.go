package tsextractor

import (
	"fmt"
	"net/url"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/types"
)

// httpClientMethods is the set of HTTP method names used by HTTP client libraries
// (axios, http, client, api, etc.) that take a URL as their first argument.
var httpClientMethods = map[string]bool{
	"get":     true,
	"post":    true,
	"put":     true,
	"delete":  true,
	"patch":   true,
	"head":    true,
	"request": true,
}

// httpClientObjects is the set of object names recognized as HTTP client instances.
var httpClientObjects = map[string]bool{
	"axios":      true,
	"http":       true,
	"client":     true,
	"api":        true,
	"httpClient": true,
}

// ExtractEndpointEdges walks a function body for HTTP client call patterns
// and creates 'consumes_endpoint' edges from the function to synthetic endpoint nodes.
func ExtractEndpointEdges(body *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash) ([]types.Node, []types.Edge) {
	if body == nil {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	seen := make(map[types.Hash]struct{})

	walkForEndpointCalls(body, opts, qnamePrefix, sourceHash, &nodes, &edges, seen)
	return nodes, edges
}

// walkForEndpointCalls recursively walks nodes looking for HTTP client call patterns.
func walkForEndpointCalls(node *sitter.Node, opts types.ExtractOptions, qnamePrefix string, sourceHash types.Hash, nodes *[]types.Node, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		httpMethod, urlPath := matchHTTPClientCall(node, opts.Content)
		if urlPath != "" {
			endpointSig := httpMethod + " " + urlPath
			endpointHash := types.ComputeNodeHash(opts.RepoURL, "endpoints", types.EmptyHash, endpointSig, "endpoint")

			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(sourceHash, endpointHash, "consumes_endpoint", provenance)

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
					SourceHash: sourceHash,
					TargetHash: endpointHash,
					EdgeType:   "consumes_endpoint",
					Confidence: 0.6,
					Provenance: provenance,
				}
				*edges = append(*edges, edge)
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForEndpointCalls(node.Child(i), opts, qnamePrefix, sourceHash, nodes, edges, seen)
	}
}

// matchHTTPClientCall checks if a call_expression matches an HTTP client pattern.
// Returns the HTTP method and URL path if matched, or ("", "") if not.
func matchHTTPClientCall(callNode *sitter.Node, content []byte) (string, string) {
	funcNode := callNode.ChildByFieldName("function")
	if funcNode == nil {
		return "", ""
	}

	argsNode := callNode.ChildByFieldName("arguments")
	if argsNode == nil {
		return "", ""
	}

	switch funcNode.Type() {
	case "identifier":
		// Pattern: fetch("/api/users")
		funcName := funcNode.Content(content)
		if funcName == "fetch" {
			urlStr := extractURLArgTS(argsNode, content)
			if urlStr != "" {
				path := extractURLPath(urlStr)
				if path != "" {
					return "ANY", path
				}
			}
		}
		return "", ""

	case "member_expression":
		// Pattern: axios.get("/api/users"), http.post("/api/users"), etc.
		obj := funcNode.ChildByFieldName("object")
		prop := funcNode.ChildByFieldName("property")
		if obj == nil || prop == nil {
			return "", ""
		}

		objName := obj.Content(content)
		methodName := prop.Content(content)

		if !httpClientObjects[objName] {
			return "", ""
		}
		if !httpClientMethods[strings.ToLower(methodName)] {
			return "", ""
		}

		urlStr := extractURLArgTS(argsNode, content)
		if urlStr == "" {
			return "", ""
		}

		path := extractURLPath(urlStr)
		if path == "" {
			return "", ""
		}

		httpMethod := deriveHTTPMethodFromClientCall(methodName)
		return httpMethod, path
	}

	return "", ""
}

// extractURLArgTS extracts a URL string from the first argument in an arguments node.
// Handles string literals and template literals without interpolation.
func extractURLArgTS(argsNode *sitter.Node, content []byte) string {
	for i := 0; i < int(argsNode.ChildCount()); i++ {
		child := argsNode.Child(i)
		switch child.Type() {
		case "string":
			val := child.Content(content)
			val = strings.Trim(val, `"'`)
			if isURLLike(val) {
				return val
			}
		case "template_string":
			// Only extract if no interpolation (no template_substitution children).
			hasInterpolation := false
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(j).Type() == "template_substitution" {
					hasInterpolation = true
					break
				}
			}
			if !hasInterpolation {
				val := child.Content(content)
				val = strings.Trim(val, "`")
				if isURLLike(val) {
					return val
				}
			}
		}
	}
	return ""
}

// isURLLike returns true if a string looks like a URL path or full URL.
func isURLLike(s string) bool {
	return strings.HasPrefix(s, "/") || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// extractURLPath extracts the path portion from a URL string.
// "/api/users" -> "/api/users"
// "http://service/api/users" -> "/api/users"
// "https://api.example.com/v1/users" -> "/v1/users"
func extractURLPath(rawURL string) string {
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

// deriveHTTPMethodFromClientCall maps an HTTP client method name to an HTTP method string.
func deriveHTTPMethodFromClientCall(methodName string) string {
	switch strings.ToLower(methodName) {
	case "get":
		return "GET"
	case "post":
		return "POST"
	case "put":
		return "PUT"
	case "delete":
		return "DELETE"
	case "patch":
		return "PATCH"
	case "head":
		return "HEAD"
	case "request":
		return "ANY"
	default:
		return "ANY"
	}
}
