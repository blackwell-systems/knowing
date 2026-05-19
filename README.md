<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-23%20tools%20%2B%208%20resources-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/extractor_types-25-blue.svg" alt="Extractor Types"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

<p align="center">
  <strong>Intelligence versioning system. Content-addressed graph where every code relationship is tracked, scored, snapshotted, and cryptographically verifiable. For AI agents: trustworthy cached context. For security and compliance: Merkle proofs, offline verification, tamper detection.</strong>
</p>

---

## What knowing Is

**Git versions files. knowing versions the understanding of code.**

knowing is an intelligence versioning system: a content-addressed graph where every relationship between symbols is tracked, scored, snapshotted, and cryptographically verifiable.

**For AI agents:** files are the wrong unit. An agent doesn't need "this file changed." It needs "this change breaks 14 callers, adds a new dependency path, and disagrees with production traffic." knowing delivers that as cached, ranked, deduplicated context.

**For security and compliance:** every snapshot is a Merkle root tied to a git commit. `knowing prove` generates a cryptographic proof that a specific relationship existed at a specific point in time. `knowing verify` checks it offline. `knowing fsck` verifies the entire graph in 98ms. No other code intelligence tool has an audit story.

Both use cases rest on the same foundation: content-addressed identity where every node, edge, and snapshot is SHA-256 hashed, and every snapshot is a hierarchical Merkle tree.

| | Git (code versioning) | knowing (intelligence versioning) |
|---|---|---|
| What it versions | File contents | Code relationships and their meaning |
| Unit of storage | file blob | node (symbol) + edge (relationship) + provenance + confidence |
| Identity model | `sha256(file content)` | `sha256("node\0" + repo + package + name + kind)` for nodes, `sha256("edge\0" + source + target + type + provenance)` for edges -- domain-type prefixes make cross-type collisions structurally impossible (same principle as git's `"blob <size>\0"` header) |
| Snapshot | tree hash of file blobs | Hierarchical Merkle root: repo -> package -> edge-type -> leaf |
| What a diff tells you | Which lines changed | Which packages changed, which edge types changed, what broke, what's new |
| What history tells you | What the code looked like | What the codebase understood about itself at each point in time |
| Incremental update | changed file = new blob | changed file = stale edges, surgical re-extraction |
| Integrity | verify tree from root hash | verify intelligence snapshot from Merkle root |

The hard problems (staleness, history, integrity, incremental updates) are structural consequences of choosing content-addressing. knowing watches for changes, detects stale files, invalidates their hashes, re-extracts only affected edges, and computes a new snapshot root. Staleness is structurally detectable and recovery is bounded to the changed files, not a full re-index.

On top of this foundation, knowing fuses static analysis, infrastructure declarations, SCIP indexes, and OpenTelemetry runtime traces into one graph. Every edge carries provenance and confidence. Feedback from past queries compounds into the intelligence: the system learns which symbols matter for which tasks, and rankings improve with use.

Use knowing when code search is too shallow, LSP is too workspace-local, dependency graphs stop at package boundaries, or you need a verifiable record of code relationships.

## What It Answers

For coding agents:

- "I am changing this function signature. Which callers, tests, routes, and repos are in the blast radius?"
- "Give me the most relevant context for this task in 5,000 tokens instead of making me grep the repo."
- "Which tests should run for these changed files?"

For platform and runtime teams:

- "Is this route actually used in production, or only declared in code?"
- "What did the service graph look like at the snapshot we deployed on Tuesday?"
- "Which runtime-observed paths disagree with static analysis?"

For security, audit, and compliance:

- "Prove that service A calls service B at this specific snapshot." (`knowing prove` generates a cryptographic Merkle proof; `knowing verify` checks it offline without database access.)
- "What relationships were added or removed between two audit points?" (Hierarchical diff, O(packages) instead of O(edges).)
- "Verify the entire graph has not been tampered with." (`knowing fsck` recomputes every hash from source data in 98ms.)
- "Show every service that touches this symbol, route, table, queue, or proto message." (Transitive callers with provenance and confidence.)
- "When did this cross-service dependency first appear?" (Walk the snapshot chain; each snapshot is a Merkle root tied to a git commit.)

## Why It Is Different

Most code-intelligence tools answer one slice of the problem:

| Tool class | What it sees | What it misses |
|---|---|---|
| LSP | Symbol references inside one workspace | Cross-repo history, runtime traffic, graph snapshots |
| Code search | Text matches | Semantic relationships and provenance |
| Dependency graphs | Package-level imports | Function-level callers, routes, infra, runtime behavior |
| APM/tracing | Production traffic | Static ownership, source-level blast radius, historical graph diffs |

knowing's unit of record is the relationship itself: `source -edge_type-> target`, with confidence and provenance. The intelligence is versioned, so you can ask "what did we understand about the service graph on Tuesday?" and get an answer, not "what did the files look like on Tuesday?"

knowing is the only code-intelligence tool where every node, edge, and snapshot is content-addressed (`sha256`). Other tools use auto-increment IDs, UUIDs, or ephemeral in-memory graphs that are regenerated from scratch each session. Content-addressing means staleness is structurally detectable (changed file = new hash = stale edges are known without scanning), snapshots are verifiable from a single Merkle root, and query results keyed to a snapshot hash are valid forever.

This also makes knowing an **audit primitive**. No other code intelligence tool can answer "prove this specific relationship existed at this specific point in time" with a cryptographic proof that verifies offline. `knowing prove` generates a Merkle proof (72us, ~3KB); `knowing verify` checks it without database access (1.2us). `knowing fsck` verifies the entire graph (98ms). The snapshot chain ties every graph state to a git commit. This is the foundation for compliance workflows, CI gates, and federated trust between teams.

Snapshots are structured as hierarchical Merkle trees: repo root -> package roots -> edge-type roots -> edge leaves. This means "which packages changed?" is an O(packages) root comparison instead of an O(edges) full scan (benchmarked at 517x faster for 100K-edge graphs). "Did call edges change?" is a single root lookup. Subgraph cache keys are computed from package roots, so queries against unchanged code return cached results instantly. Intelligence diffs ("3 new cross-service calls appeared in `internal/mcp`, 2 routes went dead in `cmd/`, blast radius of AuthMiddleware grew 40%") are scoped to the packages that actually changed.

## Proof Points

The repository includes benchmark harnesses that regenerate their own findings from the live codebase.

| Benchmark | Result | What it demonstrates |
|---|---:|---|
| Hierarchical Merkle diff | 131x faster on real graph, 517x at 100K edges | Package-level root comparison replaces full edge scan |
| Subgraph cache | 93x faster repeat queries (160ms -> 1.7ms) | Queries against unchanged code skip retrieval entirely |
| Incremental community detection | Louvain 6.9x, LP 38.4x faster (1-pkg change) | Incremental detection skips work the Merkle tree proves unchanged |
| E2E daemon community cycle | 12.6ms -> 2.5ms (5.0x with delta-save) | Full load+detect+save production path stays under 3ms |
| Context pack dedup (P5) | 93-99% byte savings (30KB -> 165 bytes) | Agents skip retransmitting unchanged context |
| Context pack persistence (P2) | Cross-session replay verified | Same task + same graph = instant context from SQLite |
| `knowing fsck` | 98ms (2,338 nodes, 11,664 edges) | Graph integrity verification in under 100ms |
| GC reachability sweep | 70ms (500 orphans pruned) | Garbage collection with full reachability sweep |
| GCF wire format | 84% fewer tokens than JSON | MCP responses carry dense graph context cheaply |
| Context retrieval | 47% fewer tool calls, 38% P@10 | One call replaces 6-8 grep+read cycles with ranked context |
| Test scope | 98% precision, 82% recall | Call-graph BFS selects affected test packages accurately |
| Feedback loop | 16% -> 36% precision after one round | Relevance improves as agents mark useful symbols |
| Edge accuracy | 27% overall confirmation rate | Two-tier extraction provides meaningful fast signal |
| Cross-repo retrieval | 46.7% R@10 on foreign codebase | Works on any Go repo with zero configuration |

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

Register and index a repository:

```bash
# Register a repo in the roster and index it.
# Each repo gets its own database at ~/.knowing/repos/<safe-name>.db
# so community detection, RWR, HITS, and BM25 operate on isolated data.
knowing add ./path/to/repo

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

Watch for file changes (re-indexes on save):

```bash
# Lightweight file watcher. Re-indexes changed files on save, runs LSP enrichment.
knowing watch ./path/to/repo
```

Run continuously (full daemon with MCP server):

```bash
# Watches git changes, re-indexes incrementally, and serves MCP over HTTP.
knowing serve -addr :8100 ./path/to/repo
```

Manage the repo roster:

```bash
# List all registered repos
knowing list

# Remove a repo from the roster
knowing remove ./path/to/repo
```

Serve MCP over stdio for local agents:

```bash
knowing mcp
```

Check graph integrity:

```bash
knowing fsck
```

## Agent Integration

Add knowing to `.mcp.json`:

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "--watch"],
      "transport": "stdio"
    }
  }
}
```

The database is auto-resolved from the roster: `defaultDB()` looks up the
current directory in the roster and returns the per-repo DB path
(`~/.knowing/repos/<safe-name>.db`). No `-db` flag is needed. Override with
`-db /path/to/db` or the `KNOWING_DB` environment variable if needed.

The `--watch` flag enables integrated file watching: the MCP server monitors the
repository for changes and re-indexes automatically, so agents always query
fresh graph data. Omit `--watch` if you manage indexing separately.

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
┌──────────────────────────────────────────────────────────────────┐
│                         knowing daemon                          │
├──────────────┬────────────────────────┬──────────────────────────┤
│   Indexer    │     Graph Store        │      MCP Server          │
│              │                        │                          │
│ 25 extractors│ Content-addressed      │ 23 tools + 8 resources   │
│ tree-sitter  │ SQLite + Merkle tree   │ stdio / HTTP             │
│ LSP + SCIP   │ Hierarchical snapshots │ GCF / GCB / JSON         │
│ OTel traces  │ Subgraph cache (93x)   │ PackRoot dedup (99%)     │
│              │ Graph notes (metadata)  │                          │
│              │ Community detection     │                          │
└──────────────┴────────────────────────┴──────────────────────────┘
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

See [docs/architecture/edge-types.md](docs/architecture/edge-types.md) for exact semantics, producers, confidence tiers, and traversal behavior.

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

### MCP Resources

8 read-only resources let agents orient to the graph without spending a tool call:

| Resource | What it returns |
|---|---|
| `knowing://report` | Graph size, top kinds, hotspot count, snapshot age |
| `knowing://schema` | Node kinds, edge types, provenance tiers, hash format |
| `knowing://stats` | Counts by repo, kind, and edge type |
| `knowing://repos` | All tracked repos with counts and last-indexed time |
| `knowing://session` | Context calls, symbols served, cache hits/misses, uptime |
| `knowing://index-health` | Healthy/stale/corrupted status, integrity check |
| `knowing://communities` | Community list with cohesion and Merkle roots |
| `knowing://community/{id}` | Single community detail (resource template) |

Full reference: [docs/guide/mcp-tools.md](docs/guide/mcp-tools.md).

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

## Content Addressing and Caching

The identity model (described in the opening section) provides three layers of caching that build on each other:

| Layer | Speed | What it does |
|---|---|---|
| **SubgraphCache** (in-memory) | 42ns lookup | Keyed by Merkle package roots. Invalidated per re-index via `DiffHierarchicalTrees`. Queries against unchanged packages return instantly. |
| **Graph notes** (SQLite) | ~1.2ms | Persists context packs and community assignments across restarts. Snapshot-validated: stale entries recomputed automatically. |
| **PackRoot dedup** (protocol) | 26 tokens | Agents pass `pack_root` from prior calls. If the context hasn't changed, knowing returns "unchanged" (165 bytes) instead of the full payload (2-30KB). |

Additional structural properties:

- **Edge events:** relationship changes are explicit (added/removed per edge per commit), not inferred from full graph scans.
- **Incremental community detection:** after re-index, only nodes in changed packages are allowed to move. Unchanged communities are frozen. 6.9x faster (Louvain) on single-package edits.
- **`knowing fsck`:** verifies edge referential integrity, hash recomputation, and snapshot chain continuity from a single Merkle root. 98ms on the live graph.

For the full storage model and hash construction, see [docs/architecture/](docs/architecture/).

## Current Boundaries

knowing is implemented, benchmarked, and usable, but it is still explicit about where precision depends on available data:

- **Breaking hash change (v0.3.0):** Hash domain prefixes (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) were added to all hash computations. Databases built before v0.3.0 must be re-indexed (`knowing add <path>` or `knowing index <path>`). Run `knowing fsck` after re-indexing to verify integrity.

- Static call-graph impact follows `calls` edges; other edge types are used for context and relationship awareness, not every blast-radius traversal.
- Runtime tools require OpenTelemetry trace ingestion and route-symbol mappings; without trace data they have no runtime observations to report.
- LSP enrichment supports Go (gopls), TypeScript (tsserver), Python (pyright), Rust (rust-analyzer), Java (jdtls), and C# (OmniSharp). Servers are auto-detected from project markers and PATH. Languages without a detected server fall back to tree-sitter extraction and SCIP where available.
- Phase 3 incremental recompute is partially shipped (community persistence, context pack persistence, incremental detection). Scoped FTS rebuild, semantic change classification, and federated graphs are planned.

See [docs/guide/features.md](docs/guide/features.md) for the implementation inventory and known gaps, and [docs/roadmap.md](docs/roadmap.md) for planned work.

## Documentation

| Doc | Contents |
|---|---|
| [Architecture](docs/architecture/) | System design, schemas, content addressing, daemon model |
| [Features](docs/guide/features.md) | Implementation inventory, entry points, limitations |
| [Audit & Compliance](docs/guide/audit-compliance.md) | Merkle proofs, fsck, snapshot chain, CI gates, compliance workflows |
| [CLI Reference](docs/guide/cli.md) | Commands, flags, examples |
| [MCP Tools](docs/guide/mcp-tools.md) | Tool schemas, parameters, return formats |
| [Edge Types](docs/architecture/edge-types.md) | Relationship semantics and provenance |
| [Context Packing](docs/architecture/context-packing.md) | RWR, HITS, ranking, token budgeting |
| [Runtime Traces](docs/operations/runtime-traces.md) | OTel ingestion and runtime confidence |
| [Wire Formats](docs/architecture/wire-formats.md) | GCF, GCB, JSON formats and benchmarks |
| [Distribution](docs/guide/distribution.md) | Release channels and package managers |
| [Roadmap](docs/roadmap.md) | Completed workstreams and next priorities |
| [Benchmarks](bench/README.md) | 13 reproducible value benchmarks with performance contracts |
| [Whitepaper](docs/research/whitepaper.md) | Hierarchical Identity Architecture thesis |
| [Hooks](hooks/README.md) | Claude Code hook integration |

## License

MIT
