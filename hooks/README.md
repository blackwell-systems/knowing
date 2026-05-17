# knowing hooks

Claude Code hooks that automatically inject graph context before file edits.

## Setup

Add to your project's `.claude/settings.local.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "command": "./hooks/knowing-pre-edit"
      }
    ]
  }
}
```

## Configuration

All via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOWING_HOOKS` | `on` | Set to `off` to disable entirely |
| `KNOWING_HOOKS_DB` | `knowing.db` | Path to SQLite database |
| `KNOWING_HOOKS_BUDGET` | `1500` | Token budget for injected context |
| `KNOWING_HOOKS_FORMAT` | `kwf` | Output format (kwf for minimal tokens, xml for readability) |
| `KNOWING_HOOKS_LOG` | `.knowing-hooks.jsonl` | Metrics log path |
| `KNOWING_HOOKS_VERBOSE` | `0` | Set to `1` to print injection info to stderr |

## How it works

1. Claude Code calls `Edit` or `Write` on a file
2. The PreToolUse hook fires, passing the file path to `knowing context -files`
3. knowing returns graph-ranked symbols related to that file (callers, callees, related types)
4. The context is prepended to the tool result so Claude sees it before deciding what to write
5. Every invocation is logged to `.knowing-hooks.jsonl` with timing and token count

## A/B Testing

The hooks log every invocation with metrics. To evaluate whether they help:

```bash
# Session 1: hooks ON (default)
# Work normally, edit files, complete tasks.
cp .knowing-hooks.jsonl hooks-on.jsonl

# Session 2: hooks OFF
export KNOWING_HOOKS=off
# Work on similar tasks.

# Analyze
./hooks/analyze-hooks hooks-on.jsonl
```

Compare between sessions:
- Did the agent make fewer tool calls to understand context? (fewer context_for_task/files calls)
- Did the agent make fewer errors in edits? (fewer retry cycles)
- Was the injected context actually referenced in the edit?

## Metrics log format

Each line in `.knowing-hooks.jsonl`:

```json
{"ts":1716000000,"file":"internal/mcp/server.go","event":"inject","latency_ms":45,"tokens":312,"format":"kwf","budget":1500}
{"ts":1716000001,"file":"README.md","event":"miss","latency_ms":12,"tokens":0}
```

Events:
- `inject`: context was found and injected (tokens > 0)
- `miss`: no relevant context found (file not in graph, empty result)

## Analysis

```bash
./hooks/analyze-hooks
```

Produces: hit rate, token overhead (mean/p50/p95), latency, per-file breakdown, and A/B evaluation guidance.

## Turning off

```bash
export KNOWING_HOOKS=off
```

Or remove the hook from `.claude/settings.local.json`. No code changes needed.
