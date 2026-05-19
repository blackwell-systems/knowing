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

#### Foundation (build first, in order)

| # | Item | What | Where | Effort |
|---|------|------|-------|--------|
| F1 | **Notes table** | General-purpose `graph_notes` table (`object_hash, key, value`). Replaces ad-hoc metadata with a unified layer that never affects Merkle computation. Prerequisite for context pack persistence and community assignment storage. Migration 012. | `internal/store/migrations/012_add_notes.sql`, `internal/store/sqlite.go`, `internal/types/interfaces.go` | 3h |
| F2 | **Incremental Algorithm interface** | Add `IncrementalAlgorithm` interface with `DetectIncremental(g, previous, changedNodes)`. Seeded Louvain: start from previous community assignments, only iterate on changed nodes. Label propagation gets it for free. | `internal/community/algorithm.go`, `internal/community/louvain.go` | 6h |
| F3 | **Scoped FTS rebuild** | `RebuildFTSForPackages(ctx, packages)` deletes and re-inserts FTS rows only for nodes in the given packages. Falls back to full rebuild if packages is empty. | `internal/store/sqlite.go` | 2h |

#### Features (build after foundation)

| # | Item | Depends on | What | Effort |
|---|------|-----------|------|--------|
| P1 | **Community assignment persistence** | F1 | Store `node_hash -> community_id` mapping from last detection run in the notes table. Incremental detection seeds from stored assignments. | 2h |
| P2 | **Context pack persistence** | F1 | Store completed ContextBlocks by PackRoot in notes (key=`"context_pack"`). `GetContextPack(packRoot)` retrieves without recomputation. Wire into ForTask for cross-session dedup. | 3h |
| P3 | **Incremental Louvain** | F2, P1 | After re-index, load previous community assignments from notes. Compute changed nodes from `DiffHierarchicalTrees.ChangedPackages`. Call `DetectIncremental` with only changed nodes allowed to move. Store new assignments. | 4h |
| P4 | **Incremental HITS/BM25** | F3 | After re-index, use `ChangedPackages` to scope FTS rebuild. HITS is already per-query (top-200 symbols), so no change needed; caching HITS results per PackRoot is a follow-up. | 2h |
| P5 | **Context pack deduplication** | P2 | Agents reference prior `ContextPackRoot` instead of resending content. MCP tool returns PackRoot; subsequent calls with same root return "unchanged" signal. | 2h |
| P6 | **Context pack comparison** | P2 | Diff two PackRoots: "what changed in the context this agent would see?" Symmetric difference of the two packs' symbol sets. | 2h |
| P7 | **Semantic change classification** | None (uses existing diff) | `ClassifyChanges(diff, oldTree, newTree)` returns `{Behavioral, Structural, RuntimeDrift, MetadataOnly}` based on which edge-type roots changed. | 1h |

Total: ~27h. Foundation (F1-F3) is ~11h. Features (P1-P7) are ~16h.

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
