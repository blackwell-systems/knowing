# knowing

Cross-repository knowledge graph for agentic development.

Agents today are blind at repository boundaries. LSP gives you symbol intelligence within a workspace. Tree-sitter gives you syntax within a file. Neither answers: "What breaks in other repos if I change this?"

knowing indexes symbol-level relationships across repositories and exposes them via MCP, giving agents the cross-repo awareness they need to operate safely across codebases.

## Status

Early development. Architecture being scoped.

## Design Goals

- **Two-tier indexing**: deep AST-level index for local repos, shallow SCIP/LSP ingest for external dependencies
- **Incremental**: git push triggers re-index of changed files only, not full re-walk
- **Language-aware at boundaries**: Go calling Go is easy; Go calling a Python service via HTTP needs route mapping
- **MCP-native**: exposed as MCP tools, consumed by agents directly
- **Fast**: sub-second queries over millions of symbols across thousands of repos

## What It Answers

- "I'm changing this function signature. Show me every consumer repo that calls it."
- "This proto field is deprecated. Which services still reference it?"
- "I refactored this internal API. Which downstream repos need a corresponding PR?"
- "What's the full data flow of this value across service boundaries?"

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
         v                         v                        |
+--------+-------------------------+--------+               |
|              Symbol Graph                  | <-------------+
|  (definitions, references, call sites,    |   MCP tools
|   type relationships, cross-repo edges)   |
+--------------------------------------------+
```

## Planned MCP Tools

| Tool | Purpose |
|------|---------|
| `cross_repo_callers` | All callers of a symbol across indexed repos |
| `blast_radius` | Full impact analysis for a proposed change |
| `trace_dataflow` | Follow a value across function and service boundaries |
| `dep_graph` | Dependency relationships between repos/packages |
| `stale_edges` | Edges that may be invalid due to recent changes |
| `index_repo` | Add a repo to the graph |
| `query` | Raw graph query (Cypher or similar) |

## Tech Stack

- Go (indexer, graph store, MCP server)
- tree-sitter (multi-language AST parsing)
- SCIP (ingest external indices)
- Persistent graph store (TBD: embedded or external)
- MCP over stdio/HTTP

## License

MIT
