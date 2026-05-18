package context

import (
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestSessionTracker_BasicRecordAndBoost(t *testing.T) {
	st := NewSessionTracker()

	h1 := types.NewHash([]byte("symbol-1"))
	h2 := types.NewHash([]byte("symbol-2"))
	h3 := types.NewHash([]byte("symbol-3"))

	// Record h1 and h2.
	st.Record(h1)
	st.Record(h2)

	// Check boosts.
	boosts := st.SessionBoosts([]types.Hash{h1, h2, h3})
	if boosts[h1] == 0 {
		t.Error("expected boost for h1, got 0")
	}
	if boosts[h2] == 0 {
		t.Error("expected boost for h2, got 0")
	}
	if boosts[h3] != 0 {
		t.Errorf("expected no boost for h3, got %f", boosts[h3])
	}
}

func TestSessionTracker_MultipleAccessesCompound(t *testing.T) {
	st := NewSessionTracker()
	h := types.NewHash([]byte("symbol-multi"))

	// Record multiple times.
	st.Record(h)
	st.Record(h)
	st.Record(h)

	boosts := st.SessionBoosts([]types.Hash{h})
	// Multiple accesses at same time should compound (up to cap).
	if boosts[h] <= 1.0 {
		t.Errorf("expected compounded boost > 1.0, got %f", boosts[h])
	}
	if boosts[h] > 2.0 {
		t.Errorf("expected boost capped at 2.0, got %f", boosts[h])
	}
}

func TestSessionTracker_DecayOverTime(t *testing.T) {
	st := NewSessionTracker()
	// Override half-life to 1 second for fast test.
	st.halfLifeSeconds = 1.0

	h := types.NewHash([]byte("symbol-decay"))
	st.Record(h)

	// Immediate boost should be ~1.0.
	boosts := st.SessionBoosts([]types.Hash{h})
	if boosts[h] < 0.9 {
		t.Errorf("expected immediate boost ~1.0, got %f", boosts[h])
	}

	// Wait for 2 half-lives.
	time.Sleep(2 * time.Second)

	boosts = st.SessionBoosts([]types.Hash{h})
	// After 2 half-lives: 1.0 * 0.25 = 0.25.
	if boosts[h] > 0.4 {
		t.Errorf("expected decayed boost < 0.4, got %f", boosts[h])
	}
}

func TestSessionTracker_Count(t *testing.T) {
	st := NewSessionTracker()
	if st.Count() != 0 {
		t.Errorf("expected 0 count, got %d", st.Count())
	}

	h1 := types.NewHash([]byte("a"))
	h2 := types.NewHash([]byte("b"))
	st.Record(h1)
	st.Record(h2)
	st.Record(h1) // duplicate

	if st.Count() != 2 {
		t.Errorf("expected 2 unique symbols, got %d", st.Count())
	}
}

func TestSessionTracker_Reset(t *testing.T) {
	st := NewSessionTracker()
	h := types.NewHash([]byte("reset-me"))
	st.Record(h)
	st.Reset()

	boosts := st.SessionBoosts([]types.Hash{h})
	if boosts[h] != 0 {
		t.Errorf("expected 0 after reset, got %f", boosts[h])
	}
}
