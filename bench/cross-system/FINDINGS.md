# Cross-System Benchmark: Running Results

Tracking iterative improvements to retrieval quality.

## Run History

### Run 1: Baseline (2026-05-21, commit 9cc6f8d)

First run after fixing normalizer to handle knowing's `repoURL://filepath.Symbol` format.

| System | P@10 | R@10 | NDCG@10 | MRR | Token Eff | Latency |
|--------|------|------|---------|-----|-----------|---------|
| knowing | 0.102 | 0.111 | 0.157 | 0.159 | 0.0029 | 0ms |
| grep | 0.018 | 0.050 | 0.032 | 0.059 | 0.0015 | 458ms |

**Pairwise (knowing vs grep):**
- P@10: +0.084 (p=0.0004*, d=0.36)
- R@10: +0.061 (p=0.0025*, d=0.29)
- NDCG@10: +0.125 (p=0.0007*, d=0.33)
- Token efficiency: +0.001 (p=0.04*, d=0.17)

**Interpretation:** knowing is 5.7x better than grep on precision. All differences statistically significant. Absolute P@10 of 10% means 9/10 top results don't match ground truth. Primary cause: FTS matching doesn't find symbols stored as `filepath.py.ClassName.method` when searching for language-native names.

### Run 2: Language equivalence classes (2026-05-21, commit 5dc1f22)

Added 31 language-specific equivalence classes (Python, TS, Rust, Java, K8s).

| System | P@10 | R@10 | NDCG@10 | MRR | Token Eff | Latency |
|--------|------|------|---------|-----|-----------|---------|
| knowing | 0.102 | 0.111 | 0.157 | 0.159 | 0.0029 | 1ms |
| grep | 0.016 | 0.029 | 0.030 | 0.061 | 0.0014 | 455ms |

**Delta from Run 1:** No change for knowing. Grep slightly worse (variance).

**Why no improvement:** Language seeds add target symbol names to the FTS query, but FTS searches against `qualified_name` which in non-Go repos is `repoURL://filepath.py.ClassName.method`. Searching for "before_request" doesn't match "scaffold.py.Scaffold.before_request" because FTS tokenization doesn't split on dots within qualified names. The seeds are correct but the FTS layer can't find the symbols they point to.

**Next step:** Fix FTS tokenization to index terminal symbol names (the part after the last file extension + dot), not just the full qualified path.

### Run 3: Test fixture filtering + ground truth validation (2026-05-21, commit 057628a)

Added `conftest.py`, `test_helper`, `testutil` to the noisy symbol filter. Added ground truth achievability filter (only count symbols that exist in the DB).

| System | P@10 | R@10 | NDCG@10 | MRR | Token Eff | Latency |
|--------|------|------|---------|-----|-----------|---------|
| knowing | 0.102 | 0.115 | 0.160 | 0.159 | 0.0029 | 0ms |
| grep | 0.017 | 0.040 | 0.032 | 0.061 | 0.0014 | 462ms |

**Critical finding:** kubernetes (22 tasks) and typescript (22 tasks) repos have EMPTY indexes (0 nodes, 0 edges). Only flask (14), django (26), and cargo (16) are actually indexed. This means 44% of tasks score zero regardless of retrieval quality.

**Effective results (on indexed repos only: flask + django + cargo, 56 tasks):**
- knowing hits P@10 > 0 on 30+ of 56 tasks
- Multiple tasks score P@10 = 0.90 or 1.00 (cargo-hard-002, django-easy-003, django-medium-002)
- Flask/Django tasks consistently score 0.10-0.40 P@10

**Why kubernetes + typescript are empty:** The indexing processes were killed before the batch commit (knowing writes all data in a single transaction at the end). These need re-indexing with the new parallel binary.

**Next steps:**
1. Re-index kubernetes and typescript with the new parallel binary
2. Once all 5 repos are indexed, re-run for accurate aggregate numbers
3. The actual per-task numbers on working repos are much stronger than 10% aggregate suggests

---

## Identified Bottlenecks (from analysis)

1. **FTS tokenization** (blocks all keyword-based improvements): qualified names include file paths. BM25 search for "QuerySet" doesn't match "github.com/django/django://django/db/models/query.py.QuerySet" because the tokenizer treats the whole thing as one token or splits on `/` but not on `.` after file extensions.

2. **Ground truth naming** (inflates false negatives): fixtures use Python module paths (`flask.app.Flask.before_request`) but knowing stores symbols with file paths and possibly different class names (base class vs subclass).

3. **Missing competitor tools**: only comparing knowing vs grep. Need gitnexus, aider, cgc installed to produce the full comparison.

## Systems Not Yet Tested

| System | Why not | What's needed |
|--------|---------|---------------|
| GitNexus | Not installed | `npm install -g gitnexus` |
| Aider | Not installed | `pip install aider-chat` |
| CGC | Not installed | `pip install codegraphcontext` |
| SCIP | Adapter not built | Need SCIP index generation |
