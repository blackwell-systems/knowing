# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **Session memory persistence** | SessionTracker is ephemeral. Persist session working sets to SQLite so resumed sessions compound. | Medium |
| 3 | **`knowing stats`** | Show session value: context calls, symbols served, feedback rate. | Low |
| 4 | **Notes table** | ~~Metadata without hash invalidation (git-inspired). Feedback, annotations, quality scores.~~ **Shipped** (v0.3.0, Phase 3 F1). | Low |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| `knowing stats` | Cumulative session value: context calls, symbols served, feedback rate, token savings. | P2 |
| `knowing fsck` roster awareness | ~~Classify dangling edges as cross-repo, stdlib, or truly dangling.~~ **Shipped**. Roster stores opened once per verify call. Stdlib heuristic checks import paths. | ~~P1~~ Done |
| Cross-repo method call resolution | ~~Extractor naming mismatch for method calls.~~ **Shipped**. Extractor uses kind="method" for selector calls. LSP enricher resolves definitions across repos via roster. | ~~P1~~ Done |
| Removed-edge diff correctness | ~~`SnapshotDiff` returned empty removed-edge sets because removed edges were deleted before events were recorded.~~ **Shipped** (migration 013). `edge_events` now stores full edge data; `SnapshotDiff` uses COALESCE. | ~~P0~~ Done |
| Synthetic file node storage | ~~Import/reference edges used synthetic file nodes as sources that were never stored.~~ **Shipped**. Go tree-sitter extractor stores file nodes when import edges exist. Zero dangling import sources. | ~~P0~~ Done |
| Phantom external nodes | ~~Stdlib and external call targets produced dangling edges.~~ **Shipped**. Extractor creates `kind="external"` nodes for inferred stdlib/external targets. LSP enricher post-enrichment sweep covers any remaining dangling targets. `knowing fsck` reports 0 errors on a correctly indexed repo. | ~~P0~~ Done |
| Cross-repo awareness for non-Go extractors | TypeScript, Python, Rust, Java, and C# extractors use the local repo URL for all targets. Only the Go extractor has `inferRepoURL` with stdlib detection. Other language extractors need the same treatment to produce correct cross-repo hashes for external targets. | P2 |
| Staleness reporting | `knowing stale` reports stale edges from changed files since last snapshot. | P2 |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. | P2 |
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges. | P3 |
| `neighborhood` MCP tool | N symbols most densely connected to X within radius R. | P3 |
| GraphML/Cypher export | `knowing export -format graphml|cypher` for Neo4j, Gephi. | P3 |
| Active project scoping | `set_active_project` / `get_active_project` MCP tools. | P3 |

## MCP Resources (Shipped)

All 8 resources shipped. Implemented in `internal/mcp/resources.go`, registered in `NewServer`.

| Resource | What it provides |
|----------|-----------------|
| `knowing://report` | Graph size, top kinds, hotspot count, snapshot age |
| `knowing://schema` | Node kinds, edge types, provenance tiers, hash format |
| `knowing://stats` | Counts by repo, kind, and edge type |
| `knowing://repos` | All tracked repos with node/edge counts and last-indexed time |
| `knowing://session` | Context calls, symbols served, cache hits/misses, uptime |
| `knowing://index-health` | Healthy/stale/corrupted status, integrity check |
| `knowing://communities` | Community list with cohesion and Merkle roots |
| `knowing://community/{id}` | Single community detail (resource template) |

## Underexploited Capabilities

| Item | Next step |
|------|-----------|
| Community-aware retrieval | Constrain RWR walk to seed communities |
| Edge event log | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Add via community registry when a Go implementation exists |

## Retrieval Pipeline

| Item | Status |
|------|--------|
| More equivalence concepts (84 -> 150+) | Ongoing |
| Session memory persistence | Planned |
| Code-tuned embedding model | Planned (optional) |
| Community-aware retrieval | Planned |

## Edge Type Expansion

| Category | P1 items |
|----------|----------|
| Runtime | `runtime_queries`, `runtime_connects_to` |
| Contract/API | `implements_endpoint` / `consumes_endpoint`, `implements_rpc` / `consumes_rpc` |
| Ownership | `owned_by` (CODEOWNERS) |
| Static semantic | `extends` / `inherits` / `overrides` |
| Agent workflow | `suggested_for_task` / `used_by_agent` |
| Deployment | `runs_on` / `deployed_by` |

## Merkle Tree Algorithms

**Phase 1 (Shipped):** Hierarchical tree, DiffHierarchicalTrees, SubgraphRoot, EdgeTypeRoot, ContextPackRoot, hash domain prefixes. 114x faster diff, 517x at 100K.

**Phase 2 (Shipped):** SubgraphCache, context/blast/test caching, daemon invalidation, PackRoot, community Merkle roots. 93x cache hit.

### Phase 3: Incremental Recompute

Phase 3 requires foundation work before the features can be built correctly. The foundation items are prerequisites; the features build on them.

#### Foundation

| # | Item | Status | Benchmarked |
|---|------|--------|-------------|
| F1 | **Notes table** | **Shipped** (v0.3.0). Migration 012, 6 GraphStore methods, 8 tests. | N/A |
| F2 | **Incremental Algorithm interface** | **Shipped**. `IncrementalAlgorithm` on Louvain (6.9x) and LP (38.4x). | `bench/community-detection/` |
| F3 | **Scoped FTS rebuild** | **Shipped**. `RebuildFTSForPackages(ctx, packages)`. Falls back to full rebuild if empty. | N/A |

#### Features

| # | Item | Status | Benchmarked |
|---|------|--------|-------------|
| P1 | **Community assignment persistence** | **Shipped**. Save/Load via notes table, BatchPutNotes (21x vs individual inserts). | `bench/community-detection/` (E2E) |
| P2 | **Context pack persistence** | **Shipped**. Three-layer cache: SubgraphCache (42ns) -> notes table (1.2ms, snapshot-validated) -> cold retrieval. Cross-session replay verified. | `bench/merkle-diff/` (persistence) |
| P3 | **Incremental Louvain e2e** | **Shipped**. Daemon wired: diff -> ChangedPackages -> load previous -> DetectIncremental -> save. 11ms full cycle. | `bench/community-detection/` (E2E) |
| P4 | **Incremental HITS/BM25** | **Shipped**. Daemon wires `RebuildFTSForPackages` with changed packages from Merkle diff. | N/A |
| P5 | **Context pack deduplication** | **Shipped**. `pack_root` parameter on `context_for_task`. 93-99% byte savings. | `bench/merkle-diff/` (dedup) |
| P6 | **Context pack comparison** | **Shipped**. `CompareContextPacks` returns added/removed/common symbols between two packs. | |
| P7 | **Semantic change classification** | **Shipped**. `ClassifyChanges` returns Behavioral/Structural/RuntimeDrift/MetadataOnly from edge-type roots. | |

| P8 | **Delta-save community assignments** | **Shipped**. 5.0x e2e speedup (12.6ms -> 2.5ms). Save dropped from 81% to ~4% of cycle. | `bench/community-detection/` (E2E) |

**Phase 3 complete.** All 11 items shipped (F1-F3, P1-P8).

### Phase 4: Proofs, Sync, Bisection

| Feature | Status |
|---------|--------|
| Merkle proofs (prove relationship existed in snapshot X) | Shipped |
| Proof of absence (`knowing prove-absent`: adjacent sorted leaves bracket the missing edge; no tree restructuring needed) | Shipped |
| Federated sync (exchange roots, transfer only differing branches) | Planned |
| Merkleized feedback validity (expires when neighborhood_root changes) | Planned |
| Merkle-based bisection (binary search on snapshot chain) | Planned |
| Lazy materialization (load only visited subtrees) | Planned (triggered when tree construction > 1s or memory > 500MB; estimated at ~1M+ edges. Static analysis alone reaches this at ~6M+ LOC, but OTLP trace ingestion changes the math: a 10-service architecture with a month of traces can produce 200K+ runtime edges on top of 150K static edges, reaching the threshold much sooner.) |
| File-level roots (finer single-file invalidation) | Planned |

## Cross-Repo Validation

Two tiers: a synthetic fixture for CI regression testing, and a real-world ecosystem for scale validation.

### Tier 1: Synthetic Multi-Repo Fixture (build first)

3 small Go modules that import each other. ~500 LOC total, runs in seconds, lives in `test/cross-repo/`.

```
module-a/  (shared library: 50 functions)
module-b/  (imports A, adds its own functions)
module-c/  (imports A and B)
```

| Test | What it proves |
|------|---------------|
| Index all 3 modules | Per-repo isolation, roster management |
| Cross-repo edge resolution | `ModuleToRepoURL` resolves imports across databases |
| `knowing prove` across repos | "Prove module-c calls module-a.Helper at this snapshot" |
| `knowing prove-absent` across repos | "Prove module-b never calls module-c directly" |
| `knowing audit` across repos | All inter-module edges with provenance |
| Incremental invalidation | Change a function in module-a, re-index, verify B and C caches invalidated |

**Why synthetic:** runs in CI, deterministic, controlled edges. Proves the cross-repo product without external dependencies. Every edge is known, so assertions are exact.

**Dogfooding:** validation uses knowing's own tools, not just unit tests:
- `knowing export` feeds knowing-viz: cross-repo edges visible as inter-cluster connections, communities should show 3 distinct module clusters
- `context_for_task` with a task that spans modules: "refactor the shared helper in module-a"
- `blast_radius` on a module-a function: callers should include module-b and module-c symbols
- `flow_between` across repos: path from module-c through module-b to module-a
- `communities`: verify modules detected as separate communities connected by bridge edges

### Tier 2: Grafana Ecosystem (scale validation, run once)

**Target:** Grafana + Loki + Tempo + Mimir (~1.3M LOC, 4 repos, Go + TypeScript)

Real cross-repo edges through `grafana/dskit` and shared pkg/ libraries. At ~200K estimated edges, this validates cross-repo at realistic scale but will not stress tree memory. Run manually, not in CI.

| Milestone | What it proves |
|-----------|---------------|
| Index all 4 repos | Indexer handles ~1.3M LOC, tree construction scales |
| Cross-repo edge resolution | dskit/pkg imports resolved across repo databases |
| Multi-repo community detection | Do Louvain communities align with repo boundaries or cross them? |
| Multi-language extraction | Grafana's TypeScript frontend + Go backend |

**Success criteria:**
- All 4 repos index without error
- Cross-repo edges resolved (non-zero resolver output)
- `knowing audit -proofs` generates valid proofs for cross-repo edges
- Total index time < 5 minutes

## Production Scale: Permanent Runtime Record

The endgame: knowing becomes the permanent record of every relationship your software has ever had, both in code and in production. An ops team runs knowing with continuous OTLP trace ingestion alongside static analysis. After a year of production:

- **Static edges:** ~150K (stable, changes with commits)
- **Runtime edges:** millions (every unique call path, endpoint, queue message observed)
- **Temporal density:** every edge has observation history (appeared, peaked, decayed, disappeared)
- **Snapshot chain:** 365+ daily snapshots, each a Merkle root tied to a deployment

### What Breaks at This Scale

| Problem | Current approach | What breaks | What's needed |
|---------|-----------------|-------------|---------------|
| Tree construction | Full rebuild in memory (~3ms at 12K edges) | OOM at millions of edges | Lazy materialization: store tree in SQLite, load subtrees on demand |
| Snapshot walk | Linear chain traversal | 365 snapshots * O(packages) per diff | Merkle bisection: O(log N) binary search |
| FTS rebuild | Full or package-scoped | Millions of runtime edge descriptions | Incremental FTS append (no rebuild, just insert new) |
| GC | Reachability sweep of all edges | Minutes at millions of rows | Partitioned GC by edge age or provenance tier |
| Proof generation | Linear scan of edge list per package | Slow at 100K+ edges per package | Pre-indexed edge lookup by (package, type) |
| Storage | Single SQLite file per repo | Multi-GB files, WAL contention | Partitioned storage: static edges in one file, runtime in another |

### What Needs to Exist

| Capability | Why |
|-----------|-----|
| **Lazy materialization** | Load only visited subtrees. A proof for one edge shouldn't require the entire tree in memory. |
| **Merkle bisection** | "When did this cross-service dependency first appear?" as O(log N) snapshots, not O(N). |
| **Partitioned storage** | Static edges (change with commits) and runtime edges (change with traffic) have different lifecycles. Separate storage lets each be optimized independently. |
| **Runtime edge compaction** | Collapse observation history: "called 50K times over 90 days" instead of 50K individual observations. |
| **Dashboard** | Real-time visualization of the runtime graph: traffic patterns, dead routes, confidence drift, new dependencies appearing. |
| **Automated compliance reports** | Scheduled `knowing audit` runs with diff against previous audit point. Alert on new cross-service dependencies. |
| **Federated sync** | CI instance indexes code. Production instance ingests traces. They exchange Merkle roots and sync only the diff. |
| **Drift alerts** | "Production traffic shows a call path that static analysis doesn't know about" or "static analysis says this is called, but production hasn't observed it in 30 days." |

### Who Runs This

This is not an individual developer tool. This is the "knowing Cloud" product: a managed service where an ops/platform team connects their repos and their OTLP pipeline. The value proposition is the permanent, verifiable, cryptographically provable record of how their software actually works, both as designed and as observed.

### Commercial Angle

| Offering | Revenue model |
|----------|--------------|
| **knowing Cloud** | Managed hosting, per-service pricing. Connect repos + OTLP endpoint, get a dashboard. |
| **Compliance reporting** | Automated quarterly audit reports with proofs. SOC 2 / ISO 27001 evidence. |
| **Federated sync service** | Coordination layer between team instances. Org-wide intelligence sharing. |
| **Drift detection** | Alerts when production behavior diverges from static analysis. SLA on detection latency. |
| **Enterprise dashboard** | Visualize graph across all repos and services. Team analytics. Dependency governance. |

The open source tool proves the technology. The production-scale offering monetizes the data density that only continuous operation produces.

## Git-Inspired (Not Yet Built)

| Item | Priority | Effort |
|------|----------|--------|
| Notes table | ~~P1~~ **Shipped** (v0.3.0) | Low |
| Proposed graph overlay (staging area) | P2 | Medium |
| Delta-compressed snapshots | P3 | High |
| N-way hierarchical diff | P3 | Medium |
| Rerere (enrichment conflict resolution) | P4 | Low |
| Transfer protocol (federated sync) | P4 | High |
| Replace/grafts (edge correction) | P4 | Medium |

## Strategic Position

knowing is an intelligence versioning system. Git versions files; knowing versions the understanding of code.

The retrieval pipeline uses equivalence classes (not embeddings). Local, deterministic, inspectable, compounds with use.

The hierarchical Merkle tree structures snapshots by semantic boundaries. The identity structure is the query structure: 114x faster diffs, O(1) subgraph roots, 93x cached retrieval, scoped invalidation.

**What's shipped:** ~70K LOC Go, 25 extractors, 23 MCP tools, 8 MCP resources, 5 wire formats, 14 benchmark harnesses, 84 equivalence classes, multi-language LSP enrichment, hierarchical Merkle tree (Phase 1+2+3 complete, Phase 4 in progress), Merkle proofs + proof of absence, `knowing audit`/`audit-diff`/`prove`/`verify`/`prove-absent` (6 compliance CLI commands), content-addressed context packs with three-layer cache and PackRoot dedup, subgraph cache with daemon invalidation, incremental community detection with delta-save, semantic change classification, git-audited integrity layer, modular community detection, React visualization, phantom external nodes (extractor + enricher), zero-dangling graph (fsck 0 errors on correctly indexed repos), migration 013 (edge event data for removed-edge diffs).
