# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **Session memory persistence** | SessionTracker is ephemeral (lost on session end). Persist session working sets to SQLite so resumed sessions pick up where they left off. Extends `internal/context/session.go` with a `session_events` table. | Medium |
| 3 | **`knowing stats`** | Show session value: context calls, symbols served, feedback rate. Lets users see the value accumulating. | Low |
| 4 | **MCP resources** | Lightweight context that doesn't cost a tool call. Resources are read directly by the MCP host for agent orientation. See detailed list below. | Medium |
| 5 | **Notes table** | Metadata without hash invalidation (git-inspired). Stores feedback, annotations, quality scores without affecting Merkle computation. See Git Lessons section. | Low |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| `knowing stats` | Cumulative session value: context calls, symbols served, feedback rate, token savings. | P2 |
| Staleness reporting | `knowing stale` reports edges that are stale because files changed since the last snapshot. Free win from content-addressing. | P2 |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. Detached mode with PID file. | P2 |
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots. Currently requires manual SQL. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges up/down/both from a type. | P3 |
| `neighborhood` MCP tool | Seed-based dense neighborhood: N symbols most densely connected to X within radius R. | P3 |
| GraphML/Cypher export | `knowing export -format graphml|cypher` for Neo4j, Gephi, yEd. | P3 |
| Active project scoping | `set_active_project` / `get_active_project` MCP tools for session-level repo default. | P3 |

## MCP Resources (Planned)

| Resource | What it provides | Priority |
|----------|-----------------|----------|
| `knowing://report` | Graph size, top languages/kinds, hotspot count. Session opener. | P1 |
| `knowing://schema` | Node kinds, edge kinds, provenance tiers, qualified-ID format. | P1 |
| `knowing://stats` | Node/edge counts, per-language and per-repo breakdown. | P1 |
| `knowing://repos` | Every tracked repo with node/edge counts. | P2 |
| `knowing://session` | Current session state: recent symbols, context calls, feedback rate. | P2 |
| `knowing://index-health` | Health score, parse failures, stale files. | P2 |
| `knowing://communities` | Community list with cohesion scores and Merkle roots. | P3 |
| `knowing://community/{id}` | Single community detail: members, key files, cross-community connections. | P3 |

## Underexploited Capabilities

| Item | Status | Next step |
|------|--------|-----------|
| Community-aware retrieval | Communities computed, not used for scoping | Constrain RWR walk to seed communities |
| Edge event log | Events recorded, nothing reads them | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Not yet available in Go | Add via community registry when a Go implementation exists |

## Retrieval Pipeline

Pipeline is shipped and measured (31.6% P@10, 55 fixtures). See [retrieval-pipeline.md](architecture/retrieval-pipeline.md).

| Item | Description | Status |
|------|-------------|--------|
| More equivalence concepts | Expand from 84 to 150+ as usage patterns emerge | Ongoing |
| Session memory persistence | Persist session working sets to SQLite | Planned |
| Code-tuned embedding model | Benchmark jina-code-v2 / bge-code when ONNX available | Planned (optional) |
| Community-aware retrieval | Constrain RWR walk to seed communities | Planned |

## Edge Type Expansion

### Runtime Intelligence

| Item | Priority |
|------|----------|
| `runtime_queries` (service queries database table/view) | P1 |
| `runtime_connects_to` (observed network connection) | P2 |
| `runtime_errors_at` (runtime errors at symbol/route) | P3 |
| `runtime_uses_config` / `runtime_emits_metric` / `runtime_logs_event` | P4-P5 |

### Contract and API Edges

| Item | Priority |
|------|----------|
| `implements_endpoint` / `consumes_endpoint` (OpenAPI routes) | P1 |
| `implements_rpc` / `consumes_rpc` (proto RPC) | P2 |
| `publishes_event_schema` / `consumes_event_schema` | P3 |

### Ownership and Governance

| Item | Priority |
|------|----------|
| `owned_by` (CODEOWNERS) | P1 |
| `classified_as` (PII, PCI, PHI) | P2 |
| `secured_by` (auth policy) | P3 |

### Static Semantic Edges

| Item | Priority |
|------|----------|
| `extends` / `inherits` / `overrides` | P1 |
| `decorates` / `annotates` | P2 |

### Agent Workflow Edges

| Item | Priority |
|------|----------|
| `suggested_for_task` / `used_by_agent` | P1 |
| `validated_by_test` / `failed_in_ci` | P2 |

### Deployment and Infrastructure Edges

| Item | Priority |
|------|----------|
| `runs_on` / `deployed_by` | P1 |
| `configured_by` | P2 |

## Merkle Tree Algorithms

Full specification in [architecture/merkle-algorithms.md](architecture/merkle-algorithms.md).

**Phase 1 (Shipped):** Hierarchical tree (repo -> package -> edge-type -> leaf), DiffHierarchicalTrees, SubgraphRoot, EdgeTypeRoot, ContextPackRoot. Flat tree dropped; hierarchical root is canonical. 114x faster diff on real graph, 517x at 100K. Hash domain prefixes shipped.

**Phase 2 (Shipped):** SubgraphCache, context_for_task/blast_radius/test_scope caching, daemon invalidation (DiffHierarchicalTrees -> selective eviction), PackRoot on ContextBlock, community Merkle roots.

### Phase 3: Incremental Recompute

| Item | Description |
|------|-------------|
| Incremental Louvain | Only recompute community membership for changed packages |
| Incremental HITS/BM25 | Only rebuild text indexes for changed packages |
| Context pack deduplication | Agents reference prior ContextPackRoot instead of resending content |
| Context pack comparison | "What changed in the context this agent would see?" |
| Semantic change classification | Diff edge-type roots: only calls changed (behavioral), only imports (structural), runtime drift |

### Phase 4: Proofs, Sync, Bisection

| Feature | Description |
|---------|-------------|
| Merkle proofs | Prove a relationship existed in snapshot X |
| Federated sync | Exchange roots, descend only differing branches |
| Snapshot-aware retrieval | Stability/activity signals from neighborhood root history |
| Merkleized feedback validity | Feedback expires when neighborhood_root changes |
| Merkle-based bisection | Binary search on snapshot chain |
| Proof of absence | Prove an edge does NOT exist |
| Lazy materialization | Load only visited subtrees |
| File-level roots | Finer single-file invalidation |

## Lessons from Git Source

Examined `github.com/git/git` source (2026-05-18). See [architecture/git-design-audit.md](architecture/git-design-audit.md) for the full 10-area audit.

**Linus's regrets applied to knowing:**
1. SHA-1 mistake: we started on SHA-256 + domain prefixes early
2. Staging area confusion: keep proposed graph overlay optional
3. Reflog visibility: surface snapshot chain in `knowing fsck` and future `knowing history`
4. Submodules broken: per-repo DBs are already better

**Git-inspired features (not yet built):**

| Item | Priority | Effort |
|------|----------|--------|
| Notes table (metadata without hash invalidation) | P1 | Low |
| Proposed graph overlay (staging area for blast radius preview) | P2 | Medium |
| Delta-compressed snapshots (store diffs, not full edge sets) | P3 | High |
| N-way hierarchical diff | P3 | Medium |
| Rerere (reuse enrichment conflict resolutions) | P4 | Low |
| Transfer protocol (have/want for federated sync) | P4 | High |
| Replace/grafts (edge correction without re-snapshot) | P4 | Medium |

**Git audit status:** 12 of 23 recommendations shipped. 15 LOW items remain. See [git-design-audit.md](architecture/git-design-audit.md) summary table.

## Strategic Position

knowing is an intelligence versioning system. Git versions files; knowing versions the understanding of code: relationships, confidence, provenance, and what changes mean.

The retrieval pipeline uses equivalence classes (not embeddings) as the primary concept-matching mechanism. Local, deterministic, inspectable, compounds with use.

The hierarchical Merkle tree structures snapshots by semantic boundaries (package, edge type). The identity structure itself is the performance architecture: 114x faster diff on real graphs, O(1) subgraph root lookups, scoped cache invalidation. No competitor uses content-addressed hierarchical graph Merkle trees.

**What's shipped:** ~67K LOC Go, 25 extractors (12 languages + 13 infrastructure formats), 23 MCP tools, 5 wire formats, 8 benchmark harnesses, 84 equivalence classes, multi-language LSP enrichment, hierarchical Merkle tree (Phase 1+2 complete), content-addressed context packs, subgraph cache with daemon invalidation, git-audited integrity layer (fsck, lockfile, GC sweep, hash domain prefixes, VerifyHash), modular community detection, React visualization with 6 grouping strategies.
