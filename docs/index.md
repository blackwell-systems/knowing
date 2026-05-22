# knowing

**The system of record for how software systems behave, change, and relate over time.**

Git is the system of record for source code. knowing is the system of record for what that source code *means* in the context of a running organization.

## Quick Start

```bash
# Install
brew install blackwell-systems/tap/knowing

# Index a repository
knowing index /path/to/repo

# Query the graph
knowing query "pkg.FunctionName"

# Generate context for an agent
knowing context --task "refactor auth middleware" --budget 50000

# Compute semantic diff
knowing diff <old-snapshot> <new-snapshot>

# Export the graph
knowing export --format json

# Start the daemon with MCP server (HTTP)
knowing serve /path/to/repo

# Start the MCP server over stdio (for .mcp.json)
knowing mcp -db knowing.db
```

## What It Does

knowing builds a content-addressed knowledge graph of software relationships and exposes it to agents via MCP. Static analysis fused with runtime traces from OpenTelemetry and SCIP indexes for external dependencies. Every edge has provenance and confidence. Every state is a hash.

- **26 extractor packages** (12 languages + 13 infrastructure/cloud formats): Go, TypeScript/JS, Python, Rust, Java, C#, Terraform, SQL, Kubernetes YAML, Cloud YAML (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework), CSS, Protocol Buffers, Dockerfile, Makefile, Helm Charts, GitLab CI, package.json/npm, GraphQL, Ansible
- **18 web frameworks**: route detection for net/http, chi, gin, echo, gorilla/mux, Express.js, Fastify, Hono, NestJS, Next.js, Flask, FastAPI, Django, Actix, Axum, Rocket, Spring, ASP.NET
- **27 MCP tools + 8 MCP resources**: graph queries, runtime traffic, semantic diff, PR impact, context packing, feedback, test scope, flow analysis, community detection (with algorithm selection), ownership queries, cryptographic proofs; resources for graph orientation without tool calls (`knowing://report`, `knowing://schema`, `knowing://stats`, `knowing://repos`, `knowing://session`, `knowing://index-health`, `knowing://communities`, `knowing://community/{id}`)
- **27 CLI commands**: serve, index, query, export, diff, context, why, mcp, watch, reindex, init, add, remove, list, stats, reset, vacuum, test-scope, ingest-scip, enrich, fsck, prove, verify, prove-absent, audit, audit-diff, version
- **Runtime intelligence**: OTel trace ingestion with observation-based confidence scoring
- **Incremental updates**: git-based change detection, re-indexes only changed files
- **Content-addressed**: every node, edge, and snapshot is a hash with domain-type prefix tags (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) for structural type safety
- **Hierarchical Merkle tree**: 216x faster diff on real graphs (~24.9K edges, 517x at 100K edges), O(1) subgraph root lookups at 59ns, modular community detection registry

## Documentation

- [Architecture](architecture/): system design, concurrency model, data flow, hierarchical Merkle tree, git design audit
- [CLI Reference](guide/cli.md): all 27 commands with flags and examples
- [MCP Tools](guide/mcp-tools.md): all 27 tools and 8 resources with parameters and return formats
- [Edge Types](architecture/edge-types.md): all 30 edge types with provenance and confidence
- [Runtime Traces](operations/runtime-traces.md): OTel ingestion design
- [Roadmap](roadmap.md): what's done, what's next
- [Distribution](guide/distribution.md): all installation channels
