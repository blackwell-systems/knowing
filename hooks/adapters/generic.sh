#!/bin/bash
# generic.sh: example adapter wiring `knowing hook` into an arbitrary agentic
# harness. Reads the harness's JSON event on stdin, forwards it to the neutral
# `knowing hook` contract, and prints the injectable context (plain text) on
# stdout. Prints nothing when there is no context (the harness treats empty
# output as a no-op).
#
# Usage in a harness that calls a hook command with the event JSON on stdin:
#   pre-task hook:  ./hooks/adapters/generic.sh pre-task
#   pre-edit hook:  ./hooks/adapters/generic.sh pre-edit
#   session-start:  ./hooks/adapters/generic.sh session-start
#
# The neutral contract accepts these payload fields (all optional):
#   {"event","file","content","task","files":[...]}
# and also folds common Claude Code keys, so most harness payloads pass through
# unchanged. Adjust the jq remap below only if your harness uses different keys.

set -euo pipefail

EVENT="${1:-pre-task}"
DB="${KNOWING_HOOKS_DB:-knowing.db}"
BUDGET="${KNOWING_HOOKS_BUDGET:-400}"

# Locate the binary (PATH or local build); exit quietly if absent.
if command -v knowing >/dev/null 2>&1; then
  KNOWING=knowing
elif [ -x ./knowing ]; then
  KNOWING=./knowing
else
  exit 0
fi

# Forward stdin verbatim; knowing hook is liberal about input shape.
# `.context` is empty when the repo is unindexed or nothing matched.
INPUT="$(cat)"
CONTEXT="$(printf '%s' "$INPUT" \
  | "$KNOWING" hook "$EVENT" --db "$DB" --budget "$BUDGET" 2>/dev/null \
  | jq -r '.context // empty')" || exit 0

[ -n "$CONTEXT" ] && printf '%s\n' "$CONTEXT"
exit 0
