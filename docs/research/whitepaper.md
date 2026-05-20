# The Hierarchical Identity Architecture: Content-Addressing as a Computation Primitive for Software Relationship Intelligence

**Dayna Blackwell, Blackwell Systems**

---

## Abstract

Content-addressing is usually treated as an integrity mechanism: a way to verify that data has not changed. In software relationship intelligence, we show that the structure of content-addressed identity can also become the query execution substrate. By organizing a Merkle tree around semantic boundaries such as package and relationship type, the system turns staleness checks, scoped cache keys, invalidation, diffing, and auditability into structural properties of the identity model itself.

A flat Merkle tree proves state. A hierarchical Merkle tree organizes computation.

When the tree is organized by package and edge type rather than by flat sorted hash, the identity structure itself becomes the query optimization layer. Diffs become O(packages) instead of O(edges). Cache keys become O(1) subgraph root lookups. Invalidation is scoped to the packages that actually changed. Critically, the hierarchical diff's output is semantically meaningful: it names changed packages and edge types, not arbitrary tree positions. This is what makes scoped invalidation possible. The tree does not merely prove state; it organizes computation.

This paper presents both insights together: the original argument (content-addressing solves six structural problems with mutable graphs) and the hierarchical revelation (organizing the Merkle tree by semantic boundaries turns identity into a query engine). Each capability in the system is a structural consequence of the hierarchical identity model, not a feature bolted onto it.

---

## 1. Problem: Agents Need Trustworthy Software Relationship Graphs

AI agents are the most demanding consumer of software relationship intelligence. They operate under token-budgeted context windows, make multiple queries per task, and need confidence signals to prioritize information. They fail in predictable ways when the graph they consult is untrustworthy.

A software system is not just source code. It is a graph of relationships:

- Function A calls function B (static analysis)
- Service X sends requests to service Y at endpoint /users (runtime observation)
- Deployment D references ConfigMap C (infrastructure declaration)
- Type T implements interface I (type system)
- Handler H is registered to route R (framework convention)

These relationships exist across repositories, languages, services, and infrastructure layers. They change over time. They have varying degrees of certainty: a tree-sitter pattern match is less confident than a type-system confirmation; a runtime trace is more confident than either for the specific question "does this actually execute in production?"

Most existing systems do not track these relationships with all of the following properties simultaneously:

- Version history (what did the graph look like Tuesday?)
- Provenance (how was this relationship discovered?)
- Confidence (how certain are we?)
- Cross-boundary awareness (across repos, services, languages)
- Provable currency (is this graph still correct?)

The reason is that most systems use mutable state. And mutable state cannot provide these properties without extraordinary complexity.

The knowing system tracks approximately 13,100 edges in its live codebase, spanning 57 packages and 25 extractor types. The hierarchical Merkle structure operates over this graph in production and is the basis for all benchmark results in this paper. Edge counts vary by run and snapshot; prose uses "~13.1K edges" and exact counts appear only in tables.

---

## 2. Why Mutable Graphs Fail

Every tool that builds a "code graph" or "dependency graph" using a mutable store (Neo4j, PostgreSQL, in-memory structures, SQLite with UPDATE queries) faces six problems that are structural consequences of mutability:

### 2.1 Re-indexing Ambiguity

You re-index a repository. What happens to existing edges?

- **Upsert?** Then you never know if an edge is current or stale (it was written at some point and never explicitly removed).
- **Delete and recreate?** Then concurrent reads see incomplete state mid-rebuild.
- **Merge?** Then you need conflict resolution logic for every edge type.

Every choice requires application-level logic to maintain consistency. Every choice has failure modes. Every production system using mutable graphs has bugs in this logic.

**With content-addressing:** re-indexing the same code produces the same hashes. `INSERT OR IGNORE` is the only write pattern. If the hash already exists, the entity already exists with identical content. No merge logic. No conflict resolution. No ambiguity.

### 2.2 Staleness Detection

How do you know if the graph reflects the current state of the code?

In mutable systems, the answer is metadata: `last_indexed_at` timestamps, version counters, TTL-based expiry. All of these are heuristics. None of them answer the actual question: "is the graph structurally identical to what a fresh index would produce?"

**With content-addressing:** compute the Merkle root of the current edge set. Compare to the stored snapshot hash. If they match, the graph is provably current. If they differ, the divergence is locatable in the Merkle tree. One comparison, one answer, no heuristics.

### 2.3 Change Attribution

"When did this relationship between service A and service B first appear?"

Mutable graphs answer this only if you added an explicit `created_at` field, remembered to set it correctly on every write path, and never accidentally overwrote it during re-indexing (see Problem 1).

**With content-addressing:** the snapshot chain is the history. Walk backwards from the current snapshot, diffing adjacent pairs, until you find the snapshot where the edge first appears. The diff is comparing two immutable states. The attribution is the commit hash stored on that snapshot. No special fields required; history is the data model.

### 2.4 Snapshot Isolation

A long-running blast-radius computation starts. Halfway through, the indexer runs and updates the graph. The computation now reads a mix of old and new state.

Mutable systems solve this with read transactions, MVCC, or full locks. All add complexity. All have edge cases (long-running transactions holding WAL files open, MVCC bloat, lock contention).

**With content-addressing:** the computation pins a snapshot hash and reads against it. The graph can be updated concurrently by writing new entities (which have different hashes). The computation's reads are against immutable data that cannot change. No locks, no transactions, no MVCC overhead.

### 2.5 Cross-Repo Identity

Repository A has a function. Repository B calls it. Both repositories are indexed independently. How do you ensure the "function" in A and the "callee reference" in B point to the same entity?

Mutable systems need either a global ID service (coordination overhead, single point of failure) or a naming convention (fragile, breaks on rename/move).

**With content-addressing:** both compute `NodeHash = SHA-256("node\0" || repoURL || packagePath || symbolName || symbolKind)`. Same canonical inputs, same hash. Global identity without coordination, without consensus, without a central registry. Two indexers running on different machines at different times produce the same hash for the same symbol.

The `"node\0"` domain-type prefix (and `"edge\0"`, `"snapshot\0"`, `"merkle\0"` for other entity types) eliminates cross-type ambiguity by construction, assuming standard collision resistance of SHA-256. This mirrors git's `"<type> <size>\0<content>"` object header.

### 2.6 Audit Provenance

"Prove that this graph state was derived from these specific source commits."

Mutable systems usually require a separate trusted log, audit table, or event stream to support this proof. Logs are separate from data, can be tampered with, and require trust in the logging system.

**With content-addressing:** the proof is the hash chain itself.

```
Snapshot hash (hierarchical Merkle root)
  -> Package roots (one per package, from edge-type roots)
    -> Edge-type roots (one per package+type, from edge hashes)
      -> Individual edge hashes (leaves)
        -> Source/target node hashes (referenced by edges)
          -> Repo hash + file content hash (referenced by nodes)
            -> Git commit hash (stored on snapshot)
```

Every step is a deterministic computation from content. An auditor can verify any claim by recomputing. The data is the audit trail.

---

## 3. Content-Addressed Relationship Identity

### 3.1 The Insight

Git is a content-addressed graph of source code. This single design decision gives Git every property that made it the dominant version control system:

- Same content always produces the same hash (determinism)
- Any modification changes the hash (integrity)
- Every previous state is retrievable by its hash (history)
- Comparing two states is comparing two hashes (O(1) staleness)
- Concurrent operations on immutable snapshots cannot conflict (isolation)
- The chain from commit to tree to blob is cryptographically verifiable (audit)

These properties are not features bolted onto a mutable store. They are structural consequences of the content-addressing choice. You cannot have a mutable system that provides them without simulating immutability (MVCC, event sourcing, temporal tables), and every simulation is more complex and less trustworthy than the real thing.

**The original thesis:** software relationship intelligence requires the same properties Git provides for source code, and content-addressing is the simplest primitive that provides these guarantees structurally rather than through application-level simulation.

### 3.2 Cross-Repo Identity Without Coordination

Content-addressing removes the need for a central ID service, but it does not remove the need to define canonical repo identity, package paths, symbol names, and symbol kinds precisely. Canonicalization is part of the correctness boundary: if two indexers canonicalize the same entity differently, they will correctly produce different hashes.

The guarantee of global identity depends on deterministic canonicalization. Canonical repo identity is the repository URL, hashed directly via `SHA-256(repoURL)`. This canonicalization must be treated as core infrastructure, not an implementation detail.

---

## 4. Hierarchical Merkle Trees as Query Architecture

### 4.1 From Flat Hash to Semantic Tree

The obvious content-addressed Merkle structure for a set of edges is a flat tree:

```
FlatRoot = MerkleRoot(sort(all edge hashes in repo))
```

This works. It is deterministic, verifiable, and gives you all six properties above. But it throws away information. Every package's edges are interleaved in a flat sorted list. When you want to know "which packages changed?", you must compare all N edges.

The hierarchical tree preserves semantic structure:

```
HierarchicalRoot
  -> PackageRoot(internal/mcp)
    -> EdgeTypeRoot(internal/mcp, calls)
      -> EdgeHash(A calls B)
      -> EdgeHash(A calls C)
    -> EdgeTypeRoot(internal/mcp, throws)
      -> EdgeHash(A throws ErrNotFound)
  -> PackageRoot(internal/store)
    -> EdgeTypeRoot(internal/store, calls)
      ...
```

The structure mirrors the conceptual hierarchy of the codebase: repo contains packages, packages contain relationships of different types. A Merkle root at each level provides a stable identity for the subtree below it.

This alignment between the identity tree and the semantic structure of the code is not accidental. It is the design. And it produces algorithmic consequences that flat content-addressing cannot deliver.

### 4.2 Worked Example

Consider two packages:

```
internal/auth:
  LoginHandler calls ValidateToken
  LoginHandler reads UserRepository

internal/billing:
  ChargeUser calls StripeClient.CreatePayment
```

In the hierarchical tree, `PackageRoot(auth)` is a function of the auth edges only, and `PackageRoot(billing)` is a function of the billing edges only. When a developer changes `ValidateToken` and re-indexes:

- `PackageRoot(auth)` changes (because auth edges changed)
- `PackageRoot(billing)` is unchanged (billing edges are the same)
- `SubgraphRoot(["billing"])` is still valid for cache

An agent querying only billing packages hits the cache even though auth was just re-indexed. The invalidation is scoped to what actually changed, not to the entire graph. No billing-scoped cache entry is evicted. This is not a feature of the cache layer; it is a consequence of the identity model.

### 4.3 The Algorithmic Wins

**Diff becomes O(packages) instead of O(edges).**

When one package changes, its `PackageRoot` changes. The diff of two hierarchical trees identifies the change by comparing P package roots, where P is the number of packages. The diff of two flat trees requires comparing all E edge hashes.

For the knowing codebase (~13.1K edges, 57 packages):

| Operation | Latency |
|-----------|---------|
| Flat linear scan (compare 13,103 edges) | 1.76ms |
| Hierarchical diff (compare 57 package roots) | 6.26us |
| **Speedup (hierarchical vs flat linear scan)** | **281x** |

For a 100K-edge synthetic graph with 100 packages, the speedup over flat linear scan is 517x. Note: a flat Merkle tree with descend-to-localize would also be fast for small changes (O(d * log E)), but would localize changes to arbitrary leaf positions without semantic context. The hierarchical structure enables O(packages) diff with semantically meaningful output: changed package names and changed edge types. This is what makes scoped invalidation and package-aware cache keys possible. See `bench/merkle-diff/FINDINGS.md`.

**Cache keys become O(1) subgraph root lookups.**

A query scoped to packages A and B needs a cache key. In a flat structure, the cache key must encode the content of all edges in A and B. In the hierarchical structure, the cache key is `hash(PackageRoot(A) || PackageRoot(B))`. Two lookups, one hash, one cache check. The cache key changes if and only if the queried packages changed.

Raw lookup latency: 42ns (measured on the live codebase, `bench/merkle-diff/FINDINGS-phase2-cache.md`).

**Invalidation is scoped to changed packages.**

When the daemon detects a file change, it runs `DiffHierarchicalTrees` to find which packages changed. It then evicts only the cache entries scoped to those packages. Everything else remains warm.

Total diff plus invalidation overhead per re-index: ~6us. The re-index itself (parsing, SQLite writes) dominates at ~149ms. The invalidation overhead is invisible (`bench/merkle-diff/FINDINGS-phase2-cache.md`).

**Queries against unchanged code are fast-path cache hits.**

With a measured cache hit rate of ~20% for varied queries (60% for exact repeats), the expected session-level speedup is approximately 0.2 * 93 + 0.8 * 1 = 19x. The 93x figure is the warm-path peak when a query hits a primed cache entry:

| Condition | Median |
|-----------|--------|
| Cache disabled | ~160ms |
| Cache enabled (primed) | ~1.7ms |
| **Peak speedup (warm path)** | **93x** |

The identity structure itself becomes the query optimization layer.

### 4.4 Why the Hierarchy Must Be Architectural from the Start

The guarantees hold only once the hierarchical Merkle structure is the exclusive write path. If the tree is only sometimes hierarchical, the package roots are not authoritative. A cache lookup against `PackageRoot(internal/mcp)` is only valid if every writer that touches `internal/mcp` routes their writes through the hierarchical tree. Any mutable write that bypasses the tree silently invalidates the cache guarantee without changing the root.

This is the same reason you cannot bolt content-addressing onto git after the fact. The guarantee is not "we hashed these things." It is "we hashed everything, using this algorithm, with no exceptions." The exception-freeness is the guarantee. It must be architectural from the start.

The knowing implementation enforces this at the write layer: every edge insertion updates the hierarchical tree. There is no path to write an edge without updating the package root. See `internal/snapshot/hierarchical.go`.

---

## 5. Formal Model and Assumptions

### 5.1 Entity Definitions

**Node** (a symbol declaration):
```
RepoHash = SHA-256(repoURL)
NodeHash = SHA-256("node\0" || repoURL || packagePath || symbolName || symbolKind)
```

Identity depends on logical position (repo, package, name, kind), not physical location (file, line number). Moving a function between files does not change its hash. Renaming it does (creating a new entity; the old entity's edges become stale, detectable via snapshot diff).

**Edge** (a directed relationship):
```
EdgeHash = SHA-256("edge\0" || sourceHash || targetHash || edgeType || provenance)
```

Identity includes provenance. The same structural relationship (A calls B) observed by tree-sitter AST analysis, LSP type resolution, and runtime tracing produces three distinct edges. This preserves the audit trail (how was this discovered?) while allowing confidence merging (take the maximum).

**Snapshot** (a point-in-time graph state):
```
SnapshotHash = HierarchicalMerkleRoot(edges grouped by package and edge type)
```

A hierarchical Merkle tree (repo root -> package roots -> edge-type roots -> edge leaves) built from all edge hashes in a repository. The root hash changes if and only if the set of edges changes. The hierarchical structure enables `DiffHierarchicalTrees` to compare package roots instead of all edges (281x vs flat linear scan on real graphs; 517x at 100K synthetic edges), producing semantically meaningful output (changed package names, edge types). `SubgraphRoot` provides O(1) cache keys for any package set. Snapshots form a linked chain (each records its parent hash). See `internal/snapshot/hierarchical.go` for the implementation.

The domain-type prefix system (`"node\0"`, `"edge\0"`, `"snapshot\0"`, `"merkle\0"`) eliminates cross-type ambiguity at the input-construction level, assuming standard collision resistance of SHA-256. The hierarchical Merkle root is the canonical snapshot hash; no flat tree is maintained alongside it.

### 5.2 Explicit Assumptions

The formal properties below hold under the following assumptions:

1. **Fixed analyzer version and configuration.** Different analyzer versions or configurations may produce different edge sets from the same source. The snapshot hash is stable only when these are fixed.
2. **Deterministic canonicalization.** Package paths, symbol names, symbol kinds, and repo identity must be canonicalized identically by all indexers. Canonicalization drift produces correct but divergent hashes.
3. **Identical source input set.** The snapshot hash captures exactly the edges produced from a specific source state. Partial indexing produces a partial (and different) hash.
4. **Deterministic or normalized extractors.** Extractors that are nondeterministic (e.g., from parallel execution ordering) must normalize their output before hashing. Uncontrolled nondeterminism breaks Property 1.
5. **SHA-256 collision resistance.** All uniqueness guarantees assume that SHA-256 behaves as a collision-resistant hash function. This is a standard cryptographic assumption.
6. **Append-only snapshot writes.** Roots are advanced only after successful hierarchical tree construction. Partial writes do not produce valid snapshot hashes.

### 5.3 Properties (formally)

**Property 1: Determinism.**
For any source state S, under the assumptions above, the function `Index(S) -> SnapshotHash` is pure. Same S produces same hash on any machine, at any time, by any operator.

By construction, staleness detection has no false positives or negatives (if the hash matches, the edge set is identical; if it differs, something changed), and currency verification is O(1) hash comparison regardless of graph size. These follow directly from the determinism assumption and the definition of cryptographic hashing.

**Property 2: History is structurally available.**
Every previous state is retrievable at O(1) cost by snapshot hash. The chain provides ordered traversal. No explicit versioning system required.

**Property 3: Isolation without locking.**
Any read pinned to a snapshot hash is consistent and cannot be affected by concurrent writes (writes produce new hashes that don't mutate existing data).

**Property 4: Global identity without coordination.**
For any symbol S described by the same canonical identity inputs, independent indexers compute the same `NodeHash(S)` without communication. Agreement comes from deterministic canonicalization, not from a central ID service.

**Property 5: O(packages) diff.**
`DiffHierarchicalTrees(snapshot_a, snapshot_b)` identifies changed packages by comparing P package roots, not E edge leaves. Complexity is O(P), not O(E).

**Property 6: O(1) subgraph cache keys.**
`SubgraphRoot(package_set)` returns a binary Merkle tree root over the sorted package roots in `package_set` in O(|package_set|) time. Cache validity is determined by one hash comparison.

### 5.4 Overhead Analysis

Content-addressing adds a SHA-256 computation per entity. The hierarchical tree adds an intermediate layer of package and edge-type roots. Measured overhead:

| Operation | Content-addressing cost | Total operation cost | Overhead |
|-----------|------------------------|---------------------|----------|
| Index one node | ~800 nanoseconds (SHA-256) | ~2 milliseconds (parse + store) | 0.04% |
| Index one edge | ~800 nanoseconds | ~500 microseconds (store) | 0.16% |
| Build hierarchical tree (~13.1K edges) | 3.47ms (vs 6.03ms flat) | ~8 seconds (full index) | 0.04% |
| Compute snapshot (10K edges) | ~3 milliseconds (sort + Merkle) | ~8 seconds (full index) | 0.04% |

The overhead is negligible. The dominant cost in every case is parsing or I/O, not hashing. Measurements were taken from the `knowing` indexing pipeline.

---

## 6. Structural Consequences of the Model

The following capabilities are structural consequences of the hierarchical content-addressing choice. They still require ordinary implementation, but they do not require separate consistency, invalidation, versioning, or provenance mechanisms layered beside the data model:

### 6.1 Three-Layer Architecture

| Layer | Primitive | Main benefit |
|-------|-----------|-------------|
| Content-addressed entities | NodeHash, EdgeHash, SnapshotHash | Determinism, provenance, immutable history |
| Hierarchical Merkle roots | PackageRoot, EdgeTypeRoot, SubgraphRoot | Scoped diffing, cache keys, invalidation |
| Agent-facing context layer | PackRoot, community roots (planned), GCF | Replay, deduplication, stable citations, compounding feedback |

Each layer depends on the one below it. The agent-facing layer is only trustworthy because the hierarchical roots are authoritative, and the roots are only authoritative because the entity hashes are deterministic.

### 6.2 Time Travel

"What did the graph look like when we deployed on Tuesday?"

```
snapshot = store.GetSnapshot(tuesday_hash)
edges = store.EdgesForSnapshot(snapshot)
```

A point lookup. No "temporal tables" extension. No "as of" query syntax. The snapshot hash is the state.

### 6.3 Blame

"When did the dependency between auth-service and user-service first appear?"

```
Walk snapshot chain backwards.
For each adjacent pair (snap_n, snap_n-1):
  diff = DiffHierarchicalTrees(snap_n-1, snap_n)
  if edge in diff.added:
    return snap_n.commit_hash, snap_n.timestamp
```

Change attribution falls out of the snapshot chain. The hierarchical diff makes each step O(packages). No separate "history" table. Note: the blame walk is O(snapshots * packages) and has not been benchmarked for long-lived repos with thousands of snapshots. This is a known limitation for the scale roadmap.

### 6.4 Integrity Verification

"Verify this graph has not been modified since the snapshot was anchored."

```
Recompute: for each edge in snapshot, verify EdgeHash == SHA-256(source || target || type || prov)
Recompute: HierarchicalMerkleRoot(verified_edge_hashes grouped by package) == snapshot.SnapshotHash
Verify: snapshot.CommitHash matches git log
```

Any tampering with edge identity (inserted edge, deleted relationship, altered provenance) changes a hash, which changes the package root, which changes the snapshot root, which fails verification. Confidence, however, is mutable metadata on an immutable edge identity: it is not included in `EdgeHash` (which uses sourceHash, targetHash, edgeType, provenance). Tampering with confidence does not break referential integrity but may degrade ranking quality. The integrity guarantee covers edge existence and provenance, not confidence scores.

The data is tamper-evident relative to a trusted root hash. The guarantee becomes meaningful when the snapshot root is anchored to something unforgeable (a signed git commit, an external witness log). Without such anchoring, an attacker with write access can tamper and recompute.

### 6.5 Subgraph Caching

"Has the context for 'internal/mcp' changed since my last query?"

```
current_root = SubgraphRoot(["internal/mcp", "internal/store"])
if current_root == cached_root:
    return cached_result  // 42ns lookup
```

Cache validity is checked in 42ns. A cache hit eliminates the full retrieval pipeline (median cold cost: ~160ms). The speedup is 93x (`bench/merkle-diff/FINDINGS-phase2-cache.md`).

### 6.6 Scoped Daemon Invalidation

The file watcher detects a change. It runs `DiffHierarchicalTrees` to identify affected packages (the diff itself takes ~6us). It evicts only those packages' cache entries. Everything else stays warm.

The total overhead added to each re-index cycle by the diff and invalidation is ~6us, invisible against the ~149ms re-index cost.

### 6.7 Context Pack Deduplication

Content-addressed context packs use:

```
PackRoot = SHA-256(sort(selectedNodeHashes))
```

This gives each context selection a stable identity. Validated in a small demonstration: 5 queries with 2 unique tasks produce exactly 2 unique PackRoots (correct deduplication). This validates the mechanism; production characterization requires longer sessions with varied query patterns. Same context selection against the same graph state produces the same PackRoot, enabling:

- Cache lookup: if PackRoot matches, skip retrieval
- Citation: agents reference a PackRoot instead of resending content
- Cross-session replay: same task, same graph state, same context

### 6.8 Community Roots for Agent Parallelization *(Design)*

Graph clustering (Louvain community detection, implemented in `internal/community/louvain.go`) partitions the graph into densely-connected modules. The design calls for each community to carry a Merkle root over the packages it spans:

```
CommunityRoot(auth_community) = MerkleRoot(PackageRoots(auth_community.packages))
```

*(Note: CommunityRoot computation is not yet implemented. The existing `SubgraphRoot` function takes arbitrary package sets, but no code currently passes community-derived package sets to it. This section describes the design, not current behavior.)*

Two agents editing disjoint communities would have disjoint roots, which proves non-overlap at the relationship-graph identity level. This does not eliminate all semantic conflicts (shared config, global state, generated code), but it provides a strong structural basis for safe parallelization and conflict pre-screening.

### 6.9 Deterministic CI

"Does this PR introduce new cross-repo dependencies?"

```
base_snapshot = snapshot at PR base commit
head_snapshot = snapshot at PR head commit  (computed by CI)
diff = DiffHierarchicalTrees(base, head)
new_cross_repo_edges = filter(diff.added, crosses_repo_boundary)
```

Because indexing is deterministic, CI produces the same snapshot hash that any developer would produce locally. There is no "CI indexed it differently" problem.

### 6.10 Efficient Sync

"Sync the graph to another machine."

Content-addressed entities are trivially distributable. Two instances can sync by exchanging Merkle roots and requesting only the subtrees that differ. This is exactly how `git fetch` works. Same principle, same efficiency.

### 6.11 Natural Garbage Collection

"Remove old snapshots but keep the last 30 days."

Walk the snapshot chain. Anything older than the retention window can be removed. Edges referenced only by removed snapshots can be garbage collected. The chain structure makes this a linear walk with clear semantics.

---

## 7. Proof Points

All benchmarks run on the knowing codebase itself. The repository includes harnesses that regenerate these results from the live graph. Edge counts vary by run and snapshot; exact counts appear in tables, and prose uses approximate figures.

### 7.1 Benchmark Summary

| Benchmark | Result | Source |
|-----------|--------|--------|
| Hierarchical diff vs flat linear scan (~13.1K edges, 57 packages) | 281x faster | `bench/merkle-diff/FINDINGS.md` |
| Hierarchical diff vs flat linear scan at 100K synthetic edges | 517x faster | `bench/merkle-diff/FINDINGS.md` |
| SubgraphRoot lookup (1 package) | 59ns | `bench/merkle-diff/FINDINGS.md` |
| Raw subgraph cache hit | 42ns | `bench/merkle-diff/FINDINGS-phase2-cache.md` |
| Full warm retrieval (cache hit path) | 1.7ms | `bench/merkle-diff/FINDINGS-phase2-cache.md` |
| Cache speedup vs cold | 93x | `bench/merkle-diff/FINDINGS-phase2-cache.md` |
| Daemon diff + invalidation overhead | ~6us | `bench/merkle-diff/FINDINGS-phase2-cache.md` |

The 281x and 517x figures compare hierarchical diff against flat linear scan of all edges. A flat Merkle tree with descend-to-localize would also be fast for small changes, but would not produce package-scoped, semantically meaningful output.

### 7.2 Integrity and Maintenance

| Benchmark | Result | Source |
|-----------|--------|--------|
| `knowing fsck` (2,611 nodes, 13,103 edges) | 98ms median | `bench/merkle-diff/FINDINGS-fsck.md` |
| GarbageCollectFull (500 orphans injected) | 70ms | `bench/merkle-diff/FINDINGS-gc.md` |
| GarbageCollectFull (clean DB, steady state) | 53ms | `bench/merkle-diff/FINDINGS-gc.md` |

### 7.3 Context Retrieval

| Benchmark | Result |
|-----------|--------|
| Context retrieval vs baseline | 55.6% fewer tool calls, 26.9% P@10 |
| Cross-repo retrieval | 46.7% R@10 on foreign codebase |
| GCF wire format | median 76.7% fewer tokens than JSON |
| Test scope | 76.5% mean precision (91.7% median), 51.4% recall |

Methodology: measured against a 55-fixture eval harness (20 easy, 20 medium, 15 hard tasks) on the knowing codebase. Baseline is no-feedback, no-session-boost retrieval. P@10 measures what fraction of top-10 results match hand-curated ground truth. R@10 measures what fraction of ground-truth items appear in the top 10. See `eval/FINDINGS.md` for full methodology and fixture definitions.

### 7.4 Build and Indexing

| Operation | Hierarchical tree | Flat tree | Notes |
|-----------|------------------|-----------|-------|
| Build time (~13.1K edges) | 3.47ms | 6.03ms | Hierarchical faster due to smaller per-group sorts |
| Full index (70K+ LOC repo) | ~8 seconds | ~8 seconds | Build time negligible vs parse + I/O |

Build time for the hierarchical tree is comparable to flat construction (3.47ms vs 6.03ms for the knowing repo, actually faster due to smaller per-group sorts). The operational complexity of maintaining intermediate roots is offset by the query-time benefits at scale.

---

## 8. Agent Implications

The hierarchical content-addressed model provides agents four properties that mutable graphs usually require additional machinery to approximate:

**1. Trustworthy staleness signals.** An agent can verify that the graph it is reading reflects the current commit. With mutable graphs, the agent must trust a timestamp or "last indexed" field that may be wrong.

**2. Consistent multi-query sessions.** An agent that makes five queries during a task reads against the same snapshot. It cannot observe inconsistent state between queries (edges appearing or disappearing mid-task). With mutable graphs, the agent may read a graph that changes between its first and fifth query.

**3. Provenance for confidence.** Every edge carries provenance (how it was discovered) and confidence (how certain we are). The agent can weight its decisions accordingly: "this call path is confirmed by production traces at confidence 0.9" vs "this call path is inferred from AST pattern matching at confidence 0.7."

**4. Context pack identity.** Every context selection carries a `PackRoot` hash. The agent can cite a PackRoot instead of resending content. The same symbol selection produces the same PackRoot, enabling cross-session replay and deduplication. Cache hits against the PackRoot cost 42ns; the full retrieval they replace costs ~160ms.

Content-addressing solves the trust problem: whether relationship data is current, attributable, immutable, and independently verifiable. GCF (Graph Context Format, a compact wire encoding for transmitting selected graph context to language models with less structural overhead than JSON) solves the consumption problem: how that trusted graph is transmitted to an LLM without wasting most of the context window on JSON structure. Together, the hierarchical Merkle model and the compact wire format make graph-backed agent reasoning both correct and cheap.

The content-addressing contract can be stated precisely. Any system claiming to provide software relationship intelligence should be evaluable against six questions. Systems that use hierarchical content-addressing answer all six with structural guarantees. Systems that do not must answer them with application logic, which is where bugs live.

| Question | Content-addressed answer | Mutable-graph answer |
|----------|------------------------|---------------------|
| Is the graph current? | Compare one hash (O(1), provable) | Check timestamps (heuristic, lossy) |
| What changed? | Diff two hierarchical Merkle roots, O(packages) | Query audit log (separate from data) |
| Is this state tamper-evident? | Verify hash chain (relative to trusted root) | Trust the system (operational) |
| Can concurrent access corrupt? | No (immutable data) | Depends on locking strategy |
| Do two instances agree? | Same content -> same hash (guaranteed) | Depends on sync protocol |
| Can I query the past? | Point lookup by hash (O(1)) | Depends on retention policy |
| Are cached results valid? | Compare package roots, O(1) | Depends on cache invalidation logic |

---

## 9. Compounding Intelligence

The most powerful consequence of hierarchical content-addressing is not what it provides to a single agent session, but what it makes possible across sessions: a shared learning substrate where agent intelligence compounds over time.

### 9.1 The Feedback Anchoring Problem

Consider an agent that reports: "the symbol `RankSymbols` was useful for this context-engine task." For this feedback to benefit future agents, it must be anchored to something stable. In mutable systems, anchoring feedback to a symbol name is fragile: renames, moves, and restructuring silently invalidate accumulated knowledge without detection.

With content-addressing, feedback is keyed on the symbol's hash: `SHA-256("node\0" || repoURL || packagePath || "RankSymbols" || "function")`. This provides three guarantees that mutable systems usually require separate audit infrastructure to approximate:

**Natural expiration.** When a symbol is renamed, it receives a new hash. Old feedback becomes structurally orphaned (no current node matches the hash). No garbage collection logic, no TTL heuristics, no manual curation. Staleness is a structural consequence of the identity model.

**Validity verification.** Feedback recorded at snapshot S is valid as long as the symbol still exists in the current graph with the same hash. One lookup confirms or invalidates. No "is this feedback still relevant?" heuristic.

**Temporal provenance.** "When was this feedback recorded, and was the symbol in the same architectural context?" Walk the snapshot chain to the recording point, verify the symbol's community membership. The chain makes this a lookup, not a guess.

### 9.2 Community-Scoped Learning *(Planned)*

Graph clustering (Louvain community detection, implemented in `internal/community/louvain.go`) partitions the graph into densely-connected modules: groups of symbols that interact heavily with each other. These communities correspond to architectural subsystems.

*(Note: feedback is currently scoped by package SubgraphRoot, not by community. Community-scoped feedback and CommunityRoot computation are planned. The following describes the design goal.)*

Feedback scoped by community would compound faster than global feedback because it respects architectural boundaries. "RankSymbols is useful for context-engine tasks" is more precise than "RankSymbols is useful." The community root would provide the scope that makes the signal actionable.

Content-addressing makes this possible because community structure is itself deterministic and verifiable. Communities are computed from edge structure at a snapshot. If the snapshot hash has not changed, communities have not changed.

### 9.3 The Through-Line

The three layers of the architecture depend on each other in a specific order:

1. **Hierarchical content-addressing** makes the graph trustworthy and cheap to diff (can I rely on this data, and can I find what changed efficiently?)
2. **Trustworthy, efficiently diffable data** enables persistent feedback and cache-backed retrieval (can I accumulate intelligence on top of it?)
3. **Persistent, structurally-scoped feedback with cache identity** enables compounding learning (does it get better over time? Currently scoped by package SubgraphRoot; community-scoped: planned.)

Without layer 1, layer 3 is impossible. You cannot accumulate intelligence on top of data you cannot trust. Every mutable-graph approach that attempts to add "learning" must build an entire verification system to determine whether accumulated signals are still valid. The hierarchical content-addressed structure provides that verification as a structural property: if the hash exists in the current graph, the feedback applies. If it does not, it does not.

---

## 10. Limitations and Threats to Validity

The properties described in this paper hold under specific conditions. This section states those conditions honestly.

**Canonicalization is hard and must be treated as core infrastructure.** The global identity guarantee is only as strong as the canonicalization it rests on. Symbol names, package paths, symbol kinds, and repo identity must be defined precisely and consistently. In practice, canonicalization requires careful decisions about case sensitivity, path normalization, symbol kind granularity, and how vendored or generated code is treated. Canonicalization errors produce correct but divergent hashes, meaning two indexers will silently disagree. This is not a property of the model; it is a precondition for the model.

**Analyzer nondeterminism can break determinism unless controlled.** Extractors that produce output in nondeterministic order (due to parallel execution, map iteration, or non-stable sort) must normalize before hashing. Uncontrolled nondeterminism violates Assumption 4 and makes Property 1 fail silently. This requires explicit design work per extractor.

**The package hierarchy only helps when query patterns align with package boundaries.** The O(packages) diff and scoped invalidation are benefits only when agents and queries are scoped to packages. If query patterns cross many packages uniformly, the hierarchical advantage shrinks. The cache hit rate for subgraph queries in realistic agent sessions is approximately 20%, rising to 60% for exact repeated queries. Both numbers are useful, but neither is universal.

**Very small graphs may not benefit enough to justify the complexity.** For repositories with fewer than a few hundred edges and a handful of packages, a flat content-addressed store is simpler and provides nearly the same properties. The hierarchical structure earns its complexity at scale.

**Generated code, vendored dependencies, and monorepos need policy decisions.** The system must decide how to handle code that is not written by project authors. Including vendored dependencies inflates the graph and can produce spurious cross-repo identity conflicts. Excluding them requires explicit filtering. Monorepo layouts may not align cleanly with package boundaries, requiring canonicalization policy specific to the repository structure.

**The benchmark comes from one live codebase plus synthetic tests; broader validation is future work.** The ~13.1K-edge live graph represents a single Go codebase of approximately 70K lines. Synthetic tests extend coverage to 100K edges with controlled parameters. Performance characteristics on other language ecosystems, significantly different package structures, or much larger codebases have not been measured. The speedup ratios should be treated as directionally correct for graphs where packages are natural query boundaries.

**The subgraph cache hit rate depends on agent query patterns.** Realistic sessions observed in development show approximately 20% subgraph cache hit rate; for exact repeated queries the rate reaches 60%. These numbers depend heavily on how agents are prompted and what tasks they perform. Different agent architectures or query strategies will produce different hit rates.

---

## 11. Related Systems

Content-addressing of code is not new. Unison content-addresses code definitions by AST hash, providing rename-immune identity for individual terms. Git content-addresses file trees. The novelty here is narrower: hierarchical content-addressed identity over relationship edges, grouped by package and edge type, used as a query-optimization substrate for diffing, invalidation, and cache validity.

### 11.1 Related System Categories

Prior systems use content-addressing for files, artifacts, or build outputs; mutable or query-oriented stores for code intelligence; or graph indexes for search. The distinction here is using hierarchical content-addressed identity over relationship edges as the primary mechanism for diffing, invalidation, cache validity, and provenance.

**Code intelligence indexes** (Sourcegraph/SCIP, LSIF, Kythe, CodeQL) build queryable databases of code relationships. These use mutable or append-only stores optimized for IDE-style queries: go-to-definition, find-references, hover documentation. They do not content-address the relationship graph itself, so they lack structural staleness detection, snapshot diffing, and cache-key derivation from identity.

**Build graph systems** (Bazel, Buck2, Pants) content-address build artifacts and action outputs, enabling hermetic builds and remote caching. Their Merkle trees organize build actions, not code relationships. The insight that a Merkle tree over semantic code boundaries enables scoped invalidation of analysis results is orthogonal to build-graph content-addressing.

**Graph databases** (Neo4j, Dgraph, Amazon Neptune) store and query graph-structured data with mutable state. They provide query languages (Cypher, GraphQL) but not content-addressed identity, snapshot chains, or Merkle-based diffing. Adding these properties requires application-layer machinery.

**Content-addressed storage** (Git, IPFS, Nix store) content-addresses files, blocks, or derivations. Git's object model is the closest precedent: it uses hierarchical Merkle trees (commit -> tree -> blob) organized by filesystem structure. The insight in this work is that the same approach applies to derived analysis artifacts, with the tree organized by semantic boundaries (packages, edge types) rather than directory hierarchy.

**LSP servers** maintain in-memory indexes of the currently open workspace. They are per-session, mutable, and do not persist or version the relationship state they compute.

Most code intelligence tools evolved from IDE plugins (mutable in-memory state), search engines (inverted indices), or database applications (CRUD against mutable tables). These origins embed the assumption that state is mutable and current. Content-addressing requires a different mental model: state is immutable and historical, with "current" being a pointer to the latest immutable snapshot.

### 11.2 The Git Barrier

Git's success made content-addressing synonymous with "version control for files." The insight that the same primitive applies to derived artifacts (relationships, analyses, metrics) is non-obvious because Git is so strongly associated with file management. The further insight that the Merkle tree's structure should reflect the semantic structure of the content, rather than just the hash values, requires seeing the tree as a computation architecture rather than an integrity mechanism.

### 11.3 The Performance Concern

"Won't hashing everything be slow?" This concern is intuitive but wrong. SHA-256 of a 200-byte node descriptor takes ~800 nanoseconds. A tree-sitter parse of a 500-line file takes ~2 milliseconds. The hash computation is three orders of magnitude cheaper than the operation that produces the data to be hashed.

The empirical evidence is clear: git content-addresses every file, directory, and commit in the largest codebases on earth. Nobody has ever rejected git because "hashing everything is too slow." The bottleneck is always I/O and parsing, never hashing. The same applies here: knowing's hashing overhead (less than 0.1% of indexing time) is invisible next to tree-sitter parsing and SQLite writes.

### 11.4 The Hierarchy Insight Requires a Different Framing

The reason prior systems do not use hierarchical Merkle trees over code relationships is not technical difficulty. It is conceptual: if you think of content-addressing as "hashing things for integrity," you see no reason to organize the Merkle tree by semantic boundaries. The flat tree gives you integrity. Why add structure?

The answer is visible only when you ask a different question: "can the identity structure do work?" A flat Merkle tree proves state. A hierarchical Merkle tree organizes computation. The identity and the algorithm become the same thing. That reframing is the contribution.

### 11.5 Storage

In the pessimistic case, storing 10K unique edge records per snapshot is small by modern storage standards. In the normal content-addressed case, unchanged edges are shared across snapshots, so the effective cost of daily snapshots is proportional to the rate of change, not the total graph size.

---

## 12. Conclusion

Every mutable-graph approach to software relationship intelligence is fighting a fundamental architectural mismatch. Relationships change over time. Consumers need history. Correctness requires integrity verification. Scale requires concurrent access. Distribution requires identity agreement. Auditors require provable derivation. Agents need cache-backed retrieval.

The original content-addressing insight (hash everything, use the hash as identity) solves the first six requirements. The hierarchical Merkle revelation solves the last: by organizing the tree to match the semantic structure of the codebase, the identity structure becomes the query optimization layer. Diffs are O(packages). Cache keys are O(1). Invalidation is scoped. The tree proves state and organizes computation simultaneously.

Build time for the hierarchical tree is comparable to flat construction (3.47ms vs 6.03ms for the knowing repo, actually faster due to smaller per-group sorts). The operational complexity of maintaining intermediate roots is offset by the query-time benefits at scale. The speedups are significant: up to 93x for cached queries (warm path), 281x for diffs vs flat linear scan on the live graph, 517x at 100K synthetic edges.

These properties hold under the assumptions stated in Section 5.2. The limitations in Section 10 are real and must be addressed as core infrastructure, not afterthoughts. Canonicalization is not a detail; it is a precondition. Deterministic extractors are not optional; they are required for Property 1 to hold.

While content-addressing of code elements exists (Unison for definitions, Git for files), we are not aware of an existing system that applies hierarchical content-addressed identity over relationship edges as a query-optimization substrate. The reason is likely conceptual: you only see the algorithmic opportunity when you stop thinking about content-addressing as an integrity mechanism and start thinking about it as a computation architecture.

Git proved this for source code. The same insight applies, with equal force and deeper consequences, to everything derived from source code.

---

## Appendix: Hash Computations

```
RepoHash     = SHA-256(repoURL)
NodeHash     = SHA-256("node\0" || repoURL || packagePath || symbolName || symbolKind)
EdgeHash     = SHA-256("edge\0" || sourceHash || targetHash || edgeType || provenance)
FileHash     = SHA-256(repoHash || relativePath || contentHash)
PackageRoot  = MerkleRoot(sort(EdgeTypeRoots for all edge types in package))
EdgeTypeRoot = MerkleRoot(sort(EdgeHashes for all edges of that type in package))
SnapshotHash = MerkleRoot(sort(PackageRoots for all packages in repo))
PackRoot     = SHA-256(sort(selectedNodeHashes))
SubgraphRoot = BuildMerkleTree(sort(PackageRoots for packages in query scope))
```

Each computation is deterministic, cheap (~800ns per entity), and produces globally unique identities without coordination, under the assumption of SHA-256 collision resistance and deterministic canonicalization. Canonical repo identity is the repository URL, hashed directly.

The domain-type prefix system (`"node\0"`, `"edge\0"`, `"snapshot\0"`, `"merkle\0"`) eliminates cross-type ambiguity at the input-construction level. This mirrors git's `"<type> <size>\0<content>"` object header. The hierarchical Merkle root is the canonical snapshot hash; no flat tree is maintained alongside it.

Implementation: `internal/snapshot/hierarchical.go`.
