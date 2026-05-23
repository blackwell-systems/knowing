# Code Quality Audit

Date: 2026-05-22
Scope: `/Users/dayna.blackwell/code/knowing`

---

## 1. Duplication

### 1.1 Mock Store Implementations (6 copies)

Every package that tests against `types.GraphStore` defines its own mock from scratch. Each re-implements the same ~20 interface methods with trivial in-memory maps.

| File | Type Name |
|------|-----------|
| `internal/mcp/handlers_test.go:18` | `mockGraphStore` |
| `internal/context/context_test.go:12` | `mockStore` |
| `internal/resolver/resolver_test.go:11` | `mockStore` |
| `internal/enrichment/enricher_test.go:14` | `mockStore` |
| `internal/snapshot/manager_test.go:13` | `mockGraphStore` |
| `internal/indexer/indexer_test.go:37` | `mockStore` |

**Severity:** Medium
**Fix:** Extract a shared `internal/testutil/mockstore.go` that all test packages import. Each consumer embeds it and overrides only the methods it needs.

---

### 1.2 `inferExternalRepoURL` (5 independent implementations)

Five extractors each define their own `inferExternalRepoURL` function with identical logic structure (check stdlib prefixes, return `"external://{pkg}"` or `"stdlib"`):

- `internal/indexer/tsextractor/extractor.go:537`
- `internal/indexer/treesitter/extractor.go:573` (Python)
- `internal/indexer/rustextractor/extractor.go:404`
- `internal/indexer/javaextractor/extractor.go:663`
- `internal/indexer/csharpextractor/extractor.go:250`

**Severity:** Medium
**Fix:** Extract a shared `internal/indexer/externalurl` package with a generic dispatcher: `InferExternalRepoURL(lang Language, modulePath string) string`. Each extractor passes its language-specific stdlib list via a configuration struct.

---

### 1.3 Deterministic Result Sorting (8+ copies)

Every extractor duplicates the same sort logic to sort nodes by `QualifiedName`/`Kind` and edges by `SourceHash`/`TargetHash`/`EdgeType`:

- `internal/indexer/goextractor/extractor.go:185-193`
- `internal/indexer/csharpextractor/extractor.go:83-91`
- `internal/indexer/javaextractor/extractor.go:117-125`
- `internal/indexer/rustextractor/extractor.go:96-104`
- `internal/indexer/tsextractor/extractor.go:120-128`
- `internal/indexer/gotsextractor/extractor.go:204-212`
- `internal/indexer/rubyextractor/extractor.go:154-162`
- `internal/indexer/terraformextractor/extractor.go:106-114`

Note: The indexer itself (`indexer.go:102-118`) also sorts the result, making the per-extractor sorts redundant.

**Severity:** Medium
**Fix:** Remove the sort from individual extractors since the indexer already normalizes the result. Or, if extractors need sorted results for testing, add a helper `types.SortResult(r *ExtractResult)`.

---

### 1.4 CanHandle Build-Directory Exclusion Pattern (11 copies)

Eleven extractors repeat the identical pattern of `strings.Split(filepath.ToSlash(path), "/")` followed by a loop checking for build output directories:

- `goextractor`, `gotsextractor`, `tsextractor`, `csharpextractor`, `javaextractor`, `rustextractor`, `cssextractor`, `protoextractor`, `terraformextractor`, `eventextractor`, `rubyextractor`

**Severity:** Low
**Fix:** Add a `PathContainsAny(path string, dirs []string) bool` helper in `internal/indexer` and call it from each `CanHandle`.

---

## 2. Dead Code / Unused Symbols

### 2.1 `types.ComputationCache` interface (zero implementations)

Defined at `internal/types/interfaces.go:150` with the comment "Interface defined now; implementation is deferred." No concrete type implements this interface anywhere in the codebase.

**Severity:** High
**Fix:** Remove until an implementation exists. Dead interface definitions mislead contributors into thinking the system has caching infrastructure.

---

### 2.2 `types.DerivedResult` struct (zero instantiations)

Defined at `internal/types/results.go:54`. Only referenced by `ComputationCache` interface methods (which are also dead).

**Severity:** High
**Fix:** Remove along with `ComputationCache`.

---

### 2.3 `types.TraversalOptions` struct (zero references outside definition)

Defined at `internal/types/results.go:66`. Not used by any function signature, query, or traversal in the codebase.

**Severity:** High
**Fix:** Remove. The actual traversal functions (`TransitiveCallers`, `TransitiveCallees`) use explicit `maxDepth` parameters, not this struct.

---

### 2.4 `types.EdgeProvenance` struct (never populated)

Defined at `internal/types/types.go:188`. The `CallerWithProvenance.Provenance []EdgeProvenance` field is always left as nil (see `internal/store/sqlite.go:645-650` where `CallerWithProvenance` is constructed without populating `Provenance`).

**Severity:** Medium
**Fix:** Either populate it in `BlastRadius` queries or remove the struct and the field from `CallerWithProvenance`.

---

### 2.5 `context.CompareContextPacks` and `PackDiff` (test-only usage)

`internal/context/pack_compare.go` exports `CompareContextPacks` and `PackDiff` but these are only referenced by their own test file (`pack_compare_test.go`). No MCP handler, CLI command, or other package calls them.

**Severity:** Low
**Fix:** Either wire it into the `snapshot_diff` MCP response or demote to unexported.

---

### 2.6 `edgetype` package constants (unused by extractors)

`internal/edgetype/constants.go` defines 23 edge type constants, but only 1 file in the entire codebase imports the package (`internal/mcp/ownership.go`). All extractors use raw string literals like `"calls"`, `"imports"`, `"implements"`.

**Severity:** High
**Fix:** Migrate all extractors to use `edgetype.Calls`, `edgetype.Imports`, etc. This prevents typos and centralizes the vocabulary.

---

## 3. Stubs / Unimplemented Functions

### 3.1 C# throws edge extraction (2 TODOs)

```
internal/indexer/csharpextractor/extractor.go:519: // TODO: extract throws edge to exception type
internal/indexer/csharpextractor/extractor.go:605: // TODO: extract throws edge to exception type
```

The Rust extractor already implements throws edges. The C# extractor has the TODO but no implementation.

**Severity:** Low
**Fix:** Implement `throws` edge extraction for `throw` statements in C# (same pattern as Rust extractor).

---

### 3.2 Merkle-aware context expiration (2 TODOs)

```
internal/context/explain.go:147:    // TODO: Pass neighborhood roots for merkleized expiration...
internal/context/context.go:526:    // TODO: Pass neighborhood roots for merkleized expiration...
```

The hierarchical Merkle tree is implemented (`internal/snapshot/hierarchical.go`) but context pack caching does not yet use it for invalidation.

**Severity:** Medium
**Fix:** Wire the snapshot's Merkle subtree roots to the context pack cache eviction logic. This is the intended design per the architecture docs.

---

## 4. Missing Abstractions

### 4.1 Chunked Batch Insert Pattern (6 copies)

The store repeats the same pattern for chunked inserts to stay within SQLite's 999-variable limit:

- `BatchPutNodes` (chunk of 99, 10 params)
- `BatchPutEdges` (chunk of 100, 9 params)
- `BatchPutFiles` (chunk of 249, 4 params)
- `BatchPutNotes` (chunk of 124, 4+ params)
- `DeleteNodesNotIn` (gc batch)
- `DeleteEdgesNotIn` (gc batch)

Each duplicates: empty check, begin tx, defer rollback, for loop with end calculation, commit.

**Severity:** Medium
**Fix:** Extract a generic `batchExecInTx(ctx, db, items, chunkSize, buildSQL func(chunk) (string, []any))` helper.

---

### 4.2 Node Kind Constants as String Literals

Node kinds (`"function"`, `"method"`, `"type"`, `"interface"`, `"var"`, `"const"`, `"route_handler"`, `"action"`, `"job"`, `"image"`) are scattered as raw strings across 24+ extractors and test files. No constants package exists for node kinds (unlike `edgetype` which at least defines edge type constants).

**Severity:** High
**Fix:** Add `internal/nodekind/constants.go` with `const Function = "function"`, etc. Use them in all extractors.

---

### 4.3 Confidence/Provenance Tier Constants

Confidence values (`0.7`, `0.8`, `0.9`, `1.0`, `0.6`, `0.95`) and provenance strings (`"ast_resolved"`, `"ast_inferred"`, `"lsp_resolved"`, `"scip_resolved"`) are scattered as raw literals. While the edge type package exists, there is no equivalent for provenance tiers.

**Severity:** Medium
**Fix:** Add to `internal/edgetype/constants.go` or create `internal/provenance/constants.go`:
```go
const (
    ASTResolved  = "ast_resolved"
    ASTInferred  = "ast_inferred"
    LSPResolved  = "lsp_resolved"
    SCIPResolved = "scip_resolved"
)
const (
    ConfidenceASTResolved  = 1.0
    ConfidenceASTInferred  = 0.7
    ConfidenceLSPResolved  = 0.9
    ConfidenceSCIPResolved = 0.95
)
```

---

## 5. Inconsistencies

### 5.1 Extractor Architecture Style

Some extractors use receiver methods on a struct:
- `(e *CSharpExtractor) Extract(...)` (csharpextractor)
- `(e *RustExtractor) Extract(...)` (rustextractor)

While others define package-level helper functions that look like they could be standalone extractors:
- `javaextractor` uses `JavaExtractor` struct but most internal logic is in package-level funcs

All extractors are consistent in implementing the `types.Extractor` interface, but internal organization varies. This is a style issue, not a bug.

**Severity:** Low (cosmetic)

---

### 5.2 Test Hash Construction

Tests use different patterns to create hashes:
- `types.NewHash([]byte("some string"))` (most test files)
- `types.ComputeNodeHash(...)` (store tests via `makeNode` helper)
- `testHash("...")` helper (only in `internal/mcp/handlers_test.go:198`)

**Severity:** Low
**Fix:** Standardize on `types.NewHash([]byte(...))` for simple test hashes. The `testHash` helper in MCP tests wraps this with hex decode, which is more verbose than needed.

---

### 5.3 `edgetype.RWRWeight` is the Only Consumer of Edge Type Constants

The `edgetype` package defines constants AND the RWR weight function, but only `internal/mcp/ownership.go` imports the constants. The RWR weight function itself is used from `internal/context/walk.go` (confirmed via the walk implementation referencing weights). However, the walk implementation duplicates its own weight map:

```
internal/context/walk.go uses edgetype.RWRWeight -- CONFIRMED
```

This is actually correct. But extractors not using these constants remains the primary inconsistency.

**Severity:** Already covered in 2.6.

---

## Summary: Top 20 Issues by Impact

| # | Category | Issue | Severity | Effort |
|---|----------|-------|----------|--------|
| 1 | Dead Code | `ComputationCache` interface (never implemented) | High | Low |
| 2 | Dead Code | `DerivedResult` struct (never instantiated) | High | Low |
| 3 | Dead Code | `TraversalOptions` struct (never used) | High | Low |
| 4 | Inconsistency | `edgetype` constants ignored by all extractors | High | Medium |
| 5 | Missing Abstraction | Node kind string literals (no constants) | High | Medium |
| 6 | Duplication | Mock store (6 independent copies) | Medium | Medium |
| 7 | Duplication | `inferExternalRepoURL` (5 copies) | Medium | High |
| 8 | Duplication | Result sorting in extractors (8 copies, redundant with indexer) | Medium | Low |
| 9 | Missing Abstraction | Provenance/confidence constants as literals | Medium | Low |
| 10 | Missing Abstraction | Chunked batch insert pattern (6 copies) | Medium | Medium |
| 11 | Stub | Merkle-aware context pack expiration (2 TODOs) | Medium | High |
| 12 | Dead Code | `EdgeProvenance` struct (never populated) | Medium | Low |
| 13 | Dead Code | `CompareContextPacks`/`PackDiff` (test-only) | Low | Low |
| 14 | Duplication | `CanHandle` path-split pattern (11 copies) | Low | Low |
| 15 | Stub | C# throws edge extraction (2 TODOs) | Low | Medium |
| 16 | Inconsistency | Test hash construction patterns vary | Low | Low |
| 17 | Inconsistency | Extractor internal code organization | Low | N/A |
| 18 | Dead Code | `edgetype` package unused by 23/24 extractors | High | Medium |
| 19 | Missing Abstraction | Build directory exclusion shared helper | Low | Low |
| 20 | Duplication | Batch insert boilerplate (tx + chunk + commit) | Medium | Medium |

---

## Recommended Priority

**Immediate (high impact, low effort):**
1. Delete `ComputationCache`, `DerivedResult`, `TraversalOptions`
2. Add `internal/nodekind/constants.go` and migrate extractors
3. Add provenance constants to `internal/edgetype/constants.go`
4. Migrate all extractors to use `edgetype.Calls` etc.

**Next sprint (medium effort):**
5. Extract shared mock store for tests
6. Remove redundant per-extractor sorting
7. Populate or remove `EdgeProvenance` field

**Backlog (high effort, medium value):**
8. Factor `inferExternalRepoURL` into shared package
9. Generic batch insert helper
10. Merkle-aware cache invalidation (TODO)
