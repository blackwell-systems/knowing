# ADR: Hierarchical Merkle Tree

**Date:** 2026-05-18 (updated 2026-05-19)
**Status:** Shipped (Phases 1-3, Phase 4 partially shipped). Implementation extracted to `merkle-forest` library (v0.1.2).
**Impact:** Foundational. Changes the role of the Merkle tree from integrity mechanism to performance architecture.

## Context

knowing's identity since day one has been content-addressed: every node, edge, and snapshot is a SHA-256 hash. Snapshots are Merkle roots over sorted edge hashes. This gave us integrity, deterministic identity, and efficient equality checks.

But the original Merkle tree was flat: sort all edge hashes, build a binary tree, compare roots. Diffing two snapshots required O(edges) work (building hash sets and scanning for differences). Caching query results was keyed to the global root, so any change anywhere invalidated everything.

The flat model scaled linearly with graph size and couldn't answer the question every downstream system actually asks: "did the part I care about change?"

## Decision

Replace the flat Merkle tree with a hierarchical structure organized by semantic boundaries:

```
repo root
  package roots (one per Go/TS/Python package)
    edge-type roots (calls, imports, implements, references, throws)
      edge leaf hashes
```

The repo root is still a single hash (backward compatible). But now intermediate roots exist at every level, and each root can be compared independently.

## Consequences

**The Merkle tree becomes the query engine, not just an integrity check.**

Before: "Did anything change?" (compare one root, then scan all edges to find what)
After: "Did package X change?" (compare one package root). "Did call edges change?" (compare one edge-type root). "Is my cached blast_radius still valid?" (compare the subgraph root for the relevant packages).

**Performance:**
- Diff: O(packages) with semantically meaningful output (package names, edge types). 281x faster than flat linear scan on the knowing repo (13K edges, 57 packages). 517x on 100K-edge synthetic graphs.
- Subgraph root lookup: 65ns regardless of graph size.
- Build cost: faster than flat construction (3.47ms vs 6.03ms) due to smaller per-group sorts.

**What this unlocked (shipped):**
- Content-addressed context packs: `ContextBlock.PackRoot` = `hash(task_normalized + sorted(selected_node_hashes))`. Same task + same graph = same PackRoot. Enables cache lookup, dedup (93-99% byte savings), cross-session replay.
- Community Merkle roots: each Louvain community carries a root over its packages. Disjoint roots = disjoint work.
- Subgraph caching (93x speedup): `context_for_task`, `blast_radius`, `test_scope` keyed to package subgraph roots. Unchanged code = cached result.
- Daemon invalidation: file save changes one package; only that package's caches invalidated.
- Semantic change classification: behavioral, structural, runtime_drift, metadata_only.
- Merkle proofs (shipped): `knowing prove` (72us), `knowing verify` (1.2us), `knowing prove-absent`. Offline verifiable.
- Merkleized feedback validity (v0.5.0): feedback records store SubgraphRoot; expires automatically when code changes.
- Generation numbers on snapshots: O(1) ancestry checks without chain walking.

**Remaining (planned):**
- Federated sync: exchange roots, descend only differing branches.
- EWAH bitmaps for reachability (from git deep dive).
- Bloom filters for per-snapshot package changes.

**What changed about knowing's identity:**

Content-addressing was always the foundation. But it was primarily an integrity and identity mechanism: "prove this graph state is what you think it is." With the hierarchical tree, content-addressing becomes a computation architecture: "the structure of the identity itself determines what's cheap to compute." The hash tree doesn't just prove state; it organizes every downstream operation.

No competitor uses hierarchical Merkle trees over code relationship graphs. Most code intelligence tools don't even have content-addressed snapshots. The ones that do (if any) use flat hashes. The hierarchical structure is a moat because it requires the content-addressing to be architectural from the start; it can't be bolted on.

## Implementation

- `internal/snapshot/hierarchical.go`: `HierarchicalTree`, `BuildHierarchicalTree` (delegates to `github.com/blackwell-systems/merkle-forest` v0.1.2 via `forest.BuildMultiLevel` with `WithPrefix([]byte("merkle\x00"))`), `DiffHierarchicalTrees`, `DiffHierarchicalTreesWithOptions` (with `DiffOptions`: `PackageFilter`, `MaxChanges`), `SubgraphRoot`, `EdgeTypeRoot`, `ContextPackRoot`
- `internal/snapshot/merkle.go`: `BuildMerkleTree` delegates to `forest.Build`; `combineHashes` retained for proof.go compatibility
- `internal/snapshot/manager.go`: `ComputeSnapshot` builds the hierarchical tree (flat tree was dropped); `extractPackagePath` now returns an error on malformed names; sets `Snapshot.Generation` for O(1) ancestry
- `internal/snapshot/gc.go`: `GarbageCollectFull` with reachability sweep and `GCStats` return type
- `internal/snapshot/verify.go`: integrity verification functions used by `knowing fsck`
- `internal/store/sqlite.go`: in-process LRU cache (50K entries) on `GetNode`/`GetEdge`; `IntegrityCheck` method for `PRAGMA integrity_check`
- `internal/store/gc.go`: `DeleteNodesNotIn`, `DeleteEdgesNotIn` implementations
- `internal/daemon/lockfile.go`: daemon lockfile to prevent multiple instances
- `internal/types/types.go`: hash domain prefixes (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`)
- `internal/types/verify.go`: `VerifyNodeHash`, `VerifyEdgeHash`
- `internal/community/`: `Algorithm` interface, registry, Louvain and label propagation implementations
- `cmd/knowing/fsck.go`: `knowing fsck` CLI command
- `bench/merkle-diff/`: benchmark harness with auto-generated `FINDINGS.md` and `FINDINGS-context-packs.md`
- `docs/architecture/merkle-algorithms.md`: full specification of 13 algorithms across 4 phases

## Alternatives Considered

1. **Keep flat tree, add a separate package-level index.** Would require maintaining two data structures that can drift. The hierarchical tree is one structure that serves both purposes.

2. **Use database indexes instead of Merkle trees for caching.** Database queries are O(log N) per lookup; Merkle root comparison is O(1). At scale, the constant factor matters enormously for hot-path operations like "is this cached?"

3. **Skip the tree entirely, use git commit hashes as cache keys.** Loses semantic granularity. A commit that touches one file invalidates everything. The Merkle tree knows which packages were affected.
