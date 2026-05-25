# Research Agenda: Publishable Contributions

Four distinct papers can be extracted from the knowing system. Each targets a
different venue and audience. Ordered by novelty (most novel first).

## Paper 1: Content-Addressed Relationship Graphs with Cryptographic Audit Proofs

**Venue:** Systems (OSDI, SOSP, EuroSys) or Security (USENIX Security, Oakland)

**Novel claim:** Content-addressing applied to code *relationships* (not just code) enables
cryptographic proofs of dependency existence and absence. `prove(A calls B at snapshot S)`
and `prove-absent(A never calls B in any snapshot)` are new primitives that no existing
system provides.

**Key contributions:**
1. Edge identity as `sha256(source || target || type || provenance)`: same relationship
   discovered independently always has same hash (deduplication, federation, replay)
2. Merkle inclusion/exclusion proofs over relationship graphs (certificate transparency
   applied to software supply chains)
3. Temporal audit: "when did this dependency first appear?" answered via snapshot chain
   without replaying all extractions
4. Feedback expiration via content-addressing: when code changes, symbol hash changes,
   stale feedback becomes invisible. Zero-maintenance decay without timers or embedding drift.

**Existing draft:** `content-addressing-as-computation-primitive.md`

**What's needed:**
- Formal security model (what does the proof guarantee? what's the trust boundary?)
- Comparison to sigstore/in-toto/SLSA (supply chain integrity frameworks)
- Real-world scenario: event-stream supply chain attack detected via proof-of-absence

---

## Paper 2: Hierarchical Merkle Trees over Semantic Boundaries for Graph Diff

**Venue:** Databases (VLDB, SIGMOD) or Systems (OSDI, ATC)

**Novel claim:** Organizing a Merkle tree around semantic boundaries (packages, edge types)
instead of sorted hashes turns the identity structure into a query optimization substrate.
Diffs become O(packages) instead of O(edges). Cache invalidation scopes to packages that
actually changed. Subgraph root lookups are O(1).

**Key contributions:**
1. 216x faster diff on real graph (268K edges) vs flat comparison
2. 517x on 100K synthetic edges
3. Package-scoped FTS rebuild avoids full-table reindex
4. Community detection skips 94% of work when one package changes (6.9x Louvain, 38x LP)
5. Context pack determinism: same query + same graph state = identical PackRoot

**Empirical validation:** 5 repos (Flask to Kubernetes), benchmarked against flat approaches.
Not a theoretical argument; measured speedups on production-scale graphs.

**Existing draft:** `content-addressing-as-computation-primitive.md` (Section 3+)

**What's needed:**
- Separate from Paper 1 (the Merkle structure is orthogonal to the audit proofs)
- Formal complexity analysis (prove O(packages) bound)
- Comparison to existing graph versioning systems (DeltaGraph, GraphLab temporal)

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

Papers 1 and 2 are the most novel (no prior art in this exact formulation).
Paper 3 is the most immediately useful to the community (benchmark gap).
Paper 4 is the most incremental (well-known algorithms, new domain).

Recommend: Paper 3 first (low effort, high visibility, establishes benchmark that
Papers 1/2/4 can reference). Then Paper 1 (strongest novelty, crypto + systems crowd).

---

## Existing Drafts Status

| Paper | Draft | Completeness | Next Action |
|-------|-------|-------------|-------------|
| 1 | `content-addressing-as-computation-primitive.md` | ~70% | Add security model, supply chain demo |
| 2 | Same doc (needs splitting) | ~50% | Extract Merkle sections, add formal analysis |
| 3 | `cross-system-benchmark.md` + METHODOLOGY | ~40% | Package as toolkit, write paper framing |
| 4 | No draft | 0% | Write from scratch using FINDINGS data |
