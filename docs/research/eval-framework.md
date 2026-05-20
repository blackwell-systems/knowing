# Eval Framework Guide

This document covers knowing's retrieval accuracy evaluation framework: how it works, how to run it, how to interpret results, and how to extend it with new fixtures and experiments.

## Purpose

The eval framework measures how well knowing's context engine (`ForTask`) surfaces the right symbols for a given development task. It serves three functions:

1. **Regression prevention.** Every engine change must produce measurable results against a fixed set of fixtures. If a change lowers scores, it is reverted or reworked.
2. **Improvement validation.** New retrieval features (BM25, equivalence classes, RRF fusion) are only shipped after demonstrating measurable gains on the eval.
3. **Baseline tracking.** The framework auto-generates `eval/FINDINGS.md` with per-fixture and per-tier metrics, providing a reproducible snapshot of engine quality.

## Metrics

The framework reports three metrics per fixture and per tier.

### Precision@10 (P@10)

Of the top 10 symbols returned by the engine, what fraction is relevant?

```
P@10 = (relevant symbols in top 10) / min(10, total returned)
```

High P@10 means the engine does not waste slots on irrelevant symbols. This matters because token budgets are finite; every irrelevant symbol displaces a useful one.

### Recall@10 (R@10)

Of all ground-truth symbols for a fixture, what fraction appears in the top 10?

```
R@10 = (relevant symbols in top 10) / (total ground-truth symbols)
```

High R@10 means the engine finds most of the symbols an expert would want to see. Note: R@10 can exceed 100% when multiple returned symbols match the same ground-truth entry via substring matching.

### Mean Reciprocal Rank (MRR)

1 divided by the rank of the first relevant result, or 0 if no relevant result appears in the top 10.

```
MRR = 1 / rank_of_first_hit    (or 0 if no hit)
```

MRR measures how quickly a developer gets oriented. An MRR of 1.0 means the very first result is relevant; 0.5 means the second result is the first hit. This metric captures whether the engine puts the most important symbol at the top, not just somewhere in the list.

### Why these three?

- **P@10** penalizes noise (returning irrelevant symbols).
- **R@10** penalizes gaps (missing important symbols).
- **MRR** penalizes poor ordering (burying the best result).

Together they cover the three failure modes of a retrieval system: too much noise, missing results, and bad ranking.

## Fixture Format

Each fixture is a YAML file in `eval/fixtures/{easy,medium,hard}/`. The format:

```yaml
task: "Description of the development task"
difficulty: easy | medium | hard
tags: [single-package, cross-package, runtime, etc.]
ground_truth:
  - package.SymbolName
  - package.AnotherSymbol
```

### Example: easy fixture

```yaml
task: "Add a new MCP tool that returns symbol documentation"
difficulty: easy
tags: [single-package, mcp, new-feature]
ground_truth:
  - mcp.registerTools
  - mcp.Server
  - mcp.NewServer
  - mcp.requireStringArg
  - mcp.contextForTaskTool
```

### Example: medium fixture

```yaml
task: "Implement HITS hub/authority reranking in the context engine ranking pipeline"
difficulty: medium
tags: [cross-package, context, algorithm]
ground_truth:
  - context.RankSymbols
  - context.HITSScores
  - context.ComputeHITS
  - context.ContextEngine
  - context.ForTask
  - context.RankedSymbol
  - context.packIntoBudget
  - context.RandomWalkWithRestart
```

### Example: hard fixture

```yaml
task: "Ingest OpenTelemetry spans and create runtime edges with confidence scoring and decay"
difficulty: hard
tags: [cross-package, trace, store, daemon, runtime]
ground_truth:
  - trace.Ingestor
  - trace.IngestSpans
  - trace.SymbolResolver
  - trace.ConfidenceFromCount
  - trace.OTLPReceiver
  - store.PutEdge
  - daemon.traceIngestLoop
  - types.Edge
```

## Difficulty Tiers

| Tier | Definition | Characteristics | Target P@10 |
|------|-----------|-----------------|-------------|
| **Easy** | All relevant symbols live in one package | Keywords should find them directly; seed quality is sufficient | > 60% |
| **Medium** | Symbols span 2-3 packages | Requires graph walk to discover cross-package relationships | > 30% |
| **Hard** | Symbols span 4+ packages across runtime, daemon, resolver, store | Requires deep graph traversal, vocabulary bridging, or multi-hop reasoning | > 15% |

The tier system exists to diagnose where the engine fails. A change that improves hard but destroys easy is shipping noise, not value. A change that only improves easy is polishing what already works.

## Matching Logic: isRelevant

The `isRelevant` function (in `eval/eval_test.go`) determines whether a returned qualified name matches a ground-truth entry. It uses two strategies:

### 1. Direct substring match

```go
strings.Contains(qualifiedName, gt)
```

If the ground-truth string `"context.ContextEngine"` appears anywhere in the fully qualified name `"github.com/blackwell-systems/knowing/internal/context.ContextEngine"`, it matches.

### 2. Package.Type.Method decomposition

Ground-truth entries use the format `package.Symbol`, but the indexed qualified names include the receiver type: `internal/store.SQLiteStore.NodesByName`. The matching logic handles this by splitting the ground-truth entry at the first dot:

```go
pkg := gt[:dot]   // e.g., "store"
sym := gt[dot+1:] // e.g., "NodesByName"
```

Then it checks that the qualified name contains `/<pkg>.` (package path segment) and ends with `.<sym>` (the target symbol name). This means:

- `store.NodesByName` matches `internal/store.SQLiteStore.NodesByName`
- `context.ForTask` matches `internal/context.ContextEngine.ForTask`
- `trace.IngestSpans` matches `internal/trace.Ingestor.IngestSpans`

A secondary check also handles sub-packages where the separator is different.

**Why this matters:** Experiment 4 in `EXPERIMENTS.md` showed that fixing this matching logic was worth +8pp overall. The eval was undercounting hits because it could not match methods through their receiver types. Always verify that `isRelevant` handles your ground-truth format before interpreting results.

## Running the Eval

### Full eval (indexes from scratch)

```bash
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

This indexes the knowing repo into a temporary SQLite database using tree-sitter extraction, loads all fixtures, runs `ForTask` for each, and reports metrics. Results are printed to stdout and written to `eval/FINDINGS.md`.

### With embeddings

```bash
KNOWING_EMBEDDINGS=1 GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

Enables the optional vector search channel (currently weight 0, awaiting a code-tuned model).

### Run a specific test

```bash
GOWORK=off go test ./eval/ -v -count=1 -run TestEval -timeout 5m
```

### Interpreting output

The test prints a per-fixture table followed by a per-tier summary:

```
=== EVAL RESULTS ===

Task                                               |  P@10  |  R@10  |  MRR  | Tier
---------------------------------------------------+--------+--------+-------+------
Add a new MCP tool that returns symbol doc...      |  90.0% |  180.0%|  1.00 | easy
...

=== PER-TIER SUMMARY ===

Tier     |  P@10  |  R@10  |  MRR  | N
---------+--------+--------+-------+---
easy     |  28.5% |  67.4% |  0.52 | 20
medium   |  29.0% |  50.1% |  0.49 | 20
hard     |  22.0% |  27.4% |  0.35 | 15

OVERALL  |  26.9% |  50.8% |  0.46 | 55 fixtures
```

Key things to check:

- **Did any tier regress?** Compare against the baseline in `eval/README.md`.
- **Which fixtures scored 0%?** These are the ones the engine completely misses. They are candidates for targeted improvements.
- **MRR per tier.** Low MRR with decent R@10 means the engine finds the symbols but buries them in the ranking.

## Cross-Repo Eval

The cross-repo eval (`eval/crossrepo_test.go`) tests the engine on an external codebase (gortex) with no knowing-specific equivalence classes. This validates that the general pipeline (keyword tiers, BM25, bigram compounds, RRF) works on code the engine has never been tuned for.

### Running

```bash
GOWORK=off go test ./eval/ -v -count=1 -run TestCrossRepo_Gortex -timeout 5m
```

Requires the gortex repo to be cloned locally. The test skips gracefully if the repo is not available.

### Fixture tiers

Cross-repo fixtures use different tier names:

| Tier | Description | Example |
|------|------------|---------|
| **exact** | Direct symbol name queries | `"BM25Backend"` |
| **concept** | Natural-language queries | `"combine text and vector search with RRF"` |
| **multi_hop** | Relational queries spanning multiple symbols | `"all language extractors registered in the system"` |

### Metric

Cross-repo eval uses **any-hit R@10**: did at least one expected symbol appear in the top 10? This is simpler than the per-fixture metrics because the goal is testing generalization, not precision.

### Current results

| Tier | R@10 | N |
|------|------|---|
| exact | 60.0% | 10 |
| concept | 20.0% | 10 |
| multi_hop | 60.0% | 10 |
| **Overall** | **46.7%** | **30** |

Results are written to `eval/CROSS_REPO_FINDINGS.md`.

## Fixture Verification: TestVerifyFixtures

The `TestVerifyFixtures` test (in `eval/verify_test.go`) checks that every ground-truth symbol in every fixture actually exists in the indexed graph. It indexes the repo, loads all nodes, and for each ground-truth entry, verifies that at least one indexed node matches via `isRelevant`.

```bash
GOWORK=off go test ./eval/ -v -count=1 -run TestVerifyFixtures
```

Symbols that fail verification are printed as:

```
MISSING [medium] Implement HITS hub/authority rerankin...: context.NonExistentSymbol
```

Run this after adding new fixtures or renaming symbols in the codebase. A missing symbol means either the ground truth is wrong or the extractor is not capturing that symbol.

## Current Baseline

As of the latest eval run (55 fixtures total: 20 easy, 20 medium, 15 hard):

| Tier | P@10 | R@10 | MRR | Fixtures |
|------|------|------|-----|----------|
| Easy | 28.5% | 67.4% | 0.52 | 20 |
| Medium | 29.0% | 50.1% | 0.49 | 20 |
| Hard | 22.0% | 27.4% | 0.35 | 15 |
| **Overall** | **26.9%** | **50.8%** | **0.46** | **55** |

Note: the easy tier regressed from a previous high of 39.0% (see experiment history). This is a known issue to investigate; it may be caused by recent extractor additions changing the graph's edge distribution.

The shipped pipeline includes:

- 5-tier keyword matching (exact > prefix > substring > path > interface)
- BM25 FTS5 index with CamelCase-aware tokenization
- Bigram compound keyword extraction ("blast radius" becomes BlastRadius)
- Equivalence class seed retrieval (20+ concepts, 200+ phrases mapped to target symbols)
- Weighted RRF fusion (tier 3x, equivalence 2x, BM25 1x)
- Mock/stub/fake symbol filtering
- Session-aware boosts (exponential decay, 3-min half-life)
- Doc comment extraction (Node.Doc field)
- Embeddings via HNSW (opt-in, weight 0, awaiting code-tuned model)

## Experiment Methodology

Every retrieval pipeline change follows a four-step process:

### 1. Hypothesis

State what you expect and why. Example: "BM25 full-text search over CamelCase-split names will find symbols that LIKE-based tiers miss."

### 2. Measure

Run the full eval before and after. Record per-tier P@10, R@10, and MRR. Note which specific fixtures changed.

### 3. Conclude

Did the hypothesis hold? Was the effect positive, negative, or neutral per tier? Was there a tradeoff (e.g., hard improved but easy regressed)?

### 4. Document

Add an entry to `eval/EXPERIMENTS.md` with date, hypothesis, what was tried, per-tier results, and conclusion. This prevents re-running failed approaches.

### Template

```markdown
## Experiment N: Short title

**Date:** YYYY-MM-DD
**Hypothesis:** What you expect and why.
**What:** Brief description of the implementation.
**Result:**
- Easy: X% -> Y% (+/-Zpp)
- Medium: X% -> Y% (+/-Zpp)
- Hard: X% -> Y% (+/-Zpp)

**Conclusion:** Did it work? Why or why not? Keep or revert?
```

## Key Lessons from 21 Experiments

The `eval/EXPERIMENTS.md` file documents 21 experiments run against the framework. Here are the distilled lessons.

### What works

1. **Equivalence classes are the highest-ROI feature.** Experiment 18 added 20 hand-curated concept-to-symbol mappings and gained +8pp on the hard tier (10% to 18%), +2.5pp on medium, +2pp on easy. Local, deterministic, zero dependencies.

2. **Expanding phrases in existing equivalence classes is cheap and safe.** Experiment 19 added phrases to existing concepts and a new EXTRACTOR concept, gaining another +3.3pp on hard with near-zero risk.

3. **Bigram compound keywords crack previously-impossible fixtures.** Experiment 8 joined adjacent words into CamelCase compounds ("blast radius" becomes "BlastRadius"), improving MRR by +0.04 and enabling fixtures that were stuck at 0%.

4. **Weighted RRF fusion works when asymmetric.** Experiments 6 and 7 showed that tiered results must be weighted higher than BM25. A 2:1 or 3:1 ratio (tier:BM25) preserves easy-tier precision while letting BM25 help hard. Equal weights (1:1) destroy easy (-28pp).

5. **Mock/stub filtering improves result quality.** Experiment 5 filtered out test mock implementations that were ranking above real implementations due to high caller counts in test files.

### What does not work

6. **Off-the-shelf embeddings do not help code retrieval.** Experiments 9-12 tested MiniLM-L6-v2 and BGE-small-en-v1.5 at various weights and with enriched text. Best case: marginal +2pp on hard with -2pp on easy. Worst case: -8pp on easy, -6pp on medium and hard. General-purpose models do not understand code vocabulary ("blast radius" != "TransitiveCallers").

7. **RWR parameter tuning is a dead end when seeds are wrong.** Experiments 13-16 tried confidence-weighted transitions, lower alpha, adaptive alpha, and dead-end handling. Pattern: +1pp on hard, -11pp on easy. The walk cannot fix fundamentally wrong seeds.

8. **Naive BM25 concatenation dilutes strong tiered seeds.** Experiment 1 showed that always adding BM25 results without fusion caused a -16pp regression on easy. BM25 must go through RRF fusion with lower weight.

9. **Untargeted text enrichment of BM25 hurts precision.** Experiments 17 and 20 added doc comments and neighbor symbol names to the FTS index. Both were net-negative because common words dilute search specificity and high-degree generic nodes appear as neighbors of everything.

### Common pitfalls

10. **Fix the eval before fixing the engine.** Experiment 4 showed that the `isRelevant` matching function was undercounting hits because it could not handle `package.Type.Method` qualified names. Fixing the eval was worth +8pp overall, more than any single engine change.

11. **Tradeoffs between tiers are real.** Many changes help hard at the expense of easy. Always report all three tiers, not just the one you are trying to improve.

12. **Targeted beats untargeted.** Equivalence classes (explicit phrase-to-symbol mapping) outperform all forms of "add more text to the index" (doc comments, neighbor names, enriched BM25). This principle applies broadly: specific, curated knowledge beats generic text expansion.

## How to Add Fixtures

### Guidelines for good ground truth

1. **Ask "what would an expert need?"** Ground truth should be the core symbols that orient a developer on the task, not an exhaustive list of every possible helper.

2. **Use the `package.Symbol` format.** The matching logic handles receiver types automatically: `store.NodesByName` matches `store.SQLiteStore.NodesByName`.

3. **Include 3-8 ground-truth symbols.** Fewer than 3 makes R@10 noisy (one hit = 33%). More than 8 makes perfect recall nearly impossible in a top-10 window.

4. **Verify the fixture.** Run `TestVerifyFixtures` to confirm all ground-truth symbols exist in the indexed graph.

5. **Place in the right tier.** Single-package tasks are easy. Cross-package (2-3 packages) are medium. Cross-system (4+ packages, runtime/daemon/resolver) are hard.

6. **Use descriptive filenames.** Follow the existing pattern: `NN-short-description.yaml` (e.g., `06-my-new-fixture.yaml`).

### Steps

```bash
# 1. Create the fixture file
cat > eval/fixtures/medium/21-my-new-fixture.yaml << 'EOF'
task: "Description of the development task"
difficulty: medium
tags: [cross-package, relevant-tags]
ground_truth:
  - package.SymbolOne
  - package.SymbolTwo
  - otherpackage.SymbolThree
EOF

# 2. Verify ground truth exists in the graph
GOWORK=off go test ./eval/ -v -count=1 -run TestVerifyFixtures

# 3. Run the full eval to see baseline impact
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

## How to Add a New Experiment

1. **Establish the baseline.** Run the full eval and record per-tier numbers before making any changes.

2. **Implement the change.** Modify the context engine, indexer, or retrieval pipeline as needed.

3. **Run the eval.** Compare per-tier P@10, R@10, and MRR against the baseline. Note which specific fixtures improved or regressed.

4. **Decide: keep or revert.** If the change helps one tier but hurts another, quantify the tradeoff. A +2pp gain on hard that costs -10pp on easy is usually not worth shipping.

5. **Document in EXPERIMENTS.md.** Use the template from the methodology section. Include the date, hypothesis, per-tier delta, and a clear conclusion.

6. **Update README.md baseline.** If the change ships, update the baseline numbers and pipeline description in `eval/README.md`.

7. **Commit FINDINGS.md.** The eval auto-generates `eval/FINDINGS.md`; commit the updated version so others can see the current state without re-running.

### Checklist

- [ ] Baseline recorded before changes
- [ ] Per-tier results documented (all three tiers, not just the target)
- [ ] Specific fixture changes noted (which went from 0% to non-zero, which regressed)
- [ ] Entry added to `eval/EXPERIMENTS.md`
- [ ] If shipped: `eval/README.md` baseline updated
- [ ] If reverted: conclusion explains why, so future work does not repeat the approach

## File Reference

| File | Purpose |
|------|---------|
| `eval/eval_test.go` | Main eval runner, `isRelevant` matching, `writeEvalFindings` |
| `eval/verify_test.go` | Fixture verification (checks ground-truth symbols exist in graph) |
| `eval/crossrepo_test.go` | Cross-repo eval on external codebases (gortex) |
| `eval/EXPERIMENTS.md` | Log of all experiments with hypotheses, results, conclusions |
| `eval/README.md` | Overview with current baseline and pipeline description |
| `eval/FINDINGS.md` | Auto-generated per-fixture and per-tier results |
| `eval/CROSS_REPO_FINDINGS.md` | Auto-generated cross-repo results |
| `eval/fixtures/easy/*.yaml` | 20 easy-tier fixtures (single-package tasks) |
| `eval/fixtures/medium/*.yaml` | 20 medium-tier fixtures (cross-package tasks) |
| `eval/fixtures/hard/*.yaml` | 15 hard-tier fixtures (cross-system tasks) |
