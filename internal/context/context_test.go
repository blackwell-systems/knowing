package context

import (
	stdctx "context"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockStore implements types.GraphStore for testing.
type mockStore struct {
	nodes []types.Node
	edges []types.Edge
	files []types.File
}

func (m *mockStore) PutNode(_ stdctx.Context, _ types.Node) error   { return nil }
func (m *mockStore) PutEdge(_ stdctx.Context, _ types.Edge) error   { return nil }
func (m *mockStore) PutFile(_ stdctx.Context, _ types.File) error   { return nil }
func (m *mockStore) PutRepo(_ stdctx.Context, _ types.Repo) error   { return nil }
func (m *mockStore) RecordEdgeEvent(_ stdctx.Context, _ types.EdgeEvent) error {
	return nil
}
func (m *mockStore) CreateSnapshot(_ stdctx.Context, _ types.Snapshot) error { return nil }

func (m *mockStore) GetNode(_ stdctx.Context, hash types.Hash) (*types.Node, error) {
	for i := range m.nodes {
		if m.nodes[i].NodeHash == hash {
			return &m.nodes[i], nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetEdge(_ stdctx.Context, hash types.Hash) (*types.Edge, error) {
	for i := range m.edges {
		if m.edges[i].EdgeHash == hash {
			return &m.edges[i], nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetSnapshot(_ stdctx.Context, _ types.Hash) (*types.Snapshot, error) {
	return nil, nil
}
func (m *mockStore) GetRepo(_ stdctx.Context, _ types.Hash) (*types.Repo, error) {
	return nil, nil
}

func (m *mockStore) NodesByName(_ stdctx.Context, prefix string) ([]types.Node, error) {
	if prefix == "" {
		return m.nodes, nil
	}
	var result []types.Node
	for _, n := range m.nodes {
		if len(n.QualifiedName) >= len(prefix) && containsPrefix(n.QualifiedName, prefix) {
			result = append(result, n)
		}
	}
	return result, nil
}

func containsPrefix(name, prefix string) bool {
	// Case-insensitive prefix match anywhere in the qualified name.
	nameLower := toLower(name)
	prefixLower := toLower(prefix)
	for i := 0; i <= len(nameLower)-len(prefixLower); i++ {
		if nameLower[i:i+len(prefixLower)] == prefixLower {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		if s[i] >= 'A' && s[i] <= 'Z' {
			b[i] = s[i] + 32
		} else {
			b[i] = s[i]
		}
	}
	return string(b)
}

func (m *mockStore) EdgesFrom(_ stdctx.Context, sourceHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.SourceHash == sourceHash && e.EdgeType == edgeType {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) EdgesTo(_ stdctx.Context, targetHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.TargetHash == targetHash && e.EdgeType == edgeType {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) DanglingEdges(_ stdctx.Context) ([]types.Edge, error) { return nil, nil }
func (m *mockStore) AllRepos(_ stdctx.Context) ([]types.Repo, error)      { return nil, nil }
func (m *mockStore) NodesByQualifiedName(_ stdctx.Context, _ string) ([]types.Node, error) {
	return nil, nil
}
func (m *mockStore) DeleteEdge(_ stdctx.Context, _ types.Hash) error { return nil }
func (m *mockStore) DeleteNodesByFile(_ stdctx.Context, _ types.Hash) (int, error) {
	return 0, nil
}
func (m *mockStore) DeleteEdgesBySourceFile(_ stdctx.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}
func (m *mockStore) EdgesBySourceFile(_ stdctx.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}
func (m *mockStore) TransitiveCallers(_ stdctx.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) {
	return nil, nil
}
func (m *mockStore) TransitiveCallees(_ stdctx.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) {
	return nil, nil
}
func (m *mockStore) BlastRadius(_ stdctx.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) {
	return nil, nil
}
func (m *mockStore) SnapshotDiff(_ stdctx.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return nil, nil
}
func (m *mockStore) StaleEdges(_ stdctx.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}
func (m *mockStore) LatestSnapshot(_ stdctx.Context, _ types.Hash) (*types.Snapshot, error) {
	return nil, nil
}
func (m *mockStore) FilesByRepo(_ stdctx.Context, _ types.Hash) ([]types.File, error) {
	return nil, nil
}
func (m *mockStore) FileByPath(_ stdctx.Context, repoHash types.Hash, path string) (*types.File, error) {
	for i := range m.files {
		if m.files[i].RepoHash == repoHash && m.files[i].Path == path {
			return &m.files[i], nil
		}
	}
	return nil, nil
}
func (m *mockStore) Close() error { return nil }

// --- Tests ---

func TestForTask_EmptyDescription(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	block, err := engine.ForTask(stdctx.Background(), TaskOptions{
		TaskDescription: "",
		TokenBudget:     1000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(block.Symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(block.Symbols))
	}
}

func TestForTask_MatchingKeywords(t *testing.T) {
	authNode := types.Node{
		NodeHash:      types.NewHash([]byte("auth-node")),
		QualifiedName: "github.com/org/repo://pkg.AuthService",
		Kind:          "type",
		Signature:     "type AuthService struct",
	}
	callerNode := types.Node{
		NodeHash:      types.NewHash([]byte("caller-node")),
		QualifiedName: "github.com/org/repo://pkg.HandleLogin",
		Kind:          "function",
		Signature:     "func HandleLogin()",
	}

	edge := types.Edge{
		EdgeHash:     types.NewHash([]byte("edge-1")),
		SourceHash:   callerNode.NodeHash,
		TargetHash:   authNode.NodeHash,
		EdgeType:     "calls",
		Confidence:   0.9,
		LastObserved: time.Now().Unix(),
	}

	store := &mockStore{
		nodes: []types.Node{authNode, callerNode},
		edges: []types.Edge{edge},
	}
	engine := NewContextEngine(store)

	block, err := engine.ForTask(stdctx.Background(), TaskOptions{
		TaskDescription: "refactor auth",
		TokenBudget:     50000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(block.Symbols) == 0 {
		t.Error("expected symbols to be returned for 'refactor auth' query")
	}

	// Should find the auth node.
	found := false
	for _, s := range block.Symbols {
		if s.Node.NodeHash == authNode.NodeHash {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected AuthService node in results")
	}
}

func TestForTask_TokenBudgetRespected(t *testing.T) {
	// Create many nodes with long names to consume tokens.
	var nodes []types.Node
	for i := 0; i < 50; i++ {
		name := "github.com/org/repo://pkg.VeryLongFunctionName" + string(rune('A'+i%26)) + string(rune('a'+i/26))
		nodes = append(nodes, types.Node{
			NodeHash:      types.NewHash([]byte(name)),
			QualifiedName: name,
			Kind:          "function",
			Signature:     "func " + name + "(ctx context.Context, req *Request) (*Response, error)",
		})
	}

	store := &mockStore{nodes: nodes}
	engine := NewContextEngine(store)

	block, err := engine.ForTask(stdctx.Background(), TaskOptions{
		TaskDescription: "VeryLongFunctionName",
		TokenBudget:     100, // Very small budget.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.TokensUsed > block.TokenBudget {
		t.Errorf("TokensUsed (%d) exceeds TokenBudget (%d)", block.TokensUsed, block.TokenBudget)
	}
}

func TestForTask_DefaultBudget(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	block, err := engine.ForTask(stdctx.Background(), TaskOptions{
		TaskDescription: "test",
		TokenBudget:     0, // Should default to 50000.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if block.TokenBudget != 50000 {
		t.Errorf("expected default budget 50000, got %d", block.TokenBudget)
	}
}

func TestForFiles_ValidFiles(t *testing.T) {
	repoURL := "github.com/org/repo"
	repoHash := types.NewHash([]byte(repoURL))
	fileHash := types.NewHash([]byte("file-content"))

	file := types.File{
		FileHash: fileHash,
		RepoHash: repoHash,
		Path:     "pkg/auth.go",
	}

	node := types.Node{
		NodeHash:      types.NewHash([]byte("auth-func")),
		FileHash:      fileHash,
		QualifiedName: "github.com/org/repo://pkg.Authenticate",
		Kind:          "function",
		Signature:     "func Authenticate(token string) error",
	}

	store := &mockStore{
		nodes: []types.Node{node},
		files: []types.File{file},
	}
	engine := NewContextEngine(store)

	block, err := engine.ForFiles(stdctx.Background(), FileOptions{
		Files:       []string{"pkg/auth.go"},
		RepoURL:     repoURL,
		TokenBudget: 50000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(block.Symbols) == 0 {
		t.Error("expected symbols for valid file")
	}

	found := false
	for _, s := range block.Symbols {
		if s.Node.NodeHash == node.NodeHash {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Authenticate node in results")
	}
}

func TestForFiles_NoMatchingFiles(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	block, err := engine.ForFiles(stdctx.Background(), FileOptions{
		Files:       []string{},
		RepoURL:     "github.com/org/repo",
		TokenBudget: 50000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(block.Symbols) != 0 {
		t.Errorf("expected 0 symbols for empty file list, got %d", len(block.Symbols))
	}
}
