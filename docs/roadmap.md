# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **Session memory persistence** | SessionTracker is ephemeral. Persist session working sets to SQLite so resumed sessions compound. | Medium |
| 3 | **`knowing stats`** | Show session value: context calls, symbols served, feedback rate. | Low |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| `knowing stats` | Cumulative session value: context calls, symbols served, feedback rate, token savings. | P2 |
| Cross-repo awareness for non-Go extractors | TypeScript, Python, Rust, Java, and C# extractors use the local repo URL for all targets. Only the Go extractor has `inferRepoURL` with stdlib detection. | P2 |
| Staleness reporting | `knowing stale` reports stale edges from changed files since last snapshot. | P2 |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. | P2 |
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges. | P3 |
| `neighborhood` MCP tool | N symbols most densely connected to X within radius R. | P3 |
| GraphML/Cypher export | `knowing export -format graphml|cypher` for Neo4j, Gephi. | P3 |
| Active project scoping | `set_active_project` / `get_active_project` MCP tools. | P3 |

## Retrieval Pipeline

| Item | Status |
|------|--------|
| More equivalence concepts (84 -> 150+) | Ongoing |
| Session memory persistence | Planned |
| Code-tuned embedding model | Planned (optional) |
| Community-aware retrieval | Planned |

## Edge Type Expansion

| Category | Items |
|----------|-------|
| Runtime | `runtime_queries`, `runtime_connects_to` |
| Contract/API | `implements_endpoint` / `consumes_endpoint`, `implements_rpc` / `consumes_rpc` |
| Ownership | `owned_by` (CODEOWNERS) |
| Static semantic | `extends` / `inherits` / `overrides` |
| Agent workflow | `suggested_for_task` / `used_by_agent` |
| Deployment | `runs_on` / `deployed_by` |

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
| Generation numbers on snapshots | commit-graph generation_number | O(1) ancestry checks ("is snapshot A ancestor of B?"), prune chain walks |
| Auto-GC with threshold | gc_auto_threshold=6700 | Trigger GC when deleted edges exceed threshold; prevents unbounded edge_events growth |

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
