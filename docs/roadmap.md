# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort | Expected Impact |
|---|------|-----|--------|-----------------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. agent-lsp has 40 stars after 1 month; knowing needs the same traction. | Ongoing | - |
| 2 | **Type-aware keyword extraction** | Task descriptions describe BEHAVIOR ("custom migration operation") not SYMBOLS. Search for TYPE nodes in packages matching one term that have methods matching another. Example: `WHERE package LIKE '%migration%' AND kind='type' AND has_method LIKE '%forward%'` finds `Operation.state_forwards`. | Medium (50 lines) | +5-10% P@10 |
| 3 | **AND-semantics path matching** | Path-context seeding currently uses OR (any term matches). With AND semantics, "migration operation" intersects to find nodes in `migrations/operations/` specifically, not the 1,743 nodes matching either term alone. | Low (30 lines) | +3-5% P@10 |
| 4 | **Semantic concept expansion** | For each extracted keyword, also seed its related code concepts from a static ~200-entry thesaurus. "migration" also seeds "schema", "alter", "table". "handler" also seeds "middleware", "route", "endpoint". No LLM, just a handcrafted concept graph. | Medium (100 lines + thesaurus) | +3-8% P@10 |
| 5 | **Local embeddings (Channel 6)** | Lightweight local embedding model (~30MB ONNX) embeds task descriptions and symbol signatures into shared vector space. Bridges vocabulary gap completely: "custom migration operation" finds `Operation.state_forwards` via semantic similarity. Infrastructure already exists (`internal/embedding/`). | High (prototyped) | +5-15% P@10 |
| 6 | **event-stream supply chain demo** | Index clean + compromised versions, show `knowing diff` catches malicious edges, prove absence/presence with Merkle proofs. Blog post: "We cryptographically proved this module can't exfiltrate data." | Medium | Commercial angle |
| 7 | **Struct field access edges** | `obj.Field` connects the accessor to the field's type definition. Real relationship, belongs in graph. | Low | Correctness |
| 8 | **Parallel write backend** | SQLite single-writer funnels all extraction results through one goroutine. Even with producer-consumer pipeline, writes are serial. Need parallel write support for large repos. | High | Performance |

### Session 14 Findings (rejected approaches)

The following were implemented, benchmarked, and confirmed neutral (P@10 unchanged):

| Approach | Why neutral |
|----------|-------------|
| **Call-chain seeding** (inject callees of top seeds as supplemental RWR seeds) | Callees are already reachable via RWR traversal; adding them as seeds just diffuses probability mass |
| **File-scoped co-retrieval** (inject sibling symbols from same file) | Same: file siblings already reachable via contains/member_of edges |
| **Import graph seeding** | Subsumed by existing path-context seeding (Channel 5) |

The 32-config parameter sweep (session 13) proved P@10 is reachability-determined. The only levers that move P@10 are those that make previously-unreachable ground truth symbols reachable for the first time. Items 2-5 above target the 44.6% of ground truth symbols that are currently unreachable because keyword seeds connect to the wrong subgraph.

## Storage Backend (P0 Performance)

Current: SQLite (single-writer, FTS5 deferred to background). Extraction is parallel (GOMAXPROCS workers, producer-consumer pipeline), but all DB writes funnel through one goroutine. Performance pragmas: `synchronous=NORMAL`, `mmap_size=256MB`, `cache_size=64MB`, `busy_timeout=5000`, `temp_store=MEMORY`. Multi-row batch INSERTs (edges: 100/statement, nodes: 99/statement, files: 249/statement) reduce per-row overhead.

### Options under evaluation

| Backend | Parallel writes | Query model | Deployment | Status |
|---------|----------------|-------------|-----------|--------|
| **SQLite sharded by package** | Yes (one file per package) | Cross-package queries need federation | Multiple files | Prototype next |
| **DuckDB** | Yes (appender API) | SQL, columnar scans | Single file, CGO | Evaluate |
| **BadgerDB/Pebble** | Yes (LSM concurrent memtable) | Key-value (custom query layer) | Single dir, pure Go | Evaluate |
| **SQLite + deferred FTS** | No (serial) | SQL + FTS5 | Single file | **Shipped (current)** |

### Sharding by package (leading candidate)

Packages are already the unit of Merkle computation, cache invalidation, diffing, and RWR scoring. One SQLite file per package means:
- Parallel writes: each extraction worker writes to its own package's DB
- No contention: workers never touch the same file
- Package-scoped queries are local reads
- Delete a package = delete the file
- Merkle computation per-package is already isolated
- Cross-package queries (blast radius, transitive callers) federate across shards

### Current performance (v0.6.0 + optimizations)

| Repo | Files | Edges | Extraction | Total (with deferred FTS) |
|------|-------|-------|-----------|--------------------------|
| knowing (84K LOC) | 448 | 25K | 0.4s | 1.7s |
| flask (15K LOC) | 97 | 9K | 0.04s | 0.3s |
| cargo (150K LOC) | 979 | 79K | 0.2s | 5.5s |
| kubernetes (3.5M LOC) | 4,877 | 268K | 18.6s | ~22s (data queryable immediately) |

## Cross-Repo Query Architecture

The context engine (ForTask, ExplainSymbol, RWR, HITS, BM25) has no repo-scoping anywhere in its query path. If multiple repos exist in the same database, cross-repo queries work with zero code changes. The challenge is the storage model: the roster currently assigns each repo its own SQLite file.

Two approaches are under evaluation:

### Option A: Unified Database (shared graph)

All repos index into a single `~/.knowing/knowing.db`. The roster tracks metadata (paths, URLs) but not separate DB files.

**Pros:**
- Zero engine changes. ForTask, BM25, RWR, FTS5 all work unchanged on the merged graph.
- Cross-repo edges resolve naturally (source and target in same DB).
- One FTS5 index covers all vocabulary. BM25 ranks across all repos in a single query.
- Simplest implementation (~30 LOC change: roster defaults to shared DB).
- Single snapshot chain covers all repos (Merkle diff shows cross-repo changes).
- `knowing remove` already deletes by repo_hash within a shared DB.

**Cons:**
- No isolation between projects. A personal side-project and work monorepo share one graph.
- Larger single file (5 repos x 30K edges = 150K edges, still trivial for SQLite, but conceptually messy).
- Can't delete a repo by deleting a file (must use `knowing remove` which does SQL DELETE).
- If the shared DB corrupts, all repos are affected.
- Users may not want their repos' symbols showing up when querying from a different project.

**Mitigation:** Add `--isolated` flag to `knowing add` for repos that should stay separate. Default to shared for most workflows.

### Option B: Federated Store (query-time merge)

A `FederatedStore` wrapper implements `GraphStore` over N underlying SQLiteStores. The primary store (current repo) receives writes; all roster stores are opened read-only for queries.

```go
type FederatedStore struct {
    primary *SQLiteStore      // writes go here
    others  []*SQLiteStore    // read-only roster DBs
}
```

Query federation strategy per method:
- `NodesByName`: query all stores, concat results, dedup by hash
- `SearchBM25Nodes`: query all stores, merge by score, take top-N
- `EdgesFrom`/`EdgesTo`: query all stores, concat (cross-repo edges live in source DB)
- `GetNode`: try primary first, then others (hash-based lookup)
- `FeedbackBoosts`: query all stores, merge maps
- Write methods (`PutNode`, `PutEdge`, `RecordFeedback`): primary only

**Pros:**
- Per-repo isolation by default. Each repo is a separate file with independent lifecycle.
- `knowing remove` is just closing and deleting a file.
- No corruption propagation between repos.
- Each repo can be backed up, synced, or deleted independently.
- No storage model change; existing per-repo DBs work as-is.
- Users opt-in to cross-repo by having multiple repos in their roster. No surprise data mixing.

**Cons:**
- N queries per method call (latency scales linearly with roster size). 3-5 repos: negligible (<5ms). 20+ repos: needs parallel goroutines.
- FTS5 indexes are per-DB; BM25 merge is approximate (scores from different corpus sizes aren't directly comparable without normalization).
- RWR adjacency map must load edges from all stores, making the first query slower.
- Cross-repo edges are split: source DB has the edge, target DB has the target node. `GetNode` must check multiple stores to resolve targets.
- Medium implementation effort (~200 LOC new type + method-by-method federation logic).
- Feedback recorded in the primary DB may reference nodes in other DBs (works, but feedback is stored asymmetrically).
- Community detection runs per-DB (Louvain on isolated subgraphs); cross-repo communities won't form.

### Comparison

| Dimension | Unified DB | Federated Store |
|-----------|-----------|-----------------|
| Implementation effort | ~30 LOC | ~200 LOC |
| Engine changes required | None | None (same interface) |
| Query latency | 1 query | N queries, merged |
| FTS5 quality | Unified corpus, accurate IDF | Per-corpus IDF, approximate merge |
| Cross-repo edges | Free (same table) | Resolved via multi-store lookup |
| Community detection | Cross-repo communities form naturally | Per-repo communities only |
| RWR walk | Seamless cross-repo | Cross-repo via edge concat |
| Isolation | None by default (opt-in via `--isolated`) | Full by default |
| Corruption blast radius | All repos | Single repo |
| Storage management | One file to manage | N files, cleaner lifecycle |
| `knowing remove` | SQL DELETE (fast) | Close + delete file (instant) |
| Feedback compounding | Cross-repo (symbol used in repo B helps repo A) | Asymmetric (feedback in primary only) |

### Decision

Not yet decided. The choice depends on real usage patterns:
- If most users work across 2-3 related repos (monorepo splits, frontend+backend): **unified DB** wins on simplicity and quality.
- If users have many unrelated projects and want clean separation: **federated store** wins on isolation.
- Both can coexist: unified by default with federated as the advanced mode, or vice versa.

Current status: per-repo isolation (no cross-repo queries). First real user who hits the limitation decides the approach.

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| **Cross-repo context_for_task** | Search across ALL indexed repos simultaneously, not just one. Real projects span multiple repos (monorepo patterns, microservices). Merge results from all repos into one ranked list. See "Cross-Repo Query Architecture" section below. | P2 |
| **Incremental context ("next page")** | After an agent gets initial context, allow requesting the NEXT N symbols not yet seen. Avoids re-querying with bigger budget and getting duplicates. Session-stateful cursor. | P2 |
| **Staleness annotations on MCP responses** | When returning context, annotate symbols whose source files changed since last index. Agents know which results might be outdated without calling `knowing stale` separately. | P2 |
| **CLI `--format gcf` output** | `knowing context` only supports json/xml/markdown. Adding gcf/gcb for direct agent consumption without MCP. | P3 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |

## Benchmarking Roadmap

14 benchmark harnesses exist today (see `bench/README.md`). The following gaps remain for a complete competitive evaluation story.

### P1: Would convince someone to adopt knowing

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **SWE-bench integration** | knowing + Claude solves N% more SWE-bench tasks than Claude alone. The definitive "does graph context help real agent work?" | Not started | High (full eval harness, 300 tasks, automated agent loop) |
| **Real-session replay** | Replay 10+ real claudewatch session transcripts. Measure: context calls saved, symbols used that came from knowing, tasks where knowing provided the critical symbol. | Not started (implicit feedback tracker now exists for attribution) | High (transcript parser, attribution detection, manual annotation) |

### P2: Proves production readiness

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Query latency p50/p95/p99** | Instrument all 28 MCP tool handlers. Report latency distribution per tool across 1000 calls. | Single number (2ms cached) exists; need per-tool distribution | Medium |

### P3: Completeness and rigor

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Multi-language extraction coverage** | For each of the 24 extractors: number of node types extracted, edge types produced, lines of test coverage. Comparison vs Sourcegraph SCIP, GitNexus, tree-sitter-graph. | Not started | Low (automated count + table) |
| **Grafana scale validation** | Full retrieval quality measurement on 714K-edge production graph (not just latency). P@10 with Grafana-specific task fixtures. | Latency test exists (`grafana_scale_test.go`); no retrieval quality measurement | Medium |
| **Graph integrity under load** | Spawn 10 concurrent indexers on overlapping repos. Run `knowing fsck` after. Proves content-addressing prevents corruption under concurrency. | Not started (fsck bench exists for single-indexer correctness) | Medium |
| **Concurrent query performance** | 100 parallel `context_for_task` calls on a 100K-edge graph. Measure throughput (queries/sec), latency degradation, and WAL checkpoint behavior. | Not started | Medium |
| **Cross-repo retrieval quality** | P@10 for tasks that span repo boundaries (e.g., "which frontend components call this backend endpoint?"). | Needs cross-repo implementation first | Medium |



### Standalone Publication: Code Retrieval Evaluation Toolkit (CRET)

Extract knowing's benchmarking infrastructure as the SWE-bench equivalent for code
context retrieval. Full proposal: [docs/proposals/code-retrieval-eval-toolkit.md](../proposals/code-retrieval-eval-toolkit.md).

**Status:** Not started. Prerequisite complete (Aider comparison done, Run 19-22).

### Not yet benchmarked (tracked for completeness)

- **Proof verification throughput**: N proofs/sec verified (currently 1.2µs each = ~800K/sec theoretical)
- **Snapshot chain walk cost**: O(chain_length) for history queries
- **FTS5 rebuild cost vs graph size**: scaling curve for the deferred FTS rebuild
- **Language-specific P@10 breakdown**: already have per-repo numbers; need per-language aggregate

## Retrieval Pipeline

Current results: see [bench/cross-system/FINDINGS.md](../bench/cross-system/FINDINGS.md).
P@10=0.210 (Run 25, 167 tasks, 9 repos). 1.56x vs codegraph, 2.80x vs GitNexus, 3.33x vs Gortex, 16.2x vs grep. Query latency 2ms on k8s (with adjacency cache).

**Key finding (session 13):** 32-config parameter sweep + 6-point doc weight sweep proved P@10 is entirely reachability-determined. No parameter tuning moves it. Only new edges or new seed sources improve retrieval.

### Retrieval Improvements

| # | Item | Why | Status |
|---|------|-----|--------|
| 7 | **BFS frontier pruning for dense graphs** | On graphs >50K nodes (VS Code 87K, k8s 117K), RWR probability mass spreads too thin and correct results drop below rank 10. Limit which nodes enter the adjacency map: prune by minimum edge weight, maximum distance from seeds, or node degree cap. Session 14 proved: correct TS extraction (87K nodes) causes VS Code P@10 to drop from 0.163 to 0.100 due to density dilution. | Experiment |
| 8 | **Community-scoped RWR tuning for dense graphs** | Already partially implemented (single-community constraint). Needs tuning: on dense TypeScript graphs, community detection may produce too-large communities (thousands of nodes). Per-community node cap or hierarchical community splitting could keep RWR focused. | Experiment |
| 9 | **More equivalence concepts** | Only add when a specific task fixture exposes a gap. Must respect Run 22 constraint (no single-word phrases, no generic targets). | On-demand |
| 10 | **`is_entry_point` / `is_exported` node flags** | Tag functions as entry points (main, handlers, CLI commands) or exported (public API). Enables filtering: entry points get higher RWR restart weight. | Experiment |
| 11 | **Feedback parameter sweep (warm-start)** | Session boost (0.20), task memory formula (0.5+score*0.4), decay (7-day linear), top-N (5) are untuned. Only affects real-user compounding. | When users exist |

## Edge Type Expansion

34 edge types shipped. See [Edge Types Reference](architecture/edge-types.md) and [CHANGELOG](CHANGELOG.md) for full details.

### P1: Reachability edges (directly improve P@10)

These edges bridge the 45.6% of unreachable ground truth symbols (407/893). Each creates new paths from keyword seeds to previously-disconnected subgraphs. Prioritized by breadth of applicability across languages.

| # | Edge Type | What it connects | Languages | Expected Impact | Effort |
|---|-----------|-----------------|-----------|-----------------|--------|
| 1 | **`implements_interface` propagation** | When function accepts interface type, connect to all concrete implementors. If `func Handle(c Cache)` exists and `RedisCache implements Cache`, create edge from `Handle` to `RedisCache`. Currently only Go has partial support; needs full cross-language extraction. | Go, Java, TypeScript, C#, Rust (traits) | +3-8% P@10 (bridges k8s 71, django 117 unreachable) | Medium |
| 2 | **`co_tested_with`** | Symbols imported/used in the same test file are functionally related. If `TestCacheIntegration` imports both `RedisCache` and `BaseCache`, create lateral edge between them. Bridges otherwise-disconnected symbols that serve the same feature. | All (test file detection is universal) | +2-5% P@10 (k8s has 58K tests edges; lateral connections between tested symbols) | Medium |
| 3 | **`type_hint_of`** | Function parameter type annotations create usage edges that aren't calls. `def process(cache: BaseCache)` means `process` depends on `BaseCache`. Invisible to call-graph extraction but structurally meaningful. | Python, TypeScript, Java, Rust, Go, C# | +2-5% P@10 (bridges django 117 unreachable: many functions accept base types as params) | Medium |

**Why these matter (failure analysis, session 14):**
- Django: 117/192 ground truth symbols unreachable. Root cause: framework base classes (`BaseCache`, `BaseDatabaseWrapper`, `Operation`) are referenced by type hint and interface contract, not by direct call. Seeds find concrete implementations but can't walk to the base class.
- Kubernetes: 71/116 unreachable. Root cause: interface-heavy architecture where functions accept interfaces (`runtime.Object`, `Informer`) but ground truth is the concrete implementations. Interface propagation would bridge these.
- Kafka: 50/93 unreachable. Root cause: consumer/producer patterns where coordinator classes are referenced via type parameters and configuration, not direct calls.

### P2: Structural edges

| Category | Items | Status |
|----------|-------|--------|
| Runtime | `runtime_queries`, `runtime_connects_to` | Planned |
| Configuration | `configures` (config key to symbol that reads it) | Planned |
| Agent workflow | `suggested_for_task` / `used_by_agent` | Planned |

## Observability Ingestion

Beyond OTLP traces (shipped), these observability signals map to graph edges. The pattern: any system that records "X talked to Y" at runtime becomes a `runtime_*` edge. Static analysis says what CAN happen. Runtime signals say what DID happen. The diff is where findings live.

| Signal Source | Edge Types | What It Enables | Priority |
|---|---|---|---|
| Database query logs (pg_stat_statements, slow query log) | `queries_table`, `writes_table`, `reads_table` | "Change this table schema, what code breaks?" | P2 |
| HTTP access logs (nginx, ALB, API gateway) | `runtime_serves`, frequency metadata | Dead route detection without full APM | P2 |
| Message queue metrics (Kafka consumer lag, SQS depth) | `runtime_consumes`, `runtime_produces` | Verify static pub/sub edges against reality | P2 |
| Error tracking (Sentry, Bugsnag) | `runtime_throws`, error frequency | Prioritize blast radius by error-prone paths | P3 |
| Container orchestration (K8s events) | `runs_on`, `colocated_with` | Infrastructure topology in the graph | P4 |
| Service mesh (Envoy, Istio, Consul) | `runtime_connects_to` | Compare declared vs actual service topology | P4 |
| Continuous profiling (pprof) | `hot_path`, duration metadata | Weight blast radius by performance impact | P4 |

**Key insight:** Static edge with no runtime observation = dead code candidate. Runtime observation with no static edge = undocumented dependency. Both agree = high-confidence relationship.

## Underexploited Capabilities

| Item | Next step |
|------|-----------|
| Edge event log | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Add via community registry when a Go implementation exists |

## Phase 4: Remaining Items

| Feature | Status |
|---------|--------|
| Federated sync (exchange roots, transfer only differing branches) | Planned |
| Merkle-based bisection (binary search on snapshot chain) | Planned |
| Lazy materialization (load only visited subtrees; triggered at ~1M+ edges) | Planned |

## Cross-Repo Validation

### Tier 1: Synthetic Multi-Repo Fixture (built)

3 Go modules at `test/cross-repo/`. Cross-repo edge resolution verified. Remaining dogfooding tests:

- `knowing prove` across repos
- `knowing prove-absent` across repos
- `knowing audit` across repos
- `knowing export` to knowing-viz with cross-repo edges
- `blast_radius` on module-a function showing callers in B and C
- Incremental invalidation across repos

### Tier 1.5: Java Monolith + Frontend (cross-language validation)

**Target:** Spring PetClinic (Java REST API) + React/Vue frontend consuming it.

**What it validates:**
- **Cross-language HTTP edges**: TypeScript `fetch()` → Java `@GetMapping` resolution
- **Java extractor correctness**: Spring Boot annotations, layered architecture (Controller → Service → Repository)
- **API contract detection**: Which frontend components consume which backend endpoints
- **Runtime vs static comparison**: Spin up service, generate OTLP traces, compare observed vs extracted edges
- **Full-stack test scope**: Change Java service → knowing surfaces which frontend tests to run
- **Dead endpoint detection**: REST endpoints defined but never called (static or runtime evidence)
- **Breaking change prevention**: "You're removing `/api/users` but 5 frontend components call it"

**Why useful:**
- Knowing is heavily validated on Go (dogfooding itself), less on Java/TypeScript
- REST API consumption edges aren't validated cross-language yet
- Enables full-stack test selection (backend change → frontend tests)
- Realistic monolith structure (50K LOC, deep call hierarchies, framework-heavy)

**Effort:** Low (4-8 hours to setup, index, validate)  
**Priority:** After session memory persistence (Priority #2). Useful once we have real users requesting Java/cross-language support.

### Tier 2: Grafana Ecosystem (scale validation)

Grafana + Loki + Tempo + Mimir (~1.3M LOC, 4 repos). Validates cross-repo at realistic scale. Run manually, not in CI.

## Production Scale: Permanent Runtime Record

The endgame: knowing with continuous OTLP trace ingestion alongside static analysis. After a year:

- Static edges: ~150K (stable)
- Runtime edges: millions (every observed call path)
- Snapshot chain: 365+ daily snapshots

### Git-Inspired Optimizations

Derived from a deep dive into git's C implementation (pack-objects, commit-graph, refs, bitmaps, merge-ort, shallow clones).

**Medium (1-3 days):**

| Capability | Git Pattern | Why |
|-----------|-------------|-----|
| Filter-based graph materialization | list-objects-filter.c | Push predicates into SQL queries; context retrieval skips irrelevant subgraphs (2-5x speedup) |
| Persistent named snapshot refs | refs/packed-backend.c | `knowing tag stable`, `knowing diff stable..latest`; stored in snapshot_refs table |
| Bloom filters for package changes | commit-graph bloom filters | Per-snapshot bloom filter of changed packages; eliminates edge_events scan during diff |
| Snapshot-graph acceleration file | commit-graph binary format | Binary file with fanout+hashes+metadata avoids N SQL queries for chain walking |
| String interning for package paths | merge-ort strmap | Pointer equality for hot-path comparisons; reduce allocation pressure |

**Architectural (3-5 days):**

| Capability | Git Pattern | Why |
|-----------|-------------|-----|
| EWAH edge-reachability bitmaps | pack-bitmap.c | One bit per edge per snapshot; Diff = XOR + popcount instead of O(E) scan; blast_radius via precomputed reachability |
| XOR-compressed bitmap chains | stored_bitmap.xor | Store consecutive snapshot bitmaps as XOR deltas; 100 snapshots in <10KB vs 125KB |
| Delta-compressed snapshot packs | diff-delta.c, Rabin fingerprint | Sliding-window delta over edge groups; 40-60% smaller sync payloads |
| Promisor nodes (lazy cross-repo) | shallow.c promisor semantics | Record cross-repo edge targets as "promisor" nodes; fetch full data on-demand from source DB |
| Three-way graph merge | merge-ort.c staged computation | Federated sync with conflict awareness: confidence_conflict, provenance_conflict, type_conflict |

### What's Needed at Scale

| Capability | Why |
|-----------|-----|
| Lazy materialization | Load only visited subtrees at millions of edges |
| Merkle bisection | O(log N) snapshot search instead of O(N) |
| Parallel tree hashing | Concurrent bottom-up hash computation for 1M+ edge trees. Current `computeMerkleRoot` is single-threaded; goroutine pool pattern for leaf-level parallelism. |
| Partitioned storage | Static and runtime edges have different lifecycles |
| Runtime edge compaction | Collapse observation history |
| Federated sync | CI instance + production instance exchange diffs |
| Drift alerts | Static analysis vs production traffic divergence |
| Dashboard | Real-time runtime graph visualization |
| Automated compliance reports | Scheduled `knowing audit` with diff against prior |

### Commercial Angle

| Offering | Revenue model |
|----------|--------------|
| knowing Cloud | Managed hosting, per-service pricing |
| Compliance reporting | Automated quarterly audit reports with proofs |
| Federated sync service | Org-wide intelligence sharing |
| Drift detection | Alerts on static/runtime divergence |
| Enterprise dashboard | Cross-repo visualization, team analytics |

## Git-Inspired (Not Yet Built)

| Item | Priority | Effort |
|------|----------|--------|
| Proposed graph overlay (staging area) | P2 | Medium |
| Delta-compressed snapshots | P3 | High |
| N-way hierarchical diff | P3 | Medium |
| Rerere (enrichment conflict resolution) | P4 | Low |
| Transfer protocol (federated sync) | P4 | High |
| Replace/grafts (edge correction) | P4 | Medium |
