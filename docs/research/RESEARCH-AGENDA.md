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

**Novel claim:** RWR on a multi-relational code graph (32 typed edges with distinct weights)
outperforms BM25, PageRank, and heuristic scoring for task-specific code retrieval.
The key insight: precision is entirely reachability-determined, not parameter-sensitive.

**Key contributions:**
1. 1.50x more precise than codegraph (19K stars), 3.21x vs Gortex, 15.5x vs grep
2. 32-config parameter sweep proving P@10 is structurally determined (zero variance)
3. Multi-channel fusion (RRF): tiered keyword + BM25 + equivalence + path-context + vector
4. Edge-type-weighted walk: `calls` 1.0, `implements` 0.8, `extends` 0.7, `imports` 0.5
5. Feedback compounding via content-addressed symbol hashes (+20pp per round)
6. Adjacency cache: 4,717x latency reduction (9s to 2ms) via binary serialization

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
Paper 1 (Proofs)     Paper 2 (Merkle Diff)     Paper 3 (CRET)     Paper 4 (RWR)
     |                      |                       |                   |
     v                      v                       v                   v
  event-stream          formal proofs            extract toolkit     ablation study
  demo (impl)           O(pkg) bound             5 more repos        embed comparison
```

**Papers 1+2 are PUBLISHED** as a combined paper: "The Hierarchical Identity Architecture:
Content-Addressing as a Computation Primitive for Software Relationship Intelligence."

Paper 3 is the most immediately useful to the community (benchmark gap).
Paper 4 is the most incremental (well-known algorithms, new domain).

Recommend: Paper 3 next (low effort, high visibility, establishes benchmark).
Then Paper 4 (leverages CRET as evaluation framework).

---

## Existing Drafts Status

| Paper | Draft | Completeness | Next Action |
|-------|-------|-------------|-------------|
| 1 | `content-addressing-as-computation-primitive.md` | ~70% | Add security model, supply chain demo |
| 2 | Same doc (needs splitting) | ~50% | Extract Merkle sections, add formal analysis |
| 3 | `cross-system-benchmark.md` + METHODOLOGY | ~40% | Package as toolkit, write paper framing |
| 4 | No draft | 0% | Write from scratch using FINDINGS data |
