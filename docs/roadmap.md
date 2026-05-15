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
| tree-sitter Python extractor | Proof of multi-language extractor interface via tree-sitter grammars | extractor framework | **done** |
| LSP enrichment | Upgrades `ast_inferred` edges to `lsp_resolved` via gopls, discovers implements/references | extractor framework | **done** |
| Incremental change detection | Git-based .git/HEAD watching, file cleanup before re-extract, edge event recording | store, indexer | **done** |
| Snapshot diff | Functional once edge events are recorded; returns added/removed edges between snapshots | store, edge events | **done** |
| Cross-repo edge resolution | Module-to-repo URL mapping, dangling edge retargeting | store, indexer | **done** |
| MCP server | 11 tools over stdio + HTTP (index_repo functional, intelligence handlers read-only) | store, call graph | **done** |
| Daemon + git watcher | Persistent process, .git/HEAD watching (1-2 FDs), incremental reindex on commit | store, indexer | **done** |
| Traversal cache | L1 in-memory LRU, L2 materialized closures, L3 bounded traversal with early termination | store | planned |

## Workstream: Edge Types

Parallelizable. Each edge type is independent. All require Graph Core.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| HTTP route edges | Extract route-to-symbol mappings during static indexing (net/http, chi, gin, echo, gorilla/mux patterns). Populates `route_symbols` table for runtime symbol resolution. | store | **next** |
| tree-sitter TypeScript extractor | Declarations + syntactic calls via tree-sitter grammar | extractor framework | planned |
| tree-sitter Rust extractor | Declarations + syntactic calls via tree-sitter grammar | extractor framework | planned |
| tree-sitter Java extractor | Declarations + syntactic calls via tree-sitter grammar | extractor framework | planned |
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
| OTLP gRPC receiver | Real gRPC server accepting OTel spans over OTLP protocol | trace ingestor | **next** (placeholder exists) |
| Route-to-symbol population | During static indexing, extract HTTP handler registrations and write route_symbols entries | HTTP route edges | **next** (table exists, nothing writes to it) |
| Daemon trace wiring | Connect traceIngestLoop to real Ingestor + OTLPReceiver | OTLP receiver | planned |
| Confidence decay | Background goroutine that runs DecayConfidence periodically | daemon trace wiring | planned (logic done, scheduling not) |
| MCP runtime tools | `runtime_traffic`, `dead_routes`, `migration_progress` query tools | trace ingestor, MCP server | planned |
| Database query edges | Ingest DB query logs as `runtime_queries` edges to schema nodes | trace ingestor | planned |

## Workstream: Developer Visibility

The features developers see. These make knowing's value obvious without requiring workflow changes.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Semantic PR diff | Relationship-level impact diff on every PR (MCP tool + GitHub Action) | snapshot chain, SnapshotDiff | planned |
| Graph-native test selection | `knowing test-scope` computes exact affected tests from the relationship graph | call graph, traversal | planned |
| Ownership routing | "Who do I need to notify about this change?" computed from graph edges | ownership edges | planned |
| Staleness dashboard | Surface edges and subgraphs that haven't been re-verified recently | snapshot chain, confidence | planned |
| `knowing export` CLI | Export graph as JSON for knowing-viz consumption | store | planned |

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
HTTP route edges (route-to-symbol map) ──> Runtime symbol resolution      ← NEXT
Runtime symbol resolution ────────────────> Trace ingestion pipeline      ✓ DONE (pipeline exists, needs data)
Trace ingestion pipeline ─────────────────> Runtime edge creation         ✓ DONE
Trace ingestion pipeline ─────────────────> Database query edges          planned
Edge provenance model ────────────────────> Confidence decay              ✓ DONE (logic done, scheduling planned)
Snapshot chain + SnapshotDiff ────────────> Semantic PR diff              planned
Snapshot chain + SnapshotDiff ────────────> Temporal reasoning            planned
Call graph + traversal ───────────────────> Graph-native test selection   planned
Ownership edges ──────────────────────────> Ownership routing             planned
MCP server ───────────────────────────────> Pending mutations             planned
Snapshot chain + Merkle sync ─────────────> Federated graphs              planned
```

## What's next (priority order)

1. **HTTP route edges + route_symbols population.** The trace pipeline works but has no data. During static indexing, the tree-sitter extractor should recognize `http.HandleFunc("/path", handler)` and similar patterns and write route_symbols entries. This makes the resolver useful immediately for any Go web service.

2. **OTLP gRPC receiver.** Replace the placeholder with a real gRPC server that accepts OTel spans. Requires adding `go.opentelemetry.io/proto/otlp` and `google.golang.org/grpc` to go.mod. After this, you can point an OTel collector at knowing.

3. **Daemon trace wiring.** Connect `traceIngestLoop` to a real Ingestor + OTLPReceiver. After this, `knowing serve --trace` actually ingests spans.

4. **MCP runtime tools.** Add `runtime_traffic`, `dead_routes`, `migration_progress` to the MCP server. After this, agents can ask "is this route called in production?"

5. **Semantic PR diff.** The most visible developer feature. Requires snapshot diff (done) and a GitHub Action wrapper.

## Parallelization Notes

When using agentic workflows (polywave or similar), the following can be implemented simultaneously:

**Now (Graph Core complete, trace pipeline complete):**
- HTTP route edges + route_symbols population
- OTLP gRPC receiver
- More language extractors (TypeScript, Rust, Java)
- Semantic PR diff
- `knowing export` CLI
- MCP runtime tools (once route_symbols has data)

**After HTTP route edges:**
- Daemon trace wiring (needs real receiver + populated route_symbols)

**After ownership edges:**
- Ownership routing

**After snapshot chain work:**
- Temporal reasoning, federated graphs (parallel)
