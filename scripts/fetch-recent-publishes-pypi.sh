#!/usr/bin/env bash
set -euo pipefail

# Fetches recently updated PyPI packages for supply chain scanning.
# Output: one line per package: "package_name new_version previous_version"
#
# Uses PyPI RSS feed and JSON API.

MAX_PACKAGES=${1:-25}

echo "# Fetching recently updated PyPI packages" >&2
echo "# Maximum packages to scan: ${MAX_PACKAGES}" >&2

# High-value Python packages to monitor
WATCHLIST=(
  "mistralai"
  "guardrails-ai"
  "openai"
  "anthropic"
  "langchain"
  "langchain-core"
  "transformers"
  "torch"
  "tensorflow"
  "numpy"
  "pandas"
  "requests"
  "boto3"
  "django"
  "flask"
  "fastapi"
  "uvicorn"
  "celery"
  "sqlalchemy"
  "pydantic"
  "cryptography"
  "paramiko"
  "fabric"
  "ansible"
  "kubernetes"
)

count=0
for pkg in "${WATCHLIST[@]}"; do
  if [ $count -ge $MAX_PACKAGES ]; then
    break
  fi

  # Get latest two versions from PyPI JSON API
  versions=$(curl -s "https://pypi.org/pypi/${pkg}/json" 2>/dev/null | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    releases = sorted(d.get('releases', {}).keys(), key=lambda v: d['releases'][v][0]['upload_time'] if d['releases'][v] else '0', reverse=True)
    # Filter to versions that have files
    releases = [v for v in releases if d['releases'][v]]
    if len(releases) >= 2:
        print(f'{releases[0]} {releases[1]}')
except:
    pass
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

echo "# Found $count PyPI packages to scan" >&2
