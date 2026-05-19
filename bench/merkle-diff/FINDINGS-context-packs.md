# Context Pack and Community Root Benchmark

Validates content-addressed context packs and community Merkle roots on the live knowing graph.

## Context Pack Roots

- **Deterministic:** same task + same graph = same PackRoot (verified)
- **Distinct:** different tasks produce different PackRoots (verified)
- **Dedup potential:** 5 queries with 2 unique tasks produce 2 unique PackRoots

PackRoot enables:
- Cache lookup: if PackRoot matches a cached result, skip retrieval entirely
- Citation: agents can reference a PackRoot instead of resending content
- Cross-session replay: same task against same graph state = same context

## Community Merkle Roots

- Each package produces a distinct SubgraphRoot (verified for mcp, context, store)
- Combined package sets produce distinct roots (verified)
- Disjoint community roots prove safe parallelization

Community roots enable:
- Scoped invalidation: "auth community root changed, invalidate auth caches"
- Agent coordination: "these two agents edit disjoint communities, safe to parallelize"
- Retrieval scoping: "restrict walk to seeded community unless bridge edges score high"

## Context Pack Persistence (P2)

Three-layer cache verified on live graph:

| Layer | Latency | Survives restart |
|-------|---------|-----------------|
| SubgraphCache (in-memory) | 42ns | No |
| Notes table (SQLite) | ~1.2ms | Yes |
| Cold retrieval | ~1.3ms | N/A |

- **Cross-session replay verified:** fresh engine (simulating restart) returns
  identical PackRoot and symbol set from persisted notes.
- **Staleness detection:** stored snapshot hash compared against current latest;
  stale packs recomputed automatically.
- **Pack size:** 154KB for 197 symbols at 50K token budget.
- On the knowing graph (~2,500 nodes), cold retrieval is already fast (~1.3ms),
  so persistence shows 1.0x latency speedup. Value is correctness (cross-session
  replay) and scales with graph size.

## Context Pack Deduplication (P5)

Agent passes `pack_root` from prior call; gets "unchanged" (26 tokens) instead
of full context payload.

| Task | Full response | Unchanged | Savings |
|------|--------------|-----------|---------|
| Small (15 symbols, 5K budget) | 557 tokens | 26 tokens | 95% |
| Medium (205 symbols, 20K budget) | 7,661 tokens | 26 tokens | 100% |
| Large (80 symbols, 50K budget) | 3,124 tokens | 26 tokens | 99% |

- **PackRoot determinism verified:** same task twice = same PackRoot.
- **Performance contract:** >90% token savings on dedup hit (actual: 95-100%).
- **Estimated session savings:** 5 calls/session * 3,754 avg = ~18,800 tokens.
- Token estimates use len/4 approximation (4 chars per token).

## Graph Size

- Nodes: 2492
- Edges: 12355
