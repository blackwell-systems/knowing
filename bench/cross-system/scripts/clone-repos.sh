#!/usr/bin/env bash
# Clone benchmark evaluation repos at pinned versions.
# Usage: ./bench/cross-system/scripts/clone-repos.sh [corpus-dir]
#
# This clones all 5 repos into corpus/repos/ at their pinned commits.
# Shallow clones (depth=1) to save disk and time.

set -euo pipefail

CORPUS_DIR="${1:-bench/cross-system/corpus/repos}"
mkdir -p "$CORPUS_DIR"

declare -A REPOS
REPOS[kubernetes]="https://github.com/kubernetes/kubernetes|v1.30.0"
REPOS[typescript]="https://github.com/microsoft/TypeScript|v5.5.4"
REPOS[flask]="https://github.com/pallets/flask|3.1.0"
REPOS[cargo]="https://github.com/rust-lang/cargo|0.82.0"
REPOS[django]="https://github.com/django/django|5.1"

for name in "${!REPOS[@]}"; do
    IFS='|' read -r url tag <<< "${REPOS[$name]}"
    dest="$CORPUS_DIR/$name"

    if [ -d "$dest/.git" ]; then
        echo "[$name] Already cloned at $dest"
        continue
    fi

    echo "[$name] Cloning $url at $tag..."
    git clone --depth 1 --branch "$tag" "$url" "$dest" 2>&1 | tail -1
    echo "[$name] Done ($(du -sh "$dest" | cut -f1))"
done

echo ""
echo "All repos cloned to $CORPUS_DIR/"
echo "Next: index each with knowing:"
echo "  for repo in $CORPUS_DIR/*/; do knowing index --repo \"\$repo\"; done"
