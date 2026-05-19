# Agent Efficiency Benchmark

This benchmark proves that Claude Code with knowing MCP tools completes tasks
with fewer tool calls, fewer tokens, and higher correctness than without.

## How it works

The benchmark has three parts:

1. **Task fixtures** (`tasks.go`): 8 tasks targeting the knowing codebase, each
   with a description, ground truth (relevant files, key symbols, answer
   keywords), and a complexity rating.

2. **Transcript analyzer** (`transcript.go`): Parses Claude Code JSONL session
   transcripts to extract token counts, tool call counts, wall-clock time, files
   read, and correctness scores against the ground truth.

3. **Comparison engine** (`compare.go`): Computes the deltas between control
   (without knowing) and treatment (with knowing) sessions, and renders a
   Markdown report.

The benchmark does NOT run Claude Code automatically. You run sessions manually
(or via the runner script) and then point the analyzer at the resulting JSONL
files.

## Step 1: Export tasks

Generate `tasks.json` so the runner script can read task descriptions without a
Go toolchain:

```bash
GOWORK=off go test ./bench/agent-efficiency/ -run TestExportTasks -v
```

This writes `bench/agent-efficiency/tasks.json`.

## Step 2: Run sessions

### Option A: Automated (requires claude CLI)

```bash
# Control session (no knowing tools)
./bench/agent-efficiency/runner.sh blast-radius-handler control

# Treatment session (with knowing tools)
./bench/agent-efficiency/runner.sh blast-radius-handler treatment
```

Run both modes for all 8 task IDs:

```bash
for task in blast-radius-handler context-engine-scoring node-struct-blast-radius \
            louvain-community-detection snapshot-package-coverage \
            hierarchical-merkle-diff edge-types file-save-to-cache-invalidation; do
  ./bench/agent-efficiency/runner.sh "$task" control
  ./bench/agent-efficiency/runner.sh "$task" treatment
done
```

### Option B: Manual

1. Open a Claude Code session in `/Users/dayna.blackwell/code/knowing`.
2. Paste the task description from `tasks.json`.
3. For control sessions: do not use any knowing MCP tools.
4. For treatment sessions: use knowing MCP tools freely.
5. Save the session JSONL transcript to:
   `bench/agent-efficiency/transcripts/<task-id>-<mode>.jsonl`

Claude Code stores session transcripts in
`~/.claude/projects/<project-hash>/`. Copy the relevant `.jsonl` file to the
transcripts directory with the naming convention above.

## Step 3: Analyze results

```bash
GOWORK=off go test ./bench/agent-efficiency/ -run TestAnalyzeTranscripts -v
```

This reads all transcripts in `bench/agent-efficiency/transcripts/`, computes
metrics, compares control vs. treatment pairs, and writes
`bench/agent-efficiency/FINDINGS.md`.

## Transcript naming convention

Transcripts must follow this naming pattern:

```
transcripts/<task-id>-<mode>.jsonl
```

Where `<mode>` is either `control` or `treatment`. Examples:

```
transcripts/blast-radius-handler-control.jsonl
transcripts/blast-radius-handler-treatment.jsonl
transcripts/context-engine-scoring-control.jsonl
transcripts/context-engine-scoring-treatment.jsonl
```

## Task list

| ID | Description | Complexity |
|----|-------------|------------|
| `blast-radius-handler` | Find the function that handles the blast_radius MCP tool | low |
| `context-engine-scoring` | Explain the context engine scoring formula and weights | medium |
| `node-struct-blast-radius` | What breaks if Node struct changes | medium |
| `louvain-community-detection` | Walk through the Louvain algorithm implementation | medium |
| `snapshot-package-coverage` | What test files cover the snapshot package | low |
| `hierarchical-merkle-diff` | How hierarchical Merkle tree improves diff performance | high |
| `edge-types` | List all supported edge types and where they are defined | medium |
| `file-save-to-cache-invalidation` | Trace data flow from git commit to cache invalidation | high |

## Metrics collected

| Metric | Description |
|--------|-------------|
| `TotalTokens` | Input + output tokens across all turns |
| `ToolCalls` | Total number of tool_use blocks |
| `ToolCallsByType` | Per-tool breakdown (Read, Grep, knowing_context, etc.) |
| `Turns` | Number of assistant messages |
| `WallClockMs` | Time from first user message to last assistant message |
| `FilesRead` | Unique files opened via Read tool |
| `FoundRelevantFiles` | Ground-truth relevant files that were actually read |
| `FoundKeySymbols` | Ground-truth key symbols that appeared in assistant output |
| `AnswerCorrectness` | Fraction of expected answer keywords present in final response |

## Interpreting results

- **Token savings**: positive means treatment used fewer tokens.
- **Tool call savings**: positive means treatment made fewer tool calls.
- **Time savings**: positive means treatment finished faster.
- **Correctness delta**: positive means treatment gave more correct answers.

A good result shows knowing tools providing significant token and tool call
savings while maintaining or improving correctness.
