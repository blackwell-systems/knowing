# Distribution

How to install knowing. The release pipeline is fully automated: every `v*` tag push builds, tests, and publishes to all channels.

## Install

| Method | Command |
|--------|---------|
| **Homebrew** (macOS/Linux) | `brew install blackwell-systems/tap/knowing` |
| **npm** | `npm install -g @blackwell-systems/knowing` |
| **PyPI** | `pip install knowing` |
| **Go** | `go install github.com/blackwell-systems/knowing/cmd/knowing@latest` |
| **Docker** | `docker pull ghcr.io/blackwell-systems/knowing:latest` |

All methods install the same single binary. No runtime dependencies.

## Channels

### Homebrew
```bash
brew install blackwell-systems/tap/knowing
```
Formula in [blackwell-systems/homebrew-tap](https://github.com/blackwell-systems/homebrew-tap) updated automatically on every release.

### npm
```bash
npm install -g @blackwell-systems/knowing
```
Uses the optionalDependencies pattern (same as esbuild): a root package with a JS shim and platform-specific packages each containing the native binary. npm installs only the package matching the current platform.

### PyPI
```bash
pip install knowing
```
Platform-specific wheels containing the Go binary. Each wheel is tagged with the correct platform (e.g. `macosx_11_0_arm64`, `manylinux2014_x86_64`), so pip resolves the right one automatically. No Go toolchain required.

### Docker (GHCR + Docker Hub)
```bash
docker pull ghcr.io/blackwell-systems/knowing:latest
# or
docker pull blackwellsystems/knowing:latest
```
Multi-arch images (`linux/amd64` + `linux/arm64`). Native performance on Apple Silicon and AWS Graviton.

### Go install
```bash
go install github.com/blackwell-systems/knowing/cmd/knowing@latest
```
Requires a Go toolchain. Builds from source and installs to `$GOPATH/bin`.

## Verify installation

```bash
knowing version   # should print the version
knowing stats     # after indexing, should show nodes and edges
```

## Updating

Use your package manager to update:

```bash
brew upgrade knowing                                    # Homebrew
npm update -g @blackwell-systems/knowing                # npm
pip install --upgrade knowing                            # PyPI
go install github.com/blackwell-systems/knowing/cmd/knowing@latest  # Go
```

## Documentation site

Docs at https://blackwell-systems.github.io/knowing. Built with mkdocs-material, deployed to GitHub Pages on every push to `main`.
