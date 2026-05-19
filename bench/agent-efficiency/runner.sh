#!/usr/bin/env bash
# runner.sh - Agent Efficiency Benchmark Runner
#
# Usage:
#   ./runner.sh <task-id> <mode>
#
#   task-id: one of the IDs defined in tasks.go (also in tasks.json after
#            running TestExportTasks)
#   mode:    "control"   - run without knowing MCP tools
#            "treatment" - run with knowing MCP tools
#
# The script launches Claude Code with the task prompt in non-interactive mode
# (--print flag) and saves the resulting session transcript to:
#   bench/agent-efficiency/transcripts/<task-id>-<mode>.jsonl
#
# Prerequisites:
#   1. Install the claude CLI: https://docs.anthropic.com/en/docs/claude-code
#   2. Export tasks: GOWORK=off go test ./bench/agent-efficiency/ -run TestExportTasks
#   3. For treatment mode, ensure the knowing MCP server is configured in
#      ~/.claude.json (or your project's .claude/settings.json).
#
# Example:
#   ./runner.sh blast-radius-handler control
#   ./runner.sh blast-radius-handler treatment

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TRANSCRIPTS_DIR="${SCRIPT_DIR}/transcripts"
TASKS_JSON="${SCRIPT_DIR}/tasks.json"

# Ensure tasks.json exists.
if [[ ! -f "${TASKS_JSON}" ]]; then
  echo "ERROR: tasks.json not found at ${TASKS_JSON}"
  echo "Run: GOWORK=off go test ./bench/agent-efficiency/ -run TestExportTasks"
  exit 1
fi

# Validate arguments.
if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <task-id> <mode>"
  echo "  mode: control | treatment"
  echo ""
  echo "Available task IDs:"
  if command -v jq &>/dev/null; then
    jq -r '.[].id' "${TASKS_JSON}"
  else
    grep '"id"' "${TASKS_JSON}" | sed 's/.*"id": *"\([^"]*\)".*/  \1/'
  fi
  exit 1
fi

TASK_ID="$1"
MODE="$2"

if [[ "${MODE}" != "control" && "${MODE}" != "treatment" ]]; then
  echo "ERROR: mode must be 'control' or 'treatment', got '${MODE}'"
  exit 1
fi

# Look up the task description from tasks.json.
if command -v jq &>/dev/null; then
  DESCRIPTION="$(jq -r --arg id "${TASK_ID}" '.[] | select(.id == $id) | .description' "${TASKS_JSON}")"
else
  # Fallback: crude grep (requires single-line values).
  DESCRIPTION="$(python3 -c "
import json, sys
tasks = json.load(open('${TASKS_JSON}'))
match = next((t for t in tasks if t['id'] == '${TASK_ID}'), None)
if not match:
    sys.exit(1)
print(match['description'])
" 2>/dev/null || true)"
fi

if [[ -z "${DESCRIPTION}" ]]; then
  echo "ERROR: task '${TASK_ID}' not found in tasks.json"
  exit 1
fi

# Ensure transcripts directory exists.
mkdir -p "${TRANSCRIPTS_DIR}"

TRANSCRIPT_FILE="${TRANSCRIPTS_DIR}/${TASK_ID}-${MODE}.jsonl"

# Build the system prompt addendum depending on mode.
if [[ "${MODE}" == "control" ]]; then
  SYSTEM_NOTE="Answer without using any knowing MCP tools. Use only standard file-reading tools."
else
  SYSTEM_NOTE="You have access to knowing MCP tools (context_for_task, blast_radius, stale_edges, etc.). Use them to answer efficiently."
fi

FULL_PROMPT="${DESCRIPTION}

Note: ${SYSTEM_NOTE}

Codebase root: /Users/dayna.blackwell/code/knowing"

# Check that the claude CLI is available.
if ! command -v claude &>/dev/null; then
  echo "ERROR: claude CLI not found in PATH"
  echo ""
  echo "To run manually:"
  echo "  Open a new Claude Code session in /Users/dayna.blackwell/code/knowing"
  echo "  Use the following prompt:"
  echo "---"
  echo "${FULL_PROMPT}"
  echo "---"
  echo "  Save the session transcript (.jsonl) to:"
  echo "  ${TRANSCRIPT_FILE}"
  exit 1
fi

echo "Running task '${TASK_ID}' in ${MODE} mode..."
echo "Transcript will be saved to: ${TRANSCRIPT_FILE}"
echo ""

# Launch Claude Code in non-interactive (print) mode.
# --output-format stream-json writes JSONL to stdout.
# We redirect stdout to the transcript file.
claude \
  --print \
  --output-format stream-json \
  --no-auto-compact \
  -p "${FULL_PROMPT}" \
  > "${TRANSCRIPT_FILE}"

echo ""
echo "Done. Transcript saved to: ${TRANSCRIPT_FILE}"
echo ""
echo "To analyze results:"
echo "  GOWORK=off go test ./bench/agent-efficiency/ -run TestAnalyzeTranscripts -v"
