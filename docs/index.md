# knowing

**The system of record for how software systems behave, change, and relate over time.**

Git is the system of record for source code. knowing is the system of record for what that source code *means* in the context of a running organization.

## Quick Start

```bash
# Install
brew install blackwell-systems/tap/knowing

# Index a repository (registers in roster, assigns per-repo database)
knowing add /path/to/repo

# Verify the index worked
knowing stats

# Query the graph for a symbol
knowing query "FunctionName"

# Generate context for an agent
knowing context -task "refactor auth middleware" -budget 50000

# Explain why a symbol ranked where it did
knowing why -task "refactor auth" -symbol "AuthHandler"

# Compute semantic diff between snapshots
knowing diff @prev @latest

# Export the graph
knowing export -format json

# Start the daemon with MCP server (HTTP)
knowing serve /path/to/repo

# Start the MCP server over stdio (for .mcp.json, zero-config)
knowing mcp --watch
```

For detailed setup instructions, troubleshooting, and MCP integration, see the
[CLI Reference](guide/cli.md) and the [README Quick Start](../README.md#quick-start).

## What It Does

knowing builds a content-addressed knowledge graph of software relationships and exposes it to agents via MCP. Static analysis fused with runtime traces from OpenTelemetry and SCIP indexes for external dependencies. Every edge has provenance and confidence. Every state is a hash.

- **26 extractor packages** (12 languages + 13 infrastructure/cloud formats): Go, TypeScript/JS, Python, Rust, Java, C#, Terraform, SQL, Kubernetes YAML, Cloud YAML (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework), CSS, Protocol Buffers, Dockerfile, Makefile, Helm Charts, GitLab CI, package.json/npm, GraphQL, Ansible
- **18 web frameworks**: route detection for net/http, chi, gin, echo, gorilla/mux, Express.js, Fastify, Hono, NestJS, Next.js, Flask, FastAPI, Django, Actix, Axum, Rocket, Spring, ASP.NET
- **28 MCP tools + 8 MCP resources**: graph queries, runtime traffic, semantic diff, PR impact, context packing, feedback, test scope, flow analysis, community detection (with algorithm selection), ownership queries, cryptographic proofs, repo management; resources for graph orientation without tool calls (`knowing://report`, `knowing://schema`, `knowing://stats`, `knowing://repos`, `knowing://session`, `knowing://index-health`, `knowing://communities`, `knowing://community/{id}`)
- **28 CLI commands**: serve, daemon, index, query, export, diff, context, why, mcp, watch, reindex, init, add, remove, list, stats, reset, vacuum, test-scope, ingest-scip, enrich, fsck, prove, verify, prove-absent, audit, audit-diff, version
- **Runtime intelligence**: OTel trace ingestion with observation-based confidence scoring
- **Incremental updates**: git-based change detection, re-indexes only changed files
- **Content-addressed**: every node, edge, and snapshot is a hash with domain-type prefix tags (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) for structural type safety
- **Hierarchical Merkle tree**: 216x faster diff on real graphs (~24.9K edges, 517x at 100K edges), O(1) subgraph root lookups at 59ns, modular community detection registry

## Documentation

- [Introduction](guide/introduction.md): how knowing works, the retrieval pipeline explained from zero, try-it-in-5-minutes walkthrough
- [CLI Reference](guide/cli.md): all 28 commands with flags, examples, and [troubleshooting](guide/cli.md#troubleshooting)
- [MCP Tools](guide/mcp-tools.md): all 28 tools and 8 resources with parameters and return formats
- [Architecture](architecture/): system design, concurrency model, data flow, hierarchical Merkle tree
- [Edge Types](architecture/edge-types.md): all 38 edge types with provenance and confidence
- [Diagnostic Tools](guide/diagnostic-tools.md): retrieval quality investigation, ablation studies
- [Runtime Traces](operations/runtime-traces.md): OTel ingestion design
- [Roadmap](roadmap.md): what's done, what's next
- [Distribution](guide/distribution.md): all installation channels
