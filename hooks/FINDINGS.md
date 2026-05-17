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

## Alternative Model: Behavioral Nudging (Serena Approach)

Research into Serena (an MCP server for code intelligence) reveals a fundamentally different hook architecture that avoids the cost problem entirely.

### How Serena Does It

Serena's PreToolUse hook does NOT inject context. Instead:

1. It counts consecutive uses of non-symbolic tools (grep, read, cat)
2. After a threshold (3 greps, 3 reads, or 4 combined), it **denies** the tool call with a message: "use symbolic tools instead"
3. Rate-limited to one nudge per 2-minute window
4. Counters reset when the agent uses a Serena symbolic tool

The hook is a behavioral redirect, not a context injection. It costs zero tokens (the nudge message is tiny). The agent is trained to reach for symbolic tools first; the hook catches when it forgets.

### Comparison of Models

| | Context Injection (our approach) | Behavioral Nudge (Serena) |
|--|----------------------------------|--------------------------|
| Token cost per invocation | 800 (fixed, every edit) | ~0 (only fires on threshold) |
| Precision requirement | Must be high (wasted tokens if not) | N/A (no content to be precise about) |
| Agent autonomy | Reduced (receives unsolicited content) | Preserved (agent decides when to call) |
| Failure mode | Wasted tokens on irrelevant context | Agent ignores nudge, uses grep anyway |
| Best for | Highly predictable edits | General-purpose coding workflows |
| Our data says | Net cost of -910 tokens across 10 tasks | Not tested (no implementation yet) |

### What a Nudge-Based Hook Would Look Like for knowing

```bash
# Pseudocode for a nudge-based hook:
# 1. Track: has the agent called context_for_task or context_for_files recently?
# 2. If it's editing files without having called context tools in the last 3 edits:
#    -> Output a nudge: "Consider calling context_for_task before editing"
# 3. If it has called context tools recently: do nothing (agent is informed)
```

Cost: effectively zero tokens (nudge is a single sentence, fires rarely).
Risk: agent may ignore the nudge and edit blind anyway.
Upside: no wasted tokens, no precision problem, agent stays in control.

### Which Model Wins?

Unknown. Both have tradeoffs:

- **Injection wins if** precision can be pushed above 50% (HITS alone didn't achieve this, but edit-aware seeding with a single well-matched symbol showed 50% precision on specific tasks like "SQLiteStore" and "SnapshotManager").
- **Nudging wins if** the agent reliably responds to nudges and calls `context_for_task` on its own (depends on agent behavior, which we can't benchmark without a full agent loop).

Both models remain available in the codebase. The injection hook is at `hooks/knowing-pre-edit`. A nudge-based hook could be added alongside without conflict.

### Edit-Aware Seeding Results

When the hook extracts the primary symbol being modified (simulating reading `old_string` from the Edit tool input) and that symbol is well-matched in the graph:

| Task (single-symbol query) | Precision | Recall |
|---------------------------|-----------|--------|
| SnapshotManager | 40.9% | 100% |
| SQLiteStore | 50.0% | 60% |
| Daemon | 34.8% | 50% |
| context (ContextEngine) | 30.4% | 83.3% |

These individual results show that edit-aware seeding CAN produce viable precision when the extracted symbol is specific and well-indexed. The challenge is consistency: for tasks where the primary symbol is generic or poorly matched ("registerTools" -> only 6 results, "GoExtractor" -> wrong neighborhood), the approach fails.

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
