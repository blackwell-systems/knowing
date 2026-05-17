# Token Savings Benchmark: Knowing vs Manual Exploration

**Date:** 2026-05-17
**Methodology:** 5 realistic task scenarios comparing manual grep/read workflows against knowing's context_for_task.

---

## Thesis Under Test

An AI agent using knowing's `context_for_task` consumes fewer tokens and makes fewer tool calls than an agent exploring the codebase manually with grep and file reads. The hypothesis: knowing reduces exploration overhead by 40%+.

---

## Experimental Setup

### Scenarios

Each scenario represents a real task an agent might receive. The "without knowing" path simulates standard agent behavior (grep for keywords, read matching files, grep for callers). The "with knowing" path calls `context_for_task` once, then reads only the files containing top-ranked symbols.

| Scenario | Task description |
|----------|-----------------|
| indexer_error_handling | "Fix error handling in the indexer extract pipeline" |
| context_ranking_bug | "Debug why context engine returns flat scores" |
| new_mcp_tool | "Add a new MCP tool for dead code detection" |
| sqlite_optimization | "Optimize SQLite store query performance" |
| snapshot_comparison | "Compare two snapshots and generate diff report" |

### Measurement

**Without knowing:**
- Run `grep -rn --include=*.go <keywords>` against the repo for each keyword in the task
- Count output lines, estimate tokens at 4 tokens/line (conservative)
- Add estimated file read costs (200 tokens per unique file touched)
- Tool calls = number of grep operations + file reads

**With knowing:**
- Call `engine.ForTask()` with 3000 token budget
- `TokensUsed` from the returned ContextBlock is the token cost
- Count unique packages in top-10 symbols for targeted file reads
- Tool calls = 1 (ForTask) + number of unique packages

---

## Results

| Scenario | Calls (w/o) | Calls (w/) | Tokens (w/o) | Tokens (w/) | Call Reduction | Token Reduction |
|----------|-------------|------------|--------------|-------------|----------------|-----------------|
| indexer_error_handling | 8 | 4 | 7,580 | 2,681 | 50.0% | 64.6% |
| context_ranking_bug | 8 | 4 | 4,688 | 2,743 | 50.0% | 41.5% |
| new_mcp_tool | 7 | 3 | 6,128 | 2,753 | 57.1% | 55.1% |
| sqlite_optimization | 6 | 4 | 3,352 | 2,140 | 33.3% | 36.2% |
| snapshot_comparison | 7 | 2 | 5,448 | 1,745 | 71.4% | 68.0% |
| **Aggregate** | **7.2 avg** | **3.4 avg** | **5,439 avg** | **2,412 avg** | **52.8%** | **55.6%** |

---

## Interpretation

### Token savings: 55.6% reduction

Knowing replaces N exploratory tool calls (grep + read loops) with a single graph-informed context retrieval. The median scenario saves 3,000+ tokens, which at current API pricing translates directly to cost reduction.

The savings are larger for broad tasks ("fix error handling in the indexer") where grep returns many false positives, and smaller for focused tasks ("optimize SQLite query") where grep is already fairly precise.

### Tool call reduction: 52.8%

Each avoided tool call saves 1-3 seconds of API round-trip latency. Reducing from 7.2 to 3.4 calls means the agent reaches useful context ~4 seconds faster per task. Over a multi-step workflow with 5 context queries, that's 20 seconds of latency eliminated.

### Why the "snapshot_comparison" scenario shows the best results (71.4% / 68.0%)

This task has very specific keywords ("snapshot", "diff") that match many symbols in grep but only a few are relevant. Knowing's graph-based ranking surfaces the 2 critical symbols (`SnapshotDiff`, `SemanticDiff`) in one call, while grep returns dozens of matches across unrelated files.

### Why "sqlite_optimization" shows the worst results (33.3% / 36.2%)

"SQLite" is a very precise keyword that matches almost exclusively relevant files. Grep is nearly as effective as knowing for highly-specific technical terms. The value of knowing increases with ambiguity.

### What this means for agent workflows

1. **Every task benefits.** Even the worst case (sqlite_optimization) saves 33% of tool calls and 36% of tokens.
2. **Ambiguous tasks benefit most.** When keywords are broad (like "error handling"), knowing's graph ranking provides the sharpest improvement.
3. **The savings compound.** An agent making 5 context queries per task saves ~15,000 tokens total, which is meaningful context window capacity for source code and tool output.

---

## Reproducibility

```bash
GOWORK=off go test ./bench/token-savings/ -v -count=1
```

Runs actual grep commands against the knowing repo and compares against ForTask results. The FINDINGS.md is auto-generated with measured values.
