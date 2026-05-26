#!/usr/bin/env bash
# Extracts max_isolation_score from an audit-supply-chain report JSON.
# Usage: check-isolation-score.sh report.json [threshold]
# Exit 0 if score > threshold, exit 1 otherwise.

REPORT="${1:-}"
THRESHOLD="${2:-0.7}"

if [ ! -f "$REPORT" ]; then
  echo "0"
  exit 1
fi

SCORE=$(python3 -c "
import json, sys
with open(sys.argv[1]) as f:
    d = json.load(f)
print(d.get('summary', {}).get('max_isolation_score', 0))
" "$REPORT" 2>/dev/null || echo "0")

echo "$SCORE"

python3 -c "exit(0 if float('$SCORE') > float('$THRESHOLD') else 1)" 2>/dev/null
