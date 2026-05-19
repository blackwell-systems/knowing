# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **Session memory persistence** | SessionTracker is ephemeral. Persist session working sets to SQLite so resumed sessions compound. | Medium |
| 3 | **`knowing stats`** | Show session value: context calls, symbols served, feedback rate. | Low |
| 4 | **Notes table** | Metadata without hash invalidation (git-inspired). Feedback, annotations, quality scores. | Low |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| `knowing stats` | Cumulative session value: context calls, symbols served, feedback rate, token savings. | P2 |
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

| Feature |
|---------|
| Merkle proofs (prove relationship existed in snapshot X) |
| Federated sync (exchange roots, transfer only differing branches) |
| Merkleized feedback validity (expires when neighborhood_root changes) |
| Merkle-based bisection (binary search on snapshot chain) |
| Proof of absence |
| Lazy materialization (load only visited subtrees) |
| File-level roots (finer single-file invalidation) |

## Git-Inspired (Not Yet Built)

| Item | Priority | Effort |
|------|----------|--------|
| Notes table | P1 | Low |
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

**What's shipped:** ~67K LOC Go, 25 extractors, 23 MCP tools, 8 MCP resources, 5 wire formats, 10 benchmark harnesses, 84 equivalence classes, multi-language LSP enrichment, hierarchical Merkle tree (Phase 1+2), content-addressed context packs, subgraph cache with daemon invalidation, git-audited integrity layer, modular community detection, React visualization.
