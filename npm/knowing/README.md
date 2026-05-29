# @blackwell-systems/knowing

**Self-adapting code intelligence engine.** Gives AI agents ranked, graph-aware context instead of grep results. Gets smarter with scale, not dumber.

## Install and verify

```bash
npm install -g @blackwell-systems/knowing
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

The MCP server auto-indexes your repo on first launch (10-30 seconds). The embedding re-ranker is on by default (downloads a 30MB model once, runs locally, no API keys).

## First useful query

Ask your agent:

> *"Use the context_for_task tool to find symbols related to [something you know exists in your code]."*

You should see ranked symbols with scores and file paths. If results are empty, the repo is still indexing. If results seem unrelated, use specific symbol names in your task description.

## What it does

knowing indexes code across 23 extractors (Go, TypeScript, Python, Rust, Java, C#, and more) into a content-addressed knowledge graph. 38 edge types, 28 MCP tools, 152 equivalence classes, local embedding re-ranker (+17% precision), gap-fill seeds (+11% precision).

P@10 = 0.264 across 257 tasks, 13 repos, 8 languages. 1.96x codegraph, 3.52x GitNexus.

## CLI usage

```bash
knowing add .                                          # index a repo
knowing context -task "refactor auth" -format gcf      # ranked context
knowing test-scope -files internal/auth/handler.go     # affected tests
knowing why -task "refactor auth" -symbol "SessionHandler"  # explain ranking
knowing enrich embeddings                              # pre-cache vectors for faster queries
```

## Documentation

Full docs at https://blackwell-systems.github.io/knowing

Source: https://github.com/blackwell-systems/knowing
