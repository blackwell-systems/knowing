package store

import (
	"context"
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
	if err := s.RecordFeedback(ctx, hash, "session-1", true); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if err := s.RecordFeedback(ctx, hash, "session-1", true); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}
	if err := s.RecordFeedback(ctx, hash, "session-2", false); err != nil {
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
	if err := s.RecordFeedback(ctx, hashA, "s1", true); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFeedback(ctx, hashA, "s2", true); err != nil {
		t.Fatal(err)
	}

	// Record feedback for B (mixed).
	if err := s.RecordFeedback(ctx, hashB, "s1", true); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFeedback(ctx, hashB, "s2", false); err != nil {
		t.Fatal(err)
	}

	boosts, err := s.FeedbackBoosts(ctx, []types.Hash{hashA, hashB, hashC})
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

	boosts, err := s.FeedbackBoosts(ctx, nil)
	if err != nil {
		t.Fatalf("FeedbackBoosts(nil): %v", err)
	}
	if boosts != nil {
		t.Errorf("expected nil for empty input, got %v", boosts)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
