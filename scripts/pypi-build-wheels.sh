#!/usr/bin/env bash
# Builds platform-specific wheels for PyPI from GoReleaser binaries.
# Usage: pypi-build-wheels.sh v0.1.0
set -euo pipefail

VERSION="${1#v}"  # Strip leading 'v'
REPO="blackwell-systems/knowing"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

cd "$(dirname "$0")/../pypi"

# Map platform tags to goreleaser archives
declare -A PLATFORMS=(
  ["macosx_11_0_arm64"]="knowing_darwin_arm64.tar.gz"
  ["macosx_10_12_x86_64"]="knowing_darwin_amd64.tar.gz"
  ["manylinux2014_aarch64"]="knowing_linux_arm64.tar.gz"
  ["manylinux2014_x86_64"]="knowing_linux_amd64.tar.gz"
)

# Update version in pyproject.toml
sed -i "s/^version = \".*\"/version = \"${VERSION}\"/" pyproject.toml

mkdir -p dist

for plat_tag in "${!PLATFORMS[@]}"; do
  archive="${PLATFORMS[$plat_tag]}"
  echo "==> Building wheel for ${plat_tag}"

  # Download and extract binary
  mkdir -p knowing/bin
  curl -fsSL "${BASE_URL}/${archive}" | tar xz -C knowing/bin knowing
  chmod +x knowing/bin/knowing

  # Build wheel
  python -m wheel pack . \
    --dest-dir dist \
    --build-number 0

  # Rename wheel with correct platform tag
  # wheel pack produces: knowing-VERSION-py3-none-any.whl
  # We need: knowing-VERSION-py3-none-PLATFORM.whl
  for whl in dist/knowing-${VERSION}-*.whl; do
    if [[ "$whl" == *"any.whl" ]]; then
      target="dist/knowing-${VERSION}-py3-none-${plat_tag}.whl"
      mv "$whl" "$target"
      echo "    -> $(basename "$target")"
    fi
  done

  rm -rf knowing/bin
done

echo "==> Built $(ls dist/*.whl | wc -l) wheels"
ls dist/*.whl
