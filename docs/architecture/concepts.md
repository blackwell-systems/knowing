# Core Concepts

This section defines every term used in the rest of this document. Read it before proceeding.

## Content-Addressed Storage

In content-addressed storage, data is identified by its content, not by a name or location. The identifier is a cryptographic hash (SHA-256) of the data itself. Two pieces of identical data always produce the same hash. Different data always produces different hashes.

This has three consequences:

1. **Deduplication is automatic.** If the same function appears in two repos, it gets the same hash. Store it once.
2. **Integrity is verifiable.** Recompute the hash from the data. If it matches the stored hash, the data is uncorrupted. If it doesn't, something changed.
3. **Cache invalidation is structural.** A query result computed against hash X is valid for all time. When the underlying data changes, it gets a new hash Y. Results keyed to X are still correct for X; results for Y must be recomputed.

knowing uses content-addressed storage for nodes, edges, files, snapshots, feedback, and derived computation results. Every piece of data in the system is identified by its hash. This includes feedback records, which store the SubgraphRoot (Merkle root of the symbol's package) at recording time; when code changes, old feedback becomes invisible because its stored root no longer matches the current root.

## Merkle DAG

A Merkle DAG (Directed Acyclic Graph) is a data structure where every node contains the cryptographic hash of its children. The root hash summarizes the entire structure: if any leaf changes, the root hash changes.

**The Git analogy:** Git is a Merkle DAG. A commit hash summarizes the entire repository state at that point. If a single byte changes in any file, the commit hash changes. You can verify the integrity of the entire repository by checking the root hash.

knowing works the same way. A snapshot hash is the root of a hierarchical Merkle tree (repo root -> package roots -> edge-type roots -> edge leaves) built from all edge hashes in the graph at a point in time. If any edge changes, the snapshot hash changes. Two snapshots with the same hash contain exactly the same graph. Two snapshots with different hashes differ in at least one edge.

**How it works in knowing:**

knowing builds a hierarchical Merkle tree with three levels (implemented in `internal/snapshot/hierarchical.go`, delegating to the [`merkle-strata`](https://github.com/blackwell-systems/merkle-strata) library):

```
repo_root
  ├── package_root[pkg/A]   = merkle(sorted edge-type roots for pkg/A)
  │     ├── edge_type_root[pkg/A:calls]
  │     └── edge_type_root[pkg/A:imports]
  └── package_root[pkg/B]
        └── edge_type_root[pkg/B:calls]
```

Structure: repo root -> package roots -> edge-type roots -> edge leaves. The hierarchical root IS the canonical snapshot identity. No separate flat tree is maintained; the flat tree was dropped after the hash domain prefix change made backward compatibility moot. `types.ComputeSnapshotHash` wraps the hierarchical root with a `"snapshot\0"` domain prefix to produce the snapshot hash stored in the database.

`DiffHierarchicalTrees` compares package roots instead of all edges: 281x faster on the knowing repo (13K edges), 517x on 100K synthetic edges. `SubgraphRoot` computes O(1) cache keys for any set of packages. `EdgeTypeRoot` answers "did call edges change?" in one lookup. See `docs/architecture/merkle-algorithms.md` for the full algorithm specification and `bench/merkle-diff/` for benchmark results.

## Knowledge Graph vs. Tree vs. Table

A **table** stores flat records. Good for lookups, bad for relationships. "Find all callers of function X" requires a join for each hop.

A **tree** stores hierarchical data (like a file system). Every node has one parent. But code relationships are not hierarchical: function A calls function B, which implements interface C, which is consumed by service D in another repository. A tree cannot represent this.

A **graph** stores nodes connected by edges with no structural constraint on connectivity. A node can have many inbound and outbound edges of different types. This matches the reality of code: a function is called by many callers, implements an interface, lives in a file owned by a team, and is invoked at runtime by three services.

knowing is a knowledge graph because code relationships are inherently graph-shaped. The graph is content-addressed (every node and edge is identified by its hash) and typed (edges carry a type like `calls`, `implements`, or `references`).

## Domain Primitives

| Primitive | What it is | Hash computation |
|-----------|-----------|-----------------|
| **Node** | A symbol in source code. Kinds: function, method, type, interface, const, var, service, route, external, file, package. Identified by qualified name. Carries a `Doc` field (first 200 chars of the declaration's doc comment) populated by 6 language extractors (Go, Python, TypeScript, Rust, Java, C#). | `sha256("node\0" \|\| repo \|\| package_path \|\| symbol_name \|\| symbol_kind)` |
| **Edge** | A relationship between two nodes. 34 edge types (calls, imports, implements, extends, tests, handles_route, publishes, subscribes, documents, gated_by_flag, contains, member_of, co_tested_with, type_hint_of, etc.). Carries a type, confidence score, and provenance. See [Edge Types](edge-types.md). | `sha256("edge\0" \|\| source_hash \|\| target_hash \|\| edge_type \|\| provenance)` |
| **Hash** | A 32-byte SHA-256 digest used as the content-addressed identifier for every entity. All hash inputs carry a domain-type prefix (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) so hashes from different entity types are structurally distinguishable -- the same approach git uses with its `"blob <size>\0"` header. | n/a |
| **Snapshot** | A point-in-time graph state. The root of a hierarchical Merkle tree (repo root -> package roots -> edge-type roots -> edge leaves). Also stores intermediate package roots and edge-type roots for scoped invalidation. Links to a parent snapshot (forming a chain like git commits), records the git commit that produced it, and carries a generation number (parent.Generation + 1) for O(1) ancestry checks. | `sha256("snapshot\0" \|\| hierarchical_merkle_root(edges grouped by package and edge type))` |
| **Provenance** | Metadata on an edge describing how it was derived, by which indexer version, at what confidence, from which commit. Tiers: `ast_inferred` (0.7, tree-sitter), `ast_resolved` (0.85, import-map resolved), `lsp_resolved` (0.9, LSP confirmed), `scip_resolved` (1.0, SCIP), `runtime_observed` (0.8, OTel trace). Provenance is what lets agents distinguish "confirmed by type checker" from "guessed from string matching." | Included in edge hash input. |

## Event Sourcing

Edges are never mutated in place. Every change to the graph is recorded as an event: an edge was "added" or an edge was "removed," keyed by the snapshot hash that recorded the event. The current graph state is the result of replaying all events (or equivalently, reading the materialized edge table).

This means:
- "When did this edge first appear?" is a query on the event log.
- "What changed between snapshot A and snapshot B?" is a range scan on events filtered by snapshot hash.
- Rolling back to a previous state means pointing to an older snapshot, not undoing mutations.

## Staleness

**Structural staleness:** A file's content hash changed, so all nodes derived from it have stale hashes, and all edges originating from those nodes are suspect. This is detected automatically by hash comparison; no heuristic is needed.

**Heuristic staleness:** An edge has not been re-confirmed by the indexer for N days, or a runtime edge has not been observed in production for N days. This requires time-based reasoning on top of the structural property.

Both forms of staleness are exposed through the `StaleEdges` API and the `knowing stale` CLI command (which uses `StaleNodesByFiles` to report stale nodes from files changed since the last snapshot). Structural staleness is authoritative. Heuristic staleness is advisory.

**SubgraphCache:** The daemon maintains a `SubgraphCache` (in `internal/cache/subgraph.go`) that stores query results keyed by Merkle subgraph roots rather than by snapshot hash. When the hierarchical tree confirms that a package's root has not changed between two index runs, all cached results for queries scoped to that package remain valid. After each index run the cache invalidates only the entries whose package roots changed, using `InvalidatePackages` to compare the old and new hierarchical trees. This makes query caching precise: an unrelated package changing elsewhere in the graph does not evict results for the unchanged package.

## Why Content Addressing Eliminates Re-Indexing

Every other code intelligence tool in the market requires explicit re-indexing. You change a file, and the tool must re-scan the entire codebase to update its model. Some are faster than others, but the fundamental operation is "throw away old state, rebuild from scratch."

knowing never re-indexes unchanged code. The content-addressed architecture makes this structural, not heuristic:

**1. File identity is a content hash.**

When knowing indexes a file, it computes `sha256(file_contents)` and stores it as the file's identity. On the next index run, it recomputes this hash. If the hash matches, the file has not changed. All nodes and edges derived from it are still valid. Skip it entirely.

This is the same mechanism git uses for its blob store: `git hash-object` computes the SHA of a file's contents. If two files have the same hash, they have the same content, regardless of where they live or what they're named.

**2. Changed files scope the work.**

When `.git/HEAD` changes (a new commit), knowing runs `git diff --name-status oldHead newHead` to get the exact set of changed, added, and deleted files. Only these files are re-processed:

- **Changed files:** delete old nodes/edges derived from this file, re-extract, record edge events for what was added/removed
- **Added files:** extract and insert (no cleanup needed)
- **Deleted files:** delete all nodes/edges derived from this file, record removal events

Everything else is untouched. In a typical commit that changes 3 files in a 10,000-file codebase, knowing processes 3 files. A full re-indexer processes 10,000.

**3. The Merkle root detects drift without scanning.**

The snapshot hash is the hierarchical Merkle root. If you have the previous snapshot hash and the current snapshot hash, you know instantly whether the graph changed. You don't need to scan edges to find out. The hierarchical tree goes further: comparing package roots tells you which packages changed without enumerating all edges, and comparing edge-type roots tells you whether call edges changed independently of runtime trace edges.

More importantly: if you have two snapshot hashes and they're identical, you know the graph is in the exact same state. This is a structural guarantee that no other representation can provide. A mutable graph database can't tell you "nothing changed" without scanning everything.

**4. Edge events make diffs O(changes), not O(graph).**

When knowing adds or removes an edge during incremental indexing, it records an event in the append-only edge_events table: `{edge_hash, snapshot_hash, event_type: "added"|"removed"}`. Computing the diff between any two snapshots is a range scan on this table filtered by snapshot hash. It returns exactly the edges that changed.

Without event sourcing, diffing two graph states requires loading both, computing set differences on all edges, and comparing them. That's O(total_edges). With event sourcing, it's O(changed_edges). For a graph with 100,000 edges where 50 changed, that's a 2,000x difference.

**5. The snapshot chain mirrors the git commit chain.**

Every snapshot links to its parent snapshot, forming a chain:

```
snapshot_C (head=abc123) --> snapshot_B (head=def456) --> snapshot_A (head=789xyz)
```

Each snapshot records which git commit produced it. This means:

- "What did the graph look like at commit X?" is a lookup by commit hash, not a reconstruction
- "What changed between deploy A and deploy B?" is a diff between two snapshot hashes
- Rollback to a previous state means pointing to an older snapshot, not undoing mutations
- Branching and merging git branches could (in theory) branch and merge the graph

This is the exact data model git uses for its commit chain. knowing extends the same principle from "versioned source code" to "versioned code relationships."

**6. Cache invalidation is solved, not approximated.**

In a mutable graph, cache invalidation is the classic hard problem. "Is this blast radius result still valid?" requires re-running the query. In knowing, query results are keyed to snapshot hashes. A result computed against snapshot hash X is valid forever for snapshot X. When the graph changes, it gets a new snapshot hash Y. You know to recompute for Y without checking whether the specific edges in your query changed.

This property enables:
- Sharing computation results across teams (if we have the same snapshot hash, we have the same graph, and your precomputed blast radius is valid for me)
- Caching derived results indefinitely (they never expire, they become irrelevant when a new snapshot supersedes them)
- Verifying graph integrity after network transfer (recompute the Merkle root from the edges; if it matches, the transfer was lossless)

**The bottom line:** every competitor requires explicit re-indexing because they use mutable state. Knowing requires no re-indexing because the architecture makes staleness detectable, changes scopeable, and history structural. This is not an optimization on top of a mutable design; it's a different data model that makes the re-indexing problem structurally impossible.

## Artifact Boundary

knowing decomposes into two planes separated by an artifact boundary:

- The **execution plane** produces the graph (24 extractors, daemon, trace ingestion, graph store).
- The **intelligence plane** interprets the graph (semantic diff, blast radius, staleness analysis, ownership routing).

The **artifact** is the content-addressed graph itself: a SQLite file containing nodes, edges, snapshots, and edge events. It is portable (copy one file), self-contained, and queryable by any tool that understands the schema.

The bright-line rule: intelligence features never write edges, nodes, or snapshots back into the graph. They read the artifact and may produce derived results (which are themselves content-addressed artifacts stored separately). A buggy intelligence feature produces a bad report, not a bad graph.

## Equivalence Classes

Equivalence classes bridge the vocabulary gap between how developers describe tasks in natural language and the symbol names that live in the graph. An equivalence class maps a canonical concept (like `TRANSITIVE_IMPACT`) to a set of natural-language phrases ("blast radius," "downstream callers," "what breaks") and a set of code symbol targets that should be boosted when those phrases appear in a query.

knowing ships **115 seed equivalence classes** organized into three tiers:

- **63 universal classes** (in `internal/context/universal_seeds.go`): software engineering concepts that appear in any codebase (entry points, error handling, caching, authentication, testing, concurrency, etc.). These are language-agnostic and shared across all projects.
- **21 knowing-specific classes** (in `internal/context/equivalence.go`): concepts specific to knowing's own domain (transitive impact, snapshot management, wire format, community detection, feedback loop, etc.). These bootstrap the context engine for queries about knowing itself.
- **31 language-specific classes** (in `internal/context/language_seeds.go`): vocabulary bridges for Python (`__init__`/constructor, Django/Flask patterns), TypeScript (React hooks, Express/Fastify), Rust (trait/impl, Result/Option), Java (Spring annotations), and Kubernetes (resource type aliases).

At runtime, matching a phrase from an equivalence class boosts the seed weight of the associated symbols before the retrieval walk begins. After the walk, graph-derived aliases (from `internal/context/graph_aliases.go`) and session feedback further adjust weights. The 115 seed classes provide the floor; graph learning builds on top.

## Density-Adaptive Retrieval

The retrieval pipeline observes the graph's node count at query time. On graphs exceeding 40K nodes, it automatically enables type-seed preference: reordering RRF candidates so type/interface/class nodes are selected as RWR seeds before methods/functions. Types make better seeds on dense graphs because they have `contains` edges to their methods (the walk propagates downward productively) rather than methods which can only walk upward to callers (competing with thousands of other methods for the same keyword).

This is not a parameter tweak. It's the system detecting that keyword competition on a dense FTS index would degrade precision, and compensating by changing its entry point into the graph. The result: knowing gets more precise as the graph grows, while static retrieval systems (codegraph, GitNexus, Gortex) get less precise.

## Retrieval Seed Channels

The retrieval pipeline uses 5 independent seed channels fused with Reciprocal Rank Fusion (RRF) to select candidates for the Random Walk with Restart:

1. **Tiered keyword matching** (weight 2.0): compound-first exact > prefix > substring > path matching on symbol names.
2. **BM25 FTS5** (weight 2.0): lexical recall over a 6-column FTS5 index (symbol_name, concepts, qualified_name, signature, file_path, doc). The `doc` column indexes docstrings for natural-language BM25 retrieval, bridging the vocabulary gap between task descriptions and symbol names.
3. **Equivalence classes** (weight 2.0): concept-level vocabulary bridging (see above).
4. **Vector/embedding search** (weight 0.0): disabled; infrastructure preserved for future code-tuned models.
5. **Path-context seeding** (weight 1.5): extracts package/directory-like terms from the task description and finds type/class nodes whose qualified name path contains those terms. Types are structural anchors: with `contains` edges, RWR walks from types to their methods.

Symbols appearing in multiple channels accumulate scores, promoting multi-channel hits. See `docs/architecture/retrieval-pipeline.md` for the full specification.

## Content-Addressed Context Packs

A `ContextBlock` (the result returned by `context.ForTask` and `context.ForFiles`) carries a `PackRoot` field of type `types.Hash`. This is the content-addressed identity of the context pack: a hash over the full retrieval result, including all symbols, scores, edges, and metadata.

Two identical queries against the same graph state produce the same `PackRoot`. This has three consequences:

1. **Session deduplication.** The GCF wire format can skip retransmitting symbols whose `PackRoot` was already sent in the current session. The daemon compares `PackRoot` hashes instead of diffing symbol lists.
2. **Cross-team sharing.** If two agents have the same `PackRoot`, they have the same context. Precomputed summaries or annotations can be shared by hash without re-running the retrieval pipeline.
3. **Cache key.** The `SubgraphCache` can use `PackRoot` as a secondary key alongside the Merkle subgraph root, ensuring results are evicted only when the underlying graph changes.

## Community Merkle Roots

Louvain community detection partitions the graph into clusters of densely connected symbols (subsystems, modules, ownership boundaries). Each detected community corresponds to a set of packages. Because the hierarchical Merkle tree assigns a `SubgraphRoot` to any package set in O(1) time, each community gets a stable, content-addressed identity.

Community Merkle roots enable:
- **Parallel cache invalidation.** Two non-overlapping communities have different roots, so their cached data can be invalidated independently.
- **Scoped retrieval.** A retrieval walk can be bounded to a community's package set using `SubgraphRoot` as the cache key, making community-scoped queries cheap to cache and re-run.
- **Ownership routing.** Code review and alerting workflows can route to the owning community by comparing the changed package's `SubgraphRoot` against stored community roots.

See `bench/merkle-diff/context_pack_test.go` (`TestContextPackAndCommunityRoots`) for the benchmark that verifies distinct community roots across disjoint package sets.
