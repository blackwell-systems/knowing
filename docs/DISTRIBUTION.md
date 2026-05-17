# Distribution

This document describes how knowing is distributed, what is automated, and what is planned.

## v0.1.0 Release Readiness

| Component | Status | Notes |
|-----------|--------|-------|
| GoReleaser config | Ready | `.goreleaser.yml`, 6 platform binaries |
| Release workflow | Ready | `.github/workflows/release.yml`, triggered on `v*` tags |
| npm package structure | Ready | `npm/` directory, root + 4 platform packages |
| PyPI package structure | Ready | `pypi/` directory, `pyproject.toml` |
| `NPM_TOKEN` secret | **Set** | |
| `PYPI_TOKEN` secret | **Set** | |
| `HOMEBREW_TAP_TOKEN` secret | Needed | GitHub PAT with repo scope for homebrew-tap |
| `DOCKERHUB_USERNAME` secret | Needed | For Docker Hub publishing |
| `DOCKERHUB_TOKEN` secret | Needed | For Docker Hub publishing |
| `docker/Dockerfile` | Ready | Alpine-based, git included for indexing |
| `scripts/npm-publish.sh` | Ready | Downloads binaries from release, publishes 5 packages |
| `scripts/pypi-build-wheels.sh` | Ready | Builds platform wheels from Go binaries |
| `server.json` | Ready | MCP registry manifest (22 tools listed) |
| homebrew-tap repo | Ready | `blackwell-systems/homebrew-tap` exists (public) |

**Minimum viable release (npm + PyPI + GitHub Release + Homebrew):** Needs HOMEBREW_TAP_TOKEN + missing scripts.

**Without Docker:** Skip DOCKERHUB secrets and Dockerfile; GoReleaser will warn but still publish binaries, Homebrew, and GitHub Release.

## Current channels

### GitHub Releases
Pre-built binaries for all platforms, published automatically by GoReleaser on every `v*` tag.

| Platform | Architecture |
|----------|-------------|
| macOS | arm64, amd64 |
| Linux | arm64, amd64 |
| Windows | arm64, amd64 |

### Homebrew
```bash
brew install blackwell-systems/tap/knowing
```
Formula in [blackwell-systems/homebrew-tap](https://github.com/blackwell-systems/homebrew-tap) updated automatically by GoReleaser on every release.

### curl | sh (macOS / Linux)
```bash
curl -fsSL https://raw.githubusercontent.com/blackwell-systems/knowing/main/install.sh | sh
```
Detects OS and architecture, downloads the matching binary from GitHub Releases, installs to `/usr/local/bin`.

### PowerShell (Windows)
```powershell
iwr -useb https://raw.githubusercontent.com/blackwell-systems/knowing/main/install.ps1 | iex
```
Detects amd64/arm64, downloads the matching zip from GitHub Releases, installs to `%LOCALAPPDATA%\knowing`, adds to user PATH. No admin required.

### Scoop (Windows)
```powershell
scoop bucket add blackwell-systems https://github.com/blackwell-systems/knowing
scoop install blackwell-systems/knowing
```
Manifest at `bucket/knowing.json` in this repo (the repo doubles as the Scoop bucket). `autoupdate` configured so `scoop update knowing` picks up new releases automatically.

### Winget (Windows)
```powershell
winget install BlackwellSystems.knowing
```
Manifests at `winget/manifests/b/BlackwellSystems/knowing/`. Submit new versions as a PR to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs).

### npm
```bash
npm install -g @blackwell-systems/knowing
```
Uses the optionalDependencies pattern (same as esbuild): a root package with a JS shim and six platform-specific packages each containing the native binary. npm installs only the package matching the current platform.

Published automatically by the `npm-publish` CI job after GoReleaser completes.

**Packages:**
- `@blackwell-systems/knowing` (root; install this)
- `@blackwell-systems/knowing-darwin-arm64`
- `@blackwell-systems/knowing-darwin-x64`
- `@blackwell-systems/knowing-linux-arm64`
- `@blackwell-systems/knowing-linux-x64`
- `@blackwell-systems/knowing-win32-x64`
- `@blackwell-systems/knowing-win32-arm64`

### Docker (GHCR + Docker Hub)
```bash
# GHCR
docker pull ghcr.io/blackwell-systems/knowing:latest

# Docker Hub
docker pull blackwellsystems/knowing:latest
```

All images are multi-arch (`linux/amd64` + `linux/arm64`) via Docker manifest lists. Native performance on Apple Silicon and AWS Graviton, with no Rosetta/QEMU emulation. Built and pushed to both registries automatically by GoReleaser on every `v*` tag. Tags: `latest`, semver (`0.1.2`, `0.1`).

### MCP registries

#### Official MCP Registry
Published automatically via `mcp-publisher` in CI using GitHub OIDC (no secrets required).

**Server name:** `io.github.blackwell-systems/knowing`

```bash
curl "https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.blackwell-systems/knowing"
```

#### Glama
Listed at [glama.ai/mcp/servers/blackwell-systems/knowing](https://glama.ai/mcp/servers/blackwell-systems/knowing). Profile managed via `glama.json` in repo root.

### PyPI
```bash
pip install knowing
```
Platform-specific wheels containing the Go binary. Each wheel is tagged with the correct platform (e.g. `macosx_11_0_arm64`, `manylinux2014_x86_64`), so pip resolves the right one automatically. No Go toolchain required. Built and published automatically by the `pypi-publish` CI job on every release tag.

### Self-update
```bash
knowing update           # Download and replace binary with latest release
knowing update --check   # Compare current vs latest version without downloading
knowing update --force   # Update even if already on the latest version
```
Fetches the latest release from the GitHub Releases API, downloads the correct binary for the current OS and architecture, and atomically replaces the running binary. Works regardless of the original install method.

### Clean uninstall
```bash
knowing uninstall           # Remove all configs, database, caches
knowing uninstall --dry-run # Preview what would be removed
```
Removes MCP server entries from `.mcp.json`, `.cursor/mcp.json`, and other config files. Removes the knowing database and cache directories. Does not remove the binary itself (prints the `rm $(which knowing)` command for manual removal).

### Go install
```bash
go install github.com/blackwell-systems/knowing/cmd/knowing@latest
```
Requires a Go toolchain. Builds from source and installs to `$GOPATH/bin`.

### Smithery
`smithery.yaml` in the repo root enables auto-indexing on Smithery. Auto-discovered from GitHub.

### cursor.directory
Submitted. Listed under Developer Tools.

### mcpservers.org
Manually submitted. Free listing.

### Awesome MCP Servers
Submitted to [punkpeye/awesome-mcp-servers](https://github.com/punkpeye/awesome-mcp-servers).

## Documentation site

**URL:** [knowing.dev](https://knowing.dev) (planned)

Built with mkdocs-material from the `docs/` folder. Deployed to GitHub Pages automatically on every push to `main`. Custom domain via Cloudflare DNS.

## Release pipeline

Every `git tag v*` push triggers three sequential CI jobs:

```
release              → GoReleaser: binaries, GitHub Release, Homebrew formula,
                       Docker images (GHCR + Docker Hub)
npm-publish          → downloads binaries from GitHub Release, publishes 7 npm packages
mcp-registry-publish → publishes metadata to official MCP Registry (GitHub OIDC)
```

Docker images are built inside the `release` job by GoReleaser (`dockers:` section). Multi-arch manifest lists built for linux/amd64 + linux/arm64.

## Marketing and Discovery

| Channel | Status | Notes |
|---------|--------|-------|
| **LinkedIn** | Not posted | Launch announcement ready. |
| **Reddit** | Not posted | r/mcp, r/ClaudeCode, r/golang. |
| **Hacker News** | Not submitted | "Runtime traces as graph edges" angle is HN-ready. |
| **Go Weekly** | Not submitted | Submit blog post link. |
| **Twitter/X** | Not active | Thread format works for the data. |
| **glama.ai** | Planned | MCP server discovery. |
| **Product Hunt** | Not launched | Save for bigger release (runtime traces). |
| **YouTube** | Not started | "Watch a codebase build its own knowledge graph" demo. |

### Awesome Lists

| List | Stars | Status | Section |
|------|------:|--------|---------|
| punkpeye/awesome-mcp-servers | 86K | Planned | Developer Tools |
| ComposioHQ/awesome-claude-skills | 59K | Planned | Development & Code Tools |
| hesreallyhim/awesome-claude-code | 43K | Planned | Tooling |
| wong2/awesome-mcp-servers | 4K | Planned | Community Servers |
| ai-for-developers/awesome-ai-coding-tools | 1.7K | Planned | MCP Servers and Directories |
| rohitg00/awesome-claude-code-toolkit | 1.6K | Planned | Skills |
| avelino/awesome-go | 172K | Blocked (5-month history req) | Go Tools |

## Planned

| Channel | Notes |
|---------|-------|
| **Nix flake** | `nix run github:blackwell-systems/knowing` |
| **mcp.so** | Top Google result for "MCP servers"; direct submission |
| **VS Code extension** | Zero-setup path for Copilot/Continue/Cline users |
| **OTel Collector contrib** | knowing as an OTel exporter plugin for direct trace ingestion |
| **GitHub Marketplace** | GitHub Action for CI-integrated indexing and semantic PR diffs |
