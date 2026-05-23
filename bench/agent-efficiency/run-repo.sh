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
  echo "Modes: control, treatment, aider"
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

# run_aider executes a task using Aider's own agent loop.
# Aider uses tree-sitter repo-map + PageRank for context (their equivalent of knowing).
# Output is saved as a pseudo-JSONL transcript for the analyzer.
run_aider() {
  local task_id="$1"
  local prompt="$2"
  local transcript_file="$3"

  local aider_bin="/tmp/aider-bench/bin/aider"
  if [ ! -x "$aider_bin" ]; then
    echo "  ERROR: aider not installed at $aider_bin"
    echo "  Install: uv venv /tmp/aider-bench --python 3.11 && source /tmp/aider-bench/bin/activate && uv pip install aider-chat"
    echo '{"type":"result","is_error":true,"result":"aider not installed"}' > "$transcript_file"
    return 1
  fi

  # Aider needs an LLM API key. Use Claude via Anthropic API or set OPENAI_API_KEY.
  # For benchmarking we use the same model class (sonnet equivalent).
  local aider_model="claude-3-5-sonnet-20241022"
  if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
    aider_model="claude-3-5-sonnet-20241022"
  elif [ -n "${OPENAI_API_KEY:-}" ]; then
    aider_model="gpt-4o"
  else
    echo "  ERROR: aider needs ANTHROPIC_API_KEY or OPENAI_API_KEY"
    echo '{"type":"result","is_error":true,"result":"no API key for aider"}' > "$transcript_file"
    return 1
  fi

  local start_time
  start_time=$(python3 -c "import time; print(int(time.time()*1000))")

  # Run aider in message mode (non-interactive, single task).
  # --yes-always: auto-accept all edits
  # --no-git: don't create commits (we verify separately)
  # --no-auto-commits: same
  # --message: the task prompt
  local aider_output
  aider_output=$(cd "$REPO_PATH" && timeout "$TIMEOUT" "$aider_bin" \
    --model "$aider_model" \
    --yes-always \
    --no-git \
    --no-auto-commits \
    --message "$prompt

Answer concisely. Report findings with file paths and line numbers where possible." \
    2>&1) || true

  local end_time
  end_time=$(python3 -c "import time; print(int(time.time()*1000))")
  local duration_ms=$((end_time - start_time))

  # Convert aider output to pseudo-JSONL format for the analyzer.
  # We create a minimal transcript with tool-use-like structure.
  local output_tokens
  output_tokens=$(echo "$aider_output" | wc -w | tr -d ' ')

  python3 -c "
import json, sys

output = sys.stdin.read()
lines = output.split('\n')

# Count aider's tool-like operations from its output.
edits = sum(1 for l in lines if 'Applied edit' in l or 'wrote' in l.lower())
searches = sum(1 for l in lines if 'searching' in l.lower() or 'grep' in l.lower())
reads = sum(1 for l in lines if 'read' in l.lower() or 'loading' in l.lower())

# Write pseudo-transcript.
transcript = [
    {'type': 'system', 'subtype': 'init', 'session_id': 'aider-${task_id}', 'tools': ['aider-repomap', 'aider-edit'], 'model': '${aider_model}'},
    {'type': 'assistant', 'message': {'role': 'assistant', 'content': [
        {'type': 'text', 'text': output}
    ], 'usage': {'input_tokens': 0, 'output_tokens': ${output_tokens}}}},
    {'type': 'result', 'subtype': 'success', 'is_error': False, 'duration_ms': ${duration_ms}, 'result': output[-500:] if len(output) > 500 else output, 'session_id': 'aider-${task_id}', 'usage': {'input_tokens': 0, 'output_tokens': ${output_tokens}}, 'aider_meta': {'edits': edits, 'searches': searches, 'reads': reads, 'total_output_chars': len(output)}}
]

for entry in transcript:
    print(json.dumps(entry))
" <<< "$aider_output" > "$transcript_file"

  local lines
  lines=$(wc -l < "$transcript_file" | tr -d ' ')
  echo "  Done: ${lines} lines, ${duration_ms}ms"
}

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
  case "$mode" in
    control)
      system_note="Do NOT use any knowing MCP tools (mcp__knowing__*). Use only Grep, Read, Glob, Bash. Be concise and efficient. Stop as soon as you have enough information to answer completely."
      ;;
    treatment)
      system_note="You have a code knowledge graph indexed for this repository. Use these tools directly:
- mcp__knowing__context_for_task: pass task description, get ranked relevant symbols
- mcp__knowing__blast_radius: pass target_hash, get all callers/dependents
- mcp__knowing__graph_query: pass symbol name prefix, find definitions
- mcp__knowing__test_scope: pass changed files, get affected tests
- mcp__knowing__flow_between: find paths between two symbols
Call context_for_task FIRST to understand the codebase structure. Be concise and efficient. Stop as soon as you have enough information to answer completely."
      ;;
    aider)
      # Aider mode: runs Aider's own agent loop instead of Claude Code.
      # No system_note needed; Aider handles its own prompting.
      system_note=""
      ;;
  esac

  local full_prompt="${prompt}

${system_note}

Codebase is at: ${REPO_PATH}
Answer concisely. Report findings with file paths and line numbers where possible."

  local transcript_file="${TRANSCRIPTS_DIR}/${task_id}-${mode}.jsonl"

  echo "Running ${REPO}/${task_id} in ${mode} mode (model: ${MODEL}, timeout: ${TIMEOUT}s)..."
  echo "Transcript: ${transcript_file}"

  if [ "$mode" = "aider" ]; then
    # Run Aider instead of Claude Code.
    run_aider "$task_id" "$prompt" "$transcript_file"
    return
  fi

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
