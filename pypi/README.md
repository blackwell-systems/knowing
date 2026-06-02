# knowing

**Self-adapting code intelligence engine.** Gives AI agents ranked, graph-aware context instead of grep results. Gets smarter with scale, not dumber.

This is the Python wrapper package that downloads and runs the `knowing` binary.

## Install and verify

```bash
pip install knowing
knowing version   # should print the version
```

## Configure your agent

Add to your agent's MCP config (`.mcp.json` for Claude Code, `.cursor/mcp.json` for Cursor, `.vscode/mcp.json` for VS Code, [see all](https://github.com/blackwell-systems/knowing#mcp-integration)):

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "--watch"],
      "transport": "stdio"
    }
  }
}
```

The MCP server auto-indexes your repo on first launch (10-30 seconds). No model downloads, no API keys required.

## First useful query

Ask your agent:

> *"Use the context_for_task tool to find symbols related to [something you know exists in your code]."*

You should see ranked symbols with scores and file paths. If results are empty, the repo is still indexing. If results seem unrelated, use specific symbol names in your task description.

## What it does

knowing indexes code across 23 extractors (Go, TypeScript, Python, Rust, Java, C#, and more) into a content-addressed knowledge graph. 38 edge types, 28 MCP tools, 263 equivalence classes bridging task vocabulary to code symbols.

P@10 = 0.281 across 308 tasks, 16 repos, 8 languages. 12 self-adapting mechanisms. 3.20x codegraph, 5.05x GitNexus.

## CLI usage

```bash
knowing add .                                          # index a repo
knowing context -task "refactor auth" -format gcf      # ranked context
knowing test-scope -files internal/auth/handler.go     # affected tests
knowing why -task "refactor auth" -symbol "SessionHandler"  # explain ranking
knowing enrich lsp                                     # LSP enrichment for higher-quality edges
```

## Documentation

Full docs at https://blackwell-systems.github.io/knowing

Source: https://github.com/blackwell-systems/knowing
