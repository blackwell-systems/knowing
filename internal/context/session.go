package context

import (
	"math"
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SessionTracker records which symbols were returned by the context engine
// during the current session. Subsequent queries boost these symbols and their
// graph neighbors, implementing the "session-aware retrieval" pattern where
// repeated interactions surface increasingly relevant context.
//
// Design informed by competitive analysis:
//   - Exponential decay (3-minute half-life for AI sessions, not days)
//   - Capped boost multiplier (max 2.0x, prevents runaway dominance)
//   - Tracks both returned symbols and queried files
//   - Thread-safe for concurrent MCP tool calls
type SessionTracker struct {
	mu      sync.RWMutex
	accesses map[types.Hash][]int64 // symbol hash -> list of access timestamps (unix seconds)
	started  int64                  // session start time

	// halfLifeSeconds controls how fast old accesses decay.
	// AI sessions churn fast: 3 minutes means a symbol accessed 6 minutes ago
	// has 25% of the boost of one accessed just now.
	halfLifeSeconds float64
}

// NewSessionTracker creates a tracker for the current session.
func NewSessionTracker() *SessionTracker {
	return &SessionTracker{
		accesses:        make(map[types.Hash][]int64),
		started:         time.Now().Unix(),
		halfLifeSeconds: 180, // 3 minutes
	}
}

// Record marks a symbol as accessed at the current time.
// Call this for every symbol returned in a context result.
func (st *SessionTracker) Record(hash types.Hash) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().Unix()
	st.accesses[hash] = append(st.accesses[hash], now)
	// Cap history per symbol to avoid unbounded growth.
	if len(st.accesses[hash]) > 20 {
		st.accesses[hash] = st.accesses[hash][len(st.accesses[hash])-20:]
	}
}

// RecordBatch marks multiple symbols as accessed.
func (st *SessionTracker) RecordBatch(hashes []types.Hash) {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now().Unix()
	for _, h := range hashes {
		st.accesses[h] = append(st.accesses[h], now)
		if len(st.accesses[h]) > 20 {
			st.accesses[h] = st.accesses[h][len(st.accesses[h])-20:]
		}
	}
}

// SessionBoosts returns a boost multiplier for each requested hash based on
// how recently and frequently it was accessed this session. Values range from
// 0.0 (never accessed) to maxBoost (frequently/recently accessed).
// The boost decays exponentially from each access timestamp.
func (st *SessionTracker) SessionBoosts(hashes []types.Hash) map[types.Hash]float64 {
	st.mu.RLock()
	defer st.mu.RUnlock()

	now := time.Now().Unix()
	k := math.Ln2 / st.halfLifeSeconds // decay constant
	const maxBoost = 2.0

	result := make(map[types.Hash]float64)
	for _, h := range hashes {
		times, ok := st.accesses[h]
		if !ok || len(times) == 0 {
			continue
		}
		// Sum decayed contributions from each access.
		// More recent accesses contribute more; multiple accesses compound.
		var score float64
		for _, t := range times {
			age := float64(now - t)
			score += math.Exp(-k * age)
		}
		// Normalize: single access at t=0 gives score=1.0.
		// Multiple recent accesses can exceed 1.0 but we cap at maxBoost.
		if score > maxBoost {
			score = maxBoost
		}
		if score > 0.01 { // skip negligible boosts
			result[h] = score
		}
	}
	return result
}

// Count returns the number of unique symbols tracked this session.
func (st *SessionTracker) Count() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return len(st.accesses)
}

// Reset clears all session history.
func (st *SessionTracker) Reset() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.accesses = make(map[types.Hash][]int64)
	st.started = time.Now().Unix()
}
