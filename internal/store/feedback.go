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
// If cluster is not EmptyHash, it scopes the feedback to a keyword cluster so that
// noise demotion for "checkout" queries doesn't affect "order" queries.
func (s *SQLiteStore) RecordFeedback(ctx context.Context, symbolHash types.Hash, sessionID string, useful bool, neighborhoodRoot types.Hash, cluster types.Hash) error {
	usefulInt := 0
	if useful {
		usefulInt = 1
	}
	var rootBytes []byte
	if neighborhoodRoot != types.EmptyHash {
		rootBytes = neighborhoodRoot[:]
	}
	var clusterBytes []byte
	if cluster != types.EmptyHash {
		clusterBytes = cluster[:]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO feedback (symbol_hash, session_id, useful, timestamp, neighborhood_root, keyword_cluster) VALUES (?, ?, ?, ?, ?, ?)`,
		symbolHash[:], sessionID, usefulInt, time.Now().Unix(), rootBytes, clusterBytes,
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
//
// cluster scopes feedback to a keyword cluster. When not EmptyHash, only feedback
// entries matching that cluster are counted, preventing cross-task interference
// (noise for "checkout" queries doesn't demote symbols needed for "order" queries).
func (s *SQLiteStore) FeedbackBoosts(ctx context.Context, hashes []types.Hash, neighborhoodRoots map[types.Hash]types.Hash, cluster ...types.Hash) (map[types.Hash]float64, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	// Extract optional cluster filter.
	var clusterBytes []byte
	if len(cluster) > 0 && cluster[0] != types.EmptyHash {
		clusterBytes = cluster[0][:]
	}

	result := make(map[types.Hash]float64)

	// Build the WHERE clause based on what filters are active.
	if len(neighborhoodRoots) == 0 {
		query := `SELECT COALESCE(SUM(useful), 0), COUNT(*) FROM feedback WHERE symbol_hash = ?`
		args := []any{}
		if clusterBytes != nil {
			query += ` AND keyword_cluster = ?`
		}
		stmt, err := s.db.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		defer stmt.Close()

		for _, h := range hashes {
			args = args[:0]
			args = append(args, h[:])
			if clusterBytes != nil {
				args = append(args, clusterBytes)
			}
			var usefulCount, total int
			if err := stmt.QueryRowContext(ctx, args...).Scan(&usefulCount, &total); err != nil {
				return nil, err
			}
			if total > 0 {
				result[h] = float64(usefulCount) / float64(total)
			}
		}
		return result, nil
	}

	// Merkleized expiration + optional cluster filter.
	query := `SELECT COALESCE(SUM(useful), 0), COUNT(*)
		 FROM feedback
		 WHERE symbol_hash = ? AND (neighborhood_root IS NULL OR neighborhood_root = ?)`
	if clusterBytes != nil {
		query += ` AND keyword_cluster = ?`
	}
	stmt, err := s.db.PrepareContext(ctx, query)
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

		args := []any{h[:], rootBytes}
		if clusterBytes != nil {
			args = append(args, clusterBytes)
		}

		var usefulCount, total int
		if err := stmt.QueryRowContext(ctx, args...).Scan(&usefulCount, &total); err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if total > 0 {
			result[h] = float64(usefulCount) / float64(total)
		}
	}

	return result, nil
}
