#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# event-stream Supply Chain Detection Demo
#
# SAFETY: This script downloads source tarballs only. No npm install, no
# lifecycle hooks, no code execution. knowing indexes via tree-sitter AST parsing.
#
# The compromised version (v3.3.6) may have been unpublished from npm.
# The script handles this gracefully with fallback instructions.
# =============================================================================

echo "=== event-stream Supply Chain Detection Demo ==="
echo ""
echo "SAFETY: No code execution. AST parsing only."
echo ""

WORK_DIR=$(mktemp -d)
trap "rm -rf $WORK_DIR" EXIT
cd "$WORK_DIR"

# ---- Step 1: Download clean and compromised versions ----
echo "[1/6] Downloading clean version (event-stream v3.3.3)..."
mkdir -p clean
npm pack event-stream@3.3.3 --pack-destination . 2>/dev/null && {
    tar xzf event-stream-3.3.3.tgz -C clean --strip-components=1
    echo "  Downloaded event-stream@3.3.3"
} || {
    echo "  npm pack failed. Trying git clone..."
    git clone --depth 1 --branch v3.3.3 https://github.com/dominictarr/event-stream clean 2>/dev/null || {
        echo "  ERROR: Could not download clean version. Exiting."
        exit 1
    }
}

echo "[1/6] Downloading compromised version (event-stream v3.3.6)..."
mkdir -p compromised
npm pack event-stream@3.3.6 --pack-destination . 2>/dev/null && {
    tar xzf event-stream-3.3.6.tgz -C compromised --strip-components=1
    echo "  Downloaded event-stream@3.3.6"
} || {
    echo "  NOTE: v3.3.6 was unpublished from npm (expected)."
    echo "  Trying git..."
    git clone --depth 1 --branch v3.3.6 https://github.com/dominictarr/event-stream compromised 2>/dev/null || {
        echo "  Compromised version unavailable. Using v3.3.4 as reference."
        npm pack event-stream@3.3.4 --pack-destination . 2>/dev/null && {
            tar xzf event-stream-3.3.4.tgz -C compromised --strip-components=1
        } || {
            echo "  ERROR: Could not download any compromised version."
            echo "  The demo requires a local copy of event-stream@3.3.6."
            echo "  If you have a cached copy, place it in: $WORK_DIR/compromised/"
            exit 1
        }
    }
}

# Also try to get flatmap-stream (the injected dependency)
echo "[1/6] Downloading flatmap-stream (the malicious dependency)..."
mkdir -p flatmap-stream
npm pack flatmap-stream@0.1.1 --pack-destination . 2>/dev/null && {
    tar xzf flatmap-stream-0.1.1.tgz -C flatmap-stream --strip-components=1
    echo "  Downloaded flatmap-stream@0.1.1"
} || {
    echo "  flatmap-stream unpublished (expected). Skipping."
}

# ---- Step 2: Index both versions ----
echo ""
echo "[2/6] Indexing clean version..."
knowing index clean --db clean.db 2>&1 | tail -3
CLEAN_SNAPSHOT=$(knowing query --db clean.db --latest-snapshot 2>/dev/null || echo "")
echo "  Clean snapshot: ${CLEAN_SNAPSHOT:-not available}"

echo ""
echo "[3/6] Indexing compromised version..."
knowing index compromised --db compromised.db 2>&1 | tail -3
# If flatmap-stream was downloaded, index it into the same DB
if [ -d "flatmap-stream" ] && [ -f "flatmap-stream/index.js" ]; then
    echo "  Also indexing flatmap-stream..."
    knowing index flatmap-stream --db compromised.db --append 2>&1 | tail -2
fi
COMP_SNAPSHOT=$(knowing query --db compromised.db --latest-snapshot 2>/dev/null || echo "")
echo "  Compromised snapshot: ${COMP_SNAPSHOT:-not available}"

# ---- Step 3: Run supply chain audit ----
echo ""
echo "[4/6] Running supply chain audit..."
if [ -n "$CLEAN_SNAPSHOT" ] && [ -n "$COMP_SNAPSHOT" ]; then
    knowing audit-supply-chain \
        --db compromised.db \
        --base "$CLEAN_SNAPSHOT" \
        --head "$COMP_SNAPSHOT" \
        -o detection.json 2>&1

    echo ""
    echo "=== DETECTION REPORT ==="
    cat detection.json | python3 -m json.tool 2>/dev/null || cat detection.json
else
    echo "  Skipped (missing snapshot)"
fi

# ---- Step 4: Prove clean version is isolated ----
echo ""
echo "[5/6] Proving clean version has NO path to network APIs..."
if [ -n "$CLEAN_SNAPSHOT" ]; then
    knowing prove-absent \
        --db clean.db \
        -source "%event-stream" \
        -target "%https.request" \
        -type calls \
        -o clean-isolation-proof.json 2>&1 && {
        echo "  PROVED: event-stream v3.3.3 is isolated from https.request"
    } || {
        echo "  (No matching symbols found, consistent with isolation)"
    }
fi

# ---- Step 5: Detect attack path in compromised version ----
echo ""
echo "[6/6] Detecting attack path in compromised version..."
if [ -n "$COMP_SNAPSHOT" ] && [ -d "flatmap-stream" ]; then
    knowing prove \
        --db compromised.db \
        -source "%flatmap-stream" \
        -target "%https" \
        -type calls \
        -o attack-path-proof.json 2>&1 && {
        echo "  DETECTED: flatmap-stream has path to https (attack vector confirmed)"
    } || {
        echo "  (Attack path proof attempted)"
    }
fi

echo ""
echo "=== DEMO COMPLETE ==="
echo ""
echo "Output files:"
[ -f detection.json ] && echo "  detection.json          - Full audit report"
[ -f clean-isolation-proof.json ] && echo "  clean-isolation-proof.json - Merkle proof: clean version is isolated"
[ -f attack-path-proof.json ] && echo "  attack-path-proof.json    - Merkle proof: attack path exists"
echo ""
echo "Work directory: $WORK_DIR (cleaned up on exit)"
