<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-16-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages"><img src="https://img.shields.io/badge/languages-10-blue.svg" alt="Languages"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

<p align="center">
  <strong>The system of record for how software systems behave, change, and relate over time.</strong>
</p>

---

## What This Is

knowing is a **content-addressed knowledge graph** that fuses static analysis, infrastructure declarations, and runtime traces into a single queryable structure. It indexes code in 10 languages, watches for git changes, ingests OpenTelemetry traces, and serves the result over MCP.

Every node, edge, and snapshot is a SHA-256 hash. The graph has full history, provable integrity, and a clear answer to "when did this relationship appear and how confident are we in it?"

## Why It Exists

Software organizations have no single place that captures how their systems actually connect. That knowledge lives in people's heads, incident postmortems, and tribal memory. Existing tools operate at the wrong granularity:

- LSP tells you where a symbol is used inside one workspace
- Code search finds matching text
- Dependency graphs tell you which packages depend on which

None of them answer: *if I change this symbol, what breaks across the rest of the system? Is this route actually called in production? What did the graph look like when we deployed on Tuesday?*

knowing answers those questions with provenance and confidence scores on every edge.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     knowing daemon                         │
├──────────────┬───────────────────┬───────────────────────┤
│   Indexer    │   Graph Store     │     MCP Server        │
│              │                   │                       │
│ 10 languages │ Content-addressed │ 16 tools + 3 prompts │
│ tree-sitter  │ SQLite + Merkle   │ stdio / HTTP          │
│ go/packages  │ Snapshot chain    │ KWF wire format       │
│ OTel ingest  │ Edge events       │                       │
└──────────────┴───────────────────┴───────────────────────┘
```

Three components, one binary:

- **Indexer**: parses ASTs across 10 languages, resolves cross-repo references, ingests OTel traces, watches git for incremental re-indexing
- **Graph store**: content-addressed SQLite with Merkle snapshot chain, edge event sourcing, runtime confidence decay
- **MCP server**: 16 tools + 3 prompts over stdio/HTTP, with KWF wire format (76% token savings vs JSON)

## Languages

| Language | Extractor | Framework Detection |
|----------|-----------|-------------------|
| Go | tree-sitter + `go/packages` | net/http, gin, echo, chi, fiber |
| TypeScript/JS | tree-sitter | Express.js, Fastify, Hono, NestJS, Next.js |
| Python | tree-sitter | Flask, FastAPI, Django (urls.py) |
| Rust | tree-sitter | Actix, Axum, Rocket |
| Java | tree-sitter | Spring |
| C# | tree-sitter | ASP.NET |
| Terraform (HCL) | tree-sitter | resource/module/variable declarations |
| SQL | tree-sitter | tables, views, procedures |
| Kubernetes YAML | yaml.v3 | deployments, services, configmaps, ingress |
| CSS/SCSS | tree-sitter | selectors, custom properties, imports |

## MCP Tools

| Tool | Purpose |
|------|---------|
| `index_repo` | Add a repo to the graph |
| `graph_query` | Query nodes by qualified name prefix |
| `cross_repo_callers` | All callers of a symbol across indexed repos |
| `blast_radius` | Full impact analysis for a proposed change |
| `trace_dataflow` | Follow a value across function and service boundaries |
| `repo_graph` | Repository and package-level dependency relationships |
| `stale_edges` | Edges invalidated by recent source changes |
| `ownership` | Who owns the code/service/consumers affected by a change |
| `snapshot_diff` | What changed in the graph between two points in time |
| `semantic_diff` | Relationship-level diff between any two snapshots |
| `pr_impact` | Semantic diff specialized for a PR |
| `runtime_traffic` | Runtime-observed edges filtered by service and route |
| `dead_routes` | Routes with no production traffic in N days |
| `trace_stats` | Aggregate statistics on runtime-derived edges |
| `context_for_task` | Graph-ranked, token-budgeted context for a task |
| `context_for_files` | Blast radius context for changed files |

**MCP Prompts:** `refactor_safely`, `review_pr`, `investigate_dead_code`

## Wire Formats

knowing serves responses in three encodings, selected per request:

| Format | Use Case | Savings vs JSON |
|--------|----------|-----------------|
| **KWF** (Knowing Wire Format) | LLM consumption | 76.7% fewer tokens |
| **KWB** (Knowing Wire Binary) | Service transport, caching | 74% fewer bytes |
| **JSON** | Human debugging, generic APIs | Baseline |

```bash
knowing context -task "refactor auth" -format kwf
```

## Quick Start

```bash
# Install
brew install blackwell-systems/tap/knowing

# Index a repository
knowing index ./path/to/repo

# Query the graph
knowing query "MyService"

# Generate context for an agent task
knowing context -task "refactor auth middleware" -format kwf

# Start the MCP server (stdio)
knowing mcp -db knowing.db

# Start the daemon (watches git, serves MCP over HTTP)
knowing serve -repo ./path/to/repo -addr :8100
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

## What It Answers

**For agents:**
- "I'm changing this function signature. Which other repos call it?"
- "What is the blast radius of this change, and how confident are we in each edge?"
- "Give me the most relevant context for this task, packed into 5000 tokens."

**For platform teams:**
- "What did the dependency graph look like when we deployed on Tuesday?"
- "Is this route actually called in production, or just declared in code?"
- "Static analysis says 47 callers; how many are active in production?"

**For security and compliance:**
- "Prove that this graph was derived from these specific source commits."
- "Show me every service that touches PII, traced through the runtime call graph."
- "What changed in the system's dependency structure between these two audit dates?"

## Content Addressing

The Git analogy is exact: Git is a content-addressed graph of source code. knowing is a content-addressed graph of source code *relationships*.

- **History**: every previous graph state is queryable by snapshot hash
- **Staleness**: a hash mismatch between snapshots is a structural fact, not a heuristic
- **Integrity**: any graph state is provably derived from specific source commits
- **Deduplication**: identical relationships across repos share a single edge record

## Documentation

| Doc | Contents |
|-----|----------|
| [Architecture](docs/architecture.md) | System design, schemas, content addressing, interfaces |
| [Wire Formats](docs/wire-formats.md) | KWF/KWB specs, grammar, benchmarks, codec registry |
| [CLI Reference](docs/CLI.md) | All commands with flags and examples |
| [MCP Tools](docs/MCP-TOOLS.md) | All 16 tools with parameters and return formats |
| [Edge Types](docs/edge-types.md) | The 9 relationship types and their semantics |
| [Context Packing](docs/context-packing.md) | RWR algorithm, scoring, token budgeting |
| [Runtime Traces](docs/runtime-traces.md) | OTel ingestion, confidence scoring, decay |
| [Roadmap](docs/roadmap.md) | Workstreams, priorities, next steps |

## License

MIT
