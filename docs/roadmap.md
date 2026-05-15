# Roadmap

The system is designed as parallel workstreams, not sequential phases. The architecture (content-addressed storage, provenance model, storage interface, snapshot chain) supports all workstreams from day one. Implementation order is driven by dependency constraints, not cautious scoping.

## Workstream: Graph Core

Everything else depends on this. Must be solid before other workstreams begin.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Content-addressed store | SQLite backend behind `GraphStore` interface, Merkle DAG, snapshot chain | -- | planned |
| Go cross-repo call graph | `go/packages` type resolution, symbol identity scheme, cross-module edges | store | planned |
| Go package/module graph | Module dependency edges, import graph | store | planned |
| Traversal cache | L1 in-memory LRU, L2 materialized closures, L3 bounded traversal with early termination | store | planned |
| MCP server | `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `repo_graph`, `stale_edges`, `snapshot_diff`, `index_repo`, `graph_query` | store, call graph | planned |
| Daemon + file watcher | Persistent process, fsnotify/git hook triggers, incremental reindex on push | store, indexer | planned |

## Workstream: Edge Types

Parallelizable. Each edge type is independent and can be implemented by a separate agent. All require Graph Core.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| SCIP ingest | Tier 2 shallow indexing for external dependencies via SCIP indices | store | planned |
| Protobuf/gRPC edges | Proto field references, service-to-service RPC relationships | store | planned |
| HTTP route edges | Route producers (declarations) and consumers (client calls), route-to-symbol mappings | store | planned |
| Event edges | Kafka/NATS/SQS topic producers and consumers | store | planned |
| Schema edges | OpenAPI specs, JSON Schema, proto-as-schema references | store | planned |
| Infrastructure edges | Terraform service references, K8s manifest relationships, docker-compose links | store | planned |
| Ownership edges | CODEOWNERS, team annotations, service catalog metadata | store | planned |
| Multi-language support | tree-sitter parsers for Python, TypeScript, Java, Rust | store | planned |

## Workstream: Runtime Intelligence

Bridges the static/dynamic gap. Gives the graph production ground truth, not just static analysis.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Runtime symbol resolution | Map runtime identifiers (routes, service names, RPC methods) to graph nodes | HTTP route edges | planned |
| Trace ingestion pipeline | OpenTelemetry span ingest, gRPC trace metadata, HTTP access logs | runtime symbol resolution | planned |
| Runtime edge creation | `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes` edges with observation-based confidence | trace ingestion | planned |
| Confidence decay | Edge confidence degrades without re-confirmation; drives reindex priority | edge provenance model | planned |
| Database query edges | Ingest DB query logs as `runtime_queries` edges to schema nodes | trace ingestion | planned |

## Workstream: Developer Visibility

The features developers see. These make knowing's value obvious without requiring workflow changes.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Semantic PR diff | Relationship-level impact diff on every PR (MCP tool + GitHub Action) | snapshot chain, SnapshotDiff | planned |
| Graph-native test selection | `knowing test-scope` computes exact affected tests from the relationship graph | call graph, traversal | planned |
| Ownership routing | "Who do I need to notify about this change?" computed from graph edges | ownership edges | planned |
| Staleness dashboard | Surface edges and subgraphs that haven't been re-verified recently | snapshot chain, confidence | planned |

## Workstream: Agent Coordination

Turns knowing from a query layer into a coordination layer for multi-agent workflows.

| Item | Description | Depends on | Status |
|------|-------------|------------|--------|
| Pending mutations | Agents announce in-flight changes; other agents see proposed state alongside current state | MCP server | planned |
| Temporal reasoning | Walk backward through snapshots to find when a cross-repo incompatibility was introduced | snapshot chain, SnapshotDiff | planned |
| Federated graphs | Independent knowing instances with cross-federation queries via Merkle diff exchange | snapshot chain, Merkle sync | planned |

## Dependency Graph

```
Graph Core ──────────────────────────────> all other workstreams
HTTP route edges (route-to-symbol map) ──> Runtime symbol resolution
Runtime symbol resolution ────────────────> Trace ingestion pipeline
Trace ingestion pipeline ─────────────────> Runtime edge creation
Trace ingestion pipeline ─────────────────> Database query edges
Edge provenance model ────────────────────> Confidence decay
Snapshot chain + SnapshotDiff ────────────> Semantic PR diff
Snapshot chain + SnapshotDiff ────────────> Temporal reasoning
Call graph + traversal ───────────────────> Graph-native test selection
Ownership edges ──────────────────────────> Ownership routing
MCP server ───────────────────────────────> Pending mutations
Snapshot chain + Merkle sync ─────────────> Federated graphs
```

Everything below Graph Core can run in parallel once the core is solid.

## Parallelization Notes

When using agentic workflows (polywave or similar), the following can be implemented simultaneously:

**After Graph Core is complete:**
- All eight edge types (SCIP, proto, HTTP, events, schemas, infrastructure, ownership, multi-language)
- Semantic PR diff
- Graph-native test selection
- Pending mutations

**After HTTP route edges are complete:**
- Runtime symbol resolution, then trace ingestion pipeline (sequential within this chain)

**After ownership edges are complete:**
- Ownership routing

**After trace ingestion is complete:**
- Runtime edge creation, database query edges, confidence decay (all parallel)

**After snapshot chain is complete:**
- Temporal reasoning, federated graphs (parallel)
