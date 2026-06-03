#!/usr/bin/env bash
set -euo pipefail

# Scan 100 held-out packages (50 npm, 50 pypi) for false positive evaluation.
# These packages are NOT in the original 200-package corpus, providing independent
# validation of the 1.0% FP rate claim.
#
# Usage: GOWORK=off go build -o knowing ./cmd/knowing/ && bash bench/supply-chain/scan-held-out.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT="$SCRIPT_DIR/false-positive-held-out.jsonl"
KNOWING="/tmp/knowing-scan-binary"

echo "Building knowing..."
(cd "$SCRIPT_DIR/../.." && GOWORK=off go build -o "$KNOWING" ./cmd/knowing/)
echo "Built: $KNOWING"

NPM_PACKAGES=(
  zod trpc drizzle-orm hono elysia
  effect neverthrow ts-pattern radash defu
  ofetch ky got node-fetch undici
  pino-pretty consola signale loglevel log4js
  date-fns dayjs luxon ms milliparsec
  zx execa shelljs cross-spawn npm-run-all2
  vitest ava tap uvu c8
  prettier biome oxlint dprint stylelint
  prisma kysely better-sqlite3 sql.js typeorm
  superjson devalue serialize-javascript flatted destr
)

PYPI_PACKAGES=(
  polars duckdb pyarrow sqlmodel peewee
  ruff mypy pyright pylint flake8
  typer textual prompt-toolkit questionary inquirerpy
  loguru structlog rich tqdm alive-progress
  pydantic-settings python-dotenv dynaconf confz hydra-core
  orjson msgpack cbor2 protobuf avro-python3
  tox invoke doit taskipy hatch
  trio anyio uvloop asyncpg aiosqlite
  moto responses vcrpy trustme time-machine
  jinja2 mako chevron pystache markupsafe
)

> "$OUTPUT"
echo "Scanning 50 npm + 50 pypi held-out packages..."
echo "Output: $OUTPUT"
echo ""

scan_package() {
  local ecosystem="$1" pkg="$2"
  WORK=$(mktemp -d)

  local download_ok=false
  if [ "$ecosystem" = "npm" ]; then
    if (cd "$WORK" && npm pack "$pkg" --pack-destination . >/dev/null 2>&1); then
      mkdir -p "$WORK/src"
      for f in "$WORK"/*.tgz; do
        [ -f "$f" ] && tar xzf "$f" -C "$WORK/src" --strip-components=1 2>/dev/null && break
      done
      download_ok=true
    fi
  elif [ "$ecosystem" = "pypi" ]; then
    if pip3 download "$pkg" --no-deps --no-binary :all: -d "$WORK/dl" >/dev/null 2>&1 || \
       pip3 download "$pkg" --no-deps -d "$WORK/dl" >/dev/null 2>&1; then
      mkdir -p "$WORK/src"
      for f in "$WORK/dl"/*.tar.gz; do
        [ -f "$f" ] && tar xzf "$f" -C "$WORK/src" --strip-components=1 2>/dev/null && break
      done
      for f in "$WORK/dl"/*.whl; do
        [ -f "$f" ] && python3 -m zipfile -e "$f" "$WORK/src" 2>/dev/null && break
      done
      download_ok=true
    fi
  fi

  if [ "$download_ok" = false ]; then
    echo '{"package":"'"$pkg"'","ecosystem":"'"$ecosystem"'","files_analyzed":0,"files_suspicious":0,"env_reads":0,"process_execs":0,"threshold":0.3,"error":"download_failed"}' >> "$OUTPUT"
    echo "download failed"
    rm -rf "$WORK"
    return
  fi

  (cd "$WORK/src" && git init -q && git add -A && git commit -q -m "init" 2>/dev/null) || true

  if ! "$KNOWING" index -url "$ecosystem/$pkg" -db "$WORK/scan.db" -no-enrich "$WORK/src" >/dev/null 2>&1; then
    echo '{"package":"'"$pkg"'","ecosystem":"'"$ecosystem"'","files_analyzed":0,"files_suspicious":0,"env_reads":0,"process_execs":0,"threshold":0.3,"error":"index_failed"}' >> "$OUTPUT"
    echo "index failed"
    rm -rf "$WORK"
    return
  fi

  SNAP=$(sqlite3 "$WORK/scan.db" "SELECT hex(snapshot_hash) FROM snapshots LIMIT 1" 2>/dev/null || echo "")
  if [ -z "$SNAP" ]; then
    echo '{"package":"'"$pkg"'","ecosystem":"'"$ecosystem"'","files_analyzed":0,"files_suspicious":0,"env_reads":0,"process_execs":0,"threshold":0.3,"error":"no_snapshot"}' >> "$OUTPUT"
    echo "no snapshot"
    rm -rf "$WORK"
    return
  fi

  "$KNOWING" audit-supply-chain -db "$WORK/scan.db" -base "$SNAP" -head "$SNAP" -scan-all -o "$WORK/report.json" >/dev/null 2>&1 || true

  if [ -f "$WORK/report.json" ]; then
    python3 -c "
import json
try:
    r = json.load(open('$WORK/report.json'))
    s = r.get('summary', {})
    print(json.dumps({
        'package': '$pkg',
        'ecosystem': '$ecosystem',
        'files_analyzed': s.get('files_analyzed', 0),
        'files_suspicious': s.get('files_suspicious', 0),
        'env_reads': s.get('total_env_reads', 0),
        'process_execs': s.get('total_process_execs', 0),
        'verdict': s.get('verdict', 'unknown'),
        'threshold': 0.3
    }))
except Exception as e:
    print(json.dumps({'package': '$pkg', 'ecosystem': '$ecosystem', 'files_analyzed': 0, 'files_suspicious': 0, 'env_reads': 0, 'process_execs': 0, 'threshold': 0.3, 'error': str(e)}))
" >> "$OUTPUT"
    echo "ok"
  else
    echo '{"package":"'"$pkg"'","ecosystem":"'"$ecosystem"'","files_analyzed":0,"files_suspicious":0,"env_reads":0,"process_execs":0,"threshold":0.3,"error":"no_report"}' >> "$OUTPUT"
    echo "no report"
  fi

  rm -rf "$WORK"
}

for pkg in "${NPM_PACKAGES[@]}"; do
  echo -n "npm/$pkg... "
  scan_package npm "$pkg"
done

for pkg in "${PYPI_PACKAGES[@]}"; do
  echo -n "pypi/$pkg... "
  scan_package pypi "$pkg"
done

echo ""
echo "=== Results ==="
TOTAL=$(wc -l < "$OUTPUT" | tr -d ' ')
SUSPICIOUS=$(python3 -c "
import json
with open('$OUTPUT') as f:
    print(sum(1 for line in f if json.loads(line).get('files_suspicious', 0) > 0))
")
ERRORS=$(python3 -c "
import json
with open('$OUTPUT') as f:
    print(sum(1 for line in f if 'error' in json.loads(line)))
")
echo "Total: $TOTAL packages"
echo "With suspicious files: $SUSPICIOUS"
echo "Errors: $ERRORS"
echo "Output: $OUTPUT"
