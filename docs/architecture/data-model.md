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
│   ├── task_memory    passive retrieval learning (disabled)
│   ├── vocab_associations  learned keyword -> symbol mappings
│   ├── route_symbols  runtime route-to-symbol mappings
│   └── schema_version migration tracking
│
└── Search layer
    ├── nodes_fts          FTS5 full-text index over nodes
    └── nodes_fts_content  backing content table for FTS
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
    kind           TEXT NOT NULL,     -- function, type, method, interface, const, var, service, route, external, file, package
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

Node kinds: `function`, `type`, `method`, `interface`, `const`, `var` are the standard source symbol kinds. `service` is used for microservice declarations (Protobuf services, Docker Compose services). `route` is used for HTTP/API route declarations (OpenAPI, Serverless, CloudFormation). `external` with `file_hash=EmptyHash` are phantom nodes representing stdlib or external symbols (created by the Go tree-sitter extractor for inferred stdlib targets and by the LSP enricher for remaining dangling targets). `file` and `package` are structural nodes emitted by the Go tree-sitter extractor for file-level and package-level declarations. Phantom nodes make the graph complete: every edge has both a source and a target, and `knowing fsck` reports zero dangling errors on a correctly indexed repo.

### edges

Content-addressed relationships. Identity includes provenance, so the same structural relationship observed by different methods produces distinct edges.

```sql
CREATE TABLE edges (
    edge_hash         BLOB PRIMARY KEY, -- SHA-256("edge\0" || source || target || type || provenance)
    source_hash       BLOB NOT NULL REFERENCES nodes(node_hash),
    target_hash       BLOB NOT NULL REFERENCES nodes(node_hash),
    edge_type         TEXT NOT NULL,    -- calls, imports, implements, references, etc.
    confidence        REAL NOT NULL DEFAULT 1.0,
    provenance        TEXT NOT NULL DEFAULT 'ast_resolved',
    callsite_line     INTEGER,          -- source location of the call/reference
    callsite_col      INTEGER,
    callsite_file     TEXT,
    observation_count INTEGER NOT NULL DEFAULT 0,  -- total observations in current window (0 for static edges)
    last_observed     INTEGER NOT NULL DEFAULT 0,  -- unix timestamp of last observation (0 for static edges)
    indexed_at        INTEGER DEFAULT 0
);
```

Edge types (38 total): `calls`, `imports`, `implements`, `references`, `handles_route`, `depends_on`, `deploys`, `exposes`, `configures`, `publishes`, `subscribes`, `connects_to`, `throws`, `extends`, `overrides`, `decorates`, `owned_by`, `tests`, `authored_by`, `documents`, `consumes_endpoint`, `implements_rpc`, `consumes_rpc`, `gated_by_flag`, `deployed_by`, `tested_by`, `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes`, `contains`, `member_of`, `similar_to`, `co_tested_with`, `type_hint_of`, `accesses_field`, `reads_env`, `executes_process`.

Provenance tiers (ordered by confidence): `ast_resolved` (1.0), `scip_resolved` (0.95), `lsp_resolved` (0.9), `runtime_observed` (0.8), `ast_inferred` (0.7), `otel_trace` (0.2-0.95 based on observation count).

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
    timestamp     INTEGER NOT NULL,
    source_hash   BLOB,             -- full edge data for removed-edge diffs (migration 013)
    target_hash   BLOB,
    edge_type     TEXT,
    confidence    REAL,
    provenance    TEXT
);
```

The `source_hash` through `provenance` columns (migration 013) store full edge data so that removed-edge diffs work without joining back to the edges table (removed edges are deleted from edges). NULL for pre-migration events.

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
    edge_count    INTEGER NOT NULL,
    generation    INTEGER NOT NULL DEFAULT 0  -- chain depth: parent.Generation + 1 (migration 015)
);
```

Generation numbers enable O(1) ancestry checks: "is snapshot A an ancestor of B?" reduces to `A.Generation < B.Generation` when A is on B's chain. This prunes chain walks during diff and GC operations.

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
- `rwr_cache`: cached RWR walk results keyed by seed+weight+Merkle root hash (incremental RWR, session 26)
- `package_roots`: per-package Merkle roots persisted during indexing for vocab/feedback expiration (session 26)
- `quality_score`: node quality annotations for ranking calibration

`BatchPutNotes` wraps multiple inserts in a single prepared-statement transaction (21x faster than individual PutNote calls). `SaveChangedAssignments` writes only the delta (5.0x e2e speedup).

### feedback

Symbol usefulness signals from agent sessions. As of migration 014, feedback records store the `neighborhood_root` (SubgraphRoot of the symbol's package at feedback time) to enable merkleized expiration: feedback becomes invalid when the symbol's package changes (detected via SubgraphRoot mismatch).

```sql
CREATE TABLE feedback (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_hash      BLOB NOT NULL,
    session_id       TEXT NOT NULL,
    useful           INTEGER NOT NULL,  -- 1 = relevant, 0 = noise
    timestamp        INTEGER NOT NULL,
    neighborhood_root BLOB,             -- SubgraphRoot of symbol's package (migration 014)
    keyword_cluster   BLOB              -- keyword cluster hash for scoped feedback (migration 020)
);
CREATE INDEX idx_feedback_neighborhood ON feedback(neighborhood_root);
CREATE INDEX idx_feedback_cluster ON feedback(keyword_cluster);
```

The `keyword_cluster` column (migration 020) scopes feedback to keyword clusters, preventing
cross-task interference. The cluster is derived from sorted primary keywords of the task.
Noise demotion for "checkout" queries doesn't affect "order" queries.

### vocab_associations

Learned keyword -> symbol associations from agent usage (migration 021). When an agent uses
a symbol after a `context_for_task` query, the association is recorded. After 2+ observations,
the association becomes a learned equivalence class with soft RRF injection (confidence-weighted, not forced).

```sql
CREATE TABLE vocab_associations (
    keyword        TEXT NOT NULL,
    symbol_name    TEXT NOT NULL,
    symbol_hash    BLOB NOT NULL,
    count          INTEGER DEFAULT 1,
    last_seen      INTEGER NOT NULL,
    subgraph_root  BLOB,              -- per-package Merkle root at recording time (migration 022)
    UNIQUE(keyword, symbol_hash)
);
CREATE INDEX idx_vocab_keyword ON vocab_associations(keyword);
```

The `subgraph_root` column (migration 022) ties each association to the symbol's package
state at recording time. When querying, associations where `subgraph_root` doesn't match
the current package Merkle root are filtered out. This provides per-package expiration:
when package A changes, only associations for symbols in package A expire.

### task_memory (disabled)

Historical passive retrieval learning. Confirmed neutral in session 24 (task memory
records keywords -> symbols that the pipeline already finds). Creation and recording
disabled in the MCP server. Table preserved for backward compatibility.

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

FTS5 full-text index for BM25 search over symbol names, docstrings, and signatures. BM25 weights: symbol_name=10x, concepts=5x, file_path=4x, qualified_name=3x, doc=3x, signature=1x.

```sql
CREATE VIRTUAL TABLE nodes_fts USING fts5(
    symbol_name, concepts, qualified_name, signature, file_path, doc,
    content='nodes_fts_content',
    content_rowid='rowid',
    tokenize="unicode61 tokenchars '_' remove_diacritics 2"
);
```

The `symbol_name` column (migration 016) stores the terminal identifier extracted by `extractSymbolName`, which strips repo URL, package path, and file extension prefix. The `concepts` column (migration 017) stores CamelCase-split tokens from file names and parent directories (e.g., "commandLineParser.ts" becomes "command Line Parser commandLineParser"), bridging the vocabulary gap between developer terminology and symbol names. `RebuildFTSForPackages` scopes rebuild to changed packages (2.9x faster than full rebuild).

The FTS index is backed by a separate content table (`nodes_fts_content`) that maps rowids to node hashes for result lookup:

```sql
CREATE TABLE nodes_fts_content (
    rowid          INTEGER PRIMARY KEY AUTOINCREMENT,
    node_hash      BLOB NOT NULL,
    symbol_name    TEXT NOT NULL DEFAULT '',
    concepts       TEXT NOT NULL DEFAULT '',
    qualified_name TEXT NOT NULL,
    signature      TEXT NOT NULL DEFAULT '',
    file_path      TEXT NOT NULL DEFAULT ''
);
```

### route_symbols

Runtime route-to-symbol mappings. Maps HTTP routes, RPC methods, and message queue topics to graph nodes, enabling the trace ingestor to resolve OpenTelemetry spans to graph symbols.

```sql
CREATE TABLE route_symbols (
    service_name  TEXT NOT NULL,
    route_pattern TEXT NOT NULL,
    node_hash     BLOB NOT NULL,
    mapping_type  TEXT NOT NULL,   -- "http", "rpc", "messaging"
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (service_name, route_pattern, mapping_type)
);
```

## Merkle Tree (Computed, Not Stored)

The hierarchical Merkle tree is computed in memory from the edge table, not stored in SQLite. Tree construction delegates to `github.com/blackwell-systems/merkle-strata` v0.4.0: `BuildMerkleTree` calls `strata.Build` and `BuildHierarchicalTree` calls `strata.BuildMultiLevel`. The `merkle\0` hash domain prefix is passed via `strata.WithPrefix`. `BuildHierarchicalTree` constructs:

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
| 013 | add_edge_event_data.sql | source_hash, target_hash, edge_type, confidence, provenance on edge_events (removed-edge diffs) |
| 014 | add_neighborhood_root.sql | neighborhood_root on feedback (merkleized expiration) |
| 015 | snapshot_generation.sql | generation column on snapshots (O(1) ancestry checks) |
| 016 | fts_symbol_name.sql | Adds symbol_name column to FTS content table; recreates FTS5 virtual table with 4 columns (symbol_name, qualified_name, signature, file_path) |
| 017 | fts_concepts_column.sql | Adds concepts column to FTS content table; stores CamelCase-split file/module names as searchable concepts; recreates FTS5 virtual table with 5 columns |
| 018 | fts_doc_column.sql | Adds doc column to FTS content/virtual table for docstring-based BM25 retrieval; recreates FTS5 with 6 columns (symbol_name, concepts, qualified_name, signature, file_path, doc) |
| 019 | add_embeddings.sql | Embeddings table for vector cache (keyed by node_hash + model) |
| 020 | add_feedback_cluster.sql | keyword_cluster column on feedback table for per-cluster scoping; prevents cross-task interference |
| 021 | add_vocab_associations.sql | vocab_associations table for learned keyword -> symbol mappings |
| 022 | add_vocab_subgraph_root.sql | subgraph_root column on vocab_associations for per-package Merkle expiration |

Migrations run automatically on `NewSQLiteStore`. Each runs in its own transaction. Schema version is tracked in `schema_version` table (current version: 22). No rollback/down migrations.

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

All database access goes through `types.GraphStore` (39 methods). `SQLiteStore` is the sole implementation. The interface exists so:
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
- `CommunitiesForNodes`: batch community_id lookups
- `TruncateGraph`: delete all nodes, edges, and edge_events (used by reindex)
- `DeleteRepoData`: atomic eviction of all data for a repo (files, nodes, edges, edge_events, snapshots, feedback, task_memory, graph_notes) in a single transaction; returns `DeleteRepoResult` with counts of deleted rows per table
- `InvalidateCache`: clears in-process node/edge caches
- `DB()`: raw access for task memory and feedback queries

## Why SQLite

- Single file: the database IS the artifact. Copy it, share it, verify it.
- WAL mode: concurrent readers during daemon indexing.
- Embedded: no external service to configure, manage, or secure.
- PRAGMA integrity_check: filesystem-level corruption detection built in.
- Pure Go driver (modernc.org/sqlite): no CGo, cross-compiles to all platforms.
- Fast enough: 98ms fsck on 7,224 nodes + 24,936 edges. 72us proof generation. 42ns cache lookup.

### Performance Pragmas

On connection open, `NewSQLiteStore` sets:

| Pragma | Value | Rationale |
|--------|-------|-----------|
| `journal_mode` | WAL | Concurrent readers, no blocking on writes |
| `synchronous` | NORMAL | Safe with WAL (fsync on checkpoint, not every commit) |
| `mmap_size` | 256 MB | Memory-mapped I/O for read-heavy workloads |
| `cache_size` | 64 MB (negative KB) | Large page cache for hot-path traversals |
| `busy_timeout` | 5000 ms | Retry on lock contention instead of immediate SQLITE_BUSY |
| `temp_store` | MEMORY | Temp tables and indexes kept in memory |
