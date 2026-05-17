# Roadmap

The system is designed as parallel workstreams, not sequential phases. The architecture (content-addressed storage, provenance model, storage interface, snapshot chain) supports all workstreams from day one. Implementation order is driven by dependency constraints, not cautious scoping.

## Workstream: Graph Core

Everything else depends on this. Complete.

| Item | Description | Status |
|------|-------------|--------|
| Content-addressed store | SQLite backend, Merkle DAG, snapshot chain | **done** |
| Extractor framework | Language-agnostic interface, worker pool parallelism | **done** |
| 12 extractors (15 formats) | Go, TypeScript/JS, Python, Rust, Java, C#, Terraform, SQL, K8s YAML, Cloud YAML (CFN/SAM, Compose, Actions, Serverless), CSS, Protocol Buffers | **done** |
| LSP enrichment | Upgrades ast_inferred to lsp_resolved via gopls | **done** |
| Incremental indexing | Git watcher, file change detection, deleted file cleanup | **done** |
| Snapshot diff | Edge event sourcing, added/removed between snapshots | **done** |
| Cross-repo resolution | Module-to-repo URL mapping, dangling edge retargeting | **done** |
| MCP server | 22 tools + 3 prompts over stdio/HTTP | **done** |
| Daemon + git watcher | Persistent process, .git/HEAD watching, incremental reindex | **done** |
| Traversal cache | L1 in-memory LRU, L2 materialized closures, L3 bounded traversal | planned |

## Workstream: Edge Types

| Item | Description | Status |
|------|-------------|--------|
| HTTP route edges | 18 frameworks across 6 languages (Go, TS, Python, Rust, Java, C#) | **done** |
| Infrastructure extractors | Terraform, SQL, K8s YAML, CSS | **done** |
| SCIP ingest | `knowing ingest-scip` CLI: import .scip protobuf indexes, provenance `scip_resolved` (0.95) | **done** |
| Cloud extractors | CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework (via CloudExtractor) | **done** |
| Protobuf/gRPC edges | Proto field references, service-to-service RPC relationships | **done** |
| Event edges | Kafka/NATS/SQS/RabbitMQ topic producers and consumers via multi-dispatch | **done** |
| Schema edges | OpenAPI 3.x, Swagger 2.x, JSON Schema document parsing via multi-dispatch | **done** |
| Ownership edges | CODEOWNERS, team annotations, service catalog metadata | planned |

## Workstream: Runtime Intelligence

Core pipeline complete. Runtime edge expansion planned.

| Item | Description | Status |
|------|-------------|--------|
| OTLP gRPC receiver | Real gRPC server accepting OTel spans | **done** |
| Confidence scoring | Observation-count-based (log2 growth, 0.95 max) | **done** |
| Confidence decay | Hourly decay in daemon trace loop | **done** |
| Symbol resolver | Maps runtime identifiers to graph nodes via route_symbols | **done** |
| MCP runtime tools | runtime_traffic, dead_routes, trace_stats | **done** |
| `runtime_queries` | Service/function queries database table/view/procedure. Highest-value missing plane: links code to data ownership and migration risk. | planned (P1) |
| `runtime_connects_to` | Observed network connection to service, host, database, broker, cache, or external API beyond traced HTTP/RPC. | planned (P2) |
| `runtime_errors_at` | Symbol/route/service produces runtime errors, tied to trace/log/error telemetry. Enables incident/debug workflows. | planned (P3) |
| `runtime_uses_config` | Service/function reads config key or secret at runtime. Enables config/secret blast radius. | planned (P4) |
| `runtime_emits_metric` | Symbol/service emits a named metric. Observability-to-code navigation. | planned (P5) |
| `runtime_logs_event` | Symbol/service emits a structured log event type. | planned (P5) |
| `runtime_writes` | Service/function writes table, bucket, queue, cache key, file, or object. | planned |
| `runtime_reads` | Service/function reads table, bucket, cache key, config, secret, file, or object. | planned |
| `runtime_scheduled` | Cron/job/workflow invoked function or service at runtime. | planned |
| `runtime_allocates` | Service/function provisions or dynamically creates cloud resource. | planned |
| `runtime_redirects_to` | HTTP route redirects/forwards/proxies to another route/service. | planned |
| `runtime_authenticates_as` | Service acts as principal/role/user/client identity. | planned |
| `runtime_authorizes` | Policy/permission check observed for route/function/action. | planned |
| `runtime_depends_on` | Observed dependency inferred from runtime behavior when static linkage is absent. | planned |

## Workstream: Context Packing

Core pipeline complete. v2 refinements identified.

| Item | Description | Status |
|------|-------------|--------|
| Context engine | Keyword extraction, RWR scoring, budget packing | **done** |
| context_for_task MCP tool | Task description + budget -> ranked context | **done** |
| context_for_files MCP tool | Changed files -> blast radius context | **done** |
| Wire format system | GCF (84% token savings), GCB (74% byte savings), JSON, codec registry | **done** |
| GCF session statefulness | Cross-call dedup, 47% additional savings on repeated symbols | **done** |
| HITS hub/authority reranking | On top-200 RWR subgraph for better prioritization | **done** |
| Density-ranked knapsack packing | Score/cost ratio optimization for budget utilization | **done** |
| context_for_pr MCP tool | PR-scoped context (changed files + diff + relationship awareness) | **done** |
| Feedback-aware scoring | FeedbackProvider interface wired into ContextEngine for ranking improvement | **done** |
| MCP resources | knowing://context/<scope> subscribable resources | planned |

## Workstream: Developer Visibility

| Item | Description | Status |
|------|-------------|--------|
| Semantic PR diff | SemanticDiff + PRImpact + knowing diff CLI | **done** |
| knowing export CLI | Export graph as JSON or DOT with Louvain community annotations | **done** |
| Claude Code hooks | PreToolUse/PostToolUse auto-context injection, benchmarked net-positive | **done** |
| Graph-native test selection | knowing test-scope: affected tests from call graph BFS | **done** |
| Louvain community detection | Communities MCP tool + DOT export with subgraphs | **done** |
| Agent feedback loop | Feedback MCP tool + FeedbackProvider for ranking improvement | **done** |
| Flow analysis | flow_between MCP tool: BFS path finding between symbols | **done** |
| Plan turn | plan_turn MCP tool: task-to-tool keyword recommender | **done** |
| Ownership routing | "Who to notify" computed from graph edges | planned |
| Staleness dashboard | Surface unverified edges and subgraphs | planned |

## Workstream: Agent Coordination

| Item | Description | Status |
|------|-------------|--------|
| Pending mutations | Agents announce in-flight changes, others see proposed state | planned |
| Temporal reasoning | Walk snapshots backward to find when incompatibilities appeared | planned |
| Federated graphs | Cross-instance queries via Merkle diff exchange | planned |

## Workstream: Contract and API Edges

Connects schemas/contracts to the code that implements or consumes them. Turns extracted schemas (OpenAPI, proto, event patterns) into actionable caller/implementor relationships.

| Item | Description | Status |
|------|-------------|--------|
| `implements_endpoint` | Handler function implements OpenAPI route | planned (P1) |
| `consumes_endpoint` | Client code calls OpenAPI route | planned (P1) |
| `implements_rpc` | Server implements proto RPC method | planned (P2) |
| `consumes_rpc` | Client invokes proto RPC method | planned (P2) |
| `publishes_event_schema` | Producer emits event matching a contract | planned (P3) |
| `consumes_event_schema` | Consumer expects event matching a contract | planned (P3) |
| `defines_schema` | Code/type defines schema or contract | planned |
| `validates_against` | Code validates payload against schema | planned |
| `serializes` / `deserializes` | Type crosses wire/storage boundary | planned |
| `breaking_change_for` | Derived edge from schema/API diff between versions | planned |

## Workstream: Ownership and Governance

Turns code intelligence into organizational intelligence. Answers "who gets paged?" and "what policy governs this?"

| Item | Description | Status |
|------|-------------|--------|
| `owned_by` | Symbol/file/service owned by team/person (CODEOWNERS, annotations) | planned (P1) |
| `classified_as` | Data/resource classification (PII, PCI, PHI) | planned (P2) |
| `secured_by` | Route/service protected by auth policy | planned (P3) |
| `reviewed_by` | Code area requires specific reviewer | planned |
| `complies_with` | Maps component to compliance control | planned |
| `violates_policy` | Derived policy finding from graph analysis | planned |

## Workstream: Static Semantic Edges

Deeper type-system relationships beyond calls/imports/implements.

| Item | Description | Status |
|------|-------------|--------|
| `extends` / `inherits` | Class/type inheritance (Java, C#, Python, TS) | planned (P1) |
| `overrides` | Method overrides parent/interface method | planned (P1) |
| `decorates` / `annotates` | Decorators, annotations, attributes | planned (P2) |
| `throws` / `raises` | Error/exception relationships | planned (P3) |
| `catches` / `handles_error` | Recovery paths | planned (P3) |
| `generates` | Codegen source produces generated file/symbol | planned |

## Workstream: Agent Workflow Edges

The graph improves itself from agent behavior. Promotes existing feedback data into first-class graph edges.

| Item | Description | Status |
|------|-------------|--------|
| `suggested_for_task` | Symbol was included in agent context for a task | planned (P1) |
| `used_by_agent` | Agent actually used/read/edited symbol | planned (P1) |
| `validated_by_test` | Test verified symbol/change | planned (P2) |
| `failed_in_ci` | Symbol/file associated with failing check | planned (P2) |
| `changed_by_pr` | PR modifies symbol | planned (P3) |
| `reviewed_in_pr` | PR review comment targets symbol | planned (P3) |

## Workstream: Deployment and Infrastructure Edges

Links code to its operational environment.

| Item | Description | Status |
|------|-------------|--------|
| `runs_on` | Service runs on deployment/node/runtime | planned (P1) |
| `deployed_by` | Workflow/pipeline deploys service | planned (P1) |
| `configured_by` | Config/secret/env var configures service | planned (P2) |
| `exposes_port` | Service/container exposes port | planned (P3) |
| `mounts` | Workload mounts volume/secret/configmap | planned |
| `assumes_role` | Workload uses IAM role/service account | planned |
| `allowed_by` / `blocked_by` | Network/security/IAM policy permits or denies access | planned |

## What's Next (priority order)

1. **`runtime_queries` edge type.** Links code to data ownership. Highest-value missing plane for migration risk and incident response.

2. **Contract edges.** `implements_endpoint` / `consumes_endpoint` connect OpenAPI schemas to handler code. Enables "which services break if I change this API?"

3. **Ownership edges.** CODEOWNERS parsing + `owned_by` edges. Enables blast radius queries that answer "which team gets paged?"

4. **MCP resources.** `knowing://context/<scope>` subscribable resources for live context updates.

5. **Traversal cache.** L1 in-memory LRU for hot paths, L2 materialized closures for common queries.

6. **v0.1.0 release.** Homebrew tap, npm/pypi wrappers, Docker images. Needs CI secrets configured.

## Dependency Graph

```
Graph Core ──────────────────────────────> all other workstreams          ✓ DONE
Edge types (12 extractors + routes) ─────> Context packing               ✓ DONE
Runtime intelligence (traces + decay) ───> Context packing               ✓ DONE
Wire format system (GCF/GCB) ────────────> Session statefulness          ✓ DONE
Context packing ─────────────────────────> Claude Code hooks             ✓ DONE
Context packing ─────────────────────────> context_for_pr                ✓ DONE
Semantic PR diff ────────────────────────> context_for_pr                ✓ DONE
Call graph + traversal ──────────────────> Graph-native test selection   ✓ DONE
Ownership edges ─────────────────────────> Ownership routing             planned
MCP server ──────────────────────────────> Pending mutations             planned
Snapshot chain + Merkle sync ────────────> Federated graphs              planned
```

## Parallelization Notes

**Independent (can be implemented any time):**
- Runtime edge expansion (all 14 types are independent of each other)
- Contract edges (OpenAPI/proto schemas already extracted)
- Ownership edges (CODEOWNERS parsing)
- Static semantic edges (tree-sitter infrastructure exists)
- Traversal cache
- Agent workflow edges (feedback table exists)

**Sequential (must wait for dependencies):**
- Ownership routing depends on ownership edges (not started)
- `breaking_change_for` depends on schema diffing (not started)
- `violates_policy` depends on `classified_as` + `secured_by` (not started)
- Federated graphs depend on Merkle sync protocol (not started)
