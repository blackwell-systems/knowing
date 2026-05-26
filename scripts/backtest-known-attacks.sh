#!/usr/bin/env bash
set -euo pipefail

# Backtest known compromised package versions against knowing's supply chain detection.
# Downloads the actual compromised + clean versions from npm/PyPI and runs audit-supply-chain.
# This proves "knowing WOULD have caught this attack at publish time."
#
# SAFETY: Downloads tarballs only. No npm install, no lifecycle hooks, no code execution.

KNOWING="${1:-./knowing}"
OUTPUT_DIR="${2:-backtest-results}"
mkdir -p "$OUTPUT_DIR"

echo "=== Supply Chain Attack Backtest ==="
echo "Testing known compromised package versions against knowing detection."
echo "SAFETY: Tarball download only. No code execution."
echo ""

# Known compromised versions from Mini Shai-Hulud campaign (May 2026)
# and other documented supply chain attacks.
# Format: "registry package compromised_version clean_version description"
KNOWN_ATTACKS=(
  # Mini Shai-Hulud campaign (2026)
  "npm @tanstack/react-router 1.120.4 1.120.3 TanStack CI OIDC token theft"
  "npm @tanstack/react-query 5.75.8 5.75.7 TanStack query (same campaign)"
  # OpenSearch (2026, same campaign)
  "npm @opensearch-project/opensearch 3.5.3 3.5.2 OpenSearch npm compromise"
  # PyPI attacks (2026, same campaign)
  # "pypi mistralai 2.4.6 2.4.5 Mistral AI PyPI compromise"
  # "pypi guardrails-ai 0.10.1 0.10.0 Guardrails AI PyPI compromise"
)

TOTAL=0
DETECTED=0
FAILED=0

for attack in "${KNOWN_ATTACKS[@]}"; do
  read -r registry pkg comp_ver clean_ver desc <<< "$attack"
  TOTAL=$((TOTAL + 1))

  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "[$TOTAL] $desc"
  echo "    Package: $pkg"
  echo "    Clean:   $clean_ver"
  echo "    Compromised: $comp_ver"
  echo "    Registry: $registry"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

  WORK=$(mktemp -d)
  SAFE_NAME=$(echo "${pkg}_${comp_ver}" | tr '/@' '_')

  # Download based on registry
  case "$registry" in
    npm)
      echo "  Downloading $pkg@$clean_ver..."
      command npm pack "${pkg}@${clean_ver}" --pack-destination "$WORK" 2>/dev/null || {
        echo "  SKIP: $clean_ver not available on npm"
        FAILED=$((FAILED + 1))
        rm -rf "$WORK"
        continue
      }

      echo "  Downloading $pkg@$comp_ver..."
      command npm pack "${pkg}@${comp_ver}" --pack-destination "$WORK" 2>/dev/null || {
        echo "  SKIP: $comp_ver not available on npm (likely unpublished)"
        FAILED=$((FAILED + 1))
        rm -rf "$WORK"
        continue
      }

      # Extract
      mkdir -p "$WORK/old" "$WORK/new"
      OLD_TGZ=$(ls "$WORK"/*"${clean_ver}"*.tgz 2>/dev/null | head -1)
      NEW_TGZ=$(ls "$WORK"/*"${comp_ver}"*.tgz 2>/dev/null | head -1)
      [ -z "$OLD_TGZ" ] || [ -z "$NEW_TGZ" ] && { echo "  SKIP: tarball extraction failed"; FAILED=$((FAILED + 1)); rm -rf "$WORK"; continue; }

      tar xzf "$OLD_TGZ" -C "$WORK/old" --strip-components=1
      tar xzf "$NEW_TGZ" -C "$WORK/new" --strip-components=1
      ;;

    pypi)
      echo "  Downloading $pkg==$clean_ver..."
      pip download --no-deps --no-binary :all: "${pkg}==${clean_ver}" -d "$WORK/dl-old" 2>/dev/null || {
        pip download --no-deps "${pkg}==${clean_ver}" -d "$WORK/dl-old" 2>/dev/null || {
          echo "  SKIP: $clean_ver not available on PyPI"
          FAILED=$((FAILED + 1))
          rm -rf "$WORK"
          continue
        }
      }

      echo "  Downloading $pkg==$comp_ver..."
      pip download --no-deps --no-binary :all: "${pkg}==${comp_ver}" -d "$WORK/dl-new" 2>/dev/null || {
        pip download --no-deps "${pkg}==${comp_ver}" -d "$WORK/dl-new" 2>/dev/null || {
          echo "  SKIP: $comp_ver not available on PyPI"
          FAILED=$((FAILED + 1))
          rm -rf "$WORK"
          continue
        }
      }

      mkdir -p "$WORK/old" "$WORK/new"
      for f in "$WORK/dl-old"/*.tar.gz; do [ -f "$f" ] && tar xzf "$f" -C "$WORK/old" --strip-components=1 && break; done
      for f in "$WORK/dl-old"/*.whl; do [ -f "$f" ] && python3 -m zipfile -e "$f" "$WORK/old" && break; done
      for f in "$WORK/dl-new"/*.tar.gz; do [ -f "$f" ] && tar xzf "$f" -C "$WORK/new" --strip-components=1 && break; done
      for f in "$WORK/dl-new"/*.whl; do [ -f "$f" ] && python3 -m zipfile -e "$f" "$WORK/new" && break; done
      ;;
  esac

  # Init git repos
  (cd "$WORK/old" && git init -q && git add . && git commit -q -m "clean $clean_ver" 2>/dev/null) || true
  (cd "$WORK/new" && git init -q && git add . && git commit -q -m "compromised $comp_ver" 2>/dev/null) || true

  # Index both
  echo "  Indexing clean version..."
  "$KNOWING" index -db "$WORK/old.db" -no-enrich "$WORK/old" 2>/dev/null || { echo "  SKIP: index failed"; FAILED=$((FAILED + 1)); rm -rf "$WORK"; continue; }

  echo "  Indexing compromised version..."
  "$KNOWING" index -db "$WORK/new.db" -no-enrich "$WORK/new" 2>/dev/null || { echo "  SKIP: index failed"; FAILED=$((FAILED + 1)); rm -rf "$WORK"; continue; }

  # Get snapshots
  OLD_SNAP=$("$KNOWING" query -db "$WORK/old.db" --latest-snapshot 2>/dev/null || echo "")
  NEW_SNAP=$("$KNOWING" query -db "$WORK/new.db" --latest-snapshot 2>/dev/null || echo "")

  if [ -z "$OLD_SNAP" ] || [ -z "$NEW_SNAP" ]; then
    echo "  SKIP: snapshot extraction failed"
    FAILED=$((FAILED + 1))
    rm -rf "$WORK"
    continue
  fi

  # Run audit
  echo "  Running supply chain audit..."
  "$KNOWING" audit-supply-chain \
    --db "$WORK/new.db" \
    --base "$OLD_SNAP" \
    --head "$NEW_SNAP" \
    -o "$WORK/report.json" 2>/dev/null || true

  # Check results
  if [ -f "$WORK/report.json" ]; then
    cp "$WORK/report.json" "$OUTPUT_DIR/${SAFE_NAME}.json"

    MAX_SCORE=$(python3 -c "
import json
with open('$WORK/report.json') as f:
    d = json.load(f)
print(d.get('summary', {}).get('max_isolation_score', 0))
" 2>/dev/null || echo "0")

    echo "  Isolation score: $MAX_SCORE"

    # Also check for new edge types
    NEW_NODES=$("$KNOWING" query -db "$WORK/new.db" --count-nodes 2>/dev/null || echo "?")
    OLD_NODES=$("$KNOWING" query -db "$WORK/old.db" --count-nodes 2>/dev/null || echo "?")
    NEW_EDGES=$("$KNOWING" query -db "$WORK/new.db" --count-edges 2>/dev/null || echo "?")
    OLD_EDGES=$("$KNOWING" query -db "$WORK/old.db" --count-edges 2>/dev/null || echo "?")

    echo "  Nodes: $OLD_NODES -> $NEW_NODES"
    echo "  Edges: $OLD_EDGES -> $NEW_EDGES"

    # Check for supply chain specific edges
    for etype in reads_env executes_process consumes_endpoint; do
      OLD_COUNT=$(sqlite3 "$WORK/old.db" "SELECT COUNT(*) FROM edges WHERE edge_type='$etype'" 2>/dev/null || echo "0")
      NEW_COUNT=$(sqlite3 "$WORK/new.db" "SELECT COUNT(*) FROM edges WHERE edge_type='$etype'" 2>/dev/null || echo "0")
      if [ "$NEW_COUNT" != "$OLD_COUNT" ]; then
        echo "  ** $etype: $OLD_COUNT -> $NEW_COUNT (CHANGED)"
      fi
    done

    if python3 -c "exit(0 if float('$MAX_SCORE') > 0.3 else 1)" 2>/dev/null; then
      echo "  RESULT: *** DETECTED (isolation score $MAX_SCORE) ***"
      DETECTED=$((DETECTED + 1))
    else
      echo "  RESULT: Not flagged (score below threshold)"
    fi
  else
    echo "  RESULT: Audit report not generated"
    FAILED=$((FAILED + 1))
  fi

  rm -rf "$WORK"
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "BACKTEST SUMMARY"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Total attacks tested: $TOTAL"
echo "  Detected:             $DETECTED"
echo "  Not detected:         $((TOTAL - DETECTED - FAILED))"
echo "  Failed/skipped:       $FAILED"
echo "  Detection rate:       $(python3 -c "print(f'{$DETECTED/$TOTAL*100:.0f}%' if $TOTAL > 0 else 'N/A')" 2>/dev/null)"
echo ""
echo "  Results saved to: $OUTPUT_DIR/"
