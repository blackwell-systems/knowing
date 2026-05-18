# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **Traversal cache** | L1 in-memory LRU for hot paths. Repeat queries should be instant. | Medium |
| 3 | **Ownership edges** | CODEOWNERS parsing + `owned_by` edges. Blast radius that answers "which team gets paged?" | Medium |
| 4 | **MCP resources** | `knowing://context/<scope>` subscribable resources for live context updates. | Medium |
| 5 | **Deepen TS/Python LSP** | Auto-detection shipped but depth is Go-only. TS and Python users get ast_inferred (0.7) not lsp_resolved (0.9). | Medium |

## Retrieval Pipeline

Pipeline is shipped and measured (31.6% P@10, 55 fixtures, 23 experiments). See [retrieval-pipeline.md](retrieval-pipeline.md) for the authoritative reference.

**Next retrieval improvements (per local-first philosophy):**

| Item | Description | Status |
|------|-------------|--------|
| More equivalence concepts | Expand from 41 to 100+ as usage patterns emerge | Ongoing |
| Passive task memory compounding | Needs real agent sessions to accumulate data | Waiting on users |
| Code-tuned embedding model | Benchmark jina-code-v2 / bge-code when ONNX available | Planned (optional) |
| Community-aware retrieval | Constrain RWR walk to seed communities | Planned |

## Edge Type Expansion

### Runtime Intelligence

| Item | Description | Priority |
|------|-------------|----------|
| `runtime_queries` | Service/function queries database table/view/procedure | P1 |
| `runtime_connects_to` | Observed network connection beyond traced HTTP/RPC | P2 |
| `runtime_errors_at` | Symbol/route produces runtime errors | P3 |
| `runtime_uses_config` | Function reads config key or secret at runtime | P4 |
| `runtime_emits_metric` | Symbol emits a named metric | P5 |
| `runtime_logs_event` | Symbol emits a structured log event type | P5 |

### Contract and API Edges

| Item | Description | Priority |
|------|-------------|----------|
| `implements_endpoint` | Handler function implements OpenAPI route | P1 |
| `consumes_endpoint` | Client code calls OpenAPI route | P1 |
| `implements_rpc` | Server implements proto RPC method | P2 |
| `consumes_rpc` | Client invokes proto RPC method | P2 |
| `publishes_event_schema` | Producer emits event matching a contract | P3 |
| `consumes_event_schema` | Consumer expects event matching a contract | P3 |

### Ownership and Governance

| Item | Description | Priority |
|------|-------------|----------|
| `owned_by` | Symbol/file/service owned by team/person (CODEOWNERS) | P1 |
| `classified_as` | Data classification (PII, PCI, PHI) | P2 |
| `secured_by` | Route/service protected by auth policy | P3 |

### Static Semantic Edges

| Item | Description | Priority |
|------|-------------|----------|
| `extends` / `inherits` | Class inheritance (Java, C#, Python, TS) | P1 |
| `overrides` | Method overrides parent/interface method | P1 |
| `decorates` / `annotates` | Decorators, annotations, attributes | P2 |
| `throws` / `raises` | Error/exception relationships | P3 |

### Agent Workflow Edges

| Item | Description | Priority |
|------|-------------|----------|
| `suggested_for_task` | Symbol was included in agent context for a task | P1 |
| `used_by_agent` | Agent actually used/read/edited symbol | P1 |
| `validated_by_test` | Test verified symbol/change | P2 |
| `failed_in_ci` | Symbol associated with failing check | P2 |

### Deployment and Infrastructure Edges

| Item | Description | Priority |
|------|-------------|----------|
| `runs_on` | Service runs on deployment/node/runtime | P1 |
| `deployed_by` | Workflow/pipeline deploys service | P1 |
| `configured_by` | Config/secret/env var configures service | P2 |

## Developer Visibility

| Item | Description |
|------|-------------|
| Ownership routing | "Who to notify" computed from graph edges (depends on ownership edges) |
| Staleness dashboard | Surface unverified edges and subgraphs |

## Agent Coordination

| Item | Description |
|------|-------------|
| Pending mutations | Agents announce in-flight changes, others see proposed state |
| Temporal reasoning | Walk snapshots backward to find when incompatibilities appeared |
| Federated graphs | Cross-instance queries via Merkle diff exchange |

## Strategic Position

knowing is a content-addressed graph retrieval layer. The retrieval pipeline uses equivalence classes (not embeddings) as the primary concept-matching mechanism. This is local, deterministic, inspectable, and compounds with use. See [retrieval-pipeline.md](retrieval-pipeline.md) for the design rationale.

**What's shipped:** 55K LOC Go, 17 extractor types, 22 MCP tools, 5 wire formats (GCF/TOON/JSON/XML/markdown), 55 eval fixtures, multi-language LSP auto-detection, `knowing init` one-command setup.
