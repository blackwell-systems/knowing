# knowing

**Self-adapting code intelligence engine.** Gives AI agents ranked, graph-aware context instead of grep results. Gets smarter with scale, not dumber.

P@10 = 0.283 across 277 tasks, 14 repos, 8 languages. 2.10x codegraph, 3.77x GitNexus. 38 edge types, 28 MCP tools, 164 equivalence classes, focused seed selection + cluster-aware gap-fill.

## Get started in 60 seconds

```bash
brew install blackwell-systems/tap/knowing
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

The MCP server auto-indexes your repo on first launch. Ask your agent: *"Use context_for_task to find symbols related to [something in your code]."* You should see ranked symbols with scores and file paths.

**New to knowing?** Read the [Introduction](guide/introduction.md) for a full walkthrough with examples.

## Choose your path

| You want to... | Start here |
|---|---|
| Give your AI agent graph-ranked context | [MCP setup](guide/cli.md#mcp) |
| Explore the graph from the command line | [CLI Quick Start](guide/cli.md#quick-start) |
| Understand how the retrieval pipeline works | [Introduction](guide/introduction.md) |
| Audit code relationships with cryptographic proofs | [Audit & Compliance](guide/audit-compliance.md) |
| Understand the architecture | [System Overview](architecture/system-overview.md) |

## What It Does

knowing builds a content-addressed knowledge graph of software relationships and exposes it to agents via MCP. Static analysis fused with runtime traces from OpenTelemetry and SCIP indexes for external dependencies. Every edge has provenance and confidence. Every state is a hash.

- **23 extractors** covering 26 formats (12 languages + 13 infrastructure/cloud): Go, TypeScript/JS, Python, Rust, Java, C#, Terraform, SQL, Kubernetes YAML, Cloud YAML (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework), CSS, Protocol Buffers, Dockerfile, Makefile, Helm Charts, GitLab CI, package.json/npm, GraphQL, Ansible
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
