# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Parallel write backend** | SQLite single-writer funnels all extraction results through one goroutine. Even with producer-consumer pipeline, writes are serial. Need parallel write support for large repos. | High |
| 2 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |

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

### P1: Cross-system competitive dimensions

Head-to-head measurements across all competitors on dimensions beyond P@10.

| Dimension | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Query robustness** | Same task rephrased 5 ways: measure P@10 variance per system. RWR should be stable (structural signal); string-matching heuristics should be volatile. A system that gives different answers for "add caching" vs "implement a cache" is unreliable. | Not started | Low (10 tasks x 5 rephrasings, same harness) |
| **Scale curve** | Plot P@10 vs repo size (15K, 150K, 300K, 1M, 3.5M LOC). Show where each system degrades. Expected: knowing flat, codegraph/Aider degrade at scale. Per-repo data exists, just needs visualization. | Data exists (per-repo breakdown in Run 23). Needs plotting. | Low |
| **Determinism** | Run same task 10 times, count unique outputs per system. knowing guarantees 1 (PackRoot). Others likely non-deterministic (FTS ranking ties, random BFS order). | Not started | Low |
| **Time-to-first-result** | End-to-end from `install` to first useful context output. Measures real onboarding friction. knowing: `brew install` + auto-index on first query. codegraph: `npm install -g` + `codegraph init -i`. | Not started | Low |
| **Failure rate by language** | Per-language pass/fail across all systems. knowing: 5/5 languages pass. codegraph: 3/5 (Java, C# fail). Aider: Python/TS only? | Partial data from Run 23 (codegraph failures on Spark/Ocelot) | Low |

### P1: Competitive demolition demo

A head-to-head harness that runs identical queries against all systems and produces a side-by-side comparison. Non-technical audience (investors, potential users) can understand the result in 30 seconds.

**Format:** 5 tasks, 5 systems, table showing: response time, results returned, callers found, callers missed, RAM used, whether it even runs.

**Tasks designed to expose competitor failures:**
- "Find all callers of X" (GitNexus: can't index; grep: false positives; knowing: precise graph traversal)
- "What breaks if I remove this interface?" (Gortex: misses interface implementors; Repomix: dumps 300K tokens)
- "Affected tests for this file change" (no competitor has call-graph test prediction)
- "Index kubernetes and query it" (GitNexus: killed at 60min; CGC: impossible; knowing: 18.6s + 60ms query)
- "Give me 5K tokens of context for this task" (Repomix: 300K tokens, no ranking; knowing: ranked, budgeted)

**Output:** Markdown table + optional terminal recording (asciinema). Publishable on README, blog, Zenodo.

**Status:** All competitive data collected. Needs packaging into polished demo format
(markdown table + terminal recording) for publication.


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
P@10=0.217 (Run 23), 1.63x vs codegraph, 4.5x vs Aider, 11.3x vs grep. Query latency 2ms on k8s (with adjacency cache).

### Retrieval Improvements

| # | Item | Why | Status |
|---|------|-----|--------|
| 7 | **More equivalence concepts** | Only add when a specific task fixture exposes a gap. Must respect Run 22 constraint (no single-word phrases, no generic targets). | On-demand |
| 12 | **Semantic similarity edges (LSH token vectors)** | Lightweight clone/similarity detection: functions with high token overlap get `SIMILAR_TO` edges. Bridges disconnected subgraphs where two functions do the same work but don't call each other. Inspired by codebase-memory's `SEMANTICALLY_RELATED` edges. NOT embedding-based (no model dependency). Compute Jaccard on tokenized function bodies, store edges above threshold. Revert immediately if P@10 regresses. | Experiment (revert if negative) |
| 13 | **`is_entry_point` / `is_exported` node flags** | Tag functions as entry points (main, handlers, CLI commands) or exported (public API). Enables filtering: entry points get higher RWR restart weight. Inspired by codebase-memory's node properties. Low effort, no regression risk. | Experiment |
| 11 | **Feedback parameter sweep (warm-start)** | Session boost (0.20), task memory formula (0.5+score*0.4), decay (7-day linear), top-N (5) are untuned. Only affects real-user compounding. | When users exist |

## Edge Type Expansion

30 edge types shipped. See [Edge Types Reference](architecture/edge-types.md) and [CHANGELOG](CHANGELOG.md) for full details.

| Category | Items | Status |
|----------|-------|--------|
| Runtime | `runtime_queries`, `runtime_connects_to` | P2 |
| Configuration | `configures` (config key to symbol that reads it) | P2 |
| Agent workflow | `suggested_for_task` / `used_by_agent` | P3 |

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
