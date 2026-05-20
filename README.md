<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-23%20tools%20%2B%208%20resources-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/extractor_types-25-blue.svg" alt="Extractor Types"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

---

Your agent grep-searches your repo 47 times per task. It reads 15 files, builds context from scratch, forgets everything next session.

**knowing gives it the right 10 symbols in one call.** Ranked by graph distance, weighted by past usefulness, deduplicated across turns. One MCP call replaces the grep-read-grep-read loop.

It gets better every time you use it. And when your code changes, stale knowledge expires automatically.

```bash
brew install blackwell-systems/tap/knowing
knowing add .
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

That's it. Your agent now has ranked code context, blast radius analysis, test scope selection, and memory that compounds.

---

## Three Things, One Architecture

knowing is three products built on one foundation (content-addressed graph with hierarchical Merkle trees):

**1. Context engine for AI agents**
One call returns the 10 most relevant symbols for a task, ranked by graph centrality, recency, and learned usefulness. 47% fewer tool calls. 84% fewer tokens. Results improve with feedback.

**2. Audit primitive for compliance**
Every graph state is a Merkle root tied to a git commit. `knowing prove` generates a cryptographic proof that a relationship existed. `knowing verify` checks it offline. `knowing fsck` verifies the entire graph in 98ms.

**3. Memory layer that learns**
Feedback from agents compounds across sessions. When code changes, feedback expires automatically (verified via package Merkle roots). The system gets smarter over time, not noisier. No other code intelligence tool has this property.

These aren't separate features. They're structural consequences of content-addressing: the same hash that makes context cacheable also makes it provable, and the same Merkle root that detects staleness also expires stale feedback.

---

## What It Answers

**For your agent:**
- "I'm changing this function. What breaks?" (blast radius across callers, tests, routes, repos)
- "Give me 5,000 tokens of context for this task." (graph-ranked, not grep-searched)
- "Which tests should run?" (call-graph traversal, 98% precision)

**For your platform team:**
- "Is this route used in production?" (static analysis + OTel runtime traces)
- "What did the service graph look like on Tuesday?" (snapshot chain, O(1) lookup)

**For your security team:**
- "Prove service A calls service B at this commit." (Merkle proof, verifiable offline)
- "Prove this dependency does NOT exist." (absence proof via sorted leaves)
- "Generate a compliance report." (`knowing audit -proofs`, one command)

---

## Numbers

| What | Result |
|---|---:|
| Agent context precision | 16% -> 50% over 5 feedback rounds |
| Tool calls saved | 47% fewer (one call replaces grep+read loops) |
| Token savings | 84% fewer tokens (GCF wire format) |
| Repeat query speed | 93x faster (Merkle-keyed subgraph cache) |
| Merkle diff | 517x faster than full edge scan at 100K edges |
| Test scope | 98% precision, 82% recall |
| Graph integrity check | 98ms (11,664 edges) |
| Proof generation | 72us generate, 1.2us verify |
| Feedback expiration | 100% expire on code change, 11% overhead |
| Cross-repo retrieval | 46.7% R@10 on foreign codebase, zero config |

All benchmarks are reproducible: `GOWORK=off go test ./bench/... -timeout 5m`

---

## Quick Start

```bash
# Install
brew install blackwell-systems/tap/knowing
# Or: go install github.com/blackwell-systems/knowing/cmd/knowing@latest
# Or: npm install -g @blackwell-systems/knowing
# Or: pip install knowing

# Index your repo
knowing add .

# Get context for a task
knowing context -task "refactor auth middleware" -format gcf

# Blast radius of a change
knowing blast-radius -symbol "SessionHandler"

# Affected tests
knowing test-scope -files internal/auth/session.go

# Prove a relationship exists (cryptographic Merkle proof)
knowing prove -source "AuthService" -target "SessionStore"

# Verify offline (no database needed)
knowing verify proof.json

# Check graph integrity
knowing fsck
```

### MCP Integration

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

The `--watch` flag re-indexes on file changes. Your agent always queries fresh data. The database auto-resolves from the repo roster; no path configuration needed.

For HTTP transport (multi-agent, daemon mode):

```bash
knowing serve -addr :8100 .
```

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

---

## Why This Works

**Git versions files. knowing versions the understanding of code.**

The entire system is built on one idea: content-addressed identity. Every symbol, relationship, and snapshot is SHA-256 hashed. This single choice gives you:

- **Staleness detection for free.** Changed file = new hash = stale edges are known without scanning.
- **Caching for free.** Same package root = same results. 93x speedup on unchanged queries.
- **Integrity for free.** Verify the entire graph from one Merkle root. 98ms.
- **History for free.** Each snapshot is a Merkle root tied to a git commit. Walk the chain.
- **Feedback expiration for free.** Feedback stores the package Merkle root. Code changes = root changes = old feedback is invisible.
- **Proofs for free.** Merkle path from leaf to root is a self-contained cryptographic proof.

| | Git | knowing |
|---|---|---|
| What it versions | File contents | Code relationships and their meaning |
| Unit of storage | blob | node + edge + provenance + confidence |
| Identity | `sha256(content)` | `sha256("node\0" + repo + package + name + kind)` |
| Snapshot | tree of blobs | Hierarchical Merkle: repo -> package -> edge-type -> leaf |
| Diff | Which lines changed | Which packages changed, what broke, what's new |
| History | What code looked like | What the codebase understood about itself |

---

## How It Works

```
+------------------------------------------------------------------+
|                         knowing daemon                            |
+----------------+------------------------+--------------------------+
|   Indexer      |     Graph Store        |      MCP Server          |
|                |                        |                          |
| 25 extractors  | Content-addressed      | 23 tools + 8 resources   |
| tree-sitter    | SQLite + Merkle tree   | stdio / HTTP             |
| LSP + SCIP     | Hierarchical snapshots | GCF / GCB / JSON         |
| OTel traces    | Subgraph cache (93x)   | PackRoot dedup (99%)     |
|                | Community detection    |                          |
+----------------+------------------------+--------------------------+
```

Two planes:
- **Execution:** indexes repos, extracts symbols and relationships, ingests traces, stores snapshots.
- **Intelligence:** computes blast radius, context packs, test scope, feedback, communities from the stored graph.

The boundary matters: intelligence features read the graph and produce derived results. They cannot corrupt graph facts. A bad ranking produces a bad recommendation; it cannot invalidate a proof.

---

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

All extractors fire per file via multi-dispatch; results are merged. Tree-sitter produces edges at confidence 0.7 (`ast_inferred`); `go/packages` and SCIP at 0.95-1.0 (`ast_resolved`, `scip_resolved`).

### MCP Tools

| Tool | Purpose |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | Build and inspect the graph |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | Understand impact and paths |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | Compare graph states and review changes |
| `runtime_traffic`, `dead_routes`, `trace_stats` | Query runtime-observed relationships |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | Ranked context for agents |
| `ownership`, `test_scope`, `communities`, `plan_turn`, `feedback` | Route work, select tests, improve ranking |

MCP prompts: `refactor_safely`, `review_pr`, `investigate_dead_code`.

### MCP Resources

8 read-only resources for agent orientation without a tool call:

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

---

## Wire Formats

| Format | Purpose | Savings vs JSON |
|---|---|---|
| **GCF** (Graph Compact Format) | LLM consumption: line-oriented, positional fields | 84% fewer tokens |
| **GCB** (Graph Compact Binary) | Service transport and caching: varint, length-prefixed | 74% fewer bytes |
| **JSON** | Human debugging, generic consumers | Baseline |

GCF uses `|`-separated fields and local IDs (`$1 -> $3`) instead of repeated qualified names. Parseable by LLMs while fitting 5x more graph context into the same token budget. Session-stateful deduplication reduces repeated symbols by 47%.

---

## Current Boundaries

- **Breaking hash change (v0.3.0):** Hash domain prefixes added. Databases from before v0.3.0 must be re-indexed. Run `knowing fsck` after.
- Static blast radius follows `calls` edges; other edge types provide context, not traversal.
- Runtime tools require OpenTelemetry trace ingestion; without traces they have no observations.
- LSP enrichment: Go, TypeScript, Python, Rust, Java, C#. Auto-detected from project markers. Others fall back to tree-sitter.

---

## Documentation

| Doc | Contents |
|---|---|
| [Architecture](docs/architecture/) | System design, schemas, content addressing, daemon model |
| [Features](docs/guide/features.md) | Implementation inventory, entry points, limitations |
| [Audit & Compliance](docs/guide/audit-compliance.md) | Merkle proofs, fsck, snapshot chain, CI gates |
| [CLI Reference](docs/guide/cli.md) | Commands, flags, examples |
| [MCP Tools](docs/guide/mcp-tools.md) | Tool schemas, parameters, return formats |
| [Edge Types](docs/architecture/edge-types.md) | Relationship semantics and provenance |
| [Context Packing](docs/architecture/context-packing.md) | RWR, HITS, ranking, token budgeting |
| [Runtime Traces](docs/operations/runtime-traces.md) | OTel ingestion and runtime confidence |
| [Wire Formats](docs/architecture/wire-formats.md) | GCF, GCB, JSON formats and benchmarks |
| [Roadmap](docs/roadmap.md) | Completed workstreams and next priorities |
| [Benchmarks](bench/README.md) | Reproducible value benchmarks with performance contracts |
| [Whitepaper](docs/research/whitepaper.md) | Hierarchical Identity Architecture thesis |
| [Hooks](hooks/README.md) | Claude Code hook integration |

## License

MIT
