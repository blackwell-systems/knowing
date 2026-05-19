# Content-Addressing as a Primitive for Software Relationship Intelligence

**Dayna Blackwell, Blackwell Systems**

---

## Abstract

We argue that content-addressing is the correct foundational primitive for tracking software system relationships over time. Git proved this for source code: by making every state a hash, you get determinism, integrity, history, and cheap comparison for free. We apply the same insight one abstraction layer up: not "what does the code say" but "how does the code relate to everything else."

The result is a graph where every node (symbol), edge (relationship), and snapshot (point-in-time state) is identified by a cryptographic hash of its content. This single design choice eliminates six classes of problems that plague every mutable-graph approach to code intelligence: re-indexing ambiguity, staleness detection, change attribution, snapshot isolation, cross-repo identity, and audit provenance.

We formalize the model, demonstrate its properties, and show that the overhead of content-addressing is negligible (< 0.1% of indexing time) while the structural guarantees it provides are difficult, expensive, and failure-prone to retrofit onto mutable systems.

---

## 1. The Insight

Git is a content-addressed graph of source code. This single design decision gives Git every property that made it the dominant version control system:

- Same content always produces the same hash (determinism)
- Any modification changes the hash (integrity)
- Every previous state is retrievable by its hash (history)
- Comparing two states is comparing two hashes (O(1) staleness)
- Concurrent operations on immutable snapshots cannot conflict (isolation)
- The chain from commit to tree to blob is cryptographically verifiable (audit)

These properties are not features bolted onto a mutable store. They are *structural consequences* of the content-addressing choice. You cannot have a mutable system that provides them without simulating immutability (MVCC, event sourcing, temporal tables), and every simulation is more complex and less trustworthy than the real thing.

**Our thesis:** software relationship intelligence requires the same properties Git provides for source code, and content-addressing is the simplest primitive we know that provides these guarantees structurally rather than through application-level simulation.

---

## 2. What We Mean by Software Relationships

A software system is not just source code. It is a graph of relationships:

- Function A calls function B (static analysis)
- Service X sends requests to service Y at endpoint /users (runtime observation)
- Deployment D references ConfigMap C (infrastructure declaration)
- Type T implements interface I (type system)
- Handler H is registered to route R (framework convention)

These relationships exist across repositories, languages, services, and infrastructure layers. They change over time. They have varying degrees of certainty (a tree-sitter pattern match is less confident than a type-system confirmation; a runtime trace is more confident than either for the specific question "does this actually execute in production?").

Most existing systems do not track these relationships with all of the following properties simultaneously:
- Version history (what did the graph look like Tuesday?)
- Provenance (how was this relationship discovered?)
- Confidence (how certain are we?)
- Cross-boundary awareness (across repos, services, languages)
- Provable currency (is this graph still correct?)

The reason is that most systems use mutable state. And mutable state cannot provide these properties without extraordinary complexity.

---

## 3. The Six Problems with Mutable Graphs

Every tool that builds a "code graph" or "dependency graph" using a mutable store (Neo4j, PostgreSQL, in-memory structures, SQLite with UPDATE queries) faces six problems that are structural consequences of mutability:

### 3.1 Re-indexing Ambiguity

You re-index a repository. What happens to existing edges?

- **Upsert?** Then you never know if an edge is current or stale (it was written at some point and never explicitly removed).
- **Delete and recreate?** Then concurrent reads see incomplete state mid-rebuild.
- **Merge?** Then you need conflict resolution logic for every edge type.

Every choice requires application-level logic to maintain consistency. Every choice has failure modes. Every production system using mutable graphs has bugs in this logic.

**With content-addressing:** re-indexing the same code produces the same hashes. `INSERT OR IGNORE` is the only write pattern. If the hash already exists, the entity already exists with identical content. No merge logic. No conflict resolution. No ambiguity.

### 3.2 Staleness Detection

How do you know if the graph reflects the current state of the code?

In mutable systems, the answer is metadata: `last_indexed_at` timestamps, version counters, TTL-based expiry. All of these are heuristics. None of them answer the actual question: "is the graph structurally identical to what a fresh index would produce?"

**With content-addressing:** compute the Merkle root of the current edge set. Compare to the stored snapshot hash. If they match, the graph is provably current. If they differ, the divergence is locatable in the Merkle tree. One comparison, one answer, no heuristics.

### 3.3 Change Attribution

"When did this relationship between service A and service B first appear?"

Mutable graphs answer this only if you added an explicit `created_at` field, remembered to set it correctly on every write path, and never accidentally overwrote it during re-indexing (see Problem 1).

**With content-addressing:** the snapshot chain is the history. Walk backwards from the current snapshot, diffing adjacent pairs, until you find the snapshot where the edge first appears. The diff is comparing two immutable states. The attribution is the commit hash stored on that snapshot. No special fields required; history is the data model.

### 3.4 Snapshot Isolation

A long-running blast-radius computation starts. Halfway through, the indexer runs and updates the graph. The computation now reads a mix of old and new state.

Mutable systems solve this with read transactions, MVCC, or full locks. All add complexity. All have edge cases (long-running transactions holding WAL files open, MVCC bloat, lock contention).

**With content-addressing:** the computation pins a snapshot hash and reads against it. The graph can be updated concurrently by writing new entities (which have different hashes). The computation's reads are against immutable data that cannot change. No locks, no transactions, no MVCC overhead.

### 3.5 Cross-Repo Identity

Repository A has a function. Repository B calls it. Both repositories are indexed independently. How do you ensure the "function" in A and the "callee reference" in B point to the same entity?

Mutable systems need either a global ID service (coordination overhead, single point of failure) or a naming convention (fragile, breaks on rename/move).

**With content-addressing:** both compute `RepoHash = SHA-256(canonicalRepoIdentity)` and then `NodeHash = SHA-256(repoHash || packagePath || symbolName || symbolKind)`. Same canonical inputs, same hash. Global identity without coordination, without consensus, without a central registry. Two indexers running on different machines at different times produce the same hash for the same symbol.

In practice, `repoURL` must be canonicalized before hashing: normalized host, owner, repository name, and transport-independent identity. Systems may also choose to hash a repository root identity derived from the VCS remote and commit lineage rather than the literal URL string.

### 3.6 Audit Provenance

"Prove that this graph state was derived from these specific source commits."

Mutable systems cannot prove this. They can log it, but logs are separate from data, can be tampered with, and require trust in the logging system.

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

Every step is a deterministic computation from content. An auditor can verify any claim by recomputing. The data *is* the audit trail.

---

## 4. Formal Model

### 4.1 Entity Definitions

**Node** (a symbol declaration):
```
RepoHash = SHA-256(canonicalRepoIdentity)
NodeHash = SHA-256(repoHash || packagePath || symbolName || symbolKind)
```

Identity depends on logical position (repo, package, name, kind), not physical location (file, line number). Moving a function between files does not change its hash. Renaming it does (creating a new entity; the old entity's edges become stale, detectable via snapshot diff).

**Edge** (a directed relationship):
```
EdgeHash = SHA-256(sourceHash || targetHash || edgeType || provenance)
```

Identity includes provenance. The same structural relationship (A calls B) observed by tree-sitter AST analysis, LSP type resolution, and runtime tracing produces three distinct edges. This preserves the audit trail (how was this discovered?) while allowing confidence merging (take the maximum).

**Snapshot** (a point-in-time graph state):
```
SnapshotHash = HierarchicalMerkleRoot(edges grouped by package and edge type)
```

A hierarchical Merkle tree (repo root -> package roots -> edge-type roots -> edge leaves) built from all edge hashes in a repository. The root hash changes if and only if the set of edges changes. A flat tree (`MerkleRoot(sort(edgeHashes))`) is also computed alongside for backward compatibility; both roots are identical. The hierarchical structure enables `DiffHierarchicalTrees` to compare package roots instead of all edges (114x faster on 11K edges, 517x on 100K synthetic edges), and `SubgraphRoot` to provide O(1) cache keys for any package set. Snapshots form a linked chain (each records its parent hash). See `internal/snapshot/hierarchical.go` for the implementation.

### 4.2 Properties (formally)

**Property 1: Determinism.**
For any source state S, the function `Index(S) -> SnapshotHash` is pure. Same S produces same hash on any machine, at any time, by any operator.

**Property 2: Completeness of staleness detection.**
For a fixed analyzer version, configuration, and input source state, `SnapshotHash(current_edges) == stored_snapshot_hash` if and only if the graph exactly reflects the indexed relationship set for that source state. No false positives (declaring current when stale). No false negatives (declaring stale when current).

**Property 3: O(1) currency check.**
Verifying currency requires exactly one hash comparison, regardless of graph size.

**Property 4: History is free.**
Every previous state is retrievable at O(1) cost by snapshot hash. The chain provides ordered traversal. No explicit versioning system required.

**Property 5: Isolation without locking.**
Any read pinned to a snapshot hash is consistent and cannot be affected by concurrent writes (writes produce new hashes that don't mutate existing data).

**Property 6: Global identity without coordination.**
For any symbol S described by the same canonical identity inputs, independent indexers compute the same `NodeHash(S)` without communication. Agreement comes from deterministic canonicalization, not from a central ID service.

### 4.3 Overhead Analysis

Content-addressing adds a SHA-256 computation per entity. Measured overhead:

| Operation | Content-addressing cost | Total operation cost | Overhead |
|-----------|------------------------|---------------------|----------|
| Index one node | ~800 nanoseconds (SHA-256) | ~2 milliseconds (parse + store) | 0.04% |
| Index one edge | ~800 nanoseconds | ~500 microseconds (store) | 0.16% |
| Compute snapshot (10K edges) | ~3 milliseconds (sort + Merkle) | ~8 seconds (full index) | 0.04% |

The overhead is negligible. The dominant cost in every case is parsing or I/O, not hashing. Measurements were taken from the `knowing` indexing pipeline using SHA-256 over canonical node and edge descriptors.

---

## 5. Implementation Status

The content-addressed relationship model is implemented in `github.com/blackwell-systems/knowing` as the persistence and identity layer for software relationship intelligence.

The implementation includes:

- Deterministic node hashes for symbols across 10 language extractors
- Deterministic edge hashes for relationships with provenance-aware identity
- Hierarchical Merkle trees (repo root -> package roots -> edge-type roots -> edge leaves) enabling package-scoped diff (114x faster than flat diff on 11K edges, 517x on 100K synthetic); flat trees built alongside for backward compatibility
- Snapshot hashes computed as hierarchical Merkle roots
- Parent-linked snapshot chains with diff between adjacent states
- Cross-repo symbol identity via canonical repo hashing
- GCF/GCB wire formats for efficient graph transmission to LLM consumers
- Tests validating deterministic identity, snapshot reproducibility, and round-trip integrity

The system is deployed as a CLI tool and MCP server (22 tools), processing repositories of 40K+ lines of code with sub-second incremental reindexing. These measurements come from indexing the `knowing` repository itself and related test fixtures.

---

## 6. What You Get for Free

The following capabilities are structural consequences of the content-addressing choice. They require no additional implementation beyond the hash computations:

### 6.1 Time Travel

"What did the graph look like when we deployed on Tuesday?"

```
snapshot = store.GetSnapshot(tuesday_hash)
edges = store.EdgesForSnapshot(snapshot)
```

A point lookup. No "temporal tables" extension. No "as of" query syntax. The snapshot hash *is* the state.

### 6.2 Blame

"When did the dependency between auth-service and user-service first appear?"

```
Walk snapshot chain backwards.
For each adjacent pair (snap_n, snap_n-1):
  diff = SnapshotDiff(snap_n-1, snap_n)
  if edge in diff.added:
    return snap_n.commit_hash, snap_n.timestamp
```

Change attribution falls out of the snapshot chain. No separate "history" table.

### 6.3 Integrity Verification

"Prove this graph was not tampered with."

```
Recompute: for each edge in snapshot, verify EdgeHash == SHA-256(source || target || type || prov)
Recompute: MerkleRoot(sort(verified_edge_hashes)) == snapshot.SnapshotHash
Verify: snapshot.CommitHash matches git log
```

Any tampering (inserted edge, modified confidence, deleted relationship) changes a hash, which changes the Merkle root, which fails verification. The data is self-authenticating.

### 6.4 Deterministic CI

"Does this PR introduce new cross-repo dependencies?"

```
base_snapshot = snapshot at PR base commit
head_snapshot = snapshot at PR head commit  (computed by CI)
diff = SnapshotDiff(base, head)
new_cross_repo_edges = filter(diff.added, crosses_repo_boundary)
```

Because indexing is deterministic, CI produces the same snapshot hash that any developer would produce locally. There is no "CI indexed it differently" problem.

### 6.5 Efficient Sync

"Sync the graph to another machine."

Content-addressed entities are trivially distributable. Two instances can sync by exchanging Merkle roots and requesting only the subtrees that differ. This is exactly how `git fetch` works. Same principle, same efficiency.

### 6.6 Natural Garbage Collection

"Remove old snapshots but keep the last 30 days."

Walk the snapshot chain. Anything older than the retention window can be removed. Edges referenced only by removed snapshots can be garbage collected. The chain structure makes this a linear walk with clear semantics.

---

## 7. Why Existing Systems Don't Do This

The obvious question: if content-addressing is so clearly superior for this use case, why doesn't every code intelligence tool use it?

### 7.1 Historical Context

Most code intelligence tools evolved from:
- IDE plugins (mutable in-memory state, rebuilt on every session)
- Search engines (inverted indices, updated in place)
- Database applications (CRUD against mutable tables)

These origins embed the assumption that state is mutable and current. Content-addressing requires a different mental model: state is immutable and historical, with "current" being a pointer to the latest immutable snapshot.

### 7.2 The Git Barrier

Git's success made content-addressing synonymous with "version control for files." The insight that the same primitive applies to derived artifacts (relationships, analyses, metrics) is non-obvious because Git is so strongly associated with file management.

### 7.3 The Performance Concern

"Won't hashing everything be slow?" This concern is intuitive but wrong. SHA-256 of a 200-byte node descriptor takes ~800 nanoseconds. A tree-sitter parse of a 500-line file takes ~2 milliseconds. The hash computation is three orders of magnitude cheaper than the operation that produces the data to be hashed.

The empirical evidence is definitive: git content-addresses every file, directory, and commit in the largest codebases on earth. The Linux kernel (36 million lines, 1 million commits), Android (hundreds of millions of lines), Chromium, and Windows all use git. Nobody has ever rejected git because "hashing everything is too slow." The overhead of content-addressing has never been a practical barrier at any scale. The bottleneck is always I/O and parsing, never hashing. The same applies here: knowing's hashing overhead (< 0.1% of indexing time) is invisible next to tree-sitter parsing and SQLite writes.

### 7.4 The Storage Concern

"Won't storing every version of every edge be expensive?" Not meaningfully. A content-addressed edge is ~200 bytes. A repository with 10,000 edges produces ~2MB of edge data per snapshot. Retaining 365 daily snapshots costs ~730MB in the pessimistic case: smaller than a typical node_modules directory. In practice, a content-addressed store shares unchanged edges across snapshots, so daily snapshots store only newly introduced edge objects plus snapshot metadata.

---

## 8. The Content-Addressing Contract

We propose that any system claiming to provide software relationship intelligence should be evaluated against six questions. Systems that use content-addressing answer all six with structural guarantees. Systems that don't must answer them with application logic, which is where bugs live.

| Question | Content-addressed answer | Mutable-graph answer |
|----------|------------------------|---------------------|
| Is the graph current? | Compare one hash (O(1), provable) | Check timestamps (heuristic, lossy) |
| What changed? | Diff two Merkle roots (structural) | Query audit log (separate from data) |
| Is this state genuine? | Verify hash chain (cryptographic) | Trust the system (operational) |
| Can concurrent access corrupt? | No (immutable data) | Depends on locking strategy |
| Do two instances agree? | Same content -> same hash (guaranteed) | Depends on sync protocol |
| Can I query the past? | Point lookup by hash (free) | Depends on retention policy |

---

## 9. Implications for AI Agents

AI agents are the most demanding consumer of software relationship intelligence. They operate under token-budgeted context windows, make multiple queries per task, and need confidence signals to prioritize information.

Content-addressing provides agents three properties that mutable graphs cannot:

**1. Trustworthy staleness signals.** An agent can verify that the graph it's reading reflects the current commit. With mutable graphs, the agent must trust a timestamp or "last indexed" field that may be wrong.

**2. Consistent multi-query sessions.** An agent that makes five queries during a task reads against the same snapshot. It cannot observe inconsistent state between queries (edges appearing or disappearing mid-task). With mutable graphs, the agent may read a graph that changes between its first and fifth query.

**3. Provenance for confidence.** Every edge carries provenance (how it was discovered) and confidence (how certain we are). The agent can weight its decisions accordingly: "this call path is confirmed by production traces at confidence 0.9" vs "this call path is inferred from AST pattern matching at confidence 0.7." With mutable graphs, confidence requires an additional metadata system separate from the graph itself.

Content-addressing solves the trust problem: whether relationship data is current, attributable, immutable, and independently verifiable. GCF solves the consumption problem: how that trusted graph is transmitted to an LLM without wasting most of the context window on JSON structure.

---

## 10. Compounding Intelligence: Why CAS Enables Learning

The most powerful consequence of content-addressing is not what it provides to a single agent session, but what it makes possible across sessions: a shared learning substrate where agent intelligence compounds over time.

### 10.1 The Feedback Anchoring Problem

Consider an agent that reports: "the symbol `RankSymbols` was useful for this context-engine task." For this feedback to benefit future agents, it must be anchored to something stable. In mutable systems, anchoring feedback to a symbol name is fragile: renames, moves, and restructuring silently invalidate accumulated knowledge without detection.

With content-addressing, feedback is keyed on the symbol's hash: `SHA-256(repoURL || packagePath || "RankSymbols" || "function")`. This provides three guarantees no mutable system can offer:

**Natural expiration.** When a symbol is renamed, it receives a new hash. Old feedback becomes structurally orphaned (no current node matches the hash). No garbage collection logic, no TTL heuristics, no manual curation. Staleness is a structural consequence of the identity model.

**Validity verification.** Feedback recorded at snapshot S is valid as long as the symbol still exists in the current graph with the same hash. One lookup confirms or invalidates. No "is this feedback still relevant?" heuristic.

**Temporal provenance.** "When was this feedback recorded, and was the symbol in the same architectural context?" Walk the snapshot chain to the recording point, verify the symbol's community membership. The chain makes this a lookup, not a guess.

### 10.2 Community-Scoped Learning

Graph clustering (Louvain community detection) partitions the graph into densely-connected modules: groups of symbols that interact heavily with each other. These communities correspond to architectural subsystems.

Feedback scoped by community compounds faster than global feedback because it respects architectural boundaries. "RankSymbols is useful for context-engine tasks" is more precise than "RankSymbols is useful." The community provides the scope that makes the signal actionable.

Content-addressing makes this possible because community structure is itself deterministic and verifiable. Communities are computed from edge structure at a snapshot. If the snapshot hash hasn't changed, communities haven't changed. Feedback validity and community membership can be verified with hash comparisons, not recomputation.

### 10.3 The Through-Line

The three layers of the architecture depend on each other in a specific order:

1. **Content-addressing** makes the graph trustworthy (can I rely on this data?)
2. **Trustworthy data** enables persistent feedback (can I accumulate intelligence on top of it?)
3. **Persistent, community-scoped feedback** enables compounding learning (does it get better over time?)

Without layer 1, layer 3 is impossible. You cannot accumulate intelligence on top of data you cannot trust. Every mutable-graph approach that attempts to add "learning" must build an entire verification system to determine whether accumulated signals are still valid. Content-addressing provides that verification as a structural property: if the hash exists in the current graph, the feedback applies. If it doesn't, it doesn't.

This is the deepest consequence of the content-addressing choice: it makes the graph not just queryable but improvable. Each session deposits knowledge. That knowledge is anchored to hashes that naturally expire when they become irrelevant. The system teaches itself which code matters for which work, and the teaching is as trustworthy as the data model itself.

---

## 11. Beyond Code: The General Principle

Content-addressing is not specific to software relationships. The principle applies to any domain where:

1. You need history (what did it look like before?)
2. You need integrity (has it been tampered with?)
3. You need staleness detection (is this current?)
4. You need concurrent access (can I read while someone writes?)
5. You need distributed identity (do two systems agree on what this entity is?)

If your domain has these requirements and you're using mutable state, you're either accepting bugs in the areas above or building increasingly complex machinery to simulate immutability. Content-addressing provides all five properties from a single primitive: hash the content, use the hash as the identity.

Git demonstrated this for source code. Blockchain demonstrated it (controversially) for financial ledgers. We demonstrate it for software system relationships. The pattern is general.

---

## 12. Conclusion

Every mutable-graph approach to software relationship intelligence is fighting a fundamental architectural mismatch. Relationships change over time. Consumers need history. Correctness requires integrity verification. Scale requires concurrent access. Distribution requires identity agreement. Auditors require provable derivation.

Content-addressing provides all of these as structural consequences of a single design choice: identity is the hash of content.

The overhead of this choice is negligible (< 0.1% of indexing time). The properties it provides are difficult, expensive, and failure-prone to achieve in mutable systems without simulating immutability through complex application logic that itself becomes a source of bugs.

Git proved this for source code. The same insight applies, with equal force, to everything derived from source code. Software relationships are the most important derived artifact, and they deserve the same foundational guarantees.

---

## Appendix: Hash Computations

```
RepoHash     = SHA-256(canonicalRepoIdentity)
NodeHash     = SHA-256(repoHash || packagePath || symbolName || symbolKind)
EdgeHash     = SHA-256(sourceHash || targetHash || edgeType || provenance)
FileHash     = SHA-256(repoHash || relativePath || contentHash)
SnapshotHash = HierarchicalMerkleRoot(edges grouped by package and edge type)
             -- flat: MerkleRoot(sort(all edge hashes in repo)) built alongside for compatibility
```

Each computation is deterministic, cheap (~800ns), and globally unique without coordination. Canonical repo identity is derived from normalized host, owner, and repository name, independent of transport protocol or URL scheme.
