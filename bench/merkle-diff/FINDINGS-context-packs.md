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

## Graph Size

- Nodes: 2903
- Edges: 14821
