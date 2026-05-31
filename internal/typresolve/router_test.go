package typresolve

import (
	"context"
	"errors"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockLspEnricher records whether Run was called.
type mockLspEnricher struct {
	called bool
	err    error
}

func (m *mockLspEnricher) Run(_ context.Context, _ types.Hash) error {
	m.called = true
	return m.err
}

func TestRouteEnrichment_AllResolvers(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	re.Register(&mockResolver{lang: "go", edges: []types.Edge{{EdgeType: "calls"}}})
	re.Register(&mockResolver{lang: "python", edges: []types.Edge{{EdgeType: "calls"}}})

	lsp := &mockLspEnricher{}
	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "app.py", Language: "python"},
	}

	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, lsp, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lsp.called {
		t.Error("LSP enricher should not be called when all languages have resolvers")
	}
	// Both files should have been resolved (2 edges total).
	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 2 {
		t.Errorf("expected 2 edges written, got %d", got)
	}
}

func TestRouteEnrichment_NoResolvers(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	// No resolvers registered.

	lsp := &mockLspEnricher{}
	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "app.py", Language: "python"},
	}

	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, lsp, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !lsp.called {
		t.Error("LSP enricher should be called when no resolvers are registered")
	}
}

func TestRouteEnrichment_Mixed(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	re.Register(&mockResolver{lang: "go", edges: []types.Edge{{EdgeType: "calls"}}})
	// No python resolver.

	lsp := &mockLspEnricher{}
	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "util.go", Language: "go"},
		{Path: "app.py", Language: "python"},
	}

	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, lsp, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Go files should be resolved (2 files, 1 edge each = 2 edges).
	store.mu.Lock()
	got := len(store.edges)
	store.mu.Unlock()
	if got != 2 {
		t.Errorf("expected 2 edges from go resolver, got %d", got)
	}

	// LSP should be called for python.
	if !lsp.called {
		t.Error("LSP enricher should be called for unresolved languages")
	}
}

func TestRouteEnrichment_NilLspWithFallback(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	// No resolvers registered, so python needs LSP fallback.

	files := []FileResult{
		{Path: "app.py", Language: "python"},
	}

	// nil lspEnricher should not panic.
	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, nil, files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRouteEnrichment_EmptyFiles(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	lsp := &mockLspEnricher{}

	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, lsp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lsp.called {
		t.Error("LSP enricher should not be called for empty file results")
	}
}

func TestRouteEnrichment_ResolverErrorDoesNotPreventLSP(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)
	// Go resolver that fails on init.
	re.Register(&mockResolver{lang: "go", initErr: errors.New("init boom")})

	lsp := &mockLspEnricher{}
	files := []FileResult{
		{Path: "main.go", Language: "go"},
		{Path: "app.py", Language: "python"},
	}

	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, lsp, files)
	// No error expected because resolver errors are logged, not propagated
	// from Run (Run returns nil per implementation).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// LSP should still run for python.
	if !lsp.called {
		t.Error("LSP enricher should still run despite resolver errors")
	}
}

func TestRouteEnrichment_LspError(t *testing.T) {
	store := &mockStore{}
	re := NewResolverEnricher(store, 2)

	lsp := &mockLspEnricher{err: errors.New("lsp boom")}
	files := []FileResult{
		{Path: "app.py", Language: "python"},
	}

	err := RouteEnrichment(context.Background(), store, "/repo", types.Hash{}, re, lsp, files)
	if err == nil {
		t.Fatal("expected error from LSP enricher")
	}
	if err.Error() != "lsp boom" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRouteStats(t *testing.T) {
	re := NewResolverEnricher(&mockStore{}, 2)
	re.Register(&mockResolver{lang: "go"})

	files := []FileResult{
		{Path: "a.go", Language: "go"},
		{Path: "b.go", Language: "go"},
		{Path: "c.py", Language: "python"},
		{Path: "d.rs", Language: "rust"},
		{Path: "e.py", Language: "python"},
		{Path: "", Language: ""}, // empty language should be skipped
	}

	stats := RouteStats(re, files)

	if len(stats) != 3 {
		t.Fatalf("expected 3 language stats, got %d", len(stats))
	}

	// Check go.
	if stats[0].Language != "go" || stats[0].FileCount != 2 || !stats[0].UseResolver {
		t.Errorf("go stats wrong: %+v", stats[0])
	}
	// Check python.
	if stats[1].Language != "python" || stats[1].FileCount != 2 || stats[1].UseResolver {
		t.Errorf("python stats wrong: %+v", stats[1])
	}
	// Check rust.
	if stats[2].Language != "rust" || stats[2].FileCount != 1 || stats[2].UseResolver {
		t.Errorf("rust stats wrong: %+v", stats[2])
	}
}

func TestRouteStats_Empty(t *testing.T) {
	re := NewResolverEnricher(&mockStore{}, 2)
	stats := RouteStats(re, nil)
	if len(stats) != 0 {
		t.Errorf("expected empty stats, got %d", len(stats))
	}
}
