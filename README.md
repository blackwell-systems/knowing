<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-11-brightgreen.svg" alt="MCP Tools"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

<p align="center">
  <strong>Persistent, content-addressed knowledge graph for software systems, built for agents.</strong>
</p>

## The Problem

Agents today are blind at repository boundaries. LSP tells you where a symbol is used inside one workspace. Code search finds matching text. Dependency graphs tell you which packages depend on which.

None of them answer the question agents actually need before making a distributed change:

> If I change this symbol, API, route, schema, or data shape, what breaks across the rest of the system?

## What knowing Does

knowing builds a boundary-aware relationship graph across repositories, services, and infrastructure, then exposes that graph through MCP so agents can reason about blast radius before they edit code.

It is a persistent daemon with three components:

- **Indexer**: crawls repositories, parses ASTs with full type resolution, computes content hashes, resolves cross-repo symbol references, and builds the graph. Watches for git changes and re-indexes incrementally.
- **Graph store**: owns the content-addressed graph in SQLite. Manages the snapshot chain, runs garbage collection, and handles traversal queries with a multi-tier cache.
- **MCP server**: exposes the graph to agents over stdio or HTTP.

Unlike tools that maintain mutable current-state graphs, knowing is **content-addressed**: every node, edge, and graph snapshot is a hash. The graph has history, staleness is a hash mismatch (not a heuristic), integrity is provable, and point-in-time queries are free.

The Git analogy is exact: Git is a content-addressed graph of source code. knowing is a content-addressed graph of source code *relationships*.

## What It Answers

- "I'm changing this function signature. Which other repos call it?"
- "This proto field is deprecated. Which services still read or write it?"
- "This HTTP route is changing. Which clients construct requests to it?"
- "This event payload field is being renamed. Which consumers depend on it?"
- "Is this route actually called in production, or just declared?"
- "What is the full data flow of this value across functions, services, queues, and repositories?"
- "Which team owns the consumers of this API?"
- "What did the dependency graph look like when we deployed on Tuesday?"
- "What changed in the relationship graph since the last deploy?"

## MCP Tools

| Tool | Purpose |
|------|---------|
| `cross_repo_callers` | All callers of a symbol across indexed repos |
| `blast_radius` | Full impact analysis for a proposed change |
| `trace_dataflow` | Follow a value across function and service boundaries |
| `repo_graph` | Repository and package-level dependency relationships |
| `stale_edges` | Edges invalidated by recent source changes (hash mismatch) |
| `ownership` | Who owns the code/service/consumers affected by a change |
| `snapshot_diff` | What changed in the graph between two points in time |
| `semantic_diff` | Relationship-level diff between any two snapshots |
| `pr_impact` | Semantic diff specialized for a PR (resolves base/head from git) |
| `index_repo` | Add a repo to the graph |
| `graph_query` | Raw graph query (Cypher or similar) |

## Relationship to agent-lsp

`agent-lsp` gives agents live semantic awareness inside a workspace: diagnostics, rename execution, edit simulation, symbol navigation.

`knowing` gives agents persistent system-level awareness across repositories: relationships, impact, ownership, staleness.

Where `agent-lsp` answers "where is this symbol used in this repo?", `knowing` answers "where is this contract used across the system?"

## Roadmap

Five parallel workstreams, not sequential phases. See [docs/roadmap.md](docs/roadmap.md) for the full breakdown with dependency graph and parallelization notes.

| Workstream | Focus |
|------------|-------|
| **Graph Core** | Content-addressed store, Go cross-repo call graph, traversal cache, MCP server, daemon |
| **Edge Types** | SCIP, protobuf/gRPC, HTTP routes, events, schemas, infrastructure, ownership, multi-language |
| **Runtime Intelligence** | OpenTelemetry trace ingestion, runtime symbol resolution, confidence decay |
| **Developer Visibility** | Semantic PR diff, graph-native test selection, ownership routing, staleness dashboard |
| **Agent Coordination** | Pending mutations, temporal reasoning, federated graphs |

## Documentation

- [Architecture](docs/architecture.md): design decisions, system overview, schemas, interfaces
- [Roadmap](docs/roadmap.md): workstreams, dependencies, parallelization notes

## Tech Stack

- Go (indexer, graph store, MCP server)
- tree-sitter (multi-language AST parsing)
- SCIP (ingest external indices)
- SQLite (content-addressed persistent store)
- MCP over stdio/HTTP

## License

MIT
