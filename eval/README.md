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

| Tier | P@10 | R@10 | MRR | Fixtures |
|------|------|------|-----|----------|
| Easy | 38.5% | 95.2% | 0.63 | 20 |
| Medium | 32.0% | 51.4% | 0.62 | 20 |
| Hard | 18.0% | 21.1% | 0.27 | 15 |
| **Overall** | **30.5%** | **59.0%** | **0.53** | **55** |

**Pipeline (all shipped):**
- 5-tier keyword matching (exact > prefix > substring > path > interface)
- BM25 FTS5 index with CamelCase-aware tokenization
- Bigram compound keyword extraction ("blast radius" -> BlastRadius)
- Equivalence class seed retrieval (20 concepts, 200+ phrases -> target symbols)
- Weighted RRF fusion (tier 3x, equivalence 2x, BM25 1x)
- Session-aware boosts (exponential decay, 3-min half-life)
- Embeddings via HNSW (opt-in: KNOWING_EMBEDDINGS=1, weight 0, awaiting code-tuned model)
- Mock/stub/fake symbol filtering
- Flexible eval matching (handles package.Type.Method qualified names)
- Doc comment extraction (Node.Doc field, used in embed text)

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

| Improvement | Status | Expected effect |
|-------------|--------|----------------|
| Better keyword extraction | **shipped** | Easy tier improves (seeds find more relevant symbols) |
| BM25 FTS5 index | **shipped** | Medium/hard improve (finds symbols by signature/path content) |
| Session-aware boosts | **shipped** | All tiers improve for repeat queries within a session |
| Equivalence class seeds | **shipped** | Hard tier +8pp (bridges vocabulary gap for known concepts) |
| 4-channel RRF fusion | **shipped** | All tiers improve (combines tiered, BM25, vector, equivalence) |
| Doc comment extraction | **shipped** | Enriches embed text for future code-tuned models |
| LLM query rewriting | planned | Hard tier improves ("auth handling" -> specific symbol names) |
| Embeddings (code-tuned) | planned | Hard tier improves (concept-level matching). BGE-small infra shipped, disabled. |
| Community-scoped walking | planned | Hard tier improves (walks stay within relevant subsystems) |

## Cross-Tool Comparison

The fixture format is tool-agnostic. To benchmark another tool:

1. For each fixture, query the tool with the task description
2. Collect its top-10 results as qualified symbol names
3. Run the same substring matching against ground truth
4. Report P@10, R@10, MRR

This enables direct comparison: knowing vs grep-baseline, vs CGC, vs Gortex, vs any tool that returns ranked code context.
