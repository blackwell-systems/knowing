# Cross-Repo Eval: gortex

**No knowing-specific equivalence classes.** Tests the general pipeline
(keyword tiers + BM25 + bigram compounds + graph-derived aliases + RRF)
on an external Go codebase with no hand-curated seed dictionary.

| Tier | R@10 | N |
|------|------|---|
| exact | 80.0% | 10 |
| concept | 40.0% | 10 |
| multi_hop | 50.0% | 10 |
| **Overall** | **56.7%** | **30** |
