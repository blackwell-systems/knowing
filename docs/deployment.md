# Deployment Models

How knowing operates at different organizational scales, from a single developer to a large microservice organization.

## Model 1: Single Instance (1-20 repos)

One knowing daemon indexes all repositories. Every agent and developer queries the same graph. The simplest deployment with zero coordination overhead.

```
Developer A (auth-service) ──┐
Developer B (api-gateway)  ──┼──> knowing daemon ──> single graph (all repos)
Developer C (billing)      ──┘
CI pipeline ─────────────────┘
```

**When to use:** One team or a few teams sharing an organization on GitHub. Total edge count under a few million. Someone owns the machine or VM that runs the daemon.

**How it works:**
- The daemon indexes all repos via `index_repo` (MCP tool or CLI)
- File watcher (fsnotify + git hooks) triggers incremental reindex on push
- All cross-repo edges resolve immediately because everything is in one graph
- The SQLite file is the single portable artifact
- Agents connect via stdio (single-agent, Claude Code / Cursor) or HTTP (multi-agent)

**Operational requirements:**
- One long-lived process (daemon)
- Disk for the SQLite graph file (typically tens of MB for 10-20 repos)
- Read access to all indexed repositories (local clones or network mounts)

## Model 2: Multi-Instance with Merkle Sync (20-100+ repos)

Multiple knowing instances, each indexing a subset of repos. Instances exchange graph state via Merkle diff so cross-repo edges resolve across team boundaries.

```
Team Alpha daemon ──> indexes auth-service, user-service
Team Beta daemon  ──> indexes api-gateway, billing-service
Team Gamma daemon ──> indexes data-pipeline, analytics

Sync layer: Merkle diff exchange between instances
            Only changed subtrees transfer
            Cross-repo edges resolve after sync
```

**When to use:** Multiple teams with separate repo ownership. Too many repos for one daemon to index efficiently. Teams want to own their own knowing instances but need cross-team visibility.

**How Merkle sync works:**

1. Each daemon produces snapshots for its repos (content-addressed root hashes)
2. Instances exchange root hashes to detect divergence
3. Only changed subtrees transfer (Merkle diff, same mechanism as git pack negotiation)
4. After sync, a cross-repo resolver pass connects edges whose source and target live in different instances' repos
5. Content-addressed hashes prove consistency without requiring trust between teams

If Team Alpha pushed a change but Team Beta didn't, only Alpha's subtree transfers. The receiving instance verifies the hash chain to confirm integrity.

**Instance ownership registry:**

Each instance needs to know which repos it owns and which repos other instances own. Options:

- Central config file listing instance-to-repo mappings (simplest)
- Derived from CODEOWNERS or a service catalog (self-maintaining)
- Self-registered: each team's CI pipeline announces its repos to a coordinator
- Graph-derived: the ownership edges (`owned_by_team`) in the graph itself can route queries to the right instance

**Cross-repo edge resolution:**

Static analysis within one repo finds `import "github.com/org/other-service/client"`, but the target is indexed by another team's daemon. Two mechanisms:

- **Tier 2 shallow ingest:** Each daemon indexes the public API surface of its dependencies via SCIP indices. Enough to connect cross-repo edges without parsing all transitive source.
- **Post-sync resolution:** After Merkle sync, unresolved edges (source in local repos, target in synced repos) are connected. The content-addressed symbol identity scheme (`{repo}://{path}.{Symbol}`) ensures unambiguous resolution.

## Model 3: CI-Integrated (any scale)

knowing runs in CI pipelines to produce semantic PR diffs and graph-native test selection. The graph file is treated as a build artifact.

```
PR opened
    │
    v
CI job: pull graph artifact from artifact store
    │
    v
knowing index --repo . (index PR branch, incremental against base snapshot)
    │
    v
knowing diff --base <base-snapshot> --head <head-snapshot>
    │
    v
Post PR comment with relationship-level impact
```

**How it works:**
- The graph SQLite file is stored as a build artifact (S3, GCS, GitHub Artifacts, or a shared volume)
- CI pulls the latest graph, indexes the PR branch (incremental, only changed files)
- `semantic_diff` or `pr_impact` computes the relationship-level diff between base and head snapshots
- Result is posted as a PR comment via the GitHub Action

**The graph file as artifact:**

The SQLite file is the portable artifact (architecture decision #15). CI doesn't need a running daemon. It needs the file. The artifact store holds the latest graph per branch or per deploy tag. CI jobs pull it, compute against it, and optionally push an updated graph back.

```yaml
# .github/workflows/knowing-diff.yml
name: Semantic PR Diff
on: [pull_request]
jobs:
  graph-diff:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/download-artifact@v4
        with:
          name: knowing-graph
          path: .knowing/
      - uses: blackwell-systems/knowing-action@v1
        with:
          base: ${{ github.event.pull_request.base.sha }}
          head: ${{ github.event.pull_request.head.sha }}
          graph-db: .knowing/graph.db
          post-comment: true
```

## Runtime Trace Integration

In a microservice organization, OpenTelemetry traces flow through a central collector. knowing taps into the collector to create runtime edges without changing any service code.

```
Service A ──┐                          ┌──> knowing trace ingest pipeline
Service B ──┼──> OTel Collector ───────┤
Service C ──┘                          └──> existing observability (Grafana, Tempo, etc.)
```

**How it works:**

The trace ingest pipeline reads spans from the OTel collector (via OTLP export or a Kafka topic that the collector writes to). Each span describes a service-to-service call. The pipeline:

1. Normalizes spans into source/target pairs
2. Resolves runtime identifiers (service names, route paths, RPC methods) to graph symbol hashes via the route-to-symbol mapping table
3. Creates `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes` edges with observation-based confidence
4. Writes edges to the graph via `GraphStore.PutEdge()` (same interface as static edges, different provenance)

**What this gives teams:**

- "Is this route actually called in production?" (runtime edge exists with recent observations)
- "Static analysis says 47 callers; runtime says 3 are active" (focus migrations on real traffic)
- "This proto field has 0 runtime reads in 90 days" (safe to deprecate)
- Production traffic patterns visible in the same graph as static analysis

**Operational requirements:**

- Access to the OTel collector's export (OTLP endpoint, Kafka topic, or log drain)
- The knowing daemon runs the trace ingest pipeline as a background goroutine
- No changes to application services required (they already emit traces to the collector)

## Cross-Team Semantic PR Diffs

When a developer opens a PR that changes a symbol with cross-repo callers, the CI integration queries the full graph (post-sync in multi-instance mode, or directly in single-instance mode) to show the full impact.

```
PR: change auth-service.Validate signature

knowing pr_impact output:

  Symbols changed: 1
  Cross-repo callers: 3 (api-gateway, billing-service, user-service)
  Teams affected: @gateway-team, @billing-team, @platform-team
  Runtime traffic: 14,000 calls/day from api-gateway, 3/day from billing

  Recommended: notify @gateway-team (high-traffic consumer)
```

The developer didn't have to know who calls their function. The graph knew. The ownership edges identified which teams to notify. The runtime edges identified which consumers actually carry traffic.

## Staleness During Deploys

When a team deploys a breaking change, the graph shows old edges until consumers reindex. Content-addressed staleness detection handles this:

1. Team Alpha deploys a new version of `auth-service`
2. The content hash of `auth-service.Validate` changes in Alpha's snapshot
3. Edges from other repos pointing to the old hash of `Validate` are flagged as stale (hash mismatch)
4. Queries return these edges with a staleness annotation rather than silently returning stale data
5. Consuming teams' daemons reindex (triggered by file watcher or CI), resolving the staleness

This is a structural advantage over mutable-state tools. A mutable graph either shows you the old state (wrong) or the new state (incomplete). knowing shows you the current state with explicit annotations about what's unverified. Agents and humans can make informed decisions.

## Organizational Memory

In a large organization, the knowledge of "service A talks to service B via this route, and team X owns the consumer side" currently lives in:

- Someone's head (lost when they leave)
- A wiki page (stale within a week)
- An incident postmortem (discovered under pressure, not captured systematically)
- Tribal memory (never written down)

knowing makes this structural and queryable:

- **Ownership edges** connect symbols to teams (derived from CODEOWNERS, service catalog, or manual annotation)
- **Runtime edges** show what actually talks to what in production (derived from OTel traces)
- **The event log** records when relationships formed and dissolved (temporal queries)
- **The snapshot chain** preserves the full history (auditable)

When someone leaves, their knowledge of system relationships stays in the graph. When an incident happens at 3 AM, the on-call engineer can query the graph instead of guessing. When a new team member joins, they can explore the graph to understand how their service fits into the system.

## Deployment Summary

| Scale | Model | Graph location | Cross-repo edges | Runtime traces |
|-------|-------|---------------|-----------------|---------------|
| 1-5 repos | Single daemon, local | SQLite on developer machine | Immediate (one graph) | Optional, local OTel |
| 5-20 repos | Single daemon, shared | SQLite on shared VM/server | Immediate (one graph) | OTel collector tap |
| 20-100 repos | Multi-instance + sync | SQLite per instance, Merkle sync | Post-sync resolution | Central OTel collector |
| 100+ repos | Multi-instance + sync + CI | SQLite as build artifact | Post-sync + CI integration | Central OTel collector |
