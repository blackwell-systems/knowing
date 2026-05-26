#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# TanStack / Mini Shai-Hulud Supply Chain Detection Demo
#
# SAFETY: This script downloads source code only. No npm install, no lifecycle
# hooks, no code execution. knowing indexes via tree-sitter AST parsing.
# =============================================================================

echo "=== TanStack Supply Chain Detection Demo ==="
echo ""
echo "SAFETY: No code execution. AST parsing only."
echo ""

WORK_DIR=$(mktemp -d)
trap "rm -rf $WORK_DIR" EXIT
cd "$WORK_DIR"

# ---- Step 1: Download clean and compromised versions ----
echo "[1/6] Downloading clean version (@tanstack/router v1.120.3)..."
npm pack @tanstack/react-router@1.120.3 --pack-destination . 2>/dev/null || {
    echo "  NOTE: If the compromised version has been pulled from npm, use git clone instead:"
    echo "  git clone --depth 1 --branch v1.120.3 https://github.com/TanStack/router clean"
    echo "  Attempting git clone fallback..."
    git clone --depth 1 --branch v1.120.3 https://github.com/TanStack/router clean 2>/dev/null || {
        echo "  FALLBACK: Using latest clean tag"
        git clone --depth 1 https://github.com/TanStack/router clean
    }
}

echo "[1/6] Downloading compromised version..."
# The compromised versions may have been unpublished.
# If npm pack fails, provide instructions for manual setup.
npm pack @tanstack/react-router@1.120.4 --pack-destination . 2>/dev/null || {
    echo "  NOTE: Compromised version likely unpublished (expected)."
    echo "  For the demo, clone at the compromised commit:"
    echo "  git clone https://github.com/TanStack/router compromised"
    echo "  git -C compromised checkout 79ac49eedf774dd4b0cfa308722bc463cfe5885c"
    echo ""
    echo "  If the commit has been force-pushed away, the demo can still run"
    echo "  using a locally saved copy of the compromised tarball."
    echo "  Skipping compromised version for now."
}

# Extract tarballs if downloaded
for f in *.tgz; do
    [ -f "$f" ] || continue
    dir=$(basename "$f" .tgz)
    mkdir -p "$dir"
    tar xzf "$f" -C "$dir" --strip-components=1
done

# ---- Step 2: Index both versions ----
echo ""
echo "[2/6] Indexing clean version..."
if [ -d "clean" ]; then
    knowing index clean --db clean.db 2>&1 | tail -3
    CLEAN_SNAPSHOT=$(knowing query --db clean.db --latest-snapshot 2>/dev/null || echo "")
    echo "  Clean snapshot: ${CLEAN_SNAPSHOT:-not available}"
fi

echo ""
echo "[3/6] Indexing compromised version..."
if [ -d "compromised" ] || ls tanstack-react-router-1.120.4* &>/dev/null 2>&1; then
    COMP_DIR=$(ls -d tanstack-react-router-1.120.4* compromised 2>/dev/null | head -1)
    knowing index "$COMP_DIR" --db compromised.db 2>&1 | tail -3
    COMP_SNAPSHOT=$(knowing query --db compromised.db --latest-snapshot 2>/dev/null || echo "")
    echo "  Compromised snapshot: ${COMP_SNAPSHOT:-not available}"
fi

# ---- Step 3: Run supply chain audit ----
echo ""
echo "[4/6] Running supply chain audit..."
if [ -n "${CLEAN_SNAPSHOT:-}" ] && [ -n "${COMP_SNAPSHOT:-}" ]; then
    knowing audit-supply-chain \
        --db compromised.db \
        --base "$CLEAN_SNAPSHOT" \
        --head "$COMP_SNAPSHOT" \
        -o detection.json 2>&1

    echo ""
    echo "=== DETECTION REPORT ==="
    cat detection.json | python3 -m json.tool 2>/dev/null || cat detection.json
else
    echo "  Skipped (missing clean or compromised snapshot)"
fi

# ---- Step 4: Generate proofs ----
echo ""
echo "[5/6] Generating absence proof (clean version)..."
if [ -n "${CLEAN_SNAPSHOT:-}" ]; then
    knowing prove-absent \
        --db clean.db \
        -source "%router_init" \
        -target "%getsession" \
        -type consumes_endpoint 2>&1 || echo "  (No matching symbols, as expected for clean version)"
fi

echo ""
echo "[6/6] Generating presence proof (compromised version)..."
if [ -n "${COMP_SNAPSHOT:-}" ]; then
    knowing prove \
        --db compromised.db \
        -source "%router_init" \
        -target "%getsession" \
        -type consumes_endpoint 2>&1 || echo "  (Proof generation attempted)"
fi

echo ""
echo "=== DEMO COMPLETE ==="
echo "Detection report: detection.json"
echo "Work directory: $WORK_DIR (cleaned up on exit)"
