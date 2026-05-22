# Cross-Repo Eval: gortex

**No knowing-specific equivalence classes.** Tests the general pipeline
(keyword tiers + BM25 + bigram compounds + graph-derived aliases + universal
equivalence classes + RRF) on an external Go codebase with no hand-curated
seed dictionary.

| Tier | R@10 | N |
|------|------|---|
| exact | 60.0% | 10 |
| concept | 20.0% | 10 |
| multi_hop | 60.0% | 10 |
| **Overall** | **46.7%** | **30** |
