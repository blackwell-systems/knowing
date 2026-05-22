# Cross-Repo Eval: gortex

**No knowing-specific equivalence classes.** Tests the general pipeline
(keyword tiers + BM25 + bigram compounds + graph-derived aliases + RRF)
on an external Go codebase with no hand-curated seed dictionary.

| Tier | R@10 | N |
|------|------|---|
| exact | 40.0% | 10 |
| concept | 10.0% | 10 |
| multi_hop | 10.0% | 10 |
| **Overall** | **20.0%** | **30** |
