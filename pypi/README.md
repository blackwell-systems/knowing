# knowing

**A self-adapting code intelligence engine that gets smarter with scale, not dumber.**

Content-addressed graph of code relationships with density-adaptive retrieval. Observes its own graph structure and adjusts how it searches: on small repos it finds symbols by keyword; on large enterprise codebases it automatically shifts to structural navigation. 34 edge types, cryptographic proofs, hierarchical Merkle snapshots.

This is the Python wrapper package that downloads and runs the `knowing` binary.

## Install

```bash
pip install knowing
```

## Usage

```bash
# Register and index a repository
knowing add ./path/to/repo

# Graph-ranked context for an agent task (84% fewer tokens than JSON)
knowing context -task "refactor auth" -format gcf

# Find affected tests for changed files
knowing test-scope -files internal/auth/session.go

# Explain why a symbol ranked where it did
knowing why -task "refactor auth" -symbol "SessionHandler"

# Run the MCP server with live file watching
knowing mcp --watch

# Verify graph integrity
knowing fsck
```

## What It Does

knowing indexes code across 12 languages and 26 extractor packages into a content-addressed knowledge graph. Every node, edge, and snapshot is SHA-256 hashed with domain-type prefixes. Snapshots are structured as hierarchical Merkle trees (repo -> package -> edge-type -> leaf), enabling O(packages) diffs instead of O(edges) full scans.

**For AI agents:** 28 MCP tools + 8 MCP resources serve graph-ranked context over stdio or HTTP. The GCF wire format delivers 84% token savings versus JSON. Agents get trustworthy, cacheable, replayable context with provenance and confidence on every edge.

**Key capabilities:**

- **26 extractor packages:** Go, TypeScript, Python, Rust, Java, C#, Protobuf, Terraform, SQL, Kubernetes, CloudFormation, Docker Compose, GitHub Actions, Helm, GraphQL, and more
- **Hierarchical Merkle diffs:** 216x faster on real graphs (~24.9K edges), 517x at 100K edges
- **Subgraph cache:** 93x faster repeat queries via content-addressed cache keys
- **Runtime fusion:** OpenTelemetry trace ingestion merges static and runtime views
- **Graph notes:** general-purpose metadata layer for community assignments, context pack persistence, and feedback annotations
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
