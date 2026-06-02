# Edge Types Reference

This document catalogs all 38 edge types in the knowing knowledge graph: what each
represents, which extractors produce it, its provenance tier, and how it
participates in blast radius traversal and context ranking.

## Summary Table

| Edge Type | Meaning | Provenance | Confidence | Producers | Blast Radius | RWR Weight |
|---|---|---|---|---|---|---|
| `calls` | Function/method invocation | ast_inferred / lsp_resolved | 0.7 / 0.9 | All 26 extractor packages, enricher | Yes (traversed) | 1.0 |
| `imports` | Module/package import | ast_inferred | 0.7 | All 26 extractor packages | No | 0.5 |
| `implements` | Type satisfies an interface | ast_inferred / lsp_resolved | 0.7 / 0.9 | Go, TS, Java, C#, Ruby, Rust, GraphQL extractors, enricher | No | 0.8 |
| `handles_route` | HTTP handler bound to a route | ast_inferred | 0.7 | Go, TS, Python, Ruby, Rust, Java, C# extractors | No | 0.7 |
| `references` | Non-call identifier usage | ast_inferred / lsp_resolved / scip_resolved | 0.7 / 0.9 / 0.95 | Go extractor, Proto extractor, GraphQL extractor, SQL extractor, SCIP ingestor, enricher | No | 0.4 |
| `depends_on` | Resource/symbol dependency | ast_inferred | 0.7 | Terraform, SQL, CSS, Dockerfile, Makefile, Helm, GitLab CI, package.json extractors | No | 0.5 |
| `deploys` | K8s Service routes to Deployment | ast_inferred | 0.7 | K8s YAML extractor | No | 0.3 (default) |
| `exposes` | K8s Ingress exposes Service | ast_inferred | 0.7 | K8s YAML extractor | No | 0.3 (default) |
| `configures` | ConfigMap/Secret provides config | ast_inferred | 0.7 | K8s YAML extractor | No | 0.3 (default) |
| `publishes` | Producer publishes to a topic/queue | ast_inferred | 0.7 | Cloud extractor (Serverless, CFN) | No | 0.3 (default) |
| `subscribes` | Consumer subscribes to a topic/queue | ast_inferred | 0.7 | Cloud extractor (Serverless, CFN) | No | 0.3 (default) |
| `connects_to` | Service connects to another service/network | ast_inferred | 0.7 | Cloud extractor (Docker Compose) | No | 0.3 (default) |
| `extends` | Class inheritance / template inheritance | ast_inferred | 0.7 | TS, Java, Python, C#, GitLab CI extractors | No | 0.7 |
| `overrides` | Method overrides parent | ast_inferred | 0.7 | TS, Java, C# extractors | No | 0.8 |
| `decorates` | Decorator/annotation applied | ast_inferred | 0.7 | TS, Java, Python, C#, Rust extractors | No | 0.3 |
| `throws` | Function throws/raises error | ast_inferred | 0.7 | Go, TS, Ruby extractors | No | 0.4 |
| `owned_by` | File/symbol owned by team/user | deterministic | 1.0 | CODEOWNERS parser | No | 0.0 |
| `tests` | Test function tests production function | ast_inferred | 0.7 | Go tree-sitter extractor | No | 0.6 |
| `authored_by` | Symbol primarily authored by person | git_blame | 1.0 | Authorship extractor (git blame) | No | 0.0 |
| `documents` | Doc comment documents a symbol | ast_inferred | 0.9 | Go tree-sitter extractor | No | 0.2 |
| `consumes_endpoint` | Code calls HTTP endpoint | ast_inferred | 0.6 | TS, Go tree-sitter extractors | No | 0.5 |
| `implements_rpc` | Struct implements gRPC service | ast_inferred | 0.9 | Proto extractor (Go source) | No | 0.8 |
| `consumes_rpc` | Code creates gRPC client | ast_inferred | 0.8 | Proto extractor (Go source) | No | 0.6 |
| `gated_by_flag` | Function gated by feature flag | ast_inferred | 0.8 | Go tree-sitter extractor | No | 0.3 |
| `deployed_by` | Binary/service deployed by workflow | ast_inferred | 0.9 | GitHub Actions extractor | No | 0.4 |
| `tested_by` | Package tested by CI workflow | ast_inferred | 0.8 | GitHub Actions extractor | No | 0.5 |
| `runtime_calls` | HTTP call observed in traces | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `runtime_rpc` | gRPC/RPC call observed in traces | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `runtime_produces` | Message published to a topic | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `runtime_consumes` | Message consumed from a topic | otel_trace | 0.2 - 0.95 | Trace ingestor | No | 0.3 (default) |
| `contains` | Type/class contains a method/field | structural | 1.0 | Indexer (QN hierarchy) | No | 0.8 |
| `member_of` | Method/field belongs to a type/class (reverse of contains) | structural | 1.0 | Indexer (QN hierarchy) | No | 0.6 |
| `co_tested_with` | Symbols referenced from the same test file | co_test_inference | 0.6 | Indexer (test file analysis) | No | 0.5 |
| `type_hint_of` | Function parameter type annotation | ast_inferred | 0.7 | Go, Java, TypeScript, Python extractors | No | 0.5 |

**Note on RWR weights:** The weights shown are base weights by edge type. Edges with
`lsp_resolved` provenance receive an additional 0.3x multiplier (session 25), reducing
their effective RWR weight to prevent enrichment from inflating framework wiring symbol
centrality. `contains` and `member_of` have weight 0.0 in the walk (excluded from BFS
frontier expansion); they are used by path seeding directly.

## Static Edge Types

### `calls`

A function or method invokes another function or method.

- **Direction:** source calls target. `pkg.HandleLogin -calls-> pkg.AuthService.Validate` means
  HandleLogin contains a call expression that resolves to AuthService.Validate.
- **Producers:** All 26 extractor packages (Go, TypeScript, Rust, Java, C#, Python,
  Terraform, SQL, Kubernetes YAML, Cloud YAML, CSS, Protocol Buffers, and tree-sitter generic extractors) produce `calls` edges.
  The enricher upgrades ast_inferred calls to lsp_resolved when the language server (gopls, pyright, tsserver, rust-analyzer, jdtls, or OmniSharp) confirms the definition.
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
  column) of the call expression. The enricher uses this position to query the language
  server for definition resolution.

### `imports`

A file imports a module or package.

- **Direction:** source imports target. `cmd/server/main.go -imports-> github.com/example/pkg`
  means the file declares an import of that package.
- **Producers:** All 26 extractor packages. For Go: import declarations. For TypeScript:
  `import` statements and `require()` calls. For Rust: `use` declarations. For Java:
  `import` declarations. For C#: `using` directives. For Python: `import` and
  `from ... import` statements. For Protocol Buffers: `import` statements. For CSS:
  `@import` rules. For Terraform, SQL, and K8s: module/dependency references. For
  Makefile: `include` directives.
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
- **Producers:** Six language extractors and the enricher:
  - Go: the type-checked extractor (`goextractor`) discovers `implements` edges by checking
    whether concrete types in the same package satisfy declared interfaces.
  - TypeScript: class `implements` clauses.
  - Java: class `implements` clauses.
  - C#: class/struct interface implementation declarations.
  - Rust: `impl Trait for Type` blocks.
  - GraphQL: `type Foo implements Bar` interface implementation declarations.
  - The LSP enricher also discovers `implements` edges via `GetImplementation` queries for
    interface symbols.
- **Provenance:** `ast_inferred` (confidence 0.7) from tree-sitter extraction or Go's
  method-set comparison; `lsp_resolved` (confidence 0.9) when discovered by the enricher
  through any supported language server (gopls, pyright, tsserver, rust-analyzer, jdtls, OmniSharp).
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
  request/response message types and from message fields to referenced message types. The
  GraphQL extractor emits `references` edges from fields to their type definitions and from
  query/mutation arguments to input types. The SCIP ingestor (`internal/indexer/scipingest/`)
  emits `references` edges for all symbol references found in imported SCIP index files. The
  LSP enricher also discovers `references` via `GetReferences` queries for functions and
  methods.
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
- **Producers:** Eight infrastructure extractors:
  - Terraform: explicit `depends_on` block references between resources
  - SQL: foreign key references, view-to-table dependencies, procedure-to-table dependencies
  - CSS: custom property (`var(--name)`) references to defining selectors
  - Dockerfile: `FROM` base image dependencies, `COPY --from` multi-stage build references
  - Makefile: target dependencies (prerequisite lists)
  - Helm: chart dependency declarations (`dependencies` in Chart.yaml)
  - GitLab CI: job `needs` dependencies between pipeline stages
  - package.json: npm `dependencies`, `devDependencies`, and `peerDependencies`
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
- **RWR weight:** 0.3 (default; not explicitly listed in the weight map).

### `exposes`

A Kubernetes Ingress exposes a Service.

- **Direction:** source ingress exposes target service.
  `Ingress/api -exposes-> Service/api` means the ingress routes external traffic to the service.
- **Producers:** K8s YAML extractor (ingress backend references).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default; not explicitly listed in the weight map).

### `configures`

A Kubernetes ConfigMap or Secret provides configuration to a Deployment.

- **Direction:** source configmap/secret configures target deployment.
  `ConfigMap/settings -configures-> Deployment/api` means the deployment mounts or references that config.
- **Producers:** K8s YAML extractor (volume mount and envFrom references).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default; not explicitly listed in the weight map).

### `publishes`

A function or service publishes messages to a topic or queue.

- **Direction:** source publishes to target topic.
  `functions/processOrder -publishes-> topic/order-events` means the function sends messages to that topic.
- **Producers:** Cloud extractor (Serverless Framework event sources, CloudFormation/SAM SNS/SQS subscriptions).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default; not explicitly listed in the weight map).

### `subscribes`

A function or service subscribes to (consumes from) a topic or queue.

- **Direction:** source subscribes to target topic.
  `functions/handleOrder -subscribes-> topic/order-events` means the function is triggered by messages on that topic.
- **Producers:** Cloud extractor (Serverless Framework SQS/SNS/Kafka event triggers, CloudFormation/SAM event source mappings).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default; not explicitly listed in the weight map).

### `connects_to`

A service connects to another service or network resource.

- **Direction:** source connects to target.
  `service/api -connects_to-> service/redis` means the api service declares a dependency on redis (via `depends_on` or network membership).
- **Producers:** Cloud extractor (Docker Compose `depends_on` links and shared network membership).
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3 (default; not explicitly listed in the weight map).

### `extends`

A class or type extends (inherits from) another class or type.

- **Direction:** source extends target. `components.AdminPanel -extends-> components.BasePanel`
  means AdminPanel inherits from BasePanel.
- **Producers:** Five language extractors:
  - TypeScript: class `extends` clauses.
  - Java: class `extends` clauses.
  - Python: class base classes in the class definition (via tree-sitter generic extractor).
  - C#: class inheritance declarations.
  - GitLab CI: job `extends: .template` directives (template inheritance).
- **Provenance:** `ast_inferred` (confidence 0.7). Inheritance is syntactically explicit and
  reliably detected from AST nodes.
- **Blast radius:** Not traversed. However, `extends` edges are valuable for understanding
  which classes inherit behavior when planning changes to a base class.
- **RWR weight:** 0.7. Reflects that inheritance relationships are structurally significant;
  a change to a base class method may affect all subclasses.

### `overrides`

A method overrides a method from a parent class.

- **Direction:** source overrides target. `AdminPanel.render -overrides-> BasePanel.render`
  means the child class provides its own implementation of the parent method.
- **Producers:** Three language extractors:
  - TypeScript: methods in a class that match a parent class method (detected via `override`
    keyword or inheritance analysis).
  - Java: methods annotated with `@Override`.
  - C#: methods declared with the `override` keyword.
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.8. High weight, matching `implements`, because changing a parent method's
  contract directly affects all overriding methods.

### `decorates`

A decorator or annotation is applied to a class, method, or function.

- **Direction:** source decorator decorates target symbol. `@Cache -decorates-> UserService.getUser`
  means the `@Cache` decorator wraps the `getUser` method.
- **Producers:** Five language extractors:
  - TypeScript: `@decorator` syntax on classes and methods.
  - Java: annotation declarations (`@Transactional`, `@Override`, etc.).
  - Python: `@decorator` syntax (via tree-sitter generic extractor).
  - C#: attribute syntax (`[Authorize]`, `[HttpGet]`, etc.).
  - Rust: `#[attribute]` and `#[derive(...)]` macros.
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3. Low weight because decorators are typically cross-cutting concerns
  (logging, caching, authorization) rather than direct functional dependencies.

### `throws`

A function or method throws or raises an error type.

- **Direction:** source function throws target error type. `api.HandleRequest -throws-> ErrNotFound`
  means HandleRequest contains a throw/return of the ErrNotFound error.
- **Producers:** Two language extractors:
  - Go: error returns detected from function bodies (sentinel errors, `fmt.Errorf`, custom
    error type construction).
  - TypeScript: `throw` statements with identifiable error types.
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.4. Moderate-low weight, similar to `references`. Knowing which errors a
  function throws is useful for error handling analysis but is a weaker signal for context
  relevance than call or inheritance relationships.

### `owned_by`

A file or symbol is owned by a team or individual (from CODEOWNERS).

- **Direction:** source file/path owned by target team/user.
  `internal/api/ -owned_by-> @backend-team` means the backend team owns files under that path.
- **Producers:** The ownership extractor (`internal/indexer/ownership/ownership.go`) parses
  CODEOWNERS files and emits `owned_by` edges for matched file patterns.
- **Provenance:** `deterministic` (confidence 1.0). CODEOWNERS rules are explicit declarations,
  not inferred relationships.
- **Blast radius:** Not traversed. Ownership is an organizational relationship, not a code
  dependency.
- **RWR weight:** 0.0. Ownership edges do not participate in the random walk. They serve a
  different purpose: identifying who should review changes, filtering context by team scope,
  and surfacing ownership in PR impact reports.

### `tests`

A test function exercises (calls into) a production function.

- **Direction:** source test function tests target production function.
  `TestHandleLogin -tests-> HandleLogin` means TestHandleLogin calls HandleLogin.
- **Producers:** Go tree-sitter extractor, from _test.go files.
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed. Tests edges make test coverage graph-queryable
  but do not affect blast radius computation (which follows calls edges only).
- **RWR weight:** 0.6. Moderate weight: a test function covering a production
  function is structurally relevant but weaker than a direct call or implementation.

### `authored_by`

A symbol is primarily authored by a person (determined via git blame).

- **Direction:** source symbol authored by target person.
  `SQLiteStore.PutNode -authored_by-> alice` means alice wrote most lines of PutNode.
- **Producers:** Authorship extractor (runs git blame, determines primary author per symbol).
- **Provenance:** `git_blame` (confidence 1.0, deterministic from git history).
- **Blast radius:** Not traversed. Authorship is organizational, not functional.
- **RWR weight:** 0.0. Like owned_by, authorship edges do not participate in the
  random walk. They serve ownership queries and team routing.

### `documents`

A doc comment documents a symbol declaration.

- **Direction:** source doc comment documents target symbol.
  `doc_comment:"HandleLogin validates credentials..." -documents-> api.HandleLogin` means
  the doc comment block describes the HandleLogin function.
- **Producers:** Go tree-sitter extractor. For each function, method, or type declaration
  with a preceding doc comment (detected by `extractDocComment`), creates a synthetic
  "doc_comment" node and a `documents` edge from it to the declaration.
- **Provenance:** `ast_inferred` (confidence 0.9). Doc comments are syntactically unambiguous.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.2. Low weight because documentation is informational context, not a
  structural dependency. However, doc comments are surfaced in context packs for LLM
  comprehension.

### `consumes_endpoint`

Code makes an HTTP client call to a specific endpoint URL path.

- **Direction:** source function consumes target endpoint.
  `api.FetchUser -consumes_endpoint-> GET /api/users/:id` means FetchUser contains an
  HTTP client call to that endpoint.
- **Producers:** Two language extractors:
  - Go: `http.Get("...")`, `http.Post("...")`, `client.Get("...")` patterns detected via tree-sitter.
  - TypeScript/JavaScript: `fetch("/api/...")`, `axios.get("/api/...")`, `http.get("/api/...")` patterns.
- **Provenance:** `ast_inferred` (confidence 0.6). URL paths extracted from string literals
  may be partial or templated.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5. Moderate weight; endpoint consumption indicates functional dependency
  between services or between frontend and backend.

### `implements_rpc`

A Go struct implements a gRPC service interface.

- **Direction:** source struct implements target RPC service.
  `pkg.UserServer -implements_rpc-> proto.UserService` means UserServer embeds
  `pb.UnimplementedUserServiceServer`.
- **Producers:** Go tree-sitter extractor via `ExtractRPCEdges`. Detects structs embedding
  `pb.Unimplemented*Server` patterns.
- **Provenance:** `ast_inferred` (confidence 0.9). The embedding pattern is highly specific.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.8. High weight (same as `implements`) because gRPC service implementation
  is a strong structural relationship; changes to the proto definition affect all implementors.

### `consumes_rpc`

Code creates a gRPC client connection to a service.

- **Direction:** source function consumes target RPC service.
  `api.CreateOrder -consumes_rpc-> proto.InventoryService` means CreateOrder calls
  `pb.NewInventoryServiceClient(conn)`.
- **Producers:** Go tree-sitter extractor via `ExtractRPCEdges`. Detects `pb.New*Client()`
  call patterns.
- **Provenance:** `ast_inferred` (confidence 0.8).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.6. Moderate-high weight; gRPC client usage indicates inter-service
  dependency that should surface in context when either side changes.

### `gated_by_flag`

A function is gated by a feature flag check.

- **Direction:** source function gated by target flag.
  `api.NewCheckout -gated_by_flag-> flag:enable-new-checkout` means NewCheckout contains
  a feature flag SDK call that checks the "enable-new-checkout" flag.
- **Producers:** Go tree-sitter extractor via `ExtractFeatureFlagEdges`. Detects patterns like
  `client.BoolVariation("flag-name")`, `unleash.IsEnabled("flag-name")`, and custom
  `IsFeatureEnabled("flag-name")` calls.
- **Provenance:** `ast_inferred` (confidence 0.8). Feature flag SDK patterns are distinctive.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.3. Low weight because feature flags are cross-cutting configuration
  concerns. However, these edges enable querying "what code is behind this flag?" for
  safe rollout analysis.

### `deployed_by`

A service or binary is deployed by a CI/CD workflow.

- **Direction:** source deployment target deployed by target workflow.
  `deployment:api-service -deployed_by-> .github/workflows/deploy.yml` means the workflow
  deploys the api-service.
- **Producers:** GitHub Actions extractor via `extractDeployedByEdges`. Detects deployment
  steps (docker push, kubectl apply, deploy actions like `aws-actions/amazon-ecs-deploy-task-definition`)
  and links detected targets to the workflow node.
- **Provenance:** `ast_inferred` (confidence 0.9). Deployment action patterns are specific.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.4. Moderate-low weight. Deployment relationships are operationally
  important but represent infrastructure-level coupling rather than code-level dependency.

### `tested_by`

A package or module is tested by a CI workflow job.

- **Direction:** source test target tested by target workflow job.
  `test_target:internal/store -tested_by-> job:test` means the test job runs
  `go test ./internal/store/...`.
- **Producers:** GitHub Actions extractor via `extractTestedByEdges`. Detects test commands
  (go test, npm test, pytest, cargo test) in workflow run steps and extracts package paths.
- **Provenance:** `ast_inferred` (confidence 0.8).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5. Moderate weight; knowing which CI job tests a package is useful for
  test-scope queries and understanding CI coverage.

### `contains`

A type or class contains a method or field.

- **Direction:** source type contains target method/field. `pkg.AuthService -contains-> pkg.AuthService.Validate`
  means the AuthService type declares the Validate method.
- **Producers:** The indexer (`internal/indexer/indexer.go`) via `generateContainsEdges()` during
  indexer post-processing. Connects type/class nodes to their methods and fields by analyzing
  qualified name (QN) hierarchy: if a node's QN is prefixed by a type's QN (e.g.,
  `pkg.Type.Method` starts with `pkg.Type`), a `contains` edge is emitted. For example, if
  `Foo.Bar` exists and `Foo` is a type node, then `Foo --contains--> Foo.Bar`. Also generated
  on-the-fly in the bench adapter (`bench/cross-system/adapters/knowing.go`) for evaluation
  without re-indexing.
- **Provenance:** `structural` (confidence 1.0). Containment is deterministic, derived purely
  from qualified name hierarchy, which is unambiguous.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.8. High weight reflecting that a type's methods are structurally related
  to the type. Enables path-context seeding to reach methods through their declaring types,
  bridging the gap between package-level concepts and implementation-level symbols.
- **Coverage:** Connected approximately 77% of previously-disconnected type/class nodes to their
  methods. Provides structural infrastructure for RWR to walk from type seeds to discover all
  member methods.

### `member_of`

A method or field belongs to a type or class (the reverse of `contains`).

- **Direction:** source method/field is a member of target type. `pkg.AuthService.Validate -member_of-> pkg.AuthService`
  means the Validate method belongs to the AuthService type.
- **Producers:** The indexer (`internal/indexer/indexer.go`) via `generateContainsEdges()` during
  indexer post-processing. For each `contains` edge emitted (type -> method/field), a
  corresponding `member_of` edge is emitted in the reverse direction (method/field -> type).
  Also generated in the bench adapter for evaluation.
- **Provenance:** `structural` (confidence 1.0). Deterministic from QN hierarchy, same as
  `contains`.
- **Blast radius:** Not traversed.
- **RWR weight:** 0.6. Moderate weight, slightly lower than `contains` (0.8). Enables RWR to walk
  from any matched method back to its parent type, then to sibling methods via outgoing
  `contains` edges. This bidirectional connectivity (contains + member_of) ensures that type
  hierarchies form tightly connected subgraphs in the random walk.

### `co_tested_with`

Two non-test symbols are referenced from the same test file.

- **Direction:** source co_tested_with target. `pkg.FuncA -co_tested_with-> pkg.FuncB`
  means both FuncA and FuncB are called or imported by the same test file.
- **Producers:** The indexer (`internal/indexer/indexer.go`) via `generateCoTestedEdges()`
  during post-processing. For each test file (detected by `IsTestFile()` across Go,
  Python, TypeScript, Rust, Java, C#), finds all non-test symbols referenced and creates
  lateral edges between them.
- **Provenance:** `co_test_inference` (confidence 0.6).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5. Moderate weight reflecting that co-tested symbols likely serve the
  same feature, bridging structurally disconnected symbols that share functional context.
- **Caps:** 20 targets per file, 20 pairs per file (prevents N^2 explosion on test files
  that reference many symbols).

### `type_hint_of`

A function parameter references a type via its type annotation.

- **Direction:** source function type_hint_of target type. `pkg.HandleRequest -type_hint_of-> pkg.AuthService`
  means HandleRequest has a parameter annotated with the AuthService type.
- **Producers:** Four language extractors:
  - Go: extracts from `parameter_declaration` nodes, resolves imported types via import map.
    Skips builtins (string, int, error, etc.).
  - Java: extracts from `formal_parameter` nodes, handles generics (`List<T>` -> `List`)
    and scoped types. Skips primitives and boxed types.
  - TypeScript: extracts from required/optional/rest parameters via `type_annotation`.
    Handles generics and nested type identifiers.
  - Python: extracts from `typed_parameter` nodes with import-map resolution.
- **Provenance:** `ast_inferred` (confidence 0.7).
- **Blast radius:** Not traversed.
- **RWR weight:** 0.5. Moderate weight: type annotations indicate structural dependency
  between functions and the types they operate on. Enables RWR to walk from functions
  to their parameter types and vice versa.

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
| `lsp_resolved` | 0.9 | LSP enricher (gopls, pyright, tsserver, rust-analyzer, jdtls, OmniSharp) | Edge confirmed by querying the language server's GetDefinition at the call site. The original ast_inferred edge is deleted and replaced. |
| `scip_resolved` | 0.95 | SCIP ingestor | Edge resolved from a SCIP index file. Near-full confidence; SCIP indexes are produced by compiler-grade tools with complete type information. |
| `ast_resolved` | 1.0 | Python extractor | Edge resolved with full confidence. (Python extractor uses this provenance, though cross-module targets may still be dangling.) |
| `structural` | 1.0 | Indexer (`generateContainsEdges`) | Edge derived from qualified name hierarchy. If a method's QN is prefixed by a type's QN, containment is certain. |
| `deterministic` | 1.0 | CODEOWNERS parser, authorship extractor | Edge derived from explicit configuration (CODEOWNERS rules) or git history. No inference involved. |
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

After indexing, the enrichment pipeline (`internal/enrichment/enricher.go`) runs each
detected language server (gopls, pyright, tsserver, rust-analyzer, jdtls, OmniSharp) to
upgrade edges:

1. **Open files** in the language server so it builds its workspace index. For async
   servers (e.g., jdtls), the enricher waits up to 120s for workspace readiness.
2. **Upgrade call edges:** For each `ast_inferred` edge with call-site position data, query
   `GetDefinition` at `(file, line, column)`. If the server returns a location, delete the
   old edge and insert a new one with provenance `lsp_resolved` and confidence 0.9.
   (Deletion is necessary because provenance is part of the edge hash.)
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
