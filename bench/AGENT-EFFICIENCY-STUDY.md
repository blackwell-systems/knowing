# Agent Efficiency Study

Controlled experiments measuring whether knowing makes AI agents more efficient
at coding tasks. This document records the full experimental narrative: hypotheses,
methodology, results, interpretation, and what each result teaches us about the
product.

**Related benchmarks:**
- [Cross-system retrieval](cross-system/FINDINGS.md) (P@10=0.230, 11.5x vs grep)
- [Context packing study](CONTEXT-PACKING-STUDY.md) (umbrella evaluation program)
- [Hook benchmark](../hooks/FINDINGS.md) (33% precision, 100% coverage)
- [Feedback loop](feedback-loop/FINDINGS.md) (+20pp compounding)
- [Token savings](token-savings/FINDINGS.md) (44% fewer calls, 80% fewer tokens)

---

## Hypothesis

> An AI agent with access to knowing's graph-ranked context tools will complete
> coding tasks with fewer tool calls, fewer tokens, and higher correctness than
> the same agent with only standard tools (grep, read, glob).

## Experimental Design

### Variables

- **Independent variable:** Tool availability (control: grep/read/glob only; experiment: + knowing MCP tools)
- **Dependent variables:** Tool call count, output tokens, exploration calls, wall clock time, build success, answer correctness
- **Controlled:** Same model (Sonnet), same repo (knowing, 160K LOC Go), same prompts, isolated worktrees, same timeout

### Infrastructure

- `runner.sh`: Single-turn `--print` mode benchmark (8 tasks)
- `multi-turn-runner.sh`: Multi-file coding tasks in isolated worktrees (9 tasks)
- `bench_test.go`: Transcript parser + comparison engine + FINDINGS generator
- Transcripts stored in `transcripts/` and `transcripts/multi-turn/`

---

## Experiment 1: Single-Turn Questions (Run 1)

**Date:** 2026-05-23
**Model:** Sonnet (Bedrock)
**Tasks:** 8 question-answer tasks targeting knowing codebase

### Setup

Basic prompts. Treatment told "you have knowing tools, use them." No specific
tool routing instructions. Agent discovers tools via ToolSearch.

### Results

| Metric | Average |
|--------|---------|
| Token savings | -288% (treatment used MORE) |
| Tool call savings | -36% |
| Time savings | -92% |
| Correctness delta | 0.000 |

### Interpretation

Treatment was worse on every metric except correctness (tied). Root causes:
1. ToolSearch overhead (1 extra call per session to discover tools)
2. Over-exploration on one task (file-save-to-cache-invalidation: 37 vs 13 calls)
3. Trivial tasks (blast-radius-handler, edge-types) answer in 1-2 greps

### What we learned

Single-turn questions are grep's home turf. "Find function X" is answered by
one grep. Adding a graph query on top adds latency without information gain.

---

## Experiment 2: Single-Turn Questions (Run 2, improved prompts)

**Date:** 2026-05-23
**Change:** Explicit tool names in prompt (no ToolSearch needed), "stop early" instruction

### Results

| Metric | Average |
|--------|---------|
| Token savings | -184% (still worse) |
| Tool call savings | -90% |
| Time savings | -436% |
| Correctness delta | -0.031 |

### Interpretation

Worse than Run 1. The explicit tool names caused the agent to USE them on every
task (including trivial ones where grep was faster). The ToolSearch call persisted
despite explicit names (Sonnet still fetches schemas). Time was dominated by MCP
round-trip latency (5-20s per knowing call vs milliseconds for grep).

### What we learned

1. Telling an agent "use these tools" makes it use them even when they're not helpful.
2. MCP latency is the killer: each graph query costs 5-20s of wall clock.
3. The comparison is unfair on time: grep is local (ms), knowing is a subprocess (s).

---

## Experiment 3: Multi-Turn Coding Tasks (Discovery Required)

**Date:** 2026-05-23
**Tasks:** Multi-file coding tasks requiring understanding before editing
**Change:** Tasks run in isolated git worktrees with build verification

### Task: add-json-flag (1-file, sanity check)

| Mode | Tool calls | Output tokens | Build |
|------|-----------|---------------|-------|
| Control | 5 | 138 | PASS |
| Experiment | 7 | 180 | PASS |
| Delta | +2 | +42 | — |

Simple task. Both modes trivially successful. Experiment overhead: context_for_task
call + extra Read. Expected: no value on 1-file tasks.

### Task: refactor-return-type v1 (callers listed in prompt)

| Mode | Tool calls | Output tokens | Build |
|------|-----------|---------------|-------|
| Control | 32 | 962 | PASS |
| Experiment | 40 | 10572 | PASS |
| Delta | +8 | +9610 | — |

**Problem identified:** The task prompt listed all 5 caller files. Neither mode
needed to discover them. Both just Read + Edit each file. knowing added overhead
without adding information.

**Lesson:** Tasks must require DISCOVERY, not just EXECUTION.

### Task: refactor-return-type v2 (must find callers yourself)

| Mode | Tool calls | Output tokens | Build |
|------|-----------|---------------|-------|
| Control | 32 | 1415 | PASS |
| Experiment | 36 | 1409 | PASS |
| Delta | +4 | -6 | — |

Even without callers listed, control did 1 grep for `InferExternalRepoURL` and
found all callers immediately. Function name is unique in the codebase.

**Lesson:** At 160K LOC, unique function names are directly greppable. The graph
can't add value when grep already returns precise results.

### Task: ambient-context (map RankSymbols neighborhood)

| Mode | Tool calls | Output tokens | Build |
|------|-----------|---------------|-------|
| Control | 4 | 102 | PASS |
| Experiment | 11 | 309 | PASS |
| Delta | +7 | +207 | — |

Both produced excellent, nearly identical answers (callers, callees, tests, types).
Control: 2 greps + 2 reads. Experiment: context_for_task + graph_query + test_scope
+ additional reads.

**Answer quality was equal.** Experiment was more structured but contained the same
information. The graph didn't surface anything grep missed.

### What we learned

**Sonnet + grep is already near-optimal for a 160K LOC Go codebase.**

At this scale:
- Function names are unique (grep returns exact matches)
- The codebase fits in context if needed
- 2 greps find callers, callees, tests for any symbol
- The agent is smart enough to grep efficiently

---

## Why The Benchmark Fails To Show Value (Analysis)

### The benchmark measures the wrong scenario

knowing's value is NOT "answer questions about a 160K LOC repo faster than grep."
It's:

1. **Scale.** At 3.5M LOC (kubernetes), grep for "Handler" returns 500+ results.
   knowing ranks them by graph centrality and returns the 10 that matter. The
   cross-system benchmark proves this: P@10=0.230 vs grep 0.020 (11.5x).

2. **Ambiguity.** At 160K LOC, function names are unique. At 1M+ LOC, names
   collide. "Server" appears in 40 packages. "Handler" in 200 files. The graph
   disambiguates by connectivity; grep cannot.

3. **Invisibility.** The pre-edit hook injects context before every edit WITHOUT
   a tool call. The agent never asks; it just has the information. This saves
   the grep-read-understand cycle entirely. Proven by hook benchmark (100%
   coverage, fires every edit, 250ms, 20 ranked symbols).

4. **Compounding.** First session = cold start. Fifth session = task memory
   boosts symbols the developer repeatedly works with. Grep never improves.
   Proven by feedback-loop benchmark (+20pp after one round).

5. **Prediction.** "Which tests should I run?" requires call-graph traversal.
   Grep for "Test" returns every test function. knowing traces the call graph
   backward from changed symbols and predicts affected tests at 98.9% precision.

### Why these don't show up in the benchmark

| Value prop | Why benchmark misses it |
|-----------|------------------------|
| Scale | Benchmark runs on knowing (160K LOC), not kubernetes (3.5M LOC) |
| Ambiguity | knowing's symbols are well-named and unique |
| Invisibility (hooks) | --print mode doesn't run hooks; sessions too short |
| Compounding | Each task runs once with zero history |
| Prediction | None of the tasks ask "which tests?" |

---

## Proven Value (From Other Benchmarks)

| Claim | Evidence | Source |
|-------|----------|--------|
| 11.5x more precise than grep at scale | P@10=0.230 vs 0.020, p<0.0001, d=0.92 | cross-system/ |
| 2.75x more precise than GitNexus | P@10=0.230 vs 0.076, p=0.0003, d=0.50 | cross-system/ |
| 46x faster indexing than Gortex on k8s | 18.6s vs 14.2 min | cross-system/ |
| 98.9% test scope prediction | Call-graph BFS vs independent Go import DAG | test-scope-accuracy/ |
| +20pp precision from feedback | Single round of ground-truth feedback | feedback-loop/ |
| 84% token savings | GCF wire format vs JSON | wire-format/ |
| 44% fewer tool calls (simulated) | context_for_task replaces grep loops | token-savings/ |
| 100% hook coverage | Hook fully replaces manual context call | hooks/FINDINGS.md |
| 517x faster diff | Hierarchical Merkle vs flat edge scan | merkle-diff/ |

---

## Next Experiments (Priority Order)

### 1. Agent benchmark on kubernetes (3.5M LOC)

Run the same tasks against kubernetes where grep returns noise. Expected: knowing
surfaces relevant symbols while grep drowns in false positives. This directly
translates the 11.5x P@10 advantage into agent-measurable tool call savings.

**Blocked by:** Need kubernetes indexed and task fixtures written for that repo.
Already have the corpus (`bench/cross-system/corpus/repos/kubernetes/`).

### 2. Agent benchmark with hooks active

Run multi-turn tasks in interactive mode (not --print) where the pre-edit hook
fires before every edit. Measure: did the agent use fewer explicit context calls
because the hook already provided the information?

**Blocked by:** Can't run hooks in --print mode. Needs interactive session or
a way to simulate hook injection in the transcript.

### 3. Multi-session compounding test

Run the same task 5 times. Measure whether later runs are faster/cheaper as
task memory accumulates. Expected: query 1 is cold-start, query 5 benefits from
memory boost (+20pp from feedback-loop bench).

**Blocked by:** Requires session persistence between runs (task_memory in SQLite).
Possible with shared DB across runs.

### 4. Aider head-to-head

Run Aider on the same multi-turn tasks. Compare: total tokens, time, correctness.
Aider uses tree-sitter repo-map for context; knowing uses graph-ranked context.
Direct comparison of context strategies.

**Unblocked:** `uv python install 3.11 && uv pip install aider-chat` (no Fortran).

### 5. Interface/indirect caller discovery

Design tasks where the dependency is through an interface (not a direct call).
Grep for the function name won't find it. The graph traces through the interface
implementation chain.

Example: "What happens if I change the `Close()` method on GraphStore? Find
everything that calls it, including through the interface."

---

## Experiment 4: Django (473K LOC Python, Ambiguous Names)

**Date:** 2026-05-23
**Model:** Sonnet (Bedrock)
**Repo:** Django (473K LOC, 2788 Python files, 164K indexed nodes)
**Hypothesis:** At 5x the scale with ambiguous names (View, Manager, Field everywhere),
knowing should outperform grep because grep returns noise while the graph ranks by
relevance.

### Task: django-queryset-callers ("find all callers of QuerySet.filter()")

| Mode | Tool calls | Output tokens | Turns | Lines |
|------|-----------|---------------|-------|-------|
| Control | 3 | 67 | 9 | 19 |
| Treatment | 106 | 2633 | 138 | 254 |
| Delta | +103 | +2566 | +129 | +235 |

**Control approach:** 2 greps + 1 bash. Grepped for `.filter(`, parsed results, done.
Produced a comprehensive table of every call site with file:line and context.

**Treatment approach:** Called `graph_query` twice, then read 96 files individually to
verify each caller. Produced an equally comprehensive table.

**Answer quality:** Both answers were nearly identical. Same callers found, same file
paths, same line numbers. The treatment answer was slightly more structured.

### What This Reveals

**"Find all callers of X" is STILL a grep task, even at 473K LOC.**

`grep -rn "\.filter("` returns every call site in the codebase. The function name
plus the dot-method syntax is specific enough that grep doesn't produce noise. The
graph added 103 tool calls of overhead to reach the same answer.

### When Grep Fails (And knowing Would Win)

The tasks that grep CANNOT solve efficiently:

1. **Transitive impact:** "If I change QuerySet.filter's signature, which tests
   break?" Grep finds direct callers but not callers-of-callers, and doesn't
   know which test functions exercise those call paths. This is test_scope (98.9%
   precision, already proven).

2. **Inheritance chains:** "What classes inherit from CharField across 3+ levels?"
   Grep for "CharField" finds declarations, not the inheritance graph. knowing
   has `extends` edges that trace the full chain.

3. **Framework path tracing:** "What middleware processes a request to /admin/?"
   This is a runtime path through Django's framework wiring. Grep for "middleware"
   returns 500+ results. knowing traces the `calls` and `handles_route` edges
   from URL patterns through middleware classes to view functions.

4. **Semantic disambiguation:** "Which 'Manager' is the one that handles user
   authentication?" Django has ~40 classes with "Manager" in the name. Grep
   returns them all. knowing ranks by graph proximity to auth-related symbols.

### The Pattern Across All Experiments

| Experiment | Repo | LOC | Result | Why |
|-----------|------|-----|--------|-----|
| 1-2 (single-turn) | knowing | 93K | grep wins | Unique names, instant results |
| 3 (multi-turn) | knowing | 93K | grep ties | Same: unique names |
| 4 (Django) | django | 473K | grep ties | `.filter(` is specific enough for grep |

**The common thread:** All tasks reduce to "find symbol X by name." As long as the
name (or method call pattern) is greppable, grep wins on speed. The graph adds value
only when the question CANNOT be answered by text matching:

- Transitive dependencies (multi-hop)
- Inheritance hierarchies
- "What tests cover this?"
- "What's related to this area?" (vague, structural)
- Disambiguation among many similar names

### Revised Understanding of knowing's Value

knowing is NOT a replacement for grep. It's a complement that handles the cases grep
cannot:

| Need | Right tool | Why |
|------|-----------|-----|
| "Find function X" | grep | Text match, instant |
| "Find all callers of X" | grep | `.X(` is a pattern |
| "What breaks if I change X?" | **knowing** | Transitive callers + test paths |
| "Which tests should I run?" | **knowing** | Call-graph traversal (98.9%) |
| "Give me context for this task" | **knowing** | Ranked structural neighborhood |
| "What inherits from X?" | **knowing** | Edge traversal across levels |
| "Is this route used?" | **knowing** | Static + runtime edge comparison |

The cross-system benchmark's 11.5x advantage comes from the "ranked structural
neighborhood" case (context_for_task), not from "find callers." The agent efficiency
benchmark has been testing the wrong query type.

---

## Meta-Observations

### On honest benchmarking

This study documents failures. The agent efficiency hypothesis was not confirmed
for 160K LOC Go repos. That's valuable information: it tells us WHERE knowing
provides value (scale, ambiguity, prediction) and where it doesn't (small repos
with unique names).

Publishing negative results builds credibility. No competitor does this.

### On product positioning

knowing is not "grep but slower." It's:
- A retrieval engine for large/ambiguous codebases (proven: 11.5x at scale)
- An automatic context layer (proven: hooks, 100% coverage)
- A learning system (proven: +20pp compounding)
- An integrity primitive (proven: Merkle proofs, 72us generate, 1.2us verify)
- A test predictor (proven: 98.9% precision)

The value prop for a 160K LOC Go repo is the hooks (invisible, automatic) and
compounding (gets better over time). The value prop for kubernetes-scale repos
is retrieval quality (11.5x). Both are proven; just by different benchmarks.

### On the right benchmark for the right claim

| Claim | Right benchmark | Wrong benchmark |
|-------|----------------|-----------------|
| "Better retrieval at scale" | cross-system (7 repos, statistical testing) | agent-efficiency on knowing repo |
| "Fewer agent tool calls" | agent-efficiency on kubernetes | agent-efficiency on knowing (grep is optimal) |
| "Automatic context" | hook benchmark (precision/recall/coverage) | agent --print mode (hooks don't fire) |
| "Compounds over time" | feedback-loop (multi-round) | single-run anything |
| "Predicts affected tests" | test-scope-accuracy (98.9%) | any benchmark without test questions |
