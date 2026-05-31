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
| Python | 3,386 | 2 weeks | HIGH (django, flask, fastapi) |
| Go | 3,144 | 2 weeks | HIGH (k8s, terraform, caddy) |
| TypeScript | 4,889 | 3 weeks | MEDIUM (vscode) |
| C# | 3,200 | 2 weeks | MEDIUM (ocelot) |
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
