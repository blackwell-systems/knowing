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

# Model to use for benchmark sessions. Override with BENCH_MODEL env var.
# Bedrock model IDs use the full ARN format. Sonnet is fast and cheap for benchmarking.
# Examples:
#   BENCH_MODEL=claude-sonnet-4-6          (Anthropic API)
#   BENCH_MODEL=sonnet                     (Claude Code shorthand)
#   BENCH_MODEL=us.anthropic.claude-sonnet-4-6-v1  (Bedrock)
MODEL="${BENCH_MODEL:-sonnet}"

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
  SYSTEM_NOTE="Answer without using any knowing MCP tools (mcp__knowing__*). Use only standard tools (Grep, Read, Glob, Bash). Be concise and efficient."
else
  SYSTEM_NOTE="You have a code knowledge graph available. Use these tools directly (no need to search for them):
- mcp__knowing__context_for_task: pass a task description, get ranked relevant symbols in one call
- mcp__knowing__blast_radius: pass a target_hash, get all callers/dependents
- mcp__knowing__graph_query: pass a symbol name prefix, find where it's defined
- mcp__knowing__test_scope: pass changed files, get affected tests
Call context_for_task FIRST with the task description, then use Read only for files it identifies. Do NOT use Grep to explore; the graph already knows the structure. Be concise and efficient."
fi

FULL_PROMPT="${DESCRIPTION}

${SYSTEM_NOTE}

Answer concisely. Do not over-explore. Stop as soon as you have enough information to answer."

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
#
# Both modes use the same invocation. Control mode relies on the system prompt
# instruction to avoid knowing tools. The analyzer validates compliance by
# checking that no mcp__knowing__* tool calls appear in control transcripts.
# If any do, the session is flagged as invalid and should be re-run.
#
# claude --print may exit non-zero even on success (stream-json mode quirk).
# We check for transcript content rather than exit code.
claude \
  --print \
  --output-format stream-json \
  --model "${MODEL}" \
  -p "${FULL_PROMPT}" \
  > "${TRANSCRIPT_FILE}" || true

# Validate that we got output.
if [[ ! -s "${TRANSCRIPT_FILE}" ]]; then
  echo "ERROR: transcript is empty (claude may have failed to start)"
  exit 1
fi

echo ""
echo "Done. Transcript saved to: ${TRANSCRIPT_FILE}"
echo ""
echo "To analyze results:"
echo "  GOWORK=off go test ./bench/agent-efficiency/ -run TestAnalyzeTranscripts -v"
