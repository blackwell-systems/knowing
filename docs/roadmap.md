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
| MCP server | 16 tools over stdio + HTTP (graph queries, runtime tools, semantic diff, PR impact, context packing) | store, call graph | **done** |
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

The insight: instead of agents making 5-10 tool calls to understand a codebase area, knowing produces a single pre-computed context block ranked by structural importance and runtime traffic. Works with any agent framework that can call an MCP tool.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| `knowing context` CLI | `knowing context --task "description" --budget 50000` produces graph-ranked, token-budgeted context | graph core, runtime intelligence | **done** |
| `context_for_task` MCP tool | Accepts task description + token budget, returns optimal context window | knowing context | **done** |
| `context_for_files` MCP tool | Accepts changed files, returns blast radius context with runtime weights | knowing context | **done** |
| Relevance retrieval pipeline | Multi-stage pipeline: identifier extraction, graph walk, hub/authority scoring, budget packing (see below) | graph core | **next** |
| `context_for_pr` MCP tool | Accepts PR (changed files + diff), returns relationship-aware review context | knowing context, semantic diff | planned |
| MCP resources | `knowing://context/<scope>` subscribable resources that update when graph changes | MCP server | planned |
| MCP prompts | Pre-built reasoning templates for common tasks (refactor, review, debug) | MCP server | planned |

### Relevance Retrieval Pipeline (v2)

The current context engine uses naive keyword substring matching, producing flat, undifferentiated scores. The v2 pipeline replaces this with a multi-stage approach that leverages the graph structure for relevance:

**Stage 1: Identifier Extraction + Seed Selection**

Extract candidate identifiers from the task description using:
- CamelCase/snake_case splitting into component words ("refactorAuthMiddleware" -> ["refactor", "auth", "middleware"])
- Abbreviation expansion via a maintained map (~50 entries: ctx/context, fmt/format, req/request, resp/response, err/error, cfg/config, etc.)
- Code stop word filtering (remove: new, get, set, make, init, is, has, err, the, a, an, for, to, from, with, this, that)
- Package path matching: if any extracted term matches a package directory name, boost symbols in that package 2x
- Exported symbol preference: public symbols get 1.5x boost over unexported

Score qualified names with BM25-style term frequency, boosted by call graph in-degree (log2(indegree) multiplier). Top N (default 20) become **seed nodes** for stage 2.

**Stage 2: Random Walk with Restart (RWR)**

From the seed nodes identified in stage 1, run Random Walk with Restart across the call/import graph:
- Restart probability: 0.2 (returns to seed nodes with 20% probability each step)
- Edge weights by type: `calls` edges propagate 1.0, `imports` edges propagate 0.5, `implements` edges propagate 0.8, `handles_route` edges propagate 0.7
- Convergence: iterate until delta < 0.001 or 20 iterations (whichever first)
- The stationary distribution assigns a relevance score to every reachable node

For graphs under 50K nodes, this completes in under 50ms. The result naturally captures transitive relevance: if you seed from `HandleRequest`, RWR surfaces its helper functions, the types it operates on, and the middleware that calls it, without explicitly querying for them.

**Stage 3: Hub/Authority Reranking (HITS)**

On the subgraph of nodes with RWR score above a threshold (top 200), run HITS (Hyperlink-Induced Topic Search):
- **Authority** nodes: heavily called symbols (the core business logic)
- **Hub** nodes: symbols that call many others (orchestrators, entry points)
- For most tasks, return authorities first (the logic you need to understand), then hubs (the entry points that wire things together)
- 5-10 iterations on a 200-node subgraph, converges instantly

Combine: `final_score = 0.5 * rwr_score + 0.3 * authority_score + 0.2 * secondary_signals` where secondary signals are confidence, recency, and runtime observation count.

**Stage 4: Density-Ranked Knapsack Packing**

Pack scored symbols into the token budget using a greedy density heuristic:
- Score each symbol as `final_score / estimated_token_cost`
- Sort descending, greedily include until budget exhausted
- Hierarchy rule: if a method is included, include its parent type signature at 0.3x cost weight
- Partial inclusion: for large symbols, include signature + first-line doc, not full body
- Token estimation: 4 chars per token (existing heuristic)

This outperforms uniform inclusion by maximizing information density per token spent.

**Why this pipeline:**
- No external dependencies (no embedding model, no vector DB, no LLM calls)
- Deterministic (same task description + same graph state = same output, always)
- Fast (all stages combined < 100ms for 10K-node graphs)
- Leverages the graph structure we already have (edges, provenance, confidence)
- Produces meaningfully differentiated scores (hub nodes score higher than leaf functions)

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
Edge types (6 languages + routes) ───────> Context packing v1            ✓ DONE
Runtime intelligence (traces + decay) ───> Context packing v1            ✓ DONE
Context packing v1 ──────────────────────> Relevance pipeline v2         ← NEXT
Relevance pipeline v2 ───────────────────> Claude Code hooks             planned
Relevance pipeline v2 ───────────────────> context_for_pr                planned
Semantic PR diff ────────────────────────> context_for_pr                ✓ DONE
Trace ingestion pipeline ────────────────> Database query edges          planned
Call graph + traversal ──────────────────> Graph-native test selection   planned
Ownership edges ─────────────────────────> Ownership routing             planned
MCP server ──────────────────────────────> Pending mutations             planned
Snapshot chain + Merkle sync ────────────> Federated graphs              planned
```

## What's next (priority order)

1. **Relevance retrieval pipeline v2.** The current context engine finds symbols by naive keyword substring matching, producing flat undifferentiated scores. The v2 pipeline (detailed above) replaces this with: identifier extraction with stop-word filtering -> Random Walk with Restart from seed nodes -> HITS hub/authority reranking -> density-ranked knapsack packing. This is the single change that makes context packing actually useful. No external model needed, deterministic, fast.

2. **Claude Code hooks.** PreToolUse hook that automatically injects relevant context before agent edits. PostToolUse hook that validates changes against the graph. This is the zero-effort adoption path: install knowing, and every agent edit gets structural awareness without the agent doing anything.

3. **`context_for_pr` MCP tool.** Accepts a PR (changed files + diff), runs the retrieval pipeline seeded from changed symbols, returns relationship-aware review context. Depends on the v2 pipeline being good enough.

4. **More edge types.** Protobuf/gRPC edges, event edges (Kafka/NATS), schema edges (OpenAPI). Each extends the graph's coverage without changing the architecture.

5. **Graph-native test selection.** `knowing test-scope` computes affected tests from the relationship graph. High value for CI: run only the tests that matter for a given change.

## Parallelization Notes

When using agentic workflows (polywave or similar), the following can be implemented simultaneously:

**Now (context packing v1 complete, pipeline v2 is next):**
- Identifier extraction + stop-word filter (new file in internal/context/)
- Random Walk with Restart implementation (new file in internal/context/)
- HITS hub/authority scoring (new file in internal/context/)
- Density-ranked knapsack packer (modify existing format/packing logic)

These four components of the v2 pipeline are independent algorithms that compose sequentially but can be implemented in parallel (each is a pure function with defined inputs/outputs).

**After v2 pipeline:**
- Claude Code hooks (PreToolUse/PostToolUse auto-context injection)
- `context_for_pr` MCP tool
- MCP resources (subscribable context)
- MCP prompts (reasoning templates)

**Independent (any time):**
- More edge types (protobuf, events, schemas, infrastructure, ownership)
- Graph-native test selection
- Traversal cache
- Database query edges
- More MCP tools (symbol search, path finding, subgraph extraction)
