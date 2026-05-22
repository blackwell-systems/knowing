# Benchmarks

Fourteen benchmark harnesses that prove knowing's value with hard data. Each benchmark
is a standalone Go test package that indexes the knowing repo, runs measurements,
and auto-generates a `FINDINGS.md` with results and interpretation.

**Context Packing Study:** [CONTEXT-PACKING-STUDY.md](CONTEXT-PACKING-STUDY.md) (umbrella document tying all benchmarks into a coherent evaluation program)
**Cross-System Specification:** [docs/research/cross-system-benchmark.md](../docs/research/cross-system-benchmark.md) (full methodology, 6 systems, fairness controls, ground truth protocol)

## Summary

| Benchmark | What it proves | Key result |
|-----------|---------------|------------|
| [cross-system](cross-system/) | Graph retrieval beats text search across languages and scales | P@10=0.147 vs grep 0.016 (9.2x, p<0.0001, d=0.67 on recall) |
| [feedback-loop](feedback-loop/) | Feedback compounding improves precision over time | 16% -> 36% precision (+20pp) after one round |
| [context-relevance](context-relevance/) | Each engine layer adds measurable value | Feedback adds +9pp precision over baseline |
| [token-savings](token-savings/) | knowing reduces agent exploration cost | 55.6% fewer tokens, 52.8% fewer tool calls |
| [edge-accuracy](edge-accuracy/) | Two-tier extraction provides meaningful signal | 53.6% import confirmation, 26.7% overall |
| [test-scope-accuracy](test-scope-accuracy/) | Call-graph BFS predicts affected tests | 98.9% precision vs independent Go import DAG |
| [wire-format](wire-format/) | GCF is dramatically more token-efficient than JSON | 84% token savings, 74% byte savings |
| [merkle-diff](merkle-diff/) | Hierarchical Merkle tree enables scoped invalidation; context pack determinism and community root distinctness | 216x faster diff on real graph (~24.9K edges), 517x on 100K synthetic edges, 59ns subgraph root lookups; 5 queries, 2 unique tasks = 2 unique PackRoots (perfect dedup) |
| [community-detection](community-detection/) | Incremental detection skips work the Merkle tree proves unchanged | Louvain 6.9x faster (1 pkg), LP 38.4x faster (1 pkg), delta-save 5.0x e2e |
| [merkle-diff](merkle-diff/) (P2 persistence) | Context packs survive process restart via notes table | Cross-session PackRoot match verified, 154KB persisted, snapshot staleness detection |
| [merkle-diff](merkle-diff/) (P5 dedup) | Agents skip retransmitting unchanged context | 95-100% token savings (557-7,661 -> 26 tokens) |
| [merkle-diff](merkle-diff/) (proof) | Merkle proof generation and verification (Phase 4) | Proves relationship existed in a given snapshot |
| [merkle-diff](merkle-diff/) (scoped FTS) | Scoped FTS rebuild vs full rebuild (Phase 3 F3/P4) | Package-scoped rebuild avoids full-table reindex |

## Running

```bash
# Run all benchmarks (takes ~60s):
GOWORK=off go test ./bench/... -timeout 5m

# Run a specific benchmark with verbose output:
GOWORK=off go test ./bench/feedback-loop/ -v -count=1

# Skip slow benchmarks in quick iteration:
GOWORK=off go test ./bench/... -short
```

All benchmarks index the live knowing repo from the working directory. Results
vary slightly as the codebase evolves.

## Design Principles

1. **Self-contained.** Each benchmark creates a temp database, indexes the repo,
   runs measurements, and cleans up. No external state or pre-existing database.

2. **Auto-generated findings.** Each test writes its own `FINDINGS.md` with
   current numbers. Run the test to refresh the report.

3. **Independent ground truth.** Benchmarks compare knowing's output against
   independent data sources (Go import graph, go/ast type resolution, manual
   ground truth fixtures) rather than circular self-validation.

4. **Honest interpretation.** FINDINGS.md documents what the data shows and
   what it does not. Limitations and caveats are stated explicitly.

## Benchmark Details

### feedback-loop

Proves the shared intelligence layer thesis: feedback anchored to content-addressed
symbol hashes compounds over sessions, scopes by community, and expires naturally
on rename.

- 4 tests: single-round, multi-round (5 rounds), community scoping, natural expiration
- 5 task fixtures with hand-curated ground truth (8 symbols each)
- Centered feedback scoring: `0.15 * (2*score - 1.0)`

### context-relevance

A/B comparison of 3 engine configurations across 10 task fixtures:
- Config A: keyword seeds only (Distance == 0)
- Config B: full engine (RWR + HITS + all 5 seed tiers)
- Config C: full engine + accumulated feedback

Shows that feedback is the strongest enhancement for precision at current repo scale,
while HITS/RWR provides score differentiation that matters more on larger repos.

### integrity (new in 2026-05-18 session)

Validates the `knowing fsck` integrity checker and hash domain prefix correctness. Indexes the repo, verifies all node and edge hashes using `VerifyNodeHash`/`VerifyEdgeHash`, checks edge referential integrity, and confirms snapshot chain continuity. Confirms that the `node\0`, `edge\0`, `snapshot\0`, and `merkle\0` prefixes are present and consistent across all stored rows.

### token-savings

Simulates agent workflows: for 5 task scenarios, measures how many grep/read tool
calls an agent would need without knowing vs one `context_for_task` call with knowing.
Estimates token cost per path.

### edge-accuracy

Indexes the repo twice (tree-sitter and go/ast) and compares edge sets. Reports
per-edge-type accuracy with a fair comparison restricted to edge types both
extractors attempt (calls + imports). Validates the two-tier speed/accuracy tradeoff.

### test-scope-accuracy

For each of the last 20 commits, predicts affected test packages via call-graph BFS
and compares against Go's import DAG (`go list -deps -test`) as independent ground
truth. Skips gracefully on shallow clones (CI).

### wire-format

Measures GCF (token-optimized) and GCB (byte-optimized) against JSON across 6
fixture payloads. Verifies round-trip integrity, monotonic improvement (GCF never
worse than JSON), and p99 encode latency < 1ms.

### merkle-diff (Phase 2 extension)

Benchmarks hierarchical vs flat Merkle tree operations on the live knowing graph.
Indexes the repo, collects all edges with package and edge-type metadata, mutates
one package, and measures diff performance. Validates that hierarchical diffs are
O(packages) instead of O(edges), subgraph root lookups are O(1), and the build
cost overhead is negligible. Also verifies correctness: the diff correctly identifies
which packages and edge types changed.

The `context_pack_test.go` suite (Phase 2 Merkle) extends the harness with two
additional proofs: (1) `PackRoot` determinism: 5 queries with 2 unique tasks
produce exactly 2 unique PackRoots (perfect dedup, verified on the live graph);
(2) community root distinctness: each Louvain community receives a distinct
Merkle root based on the packages it spans. Results are written to
`FINDINGS-context-packs.md`.

### community-detection

Proves that incremental community detection (Phase 3 F2) delivers real speedups
by exploiting the Merkle tree's change information. When the daemon knows which
packages changed (from `DiffHierarchicalTrees`), it can freeze all unchanged
nodes and only allow changed nodes to move during community optimization.

The benchmark indexes the live repo, runs full detection, then re-runs with only
`internal/store` nodes marked as changed (5.9% of the graph). This simulates the
common case: a developer edits files in one package, the daemon re-indexes, and
community assignments need updating.

- Louvain: 2.99ms full, 436us incremental (6.9x speedup), 375us frozen (8.0x)
- Label propagation: 14.3ms full, 372us incremental (38.4x), 180us frozen (79.2x)
- LP benefits more because its iteration cost is O(N * iterations); freezing 94%
  of nodes cuts 94% of the work. Louvain converges in fewer passes, so the
  per-iteration savings are proportionally smaller.
- Correctness verified: incremental with 0 changes produces identical assignments
  to full detection (bit-for-bit match on all 2,472 nodes).

The end-to-end chain (Phase 3 complete): file edit -> re-index -> hierarchical
diff -> `ChangedPackages` -> load previous assignments from notes table ->
`DetectIncremental(changedNodes)` -> store new assignments. The benchmark proves
the middle step; the wiring depends on P1 (community assignment persistence in
notes table).
