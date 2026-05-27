# Tree-Sitter Extraction

The extraction system (`internal/indexer/`) is the foundation of the knowledge graph.
It parses source files using tree-sitter grammars to produce **nodes** (code symbols:
functions, types, classes, methods, fields) and **edges** (relationships: calls, imports,
implements, extends, type hints). Everything else in the system builds on the extracted
graph: [LSP enrichment](enrichment.md) upgrades confidence, the
[retrieval pipeline](retrieval-pipeline.md) walks edges to rank symbols, and
[Merkle proofs](merkle-proofs.md) attest to graph contents.

Extraction runs during `knowing index` and produces a complete, self-contained graph
from source code alone. No language server, no build system, no network access. A single
pass over the repository produces nodes and edges at `ast_inferred` confidence (0.7),
which is sufficient for RWR-based retrieval. LSP enrichment is optional and additive; the
tree-sitter graph is the authoritative baseline.

---

## Multi-Dispatch Architecture

Each file is processed by **all** matching extractors, not just the first. The
`ExtractorRegistry.FindAllExtractors` method returns every extractor whose `CanHandle`
returns true for a given file path.

This enables overlay extraction: a `.go` file is processed by the Go tree-sitter
extractor (functions, types, calls, imports) **and** the event extractor (Kafka/NATS
publish/subscribe patterns) **and** the env extractor (`os.Getenv` calls). A `.py` file
is processed by the Python extractor **and** the event extractor. Each extractor
contributes its own nodes and edges, and results are merged before storage.

When multiple extractors handle the same file, the first extractor that parses with
tree-sitter sets `opts.ParsedTree`. Subsequent extractors reuse the parsed tree,
avoiding redundant CGO calls. The shared tree is closed after all extractors finish.

---

## Extractors

23 extractors are registered in `registerAllExtractors` (`cmd/knowing/main.go`), covering
17 language families.

### Language-Specific Extractors

| Extractor | Languages | Package | What it extracts |
|-----------|-----------|---------|-----------------|
| Go tree-sitter | Go | `gotsextractor` | Functions, types, methods, interfaces, imports, calls, struct embeddings (implements), channel send/receive, type assertions, `accesses_field` |
| Go full | Go | `goextractor` | Same as above via `go/packages` (full type resolution, confidence 1.0). Used with `--full` flag. |
| Python | Python | `treesitter` (shared) | Functions, classes, methods, imports, calls (with deep argument walking), Flask/FastAPI/Django routes, `self.field` access, type annotations |
| TypeScript/JS | TypeScript, JavaScript | `tsextractor` | Functions, classes, methods, imports, calls, Express/Fastify/Hono/NestJS/Next.js routes, `export_statement` unwrapping, `this.field` access |
| Rust | Rust | `rustextractor` | Functions, structs, traits, impls, use declarations, calls, Actix/Axum/Rocket routes, `self.field` access |
| Java | Java | `javaextractor` | Classes, interfaces, methods, fields, imports, calls, Spring annotation routes, package-qualified names, `this.field` access |
| C# | C# | `csharpextractor` | Classes, interfaces, methods, fields, using directives, calls, ASP.NET attribute routes, `this.Field` access |
| Ruby | Ruby | `rubyextractor` | Classes, modules, methods, includes, calls |

### Infrastructure Extractors

| Extractor | File types | Package | What it extracts |
|-----------|-----------|---------|-----------------|
| Terraform HCL | `.tf` | `terraformextractor` | Resources, data sources, modules, variables, dependency edges |
| SQL | `.sql` | `sqlextractor` | Tables, views, functions, procedures, FK/reference edges |
| Kubernetes YAML | K8s manifests | `k8sextractor` | Deployments, services, configmaps, label-selector edges |
| Cloud | CF/SAM, Docker Compose, GHA, Serverless | `cloudextractor` | CloudFormation resources, Compose services, Actions workflows, Serverless functions |
| CSS/SCSS | `.css`, `.scss` | `cssextractor` | Class/ID selectors, custom properties, `var()` dependency edges |
| Protocol Buffers | `.proto` | `protoextractor` | Services, messages, enums, RPC declarations, type reference edges |
| GraphQL | `.graphql`, `.gql` | `graphqlextractor` | Types, queries, mutations, field references |
| Dockerfile | `Dockerfile*` | `dockerfileextractor` | Build stages, base images, exposed ports |
| Makefile | `Makefile*` | `makefileextractor` | Targets, dependencies, phony declarations |
| Helm | `Chart.yaml`, `values.yaml`, templates | `helmextractor` | Charts, values, template references |
| GitLab CI | `.gitlab-ci.yml` | `gitlabciextractor` | Jobs, stages, script commands |
| package.json | `package.json` | `packagejsonextractor` | Dependencies, scripts, entry points |
| OpenAPI/JSON Schema | `openapi.yaml`, `swagger.json`, JSON Schema | `schemaextractor` | Endpoints, schemas, model definitions |

### Overlay Extractors

These run alongside language-specific extractors via multi-dispatch:

| Extractor | Applies to | Package | What it extracts |
|-----------|-----------|---------|-----------------|
| Event/MQ | Go, Python, TypeScript, Java | `eventextractor` | Kafka, NATS, SQS, RabbitMQ publish/subscribe patterns |
| Env | Go, Python, TypeScript, Rust, Java | `envextractor` | `reads_env` edges for `os.Getenv`, `process.env`, `os.environ` |

---

## Provenance and Confidence

Every edge carries a `Provenance` string and a `Confidence` float64 that record how the
edge was discovered and how certain the system is about its correctness. Provenance is
part of the edge hash, so an `ast_inferred` edge and an `lsp_resolved` edge between the
same two nodes are distinct records.

| Provenance | Confidence | Source |
|------------|-----------|--------|
| `ast_inferred` | 0.7 | Tree-sitter extraction. Syntax-level pattern matching: the extractor saw a function call node in the AST but cannot confirm the target resolves correctly. |
| `ast_resolved` | 0.95 | `go/packages` or import-map resolution. The extractor confirmed the call target through import maps (Python, TypeScript, Rust, Java, C#) or full type resolution (Go `--full`). |
| `lsp_resolved` | 0.9 | LSP enrichment confirmed the edge via `GetDefinition`. See [enrichment.md](enrichment.md). |
| `scip_resolved` | 1.0 | SCIP index import. External compiler-produced index with full type resolution. |
| `similarity` | variable | Jaccard similarity score between function token sets. Ranges from the threshold (0.5) to 1.0. |
| `interface_propagation` | 0.9 | Post-processing derived. Created by matching method names between implementing classes and interfaces. |
| `structural` | 1.0 | QN-structure derived. `contains` and `member_of` edges from type-to-method hierarchy. |
| `co_test_inference` | 0.6 | Post-processing derived. Lateral connections between symbols co-referenced from the same test file. |
| `type_hint_resolved` | 0.8 | Post-processing derived. Dangling `type_hint_of` edges rewritten with correct target hash. |
| `interface_type_hint_propagation` | 0.8 | Post-processing derived. `type_hint_of` edges propagated through interfaces to concrete implementors. |

See [edge-types.md](edge-types.md) for the full catalog of 38 edge types and their RWR
weights.

---

## Post-Processing Pipeline

After tree-sitter extraction completes and all nodes and edges are stored, the indexer
runs a sequence of post-processing steps that derive new edges from the extracted graph.
These steps run before snapshot computation and FTS rebuild. Each step is independently
timed in the `IndexTimings` struct.

### 1. CODEOWNERS Parsing

If a `CODEOWNERS` file exists (GitHub/GitLab format), parse ownership rules and create
`owned_by` edges from files to team/user nodes. These edges carry RWR weight 0.0 (metadata
only, not structural) but are useful for ownership queries and blast radius reporting.

### 2. Inheritance Propagation

For each `extends` edge (child class to parent class), find the parent's methods and
create `inherits` edges from the child class to those methods. This lets RWR walk from
`Flask` to `Scaffold.before_request` via the inheritance chain.

The mechanism uses name-based matching: the `extends` edge target hash is computed with
a bare base name (e.g., "Scaffold"), which may not match the actual parent class node's
hash (which includes the full file path). The propagator builds a reverse map from
terminal class names to node hashes and resolves targets through it.

Impact: +29% P@10 on the cross-system benchmark (the single largest improvement of any
change). Flask: 83 edges. Django: 14,539 edges.

### 3. Interface Propagation

For each `implements` edge (concrete class to interface), find matching method names
between the implementing class and the interface, and create `overrides` edges connecting
them. This lets RWR walk from an interface method to all concrete implementations.

Example: if `RedisCache` implements `BaseCache`, and both have a `get` method, the
propagator creates `RedisCache.get` --overrides--> `BaseCache.get`.

### 4. Type Hint Resolution

Fix dangling `type_hint_of` edges where the target hash was computed with `kind="type"`
but the actual node has a different kind (`interface`, `trait`, `class`, `struct`). The
resolver builds a lookup table keyed by `(repo, package, name)` across all type-like
nodes, computes what the hash would be with `kind="type"`, and rewrites edges whose
target matches the wrong-kind hash to point to the correct node.

Fixed 3,836 edges across 4 repos: k8s (1,087), vscode (2,068), terraform (521),
kafka (160).

### 5. Interface Type Hint Propagation

After resolution, propagate `type_hint_of` through interfaces to concrete implementors.
When a function has `type_hint_of` pointing to an interface, and concrete types implement
that interface, create additional `type_hint_of` edges from the function to each
implementor. This gives RWR a direct path from functions to the concrete types they work
with, bypassing two-hop indirection through the interface node.

808 new edges across k8s (237), terraform (473), kafka (98).

### 6. Contains Edges

Generate structural `contains` and `member_of` edges from type/class nodes to their
method/field nodes. Derived purely from qualified name structure: if a method QN equals
a type QN plus `.` plus a terminal name, emit `type --contains--> method` and the
reverse `method --member_of--> type`.

This connects 77%+ of type nodes that are otherwise completely disconnected from the
graph. Before `contains` edges, 5,457 of 7,086 type nodes in k8s had zero edges.

### 7. Similarity Edges

Compute pairwise Jaccard similarity between function/method bodies within the same
package. Functions with Jaccard > 0.5 get a `similar_to` edge with the similarity score
as confidence.

The tokenizer splits qualified names and signatures on CamelCase boundaries, underscores,
dots, and slashes, then lowercases and filters tokens shorter than 3 characters. Jaccard
is computed as `|A intersection B| / |A union B|` over the token sets.

Guards against explosion:
- Only compares within the same package (not cross-package).
- Skips packages with >500 functions (OOM fix: Kafka's
  `org.apache.kafka.streams` has 16,781 functions, which would produce 140M pairwise
  comparisons and consume 10GB+ RAM).
- Per-node cap of 5 edges (prevents hub explosion from generic tokens).
- Candidates sorted by Jaccard descending so highest-quality edges win the per-node cap.

Similarity edges carry RWR weight 0.15 (lowest of any structural edge) and are P@10
neutral. They bridge disconnected subgraphs where two functions do the same work but
have no call relationship.

### 8. Co-Tested Edges

Create lateral `co_tested_with` edges between non-test symbols referenced from the same
test file. If test file T calls or imports both symbol A and symbol B (and neither is a
test symbol), A and B get a `co_tested_with` edge.

This bridges structurally disconnected symbols that serve the same feature. For example,
`BaseCache` and `RedisCache` are both tested in `tests/cache/tests.py` but may have no
direct call edge between them.

Guards: 20 targets per file, 20 pairs per file (prevents N-squared explosion on large
test files). Test file detection uses path patterns across Go, Python, TypeScript, Rust,
Java, and C#.

### 9. Authorship

Extract `authored_by` edges from `git blame` (parallel, best-effort). One `git blame`
subprocess per changed file, running in parallel with the same worker count as extraction.
Skippable via `--skip-blame` (expensive on large repos).

Authorship edges carry RWR weight 0.0 (metadata only) and are used for ownership queries,
not retrieval ranking.

---

## Producer-Consumer Pipeline

`IndexRepo` uses a parallel producer-consumer architecture to maximize throughput while
respecting SQLite's single-writer constraint.

### File Discovery (Sequential)

1. Walk the repository directory, collecting file paths deterministically (sorted).
2. Skip hidden directories (except `.github`), dependency directories (`vendor`,
   `node_modules`, `__pycache__`), build output (`target`, `build`, `dist`), and
   monorepo mirrors (`staging`, `third_party`).
3. Skip generated files by checking the first 512 bytes for markers: `Code generated`,
   `DO NOT EDIT`, `AUTO-GENERATED`, `# Generated by`, etc.
4. Filter to files that at least one extractor can handle.
5. Compare content hashes against the database; skip unchanged files.
6. Clean up old nodes and edges for changed/deleted files (sequential, touches DB).

### Extraction Workers (Parallel)

`GOMAXPROCS` worker goroutines pull file indices from a work channel. Each worker:

1. Reads file content and builds `ExtractOptions` (repo URL, file hash, content, module
   root, cross-repo module map).
2. Calls `extractFile`, which runs all matching extractors and merges results.
3. Sends the result (nodes, edges, file record) through a buffered result channel.

Each extraction call is wrapped with a 10-second watchdog timer. Tree-sitter CGO calls
are not interruptible by Go context cancellation: if a file takes >10s, the watchdog
fires, the worker sends an empty result and moves on. The stuck CGO goroutine completes
in the background (fire-and-forget) without blocking the pipeline.

### Storage Writer (Single Goroutine)

A single consumer goroutine reads from the result channel and accumulates nodes, edges,
and files into batches. Every 500 files (or at completion), the batch is flushed to
SQLite via batch insert methods:

- `BatchPutFiles`: 249 rows per INSERT statement
- `BatchPutNodes`: 99 rows per INSERT statement
- `BatchPutEdges`: 100 rows per INSERT statement

Multi-row INSERT reduces per-row SQL parsing overhead and CGO crossing count. The
single-writer design avoids SQLite lock contention entirely.

Progress is reported to stderr every 2 seconds: `[N/total] X files/s, Y edges, ETA Zs`.

### Finalization (Sequential)

After extraction and post-processing:

1. **Snapshot computation**: builds a hierarchical Merkle tree from in-memory edge data
   (no DB re-read). See [data-flow.md](data-flow.md) for details.
2. **FTS rebuild**: synchronous full-text search index rebuild (~500ms). Must run after
   snapshot to avoid WAL contention.
3. **Edge event recording**: for incremental runs, records "added" and "removed" edge
   events for snapshot diffing.
4. **Cross-repo resolution**: retargets dangling edges whose target hashes were computed
   with the wrong repo URL.
5. **Auto-GC**: if `edge_events` exceed 5,000 rows, prune old snapshots (keep 10).

---

## Indexing Timings

The `IndexTimings` struct is populated automatically by `IndexRepo` and emitted to stderr
after every run. It provides per-phase wall-clock durations:

```
=== Index Timings ===
File discovery:     312ms
Extraction+writes:  2.1s
CODEOWNERS:         0ms
Inheritance:        45ms
Interface propagat: 12ms
Type hint resolve:  8ms
Type hint propagat: 3ms
Contains:           22ms
Similarity:         890ms
Co-tested:          15ms
Authorship:         1.4s
Snapshot:           95ms
FTS rebuild:        502ms
TOTAL:              5.8s
```

Representative timings from the cross-system benchmark corpus:

| Repo | Language | Files | Nodes | Edges | Time |
|------|----------|-------|-------|-------|------|
| Flask | Python | 97 | 1,658 | 9,237 | 0.1s |
| Cargo | Rust | 979 | 8,075 | 79,305 | 1.4s |
| Django | Python | 2,937 | 42,947 | 185,393 | 3.3s |
| VS Code | TypeScript | 38,260 | 43,379 | 93,382 | 4.1s |
| Terraform | Go | 2,242 | 37,000 | 184,000 | 18.6s |
| Kubernetes | Go | 4,877 | 117,401 | 268,249 | 18.6s |

Similarity computation was the bottleneck before the OOM fix (Kafka took 64s when
similarity ran on oversized packages). After skipping packages with >500 functions,
similarity is sub-second on all repos.

---

## Incremental Indexing

`IndexFilesIncremental` indexes only specified changed files without a full directory
walk. The daemon uses this when `changedFiles` are available from the git watcher.

The incremental path:
1. Clean up old nodes and edges for each changed file.
2. Extract new nodes and edges.
3. Batch store results.
4. Record changed files for downstream consumers (LSP enrichment scoping).

Performance: 24ms for a 1-file edit on a 7,803-node repo (494x faster than full index).
Scales linearly: 5 files = 59ms, 20 files = 93ms.

Post-processing steps (inheritance propagation, similarity, etc.) do not run during
incremental indexing. They run on the next full `IndexRepo` call.

---

## Cross-File Import Resolution

Five language extractors resolve call targets through import maps, upgrading edge
provenance from `ast_inferred` (0.7) to `ast_resolved` (0.95):

| Language | Import map builder | Resolution pattern |
|----------|-------------------|-------------------|
| Python | `buildPythonImportMap` | `from X import Y` where Y is a submodule or symbol |
| TypeScript | `buildTSImportMap` | `import`/`require` declarations |
| Rust | `buildRustImportMap` | `use` declarations, `crate::`, `super::`, `self::` paths |
| Java | `buildJavaImportMap` | `import com.pkg.Class`, `import static` |
| C# | `buildCSharpImportMap` | `using Namespace.Sub`, `using static` |

The Go tree-sitter extractor resolves imports differently: it reads `go.mod` for the
module path and builds a `ModuleToRepoURL` map from all indexed repos and the global
roster. This enables cross-repo edge targeting without heuristic inference.

---

## Known Issues

1. **Similarity OOM on packages with >500 functions.** Fixed by skipping those packages.
   Similarity edges are weighted 0.15 and P@10 neutral; skipping oversized packages
   loses nothing measurable.

2. **Tree-sitter CGO calls not interruptible by Go context cancellation.** The watchdog
   timer (10s) is a fallback: the stuck CGO goroutine continues in the background. This
   is a fundamental limitation of CGO; Go's cooperative scheduling cannot preempt C code.

3. **Root-level Go files produce 0 nodes.** `computePkgPath` requires a `go.mod` to
   derive the package path. Files at the repository root without a `go.mod` get an empty
   package path, producing invalid qualified names that hash to nothing. Roadmap item.

4. **Python has no formal interfaces.** `implements` edges are absent for Python repos
   because Python's duck typing does not produce explicit interface declarations in the
   AST. This means interface propagation (step 3) and interface type hint propagation
   (step 5) produce no edges for Python. Enrichment via pylsp can discover some
   implementations, but tree-sitter extraction alone cannot.

5. **TypeScript export_statement wrapping.** Prior to the fix in v0.10.0, all exported
   classes, functions, and interfaces were silently skipped. VS Code was extracting only
   72 TS nodes from ~1M LOC. The fix unwraps `export_statement` -> declaration child and
   recurses.

---

## Source Files

| File | What it contains |
|------|-----------------|
| `internal/indexer/indexer.go` | `Indexer`, `IndexRepo`, `IndexFilesIncremental`, `extractFile`, post-processing functions (`propagateInheritance`, `propagateInterfaceMethods`, `resolveTypeHintEdges`, `propagateInterfaceTypeHints`, `generateContainsEdges`, `GenerateCoTestedEdges`) |
| `internal/indexer/extractor.go` | `ExtractorRegistry`, `FindExtractor`, `FindAllExtractors` |
| `internal/indexer/similarity.go` | `ComputeSimilarityEdges`, `extractPackage`, `tokenize`, `jaccard` |
| `internal/indexer/authorship/` | `ExtractAuthorship` (git blame integration) |
| `internal/indexer/ownership/` | `FindCodeowners`, `ParseCodeowners`, `ExtractOwnership` |
| `internal/indexer/gotsextractor/` | Go tree-sitter extractor, shared tree parsing |
| `internal/indexer/goextractor/` | Go `go/packages` extractor (full type resolution) |
| `internal/indexer/tsextractor/` | TypeScript/JavaScript extractor |
| `internal/indexer/treesitter/` | Shared tree-sitter extractor (Python) |
| `internal/indexer/rustextractor/` | Rust extractor |
| `internal/indexer/javaextractor/` | Java extractor |
| `internal/indexer/csharpextractor/` | C# extractor |
| `internal/indexer/rubyextractor/` | Ruby extractor |
| `internal/indexer/eventextractor/` | Event/MQ pattern extractor (Kafka, NATS, SQS, RabbitMQ) |
| `internal/indexer/envextractor/` | Environment variable extractor (`reads_env` edges) |
| `cmd/knowing/main.go` | `registerAllExtractors` (full registration list) |

## Related Documents

- [LSP Enrichment](enrichment.md): what happens after extraction (confidence upgrades, new edges, phantom nodes)
- [Edge Types](edge-types.md): full catalog of 38 edge types with RWR weights
- [Data Flow](data-flow.md): end-to-end commit-to-graph pipeline
- [Retrieval Pipeline](retrieval-pipeline.md): how the extracted graph is queried via RWR
- [Context Engine](context-engine.md): how extraction quality affects retrieval scoring
- [Embedding Re-ranker](embedding-reranker.md): how embeddings interact with extracted nodes
