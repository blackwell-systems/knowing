# Edge Accuracy Benchmark: Tree-Sitter vs Go/AST

## Methodology

This benchmark compares the accuracy of knowing's two Go extraction tiers:

1. **Tree-sitter extractor** (`gotsextractor`): Fast, syntax-only extraction using
   tree-sitter queries. Produces edges with provenance `ast_inferred` and confidence
   0.7. Does not resolve types or cross-package references.

2. **Go/AST extractor** (`goextractor`): Full type-resolution extraction using
   `golang.org/x/tools/go/packages`. Produces edges with provenance `ast_resolved`
   and confidence 1.0. Serves as ground truth for Go code.

### Process

1. Index the knowing repository into a temporary DB using only the tree-sitter extractor
2. Index the same repository into a separate temporary DB using only the go/ast extractor
3. Query all edges from both databases
4. Match edges by identity tuple: `(source_hash, target_hash, edge_type)`
   (provenance is excluded from matching since it differs by design)
5. Compute overlap metrics

### Metrics

- **Confirmation rate**: % of tree-sitter edges that also appear in go/ast output.
  Higher is better; indicates tree-sitter is finding real relationships.
- **False positive rate**: % of tree-sitter edges with NO match in go/ast.
  Lower is better; indicates tree-sitter edges that may be incorrect.
- **Miss rate**: % of go/ast edges NOT found by tree-sitter.
  Lower is better; indicates relationships tree-sitter cannot detect.

## Results

Run the benchmark to populate these results:

```
GOWORK=off go test ./bench/edge-accuracy/ -v -timeout 10m
```

Results will be printed to test output with the format:

```
=== Edge Accuracy Results ===

  OVERALL:
    Tree-sitter (ast_inferred): N edges
    Go/ast (ast_resolved):      N edges
    Confirmed (both):           N (X% of inferred)
    Inferred-only (FP):         N (X% of inferred)
    Resolved-only (missed):     N (X% of resolved)
```

## Edge Type Breakdown

The benchmark reports per-edge-type metrics for:

| Edge Type    | Description                                    |
|-------------|------------------------------------------------|
| `calls`     | Function/method call relationships             |
| `imports`   | Package import relationships                   |
| `implements`| Interface implementation relationships         |
| `references`| Type/variable reference relationships          |

Expected patterns:
- **imports**: High confirmation rate (both extractors detect import statements reliably)
- **calls**: Moderate confirmation rate (tree-sitter uses heuristics for cross-package calls)
- **implements**: Lower confirmation rate (requires type resolution that tree-sitter lacks)
- **references**: Variable (tree-sitter may miss complex reference patterns)

## Interpretation

The two-tier extraction strategy is designed around a speed/accuracy tradeoff:

- **Tree-sitter (fast path)**: Runs in milliseconds, suitable for real-time feedback.
  Accepts lower accuracy (confidence 0.7) in exchange for speed.
- **Go/AST (precise path)**: Runs in seconds, used for batch indexing.
  Provides full type resolution (confidence 1.0).

A confirmation rate above 60% validates that tree-sitter extraction provides
meaningful signal even without type resolution. False positives are mitigated
by the lower confidence score (0.7 vs 1.0), which causes the context engine
to rank tree-sitter-only edges below confirmed edges.

Misses (edges only in go/ast) represent the value-add of running the full
extractor: these are relationships that require type information to discover,
such as interface implementations and cross-package method calls resolved
through type aliases.
