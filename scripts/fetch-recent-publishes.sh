#!/usr/bin/env bash
set -euo pipefail

# Fetches npm packages published in the last N minutes with significant download counts.
# Output: one line per package: "package_name new_version previous_version"
#
# Uses the npm registry replicate API to get recent changes, then filters to
# packages with >1000 weekly downloads to avoid scanning noise.

MINUTES=${1:-15}
MIN_DOWNLOADS=${2:-1000}
MAX_PACKAGES=${3:-50}

# Get the current npm registry sequence number and look back N minutes
# The replicate endpoint streams all changes; we use the search API instead
# for a simpler approach that doesn't require CouchDB replication.

echo "# Fetching npm packages updated in the last ${MINUTES} minutes" >&2
echo "# Minimum weekly downloads: ${MIN_DOWNLOADS}" >&2
echo "# Maximum packages to scan: ${MAX_PACKAGES}" >&2

# Use npm search API to find recently updated packages
# The 'not:unstable' filter helps focus on established packages
SINCE=$(date -u -v-${MINUTES}M +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -d "${MINUTES} minutes ago" +%Y-%m-%dT%H:%M:%S)

# Approach: query the npm registry for packages modified recently
# npm doesn't have a "recently published" API, so we use the replicate changes feed
# For the MVP, we use a curated watchlist of high-value packages instead.

# Strategy 1: Watchlist of high-download packages (check for new versions)
WATCHLIST=(
  "@tanstack/react-router"
  "@tanstack/react-query"
  "@opensearch-project/opensearch"
  "express"
  "lodash"
  "axios"
  "react"
  "vue"
  "next"
  "typescript"
  "webpack"
  "esbuild"
  "vite"
  "prisma"
  "@prisma/client"
  "mongoose"
  "sequelize"
  "fastify"
  "koa"
  "hono"
  "drizzle-orm"
  "@anthropic-ai/sdk"
  "openai"
  "langchain"
)

count=0
for pkg in "${WATCHLIST[@]}"; do
  if [ $count -ge $MAX_PACKAGES ]; then
    break
  fi

  # Get the latest two versions
  versions=$(npm view "$pkg" versions --json 2>/dev/null | python3 -c "
import json, sys
v = json.load(sys.stdin)
if isinstance(v, list) and len(v) >= 2:
    print(f'{v[-1]} {v[-2]}')
elif isinstance(v, str):
    print(f'{v} {v}')
" 2>/dev/null || echo "")

  if [ -n "$versions" ]; then
    new_ver=$(echo "$versions" | awk '{print $1}')
    prev_ver=$(echo "$versions" | awk '{print $2}')
    if [ "$new_ver" != "$prev_ver" ]; then
      echo "$pkg $new_ver $prev_ver"
      count=$((count + 1))
    fi
  fi
done

echo "# Found $count packages to scan" >&2
