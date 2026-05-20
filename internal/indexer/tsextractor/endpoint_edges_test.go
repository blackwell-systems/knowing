package tsextractor

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/blackwell-systems/knowing/internal/types"
)

// parseTSSource parses TypeScript source and returns the tree-sitter root node.
func parseTSSource(t *testing.T, source string) (*sitter.Node, *sitter.Tree) {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(source))
	if err != nil {
		t.Fatalf("tree-sitter parse error: %v", err)
	}
	return tree.RootNode(), tree
}

// findFunctionBody finds the body of the first function_declaration in the tree.
func findFunctionBody(root *sitter.Node) *sitter.Node {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_declaration" {
			return child.ChildByFieldName("body")
		}
	}
	return nil
}

func TestExtractEndpointEdges_Fetch(t *testing.T) {
	source := `function loadUsers() {
  fetch("/api/users")
}
`
	opts := makeOpts(t, "api.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "api"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "loadUsers", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	edge := edges[0]
	if edge.EdgeType != "consumes_endpoint" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "consumes_endpoint")
	}
	if edge.Confidence != 0.6 {
		t.Errorf("Confidence = %v, want 0.6", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("Provenance = %q, want %q", edge.Provenance, "ast_inferred")
	}
	if edge.SourceHash != sourceHash {
		t.Errorf("SourceHash mismatch")
	}

	node := nodes[0]
	if node.Kind != "endpoint" {
		t.Errorf("node Kind = %q, want %q", node.Kind, "endpoint")
	}
	if node.Signature != "ANY /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "ANY /api/users")
	}
}

func TestExtractEndpointEdges_Axios(t *testing.T) {
	source := `function getUsers() {
  axios.get("/api/users")
}
`
	opts := makeOpts(t, "api.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "api"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "getUsers", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "consumes_endpoint" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "consumes_endpoint")
	}

	node := nodes[0]
	if node.Signature != "GET /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "GET /api/users")
	}
}

func TestExtractEndpointEdges_AxiosPost(t *testing.T) {
	source := `function createUser() {
  axios.post("/api/users", { name: "test" })
}
`
	opts := makeOpts(t, "api.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "api"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "createUser", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	node := nodes[0]
	if node.Signature != "POST /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "POST /api/users")
	}
}

func TestExtractEndpointEdges_FullURL(t *testing.T) {
	source := `function loadRemoteUsers() {
  fetch("http://service/api/users")
}
`
	opts := makeOpts(t, "api.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "api"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "loadRemoteUsers", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	node := nodes[0]
	if node.Signature != "ANY /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "ANY /api/users")
	}
}

func TestExtractEndpointEdges_NoHTTPCalls(t *testing.T) {
	source := `function computeStuff() {
  const x = 1 + 2
  console.log(x)
}
`
	opts := makeOpts(t, "util.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "util"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "computeStuff", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestExtractEndpointEdges_DynamicURL(t *testing.T) {
	source := "function loadUser() {\n  fetch(`/api/users/${userId}`)\n}\n"
	opts := makeOpts(t, "api.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "api"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "loadUser", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for dynamic URL, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges for dynamic URL, got %d", len(edges))
	}
}

func TestExtractEndpointEdges_MultipleEndpoints(t *testing.T) {
	source := `function loadAll() {
  fetch("/api/users")
  axios.get("/api/posts")
  http.post("/api/comments")
}
`
	opts := makeOpts(t, "api.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "api"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "loadAll", "function")

	nodes, edges := ExtractEndpointEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(edges))
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	// Verify each edge has a unique target.
	targetSet := make(map[types.Hash]struct{})
	for _, e := range edges {
		if e.EdgeType != "consumes_endpoint" {
			t.Errorf("EdgeType = %q, want %q", e.EdgeType, "consumes_endpoint")
		}
		targetSet[e.TargetHash] = struct{}{}
	}
	if len(targetSet) != 3 {
		t.Errorf("expected 3 unique targets, got %d", len(targetSet))
	}
}
