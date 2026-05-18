<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-23-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/extractor_types-25-blue.svg" alt="Extractor Types"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

<p align="center">
  <strong>Content-addressed, provenance-scored graph artifact for software relationships. Designed to make AI agents retrieve, trust, diff, and reuse structural context cheaply.</strong>
</p>

---

## What knowing Is

**Git already solved the hard problems for files.** Content-addressed blobs. Merkle trees. Incremental updates. History and integrity as structural consequences of the identity model, not features bolted on after the fact. It works because identity is content: change a file, get a new hash, only the changed path needs recomputation.

knowing applies the same model to **code relationships**:

| | Git | knowing |
|---|---|---|
| Unit of storage | file blob | node (symbol) + edge (relationship between symbols) |
| Node identity | `sha256(file content)` | `sha256(repo + package + name + kind)` (logical identity) |
| Edge identity | n/a (paths are implicit) | `sha256(source_hash + target_hash + type + provenance)` |
| Snapshot | tree hash of sorted child hashes | Merkle root of sorted edge hashes |
| Incremental update | changed file = new blob hash | changed file = stale file hash, surgical re-extraction of derived nodes/edges |
| History | commit chain (append-only) | snapshot chain + edge event log (append-only) |
| Integrity | verify tree from root hash | verify snapshot from Merkle root |
| Diff | any two commits | any two snapshots |

The hard problems (staleness, history, integrity, incremental updates) are structural consequences of choosing content-addressing. knowing watches `.git/HEAD` and ref changes, detects changed files, invalidates their hashes, re-extracts only affected edges, and computes a new snapshot root. Staleness is structurally detectable and recovery is bounded to the changed files, not a full re-index.

On top of this foundation, knowing fuses static analysis, infrastructure declarations, SCIP indexes, and OpenTelemetry runtime traces into one graph. Every edge carries provenance and confidence. Agents can ask not just "where is this symbol?" but "what depends on it, how do we know, how confident are we, and what changed since the last snapshot?"

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
| Context retrieval | 47% fewer tool calls, 31.6% P@10 | One call replaces 6-8 grep+read cycles with ranked, relevant context |
| Retrieval (cross-repo) | 46.7% R@10 on foreign codebase | Works on any Go repo with zero configuration |
| GCF wire format | 84.0% fewer tokens than JSON | MCP responses can carry dense graph context cheaply |
| Test scope | 92.9% precision, 80.0% recall | Call-graph BFS selects affected test packages with few false positives |
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

# Explain why a symbol ranked where it did.
knowing why -task "refactor auth" -symbol "SessionHandler"

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
│ 25 extractors│ Content-addressed │ 23 tools + 3 prompts  │
│ tree-sitter  │ SQLite + Merkle   │ stdio / HTTP          │
│ LSP + SCIP   │ Snapshot chain    │ GCF / GCB / JSON      │
│ OTel traces  │ Edge events       │                       │
└──────────────┴───────────────────┴───────────────────────┘
```

The pipeline has two planes:

- **Execution plane:** indexes repos, extracts symbols and relationships, ingests traces, stores snapshots.
- **Intelligence plane:** computes blast radius, semantic diffs, context packs, runtime traffic, test scope, feedback, and graph communities from the stored artifact.

The artifact boundary matters: intelligence features read the graph and produce derived results, but they do not mutate source graph facts. A bad ranking can produce a bad recommendation; it cannot corrupt the graph.

## Capabilities

### Languages And Formats

| Language/Format | Extractor | Framework/Pattern Detection |
|---|---|---|
| Go | tree-sitter + `go/packages` + SCIP | net/http, gin, echo, chi, gorilla/mux, fiber |
| TypeScript/JavaScript | tree-sitter | Express.js, Fastify, Hono, NestJS, Next.js |
| Python | tree-sitter | Flask, FastAPI, Django |
| Rust | tree-sitter | Actix, Axum, Rocket |
| Java | tree-sitter | Spring annotations |
| C# | tree-sitter | ASP.NET attributes |
| Protocol Buffers | tree-sitter | service, message, enum, RPC declarations |
| Terraform (HCL) | tree-sitter | resource, data, module, variable declarations |
| SQL | tree-sitter | tables, views, functions, procedures, FK edges |
| Kubernetes YAML | yaml.v3 | deployments, services, configmaps, label-selector edges |
| CloudFormation/SAM | yaml.v3 | resources, !Ref/!GetAtt/!Sub cross-references |
| Docker Compose | yaml.v3 | services, ports, networks, depends_on links |
| GitHub Actions | yaml.v3 | workflows, jobs, steps, action references |
| Serverless Framework | yaml.v3 | functions, events, resource references |
| CSS/SCSS | tree-sitter | selectors, custom properties, var() dependencies |
| Event/MQ patterns | multi-language | Kafka, NATS, SQS, RabbitMQ publish/subscribe |
| OpenAPI/JSON Schema | json/yaml | endpoints, models, $ref resolution |
| Dockerfile | parser | FROM base images, COPY --from multi-stage deps, EXPOSE ports |
| Makefile | parser | target dependencies, include directives, variable references |
| Helm Charts | yaml.v3 | chart dependencies, template references, values injection |
| GitLab CI | yaml.v3 | job needs, extends templates, include files, artifacts |
| package.json (npm) | json | dependencies, devDependencies, peerDependencies, scripts |
| GraphQL | parser | type definitions, field type references, interface implementations |
| Ansible | yaml.v3 | playbook roles, task dependencies, variable references |

All extractors run through multi-dispatch: every matching extractor fires per file, results are merged. Tree-sitter extractors produce edges at confidence 0.7 (`ast_inferred`); `go/packages` and SCIP produce edges at 0.95-1.0 (`ast_resolved`, `scip_resolved`).

### Edge Types

knowing records static, infrastructure, and runtime relationships, including:

- `calls`, `imports`, `implements`, `references`, `handles_route`
- `depends_on`, `deploys`, `exposes`, `configures`
- `publishes`, `subscribes`, `connects_to`
- `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes`

See [docs/edge-types.md](docs/edge-types.md) for exact semantics, producers, confidence tiers, and traversal behavior.

### MCP Tools

The MCP server exposes 23 tools across indexing, graph queries, analysis, runtime, context, feedback, and discovery:

| Tool | Purpose |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | Build and inspect the graph |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | Understand impact and paths |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | Compare graph states and review changes |
| `runtime_traffic`, `dead_routes`, `trace_stats` | Query runtime-observed relationships |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | Pack graph-ranked context for agents; explain symbol rankings |
| `ownership`, `test_scope`, `communities`, `plan_turn`, `feedback` | Route work, select tests, cluster graph, improve ranking |

MCP prompts: `refactor_safely`, `review_pr`, `investigate_dead_code`.

Full reference: [docs/MCP-TOOLS.md](docs/MCP-TOOLS.md).

## Wire Formats

knowing serves responses in three encodings, selected per request:

| Format | Purpose | Savings vs JSON |
|---|---|---|
| **GCF** (Graph Compact Format) | LLM consumption: line-oriented, positional fields, local integer IDs for edge references | 84% fewer tokens |
| **GCB** (Graph Compact Binary) | Service transport and caching: varint encoding, length-prefixed strings, flat binary layout | 74% fewer bytes |
| **JSON** | Human debugging, generic API consumers | Baseline |

GCF replaces JSON's repeated keys (`qualified_name`, `provenance`, `components`) with a header line followed by `|`-separated positional fields. Edge references use local IDs (`$1 -> $3`) instead of repeating full qualified names. The result is parseable by LLMs (line-oriented, no ambiguous nesting) while fitting 5x more graph context into the same token budget.

```bash
knowing context -task "refactor auth" -format gcf   # LLM-optimized
knowing context -task "refactor auth" -format json  # human-readable
```

Session statefulness: when the same symbols appear across multiple MCP calls in a session, GCF deduplicates them (47% reduction on repeated symbols). The wire format is stateful per-session, stateless per-request.

Round-trip integrity is verified: encode -> decode -> re-encode produces identical output for all codecs.

## Content Addressing

The identity model is described in the opening section. Two additional properties worth noting:

- **Caching:** query results keyed to a snapshot hash remain valid forever for that snapshot. The hash is the cache key.
- **Edge events:** relationship changes are explicit (added/removed per edge per commit), not inferred from full graph scans.

For the full storage model and hash construction, see [docs/architecture.md](docs/architecture.md).

## Current Boundaries

knowing is implemented, benchmarked, and usable, but it is still explicit about where precision depends on available data:

- Static call-graph impact follows `calls` edges; other edge types are used for context and relationship awareness, not every blast-radius traversal.
- Runtime tools require OpenTelemetry trace ingestion and route-symbol mappings; without trace data they have no runtime observations to report.
- LSP enrichment supports Go (gopls), TypeScript (tsserver), Python (pyright), Rust (rust-analyzer), Java (jdtls), and C# (OmniSharp). Servers are auto-detected from project markers and PATH. Languages without a detected server fall back to tree-sitter extraction and SCIP where available.
- Some planned work remains: MCP resources, traversal caching, richer ownership routing, and federated graphs.

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
