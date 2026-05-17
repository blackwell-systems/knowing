# Edge Types Reference

This document catalogs every edge type in the knowing knowledge graph: what it
represents, which extractors produce it, its provenance tier, and how it
participates in blast radius traversal and context ranking.

## Summary Table

| Edge Type | Meaning | Provenance | Confidence | Producers | Blast Radius | RWR Weight |
|---|---|---|---|---|---|---|
| `calls` | Function/method invocation | ast_inferred / lsp_resolved | 0.7 / 0.9 | All 12 extractors, enricher | Yes (traversed) | 1.0 |
| `imports` | Module/package import | ast_inferred | 0.7 | All 12 extractors | No | 0.5 |
| `implements` | Type satisfies an interface | ast_inferred / lsp_resolved | 0.7 / 0.9 | Go extractor, enricher | No | 0.8 |
| `handles_route` | HTTP handler bound to a route | ast_inferred | 0.7 | Go, TS, Python, Rust, Java, C# extractors | No | 0.7 |
| `references` | Non-call identifier usage | ast_inferred / lsp_resolved / scip_resolved | 0.7 / 0.9 / 0.95 | Go extractor, Proto extractor, SQL extractor, SCIP ingestor, enricher | No | 0.4 |
| `depends_on` | Resource/symbol dependency | ast_inferred | 0.7 | Terraform, SQL, CSS extractors | No | 0.5 |
| `deploys` | K8s Service routes to Deployment | ast_inferred | 0.7 | K8s YAML extractor | No | 0.5 |
| `exposes` | K8s Ingress exposes Service | ast_inferred | 0.7 | K8s YAML extractor | No | 0.5 |
| `configures` | ConfigMap/Secret provides config | ast_inferred | 0.7 | K8s YAML extractor | No | 0.5 |
| `publishes` | Producer publishes to a topic/queue | ast_inferred | 0.7 | Cloud extractor (Serverless, CFN) | No | 0.5 |
| `subscribes` | Consumer subscribes to a topic/queue | ast_inferred | 0.7 | Cloud extractor (Serverless, CFN) | No | 0.5 |
| `connects_to` | Service connects to another service/network | ast_inferred | 0.7 | Cloud extractor (Docker Compose) | No | 0.5 |
| `runtime_calls` | HTTP call observed in traces | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `runtime_rpc` | gRPC/RPC call observed in traces | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `runtime_produces` | Message published to a topic | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `runtime_consumes` | Message consumed from a topic | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |

## Static Edge Types

### `calls`

A function or method invokes another function or method.

- **Direction:** source calls target. `pkg.HandleLogin -calls-> pkg.AuthService.Validate` means
  HandleLogin contains a call expression that resolves to AuthService.Validate.
- **Producers:** All 12 extractors (Go, TypeScript, Rust, Java, C#, Python,
  Terraform, SQL, Kubernetes YAML, Cloud YAML, CSS, Protocol Buffers) produce `calls` edges.
  The enricher upgrades ast_inferred calls to lsp_resolved when gopls confirms the definition.
- **Provenance:** `ast_inferred` (confidence 0.7) from tree-sitter extraction; `lsp_resolved`
  (confidence 0.9) after enrichment confirms the target via GetDefinition.
- **Blast radius:** This is the only edge type traversed by `TransitiveCallers`. The recursive
  CTE in `SQLiteStore.TransitiveCallers` walks `calls` edges backwards up to 5 levels deep.
  All blast radius computations (the `blast_radius` MCP tool, PR impact analysis, context
  ranking) depend exclusively on `calls` edges.
- **RWR weight:** 1.0 (highest). In the Random Walk with Restart algorithm, `calls` edges
  receive the maximum transition probability, making call-graph neighbors the strongest
  signal for context relevance.
- **Call-site metadata:** Every `calls` edge records the exact source location (file, line,
  column) of the call expression. The enricher uses this position to query gopls for
  definition resolution.

### `imports`

A file imports a module or package.

- **Direction:** source imports target. `cmd/server/main.go -imports-> github.com/example/pkg`
  means the file declares an import of that package.
- **Producers:** All 12 extractors. For Go: import declarations. For TypeScript:
  `import` statements and `require()` calls. For Rust: `use` declarations. For Java:
  `import` declarations. For C#: `using` directives. For Python: `import` and
  `from ... import` statements. For Protocol Buffers: `import` statements. For CSS:
  `@import` rules. For Terraform, SQL, and K8s: module/dependency references.
- **Provenance:** `ast_inferred` (confidence 0.7). Import resolution is syntactic; tree-sitter
  can reliably parse import paths without type information.
- **Blast radius:** Not traversed. Import edges express file-level dependencies, not
  function-level call chains.
- **RWR weight:** 0.5. Import edges carry moderate weight in the random walk, reflecting that
  package-level proximity is relevant but weaker than direct call relationships.

### `implements`

A concrete type satisfies an interface contract.

- **Direction:** source implements target. `pkg.SQLiteStore -implements-> types.GraphStore`
  means SQLiteStore has methods matching the GraphStore interface.
- **Producers:** The Go type-checked extractor (`goextractor`) discovers `implements` edges
  by checking whether concrete types in the same package satisfy declared interfaces. The
  LSP enricher also discovers `implements` edges via `GetImplementation` queries for
  interface symbols.
- **Provenance:** `ast_inferred` (confidence 0.7) from the Go extractor's method-set
  comparison; `lsp_resolved` (confidence 0.9) when discovered by the enricher through gopls.
- **Blast radius:** Not traversed. Blast radius only follows `calls` edges. However,
  `implements` edges are valuable for understanding which concrete types back an interface
  when planning changes.
- **RWR weight:** 0.8. The second-highest weight after `calls`, reflecting that implementation
  relationships are structurally significant. A change to an interface method likely affects
  all implementors.

### `handles_route`

An HTTP handler function is bound to a specific route pattern.

- **Direction:** source (route node) handles target (handler function).
  `GET /api/users -handles_route-> api.GetUsersHandler` means that HTTP requests matching
  `GET /api/users` are dispatched to `GetUsersHandler`.
- **Producers:** Six language extractors with web framework detection:
  - Go: `http.HandleFunc`, `mux.Handle`, gorilla/chi patterns, gin, echo
  - TypeScript: Express.js `app.get()`, Fastify, Hono, NestJS decorators, Next.js App Router
  - Python: Flask/FastAPI `@app.get()`/`@router.post()` decorator parsing, Django `path()`/`re_path()`
  - Rust: Actix-web `web::resource`, Axum `Router::route`, Rocket attribute macros
  - Java: Spring `@GetMapping`, `@PostMapping`, `@RequestMapping`, JAX-RS `@Path`/`@GET`
  - C#: ASP.NET `[HttpGet]`, `[HttpPost]`, `[Route]` attributes, minimal API `app.Map*`
- **Provenance:** `ast_inferred` (confidence 0.7). Route detection is pattern-based from
  tree-sitter AST nodes.
- **Blast radius:** Not traversed directly. However, `handles_route` edges connect runtime
  trace data (which resolves to route patterns) to static handler functions, bridging the
  gap between runtime observations and the static call graph.
- **RWR weight:** 0.7. High enough to surface route handlers as relevant context when the
  walk reaches HTTP-related code.

### `references`

A non-call usage of an identifier (type annotations, variable reads, constant references).

- **Direction:** source file references target symbol. `cmd/main.go -references-> types.Hash`
  means the file uses the `Hash` type without calling it.
- **Producers:** The Go type-checked extractor (`goextractor`) emits `references` edges for
  identifier usages that are not call expressions. Call targets already receive `calls` edges,
  so the extractor explicitly excludes call positions to avoid redundant edges. The Protocol
  Buffers extractor (`protoextractor`) emits `references` edges from RPC methods to their
  request/response message types and from message fields to referenced message types. The SCIP
  ingestor (`internal/indexer/scipingest/`) emits `references` edges for all symbol references
  found in imported SCIP index files. The LSP enricher also discovers `references` via
  `GetReferences` queries for functions and methods.
- **Provenance:** `ast_inferred` (confidence 0.7) from the Go extractor; `scip_resolved`
  (confidence 0.95) from SCIP ingest; `lsp_resolved` (confidence 0.9) when discovered by the
  enricher.
- **Blast radius:** Not traversed. References indicate structural coupling but not execution
  flow.
- **RWR weight:** 0.4. The lowest weight among static edge types, reflecting that a type
  reference is weaker evidence of functional relevance than a call or implementation
  relationship.

### `depends_on`

A resource or symbol depends on another resource or symbol.

- **Direction:** source depends on target. `aws_instance.web -depends_on-> aws_vpc.main`
  means the web instance has an explicit dependency on the VPC.
- **Producers:** Three infrastructure extractors:
  - Terraform: explicit `depends_on` block references between resources
  - SQL: foreign key references, view-to-table dependencies, procedure-to-table dependencies
  - CSS: custom property (`var(--name)`) references to defining selectors
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5 (moderate, similar to imports).

### `deploys`

A Kubernetes Service routes traffic to a Deployment (selector match).

- **Direction:** source service deploys target deployment.
  `Service/api -deploys-> Deployment/api` means the service's selector matches the deployment's labels.
- **Producers:** K8s YAML extractor (selector-to-label matching).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5.

### `exposes`

A Kubernetes Ingress exposes a Service.

- **Direction:** source ingress exposes target service.
  `Ingress/api -exposes-> Service/api` means the ingress routes external traffic to the service.
- **Producers:** K8s YAML extractor (ingress backend references).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5.

### `configures`

A Kubernetes ConfigMap or Secret provides configuration to a Deployment.

- **Direction:** source configmap/secret configures target deployment.
  `ConfigMap/settings -configures-> Deployment/api` means the deployment mounts or references that config.
- **Producers:** K8s YAML extractor (volume mount and envFrom references).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5.

### `publishes`

A function or service publishes messages to a topic or queue.

- **Direction:** source publishes to target topic.
  `functions/processOrder -publishes-> topic/order-events` means the function sends messages to that topic.
- **Producers:** Cloud extractor (Serverless Framework event sources, CloudFormation/SAM SNS/SQS subscriptions).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5.

### `subscribes`

A function or service subscribes to (consumes from) a topic or queue.

- **Direction:** source subscribes to target topic.
  `functions/handleOrder -subscribes-> topic/order-events` means the function is triggered by messages on that topic.
- **Producers:** Cloud extractor (Serverless Framework SQS/SNS/Kafka event triggers, CloudFormation/SAM event source mappings).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5.

### `connects_to`

A service connects to another service or network resource.

- **Direction:** source connects to target.
  `service/api -connects_to-> service/redis` means the api service declares a dependency on redis (via `depends_on` or network membership).
- **Producers:** Cloud extractor (Docker Compose `depends_on` links and shared network membership).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5.

## Runtime Edge Types

Runtime edges are produced by the trace ingestor (`internal/trace/ingestor.go`) from
OpenTelemetry spans and HTTP log entries. Their edge type is determined by the
`edgeTypeFromAttributes` function, which inspects span attributes.

### `runtime_calls`

An HTTP call between services observed in production traces.

- **Direction:** source service calls target service endpoint.
  `user-service -runtime_calls-> order-service:POST /api/orders`.
- **Producers:** Trace ingestor. Assigned when the span contains an `http.method` attribute,
  or as the default when no other protocol is detected.
- **Provenance:** `otel_trace` with embedded trace IDs (up to 5).
- **Confidence:** 0.2 to 0.95, computed by `ComputeConfidence(observationCount, daysSinceLastObserved)`.
  See the Provenance Tiers section below.
- **Blast radius:** Not traversed by `TransitiveCallers` (which only follows `calls` edges).
  Runtime edges provide a separate signal used for recency scoring in context ranking.
- **RWR weight:** 0.3 (default for unknown/runtime edge types).

### `runtime_rpc`

A gRPC or RPC call between services observed in traces.

- **Direction:** source service calls target RPC service.
- **Producers:** Trace ingestor. Assigned when the span contains an `rpc.service` attribute.
- **Provenance:** `otel_trace`.
- **Confidence:** 0.2 to 0.95.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default).

### `runtime_produces`

A service publishes a message to a messaging topic.

- **Direction:** source service produces to target topic/queue.
- **Producers:** Trace ingestor. Assigned when the span has both `messaging.system` and
  `messaging.destination` attributes (e.g., Kafka topic, RabbitMQ queue).
- **Provenance:** `otel_trace`.
- **Confidence:** 0.2 to 0.95.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default).

### `runtime_consumes`

A service consumes messages from a messaging system.

- **Direction:** source service consumes from the messaging system.
- **Producers:** Trace ingestor. Assigned when the span has a `messaging.system` attribute
  but no `messaging.destination` (consumer side).
- **Provenance:** `otel_trace`.
- **Confidence:** 0.2 to 0.95.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default).

## Provenance Tiers

Provenance tracks how an edge was discovered and determines its confidence level.

| Provenance | Confidence | Source | Meaning |
|---|---|---|---|
| `ast_inferred` | 0.7 | Tree-sitter extractors | Edge inferred from AST structure without type resolution. Cross-package calls resolved heuristically from import aliases. |
| `lsp_resolved` | 0.9 | LSP enricher (gopls) | Edge confirmed by querying the language server's GetDefinition at the call site. The original ast_inferred edge is deleted and replaced. |
| `scip_resolved` | 0.95 | SCIP ingestor | Edge resolved from a SCIP index file. Near-full confidence; SCIP indexes are produced by compiler-grade tools with complete type information. |
| `ast_resolved` | 1.0 | Python extractor | Edge resolved with full confidence. (Python extractor uses this provenance, though cross-module targets may still be dangling.) |
| `otel_trace` | 0.2 - 0.95 | Trace ingestor | Edge observed in production runtime data. Confidence varies by observation count and recency. |

### Runtime confidence scoring

Runtime edge confidence is computed by `ComputeConfidence(observationCount, daysSinceLastObserved)`:

1. If the edge has not been observed in over 90 days: **0.0** (garbage-collection eligible)
2. If the edge has not been observed in over 30 days: **0.2** (stale)
3. Otherwise, confidence is based on observation count via `ConfidenceFromCount`:

| Observation Count | Confidence |
|---|---|
| > 1000 | 0.95 |
| >= 100 | 0.85 |
| >= 10 | 0.70 |
| >= 1 | 0.50 |
| 0 | 0.20 |

## Edge Lifecycle

### Creation

Edges are created during indexing. Each extractor walks the tree-sitter AST and emits edges
with provenance `ast_inferred` and confidence 0.7. Edge identity is determined by the
`EdgeHash`, which is computed from `(source_hash, target_hash, edge_type, provenance)`.
This means the same source-target pair can have multiple edges if the provenance differs
(e.g., both an ast_inferred and an otel_trace edge for the same call relationship).

### Upgrade via enrichment

After indexing, the enrichment pipeline (`internal/enrichment/enricher.go`) runs gopls to
upgrade edges:

1. **Open files** in gopls so it builds its workspace index.
2. **Upgrade call edges:** For each `ast_inferred` edge with call-site position data, query
   `GetDefinition` at `(file, line, column)`. If gopls returns a location, delete the old
   edge and insert a new one with provenance `lsp_resolved` and confidence 0.9. (Deletion
   is necessary because provenance is part of the edge hash.)
3. **Discover new edges:** For each file, retrieve document symbols and query
   `GetImplementation` (producing `implements` edges) and `GetReferences` (producing
   `references` edges) to find relationships that tree-sitter missed.

### Decay

Runtime edges decay over time based on how recently they were last observed:

- **Active** (last 7 days): full confidence from observation count
- **Recent** (8-30 days): confidence from observation count
- **Stale** (31-90 days): confidence drops to 0.2 regardless of count
- **GC-eligible** (> 90 days): confidence drops to 0.0

The `DecayBracket` function in `internal/trace/confidence.go` categorizes edges into these
buckets for diagnostics.

### Garbage collection

Edges become eligible for garbage collection when their confidence reaches 0.0 (not observed
for over 90 days). The `ShouldGarbageCollect` function checks whether
`time.Now() - lastObserved > gcThresholdDays * 86400`. The `RuntimeEdgeStats` method on
the trace ingestor reports `GCEligible` counts for monitoring.

Static edges (`ast_inferred`, `lsp_resolved`) are replaced on each re-index of the
containing file rather than decayed. When a file is re-indexed, edges from the old snapshot
that are not present in the new extraction result are effectively removed by the snapshot
management system.
