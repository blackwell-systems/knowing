package context

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// TaskMemory persists which symbols were useful for which tasks, enabling
// the retrieval pipeline to learn from past agent interactions. Over time,
// the system develops per-repo vocabulary: "when a developer asks about X,
// these symbols tend to be what they actually need."
//
// The memory is passive: it records what symbols were returned by
// context_for_task and later accessed in the session (via SessionTracker).
// No explicit user action required.
type TaskMemory struct {
	db *sql.DB
}

// NewTaskMemory creates a task memory backed by the given database.
// The database must have the task_memory table (migration 008).
func NewTaskMemory(db *sql.DB) *TaskMemory {
	return &TaskMemory{db: db}
}

// Record stores a (keywords, symbol) association from a completed task.
// Call this when a symbol returned by context_for_task was later accessed
// by the agent (positive signal) or when explicit feedback is given.
func (tm *TaskMemory) Record(ctx context.Context, keywords string, symbolHash types.Hash, score float64) error {
	_, err := tm.db.ExecContext(ctx,
		`INSERT INTO task_memory (keywords, symbol_hash, score, timestamp) VALUES (?, ?, ?, ?)`,
		keywords, symbolHash[:], score, time.Now().Unix())
	return err
}

// RecordBatch stores multiple associations at once.
func (tm *TaskMemory) RecordBatch(ctx context.Context, keywords string, symbolHashes []types.Hash, score float64) error {
	tx, err := tm.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO task_memory (keywords, symbol_hash, score, timestamp) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, h := range symbolHashes {
		if _, err := stmt.ExecContext(ctx, keywords, h[:], score, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Recall finds symbols that were useful for tasks with similar keywords.
// Uses keyword overlap: the more query keywords match stored task keywords,
// the stronger the signal. Returns a map of symbol hash to boost score.
func (tm *TaskMemory) Recall(ctx context.Context, queryKeywords []string) (map[types.Hash]float64, error) {
	if len(queryKeywords) == 0 {
		return nil, nil
	}

	// Build a LIKE query for each keyword and collect matching memories.
	// Each keyword match on a stored task adds to the symbol's recall score.
	symbolScores := make(map[types.Hash]float64)

	for _, kw := range queryKeywords {
		if len(kw) < 3 {
			continue
		}
		rows, err := tm.db.QueryContext(ctx,
			`SELECT symbol_hash, score, timestamp FROM task_memory
			 WHERE keywords LIKE ?
			 ORDER BY timestamp DESC LIMIT 50`,
			"%"+kw+"%")
		if err != nil {
			continue
		}

		for rows.Next() {
			var sh []byte
			var score float64
			var ts int64
			if err := rows.Scan(&sh, &score, &ts); err != nil {
				break
			}
			if len(sh) != 32 {
				continue
			}
			var h types.Hash
			copy(h[:], sh)

			// Decay: memories older than 7 days get reduced weight.
			age := float64(time.Now().Unix()-ts) / 86400.0 // days
			decay := 1.0
			if age > 7 {
				decay = 7.0 / age // linear decay after 1 week
			}

			symbolScores[h] += score * decay
		}
		rows.Close()
	}

	return symbolScores, nil
}

// Count returns the number of stored task-symbol associations.
func (tm *TaskMemory) Count(ctx context.Context) int {
	var count int
	tm.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_memory`).Scan(&count) //nolint:errcheck
	return count
}

// NormalizeKeywords extracts and normalizes keywords from a task description
// for storage and matching. Reuses the existing keyword extraction logic
// but returns a space-joined string suitable for LIKE matching.
func NormalizeKeywords(taskDescription string) string {
	kws := extractKeywords(taskDescription)
	// Keep only the most specific keywords (longer first, cap at 10).
	sort.Slice(kws, func(i, j int) bool {
		return len(kws[i]) > len(kws[j])
	})
	if len(kws) > 10 {
		kws = kws[:10]
	}
	return strings.Join(kws, " ")
}
