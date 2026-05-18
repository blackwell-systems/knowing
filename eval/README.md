# Eval Framework

Standardized retrieval accuracy evaluation for knowing's context engine. Measures how well `context_for_task` surfaces the right symbols for a given development task.

## Quick Start

```bash
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

Results are printed to stdout and written to `eval/FINDINGS.md`.

## How It Works

1. Indexes the knowing repo into a temp database (tree-sitter only, ~2s)
2. Loads all YAML fixtures from `eval/fixtures/{easy,medium,hard}/`
3. For each fixture, calls `ForTask` with the task description
4. Compares the top-10 returned symbols against hand-curated ground truth
5. Reports Precision@10, Recall@10, and MRR per fixture and per tier

## Metrics

| Metric | What it measures |
|--------|-----------------|
| **Precision@10** | Of the top 10 results, what fraction is relevant? |
| **Recall@10** | Of all ground-truth symbols, what fraction appears in top 10? |
| **MRR** | 1/rank of the first relevant result (higher = faster orientation) |

## Fixture Format

```yaml
task: "Description of the development task"
difficulty: easy | medium | hard
tags: [single-package, cross-package, runtime, etc.]
ground_truth:
  - package.SymbolName
  - package.AnotherSymbol
```

Ground truth uses flexible matching: `package.Symbol` matches qualified names like `internal/package.Type.Symbol` (handles method-on-struct patterns where the receiver type is part of the indexed name).

## Difficulty Tiers

| Tier | What it tests | Target P@10 |
|------|--------------|-------------|
| **Easy** | All relevant symbols in one package. Keywords should find them directly. | > 60% |
| **Medium** | Symbols span 2-3 packages. Requires graph walk to discover cross-package relationships. | > 30% |
| **Hard** | Symbols span 4+ packages across runtime, daemon, resolver, and store. Requires deep graph traversal. | > 15% |

## Current Baseline

| Tier | P@10 | R@10 | MRR |
|------|------|------|-----|
| Easy | 42.0% | 79.0% | 0.67 |
| Medium | 24.0% | 34.8% | 0.34 |
| Hard | 10.0% | 12.2% | 0.27 |
| **Overall** | **25.3%** | **42.0%** | **0.42** |

**Changes from prior baseline (18% overall):**
- Fixed eval matching to handle `package.Type.Method` qualified names (+7pp across all tiers)
- Added mock/stub/fake symbol filtering (removes noise from test helpers)
- Added BM25 FTS5 index with CamelCase-aware tokenization (helps when tiers find < 8 candidates)
- Fixed stale ground truth in runtime trace fixture

## Adding Fixtures

Create a new YAML file in the appropriate tier directory:

```bash
eval/fixtures/easy/06-my-new-fixture.yaml
eval/fixtures/medium/06-my-new-fixture.yaml
eval/fixtures/hard/06-my-new-fixture.yaml
```

Ground truth should be "what symbols would an expert developer actually need to see to accomplish this task?" Not exhaustive (every possible helper), but the core symbols that orient you.

## Improving Scores

Each engine improvement should be measurable through this eval:

| Improvement | Expected effect |
|-------------|----------------|
| Better keyword extraction | Easy tier improves (seeds find more relevant symbols) |
| BM25 FTS5 index | Medium/hard improve (finds symbols by signature/path content) |
| Session-aware boosts | All tiers improve for repeat queries within a session |
| LLM query rewriting | Hard tier improves ("auth handling" -> specific symbol names) |
| Embeddings (MiniLM) | Hard tier improves (concept-level matching) |
| Community-scoped walking | Hard tier improves (walks stay within relevant subsystems) |
| RRF fusion | All tiers improve (combines multiple seed sources intelligently) |

## Cross-Tool Comparison

The fixture format is tool-agnostic. To benchmark another tool:

1. For each fixture, query the tool with the task description
2. Collect its top-10 results as qualified symbol names
3. Run the same substring matching against ground truth
4. Report P@10, R@10, MRR

This enables direct comparison: knowing vs grep-baseline, vs CGC, vs Gortex, vs any tool that returns ranked code context.
