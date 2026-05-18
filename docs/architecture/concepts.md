# Core Concepts

This section defines every term used in the rest of this document. Read it before proceeding.

## Content-Addressed Storage

In content-addressed storage, data is identified by its content, not by a name or location. The identifier is a cryptographic hash (SHA-256) of the data itself. Two pieces of identical data always produce the same hash. Different data always produces different hashes.

This has three consequences:

1. **Deduplication is automatic.** If the same function appears in two repos, it gets the same hash. Store it once.
2. **Integrity is verifiable.** Recompute the hash from the data. If it matches the stored hash, the data is uncorrupted. If it doesn't, something changed.
3. **Cache invalidation is structural.** A query result computed against hash X is valid for all time. When the underlying data changes, it gets a new hash Y. Results keyed to X are still correct for X; results for Y must be recomputed.

knowing uses content-addressed storage for nodes, edges, files, snapshots, and derived computation results. Every piece of data in the system is identified by its hash.

## Merkle DAG

A Merkle DAG (Directed Acyclic Graph) is a data structure where every node contains the cryptographic hash of its children. The root hash summarizes the entire structure: if any leaf changes, the root hash changes.

**The Git analogy:** Git is a Merkle DAG. A commit hash summarizes the entire repository state at that point. If a single byte changes in any file, the commit hash changes. You can verify the integrity of the entire repository by checking the root hash.

knowing works the same way. A snapshot hash is the Merkle root of all edge hashes in the graph at a point in time. If any edge changes, the snapshot hash changes. Two snapshots with the same hash contain exactly the same graph. Two snapshots with different hashes differ in at least one edge.

**How it works in knowing:**

```
                    snapshot_hash (Merkle root)
                   /                           \
            hash(h1+h2)                   hash(h3+h4)
           /          \                  /          \
    edge_hash_1  edge_hash_2    edge_hash_3  edge_hash_4
```

Edge hashes are sorted lexicographically, then paired and hashed upward until a single root remains. Diffing two snapshots is a tree comparison: only changed subtrees need traversal.

## Knowledge Graph vs. Tree vs. Table

A **table** stores flat records. Good for lookups, bad for relationships. "Find all callers of function X" requires a join for each hop.

A **tree** stores hierarchical data (like a file system). Every node has one parent. But code relationships are not hierarchical: function A calls function B, which implements interface C, which is consumed by service D in another repository. A tree cannot represent this.

A **graph** stores nodes connected by edges with no structural constraint on connectivity. A node can have many inbound and outbound edges of different types. This matches the reality of code: a function is called by many callers, implements an interface, lives in a file owned by a team, and is invoked at runtime by three services.

knowing is a knowledge graph because code relationships are inherently graph-shaped. The graph is content-addressed (every node and edge is identified by its hash) and typed (edges carry a type like `calls`, `implements`, or `references`).

## Domain Primitives

| Primitive | What it is | Hash computation |
|-----------|-----------|-----------------|
| **Node** | A symbol in source code (function, type, method, interface, constant, variable). Identified by qualified name. | `sha256(repo \|\| package_path \|\| symbol_name \|\| symbol_kind)` |
| **Edge** | A relationship between two nodes (calls, imports, implements, references). Carries a type, confidence score, and provenance. | `sha256(source_hash \|\| target_hash \|\| edge_type \|\| provenance)` |
| **Hash** | A 32-byte SHA-256 digest used as the content-addressed identifier for every entity. | n/a |
| **Snapshot** | A point-in-time graph state. The Merkle root of all sorted edge hashes. Links to a parent snapshot (forming a chain like git commits) and records the git commit that produced it. | `merkle_root(sorted(all_edge_hashes))` |
| **Provenance** | Metadata on an edge describing how it was derived, by which indexer version, at what confidence, from which commit. Provenance is what lets agents distinguish "confirmed by type checker" from "guessed from string matching." | Included in edge hash input. |

## Event Sourcing

Edges are never mutated in place. Every change to the graph is recorded as an event: an edge was "added" or an edge was "removed," keyed by the snapshot hash that recorded the event. The current graph state is the result of replaying all events (or equivalently, reading the materialized edge table).

This means:
- "When did this edge first appear?" is a query on the event log.
- "What changed between snapshot A and snapshot B?" is a range scan on events filtered by snapshot hash.
- Rolling back to a previous state means pointing to an older snapshot, not undoing mutations.

## Staleness

**Structural staleness:** A file's content hash changed, so all nodes derived from it have stale hashes, and all edges originating from those nodes are suspect. This is detected automatically by hash comparison; no heuristic is needed.

**Heuristic staleness:** An edge has not been re-confirmed by the indexer for N days, or a runtime edge has not been observed in production for N days. This requires time-based reasoning on top of the structural property.

Both forms of staleness are exposed through the `StaleEdges` API. Structural staleness is authoritative. Heuristic staleness is advisory.

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

The snapshot hash is the Merkle root of all edge hashes. If you have the previous snapshot hash and the current snapshot hash, you know instantly whether the graph changed. You don't need to scan edges to find out.

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

- The **execution plane** produces the graph (indexer, daemon, trace ingestion, graph store).
- The **intelligence plane** interprets the graph (semantic diff, blast radius, staleness analysis, ownership routing).

The **artifact** is the content-addressed graph itself: a SQLite file containing nodes, edges, snapshots, and edge events. It is portable (copy one file), self-contained, and queryable by any tool that understands the schema.

The bright-line rule: intelligence features never write edges, nodes, or snapshots back into the graph. They read the artifact and may produce derived results (which are themselves content-addressed artifacts stored separately). A buggy intelligence feature produces a bad report, not a bad graph.
