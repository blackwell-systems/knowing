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

**Initial implementation:** Use git commit timestamps as an approximation. Upgrade to Lamport clocks when multi-repo coordination is implemented.

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

**Schema:**

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

## 10. Storage Interface (Backend Swappability)

**Decision:** All graph operations go through an abstract `GraphStore` interface. SQLite is the first (and currently only) implementation. The rest of the system never touches SQL directly.

**Interface:**

```go
package store

// Hash is a content-addressed identifier (SHA-256).
type Hash [32]byte

// GraphStore defines the operations the graph engine requires from its
// backing store. SQLite implements this today; an adjacency-list or
// external graph backend can implement it tomorrow without changing
// callers.
type GraphStore interface {
    // --- Writes (called by the indexer) ---

    PutNode(ctx context.Context, n Node) error
    PutEdge(ctx context.Context, e Edge) error
    PutFile(ctx context.Context, f File) error
    RecordEdgeEvent(ctx context.Context, ev EdgeEvent) error
    CreateSnapshot(ctx context.Context, s Snapshot) error

    // --- Reads (called by MCP query handlers) ---

    GetNode(ctx context.Context, hash Hash) (*Node, error)
    GetEdge(ctx context.Context, hash Hash) (*Edge, error)
    GetSnapshot(ctx context.Context, hash Hash) (*Snapshot, error)

    // NodesByName returns all nodes matching a qualified name prefix.
    // Used for symbol search ("find all symbols named X across repos").
    NodesByName(ctx context.Context, qualifiedPrefix string) ([]Node, error)

    // EdgesFrom returns all outbound edges from a node (calls, imports, etc.).
    EdgesFrom(ctx context.Context, sourceHash Hash, edgeType string) ([]Edge, error)

    // EdgesTo returns all inbound edges to a node (callers, importers, etc.).
    EdgesTo(ctx context.Context, targetHash Hash, edgeType string) ([]Edge, error)

    // --- Graph traversal ---

    // TransitiveCallers walks inbound call edges from target up to maxDepth
    // hops, returning each caller with its distance. The snapshot parameter
    // scopes the query to edges that existed at that point in time.
    // Implementations may use recursive CTEs, materialized closures, or
    // adjacency-list scans depending on the backend.
    TransitiveCallers(ctx context.Context, target Hash, maxDepth int, snapshot Hash) ([]CallerResult, error)

    // TransitiveCallees walks outbound call edges (the inverse direction).
    TransitiveCallees(ctx context.Context, source Hash, maxDepth int, snapshot Hash) ([]CalleeResult, error)

    // BlastRadius computes the full impact set for a proposed change:
    // all transitive callers, grouped by repo and annotated with edge
    // provenance. This is the primary query agents use before editing.
    BlastRadius(ctx context.Context, target Hash, snapshot Hash) (*BlastRadiusResult, error)

    // --- Snapshot operations ---

    // SnapshotDiff returns edges added and removed between two snapshots.
    SnapshotDiff(ctx context.Context, oldRoot, newRoot Hash) (*DiffResult, error)

    // StaleEdges returns edges whose source nodes have content hashes
    // that no longer match the current file content hash.
    StaleEdges(ctx context.Context, snapshot Hash) ([]Edge, error)

    // LatestSnapshot returns the most recent snapshot for a repo.
    LatestSnapshot(ctx context.Context, repoHash Hash) (*Snapshot, error)

    // --- Lifecycle ---

    Close() error
}

// CallerResult is a node with its distance from the query target.
type CallerResult struct {
    Node  Node
    Depth int
}

// CalleeResult is a node with its distance from the query source.
type CalleeResult struct {
    Node  Node
    Depth int
}

// BlastRadiusResult groups transitive callers by repository and includes
// provenance so agents can assess confidence.
type BlastRadiusResult struct {
    Target     Node
    ByRepo     map[string][]CallerWithProvenance // repo URL -> callers
    TotalCount int
    Truncated  bool // true if depth limit was hit
}

// CallerWithProvenance pairs a caller node with the edge provenance chain
// that connects it to the target.
type CallerWithProvenance struct {
    Caller     Node
    Depth      int
    Confidence float64 // minimum confidence along the path
    Provenance []EdgeProvenance
}
```

**Why an interface, not just "use SQLite":**

SQLite is the right initial backend. But the system's most expensive queries (transitive callers, blast radius) are graph traversals implemented as recursive CTEs in SQL. This works for graphs up to roughly 1M edges. Beyond that, an adjacency-list backend (edges stored by node prefix so neighbors are physically co-located) turns joins into sequential reads.

The interface lets us:

- Ship on SQLite with zero operational complexity
- Benchmark against real multi-repo graphs to find the actual pain point
- Swap to an adjacency-list backend (BadgerDB, Pebble, custom) for traversal-heavy workloads without changing the indexer, MCP server, or snapshot logic
- Run both backends in tests to verify behavioral equivalence

**What stays in the interface vs. what stays in the backend:**

| Concern | Where it lives |
|---------|---------------|
| Hash computation | Caller (indexer computes hashes before calling `Put*`) |
| Merkle root computation | Snapshot manager (computes root, passes to `CreateSnapshot`) |
| Traversal strategy (CTE vs. adjacency scan) | Backend implementation |
| Caching (L1 in-memory, L2 materialized closures) | Backend implementation |
| Query depth limits | Caller passes `maxDepth`; backend respects it |
| Provenance filtering | Caller can post-filter; backend may optimize |

**Cache layer (built into the SQLite backend, not the interface):**

The content-addressed design makes query caching structurally correct: results keyed by `(query_params, snapshot_root_hash)` are guaranteed valid until the root changes. The SQLite backend implements two cache tiers internally:

- **L1 (in-memory):** LRU cache of traversal results keyed by `(target_hash, query_type, snapshot_root)`. Serves repeat queries without touching disk. Invalidated when a new snapshot is created.
- **L2 (materialized closures):** For high-fan-in symbols, the backend precomputes and stores transitive caller sets in a `transitive_callers` table. Recomputation is triggered only when the Merkle diff between snapshots touches the relevant subgraph.

Other backends implement their own caching strategies (or none, if adjacency-list traversal is fast enough without it).

**Hard to retrofit?** No. The interface is a clean boundary that can be introduced at any point before the first beta. But defining it now ensures no SQL leaks into the indexer, MCP handlers, or snapshot logic during development.

## 11. Process Model

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

## 12. Traversal Cache

**Decision:** Graph traversal results are cached in two tiers, with invalidation driven by content-addressed hash comparison rather than TTLs or heuristics.

**Why this is different from normal caching:**

Most cache invalidation is a guessing game: TTL-based expiry hopes data hasn't changed, event-driven invalidation hopes no events were missed, version counters hope nothing incremented out of band. Content-addressed storage eliminates guessing. A query result computed against snapshot root `R` is valid for all time; when a new snapshot `R'` is created, the Merkle diff between `R` and `R'` identifies exactly which subtrees changed. Only cached results that touch changed subtrees are invalidated. Everything else remains valid without re-verification.

**L1: In-Memory LRU (Process-Scoped)**

```go
// CacheKey uniquely identifies a traversal result.
type CacheKey struct {
    TargetHash   Hash
    QueryType    string // "transitive_callers", "transitive_callees", "blast_radius"
    MaxDepth     int
    SnapshotRoot Hash
}

// L1Cache is an LRU cache of traversal results held in the daemon process.
// Sized by entry count (not bytes) since traversal results are small
// relative to the graph.
type L1Cache struct {
    mu       sync.RWMutex
    entries  map[CacheKey]*cacheEntry
    lru      *list.List
    maxSize  int // default: 10,000 entries
}
```

**Behavior:**

- Keyed by `(target_hash, query_type, max_depth, snapshot_root)`. Same query against the same snapshot always returns the same result; no recomputation needed.
- On snapshot creation, the daemon computes the Merkle diff and evicts only entries whose target node (or any node in the cached result set) falls within a changed subtree. Entries outside the diff survive across snapshots.
- If memory pressure is a concern, the LRU evicts least-recently-used entries. But eviction is a performance choice, never a correctness one: re-querying always produces the same answer.
- Cache hit rate is expected to be high because agents typically query the same symbols repeatedly during a single editing session, and most snapshots change a small fraction of the graph.

**L2: Materialized Transitive Closures (SQLite, Persisted)**

For high-fan-in symbols (utility functions, shared interfaces, popular library exports), precompute the full transitive caller set and store it in the database:

```sql
CREATE TABLE transitive_callers (
    target_hash    BLOB NOT NULL,
    caller_hash    BLOB NOT NULL,
    depth          INTEGER NOT NULL,
    min_confidence REAL NOT NULL,     -- lowest confidence along the path
    snapshot_hash  BLOB NOT NULL,
    computed_at    INTEGER NOT NULL,  -- unix timestamp (for GC, not invalidation)
    PRIMARY KEY (target_hash, caller_hash, snapshot_hash)
);

CREATE INDEX idx_tc_snapshot ON transitive_callers(snapshot_hash);
CREATE INDEX idx_tc_target ON transitive_callers(target_hash, snapshot_hash);
```

**When to materialize:**

Not every symbol needs a precomputed closure. The decision is based on fan-in:

```go
// MaterializationPolicy determines which symbols get precomputed closures.
type MaterializationPolicy struct {
    // Symbols with more than this many direct callers get materialized.
    // Default: 50. At this threshold, recursive CTE cost is noticeable
    // and the symbol is likely to be queried again.
    FanInThreshold int

    // Maximum depth for materialized closures.
    // Default: 5. Covers the vast majority of practical blast-radius queries.
    MaxDepth int

    // Maximum number of materialized symbols per snapshot.
    // Prevents runaway storage for graphs where many symbols are popular.
    // Default: 1,000.
    MaxMaterialized int
}
```

**Materialization lifecycle:**

1. After a new snapshot is created, the daemon identifies candidate symbols (direct caller count > threshold).
2. For each candidate, check whether the Merkle diff touches any node in its existing closure. If not, the existing materialization is still valid; skip it.
3. For invalidated or new candidates, run the recursive CTE once and write the result to `transitive_callers`. This is background work; it doesn't block queries.
4. Queries check L2 before falling back to a live CTE. An L2 miss is always safe; it just means the query runs the CTE directly.
5. Old materializations (from snapshots that have been garbage collected) are cleaned up by the snapshot GC process.

**L3: Bounded Traversal with Early Termination**

For interactive queries where latency matters more than completeness:

```go
// TraversalOptions controls how deep and wide a traversal goes.
type TraversalOptions struct {
    MaxDepth    int  // hard cap on hops (default: 5)
    MaxResults  int  // stop after collecting this many callers (default: 500)
    MinConfidence float64 // prune paths below this confidence (default: 0.0)
}
```

When either limit is hit, the result includes `Truncated: true` and the agent can decide whether to request a deeper traversal. This keeps the common case (2-3 hops, narrow fan-out) fast regardless of graph size, and avoids accidentally traversing the entire graph for a symbol at the root of a deep call tree.

**Query resolution order:**

```
1. Check L1 (in-memory LRU) for exact key match
   → hit: return immediately
2. Check L2 (materialized closure) for target + snapshot match
   → hit: filter by depth/confidence, populate L1, return
3. Run live recursive CTE with TraversalOptions bounds
   → populate L1 (and L2 if fan-in exceeds threshold), return
```

**What this costs:**

- L1: memory proportional to cache size (10K entries ~= tens of MB). Freed on daemon restart.
- L2: disk proportional to materialized closures. For 1,000 symbols with average closure size of 200, that's ~200K rows. Negligible for SQLite.
- Background materialization: CPU time after each snapshot. Bounded by `MaxMaterialized` and skipped for unchanged subtrees. Expected to complete in under a second for typical snapshot diffs.

**What this does NOT do:**

- Does not cache node/edge lookups. Those are primary key lookups in SQLite, already O(1). Caching them would add complexity for no measurable gain.
- Does not cache across daemon restarts. L1 is process-scoped. L2 persists but is tied to specific snapshots; after restart, the daemon revalidates against the current snapshot. This is a deliberate choice: cold start should be fast (L2 is already on disk), and L1 warms up quickly during normal use.
- Does not attempt distributed caching. If multiple machines run knowing, each maintains its own cache. Cross-machine sync is handled at the snapshot level (Merkle diff exchange), not the cache level.

**Hard to retrofit?** No. The cache is an optimization layer inside the storage backend. The `GraphStore` interface doesn't expose cache details; callers just call `TransitiveCallers` or `BlastRadius` and get results. The cache can be added, tuned, or removed without changing any code outside the SQLite backend implementation.

## 13. Runtime Trace Ingestion

**Decision:** knowing ingests runtime observability data (OpenTelemetry traces, production call graphs, traffic logs) as first-class edges alongside statically-derived edges. Runtime edges use the same content-addressed storage, provenance model, and snapshot chain as static edges.

**Why:**

Static analysis has a ceiling. It can tell you that service A imports a client for service B, but not whether that client is actually called in production. It can tell you a proto field exists, but not whether any consumer reads it. It can parse an HTTP route declaration, but not whether any traffic hits it.

The gap between "statically possible" and "actually happens at runtime" is where false positives live. An agent deciding whether to deprecate a route needs to know if it has real traffic, not just whether something somewhere might construct a request to it.

No existing code intelligence tool bridges this gap. Code search operates on text. Language servers operate on types. Dependency graphs operate on declarations. None of them know what the system actually does. Runtime trace ingestion gives knowing ground truth.

**What gets ingested:**

| Source | Edge type | Example |
|--------|-----------|---------|
| OpenTelemetry spans | `runtime_calls` | Service A's handler called service B's `/api/users` endpoint 14,000 times yesterday |
| gRPC trace metadata | `runtime_rpc` | Service A invoked `UserService.GetUser` on service B |
| Message queue traces | `runtime_produces`, `runtime_consumes` | Service A published to topic X, service B consumed from topic X |
| Database query logs | `runtime_queries` | Service A executed queries against table `users` in database Y |
| HTTP access logs | `runtime_http` | Client C made 500 requests to `GET /api/v2/billing` on service D |

**Provenance and confidence:**

Runtime edges use the existing provenance model with new source types:

```json
{
  "source": "otel_trace",
  "confidence": 0.95,
  "sample_count": 14000,
  "first_seen": "2026-05-01T00:00:00Z",
  "last_seen": "2026-05-14T12:00:00Z",
  "trace_ids": ["abc123", "def456"],
  "indexer_version": "0.3.0"
}
```

Confidence for runtime edges is based on observation strength:

| Condition | Confidence |
|-----------|-----------|
| > 1,000 observations in the last 7 days | 0.95 |
| 100-1,000 observations in the last 7 days | 0.85 |
| 10-100 observations in the last 7 days | 0.7 |
| < 10 observations in the last 7 days | 0.5 |
| No observations in the last 30 days | 0.2 (edge marked stale) |
| No observations in the last 90 days | Edge eligible for GC |

**Architecture:**

```
+-------------------+     +-------------------+     +-------------------+
| OpenTelemetry     |     | Message Queue     |     | HTTP Access       |
| Collector/OTLP    |     | Trace Logs        |     | Logs              |
+---------+---------+     +---------+---------+     +---------+---------+
          |                         |                         |
          v                         v                         v
+---------+---------+---------+---------+---------+---------+-+
|                  Trace Ingest Pipeline                       |
|  (normalizes spans/logs into source/target symbol pairs,     |
|   deduplicates, aggregates counts, computes confidence)      |
+------------------------------+-------------------------------+
                               |
                               v
                +--------------+--------------+
                |   GraphStore.PutEdge()      |
                |   (same interface as static  |
                |    edges, different          |
                |    provenance source)        |
                +--------------+--------------+
                               |
                               v
                +--------------+--------------+
                |   Content-Addressed Graph   |
                |   (runtime + static edges   |
                |    coexist, queryable        |
                |    together or filtered)     |
                +-----------------------------+
```

**The hard part: symbol resolution.**

A trace span says: "service `auth-service` called `POST /api/v2/users` on service `user-service`." The graph stores symbols like `github.com/org/user-service://internal/api.UserHandler.Create`. Connecting the two requires mapping runtime identifiers (service names, route paths, RPC method names) to graph symbols.

This mapping is built during static indexing: when the indexer parses a route registration (`router.POST("/api/v2/users", handler.Create)`), it records a mapping from the runtime route to the graph symbol. The trace ingest pipeline joins against this mapping to resolve span endpoints to node hashes.

Where no mapping exists (the route was registered dynamically, or the service isn't indexed), the edge is created with provenance `runtime_unresolved` and confidence 0.3. It's still useful ("something calls this endpoint") but flagged as needing static confirmation.

**Ingest interface (extends GraphStore):**

```go
// TraceIngestor converts raw observability data into graph edges.
type TraceIngestor interface {
    // IngestSpans processes a batch of OpenTelemetry spans and creates
    // runtime edges. Returns the number of new edges created and the
    // number of existing edges whose observation counts were updated.
    IngestSpans(ctx context.Context, spans []TraceSpan) (created, updated int, err error)

    // IngestHTTPLogs processes access log entries.
    IngestHTTPLogs(ctx context.Context, entries []HTTPLogEntry) (created, updated int, err error)

    // RuntimeEdgeStats returns aggregated statistics for runtime edges:
    // total count, breakdown by source type, staleness distribution.
    RuntimeEdgeStats(ctx context.Context, snapshot Hash) (*RuntimeStats, error)
}

// TraceSpan is a normalized representation of a single span from any
// tracing system (OpenTelemetry, Jaeger, Zipkin). The ingest pipeline
// normalizes vendor-specific formats into this before processing.
type TraceSpan struct {
    TraceID       string
    SpanID        string
    ParentSpanID  string
    ServiceName   string            // source service
    OperationName string            // RPC method, HTTP route, queue topic
    PeerService   string            // target service (if known)
    Attributes    map[string]string // http.method, http.route, rpc.service, etc.
    StartTime     time.Time
    Duration      time.Duration
}
```

**What this enables that nothing else can:**

- "Is this route actually used in production?" (runtime edge exists with recent observations)
- "Which services *actually* call this function, not just which ones *could*?" (filter edges by provenance `runtime_*`)
- "This proto field has 0 runtime reads in the last 90 days; safe to deprecate" (absence of runtime edges is signal)
- "Static analysis says 47 callers; runtime says 3 are active. Focus the migration on those 3." (confidence-weighted blast radius)

**Hard to retrofit?** Moderate. The edge storage and provenance model already support runtime edges without changes. The hard part is the symbol resolution mapping (route path to graph node), which is built during static indexing. If the indexer doesn't record these mappings from day 1, adding them later requires reindexing all repos. The ingest pipeline itself can be added at any time.

**Recommendation:** Record route/endpoint-to-symbol mappings during static indexing from the start, even before the trace ingest pipeline exists. The mapping table is cheap; having it available when trace ingestion ships avoids a full reindex.

## 14. Semantic PR Diff

**Decision:** knowing generates a relationship-level diff for pull requests: not what text changed, but what the change does to the system graph. This is exposed as both an MCP tool and a CI integration (GitHub Action / webhook).

**Why:**

Code review today is text review. A reviewer sees that 40 lines changed in `auth/middleware.go` and makes a judgment about blast radius based on experience and intuition. They might grep for callers, or they might not. They almost certainly don't check cross-repo impact.

Semantic PR diff makes relationship impact visible without effort. It answers the questions reviewers should ask but often don't: "Does this change add new cross-repo dependencies? Does it increase the blast radius of a critical function? Does it affect symbols owned by other teams?"

This is the most visible feature knowing can ship. Developers see it on every PR. It demonstrates the value of the graph without requiring anyone to change their workflow or learn a new tool.

**Output format:**

```
knowing diff --base main --head feature/auth-refactor

  Graph impact for PR #482: refactor auth middleware

  Symbols changed: 4
  Edges added:     3
  Edges removed:   1
  Edges modified:  2

  +  auth-service -> user-service.GetUser (calls, confidence 1.0)
     New cross-repo dependency. user-service is owned by @platform-team.

  +  auth-service -> billing-service.ValidateSubscription (calls, confidence 1.0)
     New cross-repo dependency. billing-service is owned by @billing-team.

  +  auth-service -> notification-service.SendAlert (calls, confidence 0.8)
     New cross-repo dependency (inferred from import, no direct call site found).

  -  auth-service -> legacy-session-store.Lookup (calls, confidence 1.0)
     Cross-repo dependency removed.

  ~  AuthMiddleware.Validate blast radius: 12 callers -> 47 callers
     Gained 35 transitive callers via new edges to user-service and billing-service.

  ~  AuthMiddleware.TokenRefresh signature changed
     8 direct callers across 3 repos. 2 callers are in repos not owned by PR author.

  Ownership impact:
     Before: consumers in 1 team (@auth-team)
     After:  consumers in 3 teams (@auth-team, @platform-team, @billing-team)

  Staleness:
     2 edges in the blast radius were last verified > 14 days ago.
     Run `knowing index --repo github.com/org/billing-service` to refresh.
```

**How it works:**

```
1. PR opened (or push to PR branch)
         |
         v
2. knowing indexes the PR branch, producing a new snapshot
         |
         v
3. Merkle diff between base snapshot and PR snapshot
   (only changed subtrees are traversed)
         |
         v
4. For each changed edge:
   - Classify: added, removed, modified
   - Look up ownership for affected symbols
   - Compute blast radius delta (before vs. after)
         |
         v
5. Format and post as PR comment or check annotation
```

**MCP tool:**

```go
// SemanticDiff computes the relationship-level diff between two snapshots.
// Used by agents before committing, and by CI after push.
type SemanticDiffResult struct {
    BaseSnapshot    Hash
    HeadSnapshot    Hash
    SymbolsChanged  int
    EdgesAdded      []EdgeChange
    EdgesRemoved    []EdgeChange
    EdgesModified   []EdgeChange
    BlastRadiusDelta []BlastRadiusDelta
    OwnershipImpact  *OwnershipDelta
    StaleEdges       []Edge
}

type EdgeChange struct {
    Edge        Edge
    SourceRepo  string
    TargetRepo  string
    CrossRepo   bool  // true if source and target are in different repos
    OwnerTeam   string
}

type BlastRadiusDelta struct {
    Symbol       Node
    CallersBefore int
    CallersAfter  int
    NewCallers    []Node
    LostCallers   []Node
}

type OwnershipDelta struct {
    TeamsBefore []string
    TeamsAfter  []string
    NewTeams    []string // teams newly affected by this change
}
```

**Planned MCP tool addition:**

| Tool | Purpose |
|------|---------|
| `semantic_diff` | Relationship-level diff between any two snapshots |
| `pr_impact` | Semantic diff specialized for a PR (resolves base/head from git) |

**CI integration (GitHub Action):**

```yaml
# .github/workflows/knowing-diff.yml
name: Semantic PR Diff
on: [pull_request]
jobs:
  graph-diff:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: blackwell-systems/knowing-action@v1
        with:
          base: ${{ github.event.pull_request.base.sha }}
          head: ${{ github.event.pull_request.head.sha }}
          graph-db: .knowing/graph.db
          post-comment: true  # posts the diff as a PR comment
          fail-on:            # optional: fail the check if thresholds are exceeded
            new-cross-repo-edges: 5
            blast-radius-increase: 100
```

**What this does NOT do:**

- Does not block PRs by default. The diff is informational. Teams can optionally configure thresholds in the GitHub Action (`fail-on`) to enforce constraints, but the default is comment-only.
- Does not replace code review. It augments it with information reviewers can't easily get on their own.
- Does not require knowing to be running in CI. The GitHub Action can operate on a pre-built graph database committed to the repo or fetched from an artifact store. The graph is a single SQLite file.

**Hard to retrofit?** No. Semantic diff is a read-only consumer of the snapshot chain and Merkle diff, which are already core to the architecture. It can be built at any time after `SnapshotDiff` is implemented.

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
| Storage interface | Backend is swappable without changing callers | No (clean boundary, introduce anytime before beta) |
| Traversal cache | Content-addressed invalidation, no TTL heuristics | No (optimization layer, add when benchmarks justify) |
| Runtime trace ingestion | Ground truth from production, not just static analysis | Moderate (symbol-to-route mappings needed during indexing) |
| Semantic PR diff | Relationship impact visible on every PR | No (read-only consumer of snapshot chain) |
| Daemon process model | Graph outlives agent sessions | No (can start as CLI, add daemon later) |
