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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"

	_ "modernc.org/sqlite"
)

// nodeCacheMaxEntries is the maximum number of nodes held in the in-process
// cache before it is reset to avoid unbounded memory growth.
const nodeCacheMaxEntries = 50_000

// edgeCacheMaxEntries is the maximum number of edges held in the in-process
// cache before it is reset.
const edgeCacheMaxEntries = 50_000

// SQLiteStore implements types.GraphStore backed by a SQLite database.
// It uses WAL (Write-Ahead Logging) mode, which allows concurrent readers
// while a single writer is active. All hash columns store raw 32-byte
// blobs; the Go layer handles hex encoding/decoding.
//
// An in-process node/edge cache is layered on top of SQLite to eliminate
// redundant SQL round-trips on hot-path traversals such as blast_radius,
// which can walk hundreds of edges. The cache is invalidated at the start
// of each index run via InvalidateCache.
type SQLiteStore struct {
	db *sql.DB

	// nodeCache caches *types.Node values keyed by types.Hash.
	nodeCache      sync.Map
	nodeCacheCount atomic.Int64

	// edgeCache caches *types.Edge values keyed by types.Hash.
	edgeCache      sync.Map
	edgeCacheCount atomic.Int64
}

// Compile-time check that SQLiteStore implements GraphStore.
var _ types.GraphStore = (*SQLiteStore)(nil)

// DB returns the underlying sql.DB for direct access (e.g., task memory).
func (s *SQLiteStore) DB() *sql.DB { return s.db }

// NewSQLiteStore opens (or creates) a SQLite database at dbPath, enables WAL
// mode, and runs any pending migrations.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Performance pragmas for bulk indexing workloads.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",  // WAL mode is safe with NORMAL (fsync on checkpoint, not every commit)
		"PRAGMA mmap_size=268435456", // 256MB memory-mapped I/O for read-heavy workloads
		"PRAGMA cache_size=-64000",   // 64MB page cache (negative = KB)
		"PRAGMA busy_timeout=5000",   // 5s retry on lock contention instead of immediate SQLITE_BUSY
		"PRAGMA temp_store=MEMORY",   // temp tables/indexes in memory
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
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

// IntegrityCheck runs PRAGMA integrity_check on the SQLite database.
// Returns nil if the database passes all checks, or an error describing
// the first corruption issues found.
func (s *SQLiteStore) IntegrityCheck(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("sqlite integrity_check query: %w", err)
	}
	defer rows.Close()

	var issues []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return fmt.Errorf("sqlite integrity_check scan: %w", err)
		}
		if msg == "ok" {
			return nil
		}
		issues = append(issues, msg)
		if len(issues) >= 10 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite integrity_check rows: %w", err)
	}
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("sqlite integrity check failed: %s", strings.Join(issues, "; "))
}

// ----- Put methods -----
// All Put methods use INSERT OR REPLACE (upsert) semantics. If a row with
// the same primary key (hash) already exists, it is replaced entirely.

// PutNode upserts a single node into the nodes table.
func (s *SQLiteStore) PutNode(ctx context.Context, n types.Node) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO nodes (node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.NodeHash[:], n.FileHash[:], n.QualifiedName, n.Kind, n.Line, n.Signature, n.Doc, n.LastAuthor, n.LastCommitAt, n.CoveragePct, time.Now().Unix(),
	)
	if err != nil {
		return err
	}
	// Keep FTS index in sync (best-effort; ignore errors if table doesn't exist yet).
	s.upsertFTS(ctx, n.NodeHash[:], n.QualifiedName, n.Signature, "")
	return nil
}

// UpdateNodeBlame stamps git blame metadata on a node without replacing it.
func (s *SQLiteStore) UpdateNodeBlame(ctx context.Context, nodeHash types.Hash, author string, commitAt int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET last_author = ?, last_commit_at = ? WHERE node_hash = ?`,
		author, commitAt, nodeHash[:],
	)
	return err
}

// UpdateNodeCoverage stamps test coverage percentage on a node.
func (s *SQLiteStore) UpdateNodeCoverage(ctx context.Context, nodeHash types.Hash, pct float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET coverage_pct = ? WHERE node_hash = ?`,
		pct, nodeHash[:],
	)
	return err
}

// PutEdge upserts a single edge into the edges table.
func (s *SQLiteStore) PutEdge(ctx context.Context, e types.Edge) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, indexed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EdgeHash[:], e.SourceHash[:], e.TargetHash[:], e.EdgeType, e.Confidence, e.Provenance,
		e.CallSiteLine, e.CallSiteCol, e.CallSiteFile, time.Now().Unix(),
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
		`INSERT INTO edge_events (edge_hash, event_type, snapshot_hash, source_commit, indexer_ver, timestamp, source_hash, target_hash, edge_type, confidence, provenance)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.EdgeHash[:], ev.EventType, ev.SnapshotHash[:], ev.SourceCommit, ev.IndexerVer, ev.Timestamp,
		ev.SourceHash[:], ev.TargetHash[:], ev.EdgeType, ev.Confidence, ev.Provenance,
	)
	return err
}

// BatchPutNodes inserts multiple nodes in a single transaction.
func (s *SQLiteStore) BatchPutNodes(ctx context.Context, nodes []types.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Multi-row INSERT: 10 params per node, SQLite limit is 999 variables.
	// Use chunks of 99 nodes (990 params) per statement.
	const chunkSize = 99
	for i := 0; i < len(nodes); i += chunkSize {
		end := i + chunkSize
		if end > len(nodes) {
			end = len(nodes)
		}
		chunk := nodes[i:end]

		var b strings.Builder
		b.WriteString(`INSERT OR REPLACE INTO nodes (node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct) VALUES `)
		args := make([]interface{}, 0, len(chunk)*10)
		for j, n := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?,?,?,?,?,?,?,?,?,?)")
			args = append(args, n.NodeHash[:], n.FileHash[:], n.QualifiedName, n.Kind, n.Line, n.Signature, n.Doc, n.LastAuthor, n.LastCommitAt, n.CoveragePct)
		}

		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("batch insert nodes chunk %d: %w", i/chunkSize, err)
		}
	}
	return tx.Commit()
}

// BatchPutEdges inserts multiple edges in a single transaction using multi-row
// INSERT statements for reduced per-row overhead.
func (s *SQLiteStore) BatchPutEdges(ctx context.Context, edges []types.Edge) error {
	if len(edges) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Multi-row INSERT: 9 params per edge, SQLite limit is 999 variables.
	// Use chunks of 100 edges (900 params) per statement.
	const chunkSize = 100
	for i := 0; i < len(edges); i += chunkSize {
		end := i + chunkSize
		if end > len(edges) {
			end = len(edges)
		}
		chunk := edges[i:end]

		var b strings.Builder
		b.WriteString(`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file) VALUES `)
		args := make([]interface{}, 0, len(chunk)*9)
		for j, e := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?,?,?,?,?,?,?,?,?)")
			args = append(args, e.EdgeHash[:], e.SourceHash[:], e.TargetHash[:], e.EdgeType, e.Confidence, e.Provenance, e.CallSiteLine, e.CallSiteCol, e.CallSiteFile)
		}

		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("batch insert edges chunk %d: %w", i/chunkSize, err)
		}
	}
	return tx.Commit()
}

// BatchPutFiles inserts multiple files in a single transaction using multi-row
// INSERT statements.
func (s *SQLiteStore) BatchPutFiles(ctx context.Context, files []types.File) error {
	if len(files) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Multi-row INSERT: 4 params per file, SQLite limit is 999 variables.
	// Use chunks of 249 files (996 params) per statement.
	const chunkSize = 249
	for i := 0; i < len(files); i += chunkSize {
		end := i + chunkSize
		if end > len(files) {
			end = len(files)
		}
		chunk := files[i:end]

		var b strings.Builder
		b.WriteString(`INSERT OR REPLACE INTO files (file_hash, repo_hash, path, content_hash) VALUES `)
		args := make([]interface{}, 0, len(chunk)*4)
		for j, f := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?,?,?,?)")
			args = append(args, f.FileHash[:], f.RepoHash[:], f.Path, f.ContentHash[:])
		}

		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("batch insert files chunk %d: %w", i/chunkSize, err)
		}
	}
	return tx.Commit()
}

// CreateSnapshot upserts a snapshot record into the snapshots table.
func (s *SQLiteStore) CreateSnapshot(ctx context.Context, snap types.Snapshot) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO snapshots (snapshot_hash, parent_hash, repo_hash, commit_hash, timestamp, node_count, edge_count, generation)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.SnapshotHash[:], snap.ParentHash[:], snap.RepoHash[:], snap.CommitHash, snap.Timestamp, snap.NodeCount, snap.EdgeCount, snap.Generation,
	)
	return err
}

// DeleteSnapshot removes a snapshot and its associated edge events.
func (s *SQLiteStore) DeleteSnapshot(ctx context.Context, hash types.Hash) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM edge_events WHERE snapshot_hash = ?`, hash[:])
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM snapshots WHERE snapshot_hash = ?`, hash[:])
	return err
}

// ----- Get methods -----
// All Get methods return (nil, nil) when the requested entity is not found,
// following the convention that "not found" is not an error.

// InvalidateCache clears the in-process node and edge caches. Call this at
// the start of each index run so that freshly written rows are not shadowed
// by stale cached values.
func (s *SQLiteStore) InvalidateCache() {
	s.nodeCache.Range(func(k, _ any) bool {
		s.nodeCache.Delete(k)
		return true
	})
	s.nodeCacheCount.Store(0)

	s.edgeCache.Range(func(k, _ any) bool {
		s.edgeCache.Delete(k)
		return true
	})
	s.edgeCacheCount.Store(0)
}

// GetNode retrieves a node by its content-addressed hash. Returns nil if not found.
// Results are cached in memory; the cache is bounded to nodeCacheMaxEntries and
// is invalidated by InvalidateCache at the start of each index run.
func (s *SQLiteStore) GetNode(ctx context.Context, hash types.Hash) (*types.Node, error) {
	if v, ok := s.nodeCache.Load(hash); ok {
		return v.(*types.Node), nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct FROM nodes WHERE node_hash = ?`,
		hash[:],
	)
	n, err := scanNode(row)
	if err != nil || n == nil {
		return n, err
	}
	// Store in cache, evicting everything if cap is reached.
	if s.nodeCacheCount.Load() >= nodeCacheMaxEntries {
		s.nodeCache.Range(func(k, _ any) bool {
			s.nodeCache.Delete(k)
			return true
		})
		s.nodeCacheCount.Store(0)
	}
	s.nodeCache.Store(hash, n)
	s.nodeCacheCount.Add(1)
	return n, nil
}

// GetEdge retrieves an edge by its content-addressed hash. Returns nil if not found.
// Results are cached in memory; the cache is bounded to edgeCacheMaxEntries and
// is invalidated by InvalidateCache at the start of each index run.
func (s *SQLiteStore) GetEdge(ctx context.Context, hash types.Hash) (*types.Edge, error) {
	if v, ok := s.edgeCache.Load(hash); ok {
		return v.(*types.Edge), nil
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file FROM edges WHERE edge_hash = ?`,
		hash[:],
	)
	e, err := scanEdge(row)
	if err != nil || e == nil {
		return e, err
	}
	// Store in cache, evicting everything if cap is reached.
	if s.edgeCacheCount.Load() >= edgeCacheMaxEntries {
		s.edgeCache.Range(func(k, _ any) bool {
			s.edgeCache.Delete(k)
			return true
		})
		s.edgeCacheCount.Store(0)
	}
	s.edgeCache.Store(hash, e)
	s.edgeCacheCount.Add(1)
	return e, nil
}

// GetSnapshot retrieves a snapshot by its Merkle root hash. Returns nil if not found.
func (s *SQLiteStore) GetSnapshot(ctx context.Context, hash types.Hash) (*types.Snapshot, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT snapshot_hash, parent_hash, repo_hash, commit_hash, timestamp, node_count, edge_count, generation
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
		`SELECT node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct
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
		SELECT DISTINCT n.node_hash, n.file_hash, n.qualified_name, n.kind, n.line, n.signature, n.doc, n.last_author, n.last_commit_at, n.coverage_pct, c.depth
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
		if err := rows.Scan(&nodeHash, &fileHash, &cr.Node.QualifiedName, &cr.Node.Kind, &cr.Node.Line, &cr.Node.Signature, &cr.Node.Doc, &cr.Node.LastAuthor, &cr.Node.LastCommitAt, &cr.Node.CoveragePct, &cr.Depth); err != nil {
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
		SELECT DISTINCT n.node_hash, n.file_hash, n.qualified_name, n.kind, n.line, n.signature, n.doc, n.last_author, n.last_commit_at, n.coverage_pct, c.depth
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
		if err := rows.Scan(&nodeHash, &fileHash, &cr.Node.QualifiedName, &cr.Node.Kind, &cr.Node.Line, &cr.Node.Signature, &cr.Node.Doc, &cr.Node.LastAuthor, &cr.Node.LastCommitAt, &cr.Node.CoveragePct, &cr.Depth); err != nil {
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

	// Edges removed: read directly from edge_events (not joining to edges,
	// because removed edges have been deleted from the edges table).
	// Migration 013 added source_hash, target_hash, edge_type, confidence,
	// provenance columns to edge_events. Pre-migration events have NULLs;
	// we fall back to joining edges for those (which will return nothing
	// for truly removed edges, matching the old broken behavior).
	removedRows, err := s.db.QueryContext(ctx, `
		SELECT ev.edge_hash,
		       COALESCE(ev.source_hash, e.source_hash),
		       COALESCE(ev.target_hash, e.target_hash),
		       COALESCE(ev.edge_type, e.edge_type, ''),
		       COALESCE(ev.confidence, e.confidence, 1.0),
		       COALESCE(ev.provenance, e.provenance, 'unknown'),
		       0, 0, ''
		FROM edge_events ev
		LEFT JOIN edges e ON e.edge_hash = ev.edge_hash
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
		`SELECT snapshot_hash, parent_hash, repo_hash, commit_hash, timestamp, node_count, edge_count, generation
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

// NodesByFilePath returns all nodes belonging to a file identified by repo hash
// and relative path. It joins through the files table using the path, so it
// works regardless of whether file content (and thus file_hash) has changed.
func (s *SQLiteStore) NodesByFilePath(ctx context.Context, repoHash types.Hash, path string) ([]types.Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.node_hash, n.file_hash, n.qualified_name, n.kind, n.line, n.signature, n.doc, n.last_author, n.last_commit_at, n.coverage_pct
		 FROM nodes n
		 INNER JOIN files f ON n.file_hash = f.file_hash
		 WHERE f.repo_hash = ? AND f.path = ?`,
		repoHash[:], path,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
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
		`SELECT node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct
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
	if err == nil {
		s.edgeCache.Delete(hash)
	}
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
	// Evict all cached nodes since we don't have the individual hashes here.
	// This is conservative but correct; DeleteNodesByFile is called during
	// incremental re-indexing which immediately re-populates via PutNode.
	if n > 0 {
		s.nodeCache.Range(func(k, _ any) bool {
			s.nodeCache.Delete(k)
			return true
		})
		s.nodeCacheCount.Store(0)
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
	// Evict deleted edges from the cache.
	for _, e := range edges {
		s.edgeCache.Delete(e.EdgeHash)
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

// TruncateGraph deletes all nodes, edges, and edge events from the database.
// This is used by the reindex command to clear stale data before re-indexing.
func (s *SQLiteStore) TruncateGraph(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM edge_events`)
	if err != nil {
		return fmt.Errorf("truncate edge_events: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM edges`)
	if err != nil {
		return fmt.Errorf("truncate edges: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM nodes`)
	if err != nil {
		return fmt.Errorf("truncate nodes: %w", err)
	}
	return nil
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
	if err := row.Scan(&nodeHash, &fileHash, &n.QualifiedName, &n.Kind, &n.Line, &n.Signature, &n.Doc, &n.LastAuthor, &n.LastCommitAt, &n.CoveragePct); err != nil {
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
		if err := rows.Scan(&nodeHash, &fileHash, &n.QualifiedName, &n.Kind, &n.Line, &n.Signature, &n.Doc, &n.LastAuthor, &n.LastCommitAt, &n.CoveragePct); err != nil {
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
	if err := row.Scan(&sh, &ph, &rh, &snap.CommitHash, &snap.Timestamp, &snap.NodeCount, &snap.EdgeCount, &snap.Generation); err != nil {
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

// SearchBM25Nodes performs full-text search over the nodes_fts index using BM25 ranking.
// The query string uses FTS5 query syntax (terms joined by OR/AND).
// Returns up to limit nodes ordered by BM25 relevance (best matches first).
func (s *SQLiteStore) SearchBM25Nodes(ctx context.Context, query string, limit int) ([]types.Node, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.node_hash, n.file_hash, n.qualified_name, n.kind, n.line, n.signature, n.doc, n.last_author, n.last_commit_at, n.coverage_pct
		 FROM nodes_fts
		 JOIN nodes_fts_content c ON c.rowid = nodes_fts.rowid
		 JOIN nodes n ON n.node_hash = c.node_hash
		 WHERE nodes_fts MATCH ?
		 ORDER BY bm25(nodes_fts, 10.0, 5.0, 3.0, 1.0, 1.0)
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// upsertFTS inserts or replaces a node in the FTS content table.
// Does NOT rebuild the FTS index (that's done by RebuildFTS after batch ops).
// Best-effort: silently ignores errors (e.g., if FTS table doesn't exist yet).
func (s *SQLiteStore) upsertFTS(ctx context.Context, nodeHash []byte, qualifiedName, signature, filePath string) {
	// Delete existing entry for this node_hash (if any).
	s.db.ExecContext(ctx, //nolint:errcheck
		`DELETE FROM nodes_fts_content WHERE node_hash = ?`, nodeHash)
	// Insert new entry.
	s.db.ExecContext(ctx, //nolint:errcheck
		`INSERT INTO nodes_fts_content(node_hash, symbol_name, concepts, qualified_name, signature, file_path)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		nodeHash, extractSymbolName(qualifiedName), extractConcepts(filePath), qualifiedName, signature, filePath)
}

// extractConcepts derives searchable concept tokens from a file path.
// "src/compiler/commandLineParser.ts" -> "command Line Parser compiler"
// "src/flask/sansio/scaffold.py" -> "scaffold sansio flask"
// These tokens bridge the vocabulary gap: developers search for "parser"
// or "compiler" and find symbols defined in files with those concepts.
func extractConcepts(filePath string) string {
	if filePath == "" {
		return ""
	}

	var concepts []string

	// Split path into components.
	parts := strings.Split(strings.ReplaceAll(filePath, "\\", "/"), "/")

	// Process each path component (directories + filename).
	for _, part := range parts {
		// Skip common non-informative directories.
		lower := strings.ToLower(part)
		if lower == "src" || lower == "lib" || lower == "internal" || lower == "pkg" ||
			lower == "." || lower == "" {
			continue
		}

		// Strip file extension.
		if idx := strings.LastIndex(part, "."); idx > 0 {
			part = part[:idx]
		}

		// Split CamelCase and snake_case into words.
		words := splitCamelCase(part)
		if len(words) == 0 {
			// Not CamelCase; try snake_case.
			if strings.Contains(part, "_") {
				for _, w := range strings.Split(part, "_") {
					if len(w) >= 3 {
						concepts = append(concepts, w)
					}
				}
			} else if len(part) >= 3 {
				concepts = append(concepts, part)
			}
		} else {
			for _, w := range words {
				if len(w) >= 3 {
					concepts = append(concepts, w)
				}
			}
			// Also add the unsplit name for exact matching.
			if len(part) >= 3 {
				concepts = append(concepts, part)
			}
		}
	}

	return strings.Join(concepts, " ")
}

// extractSymbolName returns the terminal symbol identifier from a qualified name.
// For "github.com/django/django://django/db/models/query.py.QuerySet.filter"
// it returns "QuerySet.filter". For "repo://pkg/sub.TypeName.Method" it returns
// "TypeName.Method". This is the part of the name a developer would search for.
func extractSymbolName(qualifiedName string) string {
	// Strip everything before "://" (repo URL prefix).
	if idx := strings.LastIndex(qualifiedName, "://"); idx >= 0 {
		qualifiedName = qualifiedName[idx+3:]
	}
	// Find the last slash (end of package/file path).
	lastSlash := strings.LastIndex(qualifiedName, "/")
	if lastSlash >= 0 {
		qualifiedName = qualifiedName[lastSlash+1:]
	}
	// Strip file extension prefix (e.g., "query.py.QuerySet" -> strip "query.py.")
	// Look for common file extensions followed by a dot.
	for _, ext := range []string{".go.", ".py.", ".ts.", ".js.", ".rs.", ".java.", ".cs.", ".rb.", ".sql.", ".proto."} {
		if idx := strings.Index(qualifiedName, ext); idx >= 0 {
			qualifiedName = qualifiedName[idx+len(ext):]
			break
		}
	}
	return qualifiedName
}

// RebuildFTS rebuilds the entire FTS index from the nodes and files tables.
// Splits CamelCase and snake_case identifiers into individual tokens so that
// searching for "ingest" matches "TraceIngestor".
// Call after batch indexing operations for best performance (avoids per-node rebuilds).
//
// Optimization: pre-computes all splitForFTS strings in parallel (CPU-bound),
// then does a single batch INSERT (I/O-bound, sequential). The split computation
// is the expensive part for large repos (100K+ nodes).
func (s *SQLiteStore) RebuildFTS(ctx context.Context) error {
	// Clear content table.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM nodes_fts_content`); err != nil {
		return fmt.Errorf("clear fts content: %w", err)
	}

	// Read all nodes with their file paths.
	rows, err := s.db.QueryContext(ctx,
		`SELECT n.node_hash, n.qualified_name, COALESCE(n.signature, ''), COALESCE(f.path, '')
		 FROM nodes n LEFT JOIN files f ON n.file_hash = f.file_hash`)
	if err != nil {
		return fmt.Errorf("read nodes for fts: %w", err)
	}

	// Phase 1: collect all rows into memory.
	type ftsRow struct {
		nodeHash []byte
		qn, sig, fp string
	}
	var allRows []ftsRow
	for rows.Next() {
		var r ftsRow
		if err := rows.Scan(&r.nodeHash, &r.qn, &r.sig, &r.fp); err != nil {
			rows.Close()
			return fmt.Errorf("scan node for fts: %w", err)
		}
		allRows = append(allRows, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	// Phase 2: parallel splitForFTS computation.
	type ftsPrepped struct {
		nodeHash []byte
		symbolName, concepts, splitQN, splitSig, fp string
	}
	prepped := make([]ftsPrepped, len(allRows))

	var wg sync.WaitGroup
	workers := 8
	chunkSize := (len(allRows) + workers - 1) / workers
	for w := 0; w < workers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(allRows) {
			end = len(allRows)
		}
		if start >= len(allRows) {
			break
		}
		wg.Add(1)
		go func(s, e int) {
			defer wg.Done()
			for i := s; i < e; i++ {
				r := allRows[i]
				prepped[i] = ftsPrepped{
					nodeHash:   r.nodeHash,
					symbolName: splitForFTS(extractSymbolName(r.qn)),
					concepts:   extractConcepts(r.fp),
					splitQN:    splitForFTS(r.qn),
					splitSig:   splitForFTS(r.sig),
					fp:         r.fp,
				}
			}
		}(start, end)
	}
	wg.Wait()

	// Phase 3: batch INSERT (sequential, SQLite single-writer).
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fts tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO nodes_fts_content(node_hash, symbol_name, concepts, qualified_name, signature, file_path) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare fts insert: %w", err)
	}
	defer stmt.Close()

	for _, p := range prepped {
		if _, err := stmt.ExecContext(ctx, p.nodeHash, p.symbolName, p.concepts, p.splitQN, p.splitSig, p.fp); err != nil {
			return fmt.Errorf("insert fts content: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fts: %w", err)
	}

	// Rebuild the FTS index from the content table.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild fts index: %w", err)
	}
	return nil
}

// RebuildFTSForPackages deletes and re-inserts FTS rows only for nodes whose
// qualified name starts with one of the given package prefixes. Falls back to
// full RebuildFTS if packages is empty. This makes FTS rebuild proportional to
// the number of changed packages, not the total graph size.
func (s *SQLiteStore) RebuildFTSForPackages(ctx context.Context, packages []string) error {
	if len(packages) == 0 {
		return s.RebuildFTS(ctx)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin fts tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete existing FTS rows for nodes in the changed packages.
	delStmt, err := tx.PrepareContext(ctx,
		`DELETE FROM nodes_fts_content WHERE qualified_name LIKE ?`)
	if err != nil {
		return fmt.Errorf("prepare fts delete: %w", err)
	}
	defer delStmt.Close()
	for _, pkg := range packages {
		if _, err := delStmt.ExecContext(ctx, pkg+"%"); err != nil {
			return fmt.Errorf("delete fts for %s: %w", pkg, err)
		}
	}

	// Re-insert FTS rows for nodes in the changed packages.
	insStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO nodes_fts_content(node_hash, symbol_name, concepts, qualified_name, signature, file_path) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare fts insert: %w", err)
	}
	defer insStmt.Close()

	for _, pkg := range packages {
		rows, err := s.db.QueryContext(ctx,
			`SELECT n.node_hash, n.qualified_name, COALESCE(n.signature, ''), COALESCE(f.path, '')
			 FROM nodes n LEFT JOIN files f ON n.file_hash = f.file_hash
			 WHERE n.qualified_name LIKE ?`, pkg+"%")
		if err != nil {
			return fmt.Errorf("read nodes for fts (%s): %w", pkg, err)
		}
		for rows.Next() {
			var nodeHash []byte
			var qn, sig, fp string
			if err := rows.Scan(&nodeHash, &qn, &sig, &fp); err != nil {
				rows.Close()
				return fmt.Errorf("scan node for fts: %w", err)
			}
			if _, err := insStmt.ExecContext(ctx, nodeHash, splitForFTS(extractSymbolName(qn)), extractConcepts(fp), splitForFTS(qn), splitForFTS(sig), fp); err != nil {
				rows.Close()
				return fmt.Errorf("insert fts for %s: %w", qn, err)
			}
		}
		rows.Close()
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit fts: %w", err)
	}

	// Rebuild the FTS index to pick up the changes.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild fts index: %w", err)
	}
	return nil
}

// splitForFTS splits a qualified name or signature into space-separated tokens
// for full-text indexing. Splits on CamelCase boundaries, underscores, dots, and slashes.
// Preserves the original tokens alongside the splits for exact-match capability.
func splitForFTS(s string) string {
	if s == "" {
		return ""
	}
	var parts []string
	// Split on path separators and dots first.
	for _, segment := range strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == '.' || r == ':' || r == '(' || r == ')' || r == ',' || r == ' ' || r == '*'
	}) {
		if segment == "" {
			continue
		}
		parts = append(parts, segment)
		// Split CamelCase within each segment.
		camelParts := splitCamelCase(segment)
		if len(camelParts) > 1 {
			parts = append(parts, camelParts...)
		}
		// Split snake_case within each segment.
		if strings.Contains(segment, "_") {
			for _, p := range strings.Split(segment, "_") {
				if p != "" && p != segment {
					parts = append(parts, p)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

// splitCamelCase splits a CamelCase identifier into its components.
// "TraceIngestor" -> ["Trace", "Ingestor"]
// "HTMLParser" -> ["HTML", "Parser"]
// "SQLiteStore" -> ["SQLite", "Store"]
func splitCamelCase(s string) []string {
	if len(s) < 2 {
		return nil
	}
	var parts []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			if s[i-1] >= 'a' && s[i-1] <= 'z' {
				// lowercase->uppercase transition: split before the uppercase
				parts = append(parts, s[start:i])
				start = i
			} else if s[i-1] >= 'A' && s[i-1] <= 'Z' && i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z' {
				// uppercase run followed by lowercase: split before current char
				// "SQLiteStore" at i pointing to 'S' of "Store"
				parts = append(parts, s[start:i])
				start = i
			}
		}
	}
	parts = append(parts, s[start:])
	// Filter short tokens (< 2 chars) to avoid noise.
	var result []string
	for _, p := range parts {
		if len(p) >= 2 {
			result = append(result, p)
		}
	}
	return result
}

// ----- Batch notes -----

// BatchPutNotes upserts multiple notes in a single transaction.
// Significantly faster than individual PutNote calls for bulk operations
// like persisting community assignments.
func (s *SQLiteStore) BatchPutNotes(ctx context.Context, notes []types.Note) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO graph_notes (object_hash, key, value, updated_at)
		 VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, n := range notes {
		if _, err := stmt.ExecContext(ctx, n.ObjectHash[:], n.Key, n.Value, n.UpdatedAt); err != nil {
			return fmt.Errorf("exec note %s/%s: %w", n.ObjectHash, n.Key, err)
		}
	}
	return tx.Commit()
}

// ----- Notes methods -----

// PutNote upserts a note (object_hash + key is the composite key).
func (s *SQLiteStore) PutNote(ctx context.Context, n types.Note) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO graph_notes (object_hash, key, value, updated_at)
		 VALUES (?, ?, ?, ?)`,
		n.ObjectHash[:], n.Key, n.Value, n.UpdatedAt,
	)
	return err
}

// GetNote retrieves a single note by object hash and key. Returns nil if not found.
func (s *SQLiteStore) GetNote(ctx context.Context, objectHash types.Hash, key string) (*types.Note, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT object_hash, key, value, updated_at FROM graph_notes WHERE object_hash = ? AND key = ?`,
		objectHash[:], key,
	)
	var n types.Note
	var hashBytes []byte
	if err := row.Scan(&hashBytes, &n.Key, &n.Value, &n.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(n.ObjectHash[:], hashBytes)
	return &n, nil
}

// GetNotes retrieves all notes attached to an object.
func (s *SQLiteStore) GetNotes(ctx context.Context, objectHash types.Hash) ([]types.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT object_hash, key, value, updated_at FROM graph_notes WHERE object_hash = ? ORDER BY key`,
		objectHash[:],
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotes(rows)
}

// GetNotesByKey retrieves all notes with the given key across all objects.
func (s *SQLiteStore) GetNotesByKey(ctx context.Context, key string) ([]types.Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT object_hash, key, value, updated_at FROM graph_notes WHERE key = ? ORDER BY object_hash`,
		key,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotes(rows)
}

// DeleteNote removes a single note by object hash and key.
func (s *SQLiteStore) DeleteNote(ctx context.Context, objectHash types.Hash, key string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM graph_notes WHERE object_hash = ? AND key = ?`,
		objectHash[:], key,
	)
	return err
}

// DeleteNotesByObject removes all notes attached to an object.
func (s *SQLiteStore) DeleteNotesByObject(ctx context.Context, objectHash types.Hash) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM graph_notes WHERE object_hash = ?`,
		objectHash[:],
	)
	return err
}

// CommunitiesForNodes batch-retrieves community_id notes for the given hashes.
// Returns a map from hash to community ID. Hashes without a community_id note
// are omitted from the result.
func (s *SQLiteStore) CommunitiesForNodes(ctx context.Context, hashes []types.Hash) (map[types.Hash]int, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	result := make(map[types.Hash]int, len(hashes))

	// Chunk at 99 for consistency with BatchPutNodes pattern (SQLite variable
	// limit is 999; each placeholder uses 1 variable, plus 1 for the key param).
	const chunkSize = 99
	for i := 0; i < len(hashes); i += chunkSize {
		end := i + chunkSize
		if end > len(hashes) {
			end = len(hashes)
		}
		chunk := hashes[i:end]

		// Build placeholder string: (?, ?, ...)
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)+1)
		args = append(args, "community_id")
		for j, h := range chunk {
			placeholders[j] = "?"
			args = append(args, h[:])
		}

		query := `SELECT object_hash, value FROM graph_notes WHERE key = ? AND object_hash IN (` +
			strings.Join(placeholders, ",") + `)`

		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("CommunitiesForNodes chunk %d: %w", i/chunkSize, err)
		}

		for rows.Next() {
			var hashBytes []byte
			var value string
			if err := rows.Scan(&hashBytes, &value); err != nil {
				rows.Close()
				return nil, fmt.Errorf("CommunitiesForNodes scan: %w", err)
			}
			commID, err := strconv.Atoi(value)
			if err != nil {
				// Skip non-integer values (corrupt data).
				continue
			}
			var h types.Hash
			copy(h[:], hashBytes)
			result[h] = commID
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("CommunitiesForNodes rows: %w", err)
		}
	}

	return result, nil
}

// scanNotes is a shared helper for scanning note rows.
func scanNotes(rows *sql.Rows) ([]types.Note, error) {
	var notes []types.Note
	for rows.Next() {
		var n types.Note
		var hashBytes []byte
		if err := rows.Scan(&hashBytes, &n.Key, &n.Value, &n.UpdatedAt); err != nil {
			return nil, err
		}
		copy(n.ObjectHash[:], hashBytes)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}
