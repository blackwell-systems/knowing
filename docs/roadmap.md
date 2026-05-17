# Roadmap

The system is designed as parallel workstreams, not sequential phases. The architecture (content-addressed storage, provenance model, storage interface, snapshot chain) supports all workstreams from day one. Implementation order is driven by dependency constraints, not cautious scoping.

## Workstream: Graph Core

Everything else depends on this. Complete.

| Item | Description | Status |
|------|-------------|--------|
| Content-addressed store | SQLite backend, Merkle DAG, snapshot chain | **done** |
| Extractor framework | Language-agnostic interface, worker pool parallelism | **done** |
| 11 language extractors | Go, TypeScript/JS, Python, Rust, Java, C#, Terraform, SQL, K8s YAML, CSS, Protocol Buffers | **done** |
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
| SCIP ingest | Tier 2 shallow indexing for external dependencies | planned |
| Protobuf/gRPC edges | Proto field references, service-to-service RPC relationships | **done** |
| Event edges | Kafka/NATS/SQS topic producers and consumers | planned |
| Schema edges | OpenAPI specs, JSON Schema, proto-as-schema references | planned |
| Ownership edges | CODEOWNERS, team annotations, service catalog metadata | planned |

## Workstream: Runtime Intelligence

Complete.

| Item | Description | Status |
|------|-------------|--------|
| OTLP gRPC receiver | Real gRPC server accepting OTel spans | **done** |
| Confidence scoring | Observation-count-based (log2 growth, 0.95 max) | **done** |
| Confidence decay | Hourly decay in daemon trace loop | **done** |
| Symbol resolver | Maps runtime identifiers to graph nodes via route_symbols | **done** |
| MCP runtime tools | runtime_traffic, dead_routes, trace_stats | **done** |
| Database query edges | Ingest DB query logs as runtime_queries edges | planned |

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

## What's Next (priority order)

1. **MCP resources.** `knowing://context/<scope>` subscribable resources for live context updates.

3. **More edge types.** Event edges (Kafka/NATS), schema edges (OpenAPI), ownership edges (CODEOWNERS).

4. **Traversal cache.** L1 in-memory LRU for hot paths, L2 materialized closures for common queries.

5. **v0.1.0 release.** Homebrew tap, npm/pypi wrappers, Docker images. Needs CI secrets configured.

## Dependency Graph

```
Graph Core ──────────────────────────────> all other workstreams          ✓ DONE
Edge types (10 languages + routes) ──────> Context packing               ✓ DONE
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
- Protobuf/gRPC, event, schema, ownership edge types
- Graph-native test selection
- Traversal cache
- Database query edges
- SCIP ingest

**Sequential (must wait for dependencies):**
- Claude Code hooks depend on context packing (done)
- context_for_pr depends on context packing + semantic diff (both done)
- Ownership routing depends on ownership edges (not started)
- Federated graphs depend on Merkle sync protocol (not started)
