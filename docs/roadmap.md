# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **Session memory persistence** | SessionTracker is ephemeral. Persist session working sets to SQLite so resumed sessions compound. | Medium |
| 3 | **`knowing stats`** | Show session value: context calls, symbols served, feedback rate. | Low |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| `knowing stats` | Cumulative session value: context calls, symbols served, feedback rate, token savings. | P2 |
| Cross-repo awareness for non-Go extractors | TypeScript, Python, Rust, Java, and C# extractors use the local repo URL for all targets. Only the Go extractor has `inferRepoURL` with stdlib detection. | P2 |
| Staleness reporting | `knowing stale` reports stale edges from changed files since last snapshot. | P2 |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. | P2 |
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges. | P3 |
| `neighborhood` MCP tool | N symbols most densely connected to X within radius R. | P3 |
| GraphML/Cypher export | `knowing export -format graphml|cypher` for Neo4j, Gephi. | P3 |
| Active project scoping | `set_active_project` / `get_active_project` MCP tools. | P3 |

## Retrieval Pipeline

| Item | Status |
|------|--------|
| More equivalence concepts (84 -> 150+) | Ongoing |
| Session memory persistence | Planned |
| Code-tuned embedding model | Planned (optional) |
| Community-aware retrieval | Planned |

## Edge Type Expansion

| Category | Items |
|----------|-------|
| Runtime | `runtime_queries`, `runtime_connects_to` |
| Contract/API | `implements_endpoint` / `consumes_endpoint`, `implements_rpc` / `consumes_rpc` |
| Ownership | `owned_by` (CODEOWNERS) |
| Static semantic | `extends` / `inherits` / `overrides` |
| Agent workflow | `suggested_for_task` / `used_by_agent` |
| Deployment | `runs_on` / `deployed_by` |

## Underexploited Capabilities

| Item | Next step |
|------|-----------|
| Community-aware retrieval | Constrain RWR walk to seed communities |
| Edge event log | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Add via community registry when a Go implementation exists |

## Phase 4: Remaining Items

| Feature | Status |
|---------|--------|
| Federated sync (exchange roots, transfer only differing branches) | Planned |
| Merkleized feedback validity (expires when neighborhood_root changes) | Planned |
| Merkle-based bisection (binary search on snapshot chain) | Planned |
| Lazy materialization (load only visited subtrees; triggered at ~1M+ edges) | Planned |
| File-level roots (finer single-file invalidation) | Decision pending |

## Cross-Repo Validation

### Tier 1: Synthetic Multi-Repo Fixture (built)

3 Go modules at `test/cross-repo/`. Cross-repo edge resolution verified. Remaining dogfooding tests:

- `knowing prove` across repos
- `knowing prove-absent` across repos
- `knowing audit` across repos
- `knowing export` to knowing-viz with cross-repo edges
- `blast_radius` on module-a function showing callers in B and C
- Incremental invalidation across repos

### Tier 2: Grafana Ecosystem (scale validation)

Grafana + Loki + Tempo + Mimir (~1.3M LOC, 4 repos). Validates cross-repo at realistic scale. Run manually, not in CI.

## Production Scale: Permanent Runtime Record

The endgame: knowing with continuous OTLP trace ingestion alongside static analysis. After a year:

- Static edges: ~150K (stable)
- Runtime edges: millions (every observed call path)
- Snapshot chain: 365+ daily snapshots

### What's Needed at Scale

| Capability | Why |
|-----------|-----|
| Lazy materialization | Load only visited subtrees at millions of edges |
| Merkle bisection | O(log N) snapshot search instead of O(N) |
| Partitioned storage | Static and runtime edges have different lifecycles |
| Runtime edge compaction | Collapse observation history |
| Federated sync | CI instance + production instance exchange diffs |
| Drift alerts | Static analysis vs production traffic divergence |
| Dashboard | Real-time runtime graph visualization |
| Automated compliance reports | Scheduled `knowing audit` with diff against prior |

### Commercial Angle

| Offering | Revenue model |
|----------|--------------|
| knowing Cloud | Managed hosting, per-service pricing |
| Compliance reporting | Automated quarterly audit reports with proofs |
| Federated sync service | Org-wide intelligence sharing |
| Drift detection | Alerts on static/runtime divergence |
| Enterprise dashboard | Cross-repo visualization, team analytics |

## Git-Inspired (Not Yet Built)

| Item | Priority | Effort |
|------|----------|--------|
| Proposed graph overlay (staging area) | P2 | Medium |
| Delta-compressed snapshots | P3 | High |
| N-way hierarchical diff | P3 | Medium |
| Rerere (enrichment conflict resolution) | P4 | Low |
| Transfer protocol (federated sync) | P4 | High |
| Replace/grafts (edge correction) | P4 | Medium |
