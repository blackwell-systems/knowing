#!/usr/bin/env bash
# run-all.sh - Run all 16 agent efficiency benchmark sessions.
#
# This runs 8 tasks x 2 modes (control + treatment) using claude --print.
# Total expected cost: ~$2-5 depending on model and task complexity.
# Total expected time: ~5-10 minutes.
#
# Prerequisites:
#   1. claude CLI available on PATH
#   2. tasks.json exported (run: GOWORK=off go test ./bench/agent-efficiency/ -run TestExportTasks)
#   3. For treatment mode: knowing MCP server configured in .mcp.json
#
# Usage:
#   ./bench/agent-efficiency/run-all.sh           # run all 16 sessions
#   ./bench/agent-efficiency/run-all.sh --dry-run # show what would be run

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNNER="${SCRIPT_DIR}/runner.sh"

DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=true
fi

TASKS=(
  blast-radius-handler
  context-engine-scoring
  node-struct-blast-radius
  louvain-community-detection
  snapshot-package-coverage
  hierarchical-merkle-diff
  edge-types
  file-save-to-cache-invalidation
)

MODES=(control treatment)

echo "=== Agent Efficiency Benchmark ==="
echo "Tasks: ${#TASKS[@]}"
echo "Modes: ${#MODES[@]} (control, treatment)"
echo "Total sessions: $((${#TASKS[@]} * ${#MODES[@]}))"
echo ""

if [[ "${DRY_RUN}" == "true" ]]; then
  echo "[DRY RUN] Would execute:"
  for task in "${TASKS[@]}"; do
    for mode in "${MODES[@]}"; do
      echo "  ${RUNNER} ${task} ${mode}"
    done
  done
  exit 0
fi

# Run control sessions first (no knowing tools), then treatment (with knowing).
# This ordering prevents the knowing MCP server from being warmed up by
# treatment sessions and potentially caching results that help control.
TOTAL=$((${#TASKS[@]} * ${#MODES[@]}))
CURRENT=0

for mode in "${MODES[@]}"; do
  echo "--- ${mode} mode ---"
  for task in "${TASKS[@]}"; do
    CURRENT=$((CURRENT + 1))
    echo "[${CURRENT}/${TOTAL}] ${task} (${mode})..."
    if "${RUNNER}" "${task}" "${mode}"; then
      echo "  done."
    else
      echo "  FAILED (exit $?). Continuing..."
    fi
    echo ""
  done
done

echo "=== All sessions complete ==="
echo ""
echo "Analyze results:"
echo "  GOWORK=off go test ./bench/agent-efficiency/ -run TestAnalyzeTranscripts -v"
