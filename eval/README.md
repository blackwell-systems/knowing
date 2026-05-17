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

Ground truth uses substring matching: a returned symbol matches if any ground truth entry appears as a substring of its qualified name.

## Difficulty Tiers

| Tier | What it tests | Target P@10 |
|------|--------------|-------------|
| **Easy** | All relevant symbols in one package. Keywords should find them directly. | > 60% |
| **Medium** | Symbols span 2-3 packages. Requires graph walk to discover cross-package relationships. | > 30% |
| **Hard** | Symbols span 4+ packages across runtime, daemon, resolver, and store. Requires deep graph traversal. | > 15% |

## Current Baseline

| Tier | P@10 | R@10 | MRR |
|------|------|------|-----|
| Easy | 40.0% | 75.7% | 0.56 |
| Medium | 12.0% | 17.5% | 0.32 |
| Hard | 2.0% | 2.2% | 0.20 |
| **Overall** | **18.0%** | **31.8%** | **0.36** |

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
| Lower RWR threshold | Medium tier improves (more candidates in pool) |
| Feedback accumulation | All tiers improve over repeated runs |
| Community-scoped walking | Hard tier improves (walks stay within relevant subsystems) |
| Verb-pattern seeding | Easy/medium improve ("Add X" finds `types.X`, `NewX`) |

## Cross-Tool Comparison

The fixture format is tool-agnostic. To benchmark another tool:

1. For each fixture, query the tool with the task description
2. Collect its top-10 results as qualified symbol names
3. Run the same substring matching against ground truth
4. Report P@10, R@10, MRR

This enables direct comparison: knowing vs grep-baseline, vs CGC, vs Gortex, vs any tool that returns ranked code context.
