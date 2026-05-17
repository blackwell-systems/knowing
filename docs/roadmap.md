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
| Semantic embedding seeds | MiniLM-L6-v2 embeddings for concept-level retrieval (see below) | planned (high impact) |

### Embedding Retrieval (research note)

knowing's current keyword-only seeding hits 36% P@10 on easy, 2% on hard. The gap is entirely in
"concept" queries where task descriptions use different words than symbol names ("authentication
handling" should find `AuthMiddleware` even with zero keyword overlap).

**Implementation plan (no CGO required):**
- Runtime: `github.com/knights-analytics/hugot` (pure Go ONNX, auto-downloads model ~30MB)
- Storage: `github.com/coder/hnsw` (pure Go, in-memory, 384 dims, cap 100K symbols)
- Embed text: `fmt.Sprintf("%s %s %s %s", node.Kind, node.Name, signature, filePath)`
- Fusion: RRF (k=60) combining keyword seeds + vector nearest-50 as joint seed set
- RWR + HITS + feedback still rank the final output

**Expected impact:** Hard tier 2% -> 15-25%, medium 16% -> 35-45%, easy should stay >80%.
The embedding layer finds semantically related symbols that keyword matching misses entirely.

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

**Ingestion sources:** Cross-reference between schema extractor output (route definitions, proto services) and language extractor output (handler registrations, generated client stubs). The data already flows through existing extractors; the work is matching route patterns to handler functions and emitting semantically specific edge types instead of generic `references`.

| Item | Description | Ingestion source | Status |
|------|-------------|-----------------|--------|
| `implements_endpoint` | Handler function implements OpenAPI route | Schema extractor (route) + language extractor (handler registration pattern match on HTTP method + path) | planned (P1) |
| `consumes_endpoint` | Client code calls OpenAPI route | Language extractor (HTTP client calls with URL patterns matching known OpenAPI routes) | planned (P1) |
| `implements_rpc` | Server implements proto RPC method | Proto extractor (service def) + language extractor (generated server interface implementations) | planned (P2) |
| `consumes_rpc` | Client invokes proto RPC method | Proto extractor (service def) + language extractor (generated client stub calls) | planned (P2) |
| `publishes_event_schema` | Producer emits event matching a contract | Event extractor (publish calls) + schema extractor (event schema definitions), matched by topic/event name | planned (P3) |
| `consumes_event_schema` | Consumer expects event matching a contract | Event extractor (subscribe calls) + schema extractor (event schema definitions), matched by topic/event name | planned (P3) |
| `defines_schema` | Code/type defines schema or contract | Schema extractor already extracts these; promote to dedicated edge type | planned |
| `validates_against` | Code validates payload against schema | Tree-sitter pattern: validation library calls referencing schema identifiers | planned |
| `serializes` / `deserializes` | Type crosses wire/storage boundary | Tree-sitter pattern: marshaling/encoding calls with type arguments | planned |
| `breaking_change_for` | Derived edge from schema/API diff between versions | Snapshot diff on schema nodes between two snapshots; detect removed/changed fields | planned |

## Workstream: Ownership and Governance

Turns code intelligence into organizational intelligence. Answers "who gets paged?" and "what policy governs this?"

**Ingestion sources:** Primarily file-based config parsing (CODEOWNERS, Backstage catalogs, OPA policies). `classified_as` is the exception: requires heuristics or manual annotation.

| Item | Description | Ingestion source | Status |
|------|-------------|-----------------|--------|
| `owned_by` | Symbol/file/service owned by team/person | CODEOWNERS file parsing (glob patterns to team), Backstage `catalog-info.yaml`, OpsLevel service catalog YAML | planned (P1) |
| `classified_as` | Data/resource classification (PII, PCI, PHI) | Heuristics on field/variable names (`ssn`, `credit_card`, `password`), OpenAPI `x-pii` extensions, manual annotation files, or policy-as-code labels | planned (P2) |
| `secured_by` | Route/service protected by auth policy | Tree-sitter patterns: auth middleware/decorator detection (`@RequiresAuth`, `authMiddleware()`), OPA/Rego policy files, IAM policy documents | planned (P3) |
| `reviewed_by` | Code area requires specific reviewer | CODEOWNERS (same source as `owned_by`), GitHub branch protection rules API | planned |
| `complies_with` | Maps component to compliance control | Manual annotation files or compliance-as-code frameworks (compliance YAML/JSON mapping controls to paths) | planned |
| `violates_policy` | Derived policy finding from graph analysis | Derived: query graph for symbols with `classified_as: PII` that lack `secured_by` edges | planned |

## Workstream: Static Semantic Edges

Deeper type-system relationships beyond calls/imports/implements.

**Ingestion sources:** All from existing tree-sitter extractors with additional query patterns. Go/ast extractor already produces `implements`; extending to inheritance/override for OOP languages uses the same tree-sitter infrastructure.

| Item | Description | Ingestion source | Status |
|------|-------------|-----------------|--------|
| `extends` / `inherits` | Class/type inheritance (Java, C#, Python, TS) | Tree-sitter: `class Foo extends Bar`, `class Foo(Bar)`, `: BaseClass` | planned (P1) |
| `overrides` | Method overrides parent/interface method | Tree-sitter: method with same name as parent class method + `@Override`/`override` keyword | planned (P1) |
| `decorates` / `annotates` | Decorators, annotations, attributes | Tree-sitter: `@decorator`, `@Annotation`, `[Attribute]` patterns | planned (P2) |
| `throws` / `raises` | Error/exception relationships | Tree-sitter: `throw new X`, `raise X`, Go `return fmt.Errorf` patterns | planned (P3) |
| `catches` / `handles_error` | Recovery paths | Tree-sitter: `catch(X)`, `except X`, `if err != nil` with type assertion | planned (P3) |
| `generates` | Codegen source produces generated file/symbol | File header comments (`// Code generated by`), build tool config (protoc, sqlc, ent) | planned |

## Workstream: Agent Workflow Edges

The graph improves itself from agent behavior. Promotes existing feedback data into first-class graph edges.

**Ingestion sources:** Mostly already captured. Feedback table has `suggested_for_task` data. Claude Code hooks observe edits. GitHub API provides CI/PR events. The work is promoting these observations into graph edges with provenance.

| Item | Description | Ingestion source | Status |
|------|-------------|-----------------|--------|
| `suggested_for_task` | Symbol was included in agent context for a task | Already captured in feedback table (`RecordFeedback`); promote to edges | planned (P1) |
| `used_by_agent` | Agent actually used/read/edited symbol | Claude Code PostToolUse hook (file edit events); match edited ranges to graph nodes | planned (P1) |
| `validated_by_test` | Test verified symbol/change | Existing `calls` edges from test functions to symbols; reclassify with test metadata | planned (P2) |
| `failed_in_ci` | Symbol/file associated with failing check | GitHub Actions API (check run failures), map failing test to symbols via `test_scope` logic | planned (P2) |
| `changed_by_pr` | PR modifies symbol | GitHub PR API (changed files) + `NodesByFilePath` to resolve symbols | planned (P3) |
| `reviewed_in_pr` | PR review comment targets symbol | GitHub PR review comments API (file + line) + node position matching | planned (P3) |

## Workstream: Deployment and Infrastructure Edges

Links code to its operational environment.

**Ingestion sources:** Mostly from existing cloud/k8s extractors that already parse these files. The work is emitting semantically specific edges instead of generic `depends_on`. Some require cross-referencing (service name in deployment YAML matches service name in code).

| Item | Description | Ingestion source | Status |
|------|-------------|-----------------|--------|
| `runs_on` | Service runs on deployment/node/runtime | K8s extractor (Deployment -> container image), Docker Compose (service -> image), Serverless (function -> runtime) | planned (P1) |
| `deployed_by` | Workflow/pipeline deploys service | GitHub Actions extractor (deploy steps referencing service/image names) | planned (P1) |
| `configured_by` | Config/secret/env var configures service | K8s extractor (ConfigMap/Secret mounts, envFrom), Docker Compose (environment), Terraform (variable references) | planned (P2) |
| `exposes_port` | Service/container exposes port | K8s extractor (Service ports), Docker Compose (ports), already partially extracted | planned (P3) |
| `mounts` | Workload mounts volume/secret/configmap | K8s extractor (volumeMounts), Docker Compose (volumes) | planned |
| `assumes_role` | Workload uses IAM role/service account | K8s extractor (serviceAccountName), CloudFormation (Role/Policy), Terraform (aws_iam_role) | planned |
| `allowed_by` / `blocked_by` | Network/security/IAM policy permits or denies access | K8s NetworkPolicy, CloudFormation SecurityGroup, Terraform aws_security_group_rule, OPA policies | planned |

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
