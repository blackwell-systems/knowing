# Roadmap

The system is designed as parallel workstreams, not sequential phases. The architecture (content-addressed storage, provenance model, storage interface, snapshot chain) supports all workstreams from day one. Implementation order is driven by dependency constraints, not cautious scoping.

## Workstream: Graph Core

Everything else depends on this. Solid and functional.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Content-addressed store | SQLite backend behind `GraphStore` interface, Merkle DAG, snapshot chain | -- | **done** |
| Extractor framework | Language-agnostic extractor interface with worker pool parallelism | store | **done** |
| Go tree-sitter extractor | Fast-path AST extraction with call-site positions, `ast_inferred` provenance | extractor framework | **done** |
| Go packages extractor | Full type resolution via `go/packages`, implements/references edges (--full flag) | extractor framework | **done** |
| tree-sitter extractors | TypeScript/JS, Rust, Java, C# extractors with framework-specific route detection | extractor framework | **done** |
| tree-sitter Python extractor | Proof of multi-language extractor interface via tree-sitter grammars | extractor framework | **done** |
| LSP enrichment | Upgrades `ast_inferred` edges to `lsp_resolved` via gopls, discovers implements/references | extractor framework | **done** |
| Incremental change detection | Git-based .git/HEAD watching, file cleanup before re-extract, edge event recording | store, indexer | **done** |
| Snapshot diff | Functional once edge events are recorded; returns added/removed edges between snapshots | store, edge events | **done** |
| Cross-repo edge resolution | Module-to-repo URL mapping, dangling edge retargeting | store, indexer | **done** |
| MCP server | 14 tools over stdio + HTTP (graph queries, runtime tools, semantic diff, PR impact) | store, call graph | **done** |
| Daemon + git watcher | Persistent process, .git/HEAD watching (1-2 FDs), incremental reindex on commit | store, indexer | **done** |
| Traversal cache | L1 in-memory LRU, L2 materialized closures, L3 bounded traversal with early termination | store | planned |

## Workstream: Edge Types

Parallelizable. Each edge type is independent. All require Graph Core.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| HTTP route edges | Route-to-symbol mappings during static indexing for 5 Go frameworks + Express.js, Actix/Axum/Rocket, Spring, ASP.NET | store | **done** |
| TypeScript/JS extractor | Declarations, ES6/CommonJS imports, calls with positions, Express.js route detection | extractor framework | **done** |
| Rust extractor | Functions, structs, traits, impl methods, use declarations, Actix/Axum/Rocket routes | extractor framework | **done** |
| Java extractor | Classes, interfaces, enums, methods, constructors, Spring annotation routes | extractor framework | **done** |
| C# extractor | Classes, interfaces, structs, enums, methods, ASP.NET attribute routes | extractor framework | **done** |
| SCIP ingest | Tier 2 shallow indexing for external dependencies via SCIP indices | store | planned |
| Protobuf/gRPC edges | Proto field references, service-to-service RPC relationships | store | planned |
| Event edges | Kafka/NATS/SQS topic producers and consumers | store | planned |
| Schema edges | OpenAPI specs, JSON Schema, proto-as-schema references | store | planned |
| Infrastructure edges | Terraform service references, K8s manifest relationships, docker-compose links | store | planned |
| Ownership edges | CODEOWNERS, team annotations, service catalog metadata | store | planned |

## Workstream: Runtime Intelligence

Bridges the static/dynamic gap. Gives the graph production ground truth, not just static analysis.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Runtime trace types | TraceSpan, TraceIngestor interface, confidence scoring, decay logic | store | **done** |
| Store: runtime columns | Migration 004 (observation_count, last_observed on edges, route_symbols table) | store | **done** |
| Symbol resolver | Maps runtime identifiers (routes, RPC methods) to graph nodes via route_symbols | store | **done** |
| Trace ingestor | Span-to-edge conversion, batch accumulation, dedup, observation counting | symbol resolver | **done** |
| OTLP gRPC receiver | Real gRPC server accepting OTel spans over OTLP protocol | trace ingestor | **done** |
| Route-to-symbol population | During static indexing, extract HTTP handler registrations and write route_symbols entries | HTTP route edges | **done** |
| Daemon trace wiring | Real traceIngestLoop with SymbolResolver + Ingestor + OTLPReceiver lifecycle | OTLP receiver | **done** |
| Confidence decay | DecayConfidence logic done, hourly scheduling in daemon trace loop | daemon trace wiring | **done** |
| MCP runtime tools | `runtime_traffic`, `dead_routes`, `trace_stats` query tools | trace ingestor, MCP server | **done** |
| Database query edges | Ingest DB query logs as `runtime_queries` edges to schema nodes | trace ingestor | planned |

## Workstream: Graph-Aware Context Packing

**The strategic move.** Collapses two market tiers (knowledge graphs and context packing) into one tool. No other tool has both a structural graph AND runtime data to produce task-specific, token-budgeted context for agents.

The insight: instead of agents making 5-10 tool calls to understand a codebase area, knowing produces a single pre-computed context block ranked by blast radius, confidence, recency, and runtime traffic. Works with any agent framework that can call an MCP tool.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| `knowing context` CLI | `knowing context --task "description" --budget 50000` produces graph-ranked, token-budgeted context | graph core, runtime intelligence | **next** |
| `context_for_task` MCP tool | Accepts task description + token budget, returns optimal context window | knowing context | **next** |
| `context_for_files` MCP tool | Accepts changed files, returns blast radius context with runtime weights | knowing context | **next** |
| `context_for_pr` MCP tool | Accepts PR (changed files + diff), returns relationship-aware review context | knowing context, semantic diff | planned |
| Graph-ranked relevance | Rank symbols by: blast radius (callers), confidence (static vs runtime), recency (last observed), distance from task target | graph core, runtime intelligence | **next** |
| Token budget optimization | Pack the highest-ranked symbols into a token budget, XML-structured for agent consumption | relevance ranking | **next** |
| MCP resources | `knowing://context/<scope>` subscribable resources that update when graph changes | MCP server | planned |
| MCP prompts | Pre-built reasoning templates for common tasks (refactor, review, debug) | MCP server | planned |

## Workstream: Developer Visibility

The features developers see. These make knowing's value obvious without requiring workflow changes.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Semantic PR diff | Relationship-level impact diff: SemanticDiff + PRImpact + `knowing diff` CLI + GitHub Action | snapshot chain, SnapshotDiff | **done** |
| `knowing export` CLI | Export graph as JSON with --format, --repo, --snapshot filters | store | **done** |
| Graph-native test selection | `knowing test-scope` computes exact affected tests from the relationship graph | call graph, traversal | planned |
| Ownership routing | "Who do I need to notify about this change?" computed from graph edges | ownership edges | planned |
| Staleness dashboard | Surface edges and subgraphs that haven't been re-verified recently | snapshot chain, confidence | planned |
| Claude Code hooks | PreToolUse + PostToolUse hooks for automatic context injection | context packing, MCP server | planned |

## Workstream: Agent Coordination

Turns knowing from a query layer into a coordination layer for multi-agent workflows.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Pending mutations | Agents announce in-flight changes; other agents see proposed state alongside current state | MCP server | planned |
| Temporal reasoning | Walk backward through snapshots to find when a cross-repo incompatibility was introduced | snapshot chain, SnapshotDiff | planned |
| Federated graphs | Independent knowing instances with cross-federation queries via Merkle diff exchange | snapshot chain, Merkle sync | planned |

## Dependency Graph

```
Graph Core ──────────────────────────────> all other workstreams          ✓ DONE
Edge types (6 languages + routes) ───────> Context ranking               ✓ DONE
Runtime intelligence (traces + decay) ───> Context ranking               ✓ DONE
Context ranking ─────────────────────────> Graph-aware context packing   ← NEXT
Semantic PR diff ────────────────────────> PR context tool               ✓ DONE (diff done, context planned)
Context packing + MCP ───────────────────> Claude Code hooks             planned
Trace ingestion pipeline ────────────────> Database query edges          planned
Call graph + traversal ──────────────────> Graph-native test selection   planned
Ownership edges ─────────────────────────> Ownership routing             planned
MCP server ──────────────────────────────> Pending mutations             planned
Snapshot chain + Merkle sync ────────────> Federated graphs              planned
```

## What's next (priority order)

1. **Graph-aware context packing.** The highest-leverage feature. `knowing context --task "refactor auth" --budget 50000` produces a single, token-budgeted context block ranked by graph relationships and runtime traffic. This is the wedge for adoption: zero infrastructure, works with any agent, instant value. Collapses Tier 1 (knowledge graphs) and Tier 3 (context packing) into one tool.

2. **MCP expansion.** Add `context_for_task`, `context_for_files`, `context_for_pr` tools. Add MCP resources for subscribable context. Add MCP prompts for common agent tasks. Target: 25+ tools (currently 14).

3. **Claude Code hooks.** PreToolUse hook that automatically injects relevant context before agent edits. PostToolUse hook that validates changes against the graph. This is what GitNexus does and it drives their adoption.

4. **More edge types.** Protobuf/gRPC edges, event edges (Kafka/NATS), schema edges (OpenAPI). Each extends the graph's coverage without changing the architecture.

5. **Graph-native test selection.** `knowing test-scope` computes affected tests from the relationship graph. High value for CI: run only the tests that matter for a given change.

## Parallelization Notes

When using agentic workflows (polywave or similar), the following can be implemented simultaneously:

**Now (all foundations complete):**
- `knowing context` CLI + relevance ranking + token budgeting (the core)
- `context_for_task` MCP tool
- `context_for_files` MCP tool
- More MCP tools (symbol search, path finding, subgraph extraction)
- Claude Code hooks (PreToolUse/PostToolUse)

**After context packing:**
- `context_for_pr` MCP tool (needs both context packing and semantic diff)
- MCP resources (subscribable context)
- MCP prompts (reasoning templates)

**Independent (any time):**
- More edge types (protobuf, events, schemas, infrastructure, ownership)
- Graph-native test selection
- Traversal cache
- Database query edges
