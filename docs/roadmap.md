# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Parallel write backend** | SQLite single-writer funnels all extraction results through one goroutine. Even with producer-consumer pipeline, writes are serial. Need parallel write support for large repos. | High |
| 2 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 3 | ~~**Session memory persistence**~~ | ~~SessionTracker is ephemeral. Persist session working sets to SQLite so resumed sessions compound.~~ **Shipped.** Task memory records top-5 symbols per call in `task_memory` table (SQLite). Persists across restarts. Boost formula: `0.5 + score * 0.4`. | ~~Medium~~ |
| 4 | ~~**`knowing stats`**~~ | ~~Show session value: context calls, symbols served, feedback rate.~~ **Shipped.** | Low |

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

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| ~~`knowing stats`~~ | ~~Cumulative session value: context calls, symbols served, feedback rate, token savings.~~ **Shipped.** | ~~P2~~ |
| ~~Cross-repo awareness for non-Go extractors~~ | ~~TypeScript, Python, Rust, Java, and C# extractors use the local repo URL for all targets. Only the Go extractor has `inferRepoURL` with stdlib detection.~~ **Shipped.** All 5 OOP extractors now have `inferExternalRepoURL` with `"external://{packageName}"` or `"stdlib"` prefix. Python (site-packages + ~50 stdlib modules), TypeScript (bare specifiers), Rust (std::/core::/alloc::), Java (java.*/javax.*), C# (System.*/Microsoft.*). | ~~P2~~ |
| ~~Staleness reporting~~ | ~~`knowing stale` reports stale edges from changed files since last snapshot.~~ **Shipped.** `knowing stale` detects changed files via git diff, looks up stale nodes via `StaleNodesByFiles`, exits 1 when stale (CI-friendly). | ~~P2~~ |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. | P2 |
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges. | P3 |
| `neighborhood` MCP tool | N symbols most densely connected to X within radius R. | P3 |
| GraphML/Cypher export | `knowing export -format graphml|cypher` for Neo4j, Gephi. | P3 |
| Active project scoping | `set_active_project` / `get_active_project` MCP tools. | P3 |

## Retrieval Pipeline

### Cross-System Benchmark Results (v0.6.3, 18 runs, 5 competitors)

97 manual fixtures + 10 SWE-bench across 5 repos (kubernetes, VS Code, flask, cargo, django). Competitive evaluation against 4 systems.

| System | P@10 | R@10 | Index k8s | Query latency | RAM (k8s) |
|--------|------|------|-----------|--------------|-----------|
| **knowing** | **0.230** | **0.284** | **18.6s** | **60ms** | **200MB** |
| Gortex | 0.229 (flask only) | - | 14.2 min | ~600ms | 14GB |
| GitNexus | 0.076 | 0.159 | >60 min (killed) | 612ms | 5.7GB |
| Repomix | N/A (no ranking) | 100% (dumps all) | N/A | N/A | N/A |
| CGC | N/A (no task retrieval) | - | impossible | - | 1.9GB |
| grep | 0.020 | 0.035 | instant | instant | - |

**Statistical significance:**
- knowing vs grep: 11.5x (p<0.0001, d=0.92 very large)
- knowing vs GitNexus: 2.75x (p=0.0003, d=0.50 medium)
- knowing vs Repomix: 48x more token-efficient (4K vs 300K tokens)
- knowing vs Gortex: 1.4x on flask, 46x faster indexing on k8s, 70x less RAM

**Per-repo breakdown:** Django 0.330, Flask 0.321, VS Code ~0.25, Kubernetes 0.184, Cargo 0.123.

**Optimization ceiling diagnosed:** Graph connectivity exhausted. Remaining ~77% miss rate requires feedback compounding (cold-start floor 0.230, compounded ceiling ~0.40).

### Retrieval Improvements (ordered by expected impact)

| # | Item | Why (from benchmark data) | Expected Impact | Status |
|---|------|--------------------------|-----------------|--------|
| 1 | ~~**Ground truth fixture accuracy**~~ | ~~Many fixtures used language-native names not in the DB.~~ **Shipped.** Validated all 571 symbols, 95% match rate. validate-fixtures tool for ongoing verification. | Benchmark accuracy | ~~P0~~ |
| 2 | ~~**FTS/BM25 for non-Go qualified names**~~ | ~~FTS was broken (never populated in CLI mode) and tokenizer split on underscore.~~ **Shipped.** Migration 016 (symbol_name column, 10x weight), tokenchars '_', synchronous rebuild. | +0.006 P@10 | ~~P1~~ |
| 3 | ~~**Language-aware keyword extraction**~~ | ~~Task descriptions say "add a before_request hook" but the keyword extractor doesn't know "before_request" is a symbol name.~~ **Shipped.** `KeywordSet` struct separates Exact/Compounds/Components. Tiered search queries compounds first, components as fallback. Backtick-quoted identifiers are highest priority. Flask P@10 0.321->0.329. | +0.8pp Flask P@10 | ~~P1~~ |
| 4 | ~~**Equivalence classes for non-Go**~~ | ~~The 84 equivalence concepts were hand-tuned for Go patterns.~~ **Shipped (31 language-specific classes).** Python, TypeScript, Rust, Java, Kubernetes vocabulary. Total: 115 equivalence classes. | +2-4pp P@10 on non-Go repos | ~~P2~~ |
| 5 | ~~**Cross-file symbol resolution in Python/TS**~~ | ~~Python/TS calls didn't resolve through imports.~~ **Shipped.** Python buildPythonImportMap + resolveCallTarget (63 edges in flask). TypeScript buildTSImportMap + resolveCallEdgeWithImports (5,684 edges in TypeScript). | +0.013 P@10 | ~~P2~~ |
| 5a | ~~**Cross-file import resolution for Rust/Java/C#**~~ | ~~Same pattern needed for `use crate::module`, `import com.package.Class`, `using Namespace`.~~ **Shipped.** Rust (9,795 edges), Java (`buildJavaImportMap`, import + static import), C# (`buildCSharpImportMap`, using + using static). All use provenance `ast_resolved` / 0.85. Ocelot C# benchmark: P@10=0.14 (cold start, 5 tasks). | Ocelot P@10=0.14 | ~~P2~~ |
| 5b | **Terraform/infrastructure cross-file resolution** | `module.vpc.subnet_id` referencing another .tf file's output. Not in benchmark corpus but needed for real users. | User quality | P3 |
| 5c | ~~**Inheritance propagation**~~ | ~~Child classes couldn't reach parent methods via RWR.~~ **Shipped.** `propagateInheritance` creates `inherits` edges from child to parent methods. 83 edges Flask, 14,539 Django. | **+29% P@10 (Run 13)** | ~~P1~~ |
| 5d | ~~**Deeper call chain extraction (Python)**~~ | ~~Nested calls in arguments, lambdas, and inner functions were missed.~~ **Shipped.** Walk into call arguments, lambda bodies, nested functions. Flask +84% edges, Django +22% edges. | +0.001 P@10 (Run 14) | ~~P1~~ |
| 5e | ~~**Test file deprioritization**~~ | ~~36% of misses were test symbols.~~ **Shipped.** 0.3x penalty for test file symbols; conditional (removed when task mentions testing). | Noise reduction | ~~P1~~ |
| 6 | ~~**Session memory persistence**~~ | ~~The benchmark runs cold (no prior feedback). In real usage, feedback compounds. Session memory persistence carries learning across invocations.~~ **Shipped.** Task memory persists top-5 symbols per call in SQLite; boost `0.5 + score * 0.4`; 7-day linear decay. Cold-start benchmark cannot show improvement (each task unique, runs once); feedback-loop bench independently proves +20pp. | **Shipped** | ~~P2~~ |
| 6a | ~~**Phantom external node filtering**~~ | ~~External nodes from failed LSP enrichment dominated RWR results on repos with unresolved imports (Spark Java: 2282 externals, 63% of nodes).~~ **Shipped.** Filter at `filterNoisySymbols` (seed candidates) and RWR result loop (before scoring). Spark Java P@10 0.00->0.10. | Spark Java fixed | ~~P1~~ |
| 7 | **More equivalence concepts (115 -> 150+)** | Graph-derived aliases help but are limited to the repo's own vocabulary. Need broader coverage of common patterns across ecosystems. | +1-2pp P@10 | Ongoing |
| 8 | **Code-tuned embedding model** | BGE-small-en-v1.5 tested net-negative. A code-tuned model (CodeBERT, UniXcoder) might improve semantic matching between task descriptions and symbol names. | Unknown (needs evaluation) | Planned (optional) |
| 9 | ~~**Community-aware retrieval**~~ | ~~Constrain RWR walk to seed communities. Reduces noise from unrelated packages.~~ **Shipped.** `CommunityFilteredRWR` constrains BFS to seed communities when candidates cluster in 1-3 communities. Falls back to unconstrained walk on diverse queries (4+ communities). Benchmark adapter runs Louvain on index. | Benchmark pending | ~~P2~~ |

## Edge Type Expansion

30 edge types shipped. See [Edge Types Reference](architecture/edge-types.md) and [CHANGELOG](CHANGELOG.md) for full details.

| Category | Items | Status |
|----------|-------|--------|
| **Test coverage** | `tests` (test function to function under test) | **Shipped (P1).** |
| **Ownership** | `owned_by` (CODEOWNERS), `authored_by` (git blame) | **Shipped (P1).** |
| **Documentation** | `documents` (doc comment to symbol) | **Shipped (P2).** |
| **API contracts** | `consumes_endpoint` (HTTP client call), `implements_rpc` / `consumes_rpc` (gRPC) | **Shipped (P2).** |
| **Feature flags** | `gated_by_flag` (function gated by flag check) | **Shipped (P2).** |
| **Deployment** | `deployed_by` (service deployed by CI workflow), `tested_by` (package tested by CI) | **Shipped (P2).** |
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
| ~~Feature flags (LaunchDarkly, Unleash)~~ | ~~`gated_by_flag`~~ | ~~"Disable this flag, what code becomes dead?"~~ | ~~P3~~ **Shipped (static extractor).** |
| ~~CI/CD pipeline (GitHub Actions, Jenkins)~~ | ~~`tested_by`, `deployed_by`~~ | ~~Test coverage as graph edges, deployment topology~~ | ~~P3~~ **Shipped (static extractor).** |
| ~~Git blame/log~~ | ~~`authored_by`, `recently_changed`~~ | ~~Ownership routing, change frequency for ranking~~ | ~~P3~~ **Shipped (P1, authorship extractor).** |
| Container orchestration (K8s events) | `runs_on`, `colocated_with` | Infrastructure topology in the graph | P4 |
| Service mesh (Envoy, Istio, Consul) | `runtime_connects_to` | Compare declared vs actual service topology | P4 |
| Continuous profiling (pprof) | `hot_path`, duration metadata | Weight blast radius by performance impact | P4 |

**Key insight:** Static edge with no runtime observation = dead code candidate. Runtime observation with no static edge = undocumented dependency. Both agree = high-confidence relationship.

## Underexploited Capabilities

| Item | Next step |
|------|-----------|
| Community-aware retrieval | Constrain RWR walk to seed communities |
| Edge event log | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Add via community registry when a Go implementation exists |

## Phase 4: Remaining Items

| Feature | Status |
|---------|--------|
| Merkleized feedback validity (expires when neighborhood_root changes) | **Shipped (v0.5.0).** Feedback records store the SubgraphRoot of the symbol's package. When querying, only feedback matching the current SubgraphRoot is counted, so feedback automatically expires when code changes. Adds 11% overhead (255µs → 284µs for 100 symbols). Migration 014. |
| Merkle proofs and audit primitives | **Shipped.** `knowing prove` (72µs), `knowing verify` (1.2µs), `knowing prove-absent`, `knowing audit` for compliance reports. |
| Federated sync (exchange roots, transfer only differing branches) | Planned |
| Merkle-based bisection (binary search on snapshot chain) | Planned |
| Lazy materialization (load only visited subtrees; triggered at ~1M+ edges) | Planned |
| File-level roots (finer single-file invalidation) | **Deferred.** Package-level granularity is sufficient at current and projected scale (200K+ edges). Scoped FTS rebuild handles the primary use case. Revisit only if a user demonstrates single-file invalidation need. This locks the tree depth at 3 levels and clears the extraction stability gate. |

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

**Quick wins (< 1 day each):**

| Capability | Git Pattern | Why |
|-----------|-------------|-----|
| ~~Generation numbers on snapshots~~ | ~~commit-graph generation_number~~ | ~~O(1) ancestry checks ("is snapshot A ancestor of B?"), prune chain walks~~ **Shipped.** Migration 015, `Snapshot.Generation` field. |
| ~~Auto-GC with threshold~~ | ~~gc_auto_threshold=6700~~ | ~~Trigger GC when deleted edges exceed threshold; prevents unbounded edge_events growth~~ **Shipped.** Threshold 5000 edge_events, keeps 10 snapshots. |

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
