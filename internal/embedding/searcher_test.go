package embedding

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// --- Serialization round-trip tests ---

func TestFloat32sBytesRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   []float32
	}{
		{"empty", []float32{}},
		{"single", []float32{1.0}},
		{"multiple", []float32{-1.5, 0, 3.14, 100.0}},
		{"negative_zero", []float32{float32(math.Copysign(0, -1))}},
		{"positive_inf", []float32{float32(math.Inf(1))}},
		{"negative_inf", []float32{float32(math.Inf(-1))}},
		{"nan", []float32{float32(math.NaN())}},
		{"subnormal", []float32{math.SmallestNonzeroFloat32}},
		{"max_float32", []float32{math.MaxFloat32}},
		{"mixed_special", []float32{0, float32(math.Inf(1)), float32(math.NaN()), -1.0, float32(math.Copysign(0, -1))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := float32sToBytes(tt.in)
			if len(b) != len(tt.in)*4 {
				t.Fatalf("byte length: got %d, want %d", len(b), len(tt.in)*4)
			}
			out := bytesToFloat32s(b)
			if len(out) != len(tt.in) {
				t.Fatalf("round-trip length: got %d, want %d", len(out), len(tt.in))
			}
			for i := range tt.in {
				if math.IsNaN(float64(tt.in[i])) {
					if !math.IsNaN(float64(out[i])) {
						t.Errorf("[%d] expected NaN, got %v", i, out[i])
					}
				} else if tt.in[i] != out[i] {
					t.Errorf("[%d] got %v, want %v", i, out[i], tt.in[i])
				}
			}
		})
	}
}

func TestBytesToFloat32sTruncates(t *testing.T) {
	// 5 bytes should produce 1 float (truncates trailing byte).
	b := float32sToBytes([]float32{42.0})
	b = append(b, 0xFF) // extra byte
	out := bytesToFloat32s(b)
	if len(out) != 1 {
		t.Fatalf("expected 1 float, got %d", len(out))
	}
	if out[0] != 42.0 {
		t.Errorf("got %v, want 42.0", out[0])
	}
}

func TestBytesToFloat32sNilEmpty(t *testing.T) {
	out := bytesToFloat32s(nil)
	if len(out) != 0 {
		t.Fatalf("nil input: expected empty, got %d", len(out))
	}
	out = bytesToFloat32s([]byte{})
	if len(out) != 0 {
		t.Fatalf("empty input: expected empty, got %d", len(out))
	}
}

// --- Mock store ---

type mockStore struct {
	stored    map[string][]byte // key = model+hash hex
	getResult map[types.Hash][]byte
	putCalls  int
	getCalls  int
	getErr    error
	putErr    error
}

func newMockStore() *mockStore {
	return &mockStore{
		stored:    make(map[string][]byte),
		getResult: make(map[types.Hash][]byte),
	}
}

func (m *mockStore) BatchPutEmbeddings(_ context.Context, model string, hashes []types.Hash, vectors [][]byte) error {
	m.putCalls++
	if m.putErr != nil {
		return m.putErr
	}
	for i, h := range hashes {
		key := model + fmt.Sprintf("%x", h)
		m.stored[key] = vectors[i]
	}
	return nil
}

func (m *mockStore) GetEmbeddings(_ context.Context, _ string, hashes []types.Hash) (map[types.Hash][]byte, error) {
	m.getCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	result := make(map[types.Hash][]byte)
	for _, h := range hashes {
		if v, ok := m.getResult[h]; ok {
			result[h] = v
		}
	}
	return result, nil
}

// --- Mock embedder via a Searcher with fake internals ---
// We can't easily construct an Embedder without ONNX, so we test the pieces
// that don't need inference directly (serialization, store interactions).
// For ReRankByHashes and IndexBatch, we need the real embedder.

// fakeEmbedder creates a Searcher that uses a fake embed function.
// It replaces the embedder with one that returns deterministic vectors.
type fakeSearcher struct {
	Searcher
	embedFn    func(ctx context.Context, text string) ([]float32, error)
	embedBatch func(ctx context.Context, texts []string) ([][]float32, error)
}

// We'll test ReRankByHashes by creating a minimal Searcher with a mock.
// Since Searcher.embedder is private and Embedder needs ONNX, we use a
// different approach: create a thin wrapper that intercepts calls.

// testableSearcher wraps the pieces of Searcher we can test without ONNX.
// For methods that call s.embedder.Embed/EmbedBatch, we skip those in
// unit tests and gate behind model availability.

func makeHash(b byte) types.Hash {
	var h types.Hash
	h[0] = b
	return h
}

func makeVec(vals ...float32) []float32 {
	return vals
}

// --- Pure logic tests (no model needed) ---

func TestCosine(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1.0},
		{"zero_a", []float32{0, 0}, []float32{1, 1}, 0.0},
		{"zero_b", []float32{1, 1}, []float32{0, 0}, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosine(tt.a, tt.b)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("cosine = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitSymbolToWords(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"TransitiveCallers", "Transitive Callers"},
		{"blast_radius", "blast radius"},
		{"ContextEngine.ForTask", "Context Engine For Task"},
		{"HTTPServer", "HTTP Server"},
		{"a", "a"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := splitSymbolToWords(tt.in)
			if got != tt.want {
				t.Errorf("splitSymbolToWords(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSetStore(t *testing.T) {
	// Verify SetStore attaches and can be cleared.
	s := &Searcher{}
	if s.store != nil {
		t.Fatal("store should be nil initially")
	}
	ms := newMockStore()
	s.SetStore(ms)
	if s.store == nil {
		t.Fatal("store should be set after SetStore")
	}
	s.SetStore(nil)
	if s.store != nil {
		t.Fatal("store should be nil after SetStore(nil)")
	}
}

// --- Tests that require the ONNX model ---

func skipIfNoModel(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: requires ONNX model (use -short to skip)")
	}
	// Try to create an embedder; skip if model not available.
	e, err := New()
	if err != nil {
		t.Skipf("skipping: embedder unavailable: %v", err)
	}
	e.Close()
}

func newTestSearcher(t *testing.T) *Searcher {
	t.Helper()
	e, err := New()
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return NewSearcher(e)
}

func TestReRankByHashes_AllHits(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	ms := newMockStore()
	s.SetStore(ms)

	// Pre-populate the mock store with vectors for 3 hashes.
	// We need real vectors, so embed some texts first.
	texts := []string{"database query optimization", "network socket connection", "user interface rendering"}
	vecs, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	hashes := []types.Hash{makeHash(1), makeHash(2), makeHash(3)}
	for i, h := range hashes {
		ms.getResult[h] = float32sToBytes(vecs[i])
	}

	scores, err := s.ReRankByHashes(ctx, "SQL database query", hashes, texts)
	if err != nil {
		t.Fatalf("ReRankByHashes: %v", err)
	}
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(scores))
	}
	// All scores should be non-zero (vectors came from real embeddings).
	for i, sc := range scores {
		if sc == 0 {
			t.Errorf("score[%d] = 0, expected non-zero", i)
		}
	}
	// The "database query optimization" text should score highest for "SQL database query".
	if scores[0] < scores[1] || scores[0] < scores[2] {
		t.Errorf("expected scores[0] (database) to be highest: %v", scores)
	}
	// Store should have been queried but no puts (all hits).
	if ms.getCalls != 1 {
		t.Errorf("expected 1 GetEmbeddings call, got %d", ms.getCalls)
	}
	if ms.putCalls != 0 {
		t.Errorf("expected 0 BatchPutEmbeddings calls, got %d", ms.putCalls)
	}
}

func TestReRankByHashes_AllMisses(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	ms := newMockStore()
	s.SetStore(ms)

	hashes := []types.Hash{makeHash(10), makeHash(20), makeHash(30)}
	texts := []string{"database query optimization", "network socket connection", "user interface rendering"}

	// Store returns empty (all misses).
	scores, err := s.ReRankByHashes(ctx, "SQL database query", hashes, texts)
	if err != nil {
		t.Fatalf("ReRankByHashes: %v", err)
	}
	if len(scores) != 3 {
		t.Fatalf("expected 3 scores, got %d", len(scores))
	}
	for i, sc := range scores {
		if sc == 0 {
			t.Errorf("score[%d] = 0, expected non-zero", i)
		}
	}
	// Should have queried store AND persisted the newly computed vectors.
	if ms.getCalls != 1 {
		t.Errorf("expected 1 GetEmbeddings call, got %d", ms.getCalls)
	}
	if ms.putCalls != 1 {
		t.Errorf("expected 1 BatchPutEmbeddings call (persist misses), got %d", ms.putCalls)
	}
	// Verify all 3 vectors were persisted.
	if len(ms.stored) != 3 {
		t.Errorf("expected 3 persisted vectors, got %d", len(ms.stored))
	}
}

func TestReRankByHashes_PartialHits(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	ms := newMockStore()
	s.SetStore(ms)

	// Embed texts to get real vectors, cache only the first one.
	texts := []string{"database query optimization", "network socket connection"}
	vecs, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	h1, h2 := makeHash(1), makeHash(2)
	ms.getResult[h1] = float32sToBytes(vecs[0]) // hit
	// h2 is a miss

	scores, err := s.ReRankByHashes(ctx, "SQL database", []types.Hash{h1, h2}, texts)
	if err != nil {
		t.Fatalf("ReRankByHashes: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	for i, sc := range scores {
		if sc == 0 {
			t.Errorf("score[%d] = 0, expected non-zero", i)
		}
	}
	// Should persist only the 1 miss.
	if ms.putCalls != 1 {
		t.Errorf("expected 1 BatchPutEmbeddings call, got %d", ms.putCalls)
	}
}

func TestReRankByHashes_NilStore(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	// No store set: all candidates go through fallback embedding.

	hashes := []types.Hash{makeHash(1), makeHash(2)}
	texts := []string{"database query optimization", "network socket connection"}

	scores, err := s.ReRankByHashes(ctx, "SQL database query", hashes, texts)
	if err != nil {
		t.Fatalf("ReRankByHashes: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	for i, sc := range scores {
		if sc == 0 {
			t.Errorf("score[%d] = 0, expected non-zero", i)
		}
	}
}

func TestReRankByHashes_Empty(t *testing.T) {
	// Empty hashes should return nil, no model needed.
	s := &Searcher{}
	scores, err := s.ReRankByHashes(context.Background(), "query", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scores != nil {
		t.Errorf("expected nil, got %v", scores)
	}
}

func TestReRankByHashes_StoreGetError(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	ms := newMockStore()
	ms.getErr = fmt.Errorf("disk error")
	s.SetStore(ms)

	hashes := []types.Hash{makeHash(1)}
	texts := []string{"some text"}

	// Should degrade gracefully: treat all as misses.
	scores, err := s.ReRankByHashes(ctx, "query", hashes, texts)
	if err != nil {
		t.Fatalf("ReRankByHashes should not fail on store read error: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("expected 1 score, got %d", len(scores))
	}
}

func TestIndexBatch_PersistsToStore(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	ms := newMockStore()
	s.SetStore(ms)

	nodes := []types.Node{
		{Kind: "function", QualifiedName: "pkg.Foo", NodeHash: makeHash(1)},
		{Kind: "type", QualifiedName: "pkg.Bar", NodeHash: makeHash(2)},
	}
	paths := []string{"foo.go", "bar.go"}

	err := s.IndexBatch(ctx, nodes, paths)
	if err != nil {
		t.Fatalf("IndexBatch: %v", err)
	}

	// Should have persisted 2 vectors.
	if ms.putCalls != 1 {
		t.Errorf("expected 1 BatchPutEmbeddings call, got %d", ms.putCalls)
	}
	if len(ms.stored) != 2 {
		t.Errorf("expected 2 stored vectors, got %d", len(ms.stored))
	}

	// Vectors in HNSW index should also be present.
	if s.Count() != 2 {
		t.Errorf("expected 2 indexed vectors, got %d", s.Count())
	}
}

func TestIndexBatch_NoStore(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	// No store set: should not panic.

	nodes := []types.Node{
		{Kind: "function", QualifiedName: "pkg.Baz", NodeHash: makeHash(3)},
	}
	paths := []string{"baz.go"}

	err := s.IndexBatch(ctx, nodes, paths)
	if err != nil {
		t.Fatalf("IndexBatch without store: %v", err)
	}
	if s.Count() != 1 {
		t.Errorf("expected 1 indexed vector, got %d", s.Count())
	}
}

func TestIndexBatch_Empty(t *testing.T) {
	// Empty batch should be a no-op, no model needed.
	s := &Searcher{}
	err := s.IndexBatch(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIndexBatch_StorePutError(t *testing.T) {
	skipIfNoModel(t)
	ctx := context.Background()
	s := newTestSearcher(t)
	ms := newMockStore()
	ms.putErr = fmt.Errorf("write error")
	s.SetStore(ms)

	nodes := []types.Node{
		{Kind: "function", QualifiedName: "pkg.Fail", NodeHash: makeHash(99)},
	}

	err := s.IndexBatch(ctx, nodes, []string{"fail.go"})
	if err == nil {
		t.Fatal("expected error from store put failure")
	}
}
