# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **~~`knowing why <symbol>`~~** | **Shipped.** Explains why a symbol ranked where it did: seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback weight, session boost, equivalence class matches. See [CLI reference](CLI.md#why). | Done |
| 3 | **Session memory persistence** | SessionTracker is ephemeral (lost on session end), task memory is coarse (keyword-level, 7-day decay). Persist session working sets to SQLite so resumed sessions pick up where they left off and cross-session patterns compound. Extends `internal/context/session.go` with a `session_events` table. | Medium |
| 4 | **~~Negative feedback~~** | **Already implemented.** `feedback` tool accepts `useful: false`, store computes useful/total ratio, ranking formula maps to [-0.15, +0.15] penalty/boost. Updated tool description to make negative feedback explicit. | Done |
| 5 | **Traversal cache** | L1 in-memory LRU for hot paths. Repeat queries should be instant. | Medium |
| 6 | **`knowing stats`** | Show session value: context calls, symbols served, symbols marked relevant, feedback rate. Lets users see the value accumulating. | Low |
| 7 | **MCP resources** | Lightweight context that doesn't cost a tool call. Resources are read directly by the MCP host for agent orientation. See detailed list below. | Medium |
| 8 | **~~v0.2.0 release~~** | **Shipped.** 25 extractors, retrieval pipeline, TOON, `knowing init`, multi-language LSP enrichment, `knowing why`, 84 equivalence classes, 23 MCP tools, ~60K LOC. | Done |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| ~~`knowing watch`~~ | **Shipped.** Filesystem watcher (fsnotify) that re-indexes changed files on save with debounced batching and optional background LSP enrichment. | Done |
| ~~`knowing mcp --watch`~~ | **Shipped.** `knowing mcp --watch` combines MCP server + fsnotify watcher in one process. Also supports `--repo`, `--url`, `--no-enrich`, `--debounce`. | Done |
| ~~`knowing enrich blame`~~ | **Shipped.** Stamps last-author + last-commit-at on every symbol via `git blame --porcelain`. Migration 009 adds `last_author` and `last_commit_at` columns. | Done |
| `knowing enrich coverage` | Stamp coverage percentage on symbols from Go cover profiles (or lcov for other languages). Lets `test_scope` say "this function has 12% coverage" alongside "these tests cover it." | P1 |
| `knowing stats` | Show cumulative session value: context calls, symbols served, symbols marked relevant, feedback rate, token savings. Proves the value is accumulating. | P2 |
| Staleness reporting | Content-addressing makes staleness structurally detectable, but no command surfaces it. `knowing stale` should report "these N edges are stale because these files changed since the last snapshot." Free win from the architecture. | P2 |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. Currently `knowing serve` blocks the terminal. Detached mode with PID file tracking for production use. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux) for always-on graph daemon. | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. Currently handled by CLI flags and `.gitignore` only. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges up/down/both from a type. Returns the full inheritance tree. Edges already exist in the graph; this is a traversal convenience wrapper. | P3 |
| `neighborhood` MCP tool | Seed-based dense neighborhood: "give me the N symbols most densely connected to X within radius R." Different from global Louvain communities. Wraps the existing RWR computation as a standalone tool. | P3 |
| GraphML/Cypher export | `knowing export -format graphml\|cypher` for loading the graph into Neo4j, Gephi, yEd, Cytoscape. GraphML is trivial (XML), Cypher enables visual graph exploration. | P3 |
| Snapshot diff workflows | Snapshot diffing exists but isn't wired into a "what changed in my architecture this sprint" workflow. | P3 |

## Multi-Repo Management

| Item | Description | Priority |
|------|-------------|----------|
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots from the graph. Currently requires manual SQL. | P2 |
| Active project scoping | Session-level "I'm working in repo X" default so agents don't pass `repo_url` on every call. `set_active_project` / `get_active_project` MCP tools. | P3 |
| `graph_stats` MCP tool | Total nodes/edges + per-repo breakdown + session token savings. Overlaps with `knowing stats` CLI. | P3 |

## MCP Resources (Planned)

Resources are read directly by the MCP host without a tool call. They provide lightweight orientation context at zero exchange cost.

| Resource | What it provides | Data source | Priority |
|----------|-----------------|-------------|----------|
| `knowing://report` | High-level orientation: graph size, top languages/kinds, hotspot count. The opening read of a new session. | Aggregate query over nodes/edges tables | P1 |
| `knowing://schema` | Graph schema reference: node kinds, edge kinds, provenance tiers, qualified-ID format. Helps agents form valid queries. | Static, derived from types package | P1 |
| `knowing://stats` | Node/edge counts, per-language and per-repo breakdown. Cheapest health check. | `AllRepos` + count queries | P1 |
| `knowing://repos` | Every tracked repo with node/edge counts. | `AllRepos` store method | P2 |
| `knowing://session` | Current session state: recent symbols, context calls, feedback rate, token savings. | SessionTracker + counters | P2 |
| `knowing://index-health` | Health score, parse failures, stale files. Subscribe for push updates after re-index. | CAS staleness detection | P2 |
| `knowing://communities` | Community list with cohesion scores. | Louvain output | P3 |
| `knowing://community/{id}` | Single community detail: members, key files, cross-community connections. | Filtered Louvain output | P3 |

## Underexploited Capabilities

These exist in the codebase but aren't wired into retrieval or workflows yet:

| Item | Status | Next step |
|------|--------|-----------|
| Community-aware retrieval | Communities computed, not used for scoping | Constrain RWR walk to seed communities (on roadmap) |
| Leiden algorithm | Louvain can produce internally disconnected communities | Replace Louvain with Leiden when community-aware retrieval ships, since community quality directly affects retrieval accuracy. Leiden is a drop-in fix (same modularity objective, guarantees connected communities). |
| Edge event log | Events recorded, nothing reads them | Temporal queries: "when did this dependency appear?" |
| LSP enrichment (TS/Python/Java) | Shipped. TS: 98.9% upgrade rate. Python: 83% upgrade + 15K new edges. Java: working via jdtls with workspace readiness waiting. | Rust and C# enrichment available via rust-analyzer and OmniSharp when installed. |

## Retrieval Pipeline

Pipeline is shipped and measured (31.6% P@10, 55 fixtures, 23 experiments). See [retrieval-pipeline.md](retrieval-pipeline.md) for the authoritative reference.

**Next retrieval improvements (per local-first philosophy):**

| Item | Description | Status |
|------|-------------|--------|
| More equivalence concepts | Expand from 41 to 100+ as usage patterns emerge | Ongoing |
| Passive task memory compounding | Needs real agent sessions to accumulate data | Waiting on users |
| Session memory persistence | Persist session working sets to SQLite, replay on resume, compound cross-session patterns | Planned |
| Negative feedback signals | Penalize "this was noise" symbols in scoring, not just boost "this was relevant" | Planned |
| Code-tuned embedding model | Benchmark jina-code-v2 / bge-code when ONNX available | Planned (optional) |
| Community-aware retrieval | Constrain RWR walk to seed communities | Planned |

## Edge Type Expansion

### Runtime Intelligence

| Item | Description | Priority |
|------|-------------|----------|
| `runtime_queries` | Service/function queries database table/view/procedure | P1 |
| `runtime_connects_to` | Observed network connection beyond traced HTTP/RPC | P2 |
| `runtime_errors_at` | Symbol/route produces runtime errors | P3 |
| `runtime_uses_config` | Function reads config key or secret at runtime | P4 |
| `runtime_emits_metric` | Symbol emits a named metric | P5 |
| `runtime_logs_event` | Symbol emits a structured log event type | P5 |
| `runtime_writes` | Service/function writes table, bucket, queue, cache key, file, or object | Future |
| `runtime_reads` | Service/function reads table, bucket, cache key, config, secret, file, or object | Future |
| `runtime_scheduled` | Cron/job/workflow invoked function or service at runtime | Future |
| `runtime_allocates` | Service/function provisions or dynamically creates cloud resource | Future |
| `runtime_redirects_to` | HTTP route redirects/forwards/proxies to another route/service | Future |
| `runtime_authenticates_as` | Service acts as principal/role/user/client identity | Future |
| `runtime_authorizes` | Policy/permission check observed for route/function/action | Future |
| `runtime_depends_on` | Observed dependency inferred from runtime behavior when static linkage is absent | Future |

### Contract and API Edges

| Item | Description | Priority |
|------|-------------|----------|
| `implements_endpoint` | Handler function implements OpenAPI route | P1 |
| `consumes_endpoint` | Client code calls OpenAPI route | P1 |
| `implements_rpc` | Server implements proto RPC method | P2 |
| `consumes_rpc` | Client invokes proto RPC method | P2 |
| `publishes_event_schema` | Producer emits event matching a contract | P3 |
| `consumes_event_schema` | Consumer expects event matching a contract | P3 |
| `defines_schema` | Code/type defines schema or contract | Future |
| `validates_against` | Code validates payload against schema | Future |
| `serializes` / `deserializes` | Type crosses wire/storage boundary | Future |
| `breaking_change_for` | Derived edge from schema/API diff between versions | Future |

### Ownership and Governance

| Item | Description | Priority |
|------|-------------|----------|
| `owned_by` | Symbol/file/service owned by team/person (CODEOWNERS) | P1 |
| `classified_as` | Data classification (PII, PCI, PHI) | P2 |
| `secured_by` | Route/service protected by auth policy | P3 |
| `reviewed_by` | Code area requires specific reviewer | Future |
| `complies_with` | Maps component to compliance control | Future |
| `violates_policy` | Derived: symbol with PII classification lacks secured_by edge | Future |

### Static Semantic Edges

| Item | Description | Priority |
|------|-------------|----------|
| `extends` / `inherits` | Class inheritance (Java, C#, Python, TS) | P1 |
| `overrides` | Method overrides parent/interface method | P1 |
| `decorates` / `annotates` | Decorators, annotations, attributes | P2 |
| `throws` / `raises` | Error/exception relationships | P3 |
| `catches` / `handles_error` | Recovery paths for exceptions | Future |
| `generates` | Codegen source produces generated file/symbol | Future |

### Agent Workflow Edges

| Item | Description | Priority |
|------|-------------|----------|
| `suggested_for_task` | Symbol was included in agent context for a task | P1 |
| `used_by_agent` | Agent actually used/read/edited symbol | P1 |
| `validated_by_test` | Test verified symbol/change | P2 |
| `failed_in_ci` | Symbol associated with failing check | P2 |
| `changed_by_pr` | PR modifies symbol | Future |
| `reviewed_in_pr` | PR review comment targets symbol | Future |

### Deployment and Infrastructure Edges

| Item | Description | Priority |
|------|-------------|----------|
| `runs_on` | Service runs on deployment/node/runtime | P1 |
| `deployed_by` | Workflow/pipeline deploys service | P1 |
| `configured_by` | Config/secret/env var configures service | P2 |
| `exposes_port` | Service/container exposes port | Future |
| `mounts` | Workload mounts volume/secret/configmap | Future |
| `assumes_role` | Workload uses IAM role/service account | Future |
| `allowed_by` / `blocked_by` | Network/security/IAM policy permits or denies access | Future |

## Developer Visibility

| Item | Description |
|------|-------------|
| Ownership routing | "Who to notify" computed from graph edges (depends on ownership edges) |
| Staleness dashboard | Surface unverified edges and subgraphs |

## Agent Coordination

| Item | Description |
|------|-------------|
| Pending mutations | Agents announce in-flight changes, others see proposed state |
| Temporal reasoning | Walk snapshots backward to find when incompatibilities appeared |
| Federated graphs | Cross-instance queries via Merkle diff exchange |

## Strategic Position

knowing is an intelligence versioning system. Git versions files; knowing versions the understanding of code: relationships, confidence, provenance, and what changes mean. Every snapshot captures not just structure but learned intelligence (feedback, session patterns, task memory) that compounds with use.

The retrieval pipeline uses equivalence classes (not embeddings) as the primary concept-matching mechanism. This is local, deterministic, inspectable, and compounds with use. See [retrieval-pipeline.md](retrieval-pipeline.md) for the design rationale.

**What's shipped (v0.2.0):** ~60K LOC Go, 25 extractor types (12 languages + 13 infrastructure/cloud formats), 23 MCP tools, 5 wire formats (GCF/TOON/JSON/XML/markdown), 55 eval fixtures, 84 equivalence classes, multi-language LSP enrichment (Go, TS, Python, Java, Rust, C#), `knowing init` one-command setup, `knowing why` retrieval explainability.
