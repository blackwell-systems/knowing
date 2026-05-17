# Hook Experiment: Findings

## Summary

Automatic context injection via Claude Code PreToolUse hooks is **not cost-effective** at the current state of the context engine. The experiment produced clear numbers that informed this conclusion.

## Experiment Design

The hook intercepts every file Edit/Write operation and injects graph-ranked context (symbols related to the file being edited) before the agent sees the tool result. The hypothesis: pre-loading context saves the agent from making explicit `context_for_task` calls, reducing total token spend.

Two benchmarks were run:

1. **Precision/Recall benchmark**: does the hook inject relevant symbols?
2. **Cost comparison benchmark**: does the hook save net tokens vs manual calls?

## Results

### Precision & Recall (10 tasks, 800 token budget)

| Metric | Value |
|--------|-------|
| Mean precision | 32.1% |
| Mean recall | 69.3% |
| Symbols injected per edit | ~21 |
| Latency p95 | 347ms |

Interpretation: the hook provides ~70% of the symbols the task needs, but only ~32% of what it injects is directly relevant. The rest is structurally related (same package, callers) but not task-critical.

### Cost Comparison (800 token hook vs 4000 token manual call)

| Metric | Value |
|--------|-------|
| Tasks where hook eliminates manual call | 2/10 (20%) |
| Total hook cost (automatic, all tasks) | 1,306 tokens |
| Total manual cost (explicit calls) | 3,650 tokens |
| Net token impact | **-910 tokens (hooks are a net cost)** |

The hook only covers 50%+ of the manual response in 2 out of 10 cases. In the other 8, the agent still needs to call `context_for_task` explicitly, meaning it pays the hook cost AND the manual cost.

## Why Hooks Fail

The fundamental issue is **precision at small budgets**. At 800 tokens (~21 symbols), the context engine returns symbols that are graph-proximate to the query but not necessarily task-critical. The ranking treats all nearby symbols as roughly equal rather than prioritizing the structurally most important ones.

Specific failure modes:

1. **Broad package match**: querying "server" returns all 25 handler methods when the task only needs 3 of them
2. **Insufficient differentiation**: symbols at scores 0.54-0.67 are nearly indistinguishable, so the top-20 cutoff is arbitrary
3. **No task specificity**: the hook uses the filename as the query, which matches broadly rather than understanding what the edit is about

## What Would Make Hooks Viable

1. **HITS reranking** (hub/authority scoring): separate "structurally important" from "merely nearby." Authority nodes (heavily called) should rank above leaf functions.

2. **Edit-aware seeding**: instead of querying by filename, query by the symbols actually being modified (available from the Edit tool input). This would dramatically improve precision.

3. **Adaptive budget**: start with 400 tokens on first edit to a file. If the agent subsequently calls `context_for_task` anyway, increase to 1200 on the next edit to the same file. Learn per-session what budget actually prevents manual calls.

4. **Session dedup compounding**: the first edit to a file pays full cost. Subsequent edits to the same file pay near-zero (GCF session statefulness). The current benchmark measures per-edit cost but not session-level amortization.

## Decision

Hooks are shelved as experimental. The code remains in `hooks/` with full measurement infrastructure. Re-evaluate after:
- HITS reranking is implemented (precision improvement)
- Edit-aware seeding is available (query by modified symbols, not filename)
- Context engine achieves >= 50% precision at 800 tokens on the benchmark

## Paths Forward (Instead of Hooks)

### Path 1: Fix the root cause (context engine precision)

The hook failed because of ranking quality, not delivery mechanism. At 800 tokens, the engine can't distinguish "essential" from "nearby." Implementing HITS hub/authority scoring on the RWR subgraph would separate high-traffic symbols (the ones you actually need to understand) from leaf functions (related but not critical). If precision reaches 50%+ at 800 tokens, hooks become viable again.

### Path 2: Make explicit calls so good agents always use them

Instead of injecting silently, make `context_for_task` and `context_for_files` return such high-quality results that agents learn to call them as their first action. The MCP prompts (`refactor_safely`, `review_pr`, `investigate_dead_code`) already encode this pattern: the prompt template includes a context call as step 1. The agent decides when it needs context rather than receiving it unconditionally.

This path accepts that the agent should be in control. The product's job is to make the explicit call fast, cheap, and excellent, not to guess when context is needed.

### Path 3: Focus on high-value moments, not every edit

The highest-ROI integration point is not per-edit injection. It's single calls at decision points:

- **PR open**: `context_for_pr` provides full blast radius, cross-repo impact, and affected tests. One call saves the reviewer 30 minutes of manual investigation.
- **Incident triage**: `blast_radius` + `runtime_traffic` for the failing symbol. One call gives the on-call engineer the full dependency chain with production traffic weights.
- **Refactor planning**: `cross_repo_callers` before changing a shared interface. One call reveals every consumer across all repos.

These moments are high-stakes, low-frequency, and high-information-density. A 4000-token response at PR-open time is worth more than fifty 800-token per-edit injections because the agent (or human) is making a decision that affects the entire system.

## How to Re-Run

```bash
# Index the repo
knowing index -db knowing.db .

# Precision/recall benchmark
KNOWING_DB=knowing.db go test -tags hookbench ./hooks/benchmark/ -v -run TestHookBenchmark

# Cost comparison benchmark  
KNOWING_DB=knowing.db go test -tags hookbench ./hooks/benchmark/ -v -run TestHookCostComparison

# Try different budgets
KNOWING_DB=knowing.db KNOWING_HOOKS_BUDGET=1200 go test -tags hookbench ./hooks/benchmark/ -v
```

## Lessons

1. **Measure before shipping.** The hook "felt" useful (80% recall sounds great) but the cost benchmark revealed it doesn't pay for itself.
2. **Precision matters more than recall for automatic injection.** A tool the agent calls explicitly can afford low precision (the agent asked for it). A tool that injects automatically must be precise or it wastes budget.
3. **The problem is ranking, not retrieval.** The context engine finds relevant symbols; it just can't distinguish "essential" from "related" at small budgets.
