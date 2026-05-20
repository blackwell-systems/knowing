# Merkle Tree Algorithms

This document covers the suite of Merkle tree algorithms that exploit knowing's content-addressed graph structure. The foundation is the existing Merkle DAG (see [concepts.md](concepts.md) and [deep-dives.md](deep-dives.md) Section 1), where every node, edge, and snapshot is identified by its SHA-256 hash. The algorithms here build on that foundation to enable cheap invalidation, subgraph caching, incremental recompute, agent trust proofs, federated sync, and semantic change classification.

## Table of Contents

1. [Hierarchical Graph Merkle Trees (Foundation)](#1-hierarchical-graph-merkle-trees-foundation)
2. [Merkleized Subgraph Caching](#2-merkleized-subgraph-caching)
3. [Merkle Diff-Guided Incremental Recompute](#3-merkle-diff-guided-incremental-recompute)
4. [Content-Addressed Context Packs](#4-content-addressed-context-packs)
5. [Merkle Proofs for Agent Trust](#5-merkle-proofs-for-agent-trust)
6. [Federated Graph Sync](#6-federated-graph-sync)
7. [Semantic Change Classification](#7-semantic-change-classification)
8. [Snapshot-Aware Retrieval](#8-snapshot-aware-retrieval)
9. [Merkleized Feedback Validity](#9-merkleized-feedback-validity)
10. [Community Rooting](#10-community-rooting)
11. [Merkle-Based Bisection](#11-merkle-based-bisection)
12. [Proof of Absence](#12-proof-of-absence)
13. [Lazy Materialization](#13-lazy-materialization)
14. [Implementation Priority](#14-implementation-priority)

---

## 1. Hierarchical Graph Merkle Trees (Foundation)

### Shipped Implementation

The hierarchical Merkle tree is implemented in `internal/snapshot/hierarchical.go` and wired into the snapshot manager. The core Merkle construction has been extracted to `github.com/blackwell-systems/merkle-forest` v0.1.1, a standalone library. `BuildMerkleTree` delegates to `forest.Build` with `WithPrefix([]byte("merkle\x00"))` for hash parity, and `BuildHierarchicalTree` delegates to `forest.BuildMultiLevel`. All exported APIs are preserved unchanged (zero-breaking-change refactor, net -44 lines from knowing).

The hierarchical root is the canonical snapshot identity. No flat tree is built; the flat tree was dropped after the hash domain prefix change made backward compatibility moot. The snapshot hash is `ComputeSnapshotHash(hierarchicalTree.Root)`, which applies a `"snapshot\0"` domain prefix to the hierarchical root.

Key results from `bench/merkle-diff/`: `DiffHierarchicalTrees` is 114x faster than flat diff on the knowing repo (11K edges), 517x faster on 100K synthetic edges. Subgraph root lookups are O(1) at 59ns. Build cost overhead is negligible.

### Structure

The hierarchical tree organizes hashes in a tree that mirrors the structure of the codebase:

```
repo_root
  ├── package_root[pkg/A]
  │     ├── file_root[pkg/A/foo.go]
  │     │     ├── symbol_root[pkg/A/foo.go :: FuncX]
  │     │     │     └── edge_root[calls, imports, owns, ...]
  │     │     └── symbol_root[pkg/A/foo.go :: TypeY]
  │     └── file_root[pkg/A/bar.go]
  └── package_root[pkg/B]
        └── ...
```

Hash computation at each level:

```
symbol_root  = merkle_root(sorted(edge_hashes for symbol))
file_root    = merkle_root(sorted(symbol_roots in file))
package_root = merkle_root(sorted(file_roots in package))
repo_root    = merkle_root(sorted(package_roots in repo))
```

### Edge-Type Roots

In addition to the structural tree, each node carries per-edge-type roots:

```
node_edge_type_roots[node_id] = {
  "calls":         merkle_root(sorted(edge_hashes where type=calls)),
  "imports":       merkle_root(sorted(edge_hashes where type=imports)),
  "runtime_calls": merkle_root(sorted(edge_hashes where type=runtime_calls)),
  "owns":          merkle_root(sorted(edge_hashes where type=owns)),
  ...
}
```

This lets callers ask: "did the call graph for this package change, ignoring runtime traces?" or "did imports change but not calls?"

### Benefits

| Granularity | What you can invalidate cheaply |
|-------------|-------------------------------|
| Repo root | Everything (current behavior) |
| Package root | All caches scoped to one package |
| File root | All caches scoped to one file |
| Symbol root | Neighborhood caches for one symbol |
| Edge-type root | Caches split by provenance tier (static vs. runtime) |

---

## 2. Merkleized Subgraph Caching

### Problem

The three most expensive queries in knowing are `blast_radius`, `context_for_task`, and `test_scope`. Each involves graph traversal, scoring, and packing. When the global snapshot root changes (because any edge anywhere changed), all cached results are invalidated, even if the query's relevant subgraph is untouched.

### Design

Cache results against the root hash of the subgraph they depend on, not the global root:

```
subgraph_root(nodes) = merkle_root(sorted(
  edge_hashes for all edges where source ∈ nodes OR target ∈ nodes
))

cache_key = hash(query_params || subgraph_root(relevant_nodes))
```

**Invalidation rule:** A cached result is valid as long as `subgraph_root(relevant_nodes)` is unchanged, regardless of what happened elsewhere in the graph.

### Per-query subgraph scoping

| Query | Subgraph scope |
|-------|----------------|
| `blast_radius(symbol)` | All symbols reachable from `symbol` via `calls`, `imports`, `overrides` |
| `context_for_task(task, seeds)` | All symbols within radius 2 of the seed set |
| `test_scope(files)` | All test symbols that transitively depend on the changed files |

### Benefit

A one-line change in `pkg/util` invalidates only caches whose subgraph scope includes `pkg/util`. Queries scoped entirely within `pkg/api` remain valid and are served instantly from cache.

---

## 3. Merkle Diff-Guided Incremental Recompute

### Problem

After a commit, the current pipeline rebuilds several derived indexes: community detection (Louvain), equivalence class scoring, HITS centrality, and the traversal cache. All of them run every time, even when only a small part of the graph changed.

### Design

Use root comparisons at each tree level to decide which derived indexes need rebuilding:

```
diff_roots(old_root, new_root) -> changed_packages[]
```

The algorithm descends the tree, stopping at the first level where roots match (no change beneath that node). Only differing subtrees are traversed.

**Decision table:**

| What changed | Skip | Rebuild |
|---|---|---|
| Only `runtime_calls` edges changed | Static context indexes, Louvain | Runtime edge scoring, runtime-aware retrieval |
| Only one package root changed | Cross-package HITS, global communities | Local community detection for that package |
| Only doc-comment edges changed | Call graph indexes, blast radius | BM25 index, equivalence class doc scores |
| Only `owns` edges changed | Everything retrieval-related | Ownership routing, blame attribution |
| Multiple package roots changed | None | Full Louvain, full HITS |

### Concrete impact

For a typical one-file edit, only one package root changes. Community detection for that package is local and runs in milliseconds. Global Louvain (which runs on hundreds of nodes) is skipped. HITS centrality recomputes only for nodes in the changed package and their direct neighbors.

---

## 4. Content-Addressed Context Packs

### Shipped Implementation

`ContextBlock` (returned by `context_for_task`) now carries a `PackRoot` field computed by `computePackRoot()` in `internal/context/context.go`. The shipped hash function:

```
PackRoot = hash(task_normalized + sorted(selected_node_hashes))
```

This is a subset of the full design below. Benchmark: 5 queries with 2 unique tasks produced exactly 2 unique PackRoots (perfect dedup). Community roots verified distinct per package set on live graph. See `bench/merkle-diff/FINDINGS-context-packs.md`.

### Full Design

A context pack is the output of `context_for_task`: the scored, packed set of symbols and edges returned to an agent. It is expensive to produce and highly reproducible given the same inputs.

The full `ContextPackRoot` design (target for a later iteration):

```
ContextPackRoot = hash(
  task_normalized,     // lowercased, punctuation-stripped task description
  snapshot_root,       // graph state at pack-time
  selected_node_ids[], // sorted list of included symbol hashes
  selected_edge_ids[], // sorted list of included edge hashes
  scoring_config       // hash of scoring weights, equivalence class version
)
```

### What this enables

**Deduplication across agents:** Two agents working on the same task against the same graph snapshot receive the same pack. Compute once, serve twice.

**Reproducible cites:** An agent can cite a `ContextPackRoot` in a code review comment. The reviewer can replay the exact pack that the agent used and inspect the scoring decisions.

**Cross-session replay:** A resumed session can load the context pack from the previous session and pick up exactly where it left off, without re-running retrieval.

**Pack comparison:** Compare context packs between two snapshots to show which symbols entered or left the retrieval set as the codebase evolved.

### Storage

Context packs are stored in the L2 computation cache (see deep-dives.md Section 12) keyed by `ContextPackRoot`. They are evicted when the snapshot root they were computed against is no longer referenced by any live agent session.

---

## 5. Merkle Proofs for Agent Trust

### Problem

An agent claims: "function A calls function B in this codebase." How can a reviewer, CI system, or another agent verify this without re-running the full extractor?

### Design

A Merkle proof is a path from a leaf hash to a known root hash:

```
proof_path = [
  edge_hash,           // the specific edge being proved
  sibling_hash_1,      // sibling at edge-leaf level
  symbol_root,         // symbol containing this edge
  sibling_hash_2,      // sibling at symbol level
  file_root,           // file containing this symbol
  sibling_hash_3,
  package_root,        // package containing this file
  sibling_hash_4,
  repo_root            // known, externally verifiable snapshot root
]
```

A verifier recomputes the chain from `edge_hash` up to `repo_root` using only the proof path. If the recomputed root matches the known root, the edge existed in that snapshot.

### Use cases

| Use case | How proofs help |
|----------|----------------|
| Code review | Agent cites proof path; reviewer verifies caller relationship existed at review time |
| CI enforcement | Policy check proves a security-sensitive edge (e.g., `accesses_secret`) does or doesn't exist |
| Multi-agent coordination | Agent A proves to Agent B that a symbol exists and hasn't changed since both started |
| Compliance audit | Prove that data-classified symbol had no outbound `calls` edges to external services in snapshot X |

### Proof size

For a tree of depth 4 (repo, package, file, symbol) with binary branching, a proof path contains at most `4 * log2(max_siblings)` hashes. For a repo with 1000 packages, 50 files per package, 100 symbols per file: `4 * 10 = 40` hashes per proof. At 32 bytes each, a proof is under 1.3 KB.

---

## 6. Federated Graph Sync

### Problem

A developer's local knowing instance has indexed their working repo. The CI instance has indexed the same repo plus its dependencies. A remote team member's instance has a week-old index. Today, there is no efficient way to synchronize these graphs.

### Design

Federated sync uses the hierarchical tree to exchange only what differs:

```
1. Exchange repo_root hashes.
   If equal: graphs are identical. Done.

2. If not equal: exchange package_root lists.
   Identify differing package roots.

3. For each differing package: exchange file_root lists.
   Identify differing file roots.

4. For each differing file: exchange edge hashes (or full edges).
   Apply the diff.
```

This is the same protocol as git's pack-protocol, applied to graph structure instead of file trees.

### Sync scopes

| Scenario | Sync cost |
|----------|-----------|
| Local dev to CI (one new commit) | One package subtree |
| Developer A to Developer B (same repo, different working branches) | Changed packages only |
| Full team sync (remote instance, one week stale) | Changed packages since last sync |
| Cross-repo dependency (one library updated) | That library's package subtrees |

### Security

A receiver never has to trust a sender's graph content. The receiver recomputes the root hash from all received edge hashes and verifies it matches the sender's claimed root. Any tampering produces a hash mismatch.

---

## 7. Semantic Change Classification

### Design

A Merkle diff between two snapshots produces a structured change report. The edge-type roots enable classification of what kind of change occurred:

```
classify_diff(old_snapshot, new_snapshot) -> ChangeClass
```

**Change classes:**

| Edge-type roots changed | Edge-type roots unchanged | Classification |
|------------------------|--------------------------|----------------|
| `calls`, `imports` | `runtime_calls` | Structural (static) change |
| `runtime_calls` | `calls`, `imports` | Production drift (static vs. runtime divergence) |
| doc-comment edges | All call/import edges | Documentation-only change |
| `owns` | Everything else | Ownership reassignment |
| `calls` and `runtime_calls` | `imports` | Active behavioral change (both static and runtime affected) |

### Product signals

These classifications become first-class outputs:

- **"Only calls changed"**: behavioral impact; run blast radius analysis.
- **"Only doc changed"**: retrieval impact only; no functional risk.
- **"runtime_calls changed but static unchanged"**: production drift signal; a service is calling something in production that static analysis doesn't show.
- **"Only ownership changed"**: routing/notification impact; no code change.

### Integration points

- `knowing export --diff` emits a structured `ChangeClass` alongside the Merkle diff.
- The planned Semantic PR Diff (deep-dives.md Section 14) uses change classification to filter PR comments by impact type.
- CI can gate on `ChangeClass` to require different review policies for structural vs. documentation changes.

---

## 8. Snapshot-Aware Retrieval

### Design

The retrieval pipeline currently scores symbols by recency, authority (HITS), density, feedback, and equivalence class match. Snapshot-aware retrieval adds two more signals derived from the hierarchical tree:

**Stability signal:** Prefer symbols whose neighborhood root has been stable across recent snapshots. A symbol whose call graph hasn't changed in 10 commits is a reliable retrieval anchor.

```
stability_score(symbol) = 1.0 - (
  count(snapshots where symbol_root changed) /
  total_snapshots_in_window
)
```

**Activity signal:** Boost symbols in subgraphs that have changed recently, weighted by how close the change was to the symbol.

```
activity_score(symbol, task) = recency_weight * (
  1 / hop_distance_to_nearest_changed_symbol
)
```

### Context pack comparison

Given two `ContextPackRoot` values from different snapshots, the system can show:

- Symbols that entered the context pack (newly relevant).
- Symbols that left the context pack (no longer relevant after the change).
- Symbols whose scores changed significantly.

This makes retrieval history inspectable: "why did this symbol stop appearing in context after commit X?"

---

## 9. Merkleized Feedback Validity

### Shipped Implementation (v0.5.0)

**Status:** Implemented in `internal/store/feedback.go`, migration 014, wired into MCP server and context engine.

**What shipped:**
- `neighborhood_root BLOB` column added to feedback table via migration 014
- `RecordFeedback` stores SubgraphRoot of symbol's package at feedback time
- `FeedbackBoosts` accepts optional `neighborhoodRoots` map: when provided, only feedback entries where `neighborhood_root` matches the current package root are counted
- `computeNeighborhoodRoot` helper in MCP server computes package root for a symbol
- Context engine passes neighborhood roots to `FeedbackBoosts`, enabling automatic expiration
- Performance: 11% overhead (255µs baseline -> 284µs per 100 symbols with neighborhood validation)

### Problem

User feedback ("this symbol was useful for task Y") degrades over time as code changes. Previously, feedback weight decayed by a fixed time-based formula. But a symbol can change structurally (its call graph changes) while its name and file path remain the same.

### Design

Feedback is valid only when both of the following hold:

1. **Symbol hash unchanged:** The symbol's content (name, kind, source hash, package) is the same as when the feedback was recorded.
2. **Neighborhood root unchanged:** The symbol's immediate neighborhood (the package it belongs to) has the same SubgraphRoot as when the feedback was recorded.

```
feedback_valid(feedback_record, current_snapshot) = (
  current_symbol_hash == feedback_record.symbol_hash
  AND
  current_neighborhood_root == feedback_record.neighborhood_root
)
```

If `symbol_hash` changed: the symbol was rewritten; feedback is fully invalidated (entry ignored).
If `symbol_hash` unchanged but `neighborhood_root` changed: the symbol's package changed (new callers, removed callees, any edge modification in the package); feedback is fully invalidated.

The current implementation treats neighborhood change as binary (valid/invalid). Future iterations may implement partial invalidation (reduced weight instead of zero).

### Expiration events

| Event | Feedback effect |
|-------|----------------|
| Symbol content changed | Full invalidation (symbol hash mismatch) |
| New callers added to package | Full invalidation (neighborhood root changed) |
| Callee removed from package | Full invalidation (neighborhood root changed) |
| Any edge in symbol's package changed | Full invalidation (neighborhood root changed) |
| Only doc-comment changed (no edges) | No invalidation (neighborhood intact) |
| Unrelated package changed | No invalidation (neighborhood root unchanged) |

---

## 10. Community Rooting

### Shipped Implementation

The `communityInfo` struct in `internal/mcp/communities.go` now carries `MerkleRoot` (string) and `Packages` ([]string) fields. Each community detected by Louvain receives a Merkle root computed from the packages it spans. The `communities` MCP tool (action: list) returns `merkle_root` and `packages` per community entry. Community roots verified distinct per package set on live graph (see `bench/merkle-diff/FINDINGS-context-packs.md`).

### Design

Louvain community detection assigns every symbol to a community. Each community gets its own Merkle root:

```
community_root[C] = merkle_root(sorted(
  edge_hashes for all edges where source ∈ C AND target ∈ C
))
```

Cross-community edges are excluded from community roots and tracked separately:

```
cross_community_root = merkle_root(sorted(
  edge_hashes for all edges where source.community != target.community
))
```

### Benefits

**Cheap invalidation:** When a symbol in community C changes, only `community_root[C]` changes. Caches scoped to other communities remain valid.

**Safe parallelization:** Two agents working on disjoint communities operate on disjoint subtrees. Their changes cannot conflict at the Merkle level. Community roots are the natural unit of agent work partition.

**Community-aware retrieval:** When retrieving context for a task, prefer seeds that share a community root with the task's primary symbols. Community root stability is a proxy for semantic cohesion.

**Invalidation scope:**

| What changed | Invalidated community roots |
|---|---|
| One symbol in community C | community_root[C] |
| An edge between C and D | cross_community_root |
| A symbol moves communities | community_root[old], community_root[new], cross_community_root |

---

## 11. Merkle-Based Bisection

### Design

Merkle-based bisection is the graph equivalent of `git bisect`. Given a snapshot chain and a property that holds in snapshot A but not in snapshot B (for example, "function X has fewer than 5 callers"), the bisection algorithm finds the exact snapshot where the property first changed:

```
bisect(root_A, root_B, predicate) -> snapshot_root

1. If A and B are adjacent snapshots: return B (first snapshot where predicate flipped).
2. mid = snapshot at midpoint of chain between A and B.
3. If predicate(mid): recurse on (mid, B).
4. Else: recurse on (A, mid).
```

The graph structure enables early termination: if `package_root[pkg/X]` is identical in A and mid, no change relevant to `pkg/X` occurred in that half. Skip it.

### Use cases

- "When did this package first accumulate more than 50 callers?" (architecture drift detection)
- "Which commit introduced a `runtime_calls` edge from service A to service B?" (production dependency audit)
- "When did test coverage drop below 60% for this package?" (coverage regression hunt)

---

## 12. Proof of Absence

### Design

A standard Merkle proof proves that an edge IS in the tree. A proof of absence proves that an edge is NOT in the tree, without enumerating all edges.

This requires an ordered Merkle tree (similar to a Merkle Patricia trie). Edge hashes are inserted in sorted order. To prove that edge hash `H` is absent:

1. Find the two adjacent edge hashes `H_prev < H < H_next` that are present in the tree.
2. Provide Merkle proofs that `H_prev` and `H_next` are in the tree and are adjacent (no hash between them).
3. Since the tree is ordered and `H_prev` and `H_next` are adjacent, `H` cannot exist.

### Use cases

| Use case | What is proved absent |
|----------|----------------------|
| Security audit | Function X has no `runtime_calls` edge to an external service |
| Compliance | PII-classified symbol has no outbound `calls` edge to a logging function |
| Agent confidence | The `blast_radius` result is complete: no caller edges were missed |
| Access control | Service A has no `accesses_secret` edge to secret K in this snapshot |

---

## 13. Lazy Materialization

### Design

In the current implementation, the full graph is loaded into memory on startup. For large repos (100K+ symbols), this is expensive, and most of it is irrelevant to the task at hand.

Lazy materialization uses the hierarchical tree to load only the subtrees visited during a retrieval walk:

```
1. Load repo_root and package_root list (lightweight index).
2. On first access to package P: load package_root[P] and its file_root list.
3. On first access to file F in P: load file_root[F] and its symbol list.
4. On first access to symbol S: load symbol's edges.
```

Each level is loaded on demand and cached for the duration of the session.

### Cost model

For a 100K-symbol graph spanning 500 packages, a task that touches 3 packages:

| Step | Symbols loaded | % of graph |
|------|---------------|------------|
| Repo index (package roots) | 500 package roots | < 1% |
| 3 package subtrees | ~600 symbols | 0.6% |
| Traversal spillover (callee packages) | ~200 additional symbols | 0.2% |
| **Total** | **~800 symbols** | **~0.8%** |

This enables knowing to scale to very large repos without increasing per-query latency or memory footprint.

### Integration with subgraph caching

Lazy-loaded subtrees are exactly the subgraph scope for cache keying (see Section 2). The set of subtrees visited during a query is recorded as the cache dependency set. When any of those subtrees change (their roots change), the cache entry is invalidated.

---

## 14. Implementation Priority

These algorithms build on each other. The hierarchical tree structure (Phase 1) is a prerequisite for everything else.

### Phase 1: Hierarchical Tree Structure (Shipped)

**Status:** Implemented in `internal/snapshot/hierarchical.go`, wired into the snapshot manager. Benchmark harness at `bench/merkle-diff/`.

**What shipped:**
- `BuildHierarchicalTree`: builds repo root -> package roots -> edge-type roots -> edge leaves from edge inputs with package and edge-type metadata.
- `DiffHierarchicalTrees`: compares package roots only; O(packages) instead of O(edges). 114x faster on the knowing repo (11K edges), 517x on 100K synthetic edges. Subgraph root lookups: 59ns.
- `DiffHierarchicalTreesWithOptions`: adds `DiffOptions` with `PackageFilter []string` and `MaxChanges int` cap. Matches git's pathspec filtering and early-exit from `tree-diff.c:462`.
- `SubgraphRoot`: O(1) cache key for any set of packages.
- `EdgeTypeRoot`: single-lookup answer for "did call edges change?"
- `ContextPackRoot`: enables content-addressed context pack deduplication.
- Flat tree dropped; hierarchical root is the canonical snapshot hash (wrapped with `ComputeSnapshotHash` for domain-type safety).
- Hash domain prefixes (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) applied to all hash computations. Cross-type hash collisions are now structurally impossible.

**Note:** The Phase 1 spec described `symbol_root` and `file_root` levels. The shipped implementation uses two levels (package and edge-type) rather than four. The intermediate file and symbol roots may be added in a later iteration if lazy materialization (Phase 4, Section 13) is implemented.

**MED fixes shipped alongside Phase 1:**
- `extractPackagePath` returns an error on malformed qualified names instead of silently grouping them under `_root`. (`internal/snapshot/manager.go`)
- In-process LRU cache on `SQLiteStore.GetNode` and `SQLiteStore.GetEdge` (50K entries, `sync.Map`). (`internal/store/sqlite.go`)
- `GarbageCollectFull` runs a reachability sweep after pruning old snapshots. Calls `DeleteNodesNotIn` / `DeleteEdgesNotIn` to prune orphaned rows. Returns `GCStats`. (`internal/snapshot/gc.go`)
- Daemon lockfile at `<db_path>.lock` prevents multiple daemon instances. (`internal/daemon/lockfile.go`)
- `knowing fsck` CLI integrity checker. (`cmd/knowing/fsck.go`, `internal/snapshot/verify.go`)
- `indexed_at` epoch column on `nodes` and `edges` (migration 011).
- `VerifyNodeHash` / `VerifyEdgeHash` in `internal/types/verify.go`.
- `PRAGMA integrity_check` via `IntegrityCheck` method on `SQLiteStore`.

**Why this enables everything else:** Every other algorithm requires package-level roots. Without them, subgraph caching degrades to global cache invalidation (no better than today), incremental recompute has no granularity signal, and community rooting has no tree to attach to.

### Phase 2: Content-Addressed Context Packs + Community Rooting + Subgraph Cache (Shipped)

**Status:** All deliverables shipped.

**Shipped:**
- `PackRoot` field on `ContextBlock` (computed by `computePackRoot()` in `internal/context/context.go`): deterministic hash of normalized task + sorted selected node hashes. Verified: 5 queries, 2 unique tasks = 2 unique PackRoots (perfect dedup).
- `MerkleRoot` and `Packages` fields on `communityInfo` in `internal/mcp/communities.go`: each Louvain community carries a Merkle root over the packages it spans. Community roots verified distinct per package set on live graph.
- Modular community detection: `Algorithm` interface and registry in `internal/community/`. Louvain (`louvain`, `louvain-fine`) and label propagation (`label-propagation`) registered. `knowing export --algorithm` and `communities` MCP tool accept the algorithm parameter.
- `SubgraphCache` in `internal/cache/subgraph.go`: thread-safe, TTL-bounded (default 1h), max-entries (default 10K) cache keyed by `types.Hash`. Random eviction at capacity. `InvalidatePackages` evicts entries for changed packages using the hierarchical tree. `Stats()` returns hits/misses/size/evictions.
- `context_for_task` caching: cache key from `SHA-256("task\x00" + normalized_task)`. Hit = skip entire retrieval pipeline. Miss = full retrieval, then cache result.
- `blast_radius` caching: cache key from `SHA-256("blast_radius\x00" + targetHash + SubgraphRoot(package))`. Hit = skip BFS traversal.
- `test_scope` caching: cache key from `SHA-256("test_scope\x00" + sorted_files + SubgraphRoot(affected_packages))`.
- Daemon invalidation: after each index run, `DiffHierarchicalTrees(prevTree, newTree)` identifies changed packages, `ResultCache.InvalidatePackages` evicts only stale entries. Unchanged code stays cached.

**The full cache chain:** file save -> re-index -> hierarchical diff -> selective eviction -> next query hits cache or recomputes. Queries against unchanged code are free.

### Phase 3: Incremental Recompute (Shipped)

**Status:** All 11 items shipped (F1-F3, P1-P8).

**Foundation shipped:**
- **F1: Graph notes table** (`internal/store/migrations/012_add_notes.sql`): general-purpose `(object_hash, key, value)` metadata layer. 6 GraphStore methods. Never affects Merkle computation.
- **F2: IncrementalAlgorithm interface** (`internal/community/algorithm.go`): `DetectIncremental(g, previous, changedNodes)` on Louvain (6.9x) and label propagation (38.4x). Benchmarked at `bench/community-detection/`.
- **F3: Scoped FTS rebuild** (`internal/store/sqlite.go`): `RebuildFTSForPackages(ctx, packages)` scopes BM25 index rebuild to changed packages. 2.9x faster than full rebuild.

**Features shipped:**
- **P1: Community assignment persistence**: `SaveAssignments`/`LoadAssignments` via notes table. `BatchPutNotes` for 21x faster bulk writes. Communities survive daemon restart.
- **P2: Context pack persistence**: three-layer cache (SubgraphCache 42ns -> notes table 1.2ms -> cold retrieval). Cross-session replay verified. Snapshot-validated staleness detection.
- **P3: Incremental Louvain e2e**: daemon wired: diff -> ChangedPackages -> load previous -> DetectIncremental -> delta-save. Full cycle: 2.5ms.
- **P4: Incremental HITS/BM25**: daemon calls `RebuildFTSForPackages` with changed packages from Merkle diff after each re-index.
- **P5: Context pack deduplication**: `pack_root` parameter on `context_for_task`. Agent passes prior PackRoot, gets "unchanged" (165 bytes) instead of full payload (2-30KB). 93-99% byte savings.
- **P6: Context pack comparison**: `CompareContextPacks` returns added/removed/common symbols. Answers "what changed in what this agent would see?"
- **P7: Semantic change classification**: `ClassifyChanges` returns Behavioral/Structural/RuntimeDrift/MetadataOnly based on which edge-type roots changed. Agents decide whether to re-query based on change kind.
- **P8: Delta-save community assignments**: `SaveChangedAssignments` writes only the delta. 5.0x e2e speedup (12.6ms -> 2.5ms).

**The full pipeline:** file edit -> re-index -> hierarchical diff -> scoped cache invalidation -> incremental community detection -> delta-save -> scoped FTS rebuild -> context pack persistence -> PackRoot dedup -> semantic change classification.

### Phase 4: Proofs, Sync, Bisection, and Advanced Features (In Progress)

**Status:** Partially complete. Merkle proofs, proof of absence, and merkleized feedback validity shipped; remaining items planned.

**Scope:** Merkle proofs for agent trust, federated sync protocol, bisection, proof of absence, lazy materialization, snapshot-aware retrieval, merkleized feedback validity, semantic change classification.

**Shipped:**
- `GenerateProof` and `VerifyProof` in `internal/snapshot/proof.go`: Merkle proof path generation and verification API (72µs generate, 1.2µs verify).
- Proof of absence (`knowing prove-absent`): finds the two adjacent sorted leaves that bracket the missing edge hash. No tree restructuring was needed; the sorted binary tree already provides the ordering invariant. Both neighbor inclusion proofs are verified against the same root.
- `knowing audit`: compliance report with integrity check, edge inventory, and Merkle proofs in single JSON artifact.
- Merkleized feedback validity (v0.5.0): `neighborhood_root` stored on feedback records, automatic expiration when package changes. 11% overhead (255µs -> 284µs per 100 symbols).

**Remaining deliverables:**
- Federated sync protocol (root exchange, subtree transfer).
- Bisection API: `knowing bisect --predicate "callers(X) > 5" <snapshot_A> <snapshot_B>`.
- Lazy subtree loader replacing eager full-graph load.
- Stability and activity signals wired into retrieval scoring.
- `classify_diff` output in `knowing export --diff`.

**Why next:** Phase 3 (incremental recompute) is complete. The tree structure is stable. Proofs, sync, and bisection build on the shipped infrastructure. Federated sync requires team adoption. Bisection is most useful once the snapshot chain is dense with real usage data.

---

## Relationship to Existing Architecture

| Existing component | How Merkle algorithms extend it |
|---|---|
| Snapshot hash | Extended by hierarchical `repo_root` (Phase 1); flat tree dropped |
| Computation cache | SubgraphCache keyed by Merkle package roots (Phase 2); three-layer cache with notes persistence (Phase 3) |
| Community detection | Modular algorithm registry (Phase 2); incremental detection with delta-save (Phase 3, 6.9x/5.0x) |
| Context retrieval | PackRoot dedup (Phase 3 P5, 99% savings); persistent packs (Phase 3 P2); pack comparison (Phase 3 P6) |
| FTS/BM25 index | Scoped FTS rebuild for changed packages (Phase 3 F3, 2.9x) |
| Semantic PR Diff | `ClassifyChanges` returns Behavioral/Structural/RuntimeDrift/MetadataOnly (Phase 3 P7) |
| Feedback scoring | Merkleized feedback validity with `neighborhood_root` (Phase 4, shipped v0.5.0) |
| Retrieval pipeline | Stability + activity signals (Phase 4, planned) |
