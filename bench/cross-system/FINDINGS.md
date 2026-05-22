# Cross-System Benchmark: Running Results

Tracking iterative improvements to retrieval quality.

**Full specification:** [docs/research/cross-system-benchmark.md](../../docs/research/cross-system-benchmark.md)
**Study overview:** [bench/CONTEXT-PACKING-STUDY.md](../CONTEXT-PACKING-STUDY.md)

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

**Why kubernetes + typescript were empty:** The indexing processes hung because CGO-bound tree-sitter calls blocked the pipeline (context cancellation can't interrupt CGO). Fixed with watchdog goroutine pattern.

### Run 4: All 5 repos indexed (2026-05-21, watchdog timeout fix)

Re-indexed all repos after fixing the CGO timeout hang. All 5 repos now have populated indexes.

**Indexing performance (--skip-blame --no-enrich --workers 8):**

| Repo | Files | Nodes | Edges | Time | Timeouts |
|------|-------|-------|-------|------|----------|
| kubernetes | 4,877 | 117,401 | 268,249 | 18.6s | 4 (YAML templates) |
| TypeScript | 38,260 | 88,393 | 67,182 | 25.8s | 1 (reallyLargeFile.ts) |
| Django | 2,937 | 42,947 | 151,431 | 3.3s | 0 |
| Cargo | 979 | 8,075 | 79,305 | 1.4s | 0 |
| Flask | 97 | 1,658 | 5,042 | 0.1s | 0 |
| **Total** | **47,150** | **258,474** | **571,209** | **49.2s** | 5 |

**Fix:** Replaced `context.WithTimeout` (ineffective against CGO) with watchdog goroutine + timer select. Stuck extractions fire-and-forget in background; pipeline never blocks.

### Run 5: Full benchmark with all 5 repos indexed (2026-05-21, post-optimization)

All repos now indexed with parallel pipeline + watchdog timeout + multi-row INSERT + SQLite pragmas.

| System | P@10 | R@10 | NDCG@10 | MRR | Token Eff | Latency |
|--------|------|------|---------|-----|-----------|---------|
| knowing | 0.149 | 0.224 | 0.246 | 0.269 | 0.0044 | 1ms |
| grep | 0.018 | 0.049 | 0.037 | 0.067 | 0.0015 | 480ms |

**Pairwise (knowing vs grep):**
- P@10: +0.131 (p<0.0001*, d=0.53, CI=[0.085, 0.182])
- R@10: +0.175 (p<0.0001*, d=0.58, CI=[0.117, 0.236])
- NDCG@10: +0.209 (p<0.0001*, d=0.52, CI=[0.135, 0.291])
- Token efficiency: +0.003 (p<0.0001*, d=0.33, CI=[0.001, 0.005])

**Delta from Run 3:** P@10 +46% (0.102 -> 0.149), R@10 +95% (0.115 -> 0.224). Caused entirely by kubernetes and TypeScript repos now being indexed (were empty before, 44% of tasks scored zero).

**Interpretation:** knowing is 8.3x better than grep on precision across all 100 tasks, 5 repos, 5 languages. Medium effect size (d=0.53). Absolute P@10 of 15% means room for improvement: FTS tokenization and terminal symbol matching remain the highest-leverage changes.

### Run 6: FTS symbol_name column (2026-05-21, migration 016)

Added dedicated `symbol_name` column to FTS index storing just the terminal identifier (e.g., "QuerySet.filter" instead of full qualified path). BM25 weights: symbol_name=10x, qualified_name=3x, signature=1x, file_path=1x.

**Pairwise (knowing vs grep):**
- P@10: +0.148 (p<0.0001*, d=0.62, CI=[0.103, 0.196])
- R@10: +0.215 (p<0.0001*, d=0.66, CI=[0.152, 0.280])
- NDCG@10: +0.248 (p<0.0001*, d=0.61, CI=[0.172, 0.333])
- Token efficiency: +0.002 (p<0.0001*, d=0.27, CI=[0.000, 0.003])

**Delta from Run 5:** Effect sizes improved across the board (d=0.53->0.62, 0.58->0.66, 0.52->0.61). All now in medium-large range. Absolute P@10 improved ~1.7pp (0.149 -> ~0.166).

**Interpretation:** The symbol_name column helps BM25 rank terminal identifiers higher by eliminating path token dilution. Improvement is modest (+1.7pp) because the remaining bottleneck is ground truth naming: fixtures use language-native module paths (e.g., "flask.app.Flask.before_request") that don't exactly match knowing's storage format even after `extractSymbolName` stripping.

### Run 7: Corrected ground truth fixtures (2026-05-21)

Revised all 100 fixtures: validated every ground truth symbol against actual DB contents. Removed unobtainable symbols (internal functions, external deps, skipped dirs). Replaced with verified alternatives. Match rate: 73% -> 95%.

| System | P@10 | R@10 | NDCG@10 | MRR | Token Eff | Latency |
|--------|------|------|---------|-----|-----------|---------|
| knowing | 0.141 | 0.195 | 0.213 | 0.250 | 0.0028 | 0ms |
| grep | 0.018 | 0.038 | 0.034 | 0.062 | 0.0013 | 464ms |

**Pairwise (knowing vs grep):**
- P@10: +0.123 (p<0.0001*, d=0.51, CI=[0.079, 0.172])
- R@10: +0.157 (p<0.0001*, d=0.57, CI=[0.105, 0.210])
- NDCG@10: +0.179 (p<0.0001*, d=0.49, CI=[0.110, 0.253])
- Token efficiency: +0.001 (p<0.0001*, d=0.23, CI=[0.000, 0.003])

**Delta from Run 6:** P@10 dropped from ~0.166 to 0.141 because ground truth is now harder (real, verified symbols instead of fuzzy-matchable wrong names). Effect size dropped from d=0.62 to d=0.51 (still medium). This is the honest baseline.

**Interpretation:** knowing is 7.8x better than grep with verified ground truth. 14.1% absolute precision means 86% of returned symbols don't match ground truth. This is the real number. Every improvement from here is genuine, not measurement artifact.

### Run 8: FTS tokenchars '_' + synchronous FTS rebuild (2026-05-21)

Two fixes: (1) FTS was never populated in CLI mode (background goroutine killed on process exit); (2) FTS5 tokenizer now treats underscore as a token character, so `before_request` is one token.

| System | P@10 | R@10 | NDCG@10 | MRR | Token Eff | Latency |
|--------|------|------|---------|-----|-----------|---------|
| knowing | 0.147 | 0.198 | 0.213 | 0.239 | 0.0029 | 1603ms |
| grep | 0.016 | 0.024 | 0.030 | 0.058 | 0.0012 | 453ms |

**Pairwise (knowing vs grep):**
- P@10: +0.131 (p<0.0001*, d=0.51, CI=[0.084, 0.182])
- R@10: +0.174 (p<0.0001*, d=0.67, CI=[0.125, 0.226])
- NDCG@10: +0.183 (p<0.0001*, d=0.47, CI=[0.114, 0.264])

**Delta from Run 7:** P@10 +0.006 (0.141->0.147). R@10 effect size jumped to d=0.67 (was d=0.57). This is the first run where FTS actually contributed (was previously empty/broken). The latency increase (0ms -> 1603ms) confirms FTS queries now execute against populated indexes.

**Critical bug found:** FTS was NEVER populated for CLI-indexed repos in ALL previous runs. Runs 1-7 measured retrieval quality WITHOUT any BM25 contribution. The engine ran on graph traversal (RWR + seeds) alone. P@10=0.14 was achieved without FTS.

**Next steps:**
1. Cross-file import resolution for Python/TS (more call edges = better recall)
2. Session memory persistence (feedback compounding)
3. Investigate why FTS contributes so little despite being populated (P@10 +0.006 only)

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
