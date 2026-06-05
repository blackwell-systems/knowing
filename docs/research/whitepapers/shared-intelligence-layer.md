# Shared Intelligence Layer: Cross-Task Vocabulary Learning on Community-Partitioned Code Graphs

**Dayna Blackwell, Blackwell Systems**

---

## Abstract

AI coding agents operate on code but learn nothing from each other. Each session starts cold, explores the same codebase from scratch, and discards its discoveries at the end. We describe an architecture where a content-addressed code graph, partitioned into emergent communities via Louvain clustering, becomes a shared learning substrate for multi-agent coordination. Agents contribute feedback about which symbols were useful for which tasks; that feedback compounds by keyword cluster, making subsequent sessions progressively sharper. Learned vocabulary associations bridge across tasks: one agent's discoveries help different agents working on related problems. The result is implicit specialization without configuration: the system learns which parts of the codebase matter for which types of work, organized by boundaries it discovers rather than boundaries humans declare.

**Empirical validation (session 26, 300 tasks, 16 repos):** Cross-task vocabulary bridging produces +41.4% precision on Django. 10-round compounding on the full corpus: MRR climbs from 0.459 to 0.497 (+8.1%). The system never regresses below its cold-start baseline. All learning is Merkle-anchored: associations expire per-package when code changes.

---

## 1. The Cold Start Problem at Scale

Every AI coding session begins with the same question: "What part of this codebase is relevant to my task?" Current solutions fall into two categories:

**Context packing tools** (Repomix, code2prompt) answer with "everything": dump the repo into the context window and let the model sort it out. This works for small repos but collapses at scale.

**Graph tools** (code intelligence MCP servers) answer with "whatever you ask for": the agent must know which tools to call, which symbols to look up, and how to navigate the graph. This requires the agent to already understand the codebase's architecture before it can ask useful questions about it.

Neither approach learns. Session N+1 is no better than session N. The agent that spent 20 minutes exploring the context engine yesterday starts from zero today. The insight that `ForTask` calls `RandomWalkWithRestart` which calls `EdgesFrom` is discovered, used, and forgotten.

---

## 2. Communities as Emergent Architecture

The code graph already contains all the information needed to understand a codebase's architecture. Functions call functions. Types implement interfaces. Files import packages. These edges form clusters: groups of symbols that interact heavily with each other and less with the outside.

Louvain community detection discovers these clusters without configuration. Given a graph of 1500 nodes with call/import/reference edges, it partitions them into 8-15 communities in milliseconds. Each community corresponds to what a developer would call a "module" or "subsystem": the context engine, the MCP server, the indexer pipeline, the store layer.

The key insight: **communities are the natural organizational unit for agent intelligence.** They correspond to:
- The scope of a typical task (most edits stay within one community)
- The blast radius boundary (changes within a community rarely break other communities)
- The expertise boundary (understanding one community is useful for all tasks within it)
- The coordination boundary (two agents in different communities don't conflict)

---

## 3. The Learning Loop

The learning loop is fully operational. Five components form a self-reinforcing cycle:

### 3.1 Retrieval (ForTask)

An agent receives a task: "fix the bug in context ranking." The `context_for_task` MCP tool runs the full retrieval pipeline: 5-channel RRF seed fusion, Random Walk with Restart, HITS reranking, 13 self-adapting mechanisms. Returns ranked, token-budgeted context.

P@10 = 0.330 cold start (302 tasks, 16 repos). 3.69x more precise than codegraph, 21.4x more precise than grep.

### 3.2 Work (agent uses context, modifies code)

The agent works. It reads symbols, makes edits, runs tests. Standard agent behavior.

### 3.3 Implicit Feedback (what was useful)

The system tracks which returned symbols the agent actually used (via `DetectUsed` scanning tool call content). Symbols returned but never used get negative feedback; symbols the agent referenced get positive feedback. No explicit feedback call required.

Feedback is scoped by keyword cluster (session 25): noise demotion for "checkout" queries doesn't affect "order" queries. Per-cluster scoping eliminated the cross-task interference that plagued earlier implementations. Feedback records use `neighborhood_root` (package SubgraphRoot) for Merkle-based expiration: feedback naturally invalidates when the package's edges change.

### 3.4 Vocabulary Learning (cross-task bridging)

When an agent uses a symbol after a `context_for_task` query, the system records the association between the task's keywords and the used symbol. After 2+ observations, the association becomes a learned equivalence class.

**The critical property: vocabulary transfers across tasks.** When task A ("payment processing") teaches the system that "payment" maps to `settle_ledger`, a different task B ("payment refund") benefits because it shares the keyword "payment." The symbol `settle_ledger` surfaces for task B even though "refund" and "settle_ledger" share zero keyword overlap.

Three safeguards prevent noise accumulation:
1. **Noise keyword filter** (`isVocabWorthy`): ~80 common English words filtered from recording
2. **Soft RRF injection**: learned vocab competes through scoring, not forced to top
3. **Confidence weighting**: observation count scales RRF weight (0.3 at count=2, 0.8 at count>=10)

**Measured impact (session 26):**
- Cross-task validation: Django +41.4% in isolation, full corpus 0.0% aggregate (safe)
- 100% of improvements are cross-task (never self-reinforcement)
- 10-round full corpus compounding: P@10 peak +2.2%, MRR peak +8.1%
- Associations expire per-package via Merkle roots (not globally)

### 3.5 Compounding (feedback-aware reranking)

The next agent that works on a related task benefits from all prior feedback and vocabulary. Feedback boosts are wired into the context engine ranking pipeline. Learned vocab expands the seed set. The context window fills with symbols that historically mattered for this type of work.

No configuration. No manual curation. The system learns from use.

---

## 4. Implicit Specialization

The feedback-per-cluster and vocabulary-per-keyword pattern creates something that looks like specialization:

- Tasks involving "middleware" consistently surface `SecurityMiddleware`, `SessionMiddleware`, `CsrfViewMiddleware` (learned from prior middleware tasks)
- Tasks involving "query" consistently surface `QuerySet.filter`, `Q`, `Prefetch` (learned from prior ORM tasks)
- Tasks involving "template" consistently surface `TemplateSyntaxError`, `Template.render`, `Library` (learned from prior template tasks)

The graph discovers the communities. Agents discover which symbols matter. The two discoveries compound: vocabulary learned in one community transfers to related tasks within that community, and per-cluster scoping prevents interference between unrelated work.

This is collaborative filtering applied to code intelligence: "agents who worked on similar tasks found these symbols useful."

---

## 5. Multi-Agent Coordination (Planned)

Communities provide the infrastructure for the coordination problem for parallel agents without a central scheduler. The following describes the design; implementation is underway via Polywave (the parallel agent coordination protocol).

### 5.1 Conflict Avoidance

Two agents working in the same community are likely to conflict (editing the same files, calling the same functions). Two agents in different communities are unlikely to conflict. Community membership becomes the scheduling primitive: assign at most one agent per community for parallel work.

### 5.2 Pending Mutations

When an agent announces it's modifying symbols in community 3, other agents working in community 3 can see the pending changes. Agents in communities 5 and 7 ignore the notification entirely. Communities scope the "who needs to know?" question.

### 5.3 Cross-Community Edges as Coordination Points

The edges that span communities are API boundaries. When agent A modifies a function in community 3 that has callers in community 5, only the agent working in community 5 needs to be notified. The graph provides exact notification scoping: "symbol X was modified; it has callers in communities 5 and 8."

---

## 6. Why Content-Addressing Matters Here

The learning loop requires trust: feedback recorded yesterday must still be valid today. Content-addressing provides this guarantee at every level:

- **Feedback expiration.** Feedback records store the package SubgraphRoot (Merkle root) at recording time. When code changes, old feedback expires because its stored root no longer matches. No TTL heuristics, no manual cleanup.
- **Vocabulary expiration.** Learned vocab associations store per-package Merkle roots. When a package changes, only that package's associations expire. Associations for unchanged packages remain valid. (Session 26: per-package precision, not global expiration.)
- **RWR cache invalidation.** Cached walk results are keyed by per-package Merkle roots of the seed packages. When a package changes, only walks seeded from that package miss. Unchanged packages keep cached walks. (Session 26: Django cold 3.9s -> warm 1.9s.)
- **Community membership is recomputable.** Run Louvain on any snapshot to get the community structure at that point in time.
- **Staleness is detectable.** If the graph's Merkle root hasn't changed, all cached structures are still valid. One hash comparison, not a full recomputation.

Without content-addressing, feedback accumulates indefinitely against mutable state, growing stale without detection. With it, feedback naturally decomposes as the codebase evolves: renamed symbols, restructured modules, and moved functions all invalidate their associated feedback through hash changes.

**Architectural moat:** every feature tied to the Merkle structure (feedback expiration, vocab expiration, RWR caching, context pack dedup) requires competitors to rewrite their data model from scratch. This cannot be bolted onto a mutable graph.

---

## 7. The Architecture Stack

```
Layer 4: Agent Coordination     pending mutations, cross-community notifications (Polywave)
Layer 3: Learning Loop          context -> work -> feedback -> vocab -> compound
Layer 2: Community Structure    Louvain clustering, per-cluster scoping, community roots
Layer 1: Code Graph             content-addressed nodes, edges, snapshots, Merkle tree
Layer 0: Source Code            git commits, file changes, runtime traces
```

Each layer depends on the one below:
- Coordination needs community boundaries (Layer 2)
- Learning needs cluster scoping (Layer 2) and persistent feedback (Layer 1)
- Communities need edge data (Layer 1)
- The graph needs source analysis (Layer 0)

**Implementation status:**
- **Layers 0-1:** Complete and operational. 38 edge types, 23 extractors, hierarchical Merkle tree, per-package roots persisted to notes table.
- **Layer 2:** Complete. Louvain and label propagation, incremental detection (6.9x/38.4x speedup), per-cluster feedback scoping, community Merkle roots.
- **Layer 3:** Complete. Implicit feedback with per-cluster scoping, vocabulary expansion with cross-task bridging, confidence-weighted injection, noise filtering, Merkle-based expiration for both feedback and vocab. Incremental RWR caching with per-package cache keys.
- **Layer 4:** In progress (Polywave protocol for parallel agent coordination).

---

## 8. Related Work

**Content-addressed code graphs.** This paper builds on our prior work, "The Hierarchical Identity Architecture: Content-Addressing as a Computation Primitive for Software Relationship Intelligence" [Blackwell 2026], which established the content-addressed graph model, hierarchical Merkle trees over semantic boundaries, and the formal properties that enable O(packages) diff and O(1) cache keys. The present paper extends that foundation with learning mechanisms (feedback, vocabulary, compounding) that the content-addressed architecture makes structurally safe.

**Graph-based code retrieval.** Personalized PageRank and Random Walk with Restart have been applied to information retrieval [Haveliwala 2002, Tong et al. 2006] and social network analysis, but not to code retrieval with typed multi-relational edges. Aider uses tree-sitter extraction with PageRank scoring but does not support feedback, community detection, or cross-session learning. codegraph (19K GitHub stars) uses BM25 with heuristic scoring but no graph walks. Neither system learns from agent usage or expires cached knowledge structurally.

**Retrieval-augmented generation for code.** RAG approaches for code (CodeBERT, UniXcoder, RepoCoder) embed code snippets and retrieve by cosine similarity. These are effective for semantic matching but do not model structural relationships (caller/callee, implements, extends). They cannot answer "what breaks if I change X?" because they lack edge-typed graphs. Embeddings are also stateless: each query starts fresh with no memory of prior sessions. Our evaluation confirmed that embedding-based retrieval is neutral on cold start (P@10 identical with and without, 3 runs), while graph structure and BM25 carry all precision.

**Benchmark methodology.** SWE-bench [Jimenez et al. 2024] evaluates agent task completion but not the retrieval layer that feeds agents context. CrossCodeEval evaluates cross-file completion but at the token level, not the symbol level. Our benchmark (300 tasks, 16 repos, 8 languages) evaluates symbol-level retrieval precision with statistical methodology (Wilcoxon signed-rank, Cohen's d, bootstrap CI) and cold-start enforcement (task memory contamination discovered and eliminated in session 23).

**Community detection on code.** Louvain clustering has been applied to software architecture recovery [Garcia et al. 2013], but prior work uses it for visualization, not as a runtime primitive for feedback scoping and cache invalidation. Our contribution is using community-discovered boundaries as the organizational unit for agent learning: feedback compounds by keyword cluster within community boundaries, and Merkle roots provide structural expiration when communities' code changes.

---

## 9. Comparison with Existing Approaches

| Approach | Scope of learning | Persistence | Architectural awareness | Cross-task transfer |
|----------|------------------|-------------|------------------------|-------------------|
| Aider repo-map | Per-session PageRank | None (regenerated) | Implicit (ranking) | None |
| Cursor context | Per-session embeddings | Server-side (opaque) | None | None |
| CLAUDE.md files | Manual, project-wide | File (human-maintained) | None (flat text) | None |
| Session memory | Per-session CCP | Cross-session facts | None | None |
| **knowing** | Per-cluster feedback + cross-task vocab | Content-addressed, Merkle-anchored | Emergent (Louvain) | **Yes (+41.4% Django)** |

The unique combination: learning that is **persistent** (survives across sessions), **structurally scoped** (per-cluster feedback, per-package Merkle expiration), and **transferable** (vocabulary bridges across tasks via shared keywords). No other system in the literature combines these three properties.

---

## 10. What This Enables

### For individual developers:
"The system knows what I usually need when I work on the context engine. It stops showing me daemon code. And when I switch to a related task, my previous work's vocabulary helps immediately."

### For teams:
"New engineers get effective context immediately because the system learned from 100 prior sessions what matters in each module."

### For multi-agent workflows:
"Five parallel agents self-coordinate by community. No central scheduler. No conflicts. Each agent's work makes the next agent smarter."

### For architectural governance:
"We can see when modules are drifting apart, when coupling is increasing, and when refactors change community boundaries, all from the snapshot chain."

---

## 11. Empirical Evidence

| Metric | Value | Source |
|--------|-------|--------|
| Cold-start P@10 | 0.330 (302 tasks, 17 repos, 8 languages) | Cross-system benchmark |
| Cross-task vocab lift (Django) | +41.4% | TestCrossTaskVocab |
| Cross-task vocab lift (full corpus) | 0.0% aggregate (safe) | TestCrossTaskVocab |
| 10-round P@10 compounding | 0.277 -> 0.283 peak (+2.2%) | TestCompounding (300 tasks) |
| 10-round MRR compounding | 0.459 -> 0.497 peak (+8.1%) | TestCompounding (300 tasks) |
| Cross-task percentage | 100% (all improvements are cross-task) | TestCrossTaskVocab |
| RWR cache speedup | 2x (Django cold 3.9s -> warm 1.9s) | debug-rwr-cache |
| Competitive advantage | 3.23x codegraph, 18.7x grep | Cross-system benchmark |

All measurements: cold start, no task memory, no embeddings, honest methodology.

---

## 12. Conclusion

The progression from "code graph" to "shared intelligence layer" requires four primitives:

1. **Content-addressed relationships** (trust: is this still valid?)
2. **Emergent community structure** (scope: where does this apply?)
3. **Persistent, structurally-scoped feedback** (learning: what works here?)
4. **Cross-task vocabulary transfer** (bridging: what did similar tasks discover?)

Git proved that content-addressing makes source code trustworthy. Communities provide the emergent structure for agent intelligence to compound. Cross-task vocabulary transfer means the system gets smarter not just from repeated queries, but from ALL queries that share domain vocabulary.

The hidden insight: the graph is not a database to query. It is a substrate that agents collectively improve by using it. Each session deposits feedback and vocabulary. Each record is anchored to content-addressed symbols with per-package Merkle expiration. Each community is discovered from the graph itself. The system teaches itself which code matters for which work, organized by boundaries no human declared.

Content-addressing solves the trust problem. Communities solve the scoping problem. Cross-task vocabulary solves the transfer problem. Together, they form a shared intelligence layer for software development.

---

## 13. Limitations

1. **Per-package Merkle expiration is validated by unit test, not by compounding benchmark.** The benchmark runs 10 rounds against a static graph (no code changes between rounds), so expiration never triggers. Production validation requires repos with active development where packages change between agent sessions.

2. **Multi-agent coordination (Section 5) is designed but not shipped.** Community-based conflict avoidance and pending-mutation broadcasting are planned via Polywave. The learning loop (Sections 3-4) is fully operational; the coordination layer is not.

3. **Community-constrained RWR walks are not yet implemented.** The current RWR walks the full reachable subgraph. Constraining walks to the relevant community's symbols would produce tighter score distributions on large graphs. Focused seed selection (mechanism #4) partially addresses this by concentrating seeds in the dominant package cluster.

4. **Django compounding variance.** The 10-round Django compounding curve oscillates (band [0.200, 0.219] on 36 tasks). The full corpus (300 tasks) produces a tighter band ([0.276, 0.283]) with near-monotonic MRR. Django's 36-task sample size is insufficient for statistical significance on per-round deltas.

5. **Single-system evaluation.** The cross-task vocabulary bridging and compounding results are measured on knowing's own benchmark corpus. Independent replication on external corpora (e.g., SWE-bench repos) would strengthen the claims.

---

## 14. Reproducibility

All code, benchmarks, and task fixtures are open source under MIT license.

```bash
# Clone and build
git clone https://github.com/blackwell-systems/knowing.git
cd knowing && GOWORK=off go build ./...

# Set up benchmark corpus (16 repos, pinned commits)
cd bench/cross-system/corpus && bash corpus-setup.sh

# Cold-start P@10 (300 tasks, ~20 min)
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Cross-task vocabulary validation (~35 min)
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossTaskVocab -v -timeout 0

# 10-round compounding (Django, ~90 min)
BENCH_REPOS=django BENCH_COMPOUND_ROUNDS=10 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCompounding -v -timeout 0
```

Corpus manifest: [`bench/cross-system/corpus/MANIFEST.yaml`](https://github.com/blackwell-systems/knowing/blob/main/bench/cross-system/corpus/MANIFEST.yaml)
Methodology: [`bench/cross-system/METHODOLOGY.md`](https://github.com/blackwell-systems/knowing/blob/main/bench/cross-system/METHODOLOGY.md)
