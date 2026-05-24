package store

import (
	"context"
	"database/sql"
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
// If neighborhoodRoot is not EmptyHash, it is stored to enable merkleized expiration:
// feedback becomes invalid when the symbol's package changes (detected via SubgraphRoot mismatch).
func (s *SQLiteStore) RecordFeedback(ctx context.Context, symbolHash types.Hash, sessionID string, useful bool, neighborhoodRoot types.Hash) error {
	usefulInt := 0
	if useful {
		usefulInt = 1
	}
	var rootBytes []byte
	if neighborhoodRoot != types.EmptyHash {
		rootBytes = neighborhoodRoot[:]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feedback (symbol_hash, session_id, useful, timestamp, neighborhood_root) VALUES (?, ?, ?, ?, ?)`,
		symbolHash[:], sessionID, usefulInt, time.Now().Unix(), rootBytes,
	)
	if err != nil {
		return err
	}
	// Invalidate cached context packs: feedback changes ranking, so any
	// cached pack is potentially stale. Without this, ForTask returns the
	// same cached result regardless of accumulated feedback.
	_, _ = s.db.ExecContext(ctx, `DELETE FROM graph_notes WHERE key = 'context_pack'`)
	return nil
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
//
// neighborhoodRoots maps symbol hash to its current SubgraphRoot. If provided,
// only feedback entries where neighborhood_root matches are counted (merkleized expiration).
// When a symbol's package changes, its old feedback expires automatically.
func (s *SQLiteStore) FeedbackBoosts(ctx context.Context, hashes []types.Hash, neighborhoodRoots map[types.Hash]types.Hash) (map[types.Hash]float64, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	result := make(map[types.Hash]float64)

	// Query each hash individually; for large batches a temp table would be more
	// efficient, but typical context packing involves <100 symbols.

	// If no neighborhood roots provided, use legacy query (no expiration).
	if len(neighborhoodRoots) == 0 {
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

	// Merkleized expiration: filter by neighborhood_root.
	stmt, err := s.db.PrepareContext(ctx,
		`SELECT COALESCE(SUM(useful), 0), COUNT(*)
		 FROM feedback
		 WHERE symbol_hash = ? AND (neighborhood_root IS NULL OR neighborhood_root = ?)`,
	)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	for _, h := range hashes {
		root := neighborhoodRoots[h]
		var rootBytes []byte
		if root != types.EmptyHash {
			rootBytes = root[:]
		}

		var usefulCount, total int
		if err := stmt.QueryRowContext(ctx, h[:], rootBytes).Scan(&usefulCount, &total); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if total > 0 {
			result[h] = float64(usefulCount) / float64(total)
		}
	}

	return result, nil
}
