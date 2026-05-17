<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-22-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/extractors-17-blue.svg" alt="Extractors"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
</p>

<p align="center">
  <strong>The system of record for how software systems behave, change, and relate over time.</strong>
</p>

---

## What knowing Is

knowing builds a **content-addressed knowledge graph** of software relationships and serves it to agents through MCP.

It fuses static analysis, infrastructure declarations, SCIP indexes, and OpenTelemetry runtime traces into one queryable graph. Every node, edge, and snapshot has a SHA-256 identity, provenance, confidence, and history, so agents can ask not just "where is this symbol?" but "what depends on it, how do we know, and what changed since the last snapshot?"

Use knowing when code search is too shallow, LSP is too workspace-local, and dependency graphs stop at package boundaries.

## What It Answers

For coding agents:

- "I am changing this function signature. Which callers, tests, routes, and repos are in the blast radius?"
- "Give me the most relevant context for this task in 5,000 tokens instead of making me grep the repo."
- "Which tests should run for these changed files?"

For platform and runtime teams:

- "Is this route actually used in production, or only declared in code?"
- "What did the service graph look like at the snapshot we deployed on Tuesday?"
- "Which runtime-observed paths disagree with static analysis?"

For security and compliance:

- "Prove this graph was derived from these source commits."
- "Show every service that touches this symbol, route, table, queue, or proto message."
- "What relationships were added or removed between two audit points?"

## Why It Is Different

Most code-intelligence tools answer one slice of the problem:

| Tool class | What it sees | What it misses |
|---|---|---|
| LSP | Symbol references inside one workspace | Cross-repo history, runtime traffic, graph snapshots |
| Code search | Text matches | Semantic relationships and provenance |
| Dependency graphs | Package-level imports | Function-level callers, routes, infra, runtime behavior |
| APM/tracing | Production traffic | Static ownership, source-level blast radius, historical graph diffs |

knowing's unit of record is the relationship itself: `source -edge_type-> target`, with confidence and provenance. The graph is versioned like source code, so relationship history is a first-class artifact instead of a regenerated report.

## Proof Points

The repository includes benchmark harnesses that regenerate their own findings from the live codebase.

| Benchmark | Result | What it demonstrates |
|---|---:|---|
| Context retrieval | 55.6% fewer tokens, 52.8% fewer tool calls | Agents spend less time exploring with grep/read loops |
| GCF wire format | 84.0% fewer tokens than JSON | MCP responses can carry dense graph context cheaply |
| Test scope | 98.9% precision, 100% recall on analyzed commits | Call-graph BFS can select affected test packages safely |
| Feedback loop | 16% -> 36% precision after one feedback round | Relevance improves as agents mark useful symbols |
| Edge accuracy | 53.6% import confirmation, 32.2% miss rate | Two-tier extraction provides meaningful fast signal |

Run the suites:

```bash
GOWORK=off go test ./bench/... -timeout 5m
```

See [bench/README.md](bench/README.md) for methodology, design principles, and caveats.

## Quick Start

Install:

```bash
brew install blackwell-systems/tap/knowing

# Or:
go install github.com/blackwell-systems/knowing/cmd/knowing@latest
npm install -g @blackwell-systems/knowing
pip install knowing
```

Index and query a repository:

```bash
# Build the graph. The default path uses fast tree-sitter extraction plus LSP enrichment.
knowing index -url github.com/org/repo ./path/to/repo

# Search symbols by qualified-name prefix.
knowing query "MyService"

# Ask for graph-ranked context for an agent task.
knowing context -task "refactor auth middleware" -format gcf

# Find affected tests for changed files.
knowing test-scope -files internal/auth/session.go,internal/auth/middleware.go

# Compare two graph snapshots.
knowing diff <old-snapshot> <new-snapshot>
```

Run continuously:

```bash
# Watches git changes, re-indexes incrementally, and serves MCP over HTTP.
knowing serve -addr :8100 ./path/to/repo
```

Serve MCP over stdio for local agents:

```bash
knowing mcp -db knowing.db
```

## Agent Integration

Add knowing to `.mcp.json`:

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

For HTTP transport:

```json
{
  "mcpServers": {
    "knowing": {
      "url": "http://localhost:8100",
      "transport": "streamable-http"
    }
  }
}
```

Claude Code hooks are included for automatic context injection on session start, edits, compaction, task stop, and subagent launch. Start with the low-risk hooks in [hooks/README.md](hooks/README.md), then enable edit/subagent hooks if the benchmark profile matches your workflow.

## How It Works

```
┌──────────────────────────────────────────────────────────┐
│                    knowing daemon                       │
├──────────────┬───────────────────┬───────────────────────┤
│   Indexer    │   Graph Store     │      MCP Server       │
│              │                   │                       │
│ 17 extractors│ Content-addressed │ 22 tools + 3 prompts  │
│ tree-sitter  │ SQLite + Merkle   │ stdio / HTTP          │
│ gopls + SCIP │ Snapshot chain    │ GCF / GCB / JSON      │
│ OTel traces  │ Edge events       │                       │
└──────────────┴───────────────────┴───────────────────────┘
```

The pipeline has two planes:

- **Execution plane:** indexes repos, extracts symbols and relationships, ingests traces, stores snapshots.
- **Intelligence plane:** computes blast radius, semantic diffs, context packs, runtime traffic, test scope, feedback, and graph communities from the stored artifact.

The artifact boundary matters: intelligence features read the graph and produce derived results, but they do not mutate source graph facts. A bad ranking can produce a bad recommendation; it cannot corrupt the graph.

## Capabilities

### Languages And Formats

| Category | Coverage |
|---|---|
| Application code | Go, TypeScript/JavaScript, Python, Rust, Java, C# |
| Infrastructure | Terraform, SQL, Kubernetes YAML, CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework |
| Interface/schema | Protocol Buffers |
| Frontend assets | CSS/SCSS |
| Web frameworks | net/http, gin, echo, chi, gorilla/mux, Express.js, Fastify, Hono, NestJS, Next.js, Flask, FastAPI, Django, Actix, Axum, Rocket, Spring, ASP.NET |

### Edge Types

knowing records static, infrastructure, and runtime relationships, including:

- `calls`, `imports`, `implements`, `references`, `handles_route`
- `depends_on`, `deploys`, `exposes`, `configures`
- `publishes`, `subscribes`, `connects_to`
- `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes`

See [docs/edge-types.md](docs/edge-types.md) for exact semantics, producers, confidence tiers, and traversal behavior.

### MCP Tools

The MCP server exposes 22 tools across indexing, graph queries, analysis, runtime, context, feedback, and discovery:

| Tool | Purpose |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | Build and inspect the graph |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | Understand impact and paths |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | Compare graph states and review changes |
| `runtime_traffic`, `dead_routes`, `trace_stats` | Query runtime-observed relationships |
| `context_for_task`, `context_for_files`, `context_for_pr` | Pack graph-ranked context for agents |
| `ownership`, `test_scope`, `communities`, `plan_turn`, `feedback` | Route work, select tests, cluster graph, improve ranking |

MCP prompts: `refactor_safely`, `review_pr`, `investigate_dead_code`.

Full reference: [docs/MCP-TOOLS.md](docs/MCP-TOOLS.md).

## Content Addressing

Every entity in knowing is identified by content:

- **Node hash:** logical symbol identity, including repo, package/path, name, and kind.
- **Edge hash:** source, target, edge type, and provenance.
- **Snapshot hash:** Merkle root of sorted edge hashes at a point in time.

This gives the graph git-like properties:

- **History:** previous graph states are queryable by snapshot hash.
- **Integrity:** a snapshot can be verified from its edge hashes.
- **Staleness:** changed file content structurally invalidates derived nodes and edges.
- **Caching:** query results keyed to a snapshot remain valid forever for that snapshot.
- **Diffs:** edge events make relationship changes explicit instead of inferred from full graph scans.

For the full storage model, see [docs/architecture.md](docs/architecture.md).

## Current Boundaries

knowing is implemented, benchmarked, and usable, but it is still explicit about where precision depends on available data:

- Static call-graph impact follows `calls` edges; other edge types are used for context and relationship awareness, not every blast-radius traversal.
- Runtime tools require OpenTelemetry trace ingestion and route-symbol mappings; without trace data they have no runtime observations to report.
- LSP enrichment currently centers on Go through `gopls`; other languages rely on tree-sitter/static extractors and SCIP where available.
- Some planned work remains: MCP resources, multi-extractor dispatch for event/schema extractors, traversal caching, richer ownership routing, and federated graphs.

See [docs/FEATURES.md](docs/FEATURES.md) for the implementation inventory and known gaps, and [docs/roadmap.md](docs/roadmap.md) for planned work.

## Documentation

| Doc | Contents |
|---|---|
| [Architecture](docs/architecture.md) | System design, schemas, content addressing, daemon model |
| [Features](docs/FEATURES.md) | Implementation inventory, entry points, limitations |
| [CLI Reference](docs/CLI.md) | Commands, flags, examples |
| [MCP Tools](docs/MCP-TOOLS.md) | Tool schemas, parameters, return formats |
| [Edge Types](docs/edge-types.md) | Relationship semantics and provenance |
| [Context Packing](docs/context-packing.md) | RWR, HITS, ranking, token budgeting |
| [Runtime Traces](docs/runtime-traces.md) | OTel ingestion and runtime confidence |
| [Wire Formats](docs/wire-formats.md) | GCF, GCB, JSON formats and benchmarks |
| [Distribution](docs/DISTRIBUTION.md) | Release channels and package managers |
| [Roadmap](docs/roadmap.md) | Completed workstreams and next priorities |
| [Benchmarks](bench/README.md) | Reproducible value benchmarks |
| [Hooks](hooks/README.md) | Claude Code hook integration |

## License

MIT
