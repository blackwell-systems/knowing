package store

import (
	"context"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// FeedbackStats holds aggregate feedback data for a symbol.
type FeedbackStats struct {
	UsefulCount    int     `json:"useful_count"`
	NotUsefulCount int     `json:"not_useful_count"`
	Score          float64 `json:"score"` // useful / (useful + not_useful)
}

// RecordFeedback inserts a feedback record for a symbol in a session.
func (s *SQLiteStore) RecordFeedback(ctx context.Context, symbolHash types.Hash, sessionID string, useful bool) error {
	usefulInt := 0
	if useful {
		usefulInt = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feedback (symbol_hash, session_id, useful, timestamp) VALUES (?, ?, ?, ?)`,
		symbolHash[:], sessionID, usefulInt, time.Now().Unix(),
	)
	return err
}

// QueryFeedback returns aggregate feedback stats for a symbol.
// Returns zero stats (not nil) if no feedback exists.
func (s *SQLiteStore) QueryFeedback(ctx context.Context, symbolHash types.Hash) (*FeedbackStats, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(useful), 0), COUNT(*) FROM feedback WHERE symbol_hash = ?`,
		symbolHash[:],
	)

	var usefulCount, total int
	if err := row.Scan(&usefulCount, &total); err != nil {
		return nil, err
	}

	stats := &FeedbackStats{
		UsefulCount:    usefulCount,
		NotUsefulCount: total - usefulCount,
	}
	if total > 0 {
		stats.Score = float64(usefulCount) / float64(total)
	}
	return stats, nil
}

// FeedbackBoosts returns a map of symbol hash to feedback score (0.0-1.0)
// for all provided hashes that have at least one feedback entry.
// Hashes with no feedback are omitted from the result.
func (s *SQLiteStore) FeedbackBoosts(ctx context.Context, hashes []types.Hash) (map[types.Hash]float64, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	result := make(map[types.Hash]float64)

	// Query each hash individually; for large batches a temp table would be more
	// efficient, but typical context packing involves <100 symbols.
	stmt, err := s.db.PrepareContext(ctx,
		`SELECT COALESCE(SUM(useful), 0), COUNT(*) FROM feedback WHERE symbol_hash = ?`,
	)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	for _, h := range hashes {
		var usefulCount, total int
		if err := stmt.QueryRowContext(ctx, h[:]).Scan(&usefulCount, &total); err != nil {
			return nil, err
		}
		if total > 0 {
			result[h] = float64(usefulCount) / float64(total)
		}
	}

	return result, nil
}
