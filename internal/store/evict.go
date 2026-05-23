package store

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

// DeleteRepoResult summarizes what was deleted by DeleteRepoData.
type DeleteRepoResult struct {
	Nodes     int64
	Edges     int64
	Files     int64
	Snapshots int64
	Feedback  int64
	Notes     int64
}

// DeleteRepoData removes all graph data associated with a given repo_hash.
// This includes files, nodes, edges, edge_events, snapshots, feedback,
// task_memory, and graph_notes for nodes in that repo.
//
// Important: Does NOT delete from the repos table itself. The roster/CLI
// layer handles that separately.
//
// All operations are wrapped in a single transaction for atomicity.
func (s *SQLiteStore) DeleteRepoData(ctx context.Context, repoHash types.Hash) (DeleteRepoResult, error) {
	var result DeleteRepoResult

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Step 1: Create temp tables to collect file_hashes and node_hashes for this repo.
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE _evict_files (file_hash BLOB)`); err != nil {
		return result, fmt.Errorf("create temp _evict_files: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO _evict_files SELECT file_hash FROM files WHERE repo_hash = ?`, repoHash[:]); err != nil {
		return result, fmt.Errorf("populate _evict_files: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE _evict_nodes (node_hash BLOB)`); err != nil {
		return result, fmt.Errorf("create temp _evict_nodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO _evict_nodes SELECT node_hash FROM nodes WHERE file_hash IN (SELECT file_hash FROM _evict_files)`); err != nil {
		return result, fmt.Errorf("populate _evict_nodes: %w", err)
	}

	// Step 2: Delete edges where source_hash OR target_hash is in the node set.
	res, err := tx.ExecContext(ctx,
		`DELETE FROM edges WHERE source_hash IN (SELECT node_hash FROM _evict_nodes) OR target_hash IN (SELECT node_hash FROM _evict_nodes)`)
	if err != nil {
		return result, fmt.Errorf("delete edges: %w", err)
	}
	result.Edges, _ = res.RowsAffected()

	// Step 3: Delete edge_events where edge_hash matches deleted edges.
	// Since edges are already deleted, we use the node set to find events
	// via the source_hash/target_hash columns added in migration 013.
	// Also delete events for edges whose source or target was in the evicted nodes.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM edge_events WHERE source_hash IN (SELECT node_hash FROM _evict_nodes) OR target_hash IN (SELECT node_hash FROM _evict_nodes)`); err != nil {
		return result, fmt.Errorf("delete edge_events: %w", err)
	}

	// Step 4: Delete feedback where symbol_hash is in the node set.
	res, err = tx.ExecContext(ctx,
		`DELETE FROM feedback WHERE symbol_hash IN (SELECT node_hash FROM _evict_nodes)`)
	if err != nil {
		return result, fmt.Errorf("delete feedback: %w", err)
	}
	result.Feedback, _ = res.RowsAffected()

	// Step 5: Delete task_memory where symbol_hash is in the node set.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM task_memory WHERE symbol_hash IN (SELECT node_hash FROM _evict_nodes)`); err != nil {
		return result, fmt.Errorf("delete task_memory: %w", err)
	}

	// Step 6: Delete graph_notes where object_hash is in the node set.
	res, err = tx.ExecContext(ctx,
		`DELETE FROM graph_notes WHERE object_hash IN (SELECT node_hash FROM _evict_nodes)`)
	if err != nil {
		return result, fmt.Errorf("delete graph_notes: %w", err)
	}
	result.Notes, _ = res.RowsAffected()

	// Step 7: Delete nodes.
	res, err = tx.ExecContext(ctx,
		`DELETE FROM nodes WHERE file_hash IN (SELECT file_hash FROM _evict_files)`)
	if err != nil {
		return result, fmt.Errorf("delete nodes: %w", err)
	}
	result.Nodes, _ = res.RowsAffected()

	// Step 8: Delete files for the repo.
	res, err = tx.ExecContext(ctx,
		`DELETE FROM files WHERE repo_hash = ?`, repoHash[:])
	if err != nil {
		return result, fmt.Errorf("delete files: %w", err)
	}
	result.Files, _ = res.RowsAffected()

	// Step 9: Delete snapshots for the repo.
	res, err = tx.ExecContext(ctx,
		`DELETE FROM snapshots WHERE repo_hash = ?`, repoHash[:])
	if err != nil {
		return result, fmt.Errorf("delete snapshots: %w", err)
	}
	result.Snapshots, _ = res.RowsAffected()

	// Step 10: Drop temp tables.
	if _, err := tx.ExecContext(ctx, `DROP TABLE _evict_nodes`); err != nil {
		return result, fmt.Errorf("drop _evict_nodes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE _evict_files`); err != nil {
		return result, fmt.Errorf("drop _evict_files: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit: %w", err)
	}

	// Invalidate caches since we may have removed cached nodes/edges.
	s.InvalidateCache()

	return result, nil
}
