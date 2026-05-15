package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer/goextractor"
	"github.com/blackwell-systems/knowing/internal/types"
)

// mockExtractor is a test double for types.Extractor.
type mockExtractor struct {
	name      string
	canHandle func(string) bool
	extract   func(context.Context, types.ExtractOptions) (*types.ExtractResult, error)
}

func (m *mockExtractor) Name() string { return m.name }
func (m *mockExtractor) CanHandle(path string) bool {
	if m.canHandle != nil {
		return m.canHandle(path)
	}
	return false
}
func (m *mockExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	if m.extract != nil {
		return m.extract(ctx, opts)
	}
	return &types.ExtractResult{}, nil
}

// mockStore is a minimal test double for types.GraphStore.
type mockStore struct {
	nodes map[types.Hash]types.Node
	edges map[types.Hash]types.Edge
	files map[string]types.File
	repos map[types.Hash]types.Repo
}

func newMockStore() *mockStore {
	return &mockStore{
		nodes: make(map[types.Hash]types.Node),
		edges: make(map[types.Hash]types.Edge),
		files: make(map[string]types.File),
		repos: make(map[types.Hash]types.Repo),
	}
}

func (s *mockStore) PutNode(_ context.Context, n types.Node) error {
	s.nodes[n.NodeHash] = n
	return nil
}
func (s *mockStore) PutEdge(_ context.Context, e types.Edge) error {
	s.edges[e.EdgeHash] = e
	return nil
}
func (s *mockStore) PutFile(_ context.Context, f types.File) error {
	s.files[f.Path] = f
	return nil
}
func (s *mockStore) PutRepo(_ context.Context, r types.Repo) error {
	s.repos[r.RepoHash] = r
	return nil
}
func (s *mockStore) RecordEdgeEvent(_ context.Context, _ types.EdgeEvent) error { return nil }
func (s *mockStore) CreateSnapshot(_ context.Context, _ types.Snapshot) error   { return nil }
func (s *mockStore) GetNode(_ context.Context, h types.Hash) (*types.Node, error) {
	n, ok := s.nodes[h]
	if !ok {
		return nil, nil
	}
	return &n, nil
}
func (s *mockStore) GetEdge(_ context.Context, h types.Hash) (*types.Edge, error) {
	e, ok := s.edges[h]
	if !ok {
		return nil, nil
	}
	return &e, nil
}
func (s *mockStore) GetSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) {
	return nil, nil
}
func (s *mockStore) GetRepo(_ context.Context, h types.Hash) (*types.Repo, error) {
	r, ok := s.repos[h]
	if !ok {
		return nil, nil
	}
	return &r, nil
}
func (s *mockStore) NodesByName(_ context.Context, prefix string) ([]types.Node, error) {
	var result []types.Node
	for _, n := range s.nodes {
		if prefix == "" || n.QualifiedName == prefix {
			result = append(result, n)
		}
	}
	return result, nil
}
func (s *mockStore) EdgesFrom(_ context.Context, _ types.Hash, _ string) ([]types.Edge, error) {
	return nil, nil
}
func (s *mockStore) EdgesTo(_ context.Context, _ types.Hash, _ string) ([]types.Edge, error) {
	return nil, nil
}
func (s *mockStore) TransitiveCallers(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) {
	return nil, nil
}
func (s *mockStore) TransitiveCallees(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) {
	return nil, nil
}
func (s *mockStore) BlastRadius(_ context.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) {
	return nil, nil
}
func (s *mockStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return nil, nil
}
func (s *mockStore) StaleEdges(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}
func (s *mockStore) LatestSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) {
	return nil, nil
}
func (s *mockStore) FilesByRepo(_ context.Context, _ types.Hash) ([]types.File, error) {
	var result []types.File
	for _, f := range s.files {
		result = append(result, f)
	}
	return result, nil
}
func (s *mockStore) FileByPath(_ context.Context, _ types.Hash, path string) (*types.File, error) {
	f, ok := s.files[path]
	if !ok {
		return nil, nil
	}
	return &f, nil
}
func (s *mockStore) DanglingEdges(_ context.Context) ([]types.Edge, error) {
	// Return edges whose target hash does not match any node.
	var dangling []types.Edge
	for _, e := range s.edges {
		if _, exists := s.nodes[e.TargetHash]; !exists {
			dangling = append(dangling, e)
		}
	}
	return dangling, nil
}
func (s *mockStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	var result []types.Repo
	for _, r := range s.repos {
		result = append(result, r)
	}
	return result, nil
}
func (s *mockStore) NodesByQualifiedName(_ context.Context, qualifiedName string) ([]types.Node, error) {
	var result []types.Node
	for _, n := range s.nodes {
		if n.QualifiedName == qualifiedName {
			result = append(result, n)
		}
	}
	return result, nil
}
func (s *mockStore) DeleteEdge(_ context.Context, h types.Hash) error {
	delete(s.edges, h)
	return nil
}
func (s *mockStore) Close() error                                                        { return nil }

// mockSnapshotComputer is a test double for SnapshotComputer.
type mockSnapshotComputer struct {
	snap *types.Snapshot
}

func (m *mockSnapshotComputer) ComputeSnapshot(_ context.Context, repoHash types.Hash, commitHash string) (*types.Snapshot, error) {
	if m.snap != nil {
		return m.snap, nil
	}
	return &types.Snapshot{
		RepoHash:   repoHash,
		CommitHash: commitHash,
	}, nil
}

func TestExtractorRegistry_Register_FindsExtractor(t *testing.T) {
	reg := NewExtractorRegistry()
	ext := &mockExtractor{
		name:      "go",
		canHandle: func(p string) bool { return p == "main.go" },
	}
	reg.Register(ext)

	found := reg.FindExtractor("main.go")
	if found == nil {
		t.Fatal("expected to find extractor for main.go")
	}
	if found.Name() != "go" {
		t.Fatalf("expected extractor name 'go', got %q", found.Name())
	}
}

func TestExtractorRegistry_NoMatch_ReturnsNil(t *testing.T) {
	reg := NewExtractorRegistry()
	ext := &mockExtractor{
		name:      "go",
		canHandle: func(p string) bool { return p == "main.go" },
	}
	reg.Register(ext)

	found := reg.FindExtractor("main.py")
	if found != nil {
		t.Fatal("expected nil for non-matching path")
	}
}

func TestIndexer_IndexFile_StoresResults(t *testing.T) {
	store := newMockStore()
	snap := &mockSnapshotComputer{}
	idx := NewIndexer(store, snap)

	testNode := types.Node{
		NodeHash:      types.NewHash([]byte("test-node")),
		QualifiedName: "test://pkg.Func",
		Kind:          "function",
	}
	testEdge := types.Edge{
		EdgeHash:   types.NewHash([]byte("test-edge")),
		SourceHash: testNode.NodeHash,
		TargetHash: types.NewHash([]byte("target")),
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}

	idx.Register(&mockExtractor{
		name:      "test",
		canHandle: func(p string) bool { return true },
		extract: func(_ context.Context, _ types.ExtractOptions) (*types.ExtractResult, error) {
			return &types.ExtractResult{
				Nodes: []types.Node{testNode},
				Edges: []types.Edge{testEdge},
			}, nil
		},
	})

	ctx := context.Background()
	opts := types.ExtractOptions{
		RepoURL:  "test://repo",
		FilePath: "main.go",
		FileHash: types.NewHash([]byte("file-content")),
	}

	result, err := idx.IndexFile(ctx, opts)
	if err != nil {
		t.Fatalf("IndexFile failed: %v", err)
	}

	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(result.Edges))
	}

	// Verify stored in mock store.
	if len(store.nodes) != 1 {
		t.Fatalf("expected 1 stored node, got %d", len(store.nodes))
	}
	if len(store.edges) != 1 {
		t.Fatalf("expected 1 stored edge, got %d", len(store.edges))
	}
	if len(store.files) != 1 {
		t.Fatalf("expected 1 stored file, got %d", len(store.files))
	}
}

func TestIndexer_IndexRepo_SkipsUnchanged(t *testing.T) {
	store := newMockStore()
	snap := &mockSnapshotComputer{}
	idx := NewIndexer(store, snap)

	extractCount := 0
	idx.Register(&mockExtractor{
		name:      "test",
		canHandle: func(p string) bool { return p == "main.go" || p == "other.go" },
		extract: func(_ context.Context, _ types.ExtractOptions) (*types.ExtractResult, error) {
			extractCount++
			return &types.ExtractResult{}, nil
		},
	})

	// Create a temp directory with test files.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.go"), []byte("package main\nvar x int\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// First indexing: both files should be extracted.
	_, err := idx.IndexRepo(ctx, "test://repo", dir, "commit1")
	if err != nil {
		t.Fatalf("first IndexRepo failed: %v", err)
	}
	if extractCount != 2 {
		t.Fatalf("expected 2 extractions on first run, got %d", extractCount)
	}

	// Second indexing with same content: should skip both files.
	extractCount = 0
	_, err = idx.IndexRepo(ctx, "test://repo", dir, "commit2")
	if err != nil {
		t.Fatalf("second IndexRepo failed: %v", err)
	}
	if extractCount != 0 {
		t.Fatalf("expected 0 extractions on unchanged content, got %d", extractCount)
	}
}

func TestIndexRepo_MultiFileModule(t *testing.T) {
	// Create a temporary Go module with two files in different packages.
	dir := t.TempDir()

	modContent := "module testmulti\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Package "greet" with a Greet function.
	greetDir := filepath.Join(dir, "greet")
	if err := os.MkdirAll(greetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	greetSrc := `package greet

func Greet() string {
	return "hello"
}
`
	if err := os.WriteFile(filepath.Join(greetDir, "greet.go"), []byte(greetSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Package "main" that imports and calls greet.Greet.
	mainSrc := `package main

import "testmulti/greet"

func main() {
	greet.Greet()
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	snapComp := &mockSnapshotComputer{}
	idx := NewIndexer(store, snapComp)
	idx.Register(goextractor.NewGoExtractor())

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, "test://multi", dir, "abc123")
	if err != nil {
		t.Fatalf("IndexRepo failed: %v", err)
	}

	// Verify snapshot was returned.
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// We should have files for both greet/greet.go and main.go.
	if len(store.files) < 2 {
		t.Errorf("expected at least 2 files stored, got %d", len(store.files))
	}

	// We should have nodes from both packages.
	if len(store.nodes) < 2 {
		t.Errorf("expected at least 2 nodes (Greet func + main func), got %d", len(store.nodes))
	}

	// Verify there is a cross-package call edge: main calls greet.Greet.
	hasCallEdge := false
	for _, e := range store.edges {
		if e.EdgeType == "calls" {
			hasCallEdge = true
			break
		}
	}
	if !hasCallEdge {
		t.Error("expected at least one cross-package 'calls' edge (main -> greet.Greet)")
	}

	// Verify there are edges in general (calls, imports, references).
	if len(store.edges) == 0 {
		t.Error("expected at least some edges stored")
	}
}

func TestIndexRepoResolvesEdges(t *testing.T) {
	// This test verifies that IndexRepo automatically resolves dangling
	// cross-repo edges via the resolver. We set up two repos: repoB has
	// a node pre-loaded in the store, and repoA calls repoB's function.
	// After IndexRepo on repoA, the resolver should retarget any dangling
	// edges that used the wrong repo URL.

	tmpDir := t.TempDir()

	// Set up repoB as a Go module on disk (needed for go/packages).
	repoBDir := filepath.Join(tmpDir, "repoB")
	repoBPkgDir := filepath.Join(repoBDir, "pkg")
	if err := os.MkdirAll(repoBPkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoBDir, "go.mod"),
		[]byte("module github.com/test/repoB\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoBPkgDir, "lib.go"),
		[]byte("package pkg\n\nfunc DoThing() string { return \"done\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set up repoA that imports repoB.
	repoADir := filepath.Join(tmpDir, "repoA")
	if err := os.MkdirAll(repoADir, 0o755); err != nil {
		t.Fatal(err)
	}
	repoAMod := fmt.Sprintf(`module github.com/test/repoA

go 1.23

require github.com/test/repoB v0.0.0

replace github.com/test/repoB => %s
`, repoBDir)
	if err := os.WriteFile(filepath.Join(repoADir, "go.mod"), []byte(repoAMod), 0o644); err != nil {
		t.Fatal(err)
	}
	repoAMain := `package main

import "github.com/test/repoB/pkg"

func main() {
	pkg.DoThing()
}
`
	if err := os.WriteFile(filepath.Join(repoADir, "main.go"), []byte(repoAMain), 0o644); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	snapComp := &mockSnapshotComputer{}

	// Pre-load repoB's node and repo record in the store so the resolver
	// can find it when retargeting.
	repoBRepoHash := types.NewHash([]byte("github.com/test/repoB"))
	store.repos[repoBRepoHash] = types.Repo{
		RepoHash:    repoBRepoHash,
		RepoURL:     "github.com/test/repoB",
		LastCommit:  "bbb111",
		LastIndexed: 1,
	}
	repoBNodeHash := types.ComputeNodeHash(
		"github.com/test/repoB",
		"github.com/test/repoB/pkg",
		types.EmptyHash,
		"DoThing",
		"function",
	)
	store.nodes[repoBNodeHash] = types.Node{
		NodeHash:      repoBNodeHash,
		QualifiedName: "github.com/test/repoB://github.com/test/repoB/pkg.DoThing",
		Kind:          "function",
	}

	idx := NewIndexer(store, snapComp)
	idx.Register(goextractor.NewGoExtractor())

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, "github.com/test/repoA", repoADir, "aaa111")
	if err != nil {
		t.Fatalf("IndexRepo failed: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}

	// The extractor should have already used the correct repoB URL for
	// the call edge target (thanks to the extractor fix). Verify no
	// dangling edges remain.
	dangling, err := store.DanglingEdges(ctx)
	if err != nil {
		t.Fatalf("DanglingEdges failed: %v", err)
	}

	// Filter for call edges only (imports to repoB package node might be dangling
	// if we didn't pre-load a package node).
	var danglingCalls []types.Edge
	for _, e := range dangling {
		if e.EdgeType == "calls" {
			danglingCalls = append(danglingCalls, e)
		}
	}
	if len(danglingCalls) > 0 {
		for _, e := range danglingCalls {
			t.Logf("dangling call edge: source=%s target=%s", e.SourceHash, e.TargetHash)
		}
		t.Errorf("expected no dangling call edges after IndexRepo + resolve, got %d", len(danglingCalls))
	}
}

func TestIndexer_ConcurrencyField(t *testing.T) {
	store := newMockStore()
	snap := &mockSnapshotComputer{}
	idx := NewIndexer(store, snap)

	// Default Concurrency should be 0 (meaning use runtime.GOMAXPROCS).
	if idx.Concurrency != 0 {
		t.Fatalf("expected default Concurrency == 0, got %d", idx.Concurrency)
	}

	// Verify the field can be set.
	idx.Concurrency = 4
	if idx.Concurrency != 4 {
		t.Fatalf("expected Concurrency == 4 after setting, got %d", idx.Concurrency)
	}
}
