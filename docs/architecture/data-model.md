# Data Model

knowing uses SQLite in WAL mode as its sole storage backend. Every table serves one of two roles: **identity layer** (content-addressed entities whose hashes form the Merkle tree) or **metadata layer** (derived state that never affects Merkle computation).

## Storage Architecture

```
SQLite database (one per repo, at ~/.knowing/repos/<safe-name>.db)
├── Identity layer (affects Merkle tree)
│   ├── repos          canonical repo identity
│   ├── files          content-addressed file records
│   ├── nodes          content-addressed symbol declarations
│   ├── edges          content-addressed relationships
│   ├── edge_events    append-only mutation log
│   └── snapshots      Merkle root chain
│
├── Metadata layer (never affects Merkle tree)
│   ├── graph_notes    general-purpose key/value annotations
│   ├── feedback       symbol usefulness signals
│   ├── task_memory    passive retrieval learning
│   └── schema_version migration tracking
│
└── Search layer
    └── nodes_fts      FTS5 full-text index over nodes
```

The distinction matters: modifying a note, recording feedback, or updating task memory never changes any hash and never invalidates any Merkle proof or cache key. The identity layer is the audit surface; the metadata layer is the learning surface.

## Tables

### repos

Tracked repositories. One row per indexed repo.

```sql
CREATE TABLE repos (
    repo_hash    BLOB PRIMARY KEY,   -- SHA-256(canonical repo URL)
    repo_url     TEXT NOT NULL,       -- canonical URL or local path
    last_commit  TEXT,                -- git commit hash from most recent index
    last_indexed INTEGER              -- unix timestamp
);
```

### files

Content-addressed source files. Hash includes repo, path, and content.

```sql
CREATE TABLE files (
    file_hash    BLOB PRIMARY KEY,   -- SHA-256(repo_hash || path || content_hash)
    repo_hash    BLOB NOT NULL REFERENCES repos(repo_hash),
    path         TEXT NOT NULL,       -- relative to repo root
    content_hash BLOB NOT NULL        -- SHA-256(raw file bytes)
);
```

### nodes

Content-addressed symbol declarations. Identity depends on (repo, package, name, kind), not physical location.

```sql
CREATE TABLE nodes (
    node_hash      BLOB PRIMARY KEY, -- SHA-256("node\0" || repo || package || name || kind)
    file_hash      BLOB NOT NULL REFERENCES files(file_hash),
    qualified_name TEXT NOT NULL,     -- "repoURL://package/path.SymbolName"
    kind           TEXT NOT NULL,     -- function, type, method, interface, var, const
    line           INTEGER,
    signature      TEXT,              -- function/method signature
    doc            TEXT,              -- extracted doc comment
    last_author    TEXT,              -- git blame: last author
    last_commit_at INTEGER,           -- git blame: last commit timestamp
    coverage_pct   REAL,              -- test coverage percentage
    indexed_at     INTEGER DEFAULT 0  -- unix timestamp of last index run
);
```

Moving a function between files does not change its hash (identity is logical, not physical). Renaming it creates a new node (old node's edges become stale, detectable via snapshot diff).

### edges

Content-addressed relationships. Identity includes provenance, so the same structural relationship observed by different methods produces distinct edges.

```sql
CREATE TABLE edges (
    edge_hash    BLOB PRIMARY KEY, -- SHA-256("edge\0" || source || target || type || provenance)
    source_hash  BLOB NOT NULL REFERENCES nodes(node_hash),
    target_hash  BLOB NOT NULL REFERENCES nodes(node_hash),
    edge_type    TEXT NOT NULL,    -- calls, imports, implements, references, etc.
    confidence   REAL NOT NULL DEFAULT 1.0,
    provenance   TEXT NOT NULL DEFAULT 'ast_resolved',
    callsite_line INTEGER,         -- source location of the call/reference
    callsite_col  INTEGER,
    callsite_file TEXT,
    indexed_at   INTEGER DEFAULT 0
);
```

Edge types: `calls`, `imports`, `implements`, `references`, `handles_route`, `depends_on`, `deploys`, `exposes`, `configures`, `publishes`, `subscribes`, `connects_to`, `throws`, `extends`, `overrides`, `decorates`, `owned_by`, `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes`.

Provenance tiers: `ast_inferred` (0.7), `lsp_resolved` (0.9), `scip_resolved` (0.95), `ast_resolved` (1.0), `otel_trace` (0.2-0.95 based on observation count).

### edge_events

Append-only log of edge mutations. Powers snapshot diff and temporal queries.

```sql
CREATE TABLE edge_events (
    event_id      INTEGER PRIMARY KEY AUTOINCREMENT,
    edge_hash     BLOB NOT NULL,
    event_type    TEXT NOT NULL,     -- "added" or "removed"
    snapshot_hash BLOB NOT NULL,
    source_commit TEXT NOT NULL,
    indexer_ver   TEXT NOT NULL,
    timestamp     INTEGER NOT NULL
);
```

### snapshots

Point-in-time graph state. Each snapshot is a hierarchical Merkle root computed from all edges, tied to a git commit. Snapshots form a singly-linked chain via `parent_hash`.

```sql
CREATE TABLE snapshots (
    snapshot_hash BLOB PRIMARY KEY, -- ComputeSnapshotHash(hierarchical_merkle_root)
    parent_hash   BLOB REFERENCES snapshots(snapshot_hash),
    repo_hash     BLOB NOT NULL REFERENCES repos(repo_hash),
    commit_hash   TEXT NOT NULL,
    timestamp     INTEGER NOT NULL,
    node_count    INTEGER NOT NULL,
    edge_count    INTEGER NOT NULL
);
```

### graph_notes

General-purpose metadata layer (Phase 3 F1). Attaches key/value pairs to any content-addressed object without affecting Merkle computation. Composite primary key: one value per key per object.

```sql
CREATE TABLE graph_notes (
    object_hash BLOB    NOT NULL,   -- any hash: node, edge, snapshot, community, pack root
    key         TEXT    NOT NULL,   -- "community_id", "context_pack", "quality_score", etc.
    value       TEXT    NOT NULL,   -- opaque to the store; callers may use JSON
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (object_hash, key)
);
```

Current uses:
- `community_id`: persisted community assignments for incremental detection
- `context_pack`: persisted context blocks for cross-session replay

`BatchPutNotes` wraps multiple inserts in a single prepared-statement transaction (21x faster than individual PutNote calls). `SaveChangedAssignments` writes only the delta (5.0x e2e speedup).

### feedback

Symbol usefulness signals from agent sessions.

```sql
CREATE TABLE feedback (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_hash  BLOB NOT NULL,
    session_id   TEXT NOT NULL,
    useful       INTEGER NOT NULL,  -- 1 = relevant, 0 = noise
    timestamp    INTEGER NOT NULL
);
```

### task_memory

Passive retrieval learning. Records which symbols were returned for which keywords.

```sql
CREATE TABLE task_memory (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    keywords    TEXT NOT NULL,
    symbol_hash BLOB NOT NULL,
    score       REAL NOT NULL,
    timestamp   INTEGER NOT NULL
);
```

### nodes_fts

FTS5 full-text index for BM25 search over symbol names and signatures.

```sql
CREATE VIRTUAL TABLE nodes_fts USING fts5(
    node_hash, qualified_name, signature, file_path,
    content=nodes_fts_content
);
```

`RebuildFTSForPackages` scopes rebuild to changed packages (2.9x faster than full rebuild).

## Merkle Tree (Computed, Not Stored)

The hierarchical Merkle tree is computed in memory from the edge table, not stored in SQLite. `BuildHierarchicalTree` constructs:

```
repo_root = merkle(sorted(package_roots))
  package_root = merkle(sorted(edge_type_roots for this package))
    edge_type_root = merkle(sorted(edge_hashes of this type in this package))
      leaf = edge_hash
```

This is recomputed on every `ComputeSnapshot` call. The tree lives in memory for the lifetime of the daemon process. Package roots and edge-type roots are keyed by string ("package_path" and "package_path:edge_type").

The tree is not stored because:
1. It's fast to compute (~3ms for 12K edges)
2. Storing it would add a sync problem (tree must match edges exactly)
3. The SubgraphCache and notes table handle persistence of derived results

When the tree needs to be larger than memory (lazy materialization), the roots and structure would move to SQLite. This is not needed at current scale.

## Migration History

| # | File | What it adds |
|---|------|-------------|
| 001 | initial_schema.sql | repos, files, nodes, edges, edge_events, snapshots, all indexes |
| 002 | add_dangling_edge_support.sql | (index for dangling edge queries) |
| 003 | add_callsite_columns.sql | callsite_line, callsite_col, callsite_file on edges |
| 004 | add_runtime_columns.sql | observation_count, last_observed on edges; route_symbols table |
| 005 | add_feedback_table.sql | feedback table |
| 006 | add_fts5_index.sql | nodes_fts virtual table + content table |
| 007 | add_node_doc.sql | doc column on nodes |
| 008 | add_task_memory.sql | task_memory table |
| 009 | add_blame_columns.sql | last_author, last_commit_at on nodes |
| 010 | add_coverage_column.sql | coverage_pct on nodes |
| 011 | add_indexed_at.sql | indexed_at on nodes and edges |
| 012 | add_notes.sql | graph_notes table |

Migrations run automatically on `NewSQLiteStore`. Each runs in its own transaction. Schema version is tracked in `schema_version` table. No rollback/down migrations.

## Per-Repo Isolation

Each repository gets its own SQLite database at `~/.knowing/repos/<safe-name>.db`. This means:
- Community detection operates on one repo's data (no cross-repo noise)
- RWR, HITS, BM25 scores are repo-scoped
- Databases can be backed up, shared, or deleted independently
- Cross-repo edges are planned via a separate resolution layer

The roster (`~/.knowing/roster.json`) maps repo paths to database paths.

## Cross-Repo Edges

When repo A calls a function in repo B, the edge's `target_hash` points to a node that lives in repo B's database, not repo A's. This creates a "dangling edge" in repo A's graph: an edge whose target does not exist locally.

The resolution pipeline (`internal/resolver/`):
1. `DanglingEdges()` finds all edges whose target hash has no matching node.
2. `AllRepos()` lists all known repos.
3. For each dangling edge, search other repos' node tables for a matching qualified name.
4. If found, retarget the edge to the correct node hash in the foreign repo.
5. The edge is resolved; the cross-repo relationship is now explicit.

This works because content-addressing provides global identity without coordination: `SHA-256("node\0" + repoURL + package + name + kind)` produces the same hash regardless of which machine computes it. Two indexers running on different repos produce matching hashes for the same symbol.

Current state:
- Cross-repo edges are resolved within a single knowing instance that has indexed multiple repos.
- Each repo has its own database; the resolver queries across databases.
- `ModuleToRepoURL` mapping (from go.mod) helps the Go extractor target cross-repo calls correctly at extraction time.
- Federated sync (exchanging edges between separate knowing instances) is planned (Phase 4 roadmap).

### Multi-Repo Indexing Workflow

```bash
# Register and index multiple repos
knowing add ./repo-a
knowing add ./repo-b

# Each gets its own database
# ~/.knowing/repos/repo-a.db
# ~/.knowing/repos/repo-b.db

# Cross-repo edges are resolved when both repos are indexed
# The MCP server can query across repos via cross_repo_callers tool
```

### Identity Agreement

Two knowing instances that have never communicated will produce the same hash for the same symbol, as long as they use the same canonical repo URL. This is why canonicalization is core infrastructure: `github.com/org/repo` and `https://github.com/org/repo.git` must resolve to the same canonical identity for cross-repo edges to match.

`ExtractPackagePath` (the canonical package path extractor) and `ComputeNodeHash` (the canonical node hash) are the two functions that define cross-repo identity. Both are deterministic, and both use the `"node\0"` domain prefix to prevent cross-type collisions.

## GraphStore Interface

All database access goes through `types.GraphStore` (33 methods). `SQLiteStore` is the sole implementation. The interface exists so:
- Tests can use mock stores
- Future backends (Pebble, remote) can implement the same interface
- The daemon, MCP server, CLI, and context engine all consume the same abstraction

Non-interface methods on `SQLiteStore` (accessed via type assertion):
- `BatchPutNodes`, `BatchPutEdges`, `BatchPutFiles`: bulk insert in single transaction
- `BatchPutNotes`: bulk note insert (21x faster)
- `RebuildFTS`, `RebuildFTSForPackages`: FTS index management
- `SearchBM25Nodes`: full-text search
- `IntegrityCheck`: PRAGMA integrity_check
- `UpdateNodeBlame`, `UpdateNodeCoverage`: enrichment stamping
- `DB()`: raw access for task memory and feedback queries

## Why SQLite

- Single file: the database IS the artifact. Copy it, share it, verify it.
- WAL mode: concurrent readers during daemon indexing.
- Embedded: no external service to configure, manage, or secure.
- PRAGMA integrity_check: filesystem-level corruption detection built in.
- Pure Go driver (modernc.org/sqlite): no CGo, cross-compiles to all platforms.
- Fast enough: 98ms fsck on 2,338 nodes + 11,664 edges. 72us proof generation. 42ns cache lookup.
