# Research Agenda: Publishable Contributions

Four distinct papers can be extracted from the knowing system. Each targets a
different venue and audience. Ordered by novelty (most novel first).

## Paper 1: PUBLISHED

**Title:** "The Hierarchical Identity Architecture: Content-Addressing as a Computation
Primitive for Software Relationship Intelligence"

**Status:** Published (May 2026). Covers content-addressed entities, the 6 problems of
mutable graphs, hierarchical Merkle trees over semantic boundaries, formal properties,
O(packages) diff, O(1) cache keys, competitive validation vs GitNexus. Combined what
was originally scoped as two separate papers (crypto proofs + Merkle diff).

**Companion work (not a new paper):**
- event-stream supply chain demo: practical demonstration of prove/prove-absent on a
  real attack. Blog post or conference talk supplement, not a separate publication.

---

## Paper 3: Code Retrieval Evaluation Toolkit (CRET)

**Venue:** Benchmarks/Datasets (NeurIPS Datasets & Benchmarks, EMNLP Resources)

**Novel claim:** First rigorous, reproducible benchmark for symbol-level code retrieval
quality. SWE-bench evaluates agent task completion; CRET evaluates the retrieval layer
that feeds agents context. These are different things.

**Key contributions:**
1. 167 task fixtures across 9 repos, 6 languages, 3 difficulty tiers
2. Ground truth at symbol level (not file level), validated against actual DB contents
3. Statistical methodology: Wilcoxon signed-rank, Cohen's d, bootstrap CI
4. 7 systems benchmarked with fairness controls (same input, cold start, no tuning)
5. Reproducible with single command (`go test ./bench/cross-system/ -timeout 30m`)
6. Finding: P@10 is reachability-determined (32-config sweep, zero variance)

**Existing material:** `cross-system-benchmark.md`, `bench/cross-system/METHODOLOGY.md`,
`bench/cross-system/FINDINGS.md`

**What's needed:**
- Extract benchmark infrastructure as standalone toolkit
- Write fixture format specification
- Add 3-5 more repos for diversity (one non-English, one with ML code)
- Compare against SWE-bench and CrossCodeEval on scope/methodology

---

## Paper 4: Random Walk with Restart for Personalized Code Retrieval

**Venue:** IR/NLP (SIGIR, EMNLP, NAACL)

**Novel claim:** RWR on a multi-relational code graph (36 typed edges with distinct weights)
outperforms BM25, PageRank, and heuristic scoring for task-specific code retrieval.
The key insight: precision is entirely reachability-determined, not parameter-sensitive.

**Key contributions:**
1. 1.53x more precise than codegraph (19K stars), 3.29x vs Gortex, 15.9x vs grep
2. 32-config parameter sweep proving P@10 is structurally determined (zero variance)
3. Multi-channel fusion (RRF): tiered keyword + BM25 + equivalence + path-context + vector
4. Edge-type-weighted walk: `calls` 1.0, `implements` 0.8, `extends` 0.7, `imports` 0.5
5. Feedback compounding via content-addressed symbol hashes (+20pp per round)
6. Adjacency cache: 4,717x latency reduction (9s to 2ms) via binary serialization
7. Neutral edge experiment: `accesses_field` (session 15) adds 660+ edges per repo with zero P@10 change, confirming reachability-determination (fields already reachable via call edges)

**What's NOT novel here:** RWR itself, BM25, HITS, RRF are all known algorithms. The
contribution is the application domain (code retrieval), the empirical validation at
scale, and the reachability-determination finding.

**Existing material:** `internal/context/walk.go`, `bench/cross-system/FINDINGS.md`

**What's needed:**
- Ablation study (remove each component, measure delta)
- Comparison to embedding-based approaches (CodeBERT, UniXcoder retrieval)
- Theoretical analysis of why reachability dominates parameter sensitivity

---

## Ordering and Dependencies

```
Paper 1+2 (PUBLISHED)     Paper 3 (CRET)     Paper 4 (RWR)     Paper 5 (Density)
        |                       |                   |                  |
        v                       v                   v                  v
   event-stream            extract toolkit     ablation study     formalize model
   demo (blog)             5 more repos        embed comparison   additional repos
```

**Papers 1+2 are PUBLISHED** as a combined paper: "The Hierarchical Identity Architecture:
Content-Addressing as a Computation Primitive for Software Relationship Intelligence."

Paper 3 is the most immediately useful to the community (benchmark gap).
Paper 4 is the most incremental (well-known algorithms, new domain).
Paper 5 (new, session 14) addresses the density-adaptive retrieval finding.

Recommend: Paper 3 next (low effort, high visibility, establishes benchmark).
Then Paper 5 (novel finding with strong empirical evidence).

---

---

## Paper 5: Density-Adaptive Graph Retrieval (new, session 14)

**Venue:** IR/Systems (SIGIR, CIKM, WWW)

**Novel claim:** On code graphs with correct extraction, retrieval precision DEGRADES as
the graph grows denser, even when the correct results remain reachable. The root cause
is NOT the graph walk (proven: BFS depth and edge type exclusion have zero effect) but
seed selection degradation in the BM25 front-end (keyword competition). The fix is
density-adaptive: automatically prefer structural anchor nodes (types/interfaces) as
RWR seeds on graphs exceeding a density threshold.

**Key contributions:**
1. Empirical proof that graph walk parameters are irrelevant on dense graphs (32-config
   parameter sweep + edge exclusion + BFS depth sweep, all zero effect)
2. Root cause isolation: BM25 IDF saturation on large FTS indexes (3284 "action" matches
   on 87K-node graph vs ~50 on 43K-node graph)
3. Density-adaptive seed selection: prefer type/interface/class nodes as seeds when
   graph exceeds 40K nodes. +44% P@10 on VS Code, zero regression elsewhere.
4. Finding: "correct extraction hurts precision" is a general phenomenon in graph-based
   retrieval (observed independently on 3 repos with 3 different triggers)

**Empirical evidence:**
- VS Code: 43K nodes (broken extraction) P@10=0.163 -> 87K nodes (correct) P@10=0.084
- Same pattern: k8s staging (+136K nodes, -20%), LSP enrichment (+42K edges, negative)
- Fix: PreferTypeSeeds recovers 0.084 -> 0.137 (+44%), aggregate 0.202 -> 0.207
- Ablation: exclude similar_to (0.095), exclude type_hint_of (0.095), BFS depth=2 (0.100), hub dampening (0.095). None recovers. Only seed selection change works.

**What's novel:** The "correct extraction paradox" (doing the right thing makes results
worse) and the density-adaptive fix are not in existing literature. Prior work on
personalized PageRank/RWR assumes the graph is fixed; nobody has studied what happens
when correct extraction fundamentally changes the competition landscape for seed selection.

**Existing material:** `docs/research/dense-graph-dilution-analysis.md`, session 14 benchmark data

**What's needed:**
- Formalize the "keyword competition" model (why IDF degrades with node count)
- Test on additional dense codebases (chromium, linux kernel)
- Compare against dense passage retrieval approaches (DPR, ColBERT) on same corpus

---

## Existing Drafts Status

| Paper | Draft | Completeness | Next Action |
|-------|-------|-------------|-------------|
| 1+2 | `content-addressing-as-computation-primitive.md` | PUBLISHED | Supply chain demo (blog supplement) |
| 3 | `cross-system-benchmark.md` + METHODOLOGY | ~40% | Package as toolkit, write paper framing |
| 4 | No draft | 0% | Write from scratch using FINDINGS data |
| 5 | `dense-graph-dilution-analysis.md` | ~30% | Formalize model, add additional repos |
