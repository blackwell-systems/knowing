# @blackwell-systems/knowing

Content-addressed knowledge graph for software systems.

## Install

```bash
npm install -g @blackwell-systems/knowing
```

Or use directly:

```bash
npx @blackwell-systems/knowing index ./my-repo
npx @blackwell-systems/knowing context -task "refactor auth" -format gcf
npx @blackwell-systems/knowing mcp -db knowing.db
```

## What This Is

knowing indexes code across 10 languages into a content-addressed knowledge graph (SHA-256 hashed nodes, edges, and Merkle-tree snapshots). It fuses static analysis with runtime traces and serves the result over MCP with 84% token savings via the GCF wire format.

## Documentation

See https://blackwell-systems.github.io/knowing
