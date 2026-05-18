# Runtime Trace Ingestion

The runtime trace ingestion subsystem creates graph edges from production observability data. It bridges the gap between static analysis (what the code declares) and runtime behavior (what the code actually does in production). Runtime edges coexist with static edges in the same SQLite database and the same graph pipeline, distinguished by their `otel_trace` provenance prefix.

## Pipeline

```
OTel-instrumented services
        │
        ▼
┌───────────────────────────────────────────────────────┐
│ OTLPReceiver (gRPC server, OTLP trace protocol)       │
│   Listens on configurable endpoint (default :4317)    │
│   Implements coltracepb.TraceServiceServer             │
│   Receives ExportTraceServiceRequest messages          │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ Span Normalization                                     │
│   Extracts service.name from Resource attributes       │
│   Converts OTLP Span proto to internal TraceSpan      │
│   Extracts: TraceID, SpanID, ServiceName, Attributes   │
│   Extracts peer.service for cross-service edges        │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ Batch Accumulation (AddToBatch)                        │
│   Spans buffered in memory (mutex-protected slice)     │
│   Auto-flush when batch reaches configured BatchSize   │
│   Periodic flush on BatchInterval ticker               │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ Symbol Resolution (SymbolResolver.ResolveSpan)         │
│   Source: ComputeNodeHash from span.ServiceName        │
│   Target: resolve from span attributes:                │
│     http.method + http.route  → http_route lookup      │
│     rpc.service + rpc.method  → grpc_method lookup     │
│   Queries route_symbols table for target node hash     │
│   Falls back to synthetic unresolved node (conf 0.3)   │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ Edge Creation / Deduplication                          │
│   Edge hash: sha256(source + target + type + "otel_trace") │
│   If edge exists: increment observation_count,         │
│     update last_observed, recompute confidence          │
│   If new: INSERT edge + record "added" edge event      │
│   Provenance: "otel_trace:{trace_ids:[...]}"           │
└───────────────────────────────────────────────────────┘
```

## Edge Type Classification

The ingestor determines edge type from span attributes:

| Attributes present | Edge type |
|-------------------|-----------|
| `http.method` | `runtime_calls` |
| `rpc.service` | `runtime_rpc` |
| `messaging.system` + `messaging.destination` | `runtime_produces` |
| `messaging.system` (no destination) | `runtime_consumes` |
| (default) | `runtime_calls` |

## Confidence Scoring

Runtime edge confidence is computed from two factors: observation volume and recency. The `ComputeConfidence` function combines both.

**Observation-based scoring (within last 7 days):**

| Observation count | Confidence |
|------------------|------------|
| > 1000 | 0.95 |
| 100 - 1000 | 0.85 |
| 10 - 99 | 0.7 |
| 1 - 9 | 0.5 |
| 0 | 0.2 |

**Time-based decay:**

| Days since last observed | Effect |
|-------------------------|--------|
| 0 - 7 | Active; confidence from observation count |
| 8 - 30 | Recent; confidence from observation count |
| 31 - 90 | Stale; confidence forced to 0.2 |
| > 90 | GC-eligible; confidence 0.0 |

The daemon runs `DecayConfidence` hourly. This updates all `otel_`-provenance edges that have not been observed in 30+ days, setting their confidence to 0.2. Edges not observed in 90+ days are candidates for garbage collection.

**Decay brackets (diagnostic labels):**

| Bracket | Days since last observed |
|---------|------------------------|
| `active` | 0 - 7 |
| `recent` | 8 - 30 |
| `stale` | 31 - 90 |
| `gc_eligible` | > 90 |

## Symbol Resolution

The `SymbolResolver` connects runtime identifiers (HTTP routes, gRPC methods) to graph nodes using the `route_symbols` table. This table is populated during static indexing by the HTTP route extraction pass (see "HTTP Route Extraction" in [System Overview](system-overview.md)).

**Resolution flow:**

```
Span attributes → (service_name, route_pattern, mapping_type)
    │
    ▼
route_symbols table lookup (composite PK: service_name + route_pattern + mapping_type)
    │
    ├── Found: return node_hash with confidence 1.0
    └── Not found: return synthetic hash (ComputeNodeHash with "UNRESOLVED" package)
                   with confidence 0.3
```

**Source resolution:** The source hash is always a synthetic service node computed from `span.ServiceName`. This represents the calling service, not a specific function.

**Target resolution:** The target is resolved via `route_symbols` using the peer service name (or the span's own service if no peer). The mapping type is determined from span attributes: `http_route` for HTTP calls, `grpc_method` for gRPC calls, `unknown` for unrecognized patterns.

## Edge Deduplication

Runtime edges are deduplicated by their hash. The edge hash uses `"otel_trace"` as a fixed provenance string (not the specific trace ID), so the same source-target-type relationship always maps to the same hash regardless of which trace sampled it.

When a duplicate edge arrives:
- `observation_count` is incremented
- `last_observed` is updated to the current timestamp
- `confidence` is recomputed from the new count and zero days since observation

This means high-traffic routes accumulate higher confidence over time, while low-traffic routes remain at lower confidence until enough observations arrive.

## Batch Accumulation

The `Ingestor` supports two ingestion modes:

1. **Direct:** `IngestSpans` processes a slice of spans immediately.
2. **Batched:** `AddToBatch` appends spans to a pending slice (mutex-protected). The batch is flushed when it reaches `BatchSize` (auto-flush) or when the daemon's `BatchInterval` ticker fires.

The batch pattern avoids per-span database writes during high-throughput ingestion. The `OTLPReceiver.Export` method uses `AddToBatch` for each span in an OTLP request, letting the ingestor accumulate spans across multiple gRPC calls before flushing to the database.

## HTTP Log Ingestion

The ingestor also accepts HTTP access log entries via `IngestHTTPLogs`. Each `HTTPLogEntry` is converted to a `TraceSpan` with `http.method` and `http.route` attributes, then delegated to `IngestSpans`. This provides an ingestion path for environments that do not use OTel tracing but do produce standard HTTP access logs.

## Runtime and Static Edge Coexistence

Runtime edges and static edges share the same `edges` table. They are distinguished by provenance: static edges carry `ast_inferred`, `lsp_resolved`, or `ast_resolved` provenance; runtime edges carry `otel_trace` provenance. This design means:

- All graph queries (blast radius, transitive callers, dataflow tracing) automatically include runtime edges alongside static edges.
- The `observation_count` and `last_observed` columns (added by migration 004) default to 0 for static edges, which do not use observation-based scoring.
- Runtime edge statistics are computed by filtering on `provenance LIKE 'otel_%'`.
