#!/usr/bin/env bash
# Index benchmark repos with knowing for the cross-system benchmark.
# Usage: ./bench/cross-system/scripts/index-repos.sh [corpus-dir]
#
# Runs `knowing index` on each repo in corpus/repos/.
# Assumes knowing is installed and on PATH.

set -euo pipefail

CORPUS_DIR="${1:-bench/cross-system/corpus/repos}"

if ! command -v knowing &>/dev/null; then
    echo "Error: knowing not found on PATH"
    echo "Install: go install github.com/blackwell-systems/knowing/cmd/knowing@latest"
    exit 1
fi

echo "Indexing repos for cross-system benchmark..."
echo ""

for repo_dir in "$CORPUS_DIR"/*/; do
    name=$(basename "$repo_dir")
    if [ ! -d "$repo_dir/.git" ]; then
        echo "[$name] Skipping (not a git repo)"
        continue
    fi

    echo "[$name] Indexing..."
    start=$(date +%s)
    knowing index --repo "$repo_dir" 2>&1 | tail -3
    end=$(date +%s)
    echo "[$name] Done in $((end - start))s"
    echo ""
done

echo "All repos indexed. Run the benchmark with:"
echo "  GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m"
