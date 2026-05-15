# Runtime Trace Ingestion

Design document for ingesting production observability data (OpenTelemetry traces, gRPC metadata, HTTP access logs, message queue traces, database query logs) as first-class edges in the knowledge graph.

## Why This Matters

Static analysis tells you what the code says. Runtime traces tell you what the system does. The gap between these two is where false positives live, migrations stall, and incident response guesses.

| Question | Static analysis answer | Runtime trace answer |
|----------|----------------------|---------------------|
| "Can we deprecate this proto field?" | 12 services reference it | 2 services read it, 0 write it, last write 47 days ago |
| "How far is our migration from A to B?" | 40 callers of A, 15 callers of B | A: 200 req/s, B: 1,800 req/s (90% migrated) |
| "Is this route dead?" | 3 clients construct requests to it | 14,000 requests/day |
| "Which services actually talk to each other?" | 847 import edges | 124 active paths, 723 dead imports |

No other code intelligence tool bridges this gap. Runtime trace ingestion is the feature that makes knowing's graph unique.

## How It Works

### Data Sources

| Source | Protocol | What it provides | Edge type |
|--------|----------|-----------------|-----------|
| OpenTelemetry spans | OTLP (gRPC or HTTP) | Service-to-service calls with timing | `runtime_calls` |
| gRPC trace metadata | OTLP or custom exporter | RPC method calls between services | `runtime_rpc` |
| Message queue traces | OTLP span attributes or log lines | Topic producers and consumers | `runtime_produces`, `runtime_consumes` |
| HTTP access logs | Log file or log aggregator API | Route-level traffic patterns | `runtime_http` |
| Database query logs | Slow query log or APM integration | Service-to-table relationships | `runtime_queries` |

The primary source is OpenTelemetry. OTel is the industry standard for distributed tracing; most organizations already have a collector running. knowing taps into the existing collector, not individual services. No application code changes required.

### Pipeline Architecture

```
Production Services
    │ (emit OTel spans as part of normal operation)
    ▼
OTel Collector (already deployed in most organizations)
    │
    ├── Existing backends (Jaeger, Tempo, Datadog, etc.)
    │
    └── knowing trace ingest pipeline (new OTLP exporter endpoint)
            │
            ├── 1. Normalize spans into source/target pairs
            │      Extract: source service, target service, operation name,
            │      HTTP method/route, gRPC method, message topic
            │
            ├── 2. Resolve runtime identifiers to graph symbols
            │      Map "auth-service POST /api/v2/users" to
            │      graph node "auth-service://internal/api.UserHandler.Create"
            │      Using the route-to-symbol mapping table built during indexing
            │
            ├── 3. Aggregate observations
            │      Count occurrences per (source, target, operation) per time window
            │      Compute confidence from observation volume
            │
            ├── 4. Create or update edges
            │      GraphStore.PutEdge with provenance "otel_trace"
            │      RecordEdgeEvent for the append-only log
            │
            └── 5. Update snapshot
                   New snapshot includes runtime edges
                   SnapshotDiff shows what changed since last ingest
```

### Symbol Resolution (the hard part)

A trace span says: "service `auth-service` called `POST /api/v2/users` on service `user-service`."

The graph stores: `user-service://internal/api.UserHandler.Create`

Connecting these requires a mapping from runtime identifiers (service names, route paths, RPC method names) to graph node hashes.

**How the mapping is built:**

During static indexing, when the Go extractor parses a route registration:
```go
router.POST("/api/v2/users", handler.Create)
```

It records a mapping:
```
runtime_id: "POST /api/v2/users"
node_hash:  hash("user-service://internal/api.UserHandler.Create")
service:    "user-service"
```

This mapping table lives in SQLite alongside the graph:

```sql
CREATE TABLE route_symbols (
    service_name  TEXT NOT NULL,
    route_pattern TEXT NOT NULL,    -- "POST /api/v2/users", "UserService.GetUser"
    node_hash     BLOB NOT NULL,
    mapping_type  TEXT NOT NULL,    -- "http_route", "grpc_method", "queue_topic"
    created_at    INTEGER NOT NULL,
    PRIMARY KEY (service_name, route_pattern, mapping_type)
);
```

**When no mapping exists:**

If the trace references a route that isn't in the mapping table (dynamically registered, service not indexed, or route pattern mismatch), the edge is created with:
- `provenance: "runtime_unresolved"`
- `confidence: 0.3`
- Target stored as a synthetic node: `service-name://UNRESOLVED/route-pattern`

The edge is still useful ("something calls this endpoint") but flagged for manual resolution or re-indexing.

### Confidence Scoring

Runtime edge confidence is based on observation volume, not static analysis correctness. More observations mean higher confidence that the relationship is real and active.

| Condition | Confidence | Meaning |
|-----------|-----------|---------|
| > 1,000 observations in the last 7 days | 0.95 | High-traffic active relationship |
| 100-1,000 observations in the last 7 days | 0.85 | Regular active relationship |
| 10-100 observations in the last 7 days | 0.7 | Low-traffic but active |
| < 10 observations in the last 7 days | 0.5 | Minimal traffic, may be incidental |
| No observations in the last 30 days | 0.2 | Stale; edge still exists but traffic stopped |
| No observations in the last 90 days | Edge eligible for GC | Relationship is likely dead |

Confidence decays over time. An edge that was 0.95 last week becomes 0.85 this week if traffic drops, and eventually 0.2 if traffic stops entirely. This decay is a background process that runs periodically and updates edge provenance.

### Edge Provenance

Runtime edges use the same provenance model as static edges, with additional fields for observation data:

```json
{
  "source": "otel_trace",
  "confidence": 0.95,
  "sample_count": 14000,
  "first_seen": "2026-05-01T00:00:00Z",
  "last_seen": "2026-05-14T12:00:00Z",
  "trace_ids": ["abc123", "def456"],
  "indexer_version": "0.3.0"
}
```

`sample_count` is the total observations in the current time window. `first_seen` and `last_seen` track when the relationship was first and last observed. `trace_ids` are a small sample (2-5) of actual trace IDs for debugging and verification.

### Integration with Existing Pipeline

Runtime edges flow through the same pipeline as static edges:

- **Storage:** `GraphStore.PutEdge` (same method, different provenance)
- **Event log:** `RecordEdgeEvent` with "added"/"removed" events
- **Snapshots:** Included in Merkle root computation
- **Diffing:** `SnapshotDiff` shows runtime edges added/removed between snapshots
- **Staleness:** Content hash comparison applies (if observation data changes, edge hash changes)
- **Queries:** MCP tools return runtime edges alongside static edges, filterable by provenance

No new storage tables needed for edges (they're just edges). The `route_symbols` table is new. The `TraceIngestor` interface is new. Everything else is existing infrastructure.

### TraceIngestor Interface

```go
// TraceIngestor converts raw observability data into graph edges.
type TraceIngestor interface {
    // IngestSpans processes a batch of OpenTelemetry spans and creates
    // runtime edges. Returns the number of new edges created and the
    // number of existing edges whose observation counts were updated.
    IngestSpans(ctx context.Context, spans []TraceSpan) (created, updated int, err error)

    // IngestHTTPLogs processes access log entries.
    IngestHTTPLogs(ctx context.Context, entries []HTTPLogEntry) (created, updated int, err error)

    // RuntimeEdgeStats returns aggregated statistics for runtime edges:
    // total count, breakdown by source type, staleness distribution.
    RuntimeEdgeStats(ctx context.Context, snapshot Hash) (*RuntimeStats, error)

    // DecayConfidence reduces confidence for runtime edges that haven't
    // been observed recently. Called periodically by the daemon.
    DecayConfidence(ctx context.Context) (updated int, err error)
}

// TraceSpan is a normalized representation of a single span from any
// tracing system. The ingest pipeline normalizes vendor-specific formats
// into this before processing.
type TraceSpan struct {
    TraceID       string
    SpanID        string
    ParentSpanID  string
    ServiceName   string            // source service
    OperationName string            // RPC method, HTTP route, queue topic
    PeerService   string            // target service (if known)
    Attributes    map[string]string // http.method, http.route, rpc.service, etc.
    StartTime     time.Time
    Duration      time.Duration
}

// HTTPLogEntry represents a single HTTP access log line.
type HTTPLogEntry struct {
    Timestamp   time.Time
    Method      string // GET, POST, etc.
    Path        string // /api/v2/users
    StatusCode  int
    ServiceName string // the service that served the request
    ClientIP    string // or client service name if available
    Duration    time.Duration
}
```

### Daemon Integration

The daemon runs the trace ingest pipeline as a background goroutine alongside the git watcher and index worker:

```
knowing daemon
  ├── GitWatcher (watches .git/HEAD, triggers indexing)
  ├── IndexWorker (processes index requests, holds write lock)
  ├── TraceIngestor (connects to OTel collector, ingests spans)
  │   ├── Runs continuously, not triggered by commits
  │   ├── Batches spans (e.g., every 10 seconds or 1,000 spans)
  │   ├── Acquires write lock briefly for edge writes
  │   └── Confidence decay runs every hour
  ├── MCP Server (serves queries, holds read lock)
  └── Enrichment (background LSP, no lock)
```

The trace ingestor connects to the OTel collector's OTLP export endpoint on startup and remains connected. It processes spans in batches to minimize write lock contention with the index worker.

### Configuration

```yaml
# knowing daemon config (future)
trace_ingestion:
  enabled: true
  otlp_endpoint: "localhost:4317"    # OTel collector gRPC endpoint
  batch_size: 1000                   # spans per batch
  batch_interval: "10s"              # max time between batches
  confidence_decay_interval: "1h"    # how often to decay stale edges
  gc_threshold_days: 90              # edges with no observations older than this are GC'd
  
  # Service name mapping (if OTel service names don't match repo names)
  service_map:
    "auth-svc": "auth-service"       # OTel name -> knowing repo name
    "user-api": "user-service"
```

### MCP Tool Changes

Existing MCP tools gain runtime awareness:

| Tool | What changes |
|------|-------------|
| `blast_radius` | Returns runtime callers alongside static callers, with observation counts |
| `cross_repo_callers` | Includes runtime cross-service edges |
| `stale_edges` | Distinguishes static staleness (code hash mismatch) from runtime staleness (no recent observations) |
| `ownership` | Shows runtime traffic patterns per team's services |
| `graph_query` | Supports filtering by provenance type (static, runtime, or both) |

New MCP tools:

| Tool | Purpose |
|------|---------|
| `runtime_traffic` | Active traffic patterns between services for a time window |
| `dead_routes` | Routes with static declarations but zero runtime observations |
| `migration_progress` | Compare static callers vs runtime traffic for old/new service pairs |

### What This Does NOT Do

- Does not modify application code. Services emit traces to their existing OTel collector; knowing taps the collector.
- Does not replace observability tools (Grafana, Datadog, etc.). knowing creates graph edges from traces; it does not store raw traces, render dashboards, or alert on metrics.
- Does not require all services to be instrumented. Uninstrumented services simply have no runtime edges. Static edges still exist.
- Does not guarantee completeness. Trace sampling means some calls are not observed. Confidence scoring accounts for this: low observation counts produce lower confidence.

### Implementation Order

1. **Route-to-symbol mapping** (during static indexing): record HTTP route registrations, gRPC method declarations, and queue topic declarations as they're extracted. Store in `route_symbols` table. This should happen first because it's needed before any trace can be resolved.

2. **TraceIngestor implementation**: OTLP client connection, span normalization, symbol resolution via route_symbols, edge creation with observation-based confidence.

3. **Daemon integration**: background goroutine, batch processing, write lock coordination with index worker.

4. **Confidence decay**: periodic background job that reduces confidence for edges without recent observations.

5. **MCP tool updates**: add runtime traffic to blast_radius, cross_repo_callers, stale_edges. Add new tools (runtime_traffic, dead_routes, migration_progress).

6. **Testing**: integration test with a mock OTel collector that emits known spans, verifying edges are created with correct provenance and confidence.

### Edge Hash Computation for Runtime Edges

Runtime edges use the same `ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)` formula as static edges. The provenance string for runtime edges is `"otel_trace"`, which means the same source calling the same target produces different hashes for the static edge (`"lsp_resolved"`) and the runtime edge (`"otel_trace"`). This is intentional: they are different edges from different evidence sources. Both coexist in the graph.

**Observation updates do not change the edge hash.** When a new batch of spans confirms an existing runtime edge, the edge hash stays the same (same source, target, type, provenance). The observation metadata (sample_count, last_seen, confidence) is updated in place. This avoids creating a new edge and a new event on every batch. Edge events are recorded only when a runtime edge is first created ("added") or when it's garbage collected after 90 days with no observations ("removed").

### Observation Storage Model

The current Edge struct has `Confidence` (float64) and `Provenance` (string). Runtime edges need additional observation data. Two new columns on the edges table:

```sql
-- Migration 004
ALTER TABLE edges ADD COLUMN observation_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE edges ADD COLUMN last_observed INTEGER NOT NULL DEFAULT 0;  -- unix timestamp
```

The Edge struct gains corresponding fields:

```go
type Edge struct {
    // ... existing fields ...
    ObservationCount int   // total observations in current window (0 for static edges)
    LastObserved     int64 // unix timestamp of last observation (0 for static edges)
}
```

`Confidence` is recomputed from `ObservationCount` using the confidence scoring table. `first_seen` is derivable from `edge_events` (the timestamp of the "added" event). `trace_ids` (sample trace IDs for debugging) are stored in the `Provenance` field as JSON when the provenance source is `otel_trace`:

```json
"otel_trace:{\"trace_ids\":[\"abc123\",\"def456\"]}"
```

Static edges ignore both new fields (default 0). No behavioral change for existing code.

### Write Lock Contention

The index worker holds the write lock for 1.5-9 seconds during re-indexing. The trace ingestor also needs the write lock for edge writes. Contention model:

**Ingestor uses a separate write queue.** The ingestor does not compete with the index worker for the write lock directly. Instead:

1. Ingestor accumulates a batch of edge writes in memory (up to batch_size or batch_interval).
2. When the batch is ready, ingestor sends it to the index worker's write channel (same `indexCh` channel, different request type).
3. The index worker processes trace batches between index requests, holding the write lock briefly (~10ms for 1,000 edge upserts).

If the index worker is busy with a re-index, trace batches queue in the channel. The channel is buffered (128 slots). If the buffer fills (index worker is blocked for an extended period), the ingestor drops the oldest batch and logs a warning. Dropped batches reduce observation accuracy but don't lose edge relationships (the edge still exists from previous batches).

**Backpressure:** If the ingestor consistently can't flush batches fast enough (sustained high span volume + frequent re-indexes), it logs a metric and reduces batch frequency. No crash, no data corruption.

### Snapshot Strategy for Runtime Edges

Runtime edges do NOT trigger snapshot creation. Snapshots are commit-aligned only (architecture decision: snapshot chain mirrors git commit chain).

Runtime edges are included in the next snapshot that's created by a commit-triggered index. Between snapshots, runtime edges exist in the graph and are queryable, but they're not yet part of any snapshot's Merkle root.

This means:
- `SnapshotDiff` between two commit-aligned snapshots shows runtime edges that were added or removed between those commits.
- Runtime edges added between the latest snapshot and "now" are queryable via `EdgesFrom`/`EdgesTo` but not reflected in any snapshot hash until the next commit triggers a new snapshot.
- Confidence decay updates don't trigger snapshots either (they modify existing edges, not the edge set).

If a deployment-time snapshot is needed (e.g., "what was the runtime state at deploy?"), the `knowing snapshot` command (future) can create a manual snapshot that includes current runtime edges.

### Service Name to Repo URL Mapping

The `route_symbols` table needs `service_name`, but static indexing doesn't know what OTel service name a repo runs as. Three mapping strategies (prioritized):

**1. `knowing.yaml` in the repo (primary):**
```yaml
# knowing.yaml in the repo root
service_name: "auth-service"
routes:
  framework: "chi"  # optional: helps route pattern extraction
```

The indexer reads this file during `IndexRepo` and uses `service_name` for all route_symbols entries from this repo. This is explicit, versioned, and lives with the code.

**2. Config-level service_map (fallback):**
```yaml
# knowing daemon config
trace_ingestion:
  service_map:
    "auth-svc": "/Users/user/code/auth-service"
```

Maps OTel service names to repo paths. Used when `knowing.yaml` doesn't exist or when OTel service names differ from the convention.

**3. Heuristic inference (last resort):**

If no explicit mapping exists, the ingestor attempts to match the OTel service name against indexed repo names using substring matching. `"auth-service"` matches repo URL `*/auth-service`. Low confidence (0.3) on edges created via heuristic mapping.

### Framework-Specific Route Extraction

The tree-sitter extractor needs to recognize route registration patterns for each HTTP framework. This is a set of AST patterns per framework:

| Framework | Registration pattern | AST pattern |
|-----------|---------------------|-------------|
| net/http | `http.HandleFunc("/path", handler)` | `call_expression` where function is `HandleFunc` |
| chi | `r.Get("/path", handler)`, `r.Post(...)`, `r.Route(...)` | `call_expression` on method `Get`/`Post`/`Put`/`Delete`/`Route` |
| gin | `r.GET("/path", handler)`, `group.POST(...)` | `call_expression` on method `GET`/`POST`/`PUT`/`DELETE` |
| echo | `e.GET("/path", handler)` | Same pattern as gin |
| gorilla/mux | `r.HandleFunc("/path", handler).Methods("GET")` | `call_expression` chain with `HandleFunc` + `Methods` |
| gRPC | `pb.RegisterXxxServer(s, &server{})` | `call_expression` matching `Register*Server` |
| NATS/Kafka | `nc.Subscribe("topic", handler)`, `consumer.Subscribe("topic")` | `call_expression` on method `Subscribe`/`Publish` |

The extractor stores matched routes in the `route_symbols` table during indexing. Initially, support net/http, chi, and gRPC (covers most Go services). Other frameworks added as needed via the same pattern matching approach.

For non-Go languages, framework patterns are language-specific:
- Python: Flask (`@app.route`), FastAPI (`@app.get`), Django (`urlpatterns`)
- TypeScript: Express (`app.get`), NestJS (`@Get()` decorator)

Each tree-sitter extractor includes framework-specific route pattern matching for its language.

### Collector Disconnect Handling

The OTLP client connection uses gRPC with the following resilience model:

**Retry with exponential backoff:** On connection failure or disconnect, retry with backoff starting at 1 second, doubling up to 60 seconds. Use gRPC's built-in `keepalive` and `retry` policies.

**Health states:**

| State | Condition | Behavior |
|-------|-----------|----------|
| CONNECTED | Active gRPC stream | Normal span processing |
| RECONNECTING | Connection lost, retrying | Buffer spans in memory (up to 10,000), drop oldest if buffer fills |
| DISCONNECTED | Failed to reconnect after 5 minutes | Log error, stop buffering, enter polling mode (attempt reconnect every 60s) |
| DISABLED | User set `enabled: false` or endpoint is empty | No connection attempted |

**On reconnect:** Flush buffered spans. Do not replay spans from before the disconnect (they're likely already processed by other OTel exporters). Resume normal processing.

**Circuit breaker:** If 10 consecutive connection attempts fail, the ingestor enters DISCONNECTED state and stops attempting for 5 minutes before retrying. This prevents log spam and resource waste when the collector is down for maintenance.

### Data Volume and Capacity

Rough capacity estimates for SQLite write throughput:

| Span volume | Batches/hour | Edge writes/hour | SQLite feasibility |
|------------|-------------|-----------------|-------------------|
| 10K spans/hour | 10 | ~500 (deduplicated) | Trivial |
| 100K spans/hour | 100 | ~2,000 | Comfortable |
| 1M spans/hour | 1,000 | ~10,000 | Fine (SQLite handles 50K+ writes/sec) |
| 10M spans/hour | 10,000 | ~50,000 | Approaching limits; pre-aggregate in collector |
| 100M spans/hour | 100,000 | ~200,000 | Exceeds single-writer SQLite; requires pre-aggregation |

**Key insight:** Most spans are duplicate edges (the same service calling the same service on the same route). Deduplication reduces millions of spans to thousands of unique edges. The write volume is proportional to unique (source, target, operation) tuples, not raw span count.

**Pre-aggregation for high volume:** For organizations producing >10M spans/hour, configure the OTel collector to aggregate spans before exporting to knowing. The collector's `spanmetrics` connector or a custom processor can produce per-(source, target, operation) counts, reducing export volume by 100-1000x. knowing's ingestor accepts pre-aggregated data via the same OTLP endpoint.

### Artifact Boundary (Decision #15) Compliance

Runtime trace ingestion straddles the execution and intelligence planes:

| Operation | Plane | Rationale |
|-----------|-------|-----------|
| OTLP connection and span reception | Execution | Produces graph state (new edges) |
| Span normalization and symbol resolution | Execution | Transforms raw data into graph entities |
| Edge creation (PutEdge + RecordEdgeEvent) | Execution | Writes to the graph |
| Confidence decay | Execution | Modifies edge metadata (confidence, observation count) |
| `runtime_traffic` MCP tool | Intelligence | Read-only query over runtime edges |
| `dead_routes` MCP tool | Intelligence | Read-only comparison of static vs runtime edges |
| `migration_progress` MCP tool | Intelligence | Read-only analysis of traffic patterns |
| Confidence scoring (computing confidence from observation count) | Execution | Determines what gets written |

The bright-line rule holds: everything that writes to the graph is execution. Everything that reads and interprets is intelligence. Confidence decay is execution because it modifies edge data, even though it feels like analysis. The intelligence plane tools are strictly read-only; they cannot trigger decay, create edges, or modify observation counts.

### Migration Path for Existing Graphs

When trace ingestion is enabled on a graph that has only static edges:

**All existing static edges remain unchanged.** Runtime trace ingestion is additive. Static edges with `provenance: "lsp_resolved"` coexist with runtime edges with `provenance: "otel_trace"`.

**Absence of runtime data is not the same as zero traffic.** MCP tool responses must distinguish:

| State | Meaning | Display |
|-------|---------|---------|
| Runtime edge exists, high confidence | Active in production | "14,000 req/day (otel_trace, 0.95)" |
| Runtime edge exists, low confidence | Minimal or declining traffic | "3 req/day (otel_trace, 0.5)" |
| Runtime edge exists, stale | Traffic stopped | "Last observed 45 days ago (otel_trace, 0.2)" |
| No runtime edge, service instrumented | Zero traffic (confirmed dead) | "No runtime traffic (service instrumented)" |
| No runtime edge, service NOT instrumented | Unknown (no data) | "No runtime data (service not instrumented)" |

The difference between "confirmed dead" and "no data" depends on whether the source service is instrumented. If knowing has received ANY span from service X (stored in a `instrumented_services` table or derived from existing runtime edges), then the absence of a runtime edge for a specific call means zero traffic. If knowing has never seen a span from service X, the absence means unknown.

```sql
-- Derived view, not a new table
-- A service is "instrumented" if any runtime edge has it as a source
SELECT DISTINCT source_service FROM route_symbols rs
JOIN edges e ON e.source_hash = rs.node_hash
WHERE e.provenance LIKE 'otel_%'
```

### Dependencies

- `go.opentelemetry.io/proto/otlp` (OTLP protobuf definitions)
- `google.golang.org/grpc` (gRPC client for OTLP endpoint)
- No new CGo dependencies
- No changes to the GraphStore interface (runtime edges use existing PutEdge/RecordEdgeEvent)
- Route-to-symbol mapping requires a new migration (route_symbols table)
- Observation storage requires a new migration (observation_count, last_observed columns on edges)
- `knowing.yaml` file format needs specification
