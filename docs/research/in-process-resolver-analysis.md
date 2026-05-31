# In-Process Language Resolver Architecture Analysis

Analysis of codebase-memory-mcp (DeusData) in-process LSP resolver architecture.
Objective: understand their approach to inform knowing's own Go-native resolvers.

## 1. Architecture Overview

The extraction pipeline has two layers:

- **`internal/cbm/`**: core extraction engine (`cbm.c`, `extract_*.c`) and all per-language
  LSP resolvers (`lsp/` subdirectory). Pure C, compiled as static library.
- **`src/pipeline/`**: orchestrator (`pipeline.c`), thread-pool dispatch (`pass_parallel.c`),
  cross-file LSP coordination (`pass_lsp_cross.c`).

**Key insight:** LSP resolution runs inside the same process as tree-sitter extraction,
sharing the same parsed AST. In `cbm_extract_file`:

1. Tree-sitter parses file into AST
2. Three extractors run on that AST: definitions, imports, unified
3. Per-file LSP resolver runs on the *same root node*: no re-parse, no serialization

The pipeline retains two artifacts per file in `CBMFileResult`:
- `cached_tree`: the TSTree pointer, kept alive for cross-file pass
- `source`/`source_len`: source bytes, copied into per-file arena

Cross-file LSP pass can re-walk the cached tree without reading disk or re-parsing.

## 2. Per-Language Resolvers

All resolvers live in `internal/cbm/lsp/`:

| Language | File | Lines (.c) | What it resolves |
|----------|------|------------|------------------|
| Go | `go_lsp.c` | 2,989 | Scopes, all Go types including generics, import alias resolution, selector dispatch, method lookup with embedded-field promotion, interface satisfaction, builtins, multi-return tuples, generic unification |
| Python | `py_lsp.c` | 3,248 | Scopes, types (union/optional/protocol/callable/module), from-style imports, self/cls inference, MRO-based attribute lookup, decorators, lambda, dict-literal dispatch |
| TypeScript | `ts_lsp.c` | 4,733 | Full TS type system (intersection, conditional, mapped, indexed, keyof, typeof, infer), JSX element resolution, import map, dialect flags, generics, async unwrapping |
| C/C++/CUDA | `c_lsp.c` | 4,909 | Namespace resolution, function pointers, template parameter defaults, pending template resolution, base-class traversal, preprocessor macro expansion + second-pass |
| C# | `cs_lsp.c` | 3,021 | Using directives, namespace stack, partial classes, properties/indexers, primary constructors, target-typed new, tuples, lambdas, await unwrapping, extension method dispatch |
| PHP | `php_lsp.c` | 3,965 | Namespace resolution, use clauses, PHPDoc type parsing, @phpstan-type aliases, parent/self/static resolution, base-class method lookup |

**Shared infrastructure (~2,900 lines):**
- `type_rep.c/h` (1,012 lines): `CBMType` tagged union, 31 type kinds, arena-allocated
- `type_registry.c/h` (741 lines): hash-indexed func/type tables, fallback chaining, overload resolution
- `scope.c/h` (110 lines): linked-list scope chain with push/pop/bind/lookup
- `lsp_node_iter.h` (42 lines): O(n) child collection (avoids tree-sitter O(n^2))

**Generated stdlib data (~57,000 lines):**
- Go: 30,630 lines, Python: 23,527, C#: 1,139, C++: 947, PHP: 687, C: 132
- Register standard library types/signatures into registry for stdlib call resolution

**Pattern:** ~60% language-specific, ~40% shared. Each resolver follows identical pattern:
`init() -> add_import() -> process_file(root) -> eval_expr_type() + process_statement() + resolve_calls()`

## 3. Tier 1/2/3 Architecture

**Tier 1: Per-file LSP** (runs during extraction)
- Builds `CBMTypeRegistry` from file's own definitions + stdlib
- Walks AST, evaluates types, emits resolved calls
- Unresolvable calls emitted with `strategy="lsp_unresolved"` + diagnostic reason

**Tier 2: Pre-built cross-LSP registry** (after all files extracted)
- One global `CBMTypeRegistry` per language from collected `CBMLSPDef[]` array
- Finalized (hash indexes built), shared read-only across parallel resolve workers
- TS uses per-file overlay registries chained to shared base via `fallback` pointer
- `CBMModuleDefIndex`: per-module inverted index (50-100x smaller than full registry)

**Tier 3: Metadata-driven (Go only)**
- During Tier 1, Go resolver emits unresolved entries with enough metadata to resolve later:
  `symbol_not_in_registry`, `method_not_found`, `function_not_in_registry`
- At cross-file time: pure hash lookups in global registry, translate import aliases
- **No tree-sitter parse, no AST walk.** Fastest possible cross-file resolution.

## 4. Key Data Structures

**`CBMType`:** Tagged union, 31 type kinds. Language-specific kinds included
(Python: UNION, PROTOCOL, MODULE; TS: INTERSECTION, CONDITIONAL, MAPPED, INFER).
Arena-allocated, entire type graph freed in one `arena_destroy()`.

**`CBMScope`:** Linked list, chunks of 16 `CBMVarBinding` (name-to-type). O(1) push/pop,
lookup walks up chain. No per-scope hash tables.

**`CBMTypeRegistry`:** Parallel arrays of `CBMRegisteredFunc` and `CBMRegisteredType`.
Hash indexes built lazily. Supports `fallback` pointer for registry chaining.
`CBMRegisteredFunc` carries: qualified_name, receiver_type, full signature, type params, decorator info.

**`CBMResolvedCall`:** Output unit: `{caller_qn, callee_qn, strategy, confidence, reason}`.
Pipeline picks highest-confidence entry per (caller, callee) pair.

**Type propagation:** Through function signatures in registry, call expression evaluation
(callee type -> return type), generic unification + substitution, and variable binding
in scope chain.

## 5. Go Reimplementation Plan

### Shared infrastructure (build once, 2-3 weeks)

| Component | Their LOC | Go approach |
|-----------|----------|-------------|
| Type representation | 1,012 | Go interface with concrete types per kind, or struct with kind + type-switched data |
| Type registry | 741 | Go maps (hash tables free, no finalize step needed) |
| Scope chain | 110 | Linked list of `map[string]Type` |
| Cross-file coordinator | ~3,000 | Pipeline orchestration, module def index, goroutine workers |
| Stdlib data generators | scripts | Generate from typeshed (Python), go doc (Go), DefinitelyTyped (TS) |

### Per-language resolvers

| Language | Their LOC | Effort | Priority |
|----------|----------|--------|----------|
| Python | 3,248 | 2 weeks | HIGH (django, flask, fastapi) |
| Go | 2,989 | 2 weeks | HIGH (k8s, terraform, caddy) |
| TypeScript | 4,733 | 3 weeks | MEDIUM (vscode) |
| C# | 3,021 | 2 weeks | MEDIUM (ocelot) |
| Rust | N/A | 2 weeks | MEDIUM (cargo, ripgrep) |
| Ruby | N/A | 2 weeks | MEDIUM (jekyll, rails) |
| Java | N/A | 2 weeks | LOW (kafka, spark-java) |

### Why Go works (despite C being faster)

The "100x speedup" is not from C vs Go speed. It's from:
1. **No IPC** (no JSON-RPC per symbol, no pipe serialization)
2. **Shared AST** (tree-sitter parse tree already in memory from extraction)
3. **Registry built once** (shared read-only across parallel workers)
4. **Tier 3** (cross-file = pure hash lookups, zero AST interaction)

All four apply equally in Go. The CGo boundary for tree-sitter node access adds ~100ns
per call, but eliminates 10ms+ per JSON-RPC round-trip. Net: 50-100x improvement over
external LSP, even in Go.

### Build order

1. Shared infrastructure (2-3 weeks)
2. Python resolver (2 weeks): replace pyright enrichment
3. Go resolver (2 weeks): replace gopls enrichment
4. Test on benchmark corpus, validate edge quality vs external LSP
5. TypeScript, C#, Ruby, Rust, Java as needed

**Total: 11-12 weeks for Go + Python + TS + C#. Eliminates all external LSP dependencies.**

## 6. What's Novel in Their Approach

- **Tier 3 metadata-driven resolution** is clever: capture "why I couldn't resolve this"
  during extraction, then resolve later with minimal work. Could be generalized beyond Go.
- **Per-module def index** avoids O(D) registry per file. Only load defs from imported modules.
- **Arena allocation** for types: entire file's type graph freed in one call. Go's GC
  handles this differently but the principle (scope type lifetime to file) applies.
- **Registry fallback chaining**: TS overlays chain to shared base. Avoids per-file copy
  of the global registry while allowing file-local type refinement.

## 7. Integration with knowing's Enrichment Pipeline

### Current enrichment architecture

The enrichment pipeline (`internal/enrichment/enricher.go`) runs as a post-extraction
phase. The `Enricher` struct holds a `types.GraphStore`, a workspace root, and
concurrency controls (a channel-based semaphore, `sync.WaitGroup`, `sync.Mutex` for
serializing DB writes). It spawns external language servers (gopls, pyright, tsserver,
etc.) via `lsp.LSPClient`, waits for workspace readiness with probe retries, then
executes two phases:

1. **Edge upgrade:** queries `GetDefinition` at call-site positions recorded during
   tree-sitter extraction. Resolved edges are deleted and rewritten as `lsp_resolved`
   with confidence 0.9. A single DB-writer goroutine consumes results from concurrent
   LSP workers.
2. **Edge discovery:** opens files in batches, queries `GetDocumentSymbols`,
   `GetImplementation`, and `GetReferences` per symbol. New edges are written under
   `writeMu` to avoid SQLite contention.

The pipeline supports multi-module Go workspaces (spawning one gopls per module),
progress persistence for resumable runs, and cross-repo definition resolution via the
roster.

### Where in-process resolvers plug in

In-process resolvers replace the external LSP phase entirely for covered languages.
The integration point is between tree-sitter extraction (`internal/indexer/worker.go`)
and the current `Enricher.Run()` call. Instead of: extract -> store -> spawn LSP ->
query LSP -> upgrade edges, the flow becomes: extract -> retain AST -> resolve
in-process -> store resolved edges directly.

The key is that tree-sitter parse trees are already retained across extractors.
`ExtractResult.ParsedTree` returns the `*sitter.Node` root, and the indexer passes it
to subsequent extractors via `ExtractOptions.ParsedTree`. The same mechanism extends
naturally to in-process resolvers: after all extractors finish for a file, the still-live
parse tree (and source bytes from `ExtractOptions.Content`) are passed to the resolver.
No re-parse, no serialization, no IPC.

### Interface contract

A resolver must produce `[]types.Edge` compatible with the existing graph store. The
minimal interface:

```go
// Resolver performs in-process type resolution for a single language.
type Resolver interface {
    // Language returns the language ID (e.g., "go", "python").
    Language() string

    // InitWorkspace builds the cross-file type registry from all extracted
    // definitions. Called once before per-file resolution. The registry is
    // read-only after this call and safe for concurrent access.
    InitWorkspace(ctx context.Context, defs []ResolverDef) error

    // ResolveFile takes a file's parse tree, source bytes, and the
    // ast_inferred edges from extraction, and returns upgraded/new edges.
    // Edges use provenance "resolver_resolved" and confidence 0.9.
    // Thread-safe: called concurrently across files.
    ResolveFile(ctx context.Context, opts ResolveFileOpts) ([]types.Edge, error)
}

type ResolveFileOpts struct {
    FilePath   string
    FileHash   types.Hash
    Content    []byte
    ParsedTree types.ParsedTree   // *sitter.Node root, still live
    Edges      []types.Edge       // ast_inferred edges from extraction
}
```

The `InitWorkspace` / `ResolveFile` split mirrors codebase-memory's Tier 1/Tier 2
architecture: per-file resolution runs against a shared read-only registry built from
all files' definitions.

### Reusing existing parallel infrastructure

The enricher's concurrency pattern (channel semaphore + WaitGroup + single DB-writer
goroutine) transfers directly. `ResolveFile` is a pure function that returns edges;
the caller dispatches it across goroutines bounded by the semaphore and feeds results
to a writer goroutine that calls `store.PutEdge` / `store.DeleteEdge`. The `writeMu`
mutex from `insertEdgesFromLocations` is unnecessary because the single-writer pattern
already serializes mutations.

The `enrichStats` atomic counters (`edgesProcessed`, `edgesUpgraded`, `newEdges`, etc.)
carry over unchanged. Progress persistence (`EnrichProgress`) also reuses: modules
are the natural checkpoint boundary, same as today.

### What changes vs what stays

**Reusable as-is:**
- `enrichStats` and summary logging
- `createPhantomNodes` (still needed for edges targeting external/stdlib symbols)
- `upgradeEdge` helper (hash recomputation for provenance change)
- `EnrichProgress` for multi-module resume
- `isTestFile` filtering
- `resolveDefinitionToNode` and roster-based cross-repo lookup

**Replaced:**
- `lsp.LSPClient` creation, initialization, warmup probes, shutdown
- `upgradeCallEdges` (LSP round-trips replaced by in-process registry lookups)
- `discoverNewEdgesBatched` (document symbol queries replaced by AST-driven discovery)
- `processSymbolsWithSourceAndClient` (LSP GetImplementation/GetReferences replaced
  by registry-based type matching)

**New:**
- `ResolverEnricher` struct: holds a `Resolver`, a `types.GraphStore`, and concurrency
  controls. Implements the same edge-writing pattern but calls `ResolveFile` instead
  of LSP JSON-RPC
- Registry builder: collects `ResolverDef` entries from extraction results (node
  qualified names, types, signatures) and passes them to `InitWorkspace`
- AST lifetime extension: the indexer must keep parse trees alive until resolution
  completes, then close them. Currently trees are closed after extraction; this
  requires deferring cleanup to after the resolver pass

### Concrete integration sketch

```go
type ResolverEnricher struct {
    store       types.GraphStore
    resolvers   map[string]Resolver  // language -> resolver
    concurrency int
    writeMu     sync.Mutex
}

func (re *ResolverEnricher) Run(ctx context.Context, repoHash types.Hash,
    fileResults []indexer.FileResult) error {

    // Group files by language.
    byLang := groupByLanguage(fileResults)

    for lang, resolver := range re.resolvers {
        files := byLang[lang]
        if len(files) == 0 {
            continue
        }

        // Build cross-file registry from all definitions.
        defs := collectDefs(files)
        if err := resolver.InitWorkspace(ctx, defs); err != nil {
            log.Printf("resolver %s: init workspace: %v", lang, err)
            continue
        }

        // Resolve files concurrently, write edges through single writer.
        results := make(chan []types.Edge, re.concurrency*2)
        var writerWg sync.WaitGroup
        writerWg.Add(1)
        go func() {
            defer writerWg.Done()
            for edges := range results {
                for _, edge := range edges {
                    _ = re.store.PutEdge(ctx, edge)
                }
            }
        }()

        sem := make(chan struct{}, re.concurrency)
        var wg sync.WaitGroup
        for _, fr := range files {
            wg.Add(1)
            sem <- struct{}{}
            go func(fr indexer.FileResult) {
                defer wg.Done()
                defer func() { <-sem }()
                edges, err := resolver.ResolveFile(ctx, ResolveFileOpts{
                    FilePath:   fr.Path,
                    FileHash:   fr.FileHash,
                    Content:    fr.Content,
                    ParsedTree: fr.ParsedTree,
                    Edges:      fr.Edges,
                })
                if err == nil && len(edges) > 0 {
                    results <- edges
                }
            }(fr)
        }
        wg.Wait()
        close(results)
        writerWg.Wait()
    }
    return nil
}
```

### Migration path: hybrid mode

Not all languages will have in-process resolvers on day one. The enrichment
orchestrator should support running both paths:

1. **Router:** after extraction, check which languages have registered `Resolver`
   implementations. Route those languages to `ResolverEnricher`. Route remaining
   languages to the existing `Enricher` (external LSP).
2. **Shared provenance:** in-process resolvers write edges with provenance
   `"resolver_resolved"` (distinct from `"lsp_resolved"`). This allows A/B comparison
   during validation without conflicting with existing enriched edges.
3. **Fallback:** if a resolver's `InitWorkspace` fails (e.g., missing stdlib data),
   fall back to external LSP for that language. Log the fallback so it surfaces in
   benchmark diagnostics.
4. **Validation gate:** before switching a language from external LSP to in-process,
   run both paths on the benchmark corpus and compare edge counts, edge targets, and
   P@10 impact. The resolver must match or exceed external LSP edge quality; speed
   alone is not sufficient justification.
5. **Phased rollout:** Go and Python first (highest benchmark impact, best-understood
   languages). TypeScript and C# follow after the shared infrastructure proves stable.
   External LSP remains the fallback for languages without resolvers until those
   resolvers are built.
6. **Layered enrichment with deduplication:** The resolver and external LSP run as
   stacked layers, not alternatives. The resolver runs first (fast, <1s), producing
   `resolver_resolved` edges. Then external LSP runs and fills gaps. Once the resolver
   reaches 80%+ coverage, the external LSP's `upgradeCallEdges` phase should skip
   source nodes that already have a `resolver_resolved` edge for the same call site.
   This avoids redundant GetDefinition calls for edges the resolver already resolved.
   Implementation: filter the LSP work queue to exclude edges where a resolver edge
   with the same source_hash and callsite_line already exists. Saves ~80% of LSP
   round-trips once the resolver is mature. Do not implement this optimization until
   resolver coverage is validated at 80%+ on the benchmark corpus.

### Endgame: full replacement

Once every supported language has an in-process resolver, the external LSP path
becomes dead code and can be removed entirely. The dependency chain that goes away:

- `lsp.LSPClient` and all JSON-RPC protocol handling
- External process spawning, warmup probes, health checks, timeout management
- Multi-module gopls coordination (the problem that motivated this analysis)
- Language server installation requirements (gopls, pyright, tsserver, ruby-lsp,
  jdtls, rust-analyzer, csharp-ls)

**Coverage to full replacement:**

| Resolvers built | Corpus coverage | External LSP still needed for |
|----------------|----------------|-------------------------------|
| Go, Python | 9/15 repos | TS, C#, Rust, Ruby, Java |
| + TypeScript, Ruby | 13/15 repos | C#, Java |
| + C#, Rust | 15/15 repos | Java only |
| + Java | 15/15 repos | Nothing (remove external LSP) |

**What full replacement means for the product:**
- **True single binary.** No language servers to install, configure, or update.
  `knowing add .` works on a fresh machine with zero setup.
- **Enrichment in seconds, not minutes.** No process spawning, no warmup, no IPC.
  A repo that takes 90s to enrich with gopls takes <2s with in-process resolution.
- **No more enrichment failures.** External LSP servers crash, time out, refuse to
  start on certain project layouts (go.work with 30+ modules). In-process resolution
  has no external failure modes.
- **Simpler codebase.** The enricher drops from ~1,500 lines of LSP lifecycle
  management to ~200 lines of resolver dispatch.

The multi-module gopls scout (currently in progress) is a near-term fix for the
immediate go.work problem. In-process Go resolution is the long-term answer that
makes the problem structurally impossible.

### Accuracy tiers: retrieval vs supply chain

In-process resolvers and external LSP enrichment serve different accuracy
requirements. This distinction is load-bearing for the product architecture.

**Retrieval (P@10, MCP context queries):** Tolerates incomplete edges. If the
resolver produces 85% of the edges gopls would, the graph still has sufficient
connectivity for RWR to reach correct symbols. Missing edges reduce reachability
but don't produce wrong answers. False positive edges (wrong targets) wash out
in ranking: one incorrect edge among 200K correct ones doesn't move P@10.

**Supply chain detection (proof of absence):** Requires near-100% precision AND
high recall. The supply chain detector proves "this package does NOT call
os.Exec" by exhaustively walking all reachable paths. If the resolver misses
edges, a malicious call path could exist through unresolved connections, and the
package would be declared safe when it isn't. False positive edges are equally
dangerous: phantom connections mask real malicious paths, invalidating the
proof-of-absence guarantee.

| Use case | Precision required | Recall required | Resolver viable? |
|----------|-------------------|-----------------|-----------------|
| Retrieval (MCP queries) | 80%+ | 85%+ | Yes |
| Supply chain detection | ~100% | 95%+ | No |

**Architecture rule:** The resolver is the **fast path** for interactive use
(MCP server, `knowing context`, `knowing test-scope`). External LSP enrichment
is the **secure path** for supply chain analysis (`knowing audit-supply-chain`).
The routing decision is made at the call site:

- `knowing add .` (default): resolver only (fast, no dependencies)
- `knowing add . --enrich`: resolver + external LSP (full quality)
- `knowing audit-supply-chain`: external LSP required (security claims demand it)

External LSP enrichment is NOT removed from the codebase even after all
resolvers are built. It remains as the high-assurance path for security
analysis. The "true single binary" claim applies to the default interactive
mode, not to security auditing mode which may require language servers.
