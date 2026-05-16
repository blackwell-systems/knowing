#!/usr/bin/env bash
set -euo pipefail

# Install script for knowing
# Downloads the latest release binary from GitHub and installs it.

REPO="blackwell-systems/knowing"
BINARY="knowing"

# --- Detect OS and architecture ---

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  darwin) OS="darwin" ;;
  linux)  OS="linux" ;;
  *)
    echo "Error: unsupported operating system: $OS" >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

echo "Detected platform: ${OS}/${ARCH}"

# --- Resolve latest release tag ---

API_URL="https://api.github.com/repos/${REPO}/releases/latest"
echo "Fetching latest release from ${API_URL}..."

TAG=$(curl -fsSL "$API_URL" 2>/dev/null | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')

if [ -z "$TAG" ]; then
  echo "Error: no releases found for ${REPO}." >&2
  echo "This is expected if no release has been published yet." >&2
  exit 1
fi

echo "Latest release: ${TAG}"

# --- Download binary ---

VERSION="${TAG#v}"
ASSET="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${DOWNLOAD_URL}..."
if ! curl -fsSL -o "${TMPDIR}/${ASSET}" "$DOWNLOAD_URL"; then
  echo "Error: failed to download ${ASSET}" >&2
  echo "Available assets may use a different naming convention." >&2
  echo "Check: https://github.com/${REPO}/releases/tag/${TAG}" >&2
  exit 1
fi

# --- Extract ---

echo "Extracting..."
tar -xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR"

if [ ! -f "${TMPDIR}/${BINARY}" ]; then
  # Try finding the binary in a subdirectory
  FOUND=$(find "$TMPDIR" -name "$BINARY" -type f | head -1)
  if [ -z "$FOUND" ]; then
    echo "Error: binary '${BINARY}' not found in archive" >&2
    exit 1
  fi
  mv "$FOUND" "${TMPDIR}/${BINARY}"
fi

chmod +x "${TMPDIR}/${BINARY}"

# --- Install ---

INSTALL_DIR="/usr/local/bin"

if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
  echo "No write access to /usr/local/bin, installing to ${INSTALL_DIR}"
fi

mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

echo ""
echo "Successfully installed ${BINARY} ${TAG} to ${INSTALL_DIR}/${BINARY}"

# Verify
if command -v "$BINARY" >/dev/null 2>&1; then
  echo "Version: $("$BINARY" --version 2>/dev/null || echo "${TAG}")"
else
  echo ""
  echo "NOTE: ${INSTALL_DIR} is not in your PATH."
  echo "Add it with: export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
