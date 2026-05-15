# knowing

Persistent knowledge graph for software systems, built for agents.

Agents today are blind at repository boundaries.

LSP can tell an agent where a symbol is used inside one workspace.
Tree-sitter can tell an agent what syntax exists inside one file.
Code search can find matching text across repositories.
Dependency graphs can tell you which packages depend on which packages.

None of them answer the question agents actually need before making a distributed change:

> If I change this symbol, API, route, schema, or data shape, what breaks across the rest of the system?

`knowing` builds a boundary-aware relationship graph across repositories, services, and infrastructure, then exposes that graph through MCP so agents can reason about blast radius before they edit code.

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

The result is a graph that agents can query before making changes.

### Edge Freshness

Edges have confidence. When source code changes, `knowing` tracks which relationships may be stale. An edge derived from a function call that was deleted yesterday is not the same as one confirmed by today's index run. Agents receive freshness metadata with query results so they can decide whether to trust an edge or re-verify.

## What It Answers

- "I'm changing this function signature. Which other repos call it?"
- "This proto field is deprecated. Which services still read or write it?"
- "This HTTP route is changing. Which clients construct requests to it?"
- "This event payload field is being renamed. Which consumers depend on it?"
- "This internal package moved. Which downstream repos need a corresponding PR?"
- "What is the full data flow of this value across functions, services, queues, and repositories?"
- "Which team owns the consumers of this API?"
- "Which edges in the graph are stale after this week's changes?"

## Design Goals

- **Two-tier indexing**: deep AST-level index for local repos, shallow SCIP/LSP ingest for external dependencies
- **Incremental**: git push triggers re-index of changed files only, not full re-walk
- **Language-aware at boundaries**: Go calling Go is straightforward; Go calling a Python service via HTTP needs route mapping
- **MCP-native**: exposed as MCP tools, consumed by agents directly
- **Fast**: optimized for interactive agent queries over large multi-repo graphs
- **Staleness-aware**: every edge carries freshness metadata; stale edges are surfaced, not silently trusted

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
         |       Symbol Graph      |     |  Non-Code Ingest |
         |  (definitions, refs,    |<----| (Terraform, K8s, |
         |   call sites, types,    |     |  CODEOWNERS,     |
         |   cross-repo edges)     |     |  OpenAPI specs)  |
         +------------+------------+     +------------------+
                      |
                      v
         +------------+------------+
         |    Freshness Tracker    |
         |  (git events, CI hooks, |
         |   confidence scoring)   |
         +-------------------------+
```

## Planned MCP Tools

| Tool | Purpose |
|------|---------|
| `cross_repo_callers` | All callers of a symbol across indexed repos |
| `blast_radius` | Full impact analysis for a proposed change |
| `trace_dataflow` | Follow a value across function and service boundaries |
| `repo_graph` | Repository and package-level dependency relationships |
| `stale_edges` | Edges that may be invalid due to recent changes |
| `ownership` | Who owns the code/service/consumers affected by a change |
| `index_repo` | Add a repo to the graph |
| `graph_query` | Raw graph query (Cypher or similar) |

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
- Persistent graph store (TBD: embedded or external)
- MCP over stdio/HTTP

## License

MIT
