package store

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

const gcBatchSize = 500

// DeleteNodesNotIn deletes all node rows whose node_hash is not in the provided
// keep set. Uses a temporary table for efficient NOT IN checking with large sets.
// Returns the number of deleted nodes.
func (s *SQLiteStore) DeleteNodesNotIn(ctx context.Context, keep map[types.Hash]struct{}) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE _gc_keep_nodes (hash BLOB PRIMARY KEY)`); err != nil {
		return 0, fmt.Errorf("create temp table: %w", err)
	}

	// Batch-insert all hashes into the temp table.
	hashes := make([]types.Hash, 0, len(keep))
	for h := range keep {
		hashes = append(hashes, h)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO _gc_keep_nodes (hash) VALUES (?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i := 0; i < len(hashes); i += gcBatchSize {
		end := i + gcBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		for _, h := range hashes[i:end] {
			h := h // capture loop var
			if _, err := stmt.ExecContext(ctx, h[:]); err != nil {
				return 0, fmt.Errorf("insert keep hash: %w", err)
			}
		}
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE node_hash NOT IN (SELECT hash FROM _gc_keep_nodes)`)
	if err != nil {
		return 0, fmt.Errorf("delete nodes: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS _gc_keep_nodes`); err != nil {
		return 0, fmt.Errorf("drop temp table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return n, nil
}

// DeleteEdgesNotIn deletes all edge rows whose edge_hash is not in the provided
// keep set. Uses a temporary table for efficient NOT IN checking with large sets.
// Returns the number of deleted edges.
func (s *SQLiteStore) DeleteEdgesNotIn(ctx context.Context, keep map[types.Hash]struct{}) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE _gc_keep_edges (hash BLOB PRIMARY KEY)`); err != nil {
		return 0, fmt.Errorf("create temp table: %w", err)
	}

	// Batch-insert all hashes into the temp table.
	hashes := make([]types.Hash, 0, len(keep))
	for h := range keep {
		hashes = append(hashes, h)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO _gc_keep_edges (hash) VALUES (?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for i := 0; i < len(hashes); i += gcBatchSize {
		end := i + gcBatchSize
		if end > len(hashes) {
			end = len(hashes)
		}
		for _, h := range hashes[i:end] {
			h := h // capture loop var
			if _, err := stmt.ExecContext(ctx, h[:]); err != nil {
				return 0, fmt.Errorf("insert keep hash: %w", err)
			}
		}
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM edges WHERE edge_hash NOT IN (SELECT hash FROM _gc_keep_edges)`)
	if err != nil {
		return 0, fmt.Errorf("delete edges: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS _gc_keep_edges`); err != nil {
		return 0, fmt.Errorf("drop temp table: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return n, nil
}
