#!/usr/bin/env bash
# Publishes npm packages for a given release tag.
# Usage: npm-publish.sh v0.1.0
set -euo pipefail

VERSION="${1#v}"  # Strip leading 'v'
REPO="blackwell-systems/knowing"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

# Map npm platform package names to goreleaser archive names
declare -A PLATFORMS=(
  ["knowing-darwin-arm64"]="knowing_darwin_arm64.tar.gz"
  ["knowing-darwin-x64"]="knowing_darwin_amd64.tar.gz"
  ["knowing-linux-arm64"]="knowing_linux_arm64.tar.gz"
  ["knowing-linux-x64"]="knowing_linux_amd64.tar.gz"
)

cd "$(dirname "$0")/../npm"

# Publish each platform package
for pkg in "${!PLATFORMS[@]}"; do
  archive="${PLATFORMS[$pkg]}"
  echo "==> Publishing @blackwell-systems/${pkg}@${VERSION}"

  cd "$pkg"

  # Update version in package.json
  node -e "
    const fs = require('fs');
    const p = JSON.parse(fs.readFileSync('package.json'));
    p.version = '${VERSION}';
    fs.writeFileSync('package.json', JSON.stringify(p, null, 2) + '\n');
  "

  # Download and extract binary
  mkdir -p bin
  curl -fsSL "${BASE_URL}/${archive}" | tar xz -C bin knowing
  chmod +x bin/knowing

  npm publish --access public
  rm -rf bin
  cd ..
done

# Publish root package
echo "==> Publishing @blackwell-systems/knowing@${VERSION}"
cd knowing

node -e "
  const fs = require('fs');
  const p = JSON.parse(fs.readFileSync('package.json'));
  p.version = '${VERSION}';
  for (const dep of Object.keys(p.optionalDependencies || {})) {
    p.optionalDependencies[dep] = '${VERSION}';
  }
  fs.writeFileSync('package.json', JSON.stringify(p, null, 2) + '\n');
"

npm publish --access public
echo "==> Done. Published @blackwell-systems/knowing@${VERSION}"
