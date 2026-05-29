# Release Operations (Internal)

This document contains internal release pipeline details, CI secrets, marketing plans, and discovery tracking. Not part of the public docs site.

## Release Pipeline

Every `git tag v*` push triggers sequential CI jobs:

```
build                → 4 platform binaries (linux/mac, amd64/arm64)
release              → GitHub Release, Homebrew formula update
docker               → Docker images (GHCR + Docker Hub)
npm-publish          → downloads binaries from GitHub Release, publishes 7 npm packages
pypi-publish         → builds platform wheels, publishes to PyPI
mcp-registry-publish → publishes metadata to official MCP Registry (GitHub OIDC)
```

### CI secrets (all configured)

| Secret | Purpose |
|--------|---------|
| `NPM_TOKEN` | npm publish |
| `PYPI_TOKEN` | PyPI wheel upload |
| `HOMEBREW_TAP_TOKEN` | Push formula to homebrew-tap |
| `DOCKERHUB_USERNAME` | Docker Hub login |
| `DOCKERHUB_TOKEN` | Docker Hub push |

### Platform binaries

| OS | Architecture | Build method |
|----|-------------|--------------|
| Linux | amd64 | Native GCC on ubuntu-latest |
| Linux | arm64 | Cross-compile via aarch64-linux-gnu-gcc |
| macOS | arm64 | Native clang on macos-latest |
| macOS | amd64 | Cross-compile clang on macos-latest |

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

## Planned Channels

| Channel | Notes |
|---------|-------|
| **Nix flake** | `nix run github:blackwell-systems/knowing` |
| **mcp.so** | Top Google result for "MCP servers"; direct submission |
| **VS Code extension** | Zero-setup path for Copilot/Continue/Cline users |
| **OTel Collector contrib** | knowing as an OTel exporter plugin for direct trace ingestion |
| **GitHub Marketplace** | GitHub Action for CI-integrated indexing and semantic PR diffs |
