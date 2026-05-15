# Architecture

## Overview

knowing is a persistent daemon that builds and serves a content-addressed knowledge graph of cross-repository code relationships.

### Components

```
knowing daemon (long-lived)
  ├── Indexer (crawls repos, parses ASTs, resolves types, builds Merkle DAG)
  ├── Graph Store (SQLite behind GraphStore interface, WAL mode)
  ├── MCP Server (stdio or HTTP, serves agent queries)
  ├── Snapshot Manager (computes Merkle roots, GCs old snapshots)
  └── Trace Ingestor (OTel spans, HTTP logs → runtime edges)
```

### System Diagram

```
+------------------+     +------------------+     +------------------+
|   Local Repos    |     |  External Deps   |     |   Agent (MCP)    |
|  (Tier 1: deep)  |     | (Tier 2: shallow)|     |                  |
+--------+---------+     +--------+---------+     +--------+---------+
         |                         |                        |
         v                         v                        |
+--------+---------+     +---------+--------+               |
|  AST Parser      |     |  SCIP/LSP Ingest |               |
|  (go/packages,   |     |  (public API     |               |
|   tree-sitter)   |     |   surface only)  |               |
+--------+---------+     +---------+--------+               |
         |                         |                        |
         +------------+------------+                        |
                      v                                     |
         +------------+------------+     +------------------+
         |   Content-Addressed     |     |  Non-Code Ingest |
         |      Graph Store        |<----| (Terraform, K8s, |
         |  (Merkle DAG, SQLite)   |     |  CODEOWNERS,     |
         |                         |     |  OpenAPI specs)  |
         +------------+------------+     +------------------+
                      |
              +-------+-------+
              v               v
+-------------+---+   +------+-----------+
| Snapshot Chain  |   | Runtime Ingest   |
| (root hashes    |   | (OTel traces,    |
|  linked like    |   |  production      |
|  git commits)   |   |  traffic logs)   |
+-----------------+   +------------------+
```

### Language Model

The graph model is language-agnostic. Symbols, edges, hashes, provenance, and snapshots carry no language-specific semantics. A Go function, a Python class, and a TypeScript route handler all produce the same node and edge structures, identified by the same hash scheme, stored in the same graph. The extractor produces them; the graph doesn't care what language they came from.

Language-specific knowledge lives entirely in the extractors:

| Language | Extractor | What it provides |
|----------|-----------|-----------------|
| Go | `go/packages` | Full type resolution, cross-module call edges, interface satisfaction |
| Python, TypeScript, Java, Rust, etc. | tree-sitter | AST-level symbol extraction, call edges, import tracking |
| Any language with SCIP support | SCIP ingest | Pre-built index from external toolchain (scip-go, scip-java, scip-typescript, etc.) |

Adding a new language means writing an extractor that produces nodes and edges. No changes to the graph store, snapshot chain, MCP server, cache, or any other component.

### Indexing Tiers

- **Tier 1 (deep)**: local repositories. Full AST parsing with type resolution (`go/packages` for Go, tree-sitter for other languages). Every symbol, call, import, and type relationship is extracted.
- **Tier 2 (shallow)**: external dependencies. Public API surface only, ingested via SCIP indices or LSP queries. Enough to connect cross-repo edges without parsing all transitive source.

### Edge Types

The graph connects symbols with typed, provenance-annotated edges:

| Category | Edge types |
|----------|-----------|
| Code | `calls`, `imports`, `implements`, `references` |
| Protocol | `rpc_calls`, `produces_event`, `consumes_event` |
| Schema | `reads_field`, `writes_field`, `declares_route`, `consumes_route` |
| Infrastructure | `deploys`, `connects_to`, `depends_on_service` |
| Ownership | `owned_by_team`, `owned_by_user` |
| Runtime | `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes`, `runtime_queries` |

### Design Goals

- **Content-addressed**: every graph state is a hash; history, staleness, and integrity are structural properties, not bolted-on features
- **Two-tier indexing**: deep AST-level index for local repos, shallow SCIP/LSP ingest for external dependencies
- **Incremental**: git push triggers re-index of changed files only; unchanged file hashes skip re-parse entirely
- **Language-aware at boundaries**: Go calling Go is straightforward; Go calling a Python service via HTTP needs route mapping
- **MCP-native**: exposed as MCP tools, consumed by agents directly
- **Fast**: optimized for interactive agent queries over large multi-repo graphs
- **Deterministic**: same input at same commit always produces the same graph (verifiable via hash)
- **Computation cache as a primitive**: every derived result (traversals, blast radius, semantic diffs) is a content-addressed artifact that can be stored, shared, synced, and referenced with the same guarantees as the graph itself
- **Artifact-boundary separation**: the system decomposes into execution (produces the graph), artifact (the graph itself), and intelligence (interprets the graph); intelligence features never write back to the graph and can operate entirely on a portable artifact

### Architectural Planes

knowing decomposes into three planes separated by an artifact boundary. This separation is structural, not organizational. Features are placed by a bright-line rule: if a feature's value depends on the system being alive, it belongs in the execution plane; if its value survives after the system stops, it belongs in the intelligence plane.

```
Execution Plane (produces the artifact)
├── Indexer
│   ├── Go extractor (go/packages, full type resolution)
│   ├── tree-sitter extractors (Python, TypeScript, Java, Rust, etc.)
│   └── SCIP ingest (external dependency surfaces)
├── Trace ingestion pipeline
│   ├── OTel span ingest
│   ├── HTTP access log ingest
│   └── Runtime symbol resolution (route path → graph node)
├── Daemon
│   ├── File watcher (fsnotify, git hook triggers)
│   ├── Incremental reindex (changed files only)
│   └── Snapshot manager (Merkle root computation, GC)
└── Graph store
    ├── SQLite backend (behind GraphStore interface)
    ├── Node/edge/snapshot storage
    └── Event log (append-only edge events)

════════════════════════════════════════════════════
Artifact boundary: the content-addressed graph
├── SQLite file (portable, self-contained)
├── Snapshot chain (root hashes, parent pointers)
├── Edge event log (full history)
├── Provenance metadata (per-edge)
└── Derived results (content-addressed computation artifacts)
════════════════════════════════════════════════════

Intelligence Plane (interprets the artifact)
├── Semantic PR diff (relationship-level impact per PR)
├── Graph-native test selection (affected tests from graph traversal)
├── Temporal reasoning (walk snapshots to find when incompatibilities appeared)
├── Organizational materialized views (team-scoped subgraphs, standing queries)
├── Ownership routing (who to notify, computed from graph edges)
├── Compliance audit (provenance verification, audit-date comparisons)
├── Confidence decay analysis (staleness scoring, reindex prioritization)
├── Agent coordination (pending mutations, multi-agent visibility)
├── Cross-machine cache sync (Merkle-based derived result exchange)
├── Federated graph queries (cross-instance queries via Merkle diff)
├── CI integration (GitHub Action for PR comments, threshold enforcement)
└── Staleness dashboard (subgraph freshness visualization)
```

**The artifact boundary rule:**

The content-addressed graph is the artifact contract. The execution plane produces it. The intelligence plane consumes it. Intelligence features never write edges, nodes, or snapshots back into the graph. They may produce derived results (which are themselves content-addressed artifacts stored alongside the graph), but derived results are a separate artifact class that does not participate in the Merkle DAG of the primary graph.

**Why this separation matters:**

The execution plane must be trusted. It determines what the graph contains, how symbols are identified, how edges are resolved, and how provenance is recorded. If the indexer is wrong, the graph is wrong. Trust in the execution plane is non-negotiable.

The intelligence plane does not need the same trust. It interprets the graph but cannot change it. A buggy semantic PR diff produces a bad report, not a bad graph. A slow temporal reasoning query wastes time, not integrity. Intelligence features can be opinionated, approximate, or even wrong without compromising the artifact. This asymmetry is the foundation of clean architectural separation.

**Applying the four boundary tests:**

| Test | Intelligence plane features | Result |
|------|---------------------------|--------|
| Air-gap test | Can they run on a different machine with only the SQLite file? | Yes. Copy the file, disconnect, query. |
| Shutdown test | Do they produce value if the indexer stops forever? | Yes. The last snapshot is still queryable. |
| Control flow test | Do they affect what the indexer produces? | No. They read the graph; they don't write to it. |
| Trust test | Would users trust the graph if these features were proprietary? | Yes. The graph is content-addressed and verifiable regardless. |

**The MCP tool split:**

| Tool | Plane | Why |
|------|-------|-----|
| `index_repo` | Execution | Produces graph state |
| `cross_repo_callers` | Execution | Direct graph traversal (basic read) |
| `graph_query` | Execution | Direct graph query (basic read) |
| `blast_radius` | Intelligence | Computed analysis over the graph |
| `trace_dataflow` | Intelligence | Multi-hop interpreted traversal |
| `semantic_diff` | Intelligence | Snapshot comparison with classification |
| `pr_impact` | Intelligence | Semantic diff scoped to a PR |
| `snapshot_diff` | Intelligence | Structural diff between graph states |
| `stale_edges` | Intelligence | Staleness analysis |
| `ownership` | Intelligence | Cross-referencing graph with ownership metadata |
| `repo_graph` | Execution | Direct graph read (repo-level view) |

Basic graph reads (`cross_repo_callers`, `graph_query`, `repo_graph`) are execution-plane operations: they return what the graph contains without interpretation. Intelligence-plane tools compute, classify, compare, or aggregate, and they produce derived results that are themselves content-addressed artifacts.

**The trace ingestion boundary:**

Runtime trace ingestion straddles the planes. The ingest pipeline (normalizing spans, resolving symbols, writing edges) is execution: it produces graph state. The aggregation, confidence scoring, and decay analysis that operate on ingested edges are intelligence: they interpret what the ingest pipeline produced. The architecture separates these by interface: `TraceIngestor` belongs to the execution plane and writes to `GraphStore`; confidence decay and runtime aggregation caching belong to the intelligence plane and read from `GraphStore` and `ComputationCache`.

---

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

## 9. Storage: SQLite Ledger + Pebble Acceleration Index

**Decision:** Use SQLite as the authoritative persistent store (the artifact, the ledger) and Pebble as an adjacency-list acceleration index for graph traversal. Ship on SQLite alone; add Pebble when traversal benchmarks justify it.

**The two-layer model:**

```
SQLite (the artifact / ledger)
├── repos, files, nodes, edges, edge_events, snapshots, schema_version
├── derived_results (computation cache)
├── Portable: copy one file, the artifact moves with it
├── Debuggable: sqlite3 graph.db "SELECT ..."
├── Authoritative: this is the source of truth
└── Sufficient for graphs up to ~1M edges

Pebble (acceleration index, derived from SQLite)
├── edges/from/<node_hash>/<edge_hash> → edge data
├── edges/to/<node_hash>/<edge_hash> → edge data
├── Optimized: neighbors are physically co-located (prefix scan, not B-tree join)
├── Rebuildable: losing the Pebble directory triggers a one-time rebuild from SQLite
└── Required when traversal latency on SQLite exceeds interactive thresholds
```

**Why SQLite as the ledger:**

- Single file, zero configuration, embedded in the binary
- Handles tens of millions of rows without issues
- WAL mode gives concurrent read/write without blocking
- Known quantity (commitmux already uses SQLite with similar patterns)
- Queryable with standard SQL for debugging
- Backup = copy one file
- The artifact boundary (decision #15) requires the graph to be a portable, self-contained file; SQLite is that file

**Why SQLite alone is not enough:**

SQLite stores edges in B-trees indexed by hash. Finding all callers of a symbol is an indexed lookup on `idx_edges_target`, which is fast for a single hop. But multi-hop traversal (blast radius, transitive callers) requires recursive CTEs where each hop is a separate B-tree join. At depth 5 with wide fan-out, this means five random-access lookups per path, multiplied by the branching factor at each hop.

For graphs under ~1M edges, this is tens of milliseconds. For larger graphs, it becomes seconds. The computation cache (decision #12) handles repeat queries, but the first query for a hot symbol after a snapshot change is the one that hurts.

**Why Pebble as the acceleration layer:**

Pebble (CockroachDB's LSM storage engine) stores data in sorted key order. By encoding edges as `edges/to/<target_hash>/<edge_hash>`, all inbound edges to a symbol are physically contiguous on disk. Finding all callers is a single prefix scan, a sequential read instead of a random-access join. Each hop in a multi-hop traversal is a prefix scan, not a B-tree lookup.

Why Pebble specifically:
- Embedded, single-binary (like SQLite)
- Designed for high read throughput and range scans
- Native snapshots (LSM snapshots) align with knowing's snapshot model
- Battle-tested at scale in CockroachDB
- Go-native (no CGo required)

**The relationship between the two:**

SQLite is authoritative. Pebble is derived. Every edge write goes to SQLite first, then to Pebble. If Pebble is lost or corrupted, it is rebuilt from SQLite's `edges` table. The `GraphStore` interface routes queries: point lookups and event log queries go to SQLite; traversal queries (`TransitiveCallers`, `TransitiveCallees`, `BlastRadius`) go to Pebble.

```go
type HybridStore struct {
    ledger  *SQLiteStore   // authoritative: all reads and writes
    accel   *PebbleStore   // acceleration: traversal reads only
}

func (h *HybridStore) PutEdge(ctx context.Context, e Edge) error {
    // Write to ledger (authoritative)
    if err := h.ledger.PutEdge(ctx, e); err != nil {
        return err
    }
    // Write to acceleration index (derived)
    return h.accel.IndexEdge(ctx, e)
}

func (h *HybridStore) TransitiveCallers(ctx context.Context, target Hash, maxDepth int, snapshot Hash) ([]CallerResult, error) {
    if h.accel != nil {
        // Pebble prefix scan: sequential reads, physically co-located neighbors
        return h.accel.TransitiveCallers(ctx, target, maxDepth, snapshot)
    }
    // Fallback: SQLite recursive CTE
    return h.ledger.TransitiveCallers(ctx, target, maxDepth, snapshot)
}
```

**Pebble key encoding:**

```
Inbound edges (callers):
  edges/to/<target_hash>/<edge_hash> → {source_hash, edge_type, confidence, provenance}

Outbound edges (callees):
  edges/from/<source_hash>/<edge_hash> → {target_hash, edge_type, confidence, provenance}

Snapshot-scoped edges (for point-in-time traversal):
  snapedges/<snapshot_hash>/to/<target_hash>/<edge_hash> → edge data
```

The `snapedges/` prefix enables point-in-time traversal without filtering: scan `snapedges/<snapshot>/to/<target>/` to get all callers at that snapshot. Storage cost is proportional to `edges * snapshots_retained`, mitigated by snapshot GC.

**When to add Pebble:**

The trigger is benchmark results, not speculation. The criteria:

| Metric | SQLite-only threshold | Action |
|--------|----------------------|--------|
| p95 blast radius latency at depth 3 | < 200ms | Stay on SQLite |
| p95 blast radius latency at depth 3 | 200ms - 1s | Add computation cache materialization, re-measure |
| p95 blast radius latency at depth 3 | > 1s after caching | Add Pebble acceleration index |
| Total edge count | < 1M | SQLite is fine |
| Total edge count | 1M - 10M | Benchmark, likely need Pebble |
| Total edge count | > 10M | Pebble required |

**What about libSQL?**

libSQL (SQLite fork by Turso) adds built-in replication and is wire-compatible with SQLite. It doesn't improve traversal performance (same B-tree engine), but its replication protocol could simplify the federated graph workstream (decision #14 in the roadmap). Evaluate when federation becomes a priority; it's a drop-in replacement for SQLite that adds sync, not a different storage model.

**Alternatives considered and rejected:**

- **DuckDB**: columnar, optimized for analytical scans. Wrong query pattern; knowing's hot path is point lookups and graph walks, not aggregation.
- **In-memory graph (gortex pattern)**: fast traversal but loses the artifact portability story. The graph must survive process crashes and be copyable as a file. In-memory stores require all-or-nothing serialization and lose data on crash.
- **External graph databases (Neo4j, Dgraph)**: native graph traversal but kills single-binary deployment, adds operational complexity, and doesn't natively support content-addressed storage.
- **BoltDB/bbolt**: B+ tree, single-writer. Same traversal characteristics as SQLite but without SQL for debugging and without WAL concurrent reads. Strictly worse for knowing's use case.
- **SQLite virtual tables**: custom storage for the edges table behind a virtual table interface. Clever but high implementation cost (requires CGo), and the "single file" artifact story gets complicated.

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

## 12. Content-Addressed Computation Cache

**Decision:** Every derived result in knowing (traversals, blast radius analyses, semantic diffs, runtime aggregations) is a content-addressed artifact: keyed by `(query_params, snapshot_root_hash)`, deterministically reproducible, and shareable across machines with the same guarantees as the graph itself. Caching is not an optimization layer; it is a core architectural primitive that enables distribution, collaboration, and scalability.

**Why this is not normal caching:**

Most cache invalidation is a guessing game: TTL-based expiry hopes data hasn't changed, event-driven invalidation hopes no events were missed, version counters hope nothing incremented out of band. Content-addressed storage eliminates guessing entirely. A query result computed against snapshot root `R` is valid for all time. It is not "probably still fresh"; it is provably correct by construction. When a new snapshot `R'` is created, the Merkle diff between `R` and `R'` identifies exactly which subtrees changed. Only results that touch changed subtrees are invalidated. Everything else remains valid without re-verification.

This property transforms caching from a performance concern into a distribution and collaboration primitive.

### The reframing

The graph itself is a cache. Source code is the truth. The graph is a content-addressed, queryable, provenance-tracked cache of what the source code *means*. Every query result is a further derivation from that cache, and those derivations are themselves cacheable, storable, shareable, and referenceable with the same integrity guarantees.

This means knowing's scalability story is not "SQLite with some LRU on top." It is a content-addressed computation cache where every derived result is a verifiable artifact.

### Cache tiers

**L1: In-Memory LRU (Process-Scoped)**

```go
type CacheKey struct {
    TargetHash   Hash
    QueryType    string // "transitive_callers", "blast_radius", "semantic_diff", etc.
    MaxDepth     int
    SnapshotRoot Hash
}

type L1Cache struct {
    mu       sync.RWMutex
    entries  map[CacheKey]*cacheEntry
    lru      *list.List
    maxSize  int // default: 10,000 entries
}
```

Keyed by `(target_hash, query_type, max_depth, snapshot_root)`. Same query against the same snapshot always returns the same result. On snapshot creation, the Merkle diff evicts only entries whose nodes fall within changed subtrees. Entries outside the diff survive across snapshots. Eviction is a performance choice, never a correctness one.

**L2: Materialized Results (SQLite, Persisted)**

For high-fan-in symbols and expensive computations, precompute and store results in the database:

```sql
CREATE TABLE derived_results (
    result_hash    BLOB PRIMARY KEY,    -- hash(query_params + snapshot_root)
    query_type     TEXT NOT NULL,       -- "transitive_callers", "blast_radius", "semantic_diff"
    query_params   BLOB NOT NULL,       -- content-addressed query parameters
    snapshot_hash  BLOB NOT NULL,       -- snapshot this was computed against
    result_data    BLOB NOT NULL,       -- the computed result
    computed_at    INTEGER NOT NULL,    -- unix timestamp (for GC, not invalidation)
    computed_by    TEXT NOT NULL        -- node identity (for distributed provenance)
);

CREATE INDEX idx_dr_snapshot ON derived_results(snapshot_hash);
CREATE INDEX idx_dr_query ON derived_results(query_type, snapshot_hash);
```

Materialization is triggered by fan-in (symbols with > 50 direct callers), by CI pipelines (semantic PR diff results), or by explicit request (organizational standing queries). Invalidation is structural: the Merkle diff identifies which results to recompute.

**L3: Bounded Traversal with Early Termination**

For interactive queries where latency matters more than completeness:

```go
type TraversalOptions struct {
    MaxDepth      int     // hard cap on hops (default: 5)
    MaxResults    int     // stop after collecting this many results (default: 500)
    MinConfidence float64 // prune paths below this confidence (default: 0.0)
}
```

When any limit is hit, the result includes `Truncated: true`. The common case (2-3 hops, narrow fan-out) stays fast regardless of graph size.

**Query resolution order:**

```
1. L1 (in-memory) exact key match → return immediately
2. L2 (persisted) query_type + snapshot match → filter, populate L1, return
3. Live computation with TraversalOptions bounds → populate L1 and L2, return
```

### Beyond performance: caching as a distribution primitive

The content-addressed property enables six capabilities that go beyond traditional caching:

**1. Query results as first-class graph artifacts**

A blast radius result is not just a cache entry. It is a content-addressed object stored in the graph with its own hash and provenance: "computed by knowing v0.4 against snapshot abc123 on machine X at time T." An SRE asking "what was the blast radius at deploy time?" gets the stored artifact from the CI run, not a recomputation. Query results become part of the ledger.

**2. Cross-machine cache sharing via Merkle sync**

Two developers indexing the same repos at the same commit produce the same graph (deterministic reindexing, decision #8). Their query results against the same snapshot are also identical by construction. The Merkle sync mechanism designed for graph exchange also works for exchanging precomputed results. A team lead runs a comprehensive analysis; every developer on the team gets the result via cache sync, with cryptographic proof it's correct.

**3. Organizational materialized views**

Standing queries materialized as content-addressed subgraphs: "everything team X owns and all inbound cross-repo edges" or "all services that touch the payments domain." Kept current by Merkle diff (recompute only when the relevant subtree changes). These become always-consistent organizational dashboards. The cache becomes the product for non-agent audiences.

**4. Agent working set accumulation**

An agent working on auth middleware runs 15 queries that map out a neighborhood of the graph. That working set is a subgraph with a content hash. The next agent touching the same area gets the working set pre-loaded, with a Merkle diff check to confirm currency. Agent sessions build on each other's exploration rather than starting cold.

**5. CI pipeline result caching**

Semantic PR diff results cached by `(base_snapshot_root, head_snapshot_root)`. A rebase that doesn't change the effective diff is free. Multiple PRs against the same base share the base-side computation. Graph-native test selection results are cached the same way. This makes knowing's CI integration fast enough to run on every push.

**6. Runtime trace aggregation caching**

Raw trace ingestion produces millions of spans. Aggregated results ("service A called service B 14,000 times this week") are expensive to compute but stable within a time window. Cached by `(time_window, snapshot_root)`. When a new snapshot doesn't change the relevant static edges, the aggregation carries forward.

### Interface

The computation cache is not hidden inside the storage backend. It is a first-class system component:

```go
// ComputationCache manages content-addressed derived results.
type ComputationCache interface {
    // Get retrieves a cached result by its content hash.
    Get(ctx context.Context, resultHash Hash) (*DerivedResult, error)

    // GetByQuery retrieves a cached result by query parameters and snapshot.
    GetByQuery(ctx context.Context, queryType string, params Hash, snapshot Hash) (*DerivedResult, error)

    // Put stores a derived result. The result hash is computed from
    // (query_type, query_params, snapshot_root).
    Put(ctx context.Context, result DerivedResult) error

    // Invalidate removes results whose dependency sets intersect with
    // the changed subtrees between two snapshots.
    Invalidate(ctx context.Context, oldSnapshot, newSnapshot Hash, diff MerkleDiff) (evicted int, err error)

    // Sync exchanges derived results with a remote cache via Merkle diff.
    // Only results not present locally are transferred.
    Sync(ctx context.Context, remote RemoteCache, snapshot Hash) (received int, err error)

    // Materialize precomputes and stores results for a set of standing queries.
    Materialize(ctx context.Context, queries []StandingQuery, snapshot Hash) error
}

// DerivedResult is a content-addressed computation result.
type DerivedResult struct {
    ResultHash   Hash
    QueryType    string
    QueryParams  Hash   // hash of the query parameters
    SnapshotRoot Hash
    Data         []byte // the result payload
    ComputedAt   time.Time
    ComputedBy   string // node identity
}

// StandingQuery is a query that is automatically re-materialized on each snapshot.
type StandingQuery struct {
    Name       string // human-readable identifier
    QueryType  string
    Params     Hash
    Schedule   string // "on_snapshot", "hourly", "daily"
}
```

**Hard to retrofit?** The L1/L2/L3 performance cache is easy to add at any time. The elevated capabilities (cross-machine sync, standing queries, agent working sets, CI result caching) require the `ComputationCache` interface and the `derived_results` table to be designed in, but can be implemented incrementally. The key decision that must be made early is treating derived results as content-addressed artifacts with their own hashes, not as opaque cache entries. That framing shapes the storage schema and the sync protocol.

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

## 15. Artifact-Boundary Plane Separation

**Decision:** knowing is architecturally decomposed into an execution plane, an artifact boundary, and an intelligence plane. The execution plane produces the content-addressed graph. The intelligence plane interprets it. The graph (the SQLite file, snapshot chain, edge event log, and derived results) is the artifact contract between them. Intelligence features never write back to the graph.

**Why:**

This separation is the architectural primitive that makes every other property of the system trustworthy. The execution plane (indexer, trace ingestion, daemon, graph store) must be correct: if it produces a wrong graph, everything downstream is wrong. The intelligence plane (semantic diff, blast radius analysis, temporal reasoning, compliance reports, dashboards) must be useful but does not need to be correct in the same way. A buggy intelligence feature produces a bad report, not a bad graph.

This asymmetry means:
- The execution plane can be audited independently of the intelligence plane
- Intelligence features can be added, removed, or replaced without touching the graph
- The artifact (the SQLite file) is portable, self-contained, and interpretable by any tool that understands the schema
- Third parties can build their own intelligence features against the artifact without depending on knowing's intelligence plane

**The bright-line rule:**

If a feature's value depends on the system being alive (the indexer running, repos being watched, traces being ingested), it belongs in the execution plane.

If its value survives after the system stops (the last snapshot is still queryable, the graph file is still analyzable), it belongs in the intelligence plane.

**Why hard to retrofit?** Yes. If intelligence features write to the graph (even "just one edge annotation" or "just one enrichment pass"), the artifact contract is broken. The graph is no longer a pure product of execution; it's contaminated by interpretation. Staleness detection, deterministic verification, and provenance tracking all depend on the graph being produced solely by the execution plane. This constraint must be established at the beginning and enforced structurally (the intelligence plane has read-only access to `GraphStore` and write access only to `ComputationCache`).

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
| SQLite ledger + Pebble acceleration | Artifact portability (SQLite) with fast traversal (Pebble) | No (Pebble is derived, added when benchmarks justify) |
| Storage interface | Backend is swappable without changing callers | No (clean boundary, introduce anytime before beta) |
| Computation cache | Every derived result is a content-addressed, shareable artifact | Moderate (result-as-artifact framing must be early; tiers are incremental) |
| Runtime trace ingestion | Ground truth from production, not just static analysis | Moderate (symbol-to-route mappings needed during indexing) |
| Semantic PR diff | Relationship impact visible on every PR | No (read-only consumer of snapshot chain) |
| Artifact-boundary plane separation | Intelligence never writes to the graph; the artifact contract stays pure | Yes (one write-back path contaminates provenance and breaks verification) |
| Daemon process model | Graph outlives agent sessions | No (can start as CLI, add daemon later) |
