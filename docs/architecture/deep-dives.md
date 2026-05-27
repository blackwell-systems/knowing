# Deep Dives

This document records foundational design decisions for knowing. These choices are made early because they are expensive or impossible to retrofit later.

## 1. Content-Addressed Graph (Merkle DAG)

The graph is a Merkle DAG. Every node, edge, and graph state is identified by its content hash. Domain-type prefixes (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) ensure hashes from different entity types are structurally distinguishable. The four-level hierarchical Merkle tree (repo root -> package roots -> edge-type roots -> edge leaves) enables `DiffHierarchicalTrees` to compare package roots instead of all edges: 216x faster on the knowing repo (~24.9K edges), 517x on 100K synthetic edges.

For full details, see [concepts.md](concepts.md) and [data-model.md](data-model.md).

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

Edges are never mutated in place. New indexing runs produce new edges; old edges remain with their original timestamp and provenance until garbage collected. This makes "when did this edge first appear?" a trivial query, "what changed between deploy A and deploy B?" a range scan, and rollback a matter of pointing to an older snapshot. If you start with mutable state, you can never recover the history.

For full details, see [data-model.md](data-model.md) (the `edge_events` table section).

## 4. Edge Provenance

**Decision:** Every edge carries metadata about how it was derived.

**Fields:**

```json
{
  "source": "ast_resolved",
  "confidence": 0.85,
  "indexer_version": "0.1.0",
  "source_commit": "abc123def",
  "source_file_hash": "sha256:...",
  "timestamp": 1715700000
}
```

**Provenance sources and confidence tiers:**

| Source | Confidence | Meaning | Status |
|--------|-----------|---------|--------|
| `ast_resolved` | 0.85 | Import-map resolution (Python, TS, Rust, Java, C#) or Go `go/packages` (`--full`) | Implemented (5 language extractors + Go full) |
| `scip_resolved` | 0.95 | Imported from SCIP index (external dependency) | Implemented (`knowing ingest-scip`) |
| `lsp_resolved` | 0.9 | Resolved via language server query | Implemented (enrichment pipeline) |
| `ast_inferred` | 0.7 | Tree-sitter AST extraction without type resolution | Implemented (all 23 extractor packages) |
| `otel_trace` | 0.2-0.95 | Observed in runtime traces | Implemented (trace ingestor) |
| `config_declared` | 0.8 | Declared in infrastructure config (Terraform, K8s) | Not implemented (infra extractors use ast_inferred) |
| `inferred_from_import` | 0.7 | Inferred from import statement (no call site found) | Not implemented |
| `openapi_declared` | 0.7 | Declared in OpenAPI/proto spec | Not implemented |
| `text_matched` | 0.3 | Matched by text heuristic (string literal, comment) | Not implemented |
| `otel_trace` | 0.2 - 0.95 | Observed in production via OpenTelemetry traces; confidence varies by observation count and recency |
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

Numbered SQL migrations are embedded in the binary and applied on startup. Without this from day 1, the only upgrade path is "delete your graph and reindex everything." Migrations run automatically in `NewSQLiteStore`; each runs in its own transaction with no rollback/down migrations supported.

For the full migration history (migrations 001-013), see [data-model.md](data-model.md) (the Migration History section).

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

SQLite is the authoritative persistent store (the artifact, the ledger). Pebble is a planned adjacency-list acceleration index for graph traversal. The system ships on SQLite alone; Pebble is added when traversal benchmarks justify it. SQLite is the right initial choice because it is a single file, zero-configuration, embedded in the binary, queryable with standard SQL for debugging, and sufficient for graphs up to roughly 1M edges. Pebble encodes edges as `edges/to/<target_hash>/<edge_hash>` so all inbound edges are physically contiguous, turning multi-hop traversal from B-tree joins into prefix scans.

For the full schema, storage rationale, traversal thresholds, and alternatives analysis, see [data-model.md](data-model.md) (the Why SQLite and GraphStore Interface sections).

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

Every derived result in knowing (traversals, blast radius analyses, semantic diffs, runtime aggregations) is a content-addressed artifact keyed by `(query_params, snapshot_root_hash)`. A query result computed against snapshot root `R` is valid for all time; when a new snapshot `R'` is created, the Merkle diff identifies exactly which subtrees changed and only those results are invalidated. The three-tier cache (L1 in-memory LRU, L2 materialized results in SQLite, L3 bounded traversal with early termination) resolves queries from fastest to slowest and populates upper tiers on miss.

For the full cache tier design, interface definition, and six distribution capabilities, see [context-packing.md](context-packing.md).

## 13. Runtime Trace Ingestion

knowing ingests OpenTelemetry traces, production call graphs, and traffic logs as first-class edges alongside statically-derived edges. Runtime edges use the same content-addressed storage, provenance model, and snapshot chain as static edges. The critical prerequisite is recording route/endpoint-to-symbol mappings during static indexing; without these mappings, trace spans cannot be resolved to graph nodes and must be stored with provenance `runtime_unresolved` and confidence 0.3.

For the full ingestion pipeline, confidence scoring model, symbol resolution architecture, and `TraceIngestor` interface, see [runtime-traces.md](runtime-traces.md).

## 14. Semantic PR Diff

knowing generates a relationship-level diff for pull requests: not what text changed, but what the change does to the system graph. It is exposed as MCP tools (`snapshot_diff`, `semantic_diff`, `pr_impact`), a CLI command (`knowing audit-diff`), and a GitHub Actions workflow (`.github/workflows/pr-semantic-diff.yml`). Removed-edge diffs are correct as of migration 013, which added full edge data to `edge_events`. The diff is a read-only consumer of the snapshot chain and can be added at any time after `SnapshotDiff` is implemented.

For the full design, output format, implementation details, and CI workflow, see [semantic-pr-diff.md](semantic-pr-diff.md).

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
| Content-addressed graph (hierarchical Merkle tree) | Integrity, history, staleness are structural; package-scoped invalidation is 216x faster | Yes (requires full rewrite of storage) |
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
