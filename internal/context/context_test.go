package context

import (
	stdctx "context"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/testutil"
	"github.com/blackwell-systems/knowing/internal/types"
)

// mockStore embeds testutil.MockGraphStore for no-op defaults and adds
// slice-based storage that the context engine tests rely on for iteration.
type mockStore struct {
	testutil.MockGraphStore
	nodes []types.Node
	edges []types.Edge
	files []types.File
}

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

func (m *mockStore) NodesByName(_ stdctx.Context, prefix string) ([]types.Node, error) {
	if prefix == "" {
		return m.nodes, nil
	}
	// Strip leading % to simulate LIKE %keyword% behavior (same as SQLite).
	search := prefix
	if len(search) > 0 && search[0] == '%' {
		search = search[1:]
	}
	var result []types.Node
	for _, n := range m.nodes {
		if len(n.QualifiedName) >= len(search) && containsPrefix(n.QualifiedName, search) {
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
		if e.SourceHash == sourceHash && (edgeType == "" || e.EdgeType == edgeType) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) EdgesTo(_ stdctx.Context, targetHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.TargetHash == targetHash && (edgeType == "" || e.EdgeType == edgeType) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockStore) FileByPath(_ stdctx.Context, repoHash types.Hash, path string) (*types.File, error) {
	for i := range m.files {
		if m.files[i].RepoHash == repoHash && m.files[i].Path == path {
			return &m.files[i], nil
		}
	}
	return nil, nil
}

func (m *mockStore) NodesByFilePath(_ stdctx.Context, repoHash types.Hash, path string) ([]types.Node, error) {
	file, _ := m.FileByPath(nil, repoHash, path)
	if file == nil {
		return nil, nil
	}
	var nodes []types.Node
	for _, n := range m.nodes {
		if n.FileHash == file.FileHash {
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

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

func TestForTask_WithRWR(t *testing.T) {
	// Build a small graph: seed node "handler" calls "service" which calls "repo".
	// RWR should produce differentiated (non-flat) scores.
	handlerNode := types.Node{
		NodeHash:      types.NewHash([]byte("handler-node")),
		QualifiedName: "github.com/org/repo://pkg.Handler",
		Kind:          "function",
		Signature:     "func Handler()",
	}
	serviceNode := types.Node{
		NodeHash:      types.NewHash([]byte("service-node")),
		QualifiedName: "github.com/org/repo://pkg.Service",
		Kind:          "function",
		Signature:     "func Service()",
	}
	repoNode := types.Node{
		NodeHash:      types.NewHash([]byte("repo-node")),
		QualifiedName: "github.com/org/repo://pkg.Repository",
		Kind:          "function",
		Signature:     "func Repository()",
	}

	edges := []types.Edge{
		{
			EdgeHash:     types.NewHash([]byte("e-handler-service")),
			SourceHash:   handlerNode.NodeHash,
			TargetHash:   serviceNode.NodeHash,
			EdgeType:     "calls",
			Confidence:   0.9,
			LastObserved: time.Now().Unix(),
		},
		{
			EdgeHash:     types.NewHash([]byte("e-service-repo")),
			SourceHash:   serviceNode.NodeHash,
			TargetHash:   repoNode.NodeHash,
			EdgeType:     "calls",
			Confidence:   0.9,
			LastObserved: time.Now().Unix(),
		},
	}

	store := &mockStore{
		nodes: []types.Node{handlerNode, serviceNode, repoNode},
		edges: edges,
	}
	engine := NewContextEngine(store)

	block, err := engine.ForTask(stdctx.Background(), TaskOptions{
		TaskDescription: "Handler",
		TokenBudget:     50000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(block.Symbols) == 0 {
		t.Fatal("expected symbols from RWR walk")
	}

	// Scores should be differentiated: not all the same value.
	if len(block.Symbols) > 1 {
		first := block.Symbols[0].Score
		allSame := true
		for _, s := range block.Symbols[1:] {
			if s.Score != first {
				allSame = false
				break
			}
		}
		if allSame {
			t.Error("expected differentiated scores from RWR, got all identical")
		}
	}
}

func TestForFiles_EmptyDB(t *testing.T) {
	// A completely empty store (no nodes, no files, no edges) should
	// return gracefully with zero symbols and no error.
	store := &mockStore{}
	engine := NewContextEngine(store)

	block, err := engine.ForFiles(stdctx.Background(), FileOptions{
		Files:       []string{"nonexistent/file.go", "another/missing.go"},
		RepoURL:     "github.com/org/repo",
		TokenBudget: 10000,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(block.Symbols) != 0 {
		t.Errorf("expected 0 symbols for empty DB, got %d", len(block.Symbols))
	}
	if block.TokenBudget != 10000 {
		t.Errorf("expected budget 10000, got %d", block.TokenBudget)
	}
}

func TestExtractKeywordSet_BacktickQuoted(t *testing.T) {
	ks := extractKeywordSet("add a `before_request` hook")

	// Backtick-quoted identifier should be in Exact.
	found := false
	for _, e := range ks.Exact {
		if e == "before_request" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'before_request' in Exact, got %v", ks.Exact)
	}

	// Components should NOT contain the compound (it's already in Exact).
	for _, c := range ks.Components {
		if c == "before_request" {
			t.Errorf("compound 'before_request' should not be in Components")
		}
	}
}

func TestExtractKeywordSet_SnakeCaseCompound(t *testing.T) {
	ks := extractKeywordSet("fix the get_queryset method")

	// "get_queryset" contains underscore, so it should be in Compounds.
	found := false
	for _, c := range ks.Compounds {
		if c == "get_queryset" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'get_queryset' in Compounds, got %v", ks.Compounds)
	}

	// "queryset" component should be in Components as fallback.
	// ("get" is a stop word so it won't appear.)
	hasQueryset := false
	for _, c := range ks.Components {
		if c == "queryset" {
			hasQueryset = true
		}
	}
	if !hasQueryset {
		t.Errorf("expected 'queryset' in Components, got %v", ks.Components)
	}
}

func TestExtractKeywordSet_CamelCaseCompound(t *testing.T) {
	ks := extractKeywordSet("refactor SessionHandler middleware")

	// "SessionHandler" is CamelCase, should be in Compounds.
	found := false
	for _, c := range ks.Compounds {
		if c == "sessionhandler" || c == "SessionHandler" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'SessionHandler' or 'sessionhandler' in Compounds, got %v", ks.Compounds)
	}
}

func TestExtractKeywordSet_PrimaryOrder(t *testing.T) {
	ks := extractKeywordSet("add a before_request hook for auth")

	// Primary() should include exact + compounds but not components.
	primary := ks.Primary()
	all := ks.All()

	if len(primary) >= len(all) {
		t.Errorf("expected Primary() (%d) < All() (%d)", len(primary), len(all))
	}

	// Exact should come before compounds in All().
	if len(ks.Exact) > 0 && len(ks.Compounds) > 0 {
		allStr := ""
		for _, a := range all {
			allStr += a + " "
		}
		// First exact should appear before first compound in the all list.
		firstExactIdx := -1
		firstCompoundIdx := -1
		for i, a := range all {
			if firstExactIdx == -1 {
				for _, e := range ks.Exact {
					if a == e {
						firstExactIdx = i
						break
					}
				}
			}
			if firstCompoundIdx == -1 {
				for _, c := range ks.Compounds {
					if a == c {
						firstCompoundIdx = i
						break
					}
				}
			}
		}
		if firstExactIdx >= 0 && firstCompoundIdx >= 0 && firstExactIdx > firstCompoundIdx {
			t.Errorf("expected Exact keywords before Compounds in All()")
		}
	}
}

func TestExtractKeywordSet_BigramCompounds(t *testing.T) {
	ks := extractKeywordSet("compute blast radius for this module")

	// Bigrams are speculative, so they go to Components (not Compounds).
	// "blast radius" should generate "BlastRadius" and "blast_radius".
	hasCamel, hasSnake := false, false
	for _, c := range ks.Components {
		if c == "BlastRadius" {
			hasCamel = true
		}
		if c == "blast_radius" {
			hasSnake = true
		}
	}
	if !hasCamel {
		t.Errorf("expected 'BlastRadius' in Components, got %v", ks.Components)
	}
	if !hasSnake {
		t.Errorf("expected 'blast_radius' in Components, got %v", ks.Components)
	}
}

func TestExtractKeywordSet_Empty(t *testing.T) {
	ks := extractKeywordSet("")
	if !ks.IsEmpty() {
		t.Errorf("expected empty KeywordSet for empty input")
	}
}

func TestExtractKeywordSet_MultipleBackticks(t *testing.T) {
	ks := extractKeywordSet("connect `AuthService` to `SessionStore`")

	if len(ks.Exact) < 2 {
		t.Fatalf("expected at least 2 exact keywords, got %v", ks.Exact)
	}
	hasAuth, hasSession := false, false
	for _, e := range ks.Exact {
		if e == "AuthService" {
			hasAuth = true
		}
		if e == "SessionStore" {
			hasSession = true
		}
	}
	if !hasAuth {
		t.Errorf("expected 'AuthService' in Exact, got %v", ks.Exact)
	}
	if !hasSession {
		t.Errorf("expected 'SessionStore' in Exact, got %v", ks.Exact)
	}
}

func TestExtractKeywordSet_FlaskBeforeRequest(t *testing.T) {
	// The flagship benchmark case: "Add a new before_request hook..."
	// Before the fix, tiered search queried "before" and "request" separately
	// and found thousands of irrelevant matches. After the fix, "before_request"
	// is a Compound that gets queried first.
	ks := extractKeywordSet("Add a new before_request hook that validates API keys from the Authorization header before each request")

	// "before_request" must be in Compounds (it has underscore).
	found := false
	for _, c := range ks.Compounds {
		if c == "before_request" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'before_request' in Compounds, got %v", ks.Compounds)
	}

	// Primary() should contain "before_request" for first-pass querying.
	primary := ks.Primary()
	primaryHas := false
	for _, p := range primary {
		if p == "before_request" {
			primaryHas = true
			break
		}
	}
	if !primaryHas {
		t.Errorf("expected 'before_request' in Primary(), got %v", primary)
	}

	// "before" and "request" should only be in Components (fallback).
	for _, c := range ks.Compounds {
		if c == "before" || c == "request" {
			t.Errorf("individual word %q should not be in Compounds", c)
		}
	}
	for _, e := range ks.Exact {
		if e == "before" || e == "request" {
			t.Errorf("individual word %q should not be in Exact", e)
		}
	}
}

func TestExtractKeywords_BackwardCompat(t *testing.T) {
	// extractKeywords should still return a flat list in priority order.
	kws := extractKeywords("add a `before_request` hook")
	if len(kws) == 0 {
		t.Fatal("expected non-empty keywords")
	}
	// "before_request" should appear early (it's Exact, which comes first).
	found := false
	for i, kw := range kws {
		if kw == "before_request" {
			if i > 3 {
				t.Errorf("expected 'before_request' in first few keywords, found at index %d", i)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'before_request' in keywords, got %v", kws)
	}
}
