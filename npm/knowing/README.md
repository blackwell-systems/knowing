# @blackwell-systems/knowing

**Git versions files. knowing versions the understanding of code.**

An intelligence versioning system: a content-addressed graph where every relationship between symbols is tracked, scored, and snapshotted with hierarchical Merkle trees.

## Install

```bash
npm install -g @blackwell-systems/knowing
```

Or use directly:

```bash
npx @blackwell-systems/knowing index ./my-repo
npx @blackwell-systems/knowing context -task "refactor auth" -format gcf
npx @blackwell-systems/knowing mcp --watch
```

## What It Does

knowing indexes code across 12 languages and 26 extractor packages into a content-addressed knowledge graph. Every node, edge, and snapshot is SHA-256 hashed with domain-type prefixes. Snapshots are structured as hierarchical Merkle trees (repo -> package -> edge-type -> leaf), enabling O(packages) diffs instead of O(edges) full scans.

**For AI agents:** 27 MCP tools + 8 MCP resources serve graph-ranked context over stdio or HTTP. The GCF wire format delivers 84% token savings versus JSON. Agents get trustworthy, cacheable, replayable context with provenance and confidence on every edge.

**Key capabilities:**

- **26 extractor packages:** Go, TypeScript, Python, Rust, Java, C#, Protobuf, Terraform, SQL, Kubernetes, CloudFormation, Docker Compose, GitHub Actions, Helm, GraphQL, and more
- **Hierarchical Merkle diffs:** 216x faster on real graphs (~24.9K edges), 517x at 100K edges
- **Subgraph cache:** 93x faster repeat queries via content-addressed cache keys
- **Runtime fusion:** OpenTelemetry trace ingestion merges static and runtime views
- **`knowing fsck`:** git-style integrity verification of the entire graph
- **Feedback loop:** rankings improve with use as agents mark useful symbols

## Agent Integration

Add to `.mcp.json`:

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

## Documentation

Full docs, architecture, benchmarks, and roadmap at https://blackwell-systems.github.io/knowing

Source: https://github.com/blackwell-systems/knowing
