#!/usr/bin/env bash
# run-repo.sh - Run agent efficiency benchmark against any indexed repo.
#
# Usage:
#   ./run-repo.sh <repo> <task-id> <mode>
#   ./run-repo.sh <repo> --list
#   ./run-repo.sh <repo> --run-all
#
# Repos: knowing (93K LOC Go), django (473K LOC Python)
# Modes: control, treatment
#
# Environment:
#   BENCH_MODEL=sonnet            Model (default: sonnet)
#   BENCH_TIMEOUT=600             Max seconds per session (default: 600)
#   BENCH_PROVIDER=bedrock|max    Backend (default: bedrock)
#
# Examples:
#   ./run-repo.sh django django-queryset-callers control
#   BENCH_PROVIDER=max ./run-repo.sh django --run-all
#   BENCH_MODEL=opus ./run-repo.sh knowing ambient-context treatment
#
# Transcripts saved to: transcripts/<repo>/<task>-<mode>.jsonl
# This structure keeps results organized by target codebase for cross-repo comparison.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODEL="${BENCH_MODEL:-sonnet}"
TIMEOUT="${BENCH_TIMEOUT:-600}"
# Provider: "bedrock" or "max" (controls which claude backend to use).
# bedrock: uses your AWS Bedrock credits (default)
# max: uses Claude Max subscription (no per-token cost)
PROVIDER="${BENCH_PROVIDER:-bedrock}"

if [ $# -lt 1 ]; then
  echo "Usage: $0 <repo> <task-id> <mode>"
  echo "       $0 <repo> --list"
  echo "       $0 <repo> --run-all"
  echo ""
  echo "Available repos: knowing, django"
  exit 1
fi

REPO="$1"
shift

# Load repo-specific task definitions.
case "$REPO" in
  knowing)
    # Source the knowing tasks from multi-turn-runner.sh functions
    REPO_PATH="$REPO_ROOT"
    DB_PATH=""  # uses default (roster lookup)
    source "${SCRIPT_DIR}/tasks-knowing.sh"
    ALL_TASKS="$KNOWING_TASKS"
    ;;
  django)
    REPO_PATH="$(cd "${REPO_ROOT}/bench/cross-system/corpus/repos/django" && pwd)"
    # Django needs its own indexed DB. Check if it exists.
    DB_PATH="${REPO_PATH}/.knowing/graph.db"
    if [ ! -f "$DB_PATH" ]; then
      echo "Django not indexed. Indexing now..."
      knowing index -db "$DB_PATH" -url "github.com/django/django" "$REPO_PATH"
    fi
    source "${SCRIPT_DIR}/tasks-django.sh"
    ALL_TASKS="$DJANGO_TASKS"
    ;;
  *)
    echo "Unknown repo: $REPO"
    echo "Available: knowing, django"
    exit 1
    ;;
esac

TRANSCRIPTS_DIR="${SCRIPT_DIR}/transcripts/${REPO}"
mkdir -p "$TRANSCRIPTS_DIR"

list_tasks() {
  echo "Tasks for ${REPO}:"
  echo ""
  for task in $ALL_TASKS; do
    echo "  $task"
  done
  echo ""
  echo "Usage: $0 $REPO <task-id> <mode>"
}

run_task() {
  local task_id="$1"
  local mode="$2"

  # Get prompt from repo-specific function.
  local prompt
  case "$REPO" in
    knowing) prompt="$(get_prompt_knowing "$task_id")" ;;
    django)  prompt="$(get_prompt_django "$task_id")" ;;
  esac

  if [ -z "$prompt" ]; then
    echo "ERROR: unknown task '$task_id' for repo '$REPO'"
    exit 1
  fi

  # System note based on mode.
  local system_note
  if [ "$mode" = "control" ]; then
    system_note="Do NOT use any knowing MCP tools (mcp__knowing__*). Use only Grep, Read, Glob, Bash. Be concise and efficient. Stop as soon as you have enough information to answer completely."
  else
    system_note="You have a code knowledge graph indexed for this repository. Use these tools directly:
- mcp__knowing__context_for_task: pass task description, get ranked relevant symbols
- mcp__knowing__blast_radius: pass target_hash, get all callers/dependents
- mcp__knowing__graph_query: pass symbol name prefix, find definitions
- mcp__knowing__test_scope: pass changed files, get affected tests
- mcp__knowing__flow_between: find paths between two symbols
Call context_for_task FIRST to understand the codebase structure. Be concise and efficient. Stop as soon as you have enough information to answer completely."
  fi

  local full_prompt="${prompt}

${system_note}

Codebase is at: ${REPO_PATH}
Answer concisely. Report findings with file paths and line numbers where possible."

  local transcript_file="${TRANSCRIPTS_DIR}/${task_id}-${mode}.jsonl"

  echo "Running ${REPO}/${task_id} in ${mode} mode (model: ${MODEL}, timeout: ${TIMEOUT}s)..."
  echo "Transcript: ${transcript_file}"

  # Build claude command based on provider.
  if [ -n "$DB_PATH" ]; then
    export KNOWING_DB="$DB_PATH"
  fi

  local claude_cmd="claude"
  local claude_args="--print --output-format stream-json --model ${MODEL}"

  case "$PROVIDER" in
    bedrock)
      # Default: uses AWS Bedrock via configured profile
      ;;
    max)
      # Claude Max: no per-token cost, uses subscription
      claude_args="--print --output-format stream-json --model ${MODEL} --provider claude-max"
      ;;
    *)
      echo "ERROR: unknown provider '$PROVIDER' (use: bedrock, max)"
      exit 1
      ;;
  esac

  (
    cd "$REPO_PATH"
    timeout "$TIMEOUT" $claude_cmd \
      $claude_args \
      -p "$full_prompt" \
      > "$transcript_file" 2>/dev/null || true
  )

  # Check transcript has content.
  if [ ! -s "$transcript_file" ]; then
    echo "  ERROR: empty transcript"
    return 1
  fi

  local lines
  lines=$(wc -l < "$transcript_file" | tr -d ' ')
  echo "  Done: ${lines} lines"
  echo ""
}

# --- Main ---

case "${1:-}" in
  --list)
    list_tasks
    ;;
  --run-all)
    echo "=== Agent Efficiency: ${REPO} ==="
    echo "Tasks: $(echo $ALL_TASKS | wc -w | tr -d ' ')"
    echo "Model: ${MODEL}, Timeout: ${TIMEOUT}s"
    echo ""

    for mode in control treatment; do
      echo "--- ${mode} mode ---"
      for task in $ALL_TASKS; do
        run_task "$task" "$mode"
      done
    done

    echo "=== Complete ==="
    ;;
  *)
    if [ $# -lt 2 ]; then
      echo "Usage: $0 $REPO <task-id> <mode>"
      exit 1
    fi
    run_task "$1" "$2"
    ;;
esac
