# Claude Code Hooks Integration

knowing ships five hooks that integrate its graph intelligence into the Claude Code agent lifecycle. Each hook runs as a shell script invoked by Claude Code at a specific event, queries the knowing graph, and returns a JSON message that Claude Code injects into the agent's context.

## How Hooks Work

Claude Code supports lifecycle hooks: shell commands triggered at defined moments (session start, before a tool runs, before compaction, on stop). Each hook receives JSON on stdin describing the event, and may output a JSON object with a `message` field. Claude Code prepends that message to the agent's context for the current operation.

knowing's hooks call `knowing context` (or `knowing query`) against the local SQLite graph database. The context engine runs Random Walk with Restart (RWR) seeded by task-relevant symbols, ranks results using HITS hub/authority scores, and packs the top symbols into a token budget. The hook formats this as a one-line JSON message and exits.

All hooks share the same structure:

1. Check the kill switch (`KNOWING_HOOKS=off` disables everything).
2. Verify the database file exists.
3. Locate the `knowing` binary (PATH or `./knowing`).
4. Query the graph for context relevant to the hook's event.
5. Output `{"message": "..."}` or exit silently (exit 0 with no output means "no injection").

If any step fails (no database, no binary, empty results), the hook exits cleanly with no output. Hooks never block the agent or produce errors.

## Hook Suite

| Hook | Claude Code Event | Matcher | Fires When | Token Cost |
|------|------------------|---------|------------|-----------|
| `knowing-session-start` | SessionStart | (none) | Session begins | ~50 (once) |
| `knowing-pre-edit` | PreToolUse | `Edit\|Write` | Before any file edit | ~130 per edit |
| `knowing-pre-compact` | PreCompact | (none) | Before context compaction | ~100 (rare) |
| `knowing-post-task` | Stop | (none) | Agent completes a task | ~80 (once) |
| `knowing-subagent` | PreToolUse | `Agent\|Task` | Before spawning a subagent | ~130 per spawn |

## Hook Details

### knowing-session-start (SessionStart)

**When it fires:** Once, at the start of a new Claude Code session.

**What it does:** Queries the graph for a node count, then injects a brief orientation message listing available MCP tools (`context_for_task`, `context_for_pr`, `context_for_files`, `blast_radius`, `cross_repo_callers`) and recommending `format=gcf` for token savings.

**What context it injects:** A static orientation block (~50 tokens) telling the agent the graph exists, how many symbols are indexed, and which tools to call.

**Why it matters:** Without this, the agent does not know knowing's MCP tools are available until it discovers them through other means. This is a near-zero-cost nudge that fires once.

**File:** `hooks/knowing-session-start`

### knowing-pre-edit (PreToolUse: Edit/Write)

**When it fires:** Before every `Edit` or `Write` tool invocation. Claude Code passes the tool input (including `file_path` and `old_string`) on stdin.

**What it does:**

1. Parses the tool input JSON to extract the file path and `old_string`.
2. Runs edit-aware symbol extraction on `old_string`: regex patterns identify Go function names, type declarations, variable/const assignments, method calls, and package-qualified calls.
3. Queries the context engine with extracted symbols (or falls back to the filename if no symbols are found).
4. Injects graph-ranked context: callers, dependents, related types for the symbols being edited.

**What context it injects:** A GCF/KWF-formatted block of graph-ranked symbols related to the file and specific code being modified. Default budget is 800 tokens, which compresses to ~130 tokens in the injected message.

**Edit-aware seeding:** The hook does not just query by filename. It extracts the primary symbols from the code being changed (`old_string`) and uses those as the query. This produces sharply differentiated RWR scores instead of broad package-level matches. The tiered seed matching in the context engine (exact > prefix > substring) means the top results are structurally relevant to the specific edit.

**Why it matters:** In 9 out of 10 benchmark tasks, this injection provides enough context that the agent skips a separate `context_for_task` call (which would cost ~4000 tokens). Net savings: +305 tokens across 10 tasks.

**File:** `hooks/knowing-pre-edit`

### knowing-pre-compact (PreCompact)

**When it fires:** Before Claude Code compacts the conversation to free context window space.

**What it does:** Queries the context engine for "most important symbols" at a 600-token budget in GCF format. Injects a compact orientation snapshot (top symbols by blast radius and authority score).

**What context it injects:** A GCF block of the highest-impact symbols in the codebase (~100 tokens), prefixed with `[knowing orientation snapshot - carry through compaction]`.

**Why it matters:** This is arguably the highest-value hook. After compaction, the agent loses accumulated context about the codebase. This snapshot survives compaction and gives the agent a structural map to resume from, preventing the "agent amnesia" problem where it re-explores files it already understood.

**File:** `hooks/knowing-pre-compact`

### knowing-post-task (Stop)

**When it fires:** When the agent completes a task (the Stop event).

**What it does:**

1. Runs `git diff --name-only HEAD` to find files modified in the current session.
2. Filters to source files (`.go`, `.ts`, `.py`, `.rs`, `.java`, `.cs`), limited to 10 files.
3. Queries the context engine with `context -files` for those modified files at a 400-token budget.
4. Injects a diagnostic listing symbols that may be affected by the changes.

**What context it injects:** A GCF block listing callers, dependents, and related types for the modified files (~80 tokens), prefixed with `[knowing post-task diagnostic]` and the list of modified files.

**Why it matters:** Acts as a self-correction prompt. If the agent modified `server.go` but forgot to update a caller in `handler.go`, this surfaces the relationship. The agent can then decide whether follow-up changes are needed.

**File:** `hooks/knowing-post-task`

### knowing-subagent (PreToolUse: Agent/Task)

**When it fires:** Before the agent spawns a subagent via the `Agent` or `Task` tool.

**What it does:**

1. Parses the tool input JSON to extract the subagent's task description (from `prompt`, `description`, or `task` fields, truncated to 200 characters).
2. Queries the context engine with that task description at the configured budget.
3. Injects graph-ranked context scoped to the subagent's specific task.

**What context it injects:** A GCF block of symbols relevant to the subagent's task (~130 tokens), prefixed with `[knowing context for subagent task]`.

**Why it matters:** Spawned subagents start with no codebase knowledge. This hook gives them a focused graph context for their specific task, so they begin informed instead of exploring from scratch.

**File:** `hooks/knowing-subagent`

## Configuration

### Enabling Hooks

Add hook entries to `.claude/settings.local.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {"command": "./hooks/knowing-session-start"}
    ],
    "PreToolUse": [
      {"matcher": "Edit|Write", "command": "./hooks/knowing-pre-edit"},
      {"matcher": "Agent|Task", "command": "./hooks/knowing-subagent"}
    ],
    "PreCompact": [
      {"command": "./hooks/knowing-pre-compact"}
    ],
    "Stop": [
      {"command": "./hooks/knowing-post-task"}
    ]
  }
}
```

### Recommended Minimum

If you want the lowest-risk starting point, enable only SessionStart and PreCompact:

```json
{
  "hooks": {
    "SessionStart": [
      {"command": "./hooks/knowing-session-start"}
    ],
    "PreCompact": [
      {"command": "./hooks/knowing-pre-compact"}
    ]
  }
}
```

These two fire rarely (once at session start, once before each compaction) and solve concrete problems (agent does not know tools exist; agent forgets after compaction) at near-zero token cost.

### Disabling Hooks

Disable all hooks at once with an environment variable:

```bash
export KNOWING_HOOKS=off
```

Or remove individual hook entries from `.claude/settings.local.json`.

### Environment Variables

All hooks share these configuration variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOWING_HOOKS` | `on` | Set to `off` to disable all hooks |
| `KNOWING_HOOKS_DB` | `knowing.db` | Path to the SQLite graph database |
| `KNOWING_HOOKS_BUDGET` | `800` | Token budget for context injection |
| `KNOWING_HOOKS_FORMAT` | `gcf` | Output format (`gcf` for minimal tokens, `kwf`, or `xml`) |
| `KNOWING_HOOKS_LOG` | `.knowing-hooks.jsonl` | Path for the metrics log |
| `KNOWING_HOOKS_VERBOSE` | `0` | Set to `1` to print injection details to stderr |

### Prerequisites

Hooks require:

1. A `knowing.db` SQLite database (run `knowing index -db knowing.db .` to create one).
2. The `knowing` binary on PATH or as `./knowing` in the project root.

If either is missing, hooks exit silently with no effect.

**Keeping the graph fresh:** For best results, run the MCP server with
`--watch` enabled (`knowing mcp --watch -db knowing.db`) or run
`knowing watch ./repo` in a separate terminal. This re-indexes changed files
automatically on save, so hooks always query up-to-date graph data.

## How Hooks Interact with the Context Engine

The hooks are thin shell wrappers around knowing's context engine (`internal/context`). The query flow is:

1. **Seed selection:** The hook provides a query string (extracted symbols or filename). The context engine matches this against graph nodes using three-tier matching: exact symbol name match (tier 1), prefix match on the last path component (tier 2), substring fallback with a cap of 5 candidates (tier 3). This tiered approach produces 5-30 high-quality seed nodes instead of 100 broad ones.

2. **Random Walk with Restart (RWR):** Starting from the seed nodes, the engine runs RWR across the graph edges (`calls`, `implements`, `references`, etc.) to score every reachable node by structural proximity to the seeds.

3. **HITS reranking:** Hub/authority scores separate structurally important symbols (heavily called authorities) from leaf functions that happen to be graph-adjacent.

4. **Token packing:** The engine selects the top-ranked symbols that fit within the token budget using a knapsack-style packer. GCF encoding compresses the output (84% fewer tokens than JSON).

5. **Hook output:** The shell script wraps the engine's output in a JSON `{"message": "..."}` envelope that Claude Code consumes.

The edit-aware seeding in `knowing-pre-edit` is particularly important: by extracting symbols from the `old_string` of an Edit operation, the hook queries the graph for exactly what is being changed rather than the broader file neighborhood. This is what made hooks net-positive after the initial experiments showed them to be net-negative with filename-only queries.

## Performance and Benchmarks

### Measured Results (10 realistic edit tasks)

| Metric | Value |
|--------|-------|
| Tasks where hook eliminates need for manual context call | 9/10 (90%) |
| Net token savings across all tasks | +305 tokens saved |
| Precision (injected symbols that are task-relevant) | 30.5% |
| Recall (needed symbols provided by the hook) | 52.3% |
| Latency p95 | 347ms |

### What the Numbers Mean

- **Net-positive:** The hook spends ~130 tokens per edit but eliminates a ~4000-token manual `context_for_task` call in 90% of cases. Across 10 tasks, this nets +305 tokens saved.
- **Precision of 30.5%** means roughly a third of injected symbols are directly task-relevant. The rest are structurally related (same package, callers) but not critical. For automatic injection this is acceptable because the cost is low.
- **Recall of 52.3%** means the hook provides just over half the symbols the task actually needs. This is enough for the agent to skip the manual call in most cases.
- **Latency under 350ms** at p95 means the hook adds imperceptible delay to edit operations.

### Running Benchmarks

```bash
# Index the repo first
knowing index -db knowing.db .

# Precision/recall benchmark
KNOWING_DB=knowing.db go test -tags hookbench ./hooks/benchmark/ -v -run TestHookBenchmark

# Cost comparison benchmark
KNOWING_DB=knowing.db go test -tags hookbench ./hooks/benchmark/ -v -run TestHookCostComparison

# Try a different budget
KNOWING_DB=knowing.db KNOWING_HOOKS_BUDGET=1200 go test -tags hookbench ./hooks/benchmark/ -v
```

### Experimental History

The hooks went through two phases. The initial implementation used filename-only queries and produced net-negative results (-910 tokens across 10 tasks, only 20% coverage). The fix was two-fold:

1. **Edit-aware seeding:** extracting symbols from `old_string` instead of querying by filename.
2. **Tiered seed matching:** the context engine's seed selection was changed from flat substring matching to a three-tier system (exact > prefix > substring with a cap). This produced sharply differentiated RWR scores instead of flat distributions.

After these changes, coverage jumped from 20% to 90% and the token impact flipped from -910 (cost) to +305 (savings). The full experimental history is in `hooks/FINDINGS.md`.

## Metrics and Analysis

All hooks log events to `.knowing-hooks.jsonl`:

```json
{"ts":1716000000,"file":"internal/mcp/server.go","event":"inject","latency_ms":280,"tokens":145,"format":"gcf","budget":800}
{"ts":1716000001,"file":"internal/mcp/server.go","event":"miss","latency_ms":12,"tokens":0}
{"ts":1716000002,"event":"pre_compact","tokens":95}
{"ts":1716000003,"event":"subagent_inject","latency_ms":280,"tokens":145}
{"ts":1716000004,"event":"post_task","files":"internal/mcp/server.go","tokens":82}
```

Events include:
- `inject`: context was found and injected (with latency and token count).
- `miss`: query returned no results (file not in graph or no matching symbols).
- `pre_compact`: orientation snapshot injected before compaction.
- `subagent_inject` / `subagent_miss`: context for a spawned subagent.
- `post_task`: diagnostic context for modified files.

Run the analysis tool to get a summary report:

```bash
./hooks/analyze-hooks
```

This produces hit rate, token overhead (total/mean/p50/p95), latency distribution, per-file breakdown, and A/B evaluation guidance.

## File Reference

| File | Purpose |
|------|---------|
| `hooks/knowing-session-start` | SessionStart hook |
| `hooks/knowing-pre-edit` | PreToolUse hook for Edit/Write |
| `hooks/knowing-pre-compact` | PreCompact hook |
| `hooks/knowing-post-task` | Stop hook |
| `hooks/knowing-subagent` | PreToolUse hook for Agent/Task |
| `hooks/analyze-hooks` | Metrics analysis script |
| `hooks/benchmark/bench_test.go` | Precision/recall benchmark |
| `hooks/benchmark/cost_test.go` | Cost comparison benchmark |
| `hooks/README.md` | Hook suite overview |
| `hooks/FINDINGS.md` | Full experimental history |
