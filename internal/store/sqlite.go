// Package store provides the SQLite-backed implementation of types.GraphStore.
//
// SQLiteStore is the sole persistent storage backend for the knowing knowledge
// graph. It stores nodes, edges, files, repos, snapshots, and edge events in
// a single SQLite database using WAL mode for concurrent read access. All
// graph traversals (transitive callers/callees, blast radius) are implemented
// as recursive CTEs executed directly in SQLite.
//
// The schema is managed by embedded SQL migrations (see migrate.go). Batch
// insert methods (BatchPutNodes, BatchPutEdges, BatchPutFiles) wrap multiple
// inserts in a single transaction for performance during full-repo indexing.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements types.GraphStore backed by a SQLite database.
// It uses WAL (Write-Ahead Logging) mode, which allows concurrent readers
// while a single writer is active. All hash columns store raw 32-byte
// blobs; the Go layer handles hex encoding/decoding.
type SQLiteStore struct {
	db *sql.DB
}

// Compile-time check that SQLiteStore implements GraphStore.
var _ types.GraphStore = (*SQLiteStore)(nil)

// NewSQLiteStore opens (or creates) a SQLite database at dbPath, enables WAL
// mode, and runs any pending migrations.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ----- Put methods -----
// All Put methods use INSERT OR REPLACE (upsert) semantics. If a row with
// the same primary key (hash) already exists, it is replaced entirely.

// PutNode upserts a single node into the nodes table.
func (s *SQLiteStore) PutNode(ctx context.Context, n types.Node) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO nodes (node_hash, file_hash, qualified_name, kind, line, signature)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.NodeHash[:], n.FileHash[:], n.QualifiedName, n.Kind, n.Line, n.Signature,
	)
	return err
}

// PutEdge upserts a single edge into the edges table.
func (s *SQLiteStore) PutEdge(ctx context.Context, e types.Edge) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EdgeHash[:], e.SourceHash[:], e.TargetHash[:], e.EdgeType, e.Confidence, e.Provenance,
		e.CallSiteLine, e.CallSiteCol, e.CallSiteFile,
	)
	return err
}

// PutFile upserts a single file record into the files table.
func (s *SQLiteStore) PutFile(ctx context.Context, f types.File) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO files (file_hash, repo_hash, path, content_hash)
		 VALUES (?, ?, ?, ?)`,
		f.FileHash[:], f.RepoHash[:], f.Path, f.ContentHash[:],
	)
	return err
}

// PutRepo upserts a repo record into the repos table.
func (s *SQLiteStore) PutRepo(ctx context.Context, r types.Repo) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO repos (repo_hash, repo_url, last_commit, last_indexed)
		 VALUES (?, ?, ?, ?)`,
		r.RepoHash[:], r.RepoURL, r.LastCommit, r.LastIndexed,
	)
	return err
}

// RecordEdgeEvent appends an edge mutation event (added/removed) to the
// edge_events table. These events are append-only and power snapshot diffing.
func (s *SQLiteStore) RecordEdgeEvent(ctx context.Context, ev types.EdgeEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO edge_events (edge_hash, event_type, snapshot_hash, source_commit, indexer_ver, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ev.EdgeHash[:], ev.EventType, ev.SnapshotHash[:], ev.SourceCommit, ev.IndexerVer, ev.Timestamp,
	)
	return err
}

// BatchPutNodes inserts multiple nodes in a single transaction.
func (s *SQLiteStore) BatchPutNodes(ctx context.Context, nodes []types.Node) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO nodes (node_hash, file_hash, qualified_name, kind, line, signature)
		 VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, n := range nodes {
		if _, err := stmt.ExecContext(ctx, n.NodeHash[:], n.FileHash[:], n.QualifiedName, n.Kind, n.Line, n.Signature); err != nil {
			return fmt.Errorf("exec node %s: %w", n.QualifiedName, err)
		}
	}
	return tx.Commit()
}

// BatchPutEdges inserts multiple edges in a single transaction.
func (s *SQLiteStore) BatchPutEdges(ctx context.Context, edges []types.Edge) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range edges {
		if _, err := stmt.ExecContext(ctx, e.EdgeHash[:], e.SourceHash[:], e.TargetHash[:], e.EdgeType, e.Confidence, e.Provenance, e.CallSiteLine, e.CallSiteCol, e.CallSiteFile); err != nil {
			return fmt.Errorf("exec edge %s: %w", e.EdgeHash, err)
		}
	}
	return tx.Commit()
}

// BatchPutFiles inserts multiple files in a single transaction.
func (s *SQLiteStore) BatchPutFiles(ctx context.Context, files []types.File) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO files (file_hash, repo_hash, path, content_hash)
		 VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, f := range files {
		if _, err := stmt.ExecContext(ctx, f.FileHash[:], f.RepoHash[:], f.Path, f.ContentHash[:]); err != nil {
			return fmt.Errorf("exec file %s: %w", f.Path, err)
		}
	}
	return tx.Commit()
}

// CreateSnapshot upserts a snapshot record into the snapshots table.
func (s *SQLiteStore) CreateSnapshot(ctx context.Context, snap types.Snapshot) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO snapshots (snapshot_hash, parent_hash, repo_hash, commit_hash, timestamp, node_count, edge_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		snap.SnapshotHash[:], snap.ParentHash[:], snap.RepoHash[:], snap.CommitHash, snap.Timestamp, snap.NodeCount, snap.EdgeCount,
	)
	return err
}

// ----- Get methods -----
// All Get methods return (nil, nil) when the requested entity is not found,
// following the convention that "not found" is not an error.

// GetNode retrieves a node by its content-addressed hash. Returns nil if not found.
func (s *SQLiteStore) GetNode(ctx context.Context, hash types.Hash) (*types.Node, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT node_hash, file_hash, qualified_name, kind, line, signature FROM nodes WHERE node_hash = ?`,
		hash[:],
	)
	return scanNode(row)
}

// GetEdge retrieves an edge by its content-addressed hash. Returns nil if not found.
func (s *SQLiteStore) GetEdge(ctx context.Context, hash types.Hash) (*types.Edge, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file FROM edges WHERE edge_hash = ?`,
		hash[:],
	)
	return scanEdge(row)
}

// GetSnapshot retrieves a snapshot by its Merkle root hash. Returns nil if not found.
func (s *SQLiteStore) GetSnapshot(ctx context.Context, hash types.Hash) (*types.Snapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT snapshot_hash, parent_hash, repo_hash, commit_hash, timestamp, node_count, edge_count
		 FROM snapshots WHERE snapshot_hash = ?`,
		hash[:],
	)
	return scanSnapshot(row)
}

// GetRepo retrieves a repo by its hash (sha256 of repo URL). Returns nil if not found.
func (s *SQLiteStore) GetRepo(ctx context.Context, hash types.Hash) (*types.Repo, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT repo_hash, repo_url, last_commit, last_indexed FROM repos WHERE repo_hash = ?`,
		hash[:],
	)
	var r types.Repo
	var hashBytes []byte
	var lastCommit sql.NullString
	var lastIndexed sql.NullInt64
	if err := row.Scan(&hashBytes, &r.RepoURL, &lastCommit, &lastIndexed); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(r.RepoHash[:], hashBytes)
	if lastCommit.Valid {
		r.LastCommit = lastCommit.String
	}
	if lastIndexed.Valid {
		r.LastIndexed = lastIndexed.Int64
	}
	return &r, nil
}

// ----- Query methods -----

// NodesByName returns all nodes whose qualified name starts with the given
// prefix. Used by the indexer to find all nodes for a repo (prefix = repoURL)
// and by the query CLI to search by symbol name.
func (s *SQLiteStore) NodesByName(ctx context.Context, qualifiedPrefix string) ([]types.Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_hash, file_hash, qualified_name, kind, line, signature
		 FROM nodes WHERE qualified_name LIKE ? ORDER BY qualified_name`,
		qualifiedPrefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// EdgesFrom returns all edges originating from the given source node.
// If edgeType is non-empty, only edges of that type are returned.
func (s *SQLiteStore) EdgesFrom(ctx context.Context, sourceHash types.Hash, edgeType string) ([]types.Edge, error) {
	query := `SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file
		 FROM edges WHERE source_hash = ?`
	args := []interface{}{sourceHash[:]}
	if edgeType != "" {
		query += ` AND edge_type = ?`
		args = append(args, edgeType)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// EdgesTo returns all edges pointing to the given target node.
// If edgeType is non-empty, only edges of that type are returned.
func (s *SQLiteStore) EdgesTo(ctx context.Context, targetHash types.Hash, edgeType string) ([]types.Edge, error) {
	query := `SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file
		 FROM edges WHERE target_hash = ?`
	args := []interface{}{targetHash[:]}
	if edgeType != "" {
		query += ` AND edge_type = ?`
		args = append(args, edgeType)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// TransitiveCallers finds all nodes that transitively call the target node,
// up to maxDepth hops. The snapshot parameter is accepted for API compatibility
// but is not currently used for filtering.
//
// Implementation: a recursive CTE walks the "calls" edges backwards from the
// target. The base case selects all direct callers (depth=1), and the recursive
// step joins each caller against edges pointing to it, incrementing depth.
// UNION (not UNION ALL) deduplicates cycles. Results are joined back to the
// nodes table for full node data and ordered by depth then qualified name.
func (s *SQLiteStore) TransitiveCallers(ctx context.Context, target types.Hash, maxDepth int, snapshot types.Hash) ([]types.CallerResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE callers(node_hash, depth) AS (
			SELECT source_hash, 1 FROM edges
			WHERE target_hash = ? AND edge_type = 'calls'
			UNION
			SELECT e.source_hash, c.depth + 1
			FROM edges e
			JOIN callers c ON e.target_hash = c.node_hash
			WHERE c.depth < ? AND e.edge_type = 'calls'
		)
		SELECT DISTINCT n.node_hash, n.file_hash, n.qualified_name, n.kind, n.line, n.signature, c.depth
		FROM callers c
		JOIN nodes n ON n.node_hash = c.node_hash
		ORDER BY c.depth, n.qualified_name`,
		target[:], maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []types.CallerResult
	for rows.Next() {
		var cr types.CallerResult
		var nodeHash, fileHash []byte
		if err := rows.Scan(&nodeHash, &fileHash, &cr.Node.QualifiedName, &cr.Node.Kind, &cr.Node.Line, &cr.Node.Signature, &cr.Depth); err != nil {
			return nil, err
		}
		copy(cr.Node.NodeHash[:], nodeHash)
		copy(cr.Node.FileHash[:], fileHash)
		results = append(results, cr)
	}
	return results, rows.Err()
}

// TransitiveCallees finds all nodes that are transitively called by the
// source node, up to maxDepth hops. This is the forward traversal
// counterpart to TransitiveCallers.
//
// Implementation: a recursive CTE walks "calls" edges forward from the
// source. The base case selects all direct callees (depth=1), and the
// recursive step follows outgoing call edges from each callee.
func (s *SQLiteStore) TransitiveCallees(ctx context.Context, source types.Hash, maxDepth int, snapshot types.Hash) ([]types.CalleeResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE callees(node_hash, depth) AS (
			SELECT target_hash, 1 FROM edges
			WHERE source_hash = ? AND edge_type = 'calls'
			UNION
			SELECT e.target_hash, c.depth + 1
			FROM edges e
			JOIN callees c ON e.source_hash = c.node_hash
			WHERE c.depth < ? AND e.edge_type = 'calls'
		)
		SELECT DISTINCT n.node_hash, n.file_hash, n.qualified_name, n.kind, n.line, n.signature, c.depth
		FROM callees c
		JOIN nodes n ON n.node_hash = c.node_hash
		ORDER BY c.depth, n.qualified_name`,
		source[:], maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []types.CalleeResult
	for rows.Next() {
		var cr types.CalleeResult
		var nodeHash, fileHash []byte
		if err := rows.Scan(&nodeHash, &fileHash, &cr.Node.QualifiedName, &cr.Node.Kind, &cr.Node.Line, &cr.Node.Signature, &cr.Depth); err != nil {
			return nil, err
		}
		copy(cr.Node.NodeHash[:], nodeHash)
		copy(cr.Node.FileHash[:], fileHash)
		results = append(results, cr)
	}
	return results, rows.Err()
}

// BlastRadius computes the blast radius of a target symbol: all functions
// that transitively call it, grouped by repository. It combines
// TransitiveCallers with a repo lookup for each caller to produce the
// grouped result. The traversal is capped at 5 levels deep.
func (s *SQLiteStore) BlastRadius(ctx context.Context, target types.Hash, snapshot types.Hash) (*types.BlastRadiusResult, error) {
	// Get the target node itself.
	targetNode, err := s.GetNode(ctx, target)
	if err != nil {
		return nil, err
	}
	if targetNode == nil {
		return nil, fmt.Errorf("target node not found")
	}

	callers, err := s.TransitiveCallers(ctx, target, 5, snapshot)
	if err != nil {
		return nil, err
	}

	result := &types.BlastRadiusResult{
		Target: *targetNode,
		ByRepo: make(map[string][]types.CallerWithProvenance),
	}

	for _, cr := range callers {
		// Look up the file to get the repo hash, then the repo URL.
		var repoURL string
		var repoHashBytes []byte
		err := s.db.QueryRowContext(ctx,
			`SELECT r.repo_url, r.repo_hash FROM repos r
			 JOIN files f ON f.repo_hash = r.repo_hash
			 WHERE f.file_hash = ?`, cr.Node.FileHash[:]).Scan(&repoURL, &repoHashBytes)
		if err != nil {
			// If we cannot resolve the repo, use "unknown".
			repoURL = "unknown"
		}

		cwp := types.CallerWithProvenance{
			Caller:     cr.Node,
			Depth:      cr.Depth,
			Confidence: 1.0,
		}
		result.ByRepo[repoURL] = append(result.ByRepo[repoURL], cwp)
		result.TotalCount++
	}

	return result, nil
}

// SnapshotDiff computes the structural diff between two snapshots by
// querying edge_events recorded during the newer snapshot's index run.
// Added edges are events with type "added" in the new snapshot;
// removed edges are events with type "removed".
func (s *SQLiteStore) SnapshotDiff(ctx context.Context, oldRoot, newRoot types.Hash) (*types.DiffResult, error) {
	diff := &types.DiffResult{
		OldSnapshot: oldRoot,
		NewSnapshot: newRoot,
	}

	// Edges added: events in the new snapshot that are "added" and not in old.
	addedRows, err := s.db.QueryContext(ctx, `
		SELECT e.edge_hash, e.source_hash, e.target_hash, e.edge_type, e.confidence, e.provenance, e.callsite_line, e.callsite_col, e.callsite_file
		FROM edge_events ev
		JOIN edges e ON e.edge_hash = ev.edge_hash
		WHERE ev.snapshot_hash = ? AND ev.event_type = 'added'`,
		newRoot[:],
	)
	if err != nil {
		return nil, err
	}
	defer addedRows.Close()
	diff.EdgesAdded, err = scanEdges(addedRows)
	if err != nil {
		return nil, err
	}

	// Edges removed: events in the new snapshot that are "removed".
	removedRows, err := s.db.QueryContext(ctx, `
		SELECT e.edge_hash, e.source_hash, e.target_hash, e.edge_type, e.confidence, e.provenance, e.callsite_line, e.callsite_col, e.callsite_file
		FROM edge_events ev
		JOIN edges e ON e.edge_hash = ev.edge_hash
		WHERE ev.snapshot_hash = ? AND ev.event_type = 'removed'`,
		newRoot[:],
	)
	if err != nil {
		return nil, err
	}
	defer removedRows.Close()
	diff.EdgesRemoved, err = scanEdges(removedRows)
	if err != nil {
		return nil, err
	}

	return diff, nil
}

// StaleEdges finds edges whose source file has been updated since the edge
// was created. An edge is stale when its source node's file_hash points to
// a File record whose content_hash no longer matches the latest file at that
// repo+path. This indicates the source file has changed and the edge may
// no longer be valid.
//
// Implementation: joins edges -> nodes -> files, then uses an EXISTS subquery
// to find any other file at the same (repo, path) with a different content
// hash. If such a file exists, the edge is stale.
func (s *SQLiteStore) StaleEdges(ctx context.Context, snapshot types.Hash) ([]types.Edge, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT e.edge_hash, e.source_hash, e.target_hash, e.edge_type, e.confidence, e.provenance, e.callsite_line, e.callsite_col, e.callsite_file
		FROM edges e
		JOIN nodes n ON n.node_hash = e.source_hash
		JOIN files f ON f.file_hash = n.file_hash
		WHERE EXISTS (
			SELECT 1 FROM files f2
			WHERE f2.repo_hash = f.repo_hash
			AND f2.path = f.path
			AND f2.content_hash != f.content_hash
		)`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// LatestSnapshot returns the most recent snapshot for a repository, ordered
// by timestamp descending. Returns nil if no snapshots exist for the repo.
func (s *SQLiteStore) LatestSnapshot(ctx context.Context, repoHash types.Hash) (*types.Snapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT snapshot_hash, parent_hash, repo_hash, commit_hash, timestamp, node_count, edge_count
		 FROM snapshots WHERE repo_hash = ? ORDER BY timestamp DESC LIMIT 1`,
		repoHash[:],
	)
	snap, err := scanSnapshot(row)
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// FilesByRepo returns all files belonging to a repository, ordered by path.
func (s *SQLiteStore) FilesByRepo(ctx context.Context, repoHash types.Hash) ([]types.File, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT file_hash, repo_hash, path, content_hash FROM files WHERE repo_hash = ? ORDER BY path`,
		repoHash[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []types.File
	for rows.Next() {
		var f types.File
		var fh, rh, ch []byte
		if err := rows.Scan(&fh, &rh, &f.Path, &ch); err != nil {
			return nil, err
		}
		copy(f.FileHash[:], fh)
		copy(f.RepoHash[:], rh)
		copy(f.ContentHash[:], ch)
		files = append(files, f)
	}
	return files, rows.Err()
}

// FileByPath looks up a single file by repo hash and relative path.
// Returns nil if no matching file exists.
func (s *SQLiteStore) FileByPath(ctx context.Context, repoHash types.Hash, path string) (*types.File, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT file_hash, repo_hash, path, content_hash FROM files WHERE repo_hash = ? AND path = ?`,
		repoHash[:], path,
	)
	var f types.File
	var fh, rh, ch []byte
	if err := row.Scan(&fh, &rh, &f.Path, &ch); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(f.FileHash[:], fh)
	copy(f.RepoHash[:], rh)
	copy(f.ContentHash[:], ch)
	return &f, nil
}

// ----- Resolver query methods -----

// DanglingEdges returns all edges whose target_hash does not match any
// existing node. These are cross-repo edges where the target was computed
// with the wrong repo URL. The resolver uses this to find and retarget them.
func (s *SQLiteStore) DanglingEdges(ctx context.Context) ([]types.Edge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT e.edge_hash, e.source_hash, e.target_hash, e.edge_type, e.confidence, e.provenance, e.callsite_line, e.callsite_col, e.callsite_file
		 FROM edges e
		 LEFT JOIN nodes n ON n.node_hash = e.target_hash
		 WHERE n.node_hash IS NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// AllRepos returns all tracked repositories ordered by URL.
func (s *SQLiteStore) AllRepos(ctx context.Context) ([]types.Repo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT repo_hash, repo_url, last_commit, last_indexed FROM repos ORDER BY repo_url`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []types.Repo
	for rows.Next() {
		var r types.Repo
		var hashBytes []byte
		var lastCommit sql.NullString
		var lastIndexed sql.NullInt64
		if err := rows.Scan(&hashBytes, &r.RepoURL, &lastCommit, &lastIndexed); err != nil {
			return nil, err
		}
		copy(r.RepoHash[:], hashBytes)
		if lastCommit.Valid {
			r.LastCommit = lastCommit.String
		}
		if lastIndexed.Valid {
			r.LastIndexed = lastIndexed.Int64
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// NodesByQualifiedName returns all nodes with an exact qualified name match.
func (s *SQLiteStore) NodesByQualifiedName(ctx context.Context, qualifiedName string) ([]types.Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_hash, file_hash, qualified_name, kind, line, signature
		 FROM nodes WHERE qualified_name = ?`,
		qualifiedName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// DeleteEdge removes an edge by its hash. Used by the enricher to replace
// ast_inferred edges with lsp_resolved edges, and by the resolver to
// retarget dangling edges.
func (s *SQLiteStore) DeleteEdge(ctx context.Context, hash types.Hash) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM edges WHERE edge_hash = ?`,
		hash[:],
	)
	return err
}

// DeleteNodesByFile removes all nodes belonging to a file. Returns the count
// of deleted nodes. Used during incremental re-indexing to clear stale nodes
// before inserting fresh ones from the updated file.
func (s *SQLiteStore) DeleteNodesByFile(ctx context.Context, fileHash types.Hash) (int, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM nodes WHERE file_hash = ?`,
		fileHash[:],
	)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// DeleteEdgesBySourceFile removes all edges whose source node belongs to
// the given file. Returns the deleted edges so the indexer can record
// "removed" edge events for snapshot diffing.
func (s *SQLiteStore) DeleteEdgesBySourceFile(ctx context.Context, fileHash types.Hash) ([]types.Edge, error) {
	// Read the edges before deleting so we can return them for event recording.
	rows, err := s.db.QueryContext(ctx,
		`SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file
		 FROM edges WHERE source_hash IN (SELECT node_hash FROM nodes WHERE file_hash = ?)`,
		fileHash[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	edges, err := scanEdges(rows)
	if err != nil {
		return nil, err
	}

	// Then delete them.
	_, err = s.db.ExecContext(ctx,
		`DELETE FROM edges WHERE source_hash IN (SELECT node_hash FROM nodes WHERE file_hash = ?)`,
		fileHash[:],
	)
	if err != nil {
		return nil, err
	}
	return edges, nil
}

// EdgesBySourceFile returns all edges whose source node belongs to the given file.
func (s *SQLiteStore) EdgesBySourceFile(ctx context.Context, fileHash types.Hash) ([]types.Edge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file
		 FROM edges WHERE source_hash IN (SELECT node_hash FROM nodes WHERE file_hash = ?)`,
		fileHash[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// ----- Scanner helpers -----
// These functions handle the conversion between SQLite BLOB columns (raw
// byte slices) and Go types.Hash ([32]byte fixed arrays). SQLite stores
// hashes as variable-length BLOBs, so we scan into []byte and copy into
// the fixed-size array.

// scannable abstracts *sql.Row and *sql.Rows so the same scan logic works
// for both single-row and multi-row queries.
type scannable interface {
	Scan(dest ...interface{}) error
}

func scanNode(row scannable) (*types.Node, error) {
	var n types.Node
	var nodeHash, fileHash []byte
	if err := row.Scan(&nodeHash, &fileHash, &n.QualifiedName, &n.Kind, &n.Line, &n.Signature); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(n.NodeHash[:], nodeHash)
	copy(n.FileHash[:], fileHash)
	return &n, nil
}

func scanEdge(row scannable) (*types.Edge, error) {
	var e types.Edge
	var edgeHash, sourceHash, targetHash []byte
	if err := row.Scan(&edgeHash, &sourceHash, &targetHash, &e.EdgeType, &e.Confidence, &e.Provenance, &e.CallSiteLine, &e.CallSiteCol, &e.CallSiteFile); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(e.EdgeHash[:], edgeHash)
	copy(e.SourceHash[:], sourceHash)
	copy(e.TargetHash[:], targetHash)
	return &e, nil
}

func scanEdges(rows *sql.Rows) ([]types.Edge, error) {
	var edges []types.Edge
	for rows.Next() {
		var e types.Edge
		var edgeHash, sourceHash, targetHash []byte
		if err := rows.Scan(&edgeHash, &sourceHash, &targetHash, &e.EdgeType, &e.Confidence, &e.Provenance, &e.CallSiteLine, &e.CallSiteCol, &e.CallSiteFile); err != nil {
			return nil, err
		}
		copy(e.EdgeHash[:], edgeHash)
		copy(e.SourceHash[:], sourceHash)
		copy(e.TargetHash[:], targetHash)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

func scanNodes(rows *sql.Rows) ([]types.Node, error) {
	var nodes []types.Node
	for rows.Next() {
		var n types.Node
		var nodeHash, fileHash []byte
		if err := rows.Scan(&nodeHash, &fileHash, &n.QualifiedName, &n.Kind, &n.Line, &n.Signature); err != nil {
			return nil, err
		}
		copy(n.NodeHash[:], nodeHash)
		copy(n.FileHash[:], fileHash)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func scanSnapshot(row scannable) (*types.Snapshot, error) {
	var snap types.Snapshot
	var sh, ph, rh []byte
	if err := row.Scan(&sh, &ph, &rh, &snap.CommitHash, &snap.Timestamp, &snap.NodeCount, &snap.EdgeCount); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(snap.SnapshotHash[:], sh)
	copy(snap.ParentHash[:], ph)
	copy(snap.RepoHash[:], rh)
	return &snap, nil
}
