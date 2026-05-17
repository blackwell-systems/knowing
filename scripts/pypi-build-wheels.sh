#!/usr/bin/env bash
# Builds platform-specific wheels for PyPI from GoReleaser binaries.
# Usage: pypi-build-wheels.sh v0.1.0
set -euo pipefail

VERSION="${1#v}"  # Strip leading 'v'
REPO="blackwell-systems/knowing"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PYPI_DIR="${SCRIPT_DIR}/../pypi"

cd "$PYPI_DIR"

# Update version in pyproject.toml
sed -i.bak "s/^version = \".*\"/version = \"${VERSION}\"/" pyproject.toml && rm -f pyproject.toml.bak

# Map platform tags to goreleaser archives
declare -A PLATFORMS=(
  ["macosx_11_0_arm64"]="knowing_darwin_arm64.tar.gz"
  ["macosx_10_12_x86_64"]="knowing_darwin_amd64.tar.gz"
  ["manylinux2014_aarch64"]="knowing_linux_arm64.tar.gz"
  ["manylinux2014_x86_64"]="knowing_linux_amd64.tar.gz"
)

mkdir -p dist

for plat_tag in "${!PLATFORMS[@]}"; do
  archive="${PLATFORMS[$plat_tag]}"
  echo "==> Building wheel for ${plat_tag}"

  # Download and extract binary
  mkdir -p knowing/bin
  curl -fsSL "${BASE_URL}/${archive}" | tar xz -C knowing/bin knowing
  chmod +x knowing/bin/knowing

  # Create __main__.py wrapper if not exists
  mkdir -p knowing
  cat > knowing/__init__.py << 'PYEOF'
"""knowing: content-addressed graph artifact for software systems."""
PYEOF
  cat > knowing/__main__.py << 'PYEOF'
"""Entry point for knowing CLI."""
import os
import sys
import subprocess

def main():
    binary = os.path.join(os.path.dirname(__file__), "bin", "knowing")
    if not os.path.exists(binary):
        print("Error: knowing binary not found", file=sys.stderr)
        sys.exit(1)
    result = subprocess.run([binary] + sys.argv[1:])
    sys.exit(result.returncode)

if __name__ == "__main__":
    main()
PYEOF

  # Build wheel using pip wheel (creates proper .dist-info)
  pip wheel . --no-deps --wheel-dir tmp_wheel/

  # Rename the wheel to the correct platform tag
  for whl in tmp_wheel/knowing-*.whl; do
    # Replace 'any' or whatever platform tag with the correct one
    target="dist/knowing-${VERSION}-py3-none-${plat_tag}.whl"
    # Repack with correct tag using wheel tags
    python -m wheel tags --platform-tag "${plat_tag}" --remove "$whl"
    mv tmp_wheel/knowing-*.whl "$target" 2>/dev/null || mv "$whl" "$target"
    echo "    -> $(basename "$target")"
  done

  rm -rf knowing/bin tmp_wheel/
done

echo "==> Built wheels:"
ls dist/*.whl
