<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-16-brightgreen.svg" alt="MCP Tools"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

<p align="center">
  <strong>The system of record for how software systems behave, change, and relate over time.</strong>
</p>

## Vision

Git is the system of record for source code. knowing is the system of record for what that source code *means* in the context of a running organization.

Software organizations have no single place that captures how their systems actually connect, who owns what, what changed since the last deploy, or whether production behavior matches what the code declares. That knowledge lives in people's heads, incident postmortems, and tribal memory. When someone leaves or an incident happens at 3 AM, it's gone.

knowing builds a **versioned, content-addressed ledger of software system relationships**: code, infrastructure, ownership, and runtime behavior. Every state is a hash. Every edge has provenance. Every question has an auditable answer.

Agents are the first consumer. But the actual audience is anyone who needs to understand a software organization as a system: platform teams, SREs, architects, security, compliance.

## The Problem

Agents today are blind at repository boundaries. LSP tells you where a symbol is used inside one workspace. Code search finds matching text. Dependency graphs tell you which packages depend on which.

None of them answer the questions that actually matter:

> If I change this symbol, what breaks across the rest of the system? Is this route actually called in production? When did this cross-repo dependency appear? Who do I need to notify? What did the system look like when we deployed on Tuesday?

## What knowing Does

knowing builds a boundary-aware relationship graph across repositories, services, and infrastructure. It fuses static analysis with runtime observation to create a single, trustworthy model of how a software system actually works.

It is a persistent daemon with three components:

- **Indexer**: crawls repositories in any language, parses ASTs with full type resolution (`go/packages` for Go, tree-sitter for everything else), computes content hashes, resolves cross-repo symbol references, and builds the graph. The graph model is language-agnostic; extractors produce nodes and edges, the graph doesn't care what language they came from. Watches for git changes and re-indexes incrementally.
- **Graph store**: owns the content-addressed graph in SQLite. Manages the snapshot chain, runs garbage collection, and handles traversal queries with a multi-tier cache.
- **MCP server**: exposes the graph to agents over stdio or HTTP.

Unlike tools that maintain mutable current-state graphs, knowing is **content-addressed**: every node, edge, and graph snapshot is a hash. This means:

- **History**: the graph has a full audit trail; every previous state is queryable
- **Staleness**: a hash mismatch is a structural fact, not a heuristic guess
- **Integrity**: any graph state is provably derived from specific source commits
- **Runtime ground truth**: production traces fused with static analysis tell you what actually runs, not just what the code declares

The Git analogy is exact: Git is a content-addressed graph of source code. knowing is a content-addressed graph of source code *relationships*.

## What It Answers

**For agents:**
- "I'm changing this function signature. Which other repos call it?"
- "What is the blast radius of this change, and how confident are we in each edge?"
- "What is the full data flow of this value across functions, services, queues, and repositories?"

**For platform teams and SREs:**
- "What did the dependency graph look like when we deployed on Tuesday?"
- "When did this cross-repo incompatibility first appear?"
- "Is this route actually called in production, or just declared in code?"
- "Static analysis says 47 callers; how many are active in production?"

**For architects and tech leads:**
- "This PR adds 3 new cross-repo dependencies and spans 3 teams. Here's who to notify."
- "What edges in the graph are stale after this week's changes?"
- "This proto field has zero runtime reads in 90 days. Safe to deprecate."

**For security and compliance:**
- "Prove that this graph was derived from these specific source commits."
- "Show me every service that touches PII, traced through the actual runtime call graph."
- "What changed in the system's dependency structure between these two audit dates?"

## MCP Tools

| Tool | Purpose |
|------|---------|
| `index_repo` | Add a repo to the graph |
| `graph_query` | Query nodes by qualified name prefix |
| `cross_repo_callers` | All callers of a symbol across indexed repos |
| `blast_radius` | Full impact analysis for a proposed change |
| `trace_dataflow` | Follow a value across function and service boundaries |
| `repo_graph` | Repository and package-level dependency relationships |
| `stale_edges` | Edges invalidated by recent source changes (hash mismatch) |
| `ownership` | Who owns the code/service/consumers affected by a change |
| `snapshot_diff` | What changed in the graph between two points in time |
| `semantic_diff` | Relationship-level diff between any two snapshots |
| `pr_impact` | Semantic diff specialized for a PR (resolves base/head from git) |
| `runtime_traffic` | Runtime-observed edges filtered by service and route |
| `dead_routes` | Routes with no production traffic in N days |
| `trace_stats` | Aggregate statistics on runtime-derived edges |
| `context_for_task` | Graph-ranked, token-budgeted context for a task description |
| `context_for_files` | Blast radius context for a set of changed files |

## Relationship to agent-lsp

`agent-lsp` gives agents live semantic awareness inside a workspace: diagnostics, rename execution, edit simulation, symbol navigation.

`knowing` gives agents (and humans) persistent system-level awareness across repositories: relationships, impact, ownership, staleness, and runtime behavior.

Where `agent-lsp` answers "where is this symbol used in this repo?", `knowing` answers "where is this contract used across the system, who owns the consumers, and is it actually called in production?"

## Roadmap

Five parallel workstreams, not sequential phases. See [docs/roadmap.md](docs/roadmap.md) for the full breakdown with dependency graph and parallelization notes.

| Workstream | Focus |
|------------|-------|
| **Graph Core** | Content-addressed store, language-agnostic extractor framework, Go + tree-sitter extractors, traversal cache, MCP server, daemon |
| **Edge Types** | SCIP, protobuf/gRPC, HTTP routes, events, schemas, infrastructure, ownership |
| **Runtime Intelligence** | OpenTelemetry trace ingestion, runtime symbol resolution, confidence decay |
| **Developer Visibility** | Semantic PR diff, graph-native test selection, ownership routing, staleness dashboard |
| **Agent Coordination** | Pending mutations, temporal reasoning, federated graphs |

## Quick Start

```bash
# Install
brew install blackwell-systems/tap/knowing

# Index a repository
knowing index ./path/to/repo

# Query the graph
knowing query "MyService"

# Generate context for an agent task
knowing context -task "refactor auth middleware" -budget 50000

# Start the MCP server for agent integration (stdio)
knowing mcp -db knowing.db
```

### Agent Integration (.mcp.json)

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "-db", "/path/to/knowing.db"],
      "transport": "stdio"
    }
  }
}
```

## Documentation

- [Architecture](docs/architecture.md): design decisions, system overview, schemas, interfaces
- [CLI Reference](docs/CLI.md): all commands with flags and examples
- [MCP Tools](docs/MCP-TOOLS.md): all 16 tools with parameters and return formats
- [Roadmap](docs/roadmap.md): workstreams, dependencies, parallelization notes

## License

MIT
