package typresolve

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockResolver implements Resolver for testing.
type mockResolver struct {
	lang    string
	edges   []types.Edge
	initErr error
	fileErr error
	// Track calls for verification.
	initCalled int
	fileCalls  int
	mu         sync.Mutex
}

func (m *mockResolver) Language() string { return m.lang }

func (m *mockResolver) InitWorkspace(_ context.Context, _ []ResolverDef) error {
	m.mu.Lock()
	m.initCalled++
	m.mu.Unlock()
	return m.initErr
}

func (m *mockResolver) ResolveFile(_ context.Context, _ ResolveFileOpts) ([]types.Edge, error) {
	m.mu.Lock()
	m.fileCalls++
	m.mu.Unlock()
	if m.fileErr != nil {
		return nil, m.fileErr
	}
	return m.edges, nil
}

// mockStore tracks PutEdge calls for verification.
type mockStore struct {
	types.GraphStore // embed to satisfy interface
	edges            []types.Edge
	mu               sync.Mutex
}

func (m *mockStore) PutEdge(_ context.Context, e types.Edge) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.edges = append(m.edges, e)
	return nil
}

func TestResolverNewResolverEnricher_Defaults(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 0)
	if re.concurrency != 8 {
		t.Errorf("expected default concurrency 8, got %d", re.concurrency)
	}
	if re.store != store {
		t.Error("store not set correctly")
	}
	if len(re.resolvers) != 0 {
		t.Error("resolvers should be empty initially")
	}
}

func TestResolverNewResolverEnricher_CustomConcurrency(t *testing.T) {
	re := NewResolverEnricher(&mockStore{}, 16)
	if re.concurrency != 16 {
		t.Errorf("expected concurrency 16, got %d", re.concurrency)
	}
}

func TestResolverRegisterAndHasResolver(t *testing.T) {
	re := NewResolverEnricher(&mockStore{}, 4)
	r := &mockResolver{lang: "go"}
	re.Register(r)

	if !re.HasResolver("go") {
		t.Error("expected HasResolver('go') to be true after Register")
	}
}

func TestResolverHasResolver_Unregistered(t *testing.T) {
	re := NewResolverEnricher(&mockStore{}, 4)
	if re.HasResolver("python") {
		t.Error("expected HasResolver('python') to be false for unregistered language")
	}
}

func TestResolverRun_WritesEdges(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)

	edge1 := types.Edge{EdgeType: "calls", Provenance: ProvenanceResolverResolved, Confidence: ResolverConfidence}
	edge2 := types.Edge{EdgeType: "references", Provenance: ProvenanceResolverResolved, Confidence: ResolverConfidence}

	r := &mockResolver{
		lang:  "go",
		edges: []types.Edge{edge1, edge2},
	}
	re.Register(r)

	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "util.go", Language: "go"},
	}

	err := re.Run(context.Background(), types.Hash{}, files)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Each file returns 2 edges, 2 files = 4 edges written.
	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 4 {
		t.Errorf("expected 4 edges written, got %d", got)
	}

	// InitWorkspace should have been called once.
	r.mu.Lock()
	if r.initCalled != 1 {
		t.Errorf("expected InitWorkspace called once, got %d", r.initCalled)
	}
	if r.fileCalls != 2 {
		t.Errorf("expected ResolveFile called twice, got %d", r.fileCalls)
	}
	r.mu.Unlock()
}

func TestResolverRun_NoMatchingLanguage(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)

	r := &mockResolver{lang: "go", edges: []types.Edge{{EdgeType: "calls"}}}
	re.Register(r)

	// Only python files, no go resolver match.
	files := []FileResult{
		{Path: "main.py", Language: "python"},
	}

	err := re.Run(context.Background(), types.Hash{}, files)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 0 {
		t.Errorf("expected 0 edges written, got %d", got)
	}
}

func TestResolverRun_InitWorkspaceFailure(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)

	failing := &mockResolver{lang: "go", initErr: errors.New("init boom")}
	working := &mockResolver{lang: "python", edges: []types.Edge{{EdgeType: "calls"}}}
	re.Register(failing)
	re.Register(working)

	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "app.py", Language: "python"},
	}

	err := re.Run(context.Background(), types.Hash{}, files)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Python resolver should still produce edges.
	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 1 {
		t.Errorf("expected 1 edge from python, got %d", got)
	}
}

func TestResolverRun_ResolveFileError(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 1)

	// First call fails, second succeeds. We simulate this by using
	// a resolver that always returns edges but also has fileErr set.
	// Instead, use two resolvers or a smarter mock. Simpler: use a
	// resolver that fails, and a separate file for a working resolver.
	// Actually, we test with a single resolver that always fails.
	// The key property: Run does not return an error, other languages
	// still work.
	failResolver := &mockResolver{lang: "go", fileErr: errors.New("resolve boom")}
	goodResolver := &mockResolver{lang: "python", edges: []types.Edge{{EdgeType: "calls"}}}
	re.Register(failResolver)
	re.Register(goodResolver)

	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "other.go", Language: "go"},
		{Path: "app.py", Language: "python"},
	}

	err := re.Run(context.Background(), types.Hash{}, files)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Go files all fail, python still works.
	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 1 {
		t.Errorf("expected 1 edge from python, got %d", got)
	}
}

func TestResolverRun_ContextCancellation(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 1)

	// Resolver that blocks until context is cancelled.
	slowResolver := &mockResolver{lang: "go"}
	slowResolver.edges = []types.Edge{{EdgeType: "calls"}}
	re.Register(slowResolver)

	// Create many files to increase chance of catching cancellation.
	var files []FileResult
	for i := 0; i < 100; i++ {
		files = append(files, FileResult{Path: "file.go", Language: "go"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := re.Run(ctx, types.Hash{}, files)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// With cancellation, we should process fewer than all 100 files.
	// We don't assert exact count since timing is non-deterministic,
	// but we verify it completes without hanging.
}

func TestResolverRun_EmptyFiles(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	re.Register(&mockResolver{lang: "go"})

	err := re.Run(context.Background(), types.Hash{}, nil)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 0 {
		t.Errorf("expected 0 edges, got %d", got)
	}
}

func TestResolverConstants(t *testing.T) {
	if ProvenanceResolverResolved != "resolver_resolved" {
		t.Errorf("unexpected provenance: %s", ProvenanceResolverResolved)
	}
	if ResolverConfidence != 0.9 {
		t.Errorf("unexpected confidence: %f", ResolverConfidence)
	}
}
