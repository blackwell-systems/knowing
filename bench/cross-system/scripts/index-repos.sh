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

index_repo() {
    local name="$1" url="$2"
    local repo_dir="$CORPUS_DIR/$name"

    if [ ! -d "$repo_dir/.git" ]; then
        echo "[$name] Skipping (not cloned)"
        return
    fi

    local db_path="$repo_dir/.knowing/graph.db"
    if [ -f "$db_path" ]; then
        echo "[$name] Already indexed at $db_path"
        return
    fi

    echo "[$name] Indexing..."
    local start=$(date +%s)
    (cd "$repo_dir" && knowing index -url "$url" -db "$db_path" .) 2>&1 | tail -3
    local end=$(date +%s)
    echo "[$name] Done in $((end - start))s"
    echo ""
}

index_repo kubernetes "github.com/kubernetes/kubernetes"
index_repo typescript "github.com/microsoft/TypeScript"
index_repo flask      "github.com/pallets/flask"
index_repo cargo      "github.com/rust-lang/cargo"
index_repo django     "github.com/django/django"

echo "All repos indexed. Run the benchmark with:"
echo "  GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m"
