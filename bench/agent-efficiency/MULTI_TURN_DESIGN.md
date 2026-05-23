# Multi-Turn Agent Efficiency Benchmark Design

## Problem Statement

Single-turn benchmarks (Run 1, Run 2) test "answer a question" and find knowing
adds overhead. This is because grep answers single questions faster than a graph
round-trip. knowing's value is in multi-turn work sessions where:

- The agent explores, plans, edits, and verifies across multiple files
- Repeated exploration is eliminated by upfront context
- Connections between components are surfaced without grep iteration
- The pre-edit hook injects context automatically before every edit

## Benchmark Architecture

### Session structure

Each task is a **multi-turn coding task** run with `claude -c` (conversational mode).
The agent receives an initial prompt, works through the task with full tool access,
and produces a final result (code change, answer, or PR). The session continues
until the agent reports completion.

### Measurement approach

Instead of `--print` (single turn), use `--continue` or interactive mode and
capture the full session transcript. Compare:

| Metric | How measured |
|--------|-------------|
| Total tokens (input+output) | Sum from transcript `usage` fields |
| Tool calls | Count all `tool_use` blocks |
| Exploration calls | Count Grep + Read + Glob (non-targeted) |
| Context calls | Count `mcp__knowing__*` calls |
| Time to first edit | Seconds from session start to first Edit tool call |
| Time to completion | Total session wall clock |
| Files edited | Count unique files in Edit calls |
| Correctness | Does the final code compile? Do tests pass? Does it match ground truth? |
| Regressions | `go vet` + `go test` before vs after |

### Key insight: "time to first edit"

In real sessions, agents spend 30-60% of their time understanding the codebase
before making changes. knowing's value is collapsing that exploration phase.
"Time to first edit" directly measures this: how quickly does the agent feel
confident enough to start writing code?

## Task Fixtures (6 tasks, escalating complexity)

### Task 1: Add a CLI flag (Low complexity, 1 file)

```
Add a --json flag to the `knowing stale` command that outputs results as JSON
instead of human-readable text. The flag should be a bool, default false.
When set, output a JSON object with fields: stale_files ([]string),
stale_node_count (int), checked_at (ISO timestamp).
```

Ground truth: edit `cmd/knowing/stale.go`, add flag parsing + JSON output path.
Verification: `go build ./cmd/knowing && knowing stale --json` produces valid JSON.

### Task 2: Fix a bug (Medium complexity, 2-3 files)

```
The `knowing context` CLI command ignores the -format flag when the format is "gcf".
It always outputs XML regardless. Find and fix the bug. The issue is in how the
CLI passes the format to the context engine.
```

Ground truth: the bug is that `cmdContext` passes `Format: "xml"` hardcoded instead
of using the parsed flag value. Fix in `cmd/knowing/main.go` (cmdContext function).
Verification: `knowing context -task "test" -format gcf` produces GCF output.

NOTE: This task requires seeding the bug first (introduce it in a worktree before
running the benchmark). The benchmark runner creates the bug, then the agent finds
and fixes it.

### Task 3: Add a new MCP tool (Medium-high complexity, 3 files)

```
Add a `symbol_info` MCP tool that takes a qualified_name parameter and returns:
the node's kind, file path, line number, all incoming edges (callers), and all
outgoing edges (callees). Register it in the server. Format the output as JSON.
```

Ground truth: new function in `internal/mcp/handlers.go`, tool definition, registration
in `registerTools()`. Uses `store.NodesByQualifiedName`, `store.EdgesTo`, `store.EdgesFrom`.
Verification: `go build ./...` + tool appears in `knowing mcp` tool list.

### Task 4: Refactor with blast radius awareness (High complexity, 5+ files)

```
The function `inferExternalRepoURL` in internal/resolve/external.go currently uses
string return values ("external://...", "stdlib", ""). Refactor it to return a
typed result: type ExternalURLResult struct { URL string; Kind string } where Kind
is "external", "stdlib", or "local". Update all callers in the 5 extractors that
use this function.
```

Ground truth: modify `internal/resolve/external.go` (return type change) + update
5 extractor call sites + update `internal/resolve/external_test.go`.
Verification: `go build ./...` + `go test ./internal/resolve/ ./internal/indexer/...`

### Task 5: Cross-package investigation (High complexity, understanding-first)

```
The context engine's RWR walk sometimes produces identical scores for unrelated
symbols when the graph has multiple disconnected components. Investigate why this
happens, identify the root cause in the code, and propose a fix (implement if
straightforward, otherwise write a detailed comment explaining the fix).
```

Ground truth: the issue is in `RandomWalkWithRestart` (walk.go) where the restart
probability distributes evenly across ALL seeds regardless of community membership.
Fix: weight restart probability by seed proximity (already partially implemented
via community-aware RWR). The agent must find walk.go, understand the restart logic,
trace how seeds are selected, and connect it to the community filtering.
Verification: agent identifies the correct root cause in their response.

### Task 6: Multi-file feature with tests (High complexity, full workflow)

```
Add a `knowing diff --since <duration>` flag that shows which packages have changed
in the last N hours/days based on git log. It should:
1. Parse the duration (e.g., "24h", "7d")
2. Get commits since that time via git log
3. Determine changed files from those commits
4. Map files to packages using the node table
5. Output: list of packages with change counts, sorted by most changes
```

Ground truth: edit `cmd/knowing/main.go` (cmdDiff), add duration parsing, git log
integration, file-to-package mapping via store queries.
Verification: `go build ./cmd/knowing && knowing diff --since 24h` produces output.

## Runner Design

```bash
#!/usr/bin/env bash
# multi-turn-runner.sh <task-id> <mode>
#
# Runs a multi-turn task in a fresh git worktree (isolated from main).
# Control: no knowing MCP tools
# Treatment: knowing MCP tools available
#
# Session ends when the agent outputs "TASK COMPLETE" or after 5 minutes timeout.

TASK_ID="$1"
MODE="$2"
MODEL="${BENCH_MODEL:-sonnet}"
TIMEOUT=300  # 5 minutes max per session

# Create isolated worktree for the task
WORKTREE=$(mktemp -d)
git worktree add "$WORKTREE" -b "bench-${TASK_ID}-${MODE}" HEAD

# Run claude in the worktree with conversation mode
cd "$WORKTREE"
timeout $TIMEOUT claude \
  --model "$MODEL" \
  --output-format stream-json \
  -p "$PROMPT" \
  > "$TRANSCRIPT_FILE"

# Verify result
GOWORK=off go build ./... 2>"$RESULT_DIR/build.log"
BUILD_OK=$?
GOWORK=off go test -short ./... 2>"$RESULT_DIR/test.log"
TEST_OK=$?

# Cleanup worktree
git worktree remove "$WORKTREE" --force
```

## Analysis Additions

The existing `transcript.go` parser works unchanged. Additional metrics to extract:

```go
type MultiTurnMetrics struct {
    SessionMetrics  // embed existing

    // New fields for multi-turn
    TimeToFirstEdit   int64    // ms from session start to first Edit call
    ExplorationCalls  int      // Grep + Read + Glob before first Edit
    ContextCalls      int      // mcp__knowing__* calls
    FilesEdited       []string // unique files in Edit calls
    BuildSuccess      bool     // did the result compile?
    TestSuccess       bool     // did tests pass?
    SubagentsSpawned  int      // Agent tool calls (exploration delegation)
}
```

## Expected Results

| Metric | Control (no knowing) | Treatment (knowing) | Expected delta |
|--------|---------------------|--------------------|----|
| Time to first edit | 60-120s (explore first) | 15-30s (context_for_task then edit) | -50-75% |
| Exploration calls | 8-15 (grep loops) | 1-3 (one context call + targeted reads) | -70-80% |
| Total tool calls | 15-30 | 8-15 | -40-50% |
| Total tokens | Varies | Lower (less exploration noise) | -20-40% |
| Correctness | Varies | Same or better (graph surfaces dependencies) | >= 0 |
| Build success | 80-90% | 90-95% (fewer missed callers) | +5-10% |

The key differentiator is **time to first edit** and **exploration call count**.
These directly measure "did the agent understand the codebase faster with knowing?"

## Why This Will Work When Single-Turn Didn't

1. **Multi-file tasks require understanding structure.** You can't grep your way to
   "update all 5 callers of this function" without knowing WHERE they are. context_for_task
   provides that map in one call.

2. **The exploration phase is where knowing saves.** Single-turn tasks skip this phase
   (one question, one answer). Multi-turn tasks have a mandatory exploration phase
   that knowing collapses.

3. **Blast radius is the killer feature.** Task 4 (refactor with callers) directly
   tests whether knowing identifies all call sites vs the agent grepping and potentially
   missing one.

4. **Correctness matters more.** In multi-file edits, missing a caller = broken build.
   knowing's graph traversal finds callers that grep might miss (indirect, through
   interfaces, cross-package). Build success rate is the ultimate metric.

## Prerequisites

- Tasks 2-6 require the knowing repo to be in a buildable state
- Task 2 requires seeding a bug (automated by the runner)
- Tasks 4 and 6 modify real code (run in isolated worktrees, not main branch)
- `claude -c` or `claude --print` with multi-turn support needed
- Timeout of 5 minutes prevents runaway sessions

## Implementation Order

1. Implement the worktree-based runner (`multi-turn-runner.sh`)
2. Implement Task 1 (simplest, validates the harness works)
3. Run Task 1 control + treatment, verify analysis works
4. Implement Tasks 2-6
5. Full run: 6 tasks x 2 modes = 12 sessions
6. Generate FINDINGS-multi-turn.md
