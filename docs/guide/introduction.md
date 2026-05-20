# Introduction to knowing

This guide builds understanding from zero. No assumed background in content-addressing, Merkle trees, or graph theory. By the end you'll understand why knowing exists, how it works, and what makes it different from every other code intelligence tool.

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

When you ask "what's the blast radius of changing this function signature?", the answer isn't in any file. It's in the set of edges pointing TO that function from everywhere else.

### What existing tools provide

| Tool | What it knows | What it misses |
|---|---|---|
| **LSP (gopls, pyright)** | References within one workspace | Cross-repo callers, history, runtime behavior |
| **grep/ripgrep** | Text matches | Semantic relationships (string match != function call) |
| **Dependency graphs** | Package-level imports | Function-level callers, routes, runtime traffic |
| **APM (Datadog, etc)** | What happened in production | What the code declares, static blast radius |

None of them version the relationships. None of them can prove a relationship existed at a specific point in time. None of them learn from use.

## What a Code Graph Is

A code graph has two things:

**Nodes** are symbols: functions, methods, types, interfaces, variables, routes, database tables, queue topics, config keys.

**Edges** are relationships between symbols: calls, imports, implements, references, handles_route, publishes, subscribes, connects_to.

```
[OwnerController.createOwner] --calls--> [OwnerRepository.save]
[OwnerController.createOwner] --handles_route--> [POST /owners/new]
[OwnerRepository] --implements--> [Repository interface]
```

Every edge carries metadata:
- **Type**: what kind of relationship (calls, imports, implements)
- **Confidence**: how sure we are (0.7 = tree-sitter inferred, 0.95 = LSP resolved, 1.0 = SCIP confirmed)
- **Provenance**: who discovered it (which extractor, at which commit)

The graph represents what your codebase *understands about itself*: who calls what, who depends on what, what routes exist, what runtime traffic looks like.

## Why Content-Addressing

### The idea

In a normal database, data has an ID assigned by the system (auto-increment, UUID). The ID is arbitrary: it has no relationship to the content.

In a content-addressed system, the ID IS the content. Specifically, it's a cryptographic hash (SHA-256) of the content:

```
NodeHash = SHA-256("node\0" + repoURL + packagePath + symbolName + symbolKind)
EdgeHash = SHA-256("edge\0" + sourceHash + targetHash + edgeType + provenance)
```

The same function in the same package in the same repo always gets the same hash. Different functions always get different hashes.

### Why this matters

This single design choice gives you six properties for free:

**1. Staleness detection without scanning.**
A file changed? Its content hash changed. All nodes derived from it have potentially stale hashes. You know exactly what's stale without scanning the entire graph.

**2. Caching without invalidation logic.**
A query result computed against snapshot hash X is valid forever for X. When the graph changes, it gets a new hash Y. You don't need cache TTLs or invalidation rules. The hash IS the validity check.

**3. Integrity verification.**
Recompute the hash from the data. If it matches what's stored, the data is uncorrupted. If it doesn't, something was modified. This works recursively up the entire tree.

**4. Determinism.**
Same source code + same analyzer = same hashes. On any machine, at any time, by any operator. Two people who independently index the same repo get the same snapshot hash.

**5. Efficient equality.**
"Did the graph change?" is a single 32-byte comparison. Not a full scan. Not a diff. One comparison.

**6. Natural deduplication.**
The same symbol appearing in two contexts gets the same hash. Store it once.

### How git uses the same idea

Git is a content-addressed system. A commit hash summarizes the entire repository state. If a single byte changes in any file, the commit hash changes. You verify integrity by checking the root hash.

knowing applies the same principle to code *relationships* instead of code *text*.

## Why Hierarchical

### The limitation of flat hashing

A flat content-addressed system can tell you "something changed" (the root hash is different). But it can't tell you WHAT changed without scanning everything.

Imagine you have 100,000 edges in your graph. Something changed. Which edge? With a flat hash, you compare all 100,000 edges between the old and new state. That's O(edges).

### The hierarchical tree

knowing organizes edges into a three-level structure:

```
repo root
  package root [internal/auth]
    edge-type root [internal/auth:calls]
      edge leaves (the actual edge hashes)
    edge-type root [internal/auth:imports]
      edge leaves
  package root [internal/store]
    edge-type root [internal/store:calls]
      edge leaves
```

Each level's root is the hash of its children (a Merkle tree). The repo root summarizes everything. A package root summarizes one package. An edge-type root summarizes one category of relationships within one package.

### What this enables

**O(packages) diff instead of O(edges):**
"What changed?" Compare package roots. Only packages with different roots need investigation. For a 100,000-edge graph with 500 packages where 3 changed, that's 500 comparisons instead of 100,000.

**Semantically meaningful output:**
The diff doesn't say "edge at position 47,832 changed." It says "package `internal/auth` changed, specifically the `calls` edges." That's actionable information.

**Scoped cache invalidation:**
Your cached blast-radius result for `internal/store` is still valid even though `internal/auth` changed. The package root for `internal/store` didn't change, so anything computed from it is still correct.

**Subgraph queries:**
"Give me a single hash that represents the state of these 5 packages." That's a `SubgraphRoot` computation: hash the 5 package roots together. Use it as a cache key, a validity check, or a proof anchor.

## What knowing Does With This

### 1. Context Engine (for AI agents)

Instead of grep-read-grep-read, one call:

```bash
knowing context -task "refactor auth middleware" -format gcf
```

knowing takes the task description, extracts keywords, seeds a graph walk from matching symbols, scores by graph centrality (Random Walk with Restart + HITS), and packs the most relevant symbols into a token budget. One call replaces 6-8 grep+read cycles.

The results improve with use: feedback records which symbols were actually useful, and that signal is incorporated into future rankings.

### 2. Audit Primitive (for compliance)

```bash
knowing prove -source "PaymentService" -target "UserDB" -type calls -human
```

Generates a cryptographic Merkle proof that a relationship exists (or doesn't exist) at a specific git commit. The proof is 3KB of JSON. It verifies offline without database access. An auditor needs only the proof file and a SHA-256 implementation.

This is something no other code intelligence tool can do. Proving a negative ("service A does NOT call service B") requires sorted leaf adjacency proofs: you prove the two neighbors that bracket the gap, demonstrating there's no room for the missing edge.

### 3. Memory Layer (that learns)

Feedback from agents is content-addressed too. When you mark a symbol as "useful," the feedback record stores the Merkle root of the symbol's package at that moment. Later, if the package code changes, the root changes, and the old feedback becomes invisible (expired). No manual cleanup.

This means the system can safely accumulate feedback over months and years without old feedback poisoning current results. It gets smarter without getting noisier.

## How It Fits Together

```
Source code (files)
    |
    v
Extractors (25 languages: tree-sitter, LSP, SCIP, YAML parsers)
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
    +--> Context Engine (ranked retrieval for agents)
    +--> Proof System (inclusion + absence proofs, offline verifiable)
    +--> Diff Engine (O(packages), semantic output)
    +--> Feedback Loop (merkleized, self-expiring)
    +--> Subgraph Cache (93x speedup on repeat queries)
```

The Merkle tree is not just an integrity mechanism. It's the query optimization substrate. The same structure that proves "this edge exists" also invalidates stale caches, scopes diffs to changed packages, and expires old feedback. One data structure, five use cases.

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

### What knowing is NOT

- **Not a code search engine.** It doesn't index text. It indexes relationships.
- **Not an LSP replacement.** LSP provides single-workspace references. knowing provides cross-repo, historical, runtime-aware graph queries.
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
