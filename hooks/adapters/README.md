# Harness adapters for `knowing hook`

`knowing hook <event>` speaks one transport-neutral contract (see
`docs/architecture/hooks-integration.md`): a JSON payload in on stdin, a JSON
result out on stdout. Wiring knowing into a new harness is therefore a config
task, not a code task. This directory holds thin examples.

## The contract in one line

```
echo '{"task":"<prompt>"}' | knowing hook pre-task            # -> {"context": "...", ...}
echo '{"file":"x.go","content":"<edit>"}' | knowing hook pre-edit
echo '{}'                | knowing hook session-start
```

`--emit neutral` (default) returns a JSON object with a `context` string. Inject
`context` if it is non-empty; otherwise do nothing.

## Claude Code

No adapter needed. `knowing hook` accepts a raw Claude Code hook payload and can
emit Claude Code's native envelope directly:

```json
{
  "hooks": {
    "PreToolUse":   [{ "matcher": "Edit|Write", "command": "knowing hook pre-edit --emit claude-code" }],
    "SessionStart": [{ "command": "knowing hook session-start --emit claude-code" }]
  }
}
```

## Any other harness

Two patterns, depending on what the harness gives your hook:

1. **The harness already emits JSON with a `task`/`prompt` or `file` field.** Pipe
   it straight into `knowing hook <event>` and read `.context` from the result.
   `generic.sh` in this directory is a complete example (uses `jq`).

2. **The harness passes fields as arguments or env vars.** Build the neutral
   JSON yourself, one line:

   ```sh
   printf '{"task":%s}' "$(printf '%s' "$PROMPT" | jq -Rs .)" \
     | knowing hook pre-task | jq -r .context
   ```

That is the whole integration surface.
