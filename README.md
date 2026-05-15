# knowing

Persistent, content-addressed knowledge graph for software systems, built for agents.

Agents today are blind at repository boundaries.

LSP can tell an agent where a symbol is used inside one workspace.
Tree-sitter can tell an agent what syntax exists inside one file.
Code search can find matching text across repositories.
Dependency graphs can tell you which packages depend on which packages.

None of them answer the question agents actually need before making a distributed change:

> If I change this symbol, API, route, schema, or data shape, what breaks across the rest of the system?

`knowing` builds a boundary-aware relationship graph across repositories, services, and infrastructure, then exposes that graph through MCP so agents can reason about blast radius before they edit code.

Unlike tools that maintain mutable current-state graphs, knowing is **content-addressed**: every node, edge, and graph snapshot is a hash. This means the graph has history, staleness is a hash mismatch (not a heuristic), integrity is provable, and point-in-time queries are free.

## Status

Early development. Architecture being scoped.

**v0 target:** cross-repo symbol/reference graph for Go repos, exposed through MCP.

## Core Idea

`knowing` treats repositories as parts of one larger semantic system.

It indexes local repositories deeply, ingests external dependency surfaces shallowly, and connects them with cross-boundary edges:

- Package imports and function/method calls
- Generated code references
- Protobuf/gRPC relationships
- HTTP route producers and consumers
- Event producers and consumers
- Shared schema usage (OpenAPI, JSON Schema, protobuf)
- Configuration references
- Infrastructure-defined service relationships (Terraform, K8s manifests, docker-compose)
- Ownership metadata (CODEOWNERS, team annotations)

The result is a persistent, versioned graph that agents can query before making changes.

## Content-Addressed Architecture

The graph is a Merkle DAG. Every node, edge, and snapshot is content-addressed:

- **Node hash** = `hash(repo + path + content_hash + symbol_name + kind)`
- **Edge hash** = `hash(source_hash + target_hash + edge_type + provenance)`
- **Snapshot root** = Merkle root over all edges at a point in time

This gives you:

| Property | How |
|----------|-----|
| **Point-in-time queries** | Look up any previous root hash |
| **Staleness detection** | File content hash changed → derived nodes are stale → their edges are suspect |
| **Structural diffing** | Compare two root hashes to see exactly what changed in the graph |
| **Deduplication** | Same symbol in multiple repos = same hash = stored once |
| **Integrity verification** | Prove a graph was derived from specific source commits |
| **Incremental sync** | Exchange only hash differences between machines |
| **Cache invalidation** | Query results cached by root hash; hash changes = stale |

The Git analogy is exact: Git is a content-addressed graph of source code. knowing is a content-addressed graph of source code *relationships*.

## What It Answers

- "I'm changing this function signature. Which other repos call it?"
- "This proto field is deprecated. Which services still read or write it?"
- "This HTTP route is changing. Which clients construct requests to it?"
- "This event payload field is being renamed. Which consumers depend on it?"
- "This internal package moved. Which downstream repos need a corresponding PR?"
- "What is the full data flow of this value across functions, services, queues, and repositories?"
- "Which team owns the consumers of this API?"
- "What edges in the graph are stale after this week's changes?"
- "What did the dependency graph look like when we deployed on Tuesday?"
- "When did this cross-repo edge first appear?"

## Design Goals

- **Content-addressed**: every graph state is a hash; history, staleness, and integrity are structural properties, not bolted-on features
- **Two-tier indexing**: deep AST-level index for local repos, shallow SCIP/LSP ingest for external dependencies
- **Incremental**: git push triggers re-index of changed files only; unchanged file hashes skip re-parse entirely
- **Language-aware at boundaries**: Go calling Go is straightforward; Go calling a Python service via HTTP needs route mapping
- **MCP-native**: exposed as MCP tools, consumed by agents directly
- **Fast**: optimized for interactive agent queries over large multi-repo graphs
- **Deterministic**: same input at same commit always produces the same graph (verifiable via hash)

## Architecture

```
+------------------+     +------------------+     +------------------+
|   Local Repos    |     |  External Deps   |     |   Agent (MCP)    |
|  (Tier 1: deep)  |     | (Tier 2: shallow)|     |                  |
+--------+---------+     +--------+---------+     +--------+---------+
         |                         |                        |
         v                         v                        |
+--------+---------+     +---------+--------+               |
|  AST Parser      |     |  SCIP/LSP Ingest |               |
|  (go/packages,   |     |  (public API     |               |
|   tree-sitter)   |     |   surface only)  |               |
+--------+---------+     +---------+--------+               |
         |                         |                        |
         +------------+------------+                        |
                      v                                     |
         +------------+------------+     +------------------+
         |   Content-Addressed     |     |  Non-Code Ingest |
         |      Graph Store        |<----| (Terraform, K8s, |
         |  (Merkle DAG, SQLite)   |     |  CODEOWNERS,     |
         |                         |     |  OpenAPI specs)  |
         +------------+------------+     +------------------+
                      |
                      v
         +------------+------------+
         |     Snapshot Chain      |
         |  (root hashes linked    |
         |   like git commits)     |
         +-------------------------+
```

## Planned MCP Tools

| Tool | Purpose |
|------|---------|
| `cross_repo_callers` | All callers of a symbol across indexed repos |
| `blast_radius` | Full impact analysis for a proposed change |
| `trace_dataflow` | Follow a value across function and service boundaries |
| `repo_graph` | Repository and package-level dependency relationships |
| `stale_edges` | Edges invalidated by recent source changes (hash mismatch) |
| `ownership` | Who owns the code/service/consumers affected by a change |
| `snapshot_diff` | What changed in the graph between two points in time |
| `index_repo` | Add a repo to the graph |
| `graph_query` | Raw graph query (Cypher or similar) |

## Why Content-Addressed?

Most code intelligence tools maintain a mutable "current state" graph. When you query them, you get today's answer. The old answer is gone. This means:

- No way to ask "what did the graph look like at the last deploy?"
- Staleness is heuristic (timestamps, TTLs) rather than structural
- No way to prove a graph was derived from specific source commits
- No deduplication across repos (same symbol stored N times)
- Cache invalidation requires guessing

Content-addressed storage solves all of these at the data structure level. Merkle DAGs have been proven at planetary scale for software artifacts (billions of files, hundreds of millions of commits). The same structure that makes git reliable for source history makes knowing reliable for relationship history.

The tradeoff is implementation complexity and storage cost. knowing accepts that tradeoff because agents making distributed changes need trust, not just answers.

## Why Not Just Use Code Search?

Code search finds matching text. `knowing` tracks relationships.

A call edge is not just a string match. A route consumer may be constructed through a client library. A protobuf field may flow through generated code. A service dependency may be declared in infrastructure instead of application code. A symbol may be renamed while preserving behavior, or reused in unrelated contexts.

Agents need relationship-aware answers, not grep results.

## Relationship to agent-lsp

`agent-lsp` gives agents live semantic awareness inside a workspace: diagnostics, rename execution, edit simulation, symbol navigation.

`knowing` gives agents persistent system-level awareness across repositories: relationships, impact, ownership, staleness.

Where `agent-lsp` answers "where is this symbol used in this repo?", `knowing` answers "where is this contract used across the system?"

`knowing` does not do live edits, diagnostics, file overlays, or workspace-scoped language server orchestration. Those remain agent-lsp's domain. `knowing` may ingest facts that agent-lsp produces, but it does not replicate agent-lsp's capabilities.

## Roadmap

1. Go symbols across repos (v0)
2. Go package/module dependency graph
3. SCIP ingest for external dependencies
4. Protobuf/gRPC edges
5. HTTP route edges
6. Infrastructure and ownership ingest (Terraform, K8s, CODEOWNERS)
7. Event/schema/dataflow edges

## Tech Stack

- Go (indexer, graph store, MCP server)
- tree-sitter (multi-language AST parsing)
- SCIP (ingest external indices)
- SQLite (content-addressed persistent store)
- MCP over stdio/HTTP

## License

MIT
