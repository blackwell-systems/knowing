# Semantic PR Diff

knowing generates a relationship-level diff for pull requests: not what text changed, but what the change does to the system graph. This is exposed as MCP tools, a CLI command, and a CI integration (GitHub Actions workflow).

## Why

Code review today is text review. A reviewer sees that 40 lines changed in `auth/middleware.go` and makes a judgment about blast radius based on experience and intuition. They might grep for callers, or they might not. They almost certainly do not check cross-repo impact.

Semantic PR diff makes relationship impact visible without effort. It answers the questions reviewers should ask but often do not: Does this change add new cross-repo dependencies? Does it increase the blast radius of a critical function? Does it affect symbols owned by other teams?

This is the most visible feature knowing can ship. Developers see it on every PR. It demonstrates the value of the graph without requiring anyone to change their workflow or learn a new tool.

## Output Format

The `knowing diff` command produces a graph-level summary:

```
knowing diff --base main --head feature/auth-refactor

  Graph impact for PR #482: refactor auth middleware

  Symbols changed: 4
  Edges added:     3
  Edges removed:   1
  Edges modified:  2

  +  auth-service -> user-service.GetUser (calls, confidence 1.0)
     New cross-repo dependency. user-service is owned by @platform-team.

  +  auth-service -> billing-service.ValidateSubscription (calls, confidence 1.0)
     New cross-repo dependency. billing-service is owned by @billing-team.

  +  auth-service -> notification-service.SendAlert (calls, confidence 0.8)
     New cross-repo dependency (inferred from import, no direct call site found).

  -  auth-service -> legacy-session-store.Lookup (calls, confidence 1.0)
     Cross-repo dependency removed.

  ~  AuthMiddleware.Validate blast radius: 12 callers -> 47 callers
     Gained 35 transitive callers via new edges to user-service and billing-service.

  ~  AuthMiddleware.TokenRefresh signature changed
     8 direct callers across 3 repos. 2 callers are in repos not owned by PR author.

  Ownership impact:
     Before: consumers in 1 team (@auth-team)
     After:  consumers in 3 teams (@auth-team, @platform-team, @billing-team)

  Staleness:
     2 edges in the blast radius were last verified > 14 days ago.
     Run `knowing index --repo github.com/org/billing-service` to refresh.
```

## How It Works

```
1. PR opened (or push to PR branch)
         |
         v
2. knowing indexes the PR branch, producing a new snapshot
         |
         v
3. Merkle diff between base snapshot and PR snapshot
   (only changed subtrees are traversed)
         |
         v
4. For each changed edge:
   - Classify: added, removed, modified
   - Look up ownership for affected symbols
   - Compute blast radius delta (before vs. after)
         |
         v
5. Format and post as PR comment or check annotation
```

The Merkle diff (via `DiffHierarchicalTrees` in `internal/snapshot/hierarchical.go`) compares package roots first and only descends into edge-type roots for packages that changed. This makes the diff fast even for large graphs.

**Removed-edge correctness:** Migration 013 (`add_edge_event_data.sql`) added `source_hash`, `target_hash`, `edge_type`, `confidence`, and `provenance` columns to `edge_events`. `SnapshotDiff` uses `COALESCE` to read from the event record first, falling back to the edges table for pre-migration events. Removed-edge diffs return full edge data, not just hashes.

## Implementation

The implementation lives in `internal/diff/`:

- `semantic.go`: `SemanticDiff` computes the relationship-level diff between two snapshots. Classifies edges as added, removed, or modified. Annotates with ownership and blast radius delta.
- `impact.go`: `ImpactAnalysis` computes per-symbol blast radius before and after, identifying new and lost transitive callers.
- `types.go`: `SemanticDiffResult`, `EdgeChange`, `BlastRadiusDelta`, `OwnershipDelta` types.
- `ci.go`: CI-specific helpers for the GitHub Actions integration.

Key types:

```go
type SemanticDiffResult struct {
    BaseSnapshot    Hash
    HeadSnapshot    Hash
    SymbolsChanged  int
    EdgesAdded      []EdgeChange
    EdgesRemoved    []EdgeChange
    EdgesModified   []EdgeChange
    BlastRadiusDelta []BlastRadiusDelta
    OwnershipImpact  *OwnershipDelta
    StaleEdges       []Edge
}

type EdgeChange struct {
    Edge        Edge
    SourceRepo  string
    TargetRepo  string
    CrossRepo   bool  // true if source and target are in different repos
    OwnerTeam   string
}

type BlastRadiusDelta struct {
    Symbol        Node
    CallersBefore int
    CallersAfter  int
    NewCallers    []Node
    LostCallers   []Node
}

type OwnershipDelta struct {
    TeamsBefore []string
    TeamsAfter  []string
    NewTeams    []string // teams newly affected by this change
}
```

## MCP Tools

Three MCP tools expose semantic diff functionality to agents:

| Tool | Purpose |
|------|---------|
| `snapshot_diff` | Raw edge-level diff between any two snapshot hashes |
| `semantic_diff` | Relationship-level diff with ownership and blast radius annotations |
| `pr_impact` | Semantic diff specialized for a PR: resolves base/head from git, formats for review |

Agents use `pr_impact` before committing to verify a change does not introduce unexpected cross-repo dependencies or blast radius growth.

## CLI Command: `knowing audit-diff`

`knowing audit-diff` is the CLI equivalent of the CI workflow. It computes the semantic diff between the current working tree and a base ref:

```bash
# Diff against main
knowing audit-diff --base main

# Diff between two specific commits
knowing audit-diff --base abc123 --head def456

# JSON output for programmatic use
knowing audit-diff --base main --format json
```

The output format matches the PR comment format. The command exits with a non-zero status if thresholds are exceeded (configurable via flags).

## CI Integration

`.github/workflows/pr-semantic-diff.yml` implements the GitHub Actions integration. It runs on every PR against `main`:

1. Checks out the repo with full history (`fetch-depth: 0`).
2. Builds the `knowing` binary.
3. Indexes the base branch commit into `base.db`.
4. Indexes the head branch commit into `head.db`.
5. Merges base graph data into `head.db` (so `SnapshotDiff` has both snapshots in one database).
6. Runs `knowing diff` to produce `diff-result.json`.
7. Posts or updates a PR comment with the diff summary (nodes added/removed/modified, edges added/removed, with formatted lists truncated at 20 nodes and 15 edges).

The workflow uses `GOWORK=off` to isolate module resolution during CI indexing.

## What This Does Not Do

- Does not block PRs by default. The diff is informational. Teams can configure thresholds in `knowing audit-diff` flags to enforce constraints, but the default is comment-only.
- Does not replace code review. It augments it with information reviewers cannot easily get on their own.
- Does not require a running daemon in CI. The GitHub Action builds a fresh `knowing` binary and operates on temporary database files created during the job.

## Retrofit Cost

Low. Semantic diff is a read-only consumer of the snapshot chain and Merkle diff. It can be added at any time after `SnapshotDiff` is implemented. The key prerequisite is migration 013: without full edge data in `edge_events`, removed-edge diffs return incomplete information.
