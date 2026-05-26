#!/usr/bin/env bash
# false-positive-eval.sh: Scan 200+ known-clean packages to measure FP rate.
# Each package is downloaded, indexed, scanned with audit-supply-chain --scan-all,
# and the result is recorded as a JSON line in the output file.
#
# Usage: ./scripts/false-positive-eval.sh [output.jsonl]
#
# Requirements: knowing binary on PATH, npm, pip (for downloads), git

set -euo pipefail

OUTPUT="${1:-bench/supply-chain/false-positive-results.jsonl}"
WORKDIR=$(mktemp -d)
THRESHOLD=0.3

mkdir -p "$(dirname "$OUTPUT")"
> "$OUTPUT"  # truncate

echo "=== Supply Chain False Positive Evaluation ==="
echo "Working directory: $WORKDIR"
echo "Output: $OUTPUT"
echo "Threshold: $THRESHOLD"
echo ""

# Popular, well-maintained npm packages (clean, no supply chain payload).
NPM_PACKAGES=(
  lodash
  express
  axios
  chalk
  debug
  commander
  yargs
  minimist
  semver
  uuid
  dotenv
  cors
  body-parser
  cookie-parser
  morgan
  compression
  helmet
  passport
  jsonwebtoken
  bcryptjs
  mongoose
  sequelize
  knex
  pg
  mysql2
  redis
  ioredis
  bullmq
  ws
  socket.io
  fastify
  koa
  hapi
  restify
  next
  react
  react-dom
  vue
  svelte
  preact
  lit
  storybook
  jest
  mocha
  chai
  sinon
  nyc
  eslint
  prettier
  typescript
  webpack
  rollup
  esbuild
  vite
  parcel
  babel-core
  postcss
  tailwindcss
  sass
  less
  pino
  winston
  bunyan
  dayjs
  date-fns
  luxon
  ramda
  rxjs
  zod
  joi
  ajv
  yup
  superstruct
  marked
  highlight.js
  prismjs
  cheerio
  puppeteer-core
  playwright-core
  sharp
  jimp
  multer
  formidable
  busboy
  mime-types
  content-type
  accepts
  negotiator
  fresh
  etag
  on-finished
  destroy
  depd
  statuses
  http-errors
  raw-body
  type-is
  content-disposition
  vary
  proxy-addr
  forwarded
  ipaddr.js
)

# Popular PyPI packages (clean).
PYPI_PACKAGES=(
  requests
  flask
  django
  fastapi
  uvicorn
  gunicorn
  celery
  redis
  sqlalchemy
  alembic
  pydantic
  marshmallow
  click
  typer
  rich
  httpx
  aiohttp
  boto3
  paramiko
  cryptography
  PyJWT
  python-dotenv
  Pillow
  numpy
  pandas
  scipy
  matplotlib
  pytest
  coverage
  black
  ruff
  mypy
  isort
  bandit
  tox
  nox
  pre-commit
  setuptools
  wheel
  twine
  build
  flit
  poetry-core
  jinja2
  mako
  pyyaml
  toml
  tomli
  orjson
  msgpack
  protobuf
  grpcio
  wrapt
  decorator
  attrs
  cattrs
  dataclasses-json
  sentry-sdk
  structlog
  loguru
  tenacity
  backoff
  cachetools
  diskcache
  watchdog
  psutil
  humanize
  tabulate
  tqdm
  colorama
  pygments
  docutils
  sphinx
  mkdocs
  certifi
  urllib3
  charset-normalizer
  idna
  packaging
  importlib-metadata
  typing-extensions
  six
  distlib
  filelock
  platformdirs
  appdirs
  pathspec
  pluggy
  iniconfig
  py
  more-itertools
  toolz
  funcy
  boltons
  arrow
  pendulum
  python-dateutil
  pytz
)

total=0
flagged=0
errors=0

scan_package() {
  local name="$1"
  local ecosystem="$2"
  local pkg_dir="$WORKDIR/$ecosystem/$name"

  mkdir -p "$pkg_dir"

  # Download
  if [ "$ecosystem" = "npm" ]; then
    (cd "$pkg_dir" && npm pack "$name" --pack-destination . 2>/dev/null && \
     tar xzf *.tgz --strip-components=1 2>/dev/null) || { echo "  SKIP $ecosystem/$name (download failed)"; return 1; }
  elif [ "$ecosystem" = "pypi" ]; then
    pip download --no-deps --no-binary :all: "$name" -d "$pkg_dir" 2>/dev/null || \
    pip download --no-deps "$name" -d "$pkg_dir" 2>/dev/null || { echo "  SKIP $ecosystem/$name (download failed)"; return 1; }
    # Extract the archive
    (cd "$pkg_dir" && for f in *.tar.gz *.zip; do
      [ -f "$f" ] && (tar xzf "$f" 2>/dev/null || python3 -m zipfile -e "$f" . 2>/dev/null) && break
    done) || true
  fi

  # Find the source directory (may be nested)
  local src_dir
  src_dir=$(find "$pkg_dir" -name "*.js" -o -name "*.ts" -o -name "*.py" | head -1 | xargs dirname 2>/dev/null || echo "$pkg_dir")
  [ -z "$src_dir" ] && src_dir="$pkg_dir"

  # Initialize a git repo (knowing needs it)
  (cd "$src_dir" && git init -q && git add -A && git commit -q -m "init" --allow-empty) 2>/dev/null || true

  # Index (use KNOWING env var for custom binary path, skip LSP enrichment)
  local db="$pkg_dir/knowing.db"
  local KNOWING="${KNOWING:-knowing}"
  "$KNOWING" index -url "$ecosystem/$name" -db "$db" "$src_dir" 2>/dev/null || { echo "  SKIP $ecosystem/$name (index failed)"; return 1; }

  # Scan
  local result
  result=$("$KNOWING" audit-supply-chain -db "$db" -base @first -scan-all -threshold "$THRESHOLD" 2>/dev/null) || { echo "  SKIP $ecosystem/$name (scan failed)"; return 1; }

  local files_analyzed files_suspicious env_reads process_execs
  files_analyzed=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['summary']['files_analyzed'])" 2>/dev/null || echo 0)
  files_suspicious=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['summary']['files_suspicious'])" 2>/dev/null || echo 0)
  env_reads=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['summary']['env_reads_total'])" 2>/dev/null || echo 0)
  process_execs=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['summary']['process_exec_total'])" 2>/dev/null || echo 0)

  # Record result
  echo "{\"package\":\"$name\",\"ecosystem\":\"$ecosystem\",\"files_analyzed\":$files_analyzed,\"files_suspicious\":$files_suspicious,\"env_reads\":$env_reads,\"process_execs\":$process_execs,\"threshold\":$THRESHOLD}" >> "$OUTPUT"

  if [ "$files_suspicious" -gt 0 ]; then
    echo "  FLAG $ecosystem/$name: $files_suspicious suspicious ($files_analyzed analyzed, $env_reads env, $process_execs proc)"
    return 2  # flagged but not error
  else
    echo "  PASS $ecosystem/$name: $files_analyzed files, 0 suspicious"
    return 0
  fi
}

echo "--- npm packages (${#NPM_PACKAGES[@]}) ---"
for pkg in "${NPM_PACKAGES[@]}"; do
  total=$((total + 1))
  rc=0
  scan_package "$pkg" "npm" || rc=$?
  if [ "$rc" -eq 2 ]; then
    flagged=$((flagged + 1))
  elif [ "$rc" -ne 0 ]; then
    errors=$((errors + 1))
    total=$((total - 1))  # don't count failed downloads
  fi
done

echo ""
echo "--- PyPI packages (${#PYPI_PACKAGES[@]}) ---"
for pkg in "${PYPI_PACKAGES[@]}"; do
  total=$((total + 1))
  rc=0
  scan_package "$pkg" "pypi" || rc=$?
  if [ "$rc" -eq 2 ]; then
    flagged=$((flagged + 1))
  elif [ "$rc" -ne 0 ]; then
    errors=$((errors + 1))
    total=$((total - 1))  # don't count failed downloads
  fi
done

echo ""
echo "=== Results ==="
echo "Total scanned: $total"
echo "Flagged (FP):  $flagged"
echo "Errors/skips:  $errors"
if [ "$total" -gt 0 ]; then
  fp_rate=$(python3 -c "print(f'{$flagged/$total*100:.1f}%')" 2>/dev/null || echo "?")
  echo "FP rate:       $fp_rate"
fi
echo "Output:        $OUTPUT"

# Cleanup
rm -rf "$WORKDIR"
