# Introduction to knowing

This guide builds understanding from zero. No assumed background in content-addressing, Merkle trees, or graph theory. By the end you'll understand why knowing exists, how it works, and what makes it different from every other code intelligence tool.

## System at a Glance

knowing is a local-first, content-addressed code intelligence graph. It runs entirely on a developer laptop with no paid LLM calls and no cloud API dependencies.

**Scale:** 253K nodes and 614K edges on Kubernetes. 94K lines of Go.

**Coverage:** 17 extractors spanning Go, TypeScript, Python, Rust, Java, C#, Ruby, SQL, Proto, GraphQL, Helm, Kubernetes YAML, Dockerfile, Makefile, CloudFormation, GitLab CI, and .env files. 30 edge types (calls, imports, implements, references, similar_to, authored_by, handles_route, publishes, subscribes, connects_to, and 20 more).

**Integrity:** Every node, edge, and snapshot has a SHA-256 hash. A hierarchical Merkle tree organizes edges by package and type, enabling O(packages) diffs and cryptographic proofs of existence and absence.

**Retrieval pipeline:** Tiered keyword search (exact/compound/component) into BM25/FTS, equivalence matching, RWR graph walk, HITS authority scoring, RRF fusion, and budget-constrained knapsack packing.

**Operational characteristics:**
- Daemon mode with file watcher for incremental re-indexing
- Time-to-consistency: 167ms (edit a file, reindex, query finds the new symbol)
- Adjacency cache: 4,717x latency improvement (9s to 2ms on Kubernetes-scale graph)
- LSP enrichment upgrades edge confidence from 0.7 to 0.9
- P@10 = 0.181 reproducible (1.33x codegraph, 13.9x grep baseline)
- MCP server interface with 28 tools for agent consumption

## The Problem

### How AI agents work with code today

When an AI coding agent (Claude Code, Cursor, Copilot) needs to understand your codebase, it does this:

1. Reads the file you're editing
2. Greps for related symbols
3. Reads those files
4. Greps again for callers
5. Reads more files
6. Builds a mental model from fragments
7. Writes code
8. Next turn: forgets everything and starts over

This costs tokens, time, and accuracy. The agent spends 60% of its context window re-reading files it saw last turn. It misses relationships that span multiple files or repos. It has no memory of what was useful last time.

### Why files are the wrong unit

Source code is text organized into files. But the *meaning* of code is relationships:

- Function A calls function B
- Type C implements interface D
- Service E publishes to queue F
- Endpoint G was called 10,000 times yesterday

These relationships cross file boundaries, package boundaries, and repository boundaries. No single file contains the information "this change breaks 14 callers in 3 repos." That information lives in the *graph* of relationships between symbols.

When you ask "what's the blast radius of changing this function signature?", the answer isn't in any file. It's in the set of edges pointing TO that function from everywhere else. Concretely, for `BuildHierarchicalTree` in knowing's own codebase:

```
BlastRadius(BuildHierarchicalTree) = {
  e | e.target = BuildHierarchicalTree ∧ e.type = "calls"
} = {
  SnapshotManager.ComputeSnapshot       --calls-->  BuildHierarchicalTree
  cmd/knowing/prove.cmdProve            --calls-->  BuildHierarchicalTree
  cmd/knowing/prove_absent.cmdProveAbsent --calls--> BuildHierarchicalTree
  cmd/knowing/audit.cmdAudit            --calls-->  BuildHierarchicalTree
  mcp/feedback.computeNeighborhoodRoot  --calls-->  BuildHierarchicalTree
  TestBuildHierarchicalTree_Deterministic --calls--> BuildHierarchicalTree
  TestBuildHierarchicalTree_PackageRoots  --calls--> BuildHierarchicalTree
  TestMerkleDiffBenchmark               --calls-->  BuildHierarchicalTree
  TestContextPackAndCommunityRoots      --calls-->  BuildHierarchicalTree
  cache.TestInvalidatePackages          --calls-->  BuildHierarchicalTree
  ... (10+ callers across 5 packages)
}
```

Change `BuildHierarchicalTree`'s signature? Every one of these breaks. No single file contains this information. The edges do.

### What existing tools provide

| Tool | What it knows | What it misses |
|---|---|---|
| **LSP (gopls, pyright)** | References within one workspace | Cross-repo callers, history, runtime behavior |
| **grep/ripgrep** | Text matches | Semantic relationships (string match != function call) |
| **Dependency graphs** | Package-level imports | Function-level callers, routes, runtime traffic |
| **APM (Datadog, etc)** | What happened in production | What the code declares, static blast radius |

None of them version the relationships. None of them can prove a relationship existed at a specific point in time. None of them learn from use.

### The emerging landscape

The AI coding agent era has produced several categories of tools trying to solve the context problem:

**Context packers** (Aider, Repo Map, etc) analyze your repo and produce a condensed map for the agent's context window. They run at query time, produce text, and are stateless: they don't remember what was useful last time. They don't version their output or prove anything about it.

**Code graphs / indexers** (Sourcegraph, codegraph, GitNexus, Stack Graphs) build a queryable index of code relationships. Most use mutable state (database rows with auto-increment IDs). They can answer "who calls X?" but can't answer "who called X last Tuesday?" or "prove no one calls X." They don't learn from feedback. In head-to-head benchmark (7 repos, 117 tasks): knowing achieves 1.63x the precision of codegraph (19K stars) and 1.51x vs codebase-memory (2.6K stars).

**Agent memory systems** (MemGPT, various RAG frameworks) persist information across sessions. They remember conversations but not code structure. They can recall "you asked about auth last time" but can't tell you "auth's blast radius grew by 3 callers since then."

**Runtime observability** (OpenTelemetry, Datadog, Honeycomb) tracks what actually happens in production. Rich temporal data but disconnected from source code. Knows "service A called service B 10,000 times today" but not "the code that enables this call lives in file X at line 42."

**Where knowing sits:**
knowing combines elements of all four categories into one system:
- Builds a code graph (like Sourcegraph) but content-addressed and versioned
- Packs context for agents (like Aider) but graph-ranked and cached
- Remembers what was useful (like agent memory) but expires when code changes
- Can ingest runtime traces (like APM) and compare against static analysis
- Does something none of them do: cryptographic proofs of existence and absence

The categories overlap, but no existing tool occupies knowing's exact position: a versioned, provable, learning code graph that serves both agents and auditors from the same foundation.

## What a Code Graph Is

### Why a graph, not a tree

A tree enforces one parent per node. File systems are trees: every file has exactly one directory. But code relationships violate this constantly:

- A function is called by **12 different callers** (12 inbound edges, not 1 parent)
- A type implements **3 interfaces** simultaneously
- A service publishes to a queue AND handles HTTP routes AND connects to a database
- A symbol in repo A is called by repos B and C (cross-repo fan-in)
- Runtime traces show a call path that static analysis doesn't see (multiple views of the same relationship)

You can't represent "function X is called by A, B, and C, implements interface D, handles route /users, and was observed calling external service E" in a tree without duplicating nodes or losing edges. A graph stores all of these naturally: multiple inbound edges, multiple outbound edges, multiple types, no structural constraint.

The Merkle tree is layered ON TOP of the graph for integrity and efficient querying. It doesn't constrain the graph's topology. The tree organizes edges by package and type; the graph stores the actual multi-directional relationships.

### Structure

A code graph has two things:

**Nodes** are symbols: functions, methods, types, interfaces, variables, routes, database tables, queue topics, config keys.

**Edges** are relationships between symbols: calls, imports, implements, references, handles_route, publishes, subscribes, connects_to.

```
[OwnerController.createOwner] --calls--> [OwnerRepository.save]
[OwnerController.createOwner] --handles_route--> [POST /owners/new]
[OwnerRepository] --implements--> [Repository interface]
```

Every entity in the graph is content-addressed via SHA-256. The hash formulas (covered in detail in the next section) are:

- **Node hash** = `sha256("node\0" + repoURL + packagePath + qualifiedName + kind)`
- **Edge hash** = `sha256("edge\0" + sourceHash + targetHash + edgeType + provenance)`
- **File hash** = `sha256(file content)`
- **Snapshot hash** = Merkle root of all edge hashes (hierarchical tree over the full graph)

This means identity is derived from content. Same relationship on any machine produces the same hash. Different relationship produces a different hash. No assigned IDs, no coordination, no ambiguity.

### Node metadata

Each node carries:

| Field | Description |
|---|---|
| `qualified_name` | Fully qualified symbol name (e.g., `github.com/org/repo/pkg.FuncName`) |
| `kind` | Symbol type: function, method, type, interface, variable, route, table, topic, config |
| `file_hash` | SHA-256 of the file content at time of indexing; changes when the file changes |
| `line` | Line number in the source file where the symbol is defined |
| `signature` | Type signature or function prototype (language-dependent) |
| `last_author` | Git blame author of the line where the symbol is defined |
| `last_commit_ts` | Timestamp of the most recent commit touching the symbol's definition |

The file hash links a node to a specific file state. When a file is re-indexed, its new hash is compared against the stored hash. If they differ, all nodes in that file are re-extracted. If they match, the file is skipped entirely (no re-parsing needed).

### Edge metadata

Each edge carries:

| Field | Description |
|---|---|
| `type` | Relationship kind: calls, imports, implements, references, similar_to, etc. (30 types total) |
| `confidence` | Float 0.0 to 1.0 indicating certainty of the relationship |
| `provenance` | Discovery method: `ast_inferred`, `lsp_resolved`, `git_blame`, `similarity`, `runtime` |
| `file` | Source file where the call site exists |
| `line` | Line number of the call site |
| `col` | Column number of the call site |

The call site location (file, line, col) lets consumers jump directly to where the relationship is expressed in source code.

### Provenance tiers

Provenance is not just a label; it implies a confidence floor:

| Provenance | Confidence floor | Meaning |
|---|---|---|
| `ast_inferred` | 0.7 | Tree-sitter parsed the syntax and inferred a relationship. No type resolution. May be wrong if a local variable shadows a package name. |
| `lsp_resolved` | 0.9 | A language server (gopls, pyright, rust-analyzer) resolved the call site to a concrete symbol with full type information. Nearly certain. |
| `runtime` | 1.0 | Observed in production via OpenTelemetry trace or runtime instrumentation. The relationship definitely exists at runtime. |
| `git_blame` | 0.8 | Derived from git history (authored_by edges, temporal co-change). Reliable but reflects past state. |
| `similarity` | 0.6 | Embedding-based similarity detected structural resemblance. Useful for discovery, not proof. |

When a higher-tier extractor confirms a relationship already found by a lower-tier extractor, the confidence is upgraded in place. The edge hash remains the same (same source, target, type, provenance), but the mutable confidence field reflects the stronger signal. Consumers can filter by confidence threshold to control precision vs. recall.

### Snapshots and the snapshot chain

Each index run produces a new **snapshot**. A snapshot is a single hash (the Merkle root of all edge hashes in the hierarchical tree) tied to a git commit SHA. It represents the complete state of the graph at that point in time.

The snapshot chain is the sequence of snapshots across index runs:

```
Snapshot S1 (commit abc123) -> Snapshot S2 (commit def456) -> Snapshot S3 (commit 789ghi)
```

Diffing two snapshots reveals exactly what changed between them: new edges added, edges removed, confidence upgrades. The diff is O(packages) because only packages with differing Merkle roots need investigation (see the hierarchical tree section below).

This enables:
- **Incremental processing:** only re-process files whose hashes changed since the last snapshot.
- **Temporal queries:** "when did this call edge first appear?" Walk the snapshot chain backward.
- **Audit trails:** "prove the graph state at commit X." The snapshot hash is the proof anchor.

### Edge events

Every edge creation and deletion is recorded as a timestamped **event**:

```
{ action: "created", edge_hash: "7b3c...", snapshot: "S2", timestamp: "2024-01-15T10:32:00Z" }
{ action: "deleted", edge_hash: "a27e...", snapshot: "S3", timestamp: "2024-01-16T08:11:00Z" }
```

Edge events enable temporal analysis: "when did service A stop calling service B?", "how many new call edges appeared in the last week?", "which edges are churn-heavy (created and deleted repeatedly)?" The event log is append-only; events are never modified after creation.

### Why content-addressing matters (summary)

The content-addressed design gives four structural properties that pervade the system:

1. **Deduplication.** The same symbol or relationship, discovered by multiple extractors or across multiple index runs, produces the same hash. Store once, reference everywhere.
2. **Integrity verification.** Recompute any hash from its inputs. If it matches, the data is uncorrupted. This works recursively up to the snapshot root.
3. **Incremental diffing.** Compare two snapshot hashes: if equal, nothing changed. If different, walk the tree to find exactly what changed. No full scans.
4. **Merkle proofs for audit trails.** Any edge's existence (or absence) in a snapshot can be proven with a small proof file (under 1KB) that verifies offline using only SHA-256.

The next section ("Why Content-Addressing") covers the mechanics in depth: domain prefixes, worked examples, cache implications, and what happens when extractors are wrong.

The graph represents what your codebase *understands about itself*: who calls what, who depends on what, what routes exist, what runtime traffic looks like.

### Edges are the intelligence, nodes are the vocabulary

Both nodes and edges are stored, content-addressed, and queryable. But the Merkle tree (the versioning structure) is built from **edges**, not nodes. This is deliberate.

A node's existence rarely changes: functions get added or removed occasionally. But relationships change constantly: new callers appear, imports shift, runtime traffic patterns evolve, dependencies get added or removed. The edge set is where the interesting changes happen.

If you built the tree from nodes, "did anything change?" would mean "was a symbol added or removed?" That's a coarse signal. Building from edges means "did any relationship change?" which captures: new callers, removed dependencies, changed routes, different runtime traffic. Every downstream operation (diff, cache invalidation, proofs, blast radius) cares about relationships, not symbol existence.

Think of it this way: knowing that `CreateOwner` exists tells you almost nothing. Knowing that `CreateOwner` calls `save`, handles `POST /owners/new`, is called by 3 controllers, and was observed 10K times in production: that's the intelligence. The edges carry the meaning. The nodes are just anchor points.

## Why Content-Addressing

### The idea

In a normal database, data has an ID assigned by the system (auto-increment, UUID). The ID is arbitrary: it has no relationship to the content. You could swap the content and keep the ID and nothing would detect it.

In a content-addressed system, the ID IS the content. Specifically, it's a cryptographic hash (SHA-256) of the content.

**What's a cryptographic hash?** A function that takes any input (a byte, a file, a sentence) and produces a fixed-size output (32 bytes for SHA-256). Two key properties:

1. **Deterministic.** Same input always produces the same output. Always. On any machine.
2. **Collision-resistant.** Two different inputs producing the same output is so unlikely (1 in 2^256) that we treat it as impossible.

This means: if you know the hash, you know exactly what the content must be. If the content changes by even one bit, the hash changes completely (the "avalanche effect").

### Domain prefixes: preventing cross-type confusion

knowing hashes four types of things: nodes, edges, snapshots, and Merkle tree interior nodes. Without care, a node hash could accidentally match an edge hash (different data that happens to SHA-256 to the same value). To prevent this, every hash computation starts with a domain prefix:

```
NodeHash     = SHA-256("node\0"     + repoURL + packagePath + symbolName + symbolKind)
EdgeHash     = SHA-256("edge\0"     + sourceHash + targetHash + edgeType + provenance)
SnapshotHash = SHA-256("snapshot\0" + merkleRoot)
MerkleNode   = SHA-256("merkle\0"   + leftChild + rightChild)
```

The `\0` (null byte) separates the prefix from the data. This is the same pattern git uses: git hashes files as `"blob <size>\0<content>"`. The prefix makes it structurally impossible for a node hash to collide with an edge hash, because their inputs always start with different bytes.

### The four hashed entity types

knowing's content-addressed universe consists of exactly four types. The first three (nodes, edges, snapshots) are **domain objects**: they represent code and its relationships. The fourth (Merkle interior nodes) is **structural**: it exists solely for efficient verification and diffing, not to represent any code entity.

#### 1. Nodes

**Hash formula:** `sha256("node\0" + repoURL + qualifiedName + kind)`

A node represents a code symbol: a function, type, method, variable, route, table, or topic. The hash uniquely identifies this symbol across all repositories. Two developers indexing the same repo at the same commit produce the same node hashes because identity is derived entirely from the symbol's location and kind. Renaming a function or moving it to a different package produces a new hash (it is now a different node).

#### 2. Edges

**Hash formula:** `sha256("edge\0" + sourceHash + targetHash + edgeType + provenance)`

An edge represents a relationship between two nodes: calls, imports, implements, references, similar_to, authored_by, and 24 others. The provenance (discovery method) is part of the hash because the same relationship discovered by different methods (tree-sitter vs LSP vs runtime trace) produces different edges with different confidence levels. This is intentional: a tree-sitter guess at confidence 0.7 and an LSP confirmation at confidence 0.9 are tracked as the same edge (same source, target, type, provenance category), but a runtime-observed call is a distinct edge from a statically-inferred one, because the evidence is fundamentally different.

#### 3. Snapshots

**Hash formula:** Merkle root of all edge hashes in the repository at index time.

A snapshot represents the complete state of a repository's graph at a point in time. It is the top of the Merkle tree: one 32-byte value that summarizes every relationship in the repository. Comparing two snapshot hashes tells you instantly whether anything changed (O(1) equality check). If they match, the entire graph is identical. If they differ, you descend the tree to find what changed. Each snapshot is tied to a git commit SHA, creating an auditable chain of graph states over time.

#### 4. Merkle tree interior nodes

**Hash formula:** `sha256("merkle\0" + leftChildHash + rightChildHash)`

These are NOT code entities. They exist only for integrity verification and efficient diffing. The tree is hierarchical: leaves are edge hashes (Level 1), Level 2 groups edges by package into subgraph roots, and Level 3 is the repository root (the snapshot hash). Interior nodes enable O(log n) diff: to find which packages changed between two snapshots, compare Level 2 hashes; only descend into children that differ. This is how knowing achieves 167ms time-to-consistency: it does not recompute unchanged packages. Interior nodes also provide the sibling hashes needed for Merkle inclusion and absence proofs.

#### Summary

| Type | Hash inputs | Represents | Purpose |
|---|---|---|---|
| **Node** | repoURL + qualifiedName + kind | A code symbol | Unique identity for any symbol across all repos |
| **Edge** | sourceHash + targetHash + edgeType + provenance | A relationship between symbols | Track every connection, with provenance-aware deduplication |
| **Snapshot** | Merkle root of all edges | Complete graph state at a point in time | O(1) change detection, audit anchoring, cache keys |
| **Merkle interior node** | leftChildHash + rightChildHash | Nothing in the domain (structural only) | O(log n) diffing, proof generation, scoped cache invalidation |

The key architectural insight: nodes, edges, and snapshots model the problem domain (code and its relationships). Merkle interior nodes model the verification and query optimization structure layered on top. Consumers of the graph interact with the first three; the fourth is invisible infrastructure that makes the system fast and provable.

### A worked example

Say you have a function `CreateOwner` in package `petclinic/owner` in repo `github.com/spring-projects/spring-petclinic`:

```
Input:  "node\0" + "github.com/spring-projects/spring-petclinic"
                  + "org.springframework.samples.petclinic.owner"
                  + "CreateOwner"
                  + "method"

Output: a27eac262d3e6a8f7c59a220cec65ce426bcdae95f14f0b24007e0312f87fc03
```

That 64-character hex string (32 bytes) is the node's permanent identity. If you rename the function, the hash changes. If you move it to a different package, the hash changes. If everything stays the same, the hash stays the same. On any machine, forever.

Now say `CreateOwner` calls `OwnerRepository.save`. The edge hash:

```
Input:  "edge\0" + a27eac26...  (CreateOwner's hash)
                  + f891bb04...  (save's hash)
                  + "calls"
                  + "ast_inferred"

Output: 7b3c910f4d82a1e5...
```

The edge's identity is derived from WHAT it connects (source + target hashes), HOW (edge type), and WHO discovered it (provenance). Same relationship = same hash. Different relationship = different hash.

### Why this matters

This single design choice gives you six properties for free:

**1. Staleness detection without scanning.**
A file changed? Recompute its nodes' hashes. Any hash that differs from what's stored means that node changed. You know exactly what's stale without scanning the entire graph. The changed hashes propagate: edges FROM those nodes are suspect. Edges TO those nodes from unchanged code are still valid.

**2. Caching without invalidation logic.**
A query result computed against snapshot hash X is valid forever for X. The hash is the cache key AND the validity check. No TTLs. No background invalidation threads. No "maybe this is stale." If the hash matches, the result is correct. Period.

**3. Integrity verification.**
Recompute the hash from the stored data. If it matches the stored hash, the data is uncorrupted. If it doesn't, someone (or something) modified the data after it was indexed. This works recursively: verify one edge's hash, then verify the tree node above it, all the way to the root. One root hash verifies the entire graph.

**4. Determinism.**
Same source code + same analyzer version = same hashes. On any machine, at any time, by any operator. Two people who independently index the same repo at the same commit get the same snapshot hash. CI and local development produce identical graphs. There is no "it indexed differently on my machine."

**5. Efficient equality.**
"Did the graph change?" is a single 32-byte comparison (compare two snapshot hashes). Not a full scan. Not a diff. One `==` operation. O(1). This matters when you're checking cache validity thousands of times per second.

**6. Natural deduplication.**
The same symbol appearing in two contexts (e.g., a function called from 50 places) gets the same node hash. Store it once. Reference it 50 times by hash. The hash IS the pointer.

### What happens when the extractor is wrong

Confidence scores exist because extractors make mistakes. When tree-sitter parses `auth.Login()` it doesn't know if `auth` is an imported package, a local variable, or a struct field. It infers "probably a call to something named Login in something named auth" at confidence 0.7.

Later, if the LSP server (gopls, pyright) resolves the same call site to a specific function with full type information, the edge gets upgraded to confidence 0.9-1.0. The edge hash stays the same (same source, target, type, provenance). Only the mutable confidence metadata changes.

If the extractor is completely wrong (infers a call that doesn't exist), the edge will have confidence 0.7 and no LSP confirmation. Over time, if nobody queries it and nobody confirms it, it sits there as low-confidence data. `knowing fsck` won't flag it (referential integrity is intact). But ranking will deprioritize it: low-confidence edges carry less weight in graph walks.

**The system tolerates noise but prefers signal.** Confidence is the mechanism that separates tree-sitter guesses from LSP confirmations from SCIP proofs from runtime observations. A consumer can filter by confidence threshold.

### How git uses the same idea

Git is the original content-addressed system for developers. A commit hash summarizes the entire repository state. If a single byte changes in any file, the commit hash changes. You verify integrity by checking the root hash.

But git content-addresses *file contents*. knowing content-addresses *relationships between code*. Git can tell you "file auth.go changed." knowing can tell you "3 new callers of ValidateToken appeared, the blast radius of SessionHandler grew by 2, and the call edge to DeprecatedAuth was removed."

## Why Hierarchical

### The limitation of flat hashing

A flat content-addressed system can tell you "something changed" (the root hash is different). But it can't tell you WHAT changed without scanning everything.

Imagine you have 100,000 edges in your graph. Something changed. Which edge? With a flat hash, you compare all 100,000 edges between the old and new state. That's O(edges). At Kubernetes scale (614K edges), this takes seconds. For a CI pipeline that runs on every push, seconds add up.

### The hierarchical tree

knowing organizes edges into a three-level Merkle tree. Each level serves a distinct purpose:

**Level 1: Edge level (leaves).** Individual edge hashes form the leaves of the tree. Each edge hash is computed as `sha256("edge\0" + sourceHash + targetHash + edgeType + provenance)`. These are the atomic units of the graph; every relationship between two symbols is one leaf.

**Level 2: Package/subgraph level (interior).** All edges belonging to a single package are grouped by edge type, then hashed into a **subgraph root** (also called the package root). This root is a single 32-byte hash that summarizes every relationship within that package. If any edge in the package changes (added, removed, or replaced), the subgraph root changes. If nothing in the package changes, the subgraph root is identical. This level is what enables per-package diffing and per-package cache invalidation.

**Level 3: Repository level (root).** All subgraph roots (one per package) are sorted and hashed into a single **repository Merkle root**. This is the snapshot hash. One 32-byte value represents the entire graph state. Comparing two repository roots is O(1); if they match, nothing changed anywhere. If they differ, descend to Level 2 to find which packages changed, then to Level 1 to find which edges changed.

The three levels correspond to three granularities of operation:
- **Edge granularity:** prove a specific relationship exists or doesn't exist.
- **Package granularity:** invalidate caches, scope diffs, expire feedback, compute blast radius for one package.
- **Repository granularity:** verify entire graph integrity, compare snapshots, anchor audit proofs.

```
Level 3 (repository root)
repo root = SHA-256("merkle\0" + left_child + right_child)  [of sorted package roots]
  |
  +-- Level 2 (package/subgraph roots)
  |   package root [internal/auth] = merkle(sorted edge-type roots)
  |     |
  |     +-- edge-type root [internal/auth:calls] = merkle(sorted edge hashes)
  |     |     |
  |     |     +-- Level 1 (edge leaves)
  |     |     +-- edge hash: a27e...  (CreateOwner -> save)
  |     |     +-- edge hash: b91f...  (CreateOwner -> validate)
  |     |     +-- edge hash: c44d...  (ListOwners -> findAll)
  |     |
  |     +-- edge-type root [internal/auth:imports] = merkle(sorted edge hashes)
  |           |
  |           +-- edge hash: d55e...  (auth -> repository)
  |
  +-- package root [internal/store] = merkle(sorted edge-type roots)
        |
        +-- edge-type root [internal/store:calls] = merkle(sorted edge hashes)
              |
              +-- edge hash: e66f...  (save -> db.Exec)
              +-- edge hash: f77a...  (findAll -> db.Query)
```

Within Level 2, edges are further grouped by edge type (calls, imports, implements, etc.) before being Merkle-hashed. This adds a sub-level that allows even finer diffing: "package X changed, specifically its `calls` edges, not its `imports` edges." In practice this means you can determine not just which package was affected but what kind of relationship changed.

**How interior nodes are computed:**

Each interior node is `SHA-256("merkle\0" + left_child_hash + right_child_hash)`. Leaves are the raw edge hashes. If there's an odd number of children, the last one is paired with itself (standard Merkle tree padding).

The leaves at each level are sorted by `bytes.Compare` before tree construction. This makes the root deterministic regardless of insertion order: same edges = same root, always.

**Per-package diffing in practice:**

Because each package has its own subgraph root, diffing two snapshots only requires comparing subgraph roots (one comparison per package). Only packages whose subgraph roots differ need deeper investigation. For a repository with 500 packages where 3 changed, the diff algorithm performs 500 hash comparisons at Level 2, then drills into only those 3 packages at Level 1. The other 497 packages are proven unchanged by a single `==` on their 32-byte root.

This same property enables integrity proofs at any granularity:
- Prove a single edge exists: inclusion proof from leaf to repository root (~16 hash steps).
- Prove a package is unchanged: compare its subgraph root against a trusted snapshot (1 comparison).
- Prove the entire graph is intact: recompute the repository root from all subgraph roots and compare (O(packages)).

### Worked example: detecting a change

Suppose someone adds a new call edge in `internal/auth` (a new function calls `validate`). Here's what changes:

```
Before:
  edge-type root [internal/auth:calls] = merkle(a27e, b91f, c44d) = X
  package root [internal/auth] = merkle(X, imports_root) = P
  repo root = merkle(P, store_root) = R

After (new edge e88b added):
  edge-type root [internal/auth:calls] = merkle(a27e, b91f, c44d, e88b) = X'  (CHANGED)
  package root [internal/auth] = merkle(X', imports_root) = P'  (CHANGED, because X changed)
  repo root = merkle(P', store_root) = R'  (CHANGED, because P changed)
```

The change cascades up. But notice: `store_root` didn't change. `internal/store`'s package root is identical. Any cached computation scoped to `internal/store` is still valid.

**The diff algorithm:**
1. Compare repo roots: R != R'. Something changed.
2. Compare package roots: P != P' (auth changed), store_root == store_root (store didn't).
3. For changed packages, compare edge-type roots: X != X' (calls changed), imports_root == imports_root.
4. Only drill into the changed edge-type to find the specific new edge.

Steps 1-3 are O(packages). Step 4 is O(edges in that one edge-type). Total work: proportional to what changed, not to the total graph size.

### What this enables

**O(packages) diff instead of O(edges):**
"What changed?" Compare package roots. Only packages with different roots need investigation. For a 100,000-edge graph with 500 packages where 3 changed, that's 500 comparisons instead of 100,000. Benchmarked: 565x faster at 100K edges.

**Semantically meaningful output:**
The diff doesn't say "edge at position 47,832 changed." It says "package `internal/auth` changed, specifically the `calls` edges." That's actionable information. You can route it to the auth team. You can scope your cache invalidation to auth callers only.

**Scoped cache invalidation:**
Your cached blast-radius result for `internal/store` is still valid even though `internal/auth` changed. The package root for `internal/store` didn't change, so anything computed from it is still correct. The cache key IS the package root. When you check "is my cache valid?", you compare one 32-byte hash. 42 nanoseconds.

**Subgraph queries:**
"Give me a single hash that represents the state of these 5 packages." That's a `SubgraphRoot` computation: sort the 5 package roots, build a Merkle tree over them. Use it as a cache key, a validity check, or a proof anchor. 1.5 microseconds at Grafana scale.

### How proofs work (mechanically)

An inclusion proof answers: "Prove that edge X exists in the tree rooted at R."

The proof is a path from the leaf (edge hash) to the root, plus the sibling hashes at each level that let a verifier reconstruct the root:

```
Edge hash: a27e...
Level 1 (leaf -> edge-type root):
  Step 1: sibling = b91f..., direction = right   (I'm left, sibling is right)
  Step 2: sibling = c44d..., direction = right   (I'm left at next level)
  ... (log2(edges_in_this_type) steps)

Level 2 (edge-type root -> package root):
  Step 1: sibling = imports_root, direction = right
  ... (log2(edge_types_in_package) steps)

Level 3 (package root -> repo root):
  Step 1: sibling = store_root, direction = right
  ... (log2(packages) steps)
```

**What the verifier does:**
1. Start with the edge hash.
2. At each step, combine with the sibling: `SHA-256("merkle\0" + me + sibling)` (or sibling + me, depending on direction).
3. The result becomes the input for the next step.
4. After all steps, the final value should equal the claimed root.
5. If it does: the edge is proven to exist in this tree. If it doesn't: the proof is invalid.

The verifier needs: the edge hash, the proof steps (sibling hashes + directions), and the expected root. Nothing else. No database. No network. Just SHA-256.

**Total proof size:** ~16 steps for a graph with 13K edges (log2 at each of 3 levels). Each step is 33 bytes (32-byte hash + 1-byte direction). Total: ~660 bytes + metadata. The proof is a self-contained JSON file.

### How absence proofs work

An absence proof answers: "Prove that edge X does NOT exist in the tree."

The mechanism: leaves are sorted. If edge X doesn't exist, there must be two adjacent leaves A and B such that A < X < B (in byte order). If you can prove A is in the tree, prove B is in the tree, and show they're adjacent (no room between them), then X cannot be there.

```
Sorted leaves: ... a27e, b91f, c44d, e88b, f77a ...

Missing edge hash: d55e...

Left neighbor:  c44d... (largest leaf < d55e)
Right neighbor: e88b... (smallest leaf > d55e)

Proof: two inclusion proofs (one for c44d, one for e88b) against the same root.
```

The verifier checks:
1. Left neighbor proof verifies against the root.
2. Right neighbor proof verifies against the root.
3. Left < missing < right (byte order).
4. Left and right are adjacent in the tree (no leaf between them).

If all four pass: the missing edge cannot exist in this tree. The sorted structure guarantees it.

This is the same principle Certificate Transparency uses to prove a certificate was never issued. Applied to code relationships.

## What knowing Does With This

### 1. Context Engine (for AI agents)

Instead of grep-read-grep-read, one call:

```bash
knowing context -task "refactor auth middleware" -format gcf
```

**What happens inside that call:**

**Step 1: Keyword extraction.**
Parse the task description into a `KeywordSet` with three tiers: Exact (backtick-quoted identifiers like `before_request`), Compounds (snake_case, CamelCase, dotted identifiers found in the text), and Components (individual split words, used as fallback). Expand abbreviations ("auth" -> "authentication", "authorize"). Filter stop words. Generate bigram compounds from adjacent words (e.g., "auth" + "middleware" -> "auth_middleware"). The structured set ensures compound identifiers are queried first, preventing split fragments from drowning out the actual symbol names.

**Step 2: Seed selection (compound-first tiered search).**
Find symbols in the graph using a tiered search strategy that respects the KeywordSet structure:
1. Tier 1: Exact match on Exact + Compound keywords (full identifier match)
2. Tier 2: Prefix match on Exact + Compound keywords (symbol name starts with compound)
3. Fallback to Components only when Tiers 1-2 yield fewer than 5 results

This prevents "before" and "request" from filling results before "before_request" is found. Each matched symbol becomes a "seed" for the graph walk, weighted by which tier matched it.

**Step 3: Random Walk with Restart (RWR).**
Starting from the seeds, simulate a random walk on the graph. At each step, either follow an edge to a neighbor (probability 0.8) or "restart" by jumping back to a seed (probability 0.2, i.e., alpha=0.2). Repeat for many steps. The probability of landing on each node after convergence is its "relevance score." Community-aware filtering: when all seeds cluster in a single community, the walk is constrained to that community, producing more focused results.

Intuition: symbols that are close to many seeds (reachable by short paths) get high scores. Symbols that are far away or only reachable through long chains get low scores. The restart probability prevents the walk from drifting too far from the task.

**Step 4: HITS reranking.**
On the top-K results from RWR, run the HITS algorithm (Hyperlink-Induced Topic Search). This separates "hubs" (symbols that call many things) from "authorities" (symbols that are called by many things). For a refactoring task, authorities matter more (the things being called). For a wiring task, hubs matter more (the connectors).

**Step 5: Feedback boost.**
Feedback boost is computed as `0.5 + recall_score * 0.4`, where recall_score comes from task memory (top-5 symbols stored per query). Feedback automatically expires when the package's SubgraphRoot changes (code was modified). This is how the system learns over time without accumulating stale signals.

**Step 6: Token budget packing.**
The ranked symbols are packed into the token budget (default 50,000 tokens) using a knapsack algorithm: maximize total relevance score within the budget constraint. Larger symbols (more edges, longer signatures) cost more tokens. The packer selects the combination that maximizes information density.

**Result:** 85-200 symbols, ranked by graph relevance, packed into a budget. One call. No grep loops.

The results improve with use: feedback records which symbols were actually useful, and that signal compounds across sessions while automatically expiring when code changes.

### 2. Audit Primitive (for compliance)

```bash
knowing prove -source "PaymentService" -target "UserDB" -type calls -human
```

Generates a cryptographic Merkle proof that a relationship exists (or doesn't exist) at a specific git commit. The proof is 3KB of JSON. It verifies offline without database access. An auditor needs only the proof file and a SHA-256 implementation.

This is something no other code intelligence tool can do. Proving a negative ("service A does NOT call service B") requires sorted leaf adjacency proofs: you prove the two neighbors that bracket the gap, demonstrating there's no room for the missing edge.

### 3. Memory Layer (that learns)

**The problem with persistent feedback:**
If you record "symbol X was useful" and then completely rewrite X, the old feedback is now wrong. It points at code that no longer exists in the same form. Over time, a system that never expires feedback accumulates noise: stale positive signals for code that has been rewritten, reorganized, or deleted.

**Merkleized feedback validity:**
When you mark a symbol as "useful," the feedback record stores two things:
1. The symbol's hash (what was useful)
2. The SubgraphRoot of the symbol's package at that moment (what the surrounding code looked like)

The SubgraphRoot is the Merkle root of all edges in that package. It changes when any edge in the package changes (a function is added, renamed, or deleted).

**Automatic expiration:**
When querying feedback later, knowing compares the stored SubgraphRoot against the CURRENT SubgraphRoot for that package. If they differ (code changed), the feedback is invisible. If they match (code is the same), the feedback applies.

```
Record feedback:
  symbol_hash: a27e...
  useful: true
  neighborhood_root: 9dc3...  (SubgraphRoot of internal/auth at this moment)

Query feedback later:
  Current SubgraphRoot of internal/auth = 9dc3...  (matches! feedback valid)

After code changes:
  Current SubgraphRoot of internal/auth = f891...  (doesn't match! feedback expired)
```

No manual cleanup. No TTLs. No background jobs. The Merkle root IS the validity check. Same mechanism as caching, applied to feedback.

**Why this matters at scale:**
After 1,000 sessions of agent feedback, the system has thousands of signal records. Without expiration, old signals from rewritten code would pollute current rankings. With merkleized expiration, only signals from code that's still in its current form apply. The system can run for years and get smarter without getting noisier.

**Measured impact:** Precision improves from 16% to 50% over 5 rounds of feedback. The improvement is immediate (first round = +20pp) and sustained (doesn't degrade in subsequent rounds). The 11% overhead of checking neighborhood roots (255us -> 284us per 100 symbols) is negligible compared to the retrieval benefit.

## How It Fits Together

```
Source code (files)
    |
    v
Daemon (file watcher, incremental re-indexing, 167ms time-to-consistency)
    |
    v
Extractors (17 extractors: tree-sitter, LSP, SCIP, YAML/config parsers)
    |
    v
Code Graph (nodes + edges + provenance + confidence)
    |
    v
Hierarchical Merkle Tree (repo -> packages -> edge-types -> leaves)
    |
    v
Snapshot (one hash = entire graph state, tied to git commit)
    |
    +--> Context Engine (ranked retrieval for agents, cross-repo identity)
    +--> Proof System (inclusion + absence proofs, offline verifiable)
    +--> Diff Engine (O(packages), semantic output)
    +--> Feedback Loop (merkleized, self-expiring)
    +--> Adjacency Cache (4,717x latency improvement, 9s -> 2ms at k8s scale)
    +--> MCP Server (28 tools for agent consumption)
```

The Merkle tree is not just an integrity mechanism. It's the query optimization substrate. The same structure that proves "this edge exists" also invalidates stale caches, scopes diffs to changed packages, and expires old feedback. One data structure, five use cases.

The daemon watches the filesystem and triggers incremental re-indexing on file changes. From the moment a developer saves a file to the moment a query returns the new symbol: 167ms. The adjacency cache precomputes neighbor lookups, reducing graph traversal latency from 9 seconds to 2 milliseconds on Kubernetes-scale graphs (253K nodes, 614K edges).

Since the initial architecture, knowing has added cross-repo awareness (external packages get canonical identity hashes like `external://flask` or `stdlib`), community-aware graph walks (constrain RWR to seed communities for focused queries), and 28 MCP tools for agent integration. The system requires no cloud services; everything runs locally with zero configuration.

## The Landscape

### What other systems use content-addressing for

| System | What it content-addresses | What it enables |
|---|---|---|
| **Git** | File contents | Version history, integrity, distributed collaboration |
| **IPFS** | Arbitrary data blocks | Distributed storage, deduplication |
| **Nix** | Build inputs | Reproducible builds, binary caching |
| **Unison** | Code definitions (AST hash) | Rename-immune identity, structural editing |
| **knowing** | Code relationships (edges) | Scoped diff, caching, proofs, feedback expiration |

knowing is unique in applying content-addressing to *relationships between code*, organized by *semantic boundaries* (packages, edge types), and using the resulting Merkle structure as a *query optimization substrate*.

### Measured performance

These numbers are reproducible via `knowing eval` on the benchmark suite (7 repos, 117 tasks):

| Metric | Value | Context |
|---|---|---|
| P@10 (precision at 10) | 0.181 | 1.33x codegraph (19K stars), 13.9x grep baseline |
| Adjacency cache latency | 2ms | Down from 9s uncached on k8s (4,717x improvement) |
| Time-to-consistency | 167ms | File edit to query returning new symbol |
| Feedback impact | +34pp over 5 rounds | 16% baseline to 50% with feedback |
| Index throughput | 494x incremental vs full | IndexFilesIncremental on warm graph |
| Similarity edges | +19.5% MRR | Embedding-based similar_to edges vs graph-only |

All measurements taken on developer hardware (no GPU, no cloud). The system is designed to be fast enough that agents never wait for it.

### What knowing is NOT

- **Not a cloud service.** It runs entirely on a developer laptop. No API keys, no network calls, no paid LLM. Local-first by design.
- **Not a code search engine.** It doesn't index text. It indexes relationships.
- **Not an LSP replacement.** LSP provides single-workspace references. knowing provides cross-repo, historical, runtime-aware graph queries. LSP is an input (enrichment source), not a competitor.
- **Not a build system.** It doesn't compile or run code. It observes and analyzes.
- **Not a runtime monitor.** It can ingest OpenTelemetry traces, but it's primarily a static analysis system augmented with runtime observations.

## Where to Go Next

| Goal | Read |
|---|---|
| Install and try it | [CLI Reference](cli.md) |
| Add to your agent (Claude Code, etc) | [README Quick Start](../../README.md#quick-start) |
| Understand the architecture | [System Overview](../architecture/system-overview.md) |
| Formal definitions of every concept | [Core Concepts](../architecture/concepts.md) |
| Audit and compliance workflows | [Audit & Compliance](audit-compliance.md) |
| MCP tool reference | [MCP Tools](mcp-tools.md) |
| Hierarchical Merkle tree deep dive | [Merkle Algorithms](../architecture/merkle-algorithms.md) |
| What's planned next | [Roadmap](../../docs/roadmap.md) |
