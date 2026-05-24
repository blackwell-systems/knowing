package indexer

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestComputeSimilarityEdges(t *testing.T) {
	// Create nodes with similar qualified names in the same package.
	repoURL := "https://github.com/example/app"
	pkg := "internal/handlers"

	makeNode := func(name, sig string) types.Node {
		qn := repoURL + "://" + pkg + "." + name
		return types.Node{
			NodeHash:      types.NewHash([]byte(qn)),
			QualifiedName: qn,
			Kind:          "function",
			Signature:     sig,
		}
	}

	nodes := []types.Node{
		makeNode("HandleUserCreate", "func HandleUserCreate(ctx context.Context, req CreateUserRequest) error"),
		makeNode("HandleUserUpdate", "func HandleUserUpdate(ctx context.Context, req UpdateUserRequest) error"),
		makeNode("HandleUserDelete", "func HandleUserDelete(ctx context.Context, req DeleteUserRequest) error"),
		makeNode("HandleOrderCreate", "func HandleOrderCreate(ctx context.Context, req CreateOrderRequest) error"),
		makeNode("ParseConfig", "func ParseConfig(path string) (*Config, error)"),
	}

	edges := ComputeSimilarityEdges(nodes, 0.5)

	// Verify that HandleUserCreate and HandleUserUpdate get a similar_to edge.
	// They share tokens: "handle", "user", "func", "ctx", "context", "request", "error".
	found := make(map[[2]string]bool)
	for _, e := range edges {
		// Map hashes back to names for readability.
		var sourceName, targetName string
		for _, n := range nodes {
			if n.NodeHash == e.SourceHash {
				sourceName = n.QualifiedName
			}
			if n.NodeHash == e.TargetHash {
				targetName = n.QualifiedName
			}
		}
		found[[2]string{sourceName, targetName}] = true
		found[[2]string{targetName, sourceName}] = true // bidirectional lookup

		// Verify edge properties.
		if e.EdgeType != "similar_to" {
			t.Errorf("expected edge type similar_to, got %s", e.EdgeType)
		}
		if e.Confidence <= 0.5 || e.Confidence > 1.0 {
			t.Errorf("expected confidence > 0.5, got %f", e.Confidence)
		}
		if e.Provenance != "similarity" {
			t.Errorf("expected provenance 'similarity', got %s", e.Provenance)
		}
	}

	// HandleUserCreate <-> HandleUserUpdate should be similar.
	createQN := repoURL + "://" + pkg + ".HandleUserCreate"
	updateQN := repoURL + "://" + pkg + ".HandleUserUpdate"
	if !found[[2]string{createQN, updateQN}] {
		t.Errorf("expected similar_to edge between HandleUserCreate and HandleUserUpdate, got none")
		t.Logf("edges found: %d", len(edges))
		for _, e := range edges {
			for _, n := range nodes {
				if n.NodeHash == e.SourceHash {
					t.Logf("  %s -> (jaccard=%.3f)", n.QualifiedName, e.Confidence)
				}
			}
		}
	}

	// ParseConfig should NOT be similar to any HandleUser* function.
	configQN := repoURL + "://" + pkg + ".ParseConfig"
	for _, userFunc := range []string{createQN, updateQN} {
		if found[[2]string{configQN, userFunc}] {
			t.Errorf("ParseConfig should NOT have similar_to edge with %s", userFunc)
		}
	}
}

func TestComputeSimilarityEdges_MaxCap(t *testing.T) {
	// Create 10 nearly identical functions in the same package.
	// With a cap of 5 per node, no node should exceed 5 edges.
	repoURL := "https://github.com/example/app"
	pkg := "internal/handlers"

	var nodes []types.Node
	for i := 0; i < 10; i++ {
		name := "HandleUserAction" + string(rune('A'+i))
		qn := repoURL + "://" + pkg + "." + name
		nodes = append(nodes, types.Node{
			NodeHash:      types.NewHash([]byte(qn)),
			QualifiedName: qn,
			Kind:          "function",
			Signature:     "func " + name + "(ctx context.Context, req UserActionRequest) error",
		})
	}

	edges := ComputeSimilarityEdges(nodes, 0.5)

	// Count edges per node.
	edgeCount := make(map[types.Hash]int)
	for _, e := range edges {
		edgeCount[e.SourceHash]++
		edgeCount[e.TargetHash]++
	}

	for hash, count := range edgeCount {
		if count > 5 {
			t.Errorf("node %x has %d similar_to edges, exceeding cap of 5", hash[:4], count)
		}
	}
}

func TestComputeSimilarityEdges_CrossPackageIsolation(t *testing.T) {
	// Functions in different packages should never get similar_to edges,
	// even if their names are identical.
	repoURL := "https://github.com/example/app"

	node1 := types.Node{
		NodeHash:      types.NewHash([]byte(repoURL + "://pkg1.HandleUserCreate")),
		QualifiedName: repoURL + "://pkg1.HandleUserCreate",
		Kind:          "function",
		Signature:     "func HandleUserCreate(ctx context.Context) error",
	}
	node2 := types.Node{
		NodeHash:      types.NewHash([]byte(repoURL + "://pkg2.HandleUserCreate")),
		QualifiedName: repoURL + "://pkg2.HandleUserCreate",
		Kind:          "function",
		Signature:     "func HandleUserCreate(ctx context.Context) error",
	}

	edges := ComputeSimilarityEdges([]types.Node{node1, node2}, 0.5)
	if len(edges) != 0 {
		t.Errorf("expected no edges across packages, got %d", len(edges))
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("github.com/example/app://internal/handlers.HandleUserCreate", "func HandleUserCreate(ctx context.Context, req CreateUserRequest) error")

	// Should contain meaningful tokens.
	expected := []string{"handle", "user", "create", "context", "request", "error", "internal", "handlers"}
	for _, tok := range expected {
		if !tokens[tok] {
			t.Errorf("expected token %q in result, tokens: %v", tok, tokens)
		}
	}

	// Should NOT contain short tokens.
	short := []string{"", "a", "ab"}
	for _, tok := range short {
		if tokens[tok] {
			t.Errorf("unexpected short token %q in result", tok)
		}
	}
}

func TestJaccard(t *testing.T) {
	a := map[string]bool{"handle": true, "user": true, "create": true, "context": true}
	b := map[string]bool{"handle": true, "user": true, "update": true, "context": true}

	// Intersection: handle, user, context = 3
	// Union: handle, user, create, update, context = 5
	// Jaccard = 3/5 = 0.6
	j := jaccard(a, b)
	if j < 0.59 || j > 0.61 {
		t.Errorf("expected Jaccard ~0.6, got %f", j)
	}

	// Completely disjoint sets.
	c := map[string]bool{"parse": true, "config": true, "path": true}
	j2 := jaccard(a, c)
	if j2 != 0 {
		t.Errorf("expected Jaccard 0 for disjoint sets, got %f", j2)
	}
}

func TestExtractPackage(t *testing.T) {
	tests := []struct {
		qn   string
		want string
	}{
		{"https://github.com/example/app://internal/handlers.HandleUserCreate", "internal/handlers"},
		{"https://github.com/example/app://cmd/server.main", "cmd/server"},
		{"https://github.com/example/app://pkg.Func", "pkg"},
	}
	for _, tt := range tests {
		got := extractPackage(tt.qn)
		if got != tt.want {
			t.Errorf("extractPackage(%q) = %q, want %q", tt.qn, got, tt.want)
		}
	}
}
