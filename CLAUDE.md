# knowing

Self-adapting code intelligence engine. 101K LOC Go. Single binary, zero runtime deps.
Gets smarter with scale, not dumber: observes its own graph density and adjusts retrieval
strategy automatically.

## Build & Test

```bash
GOWORK=off go build ./...           # build (GOWORK=off required: go.work refs missing module)
GOWORK=off go test ./internal/...   # unit tests
GOWORK=off go test ./cmd/...        # CLI tests
GOWORK=off go test ./bench/...      # benchmark harnesses (some need pre-indexed repos)
```

## Benchmark (P@10 evaluation)

```bash
# Full cross-system benchmark (167 tasks, 9 repos, ~5 min)
GOWORK=off BENCH_ADAPTERS=knowing go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m

# Single repo (fast iteration)
BENCH_REPOS=flask BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 10m

# Diagnostic env vars (compose freely, no reindex needed):
BENCH_EXCLUDE_EDGES=similar_to,type_hint_of   # exclude edge types from RWR walk
BENCH_BFS_DEPTH=2                             # limit walk depth (default 4)
BENCH_PREFER_TYPE_SEEDS=1                     # force type-seed preference
BENCH_HUB_DAMPEN=50                           # penalize nodes with in-degree >50
```

## When Benchmark Numbers Change

After any P@10 improvement, these files ALL need updating with the new aggregate,
per-repo breakdown, and competitive ratios. This is a standard procedure:

1. **bench/cross-system/FINDINGS.md** — executive summary, per-repo table, competitive advantages
2. **bench/CONTEXT-PACKING-STUDY.md** — Dimension 1, competitive summary, run history
3. **bench/README.md** — cross-system row in summary table
4. **docs/guide/introduction.md** — operational characteristics, measured performance table
5. **docs/architecture/retrieval-pipeline.md** — eval baseline line
6. **docs/architecture/system-overview.md** — benchmark section
7. **docs/architecture/design-principles.md** — benchmark results
8. **docs/architecture/context-engine.md** — current performance
9. **docs/roadmap.md** — retrieval pipeline section
10. **Blog post** (`/Users/dayna.blackwell/code/blog/content/posts/ai-code-context-tools-benchmark.md`) — headline table, per-repo, ratios, parameter sweep values, "Where We Lose" section
11. **npm/knowing/README.md** — package description on npmjs.com
12. **pypi/README.md** — package description on PyPI

Competitive ratios to recalculate from new P@10:
- vs codegraph: P@10 / 0.135
- vs GitNexus: P@10 / 0.075
- vs Gortex: P@10 / 0.063
- vs grep: P@10 / 0.013

## Key Architecture

- `internal/context/` — retrieval pipeline (RWR, HITS, RRF, density-adaptive seeding, concept thesaurus)
- `internal/context/walk.go` — RWR implementation, BFS adjacency map, ExcludeEdgeTypes, BFSMaxDepth, PreferTypeSeeds, HubDampeningThreshold, GraphNodeCount (all package-level vars)
- `internal/context/concept_thesaurus.go` — BM25 keyword expansion (~80 domain clusters)
- `internal/indexer/` — 17 extractors (tree-sitter), post-processing pipeline (inheritance, interface propagation, contains, similarity, co-tested, type-hint)
- `internal/indexer/indexer.go` — IndexRepo pipeline, --edge-types filter, GenerateCoTestedEdges, IsTestFile
- `internal/store/` — SQLite backend (GraphStore interface, NodesByFileHash)
- `internal/edgetype/` — 34 edge type constants
- `internal/snapshot/` — hierarchical Merkle tree (via merkle-strata library)
- `internal/mcp/` — MCP server (28 tools, 8 resources)
- `internal/enrichment/` — LSP enrichment (multi-module gopls, per-symbol timeout, progress persistence)
- `bench/cross-system/` — competitive benchmark (167 tasks, 9 repos, 7 competitors)
- `bench/cross-system/adapters/knowing.go` — bench adapter (ensureContainsEdges, ensureCoTestedEdges, GraphNodeCount caching)

## Current State

- **P@10 = 0.207** (167 tasks, 9 repos, 6 languages, 36 edge types)
- **Density-adaptive:** auto-enables PreferTypeSeeds when GraphNodeCount > 40K
- **Competitive:** 1.53x codegraph, 2.76x GitNexus, 3.29x Gortex, 15.9x grep
- **Identity:** "self-adapting code intelligence engine that gets smarter with scale"

## Key Findings (inform all future retrieval work)

1. **P@10 is reachability-determined.** 32-config parameter sweep proved zero variance. Only new edges or new seed sources move the metric. Don't tune weights.
2. **Dense graph dilution is a seed selection problem.** Edge exclusion, BFS depth, hub dampening all tested neutral. The fix is density-adaptive seed selection (PreferTypeSeeds).
3. **Correct extraction can hurt precision.** TS export_statement fix (43K->87K nodes) dropped VS Code from 0.163 to 0.084. PreferTypeSeeds recovered it to 0.137.
4. **Enrichment hurts retrieval.** LSP enrichment adds correct edges but dilutes RWR. Useful for audit, harmful for retrieval.
5. **The concept thesaurus helps messaging/concurrency domains.** Framework-specific expansions ("backend"->"base") hurt.
6. **Struct field access edges are neutral for P@10.** Fields are already reachable via call edges. But they improve graph completeness (blast radius, test-scope, field-level impact).

## Conventions

- Always use `GOWORK=off` (go.work references shelfctl which may not be present)
- Run benchmark before AND after shipping any retrieval/engine changes
- Do NOT use em dashes in prose or documentation
- Use `command npm` to bypass nvm shell hook
- Check CI: `gh run list --limit 5`
- Commit messages: conventional commits (feat:, fix:, docs:)
- Do not commit CLAUDE.md to git (it's in .gitignore)

## Next Priorities

1. Local embeddings / pure Go inference (docs/proposals/pure-go-embeddings.md)
2. Rust/C# field access extraction (extend accesses_field to other languages)
