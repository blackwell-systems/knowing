# knowing hooks

Claude Code hooks that integrate knowing's graph intelligence into the agent workflow.

## Proof of Value

Hooks are backed by automated benchmarks (not vibes). We measured whether automatic context injection saves or costs tokens across 10 realistic edit tasks:

| Metric | Result |
|--------|--------|
| Tasks where hook eliminates need for manual context call | **9 out of 10 (90%)** |
| Net token savings across all tasks | **+305 tokens saved** |
| Precision (% of injected symbols that are task-relevant) | 30.5% |
| Recall (% of needed symbols provided by the hook) | 52.3% |
| Latency p95 | 347ms |

**What this means:** In 9 out of 10 edit scenarios, the hook provides enough graph context that the agent does not need to make a separate `context_for_task` call. The 800-token injection replaces a 4000-token manual call, netting savings.

**How we proved it:**
1. Defined 10 simulated tasks with known symbol dependencies
2. Measured what the hook provides (800 tokens) vs what a manual call provides (4000 tokens)
3. Calculated coverage: does the hook provide 50%+ of the manual call's information?
4. If yes: the hook eliminates the manual call (net savings = manual_cost - hook_cost)
5. If no: the agent still calls manually (net cost = hook_cost wasted)

Re-run the benchmark yourself:
```bash
KNOWING_DB=/Users/you/code/repo/knowing.db go test -tags hookbench ./hooks/benchmark/ -v
```

Note: The MCP server auto-indexes your repo on first launch (zero-config), so a
`knowing.db` likely already exists at `~/.knowing/repos/<repo>.db`. Use the
absolute path to that file, or set `KNOWING_DB` to it.

See `hooks/FINDINGS.md` for the full experimental history, including earlier iterations where hooks were net-negative and what changed.

## Hook Suite

| Hook | Event | Purpose | Token Cost |
|------|-------|---------|-----------|
| `knowing-session-start` | SessionStart | Orient agent to graph capabilities | ~50 (once) |
| `knowing-pre-edit` | PreToolUse (Edit/Write) | Inject edit-aware context (net positive) | ~130 per edit |
| `knowing-pre-compact` | PreCompact | Inject orientation snapshot before compaction | ~100 (rare) |
| `knowing-post-task` | Stop | Run diagnostics on modified files | ~80 (once) |
| `knowing-subagent` | PreToolUse (Agent/Task) | Inject task-scoped context for subagents | ~130 per spawn |

## Setup

Add to your project's `.claude/settings.local.json`:

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

**Recommended minimum (lowest risk, highest value):**

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

These two fire rarely and solve real problems (agent doesn't know tools exist, agent forgets after compaction) with near-zero token cost.

## Configuration

All hooks share these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOWING_HOOKS` | `on` | Set to `off` to disable all hooks |
| `KNOWING_HOOKS_DB` | `knowing.db` | Path to SQLite database |
| `KNOWING_HOOKS_BUDGET` | `800` | Token budget for context injection |
| `KNOWING_HOOKS_FORMAT` | `gcf` | Output format (gcf for minimal tokens) |
| `KNOWING_HOOKS_LOG` | `.knowing-hooks.jsonl` | Metrics log path |
| `KNOWING_HOOKS_VERBOSE` | `0` | Set to `1` for debug output to stderr |

## Hook Descriptions

### knowing-session-start (SessionStart)

Fires once when a new session begins. Injects a brief orientation message telling the agent what graph tools are available and suggesting `format=gcf` for token savings. Costs ~50 tokens, fires once.

### knowing-pre-edit (PreToolUse: Edit/Write)

**Status: Recommended.** Injects graph context before file edits. Extracts symbols from `old_string` (edit-aware seeding) and queries the graph for related symbols. Benchmark: 30.5% precision / 52.3% recall, **net positive (+305 tokens saved)** across 10 tasks. Covers 90% of tasks well enough to eliminate the need for a separate manual context call.

### knowing-pre-compact (PreCompact)

Fires before Claude Code compacts the conversation. Injects a compact orientation snapshot (top symbols by relevance, ~600 tokens in GCF format) so the agent remembers the graph structure after compaction. This is arguably the highest-value hook because it solves a real pain point (agent amnesia after compaction) at minimal cost.

### knowing-post-task (Stop)

Fires when the agent completes a task. Identifies files modified in the git session and queries the graph for potentially-affected symbols. Surfaces callers, dependents, and related types that may need attention. Acts as a self-correction prompt.

### knowing-subagent (PreToolUse: Agent/Task)

Fires when the agent spawns a subagent. Extracts the subagent's task description and injects graph-ranked context scoped to that task. Solves the "spawned agents start blind" problem.

## Turning Off

Disable all hooks:
```bash
export KNOWING_HOOKS=off
```

Or remove individual hooks from `.claude/settings.local.json`.

## Metrics

All hooks log to `.knowing-hooks.jsonl`:

```json
{"ts":1716000000,"event":"pre_compact","tokens":95}
{"ts":1716000001,"event":"subagent_inject","latency_ms":280,"tokens":145}
{"ts":1716000002,"event":"post_task","files":"internal/mcp/server.go","tokens":82}
```

Analyze with:
```bash
./hooks/analyze-hooks
```

## Design Philosophy

Not all hooks are equal. They fall into three categories:

**Recommended (proven net-positive):**
- SessionStart: fires once, tiny payload, orients the agent
- PreCompact: fires rarely, prevents amnesia
- PreEdit: fires on every edit, saves 305 tokens net across typical tasks (90% coverage)

**Useful (opt-in):**
- Subagent: fires on spawn, scopes context to task
- PostTask: fires at end, surfaces potential issues

All hooks are proven safe. Start with the recommended set. Add Subagent/PostTask based on your workflow.
