# Edge Accuracy Benchmark: Tree-Sitter vs Go/AST

**Auto-generated from test run. Do not edit manually.**

## Methodology

Compares knowing's two Go extraction tiers by indexing the same repo with each:

1. **Tree-sitter** (`gotsextractor`): Syntax-only, fast. Produces `calls` and `imports` edges.
   Provenance `ast_inferred`, confidence 0.7.
2. **Go/AST** (`goextractor`): Full type resolution via `go/packages`. Produces `calls`,
   `imports`, `implements`, and `references` edges. Provenance `ast_resolved`, confidence 1.0.

Edges match by identity tuple: `(source_hash, target_hash, edge_type)`. Provenance
is excluded from matching since it differs by design.

## Overall Results

| Metric | Count | Rate |
|--------|-------|------|
| Tree-sitter edges (ast_inferred) | 29283 | - |
| Go/ast edges (ast_resolved) | 59983 | - |
| Confirmed (in both) | 8082 | 27.6% of inferred |
| Inferred-only (potential FP) | 21201 | 72.4% of inferred |
| Resolved-only (missed) | 51901 | 86.5% of resolved |

## Per-Edge-Type Breakdown

| Edge Type | Tree-sitter | Go/ast | Confirmed | FP Rate | Miss Rate |
|-----------|-------------|--------|-----------|---------|----------|
| calls | 14420 | 6280 | 39.6% | 60.4% | 9.0% |
| imports | 1563 | 870 | 54.9% | 45.1% | 1.4% |
| implements | 0 | 9 | 0.0% | 0.0% | 100.0% |
| references | 0 | 50826 | 0.0% | 0.0% | 100.0% |

## Fair Comparison (calls + imports only)

Tree-sitter does not produce `implements` or `references` edges (these require
type resolution). The overall numbers are misleading because go/ast's 19K+ reference
edges inflate the miss rate. A fair comparison restricts to edge types both extractors
attempt:

| Metric | Count | Rate |
|--------|-------|------|
| Tree-sitter edges | 15983 | - |
| Go/ast edges | 7150 | - |
| Confirmed | 6573 | 41.1% of inferred |
| Inferred-only (FP) | 9410 | 58.9% of inferred |
| Resolved-only (missed) | 577 | 8.1% of resolved |

## Interpretation

### Why confirmation rate is low for `calls`

Tree-sitter identifies function calls syntactically (any `identifier()` pattern) but
cannot resolve which package the callee belongs to. It generates candidate edges using
name matching heuristics. Go/ast resolves the actual call target through type information.
The mismatch means tree-sitter over-generates call edges (multiple candidates per call site)
while go/ast produces one precise edge per call.

### Why `imports` has high confirmation

Import statements are unambiguous in Go syntax. Both extractors detect them reliably.
The tree-sitter inferred-only imports are likely aliased or dot imports where the
hash computation differs.

### What this means for knowing's two-tier strategy

The 41.1% confirmation rate for calls+imports means tree-sitter provides
a noisy but non-zero signal. The lower confidence score (0.7 vs 1.0) causes the
context engine to rank tree-sitter-only edges below confirmed edges in scoring.
This is the intended behavior: tree-sitter provides fast initial coverage that
the go/ast extractor later refines with precision.

The value proposition is speed vs accuracy: tree-sitter runs in milliseconds per file,
while go/ast requires loading the full dependency graph (~30s for this repo).
For real-time IDE feedback, the noisy tree-sitter signal is better than no signal.
For batch indexing (CI, nightly), go/ast provides ground truth.

## Reproducibility

```bash
GOWORK=off go test ./bench/edge-accuracy/ -v -timeout 5m
```
