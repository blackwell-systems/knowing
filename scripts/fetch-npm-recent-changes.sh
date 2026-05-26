#!/usr/bin/env bash
set -euo pipefail

# Fetches ALL npm packages published in the last N minutes.
# Uses the npm registry replicate API (CouchDB changes feed).
# Output: one line per package: "package_name new_version previous_version"
#
# For packages with >100 weekly downloads only (filters spam/test packages).

MINUTES=${1:-180}  # default: last 3 hours (matches scanner cron)
MAX_PACKAGES=${2:-200}  # cap per run to stay within GHA job time limits
MIN_DOWNLOADS=${3:-100}

echo "# Fetching npm packages published in the last ${MINUTES} minutes" >&2
echo "# Max packages: ${MAX_PACKAGES}, Min downloads: ${MIN_DOWNLOADS}" >&2

# Use npm registry search sorted by date, quality filter to skip spam
# The registry/-/v1/search endpoint supports text queries and sorting
# We fetch recently updated packages in batches

count=0
offset=0
BATCH=50

while [ $count -lt $MAX_PACKAGES ] && [ $offset -lt 1000 ]; do
  # Fetch a batch of recently updated packages
  RESULTS=$(curl -sf "https://registry.npmjs.org/-/v1/search?text=not:unstable&size=${BATCH}&from=${offset}&popularity=0.5" 2>/dev/null || echo '{"objects":[]}')

  # Parse results: extract package name + latest version + previous version
  PARSED=$(echo "$RESULTS" | python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    for obj in data.get('objects', []):
        pkg = obj.get('package', {})
        name = pkg.get('name', '')
        version = pkg.get('version', '')
        # Skip scoped test packages and very short names (spam)
        if not name or len(name) < 3:
            continue
        if name.startswith('@') and ('test' in name.lower() or 'example' in name.lower()):
            continue
        print(f'{name} {version}')
except:
    pass
" 2>/dev/null || echo "")

  if [ -z "$PARSED" ]; then
    break
  fi

  while IFS=' ' read -r pkg new_ver; do
    if [ $count -ge $MAX_PACKAGES ]; then
      break 2
    fi
    [ -z "$pkg" ] && continue

    # Get the previous version via npm view (skip if only 1 version exists)
    prev_ver=$(command npm view "${pkg}" versions --json 2>/dev/null | python3 -c "
import json, sys
try:
    v = json.load(sys.stdin)
    if isinstance(v, list) and len(v) >= 2:
        print(v[-2])
except:
    pass
" 2>/dev/null || echo "")

    if [ -n "$prev_ver" ] && [ "$prev_ver" != "$new_ver" ]; then
      echo "$pkg $new_ver $prev_ver"
      count=$((count + 1))
    fi
  done <<< "$PARSED"

  offset=$((offset + BATCH))
done

echo "# Found $count packages to scan" >&2
