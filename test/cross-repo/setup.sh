#!/bin/bash
# Initialize the cross-repo test fixture.
# Creates independent git repos for each module so knowing add/index
# detects them as separate repositories with correct URLs.
#
# Usage: ./test/cross-repo/setup.sh

set -e

DIR="$(cd "$(dirname "$0")" && pwd)"

for mod in module-a module-b module-c; do
    moddir="$DIR/$mod"
    if [ -d "$moddir/.git" ]; then
        echo "$mod: already initialized"
        continue
    fi
    git init "$moddir"
    git -C "$moddir" add .
    git -C "$moddir" commit -m "initial"
    git -C "$moddir" remote add origin "https://github.com/blackwell-systems/cross-repo-test/$mod"
    echo "$mod: initialized"
done

echo ""
echo "Fixture ready. Index with:"
echo "  knowing add $DIR/module-a"
echo "  knowing add $DIR/module-b"
echo "  knowing add $DIR/module-c"
