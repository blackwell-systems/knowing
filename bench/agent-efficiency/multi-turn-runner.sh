#!/usr/bin/env bash
# multi-turn-runner.sh - Multi-turn agent efficiency benchmark runner.
#
# Runs coding tasks that require multi-file understanding and editing.
# Each task runs in an isolated git worktree.
#
# Usage:
#   ./multi-turn-runner.sh <task-id> <mode>
#   ./multi-turn-runner.sh --list
#   ./multi-turn-runner.sh --run-all
#
# Environment:
#   BENCH_MODEL=sonnet        Model to use (default: sonnet)
#   BENCH_TIMEOUT=300         Max seconds per session (default: 300)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TRANSCRIPTS_DIR="${SCRIPT_DIR}/transcripts/multi-turn"
MODEL="${BENCH_MODEL:-sonnet}"
TIMEOUT="${BENCH_TIMEOUT:-600}"  # 10 minutes: multi-file tasks need time to iterate

ALL_TASKS="add-json-flag add-symbol-info-tool refactor-return-type find-rwr-convergence-issue add-diff-since-flag cross-package-test-coverage interface-implementors cascading-breakage ambient-context"

get_prompt() {
  case "$1" in
    add-json-flag)
      echo 'Add a --json flag to the `knowing stale` command (in cmd/knowing/stale.go) that outputs results as JSON instead of human-readable text. The flag should be a bool, default false. When set, output a JSON object with fields: stale_files ([]string of changed file paths), stale_node_count (int total stale nodes), checked_at (ISO 8601 timestamp). Keep the existing human-readable output as the default.'
      ;;
    add-symbol-info-tool)
      echo 'Add a `symbol_info` MCP tool to the knowing server. It takes a `qualified_name` string parameter and returns JSON with: the node'\''s kind, file_path (from File table), line number, incoming edges (callers with their qualified names), and outgoing edges (callees with their qualified names). Register it in registerTools() in internal/mcp/server.go. Implementation goes in internal/mcp/handlers.go. Use store.NodesByQualifiedName to find the node, then store.EdgesTo and store.EdgesFrom for edges.'
      ;;
    refactor-return-type)
      echo 'Refactor the `InferExternalRepoURL` function in internal/resolve/external.go. Currently it returns a plain string ("external://...", "stdlib", or ""). Change the return type to: type ExternalResult struct { URL string; Kind string } where Kind is "external", "stdlib", or "local" (for empty string returns). Update the function signature, its implementation, ALL callers across the codebase, and all tests. You must find every file that calls this function yourself.'
      ;;
    find-rwr-convergence-issue)
      echo 'The context engine'\''s Random Walk with Restart in internal/context/walk.go sometimes produces nearly identical scores for unrelated symbols when the graph has disconnected components. Read walk.go, understand how the restart probability works, identify why disconnected components get similar scores, and add a code comment at the relevant location explaining the root cause and a proposed fix. Do not change the algorithm logic, just add the explanatory comment.'
      ;;
    add-diff-since-flag)
      echo 'Add a `--since` flag to the `knowing diff` command in cmd/knowing/main.go (the cmdDiff function). The flag accepts a Go duration string (e.g., "24h", "168h" for 7 days). When set, it should: 1) Run `git log --since=<duration> --name-only --pretty=format:` to get files changed in that period, 2) Look up which packages those files belong to using store.NodesByFilePath, 3) Output a table of packages sorted by change count (most changed first). Format: one package per line with change count.'
      ;;
    cross-package-test-coverage)
      echo 'Add a new benchmark test in bench/test-scope-accuracy/ called TestCrossPackageTestCoverage. It should: 1) Index the knowing repo into a temp DB, 2) For each package that has a _test.go file, count how many of its functions are called by tests in OTHER packages (not its own test file), 3) Report: package name, total exported functions, cross-package test callers, coverage percentage. Use store.EdgesTo to find incoming '\''tests'\'' edges from other packages. Output as a t.Log table sorted by coverage ascending (least covered first).'
      ;;
    interface-implementors)
      echo 'The GraphStore interface in internal/types/interfaces.go defines the contract that all store implementations must satisfy. I want to add a new method to this interface: SymbolsByKind(ctx context.Context, kind string) ([]Node, error). Add this method to the interface, then implement it in EVERY type that implements GraphStore. You must find all implementors yourself. Do not assume you know which files implement this interface. After adding the method everywhere, verify the build passes.'
      ;;
    cascading-breakage)
      echo 'I want to understand what would break if I deleted the ComputeNodeHash function from internal/types/hash.go. Do NOT actually delete it. Instead, find every direct caller of ComputeNodeHash, then for each caller find THEIR callers (second-hop dependents). Report a two-level dependency tree: level 1 (direct callers with file:line) and level 2 (callers of callers). This requires transitive analysis, not just grep for the function name.'
      ;;
    ambient-context)
      echo 'I am about to modify the RankSymbols function in internal/context/ranking.go. Before I start editing, I need to understand: 1) What calls RankSymbols and what do those callers expect from it? 2) What does RankSymbols call internally (its dependencies)? 3) What test functions exercise RankSymbols? 4) Are there any other ranking-related functions in the same package that interact with it? Give me a complete map of this function'\''s neighborhood so I can edit it safely.'
      ;;
    *)
      echo ""
      ;;
  esac
}

get_verify() {
  case "$1" in
    add-json-flag)          echo "GOWORK=off go build ./cmd/knowing/" ;;
    add-symbol-info-tool)   echo "GOWORK=off go build ./..." ;;
    refactor-return-type)   echo "GOWORK=off go build ./... && GOWORK=off go test ./internal/resolve/ -timeout 1m" ;;
    find-rwr-convergence-issue) echo "GOWORK=off go build ./internal/context/" ;;
    add-diff-since-flag)    echo "GOWORK=off go build ./cmd/knowing/" ;;
    cross-package-test-coverage) echo "GOWORK=off go vet ./bench/test-scope-accuracy/" ;;
    interface-implementors) echo "GOWORK=off go build ./..." ;;
    cascading-breakage)     echo "true" ;;
    ambient-context)        echo "true" ;;
    *)                      echo "true" ;;
  esac
}

list_tasks() {
  echo "Available multi-turn tasks:"
  echo ""
  for task in $ALL_TASKS; do
    echo "  $task"
  done
  echo ""
  echo "Usage: $0 <task-id> <mode>"
  echo "  mode: control | treatment"
}

run_task() {
  local task_id="$1"
  local mode="$2"

  local prompt
  prompt="$(get_prompt "$task_id")"
  if [ -z "$prompt" ]; then
    echo "ERROR: unknown task '$task_id'. Use --list to see available tasks."
    exit 1
  fi

  local verify
  verify="$(get_verify "$task_id")"

  # Create isolated worktree.
  local worktree_branch="bench-mt-${task_id}-${mode}-$$"
  local worktree_path="/tmp/knowing-bench-${task_id}-${mode}"
  rm -rf "$worktree_path" 2>/dev/null || true

  echo "Creating worktree at $worktree_path..."
  git -C "$REPO_ROOT" worktree add "$worktree_path" -b "$worktree_branch" HEAD --quiet

  # Copy MCP config so knowing tools are available in the worktree.
  if [ -f "$REPO_ROOT/.mcp.json" ]; then
    cp "$REPO_ROOT/.mcp.json" "$worktree_path/.mcp.json"
  fi
  # Symlink .claude settings so hooks and permissions carry over.
  if [ -d "$REPO_ROOT/.claude" ]; then
    ln -sf "$REPO_ROOT/.claude" "$worktree_path/.claude"
  fi

  # Build system note based on mode.
  local system_note
  if [ "$mode" = "control" ]; then
    system_note="Do NOT use any knowing MCP tools (mcp__knowing__*). Use only Grep, Read, Glob, Bash, Edit, Write. Be concise and efficient. Stop as soon as the task is complete."
  else
    system_note="You have a code knowledge graph. Use these tools directly (no ToolSearch needed):
- mcp__knowing__context_for_task: pass task description, get ranked relevant symbols
- mcp__knowing__blast_radius: pass target_hash, get all callers/dependents
- mcp__knowing__graph_query: pass symbol name prefix, find definitions
- mcp__knowing__test_scope: pass changed files, get affected tests
Call context_for_task FIRST to understand the codebase structure, then make targeted edits. Be concise and efficient. Stop as soon as the task is complete."
  fi

  local full_prompt="${prompt}

${system_note}

After completing the task, verify with: ${verify}
If the build/test passes, say TASK COMPLETE. If it fails, fix the errors and try again."

  # Ensure transcripts directory exists.
  mkdir -p "$TRANSCRIPTS_DIR"
  local transcript_file="${TRANSCRIPTS_DIR}/${task_id}-${mode}.jsonl"

  echo "Running task '${task_id}' in ${mode} mode (model: ${MODEL}, timeout: ${TIMEOUT}s)..."
  echo "Transcript: ${transcript_file}"
  echo ""

  # Run claude in the worktree.
  (
    cd "$worktree_path"
    timeout "$TIMEOUT" claude \
      --print \
      --output-format stream-json \
      --model "$MODEL" \
      -p "$full_prompt" \
      > "$transcript_file" 2>/dev/null || true
  )

  # Verify result in the worktree.
  local build_ok="false"
  if (cd "$worktree_path" && eval "$verify" >/dev/null 2>&1); then
    build_ok="true"
  fi

  # Record verification result.
  echo "{\"task\":\"${task_id}\",\"mode\":\"${mode}\",\"build_success\":${build_ok},\"verify_command\":\"${verify}\"}" \
    > "${TRANSCRIPTS_DIR}/${task_id}-${mode}-verify.json"

  if [ "$build_ok" = "true" ]; then
    echo "  VERIFY: PASS"
  else
    echo "  VERIFY: FAIL"
  fi

  # Cleanup worktree.
  git -C "$REPO_ROOT" worktree remove "$worktree_path" --force 2>/dev/null || true
  git -C "$REPO_ROOT" branch -D "$worktree_branch" 2>/dev/null || true

  echo "Done: ${task_id} (${mode})"
  echo ""
}

# --- Main ---

if [ $# -lt 1 ]; then
  list_tasks
  exit 0
fi

case "$1" in
  --list)
    list_tasks
    ;;
  --run-all)
    echo "=== Multi-Turn Agent Efficiency Benchmark ==="
    echo "Tasks: $(echo $ALL_TASKS | wc -w | tr -d ' ')"
    echo "Modes: 2 (control, treatment)"
    echo "Model: ${MODEL}"
    echo "Timeout: ${TIMEOUT}s per session"
    echo ""

    TOTAL=$(( $(echo $ALL_TASKS | wc -w | tr -d ' ') * 2 ))
    CURRENT=0

    for mode in control treatment; do
      echo "--- ${mode} mode ---"
      for task in $ALL_TASKS; do
        CURRENT=$((CURRENT + 1))
        echo "[${CURRENT}/${TOTAL}] ${task} (${mode})..."
        run_task "$task" "$mode"
      done
    done

    echo "=== All sessions complete ==="
    echo "Analyze: GOWORK=off go test ./bench/agent-efficiency/ -run TestAnalyzeMultiTurn -v"
    ;;
  *)
    if [ $# -lt 2 ]; then
      echo "Usage: $0 <task-id> <mode>"
      echo "  mode: control | treatment"
      exit 1
    fi
    if [ "$2" != "control" ] && [ "$2" != "treatment" ]; then
      echo "ERROR: mode must be 'control' or 'treatment', got '$2'"
      exit 1
    fi
    run_task "$1" "$2"
    ;;
esac
