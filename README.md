<p align="center">
  <img src="assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="https://zenodo.org/records/20342255"><img src="https://zenodo.org/badge/DOI/10.5281/zenodo.20342255.svg" alt="DOI"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-28%20tools%20%2B%208%20resources-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/languages_and_formats-26-blue.svg" alt="Languages and Formats"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
</p>

---

<p align="center">
Self-adapting code intelligence engine. Observes its own graph density and adjusts retrieval strategy automatically. 38 edge types, 28 MCP tools, local embedding re-ranker, cryptographic proofs. Gets smarter with scale, not dumber.
</p>

---

> [!NOTE]
> **Research paper:** [Content-Addressing as a Computation Primitive for Software Relationship Intelligence](https://zenodo.org/records/20342255) (DOI: 10.5281/zenodo.20342255)

Your architecture diagram says service A calls service B. Can you prove it?

**knowing can.** It builds a content-addressed graph of extracted code relationships, snapshots it as a Merkle tree tied to a git commit, and generates cryptographic proofs that verify offline. Agents use it for ranked context. Security teams use it for audit. Platform teams use it to compare code against production traces.

It gets better every time you use it. When code changes, stale knowledge expires automatically.

```bash
brew install blackwell-systems/tap/knowing
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

That's it. The MCP server auto-indexes your repo on first launch and downloads a 30MB embedding model once. Your agent now has ranked context, blast radius, test scope, and memory that compounds.

**Verify it works:** Ask your agent: *"Use the context_for_task tool to find symbols related to [something you know exists in your code]."* You should see ranked symbols from your codebase. If results are empty, the repo is still indexing (10-30 seconds on first launch). If results seem unrelated, see [Troubleshooting](docs/guide/cli.md#troubleshooting).

> **Not using an AI agent?** Skip to [CLI usage](#path-b-cli-usage-explore-the-graph-yourself) below.

| You want to... | Start here |
|---|---|
| Give your AI agent graph-ranked context | [MCP setup](#mcp-integration) |
| Explore the graph from the CLI | [CLI usage](#path-b-cli-usage-explore-the-graph-yourself) |
| Understand how retrieval works | [Introduction](docs/guide/introduction.md) |
| Audit with cryptographic proofs | [Audit & Compliance](docs/guide/audit-compliance.md) |

---

## Three Things, One Architecture

knowing is three products built on one foundation (content-addressed graph with hierarchical Merkle trees):

**1. Context engine for AI agents**
One call returns the most relevant symbols for a task, ranked by graph centrality, recency, and learned usefulness, packed to fit your token budget. An optional local embedding re-ranker (+17% precision) reorders graph-surfaced candidates by semantic similarity, using pure Go inference with no API calls. 47% fewer tool calls. 84% fewer tokens. Results improve with feedback.

**2. Audit primitive for compliance**
Every graph state is a Merkle root tied to a git commit. `knowing prove` generates a cryptographic proof that a relationship existed. `knowing verify` checks it offline. `knowing fsck` verifies the entire graph in 98ms. Supply chain detection extracts credential access, process spawning, and network exfiltration edges to flag structurally suspicious code.

**3. Memory layer that learns**
Feedback from agents compounds across sessions. When code changes, feedback expires automatically (verified via package Merkle roots). The system gets smarter over time, not noisier. That is the property knowing is built around.

These aren't separate features. They're structural consequences of content-addressing: the same hash that makes context cacheable also makes it provable, and the same Merkle root that detects staleness also expires stale feedback.

---

## What It Answers

**For your agent:**
- "I'm changing this function. What breaks?" (blast radius across callers, tests, routes, repos)
- "Give me 50,000 tokens of context for this task." (graph-ranked, not grep-searched)
- "Which tests should run?" (call-graph traversal, 98% precision)

**For your platform team:**
- "Is this route used in production?" (static analysis + OTel runtime traces)
- "What did the service graph look like at a specific snapshot?" (snapshot chain, each root tied to a git commit)

**For your security team:**
- "Prove service A calls service B at this commit." (Merkle proof, verifiable offline)
- "Prove this dependency does NOT exist." (absence proof via sorted leaves)
- "Generate a compliance report." (`knowing audit -proofs`, one command)
- "Does this package read credentials and spawn processes?" (`knowing audit-supply-chain --scan-all`)

---

## Numbers

| What | Result |
|---|---:|
| Cross-system retrieval | **P@10=0.257 cold, 0.262 warm** (237 tasks, 12 repos, 7 languages) |
| vs competitors | 1.90x codegraph (19K stars), 1.88x codebase-memory (2.7K stars), 3.43x GitNexus, 4.08x Gortex, 19.8x grep |
| Embedding re-ranker | +17% P@10 (local inference, no API, no charges, on by default) |
| Gap-fill seeds | +11% P@10 (embedding-based fallback when keywords fail) |
| Equivalence classes | 152 concepts bridging task vocabulary to code symbols |
| Re-rank latency | 220ms cached (vector cache in SQLite) |
| Agent context precision | +20pp after 1 round, +34pp after 5 |
| Tool calls saved | 47% fewer (one context call replaces repeated grep+read) |
| Token savings | 84% fewer tokens (GCF wire format) |
| Repeat query speed | 93x faster (Merkle-keyed subgraph cache) |
| Merkle diff | 517x faster than full edge scan at 100K edges |
| Test scope | 98% precision, 82% recall |
| Graph integrity check | 98ms (24,936 edges) |
| Proof generation | 72us generate, 1.2us verify |
| Feedback expiration | 100% expire on code change, 11% overhead |
| Indexing throughput | 12 repos (7 languages) in ~52s |
| Language coverage | 12/12 repos pass (Go, Python, TS, Rust, Java, C#, multi) |
| Edge types | 38 (including supply chain: reads_env, executes_process) |

All benchmarks are reproducible: `GOWORK=off go test ./bench/... -timeout 5m`

---

## Quick Start

### Path A: MCP server (recommended for AI agents)

```bash
# 1. Install
brew install blackwell-systems/tap/knowing
# Or: npm install -g @blackwell-systems/knowing
# Or: pip install knowing
# Or: go install github.com/blackwell-systems/knowing/cmd/knowing@latest

# 2. Add to your agent config (.mcp.json, Claude Code settings, etc.)
#    See "MCP Integration" below for the config block.
#    The server auto-indexes your repo on first launch. Done.
```

### Path B: CLI usage (explore the graph yourself)

```bash
# 1. Install (same as above)
brew install blackwell-systems/tap/knowing

# 2. Index your repo
knowing add .

# 3. Verify the index worked
knowing stats
# You should see node and edge counts. A healthy TypeScript repo with 50K LOC
# typically produces 2K-10K nodes and 5K-30K edges. If you see very few edges,
# the extractors may not have found your code (check language support below).

# 4. Get context for a task
knowing context -task "refactor auth middleware" -format gcf

# 5. Check graph integrity
knowing fsck
```

### Verify your setup

After indexing, run these commands to confirm everything is working:

```bash
# Show node/edge counts, repos, snapshots
knowing stats

# Search for a symbol you know exists in your code
knowing query "MyKnownFunction"

# Check graph integrity (should report 0 errors)
knowing fsck

# If results seem wrong, check if the graph is stale
knowing stale
```

If `knowing stats` shows zero nodes or very few edges, see
[Troubleshooting](docs/guide/cli.md#troubleshooting) below.

### More CLI commands

```bash
# Find affected tests
knowing test-scope -files internal/auth/middleware.go

# Explain why a symbol ranked where it did
knowing why -task "refactor auth" -symbol "SessionHandler"

# Prove a relationship exists (cryptographic Merkle proof)
knowing prove -source "AuthService" -target "SessionStore"

# Verify offline (no database needed)
knowing verify proof.json

# Check if the graph is stale (CI gate: exits 1 if stale)
knowing stale

# Supply chain audit (scan all files for suspicious patterns)
knowing audit-supply-chain --scan-all

# Remove a repo (evicts all data: nodes, edges, snapshots, feedback)
knowing remove ./path/to/repo
```

For the full command reference, see [CLI Reference](docs/guide/cli.md).

### MCP Integration

**For Claude Code** (`.mcp.json` in your project root or `~/.claude/mcp.json` globally):

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

**For Cursor** (`.cursor/mcp.json`):

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

The `--watch` flag re-indexes on file changes. Your agent always queries fresh data. No manual `knowing index` or database path needed: the MCP server auto-indexes the git repository on first launch and registers it in the roster for future sessions.

The embedding re-ranker is on by default (+17% precision, fully offline, no API keys, no charges). The model auto-downloads on first use (~30MB). To disable it, add `--no-embeddings`.

**What your agent gets:** The key tool is `context_for_task`. When your agent calls it with a task description, knowing returns ranked, relevant code symbols packed into a token budget. This replaces grep-read loops. Other useful tools: `blast_radius` (what breaks if I change this?), `test_scope` (which tests to run?), `explain_symbol` (why did this rank here?). See [MCP Tools Reference](docs/guide/mcp-tools.md) for all 28 tools.

**Verify it works:**

1. Start a session with your agent
2. Ask: *"Use the context_for_task tool to find symbols related to [something specific in your code]"*
3. You should see ranked symbols with scores and file paths from your codebase

If results are empty: the repo may still be indexing (10-30 seconds on first launch). If results seem unrelated: use specific symbol names in your task description (e.g., "find the `AuthMiddleware` handler" not "find auth code"). You can also verify from the CLI:

```bash
knowing stats          # should show nodes and edges
knowing query "MyFunc" # should find symbols you recognize
```

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
- **Integrity for free.** Verify all stored hashes and snapshot chain continuity. 98ms.
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
| 23 extractors  | Content-addressed      | 28 tools + 8 resources   |
| tree-sitter    | SQLite + Merkle tree   | stdio / HTTP (1.8s index)|
| LSP + SCIP     | 38 edge types          | GCF / GCB / JSON         |
| OTel traces    | Subgraph cache (93x)   | PackRoot dedup (99%)     |
|                | Embedding vector cache | Embedding re-ranker      |
|                | Community detection    | Supply chain audit       |
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
| Go | tree-sitter + `go/packages` + SCIP | net/http, gin, echo, chi, gorilla/mux |
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
| Ruby | tree-sitter | classes, modules, method definitions, require edges |
| .env files | parser | environment variable declarations, cross-file references |

All extractors fire per file via multi-dispatch; results are merged. Tree-sitter produces edges at confidence 0.7 (`ast_inferred`); `go/packages` and SCIP at 0.95-1.0 (`ast_resolved`, `scip_resolved`).

### MCP Tools

| Tool | Purpose |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | Build and inspect the graph |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | Understand impact and paths |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | Compare graph states and review changes |
| `runtime_traffic`, `dead_routes`, `trace_stats` | Query runtime-observed relationships |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | Ranked context for agents |
| `ownership`, `ownership_query`, `test_scope`, `communities`, `plan_turn`, `feedback` | Route work, query code owners/authors, select tests, improve ranking |
| `prove`, `prove_absent`, `fsck` | Cryptographic proofs, absence proofs, integrity verification |
| `untrack_repo` | Evict all data for a repository (nodes, edges, files, snapshots, feedback, task memory, graph notes) |

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
- Embedding re-ranker is on by default. Adds ~220ms to query time (cached vectors). Disable with `--no-embeddings`. Downloads a 30MB model on first use.

---

## Documentation

| Doc | Contents |
|---|---|
| [Introduction](docs/guide/introduction.md) | How it works, retrieval pipeline explained, 5-minute walkthrough |
| [Architecture](docs/architecture/) | System design, schemas, content addressing, daemon model |
| [Features](docs/guide/features.md) | Implementation inventory, entry points, limitations |
| [Audit & Compliance](docs/guide/audit-compliance.md) | Merkle proofs, fsck, snapshot chain, CI gates |
| [CLI Reference](docs/guide/cli.md) | Commands, flags, examples, [troubleshooting](docs/guide/cli.md#troubleshooting) |
| [MCP Tools](docs/guide/mcp-tools.md) | Tool schemas, parameters, return formats |
| [Edge Types](docs/architecture/edge-types.md) | Relationship semantics and provenance |
| [Context Packing](docs/architecture/context-packing.md) | RWR, HITS, ranking, token budgeting |
| [Embedding Re-ranker](docs/architecture/embedding-reranker.md) | Local inference, vector cache, latency profile |
| [Runtime Traces](docs/operations/runtime-traces.md) | OTel ingestion and runtime confidence |
| [Wire Formats](docs/architecture/wire-formats.md) | GCF, GCB, JSON formats and benchmarks |
| [Roadmap](docs/roadmap.md) | Completed workstreams and next priorities |
| [Benchmarks](bench/README.md) | Reproducible value benchmarks with performance contracts |
| [Whitepaper](docs/research/content-addressing-as-computation-primitive.md) | Hierarchical Identity Architecture thesis ([DOI: 10.5281/zenodo.20342255](https://zenodo.org/records/20342255)) |
| [Hooks](hooks/README.md) | Claude Code hook integration |

## License

MIT
