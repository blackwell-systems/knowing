package context

import (
	stdctx "context"
	"fmt"
	"math"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockVectorReRanker implements both VectorSearcher and VectorReRanker.
type mockVectorReRanker struct {
	// scores maps query string to a slice of scores to return from ReRankByHashes.
	scores    []float64
	err       error
	callCount int
}

func (m *mockVectorReRanker) EmbedAndSearch(_ stdctx.Context, _ string, _ int) ([]types.Hash, error) {
	return nil, nil
}

func (m *mockVectorReRanker) ReRank(_ stdctx.Context, _ string, _ []string) ([]int, error) {
	return nil, nil
}

func (m *mockVectorReRanker) ReRankScores(_ stdctx.Context, _ string, _ []string) ([]float64, error) {
	return nil, nil
}

func (m *mockVectorReRanker) ReRankByHashes(_ stdctx.Context, _ string, hashes []types.Hash, _ []string) ([]float64, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	// Return scores sliced to the number of hashes requested.
	if len(m.scores) >= len(hashes) {
		return m.scores[:len(hashes)], nil
	}
	return m.scores, nil
}

func makeRankedSymbols(n int) []RankedSymbol {
	ranked := make([]RankedSymbol, n)
	for i := 0; i < n; i++ {
		ranked[i] = RankedSymbol{
			Node: types.Node{
				NodeHash:      types.NewHash([]byte(fmt.Sprintf("node-%d", i))),
				QualifiedName: fmt.Sprintf("pkg.Symbol%d", i),
				Kind:          "function",
			},
			Score: float64(n - i), // descending: n, n-1, ..., 1
		}
	}
	return ranked
}

func TestReRankWithEmbeddings_ReordersByScore(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	// 5 symbols with original scores 5,4,3,2,1.
	// Embedding scores invert the order: highest score for the last candidate.
	reranker := &mockVectorReRanker{
		scores: []float64{0.1, 0.2, 0.3, 0.4, 0.9},
	}

	ranked := makeRankedSymbols(5)

	// Save original weight; use pure embedding for this test.
	origWeight := ReRankOriginalWeight
	ReRankOriginalWeight = 0.0
	defer func() { ReRankOriginalWeight = origWeight }()

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, ranked, "find the symbol")

	if len(result) != 5 {
		t.Fatalf("expected 5 results, got %d", len(result))
	}

	// With pure embedding weight, Symbol4 (score=0.9) should be first.
	if result[0].Node.QualifiedName != "pkg.Symbol4" {
		t.Errorf("expected first result to be pkg.Symbol4, got %s", result[0].Node.QualifiedName)
	}
	// Symbol0 (score=0.1) should be last.
	if result[4].Node.QualifiedName != "pkg.Symbol0" {
		t.Errorf("expected last result to be pkg.Symbol0, got %s", result[4].Node.QualifiedName)
	}
}

func TestReRankWithEmbeddings_EmptyCandidates(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)
	reranker := &mockVectorReRanker{}

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, nil, "some task")

	if len(result) != 0 {
		t.Errorf("expected 0 results for empty candidates, got %d", len(result))
	}
	if reranker.callCount != 0 {
		t.Errorf("expected no calls to ReRankByHashes, got %d", reranker.callCount)
	}
}

func TestReRankWithEmbeddings_FewerThan50(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	n := 10
	// Invert scores: last candidate gets highest embedding score.
	scores := make([]float64, n)
	for i := 0; i < n; i++ {
		scores[i] = float64(i) / float64(n)
	}

	reranker := &mockVectorReRanker{scores: scores}
	ranked := makeRankedSymbols(n)

	origWeight := ReRankOriginalWeight
	ReRankOriginalWeight = 0.0
	defer func() { ReRankOriginalWeight = origWeight }()

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, ranked, "find symbols")

	if len(result) != n {
		t.Fatalf("expected %d results, got %d", n, len(result))
	}

	// All 10 should be re-ranked. Last original symbol (highest embed score) should be first.
	if result[0].Node.QualifiedName != fmt.Sprintf("pkg.Symbol%d", n-1) {
		t.Errorf("expected first result to be pkg.Symbol%d, got %s", n-1, result[0].Node.QualifiedName)
	}
}

func TestReRankWithEmbeddings_MoreThan50(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	n := 70
	// Scores for top 50: give highest score to candidate index 49.
	scores := make([]float64, 50)
	for i := 0; i < 50; i++ {
		scores[i] = float64(i) / 50.0
	}

	reranker := &mockVectorReRanker{scores: scores}
	ranked := makeRankedSymbols(n)

	origWeight := ReRankOriginalWeight
	ReRankOriginalWeight = 0.0
	defer func() { ReRankOriginalWeight = origWeight }()

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, ranked, "find symbols")

	if len(result) != n {
		t.Fatalf("expected %d results, got %d", n, len(result))
	}

	// Top 50 should be re-ranked: index 49 (highest embed score) should be first.
	if result[0].Node.QualifiedName != "pkg.Symbol49" {
		t.Errorf("expected first result to be pkg.Symbol49, got %s", result[0].Node.QualifiedName)
	}

	// Tail (indices 50-69) should be preserved in original order.
	for i := 50; i < n; i++ {
		expected := fmt.Sprintf("pkg.Symbol%d", i)
		if result[i].Node.QualifiedName != expected {
			t.Errorf("tail position %d: expected %s, got %s", i, expected, result[i].Node.QualifiedName)
		}
	}
}

func TestReRankWithEmbeddings_EmptyTask(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)
	reranker := &mockVectorReRanker{scores: []float64{0.5, 0.5, 0.5}}
	ranked := makeRankedSymbols(3)

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, ranked, "")

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	// Should return original order (no re-ranking).
	for i := 0; i < 3; i++ {
		expected := fmt.Sprintf("pkg.Symbol%d", i)
		if result[i].Node.QualifiedName != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, result[i].Node.QualifiedName)
		}
	}
	if reranker.callCount != 0 {
		t.Errorf("expected no calls to ReRankByHashes for empty task, got %d", reranker.callCount)
	}
}

func TestReRankWithEmbeddings_ErrorReturnsOriginal(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)
	reranker := &mockVectorReRanker{err: fmt.Errorf("embedding service unavailable")}
	ranked := makeRankedSymbols(5)

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, ranked, "find symbols")

	if len(result) != 5 {
		t.Fatalf("expected 5 results, got %d", len(result))
	}
	// Should preserve original order on error.
	for i := 0; i < 5; i++ {
		expected := fmt.Sprintf("pkg.Symbol%d", i)
		if result[i].Node.QualifiedName != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, result[i].Node.QualifiedName)
		}
	}
}

func TestReRankWithEmbeddings_BlendedScoring(t *testing.T) {
	store := &mockStore{}
	engine := NewContextEngine(store)

	// 4 symbols with original scores 4,3,2,1 (normalized: 1.0, 0.75, 0.5, 0.25).
	// Embedding scores: 0.0, 0.0, 0.0, 1.0 (strongly prefer Symbol3).
	reranker := &mockVectorReRanker{
		scores: []float64{0.0, 0.0, 0.0, 1.0},
	}
	ranked := makeRankedSymbols(4)

	origWeight := ReRankOriginalWeight
	ReRankOriginalWeight = 0.5
	defer func() { ReRankOriginalWeight = origWeight }()

	result := engine.reRankWithEmbeddings(stdctx.Background(), reranker, ranked, "find symbols")

	if len(result) != 4 {
		t.Fatalf("expected 4 results, got %d", len(result))
	}

	// Blended scores at weight=0.5:
	// Symbol0: 0.5*1.0 + 0.5*0.0 = 0.5
	// Symbol1: 0.5*0.75 + 0.5*0.0 = 0.375
	// Symbol2: 0.5*0.5 + 0.5*0.0 = 0.25
	// Symbol3: 0.5*0.25 + 0.5*1.0 = 0.625
	// Expected order: Symbol3 (0.625), Symbol0 (0.5), Symbol1 (0.375), Symbol2 (0.25)
	expectedOrder := []string{"pkg.Symbol3", "pkg.Symbol0", "pkg.Symbol1", "pkg.Symbol2"}
	for i, expected := range expectedOrder {
		if result[i].Node.QualifiedName != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, result[i].Node.QualifiedName)
		}
	}

	// Verify blending is actually happening by checking that the result differs
	// from both pure-original and pure-embedding orderings.
	// Pure original: Symbol0, Symbol1, Symbol2, Symbol3
	// Pure embedding: Symbol3, Symbol0/1/2 (all 0.0)
	// Blended: Symbol3, Symbol0, Symbol1, Symbol2
	if result[0].Node.QualifiedName == "pkg.Symbol0" {
		t.Error("blending had no effect: result matches pure original order")
	}

	// Sanity: verify scores are finite (no NaN from division).
	for i := range result {
		if math.IsNaN(result[i].Score) {
			t.Errorf("position %d has NaN score", i)
		}
	}
}
