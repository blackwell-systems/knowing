# Architecture Decisions

This document records foundational design decisions for knowing. These choices are made early because they are expensive or impossible to retrofit later.

## 1. Content-Addressed Graph (Merkle DAG)

**Decision:** The graph is a Merkle DAG. Every node, edge, and graph state is identified by its content hash.

**Why:**

Mutable-state graphs (the default in every existing code intelligence tool) lose history, can't detect staleness structurally, and can't prove integrity. A content-addressed graph gets history, staleness, integrity, deduplication, and cache invalidation as emergent properties rather than bolted-on features.

**How it works:**

```
node_hash   = sha256(repo || package_path || content_hash || symbol_name || symbol_kind)
edge_hash   = sha256(source_node_hash || target_node_hash || edge_type || provenance_json)
snapshot    = merkle_root(sorted(all_edge_hashes))
```

A snapshot chain links root hashes like git commits (each snapshot points to its parent). Diffing two snapshots is a Merkle tree comparison: only changed subtrees need traversal.

**What this enables:**

- Point-in-time queries: any previous root hash is a valid query target
- Staleness = hash mismatch: file content hash changed → all derived nodes have stale hashes → edges from those nodes are suspect
- Deterministic verification: same repo at same commit always produces the same graph hash
- Incremental sync: exchange only differing subtrees between machines
- Cache invalidation: query results keyed by root hash; root changes = cache invalidates

**What this costs:**

- More storage than mutable state (old snapshots retained until GC)
- Hash computation on every ingest (SHA-256 is fast; millions of hashes per second)
- Garbage collection policy for old snapshots

**Alternatives considered:**

- Mutable relational tables with `updated_at` timestamps (gortex pattern): loses history, staleness is heuristic
- Append-only log without content addressing: gets history but not integrity or deduplication
- External graph database (Neo4j, Dgraph): too heavy, not embeddable, kills single-binary deployment

## 2. Symbol Identity Scheme

**Decision:** Symbols are identified by a canonical qualified name, and their hash incorporates source content.

**Format:**

```
{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}
```

Examples:
```
github.com/blackwell-systems/mcp-assert://cmd/mcp-assert/main.run
github.com/blackwell-systems/knowing://internal/graph.Graph.AddEdge
github.com/mark3labs/mcp-go://mcp.Tool.InputSchema
```

**Edge cases handled:**

| Case | Resolution |
|------|-----------|
| Methods on types | `package.Type.Method` |
| Interface methods | Same as concrete methods; edge type distinguishes |
| Package-level functions | `package.FunctionName` (no type component) |
| Vendored dependencies | Canonical import path, not vendor path |
| Generated code | Uses the import path seen by consumers, not generator path |
| Same package in multiple repos | `repo://` prefix disambiguates |

**Why this matters:**

Symbol identity is the primary key for every node in the graph. Getting it wrong means edges connect to the wrong symbols, deduplication fails, and cross-repo queries return garbage. Changing the identity scheme later requires full reindex of every repo.

## 3. Append-Only Edge Log (Event Sourcing)

**Decision:** Edges are never mutated in place. New indexing runs produce new edges. Old edges remain with their original timestamp and provenance until garbage collected.

**Schema (conceptual):**

```sql
CREATE TABLE edge_events (
    event_id      INTEGER PRIMARY KEY,
    edge_hash     BLOB NOT NULL,       -- content-addressed
    source_hash   BLOB NOT NULL,       -- node hash
    target_hash   BLOB NOT NULL,       -- node hash
    edge_type     TEXT NOT NULL,        -- call, import, implements, produces, consumes
    event_type    TEXT NOT NULL,        -- 'added' | 'removed'
    snapshot_hash BLOB NOT NULL,        -- which snapshot introduced this event
    source_commit TEXT NOT NULL,        -- git commit that produced this edge
    indexer_ver   TEXT NOT NULL,        -- indexer version that produced this edge
    timestamp     INTEGER NOT NULL      -- unix timestamp
);
```

**Why:**

- "When did this edge first appear?" is a trivial query
- "What changed between deploy A and deploy B?" is a range scan
- Rollback is pointing to an older snapshot, not undoing mutations
- Bug in the indexer? Invalidate all edges from that indexer version without reindexing

**Why hard to retrofit:**

If you start with INSERT/UPDATE/DELETE (mutable state), you can never recover the history. Event sourcing must be the foundation, not an addition.

## 4. Edge Provenance

**Decision:** Every edge carries metadata about how it was derived.

**Fields:**

```json
{
  "source": "ast_resolved",
  "confidence": 1.0,
  "indexer_version": "0.1.0",
  "source_commit": "abc123def",
  "source_file_hash": "sha256:...",
  "timestamp": 1715700000
}
```

**Provenance sources and confidence tiers:**

| Source | Confidence | Meaning |
|--------|-----------|---------|
| `ast_resolved` | 1.0 | Parsed from source with full type resolution |
| `scip_imported` | 0.9 | Imported from SCIP index (external dependency) |
| `lsp_resolved` | 0.9 | Resolved via language server query |
| `config_declared` | 0.8 | Declared in infrastructure config (Terraform, K8s) |
| `inferred_from_import` | 0.7 | Inferred from import statement (no call site found) |
| `openapi_declared` | 0.7 | Declared in OpenAPI/proto spec |
| `text_matched` | 0.3 | Matched by text heuristic (string literal, comment) |
| `manual` | 1.0 | Manually declared by user |

**Why:**

Agents need to know how much to trust an edge. "This function is called by repo X (confidence 1.0, confirmed today)" is different from "this route might be consumed by repo Y (confidence 0.3, text match from 2 weeks ago)."

Without provenance from day 1, old edges are just "edges" with no way to distinguish reliable from speculative.

## 5. Content-Addressed File Identity

**Decision:** Files are identified by `(repo, path, content_hash)`, not by path alone.

**Why:**

- File renamed: same content hash → edges survive the rename automatically
- File copied across repos: same hash → deduplicated in the graph
- File unchanged: hash matches previous → skip re-parse entirely (fast incremental)
- File modified: hash differs → invalidate all nodes derived from old hash

**Implementation:**

On each indexing run, compute `sha256(file_contents)` for each file. Compare against stored hash. Only re-parse files with changed hashes. This makes incremental indexing O(changed files), not O(all files).

## 6. Causal Ordering Across Repos

**Decision:** Use Lamport timestamps (not wall clocks) to establish causal ordering of changes across repositories.

**Why:**

Wall clocks lie. Developer A commits at 3:01 PM (clock 2 minutes fast), developer B commits at 3:02 PM (clock correct). Wall clock says A first, but B may have pushed first. For staleness detection, we need to answer: "Did the consumer update after the producer changed?" This requires causal ordering, not chronological.

**Implementation:**

Each repo maintains a monotonically increasing counter (Lamport clock). When repo A's index triggers a re-index of repo B (because A's export changed and B imports it), B's counter increments past A's. The resulting snapshot records both counters, establishing "B's snapshot was caused by A's change."

**Simplified for v0:** Use git commit timestamps as an approximation. Upgrade to Lamport clocks when multi-repo coordination is implemented.

## 7. Schema Migration Framework

**Decision:** Embed numbered SQL migrations in the binary. Apply on startup.

**Format:**

```
internal/store/migrations/
  001_initial_schema.sql
  002_add_provenance_fields.sql
  003_add_ownership_table.sql
```

**Why:**

The SQLite schema will evolve. Without a migration framework from day 1, the only upgrade path is "delete your graph and reindex everything." With migrations, schema changes are incremental and non-destructive.

**Implementation:**

```go
//go:embed migrations/*.sql
var migrations embed.FS

func Migrate(db *sql.DB) error {
    // read current version from schema_version table
    // apply all migrations > current version in order
    // update schema_version
}
```

## 8. Deterministic Reindexing

**Decision:** Given the same repo at the same commit, the indexer MUST produce byte-identical output (same node hashes, same edge hashes, same snapshot hash).

**Rules:**

- No map iteration in output paths (sort keys first)
- No timestamps in hash inputs (use commit hash, not time)
- No randomness anywhere in the indexing pipeline
- No dependency on indexing order (file A before file B must produce same result as B before A)

**Why:**

- Snapshot tests for indexer correctness (golden files)
- Reproducible bug reports ("at this commit, the graph hash should be X")
- Content-addressing requires determinism (same content = same hash, always)
- Two developers indexing the same repos independently get the same graph

## 9. Storage: SQLite

**Decision:** Use SQLite as the persistent backing store.

**Why:**

- Single file, zero configuration, embedded in the binary
- Handles tens of millions of rows without issues
- WAL mode gives concurrent read/write without blocking
- Known quantity (commitmux already uses SQLite with similar patterns)
- Queryable with standard SQL for debugging
- Backup = copy one file

**Why not:**

- Not a graph database: joins for multi-hop traversal can be slow. Mitigated by materializing common paths (e.g., transitive callers up to N hops) and caching hot queries by root hash.
- Single writer: only one process can write at a time. Acceptable for a daemon model where one process owns the graph.

**Schema sketch (v0):**

```sql
-- Repos tracked by knowing
CREATE TABLE repos (
    repo_hash   BLOB PRIMARY KEY,
    repo_url    TEXT NOT NULL,
    last_commit TEXT,
    last_indexed INTEGER
);

-- Files with content hashes
CREATE TABLE files (
    file_hash    BLOB PRIMARY KEY,
    repo_hash    BLOB NOT NULL REFERENCES repos(repo_hash),
    path         TEXT NOT NULL,
    content_hash BLOB NOT NULL
);

-- Symbols (nodes in the graph)
CREATE TABLE nodes (
    node_hash    BLOB PRIMARY KEY,
    file_hash    BLOB NOT NULL REFERENCES files(file_hash),
    qualified_name TEXT NOT NULL,
    kind         TEXT NOT NULL,  -- function, type, method, interface, const, var
    line         INTEGER,
    signature    TEXT            -- type signature for display
);

-- Relationships (edges in the graph)
CREATE TABLE edges (
    edge_hash    BLOB PRIMARY KEY,
    source_hash  BLOB NOT NULL REFERENCES nodes(node_hash),
    target_hash  BLOB NOT NULL REFERENCES nodes(node_hash),
    edge_type    TEXT NOT NULL,  -- calls, imports, implements, produces, consumes
    confidence   REAL NOT NULL DEFAULT 1.0,
    provenance   TEXT NOT NULL DEFAULT 'ast_resolved'
);

-- Append-only event log
CREATE TABLE edge_events (
    event_id      INTEGER PRIMARY KEY AUTOINCREMENT,
    edge_hash     BLOB NOT NULL,
    event_type    TEXT NOT NULL,  -- added, removed
    snapshot_hash BLOB NOT NULL,
    source_commit TEXT NOT NULL,
    indexer_ver   TEXT NOT NULL,
    timestamp     INTEGER NOT NULL
);

-- Graph snapshots (linked list of root hashes)
CREATE TABLE snapshots (
    snapshot_hash BLOB PRIMARY KEY,
    parent_hash   BLOB REFERENCES snapshots(snapshot_hash),
    repo_hash     BLOB NOT NULL REFERENCES repos(repo_hash),
    commit_hash   TEXT NOT NULL,
    timestamp     INTEGER NOT NULL,
    node_count    INTEGER NOT NULL,
    edge_count    INTEGER NOT NULL
);

-- Schema version tracking
CREATE TABLE schema_version (
    version INTEGER PRIMARY KEY
);

-- Indexes for common query patterns
CREATE INDEX idx_nodes_qualified ON nodes(qualified_name);
CREATE INDEX idx_nodes_file ON nodes(file_hash);
CREATE INDEX idx_edges_source ON edges(source_hash);
CREATE INDEX idx_edges_target ON edges(target_hash);
CREATE INDEX idx_edges_type ON edges(edge_type);
CREATE INDEX idx_edge_events_snapshot ON edge_events(snapshot_hash);
CREATE INDEX idx_files_repo ON files(repo_hash);
```

## 10. Process Model

**Decision:** Persistent daemon with MCP interface.

**Why:**

- The graph must survive between agent invocations (agents start and stop constantly)
- File watching (fsnotify/git hooks) requires a long-lived process
- Multiple agents may query simultaneously (concurrent reads)
- Indexing is background work that shouldn't block queries

**Architecture:**

```
knowing daemon (long-lived)
  ├── Indexer (background, watches for git changes)
  ├── Graph Store (SQLite, WAL mode)
  ├── MCP Server (stdio or HTTP, serves agent queries)
  └── Snapshot Manager (computes roots, GCs old snapshots)
```

**MCP transport:** stdio for single-agent use (Claude Code, Cursor), HTTP for multi-agent or remote access.

## Summary

| Decision | Core principle | Hard to retrofit? |
|----------|---------------|-------------------|
| Content-addressed graph | Integrity, history, staleness are structural | Yes (requires full rewrite of storage) |
| Symbol identity scheme | Stable primary key across all edges | Yes (changing means full reindex) |
| Append-only edge log | Never lose history | Yes (can't recover deleted history) |
| Edge provenance | Trust is quantifiable | Yes (old edges become unknowable) |
| Content-addressed files | Renames don't break edges | Yes (path-keyed edges are unfixable) |
| Causal ordering | Cross-repo ordering is correct | Moderate (can approximate with timestamps initially) |
| Schema migrations | Upgrades don't destroy data | Yes (no migrations = delete and rebuild) |
| Deterministic reindexing | Same input = same output, always | Yes (non-determinism poisons the hash tree) |
| SQLite | Single-binary, embedded, proven | No (storage backend is swappable) |
| Daemon process model | Graph outlives agent sessions | No (can start as CLI, add daemon later) |
