package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestFeedback_RecordAndQuery(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hash := types.NewHash([]byte("test-symbol"))

	// Query with no feedback should return zero stats.
	stats, err := s.QueryFeedback(ctx, hash)
	if err != nil {
		t.Fatalf("QueryFeedback (empty): %v", err)
	}
	if stats.UsefulCount != 0 || stats.NotUsefulCount != 0 || stats.Score != 0 {
		t.Fatalf("expected zero stats, got %+v", stats)
	}

	// Record some feedback.
	if err := s.RecordFeedback(ctx, hash, "session-1", true, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if err := s.RecordFeedback(ctx, hash, "session-1", true, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if err := s.RecordFeedback(ctx, hash, "session-2", false, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}

	// Query should now return correct counts.
	stats, err = s.QueryFeedback(ctx, hash)
	if err != nil {
		t.Fatalf("QueryFeedback: %v", err)
	}
	if stats.UsefulCount != 2 {
		t.Errorf("UsefulCount: got %d, want 2", stats.UsefulCount)
	}
	if stats.NotUsefulCount != 1 {
		t.Errorf("NotUsefulCount: got %d, want 1", stats.NotUsefulCount)
	}
	// Score = 2/3 ~ 0.6667
	expectedScore := 2.0 / 3.0
	if abs(stats.Score-expectedScore) > 0.001 {
		t.Errorf("Score: got %f, want %f", stats.Score, expectedScore)
	}
}

func TestFeedback_Boosts(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hashA := types.NewHash([]byte("symbol-a"))
	hashB := types.NewHash([]byte("symbol-b"))
	hashC := types.NewHash([]byte("symbol-c")) // no feedback

	// Record feedback for A (all useful).
	if err := s.RecordFeedback(ctx, hashA, "s1", true, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFeedback(ctx, hashA, "s2", true, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatal(err)
	}

	// Record feedback for B (mixed).
	if err := s.RecordFeedback(ctx, hashB, "s1", true, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFeedback(ctx, hashB, "s2", false, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatal(err)
	}

	boosts, err := s.FeedbackBoosts(ctx, []types.Hash{hashA, hashB, hashC}, nil)
	if err != nil {
		t.Fatalf("FeedbackBoosts: %v", err)
	}

	// hashA should have score 1.0.
	if v, ok := boosts[hashA]; !ok || abs(v-1.0) > 0.001 {
		t.Errorf("hashA boost: got %f, want 1.0", v)
	}

	// hashB should have score 0.5.
	if v, ok := boosts[hashB]; !ok || abs(v-0.5) > 0.001 {
		t.Errorf("hashB boost: got %f, want 0.5", v)
	}

	// hashC should not be present.
	if _, ok := boosts[hashC]; ok {
		t.Error("hashC should not be in boosts map (no feedback)")
	}
}

func TestFeedback_BoostsEmpty(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	boosts, err := s.FeedbackBoosts(ctx, nil, nil)
	if err != nil {
		t.Fatalf("FeedbackBoosts(nil): %v", err)
	}
	if boosts != nil {
		t.Errorf("expected nil for empty input, got %v", boosts)
	}
}

func TestFeedbackExpiration(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hashA := types.NewHash([]byte("symbol-a"))
	rootV1 := types.NewHash([]byte("neighborhood-v1"))
	rootV2 := types.NewHash([]byte("neighborhood-v2"))

	// Record feedback with neighborhood root v1.
	if err := s.RecordFeedback(ctx, hashA, "s1", true, rootV1, types.EmptyHash); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFeedback(ctx, hashA, "s2", true, rootV1, types.EmptyHash); err != nil {
		t.Fatal(err)
	}

	// Query with v1 root - should see both feedback entries.
	boostsV1, err := s.FeedbackBoosts(ctx, []types.Hash{hashA}, map[types.Hash]types.Hash{hashA: rootV1})
	if err != nil {
		t.Fatalf("FeedbackBoosts(v1): %v", err)
	}
	if v, ok := boostsV1[hashA]; !ok || abs(v-1.0) > 0.001 {
		t.Errorf("with v1 root: got boost %f, want 1.0 (2/2 useful)", v)
	}

	// Query with v2 root - feedback has expired (package changed).
	boostsV2, err := s.FeedbackBoosts(ctx, []types.Hash{hashA}, map[types.Hash]types.Hash{hashA: rootV2})
	if err != nil {
		t.Fatalf("FeedbackBoosts(v2): %v", err)
	}
	if _, ok := boostsV2[hashA]; ok {
		t.Error("with v2 root: expected no boost (feedback expired), but got one")
	}

	// Record new feedback with v2 root.
	if err := s.RecordFeedback(ctx, hashA, "s3", false, rootV2, types.EmptyHash); err != nil {
		t.Fatal(err)
	}

	// Query with v2 root - should only see the new feedback.
	boostsV2New, err := s.FeedbackBoosts(ctx, []types.Hash{hashA}, map[types.Hash]types.Hash{hashA: rootV2})
	if err != nil {
		t.Fatalf("FeedbackBoosts(v2 new): %v", err)
	}
	if v, ok := boostsV2New[hashA]; !ok || abs(v-0.0) > 0.001 {
		t.Errorf("with v2 root after new feedback: got boost %f, want 0.0 (0/1 useful)", v)
	}

	// Query with v1 root - should still see only the old feedback.
	boostsV1Still, err := s.FeedbackBoosts(ctx, []types.Hash{hashA}, map[types.Hash]types.Hash{hashA: rootV1})
	if err != nil {
		t.Fatalf("FeedbackBoosts(v1 still): %v", err)
	}
	if v, ok := boostsV1Still[hashA]; !ok || abs(v-1.0) > 0.001 {
		t.Errorf("with v1 root after v2 feedback: got boost %f, want 1.0 (old feedback still valid for v1)", v)
	}
}

func BenchmarkFeedbackBoosts(b *testing.B) {
	s := tempDB(&testing.T{})
	ctx := context.Background()

	// Create 100 symbols with feedback.
	var hashes []types.Hash
	roots := make(map[types.Hash]types.Hash)
	for i := 0; i < 100; i++ {
		h := types.NewHash([]byte(fmt.Sprintf("symbol-%d", i)))
		root := types.NewHash([]byte(fmt.Sprintf("root-%d", i%10))) // 10 unique roots
		hashes = append(hashes, h)
		roots[h] = root

		// Record feedback with neighborhood root.
		s.RecordFeedback(ctx, h, "session", true, root, types.EmptyHash)
		s.RecordFeedback(ctx, h, "session", true, root, types.EmptyHash)
	}

	b.Run("WithoutExpiration", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := s.FeedbackBoosts(ctx, hashes, nil)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WithExpiration", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := s.FeedbackBoosts(ctx, hashes, roots)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
