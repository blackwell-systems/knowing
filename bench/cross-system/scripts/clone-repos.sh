#!/usr/bin/env bash
# Clone benchmark evaluation repos at pinned versions.
# Usage: ./bench/cross-system/scripts/clone-repos.sh [corpus-dir]
#
# This clones all 5 repos into corpus/repos/ at their pinned commits.
# Shallow clones (depth=1) to save disk and time.

set -euo pipefail

CORPUS_DIR="${1:-bench/cross-system/corpus/repos}"
mkdir -p "$CORPUS_DIR"

clone_repo() {
    local name="$1" url="$2" tag="$3"
    local dest="$CORPUS_DIR/$name"

    if [ -d "$dest/.git" ]; then
        echo "[$name] Already cloned at $dest"
        return
    fi

    echo "[$name] Cloning $url at $tag..."
    git clone --depth 1 --branch "$tag" "$url" "$dest" 2>&1 | tail -1
    echo "[$name] Done ($(du -sh "$dest" | cut -f1))"
}

clone_repo kubernetes "https://github.com/kubernetes/kubernetes" "v1.30.0"
# Note: "typescript" was replaced by "vscode" (the TS compiler uses an unusual
# factory-function pattern that isn't representative of normal TS codebases)
clone_repo vscode "https://github.com/microsoft/vscode" "1.90.0"
clone_repo flask      "https://github.com/pallets/flask"        "3.1.0"
clone_repo cargo      "https://github.com/rust-lang/cargo"      "0.82.0"
clone_repo django     "https://github.com/django/django"        "5.1"

echo ""
echo "All repos cloned to $CORPUS_DIR/"
echo "Next: index each with knowing:"
echo "  ./bench/cross-system/scripts/index-repos.sh"
