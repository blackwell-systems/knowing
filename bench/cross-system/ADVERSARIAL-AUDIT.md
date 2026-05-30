# Adversarial Audit: knowing Cross-System Benchmark (P@10 = 0.283)

**Auditor perspective:** Senior researcher at a competing code intelligence company.
knowing claims P@10=0.283 across 277 tasks, 14 repos, 8 languages, beating
competitors 2.10x. This audit evaluates the methodology, corpus, and
reproducibility for weaknesses.

**Date:** 2026-05-30

---

## 1. Executive Summary

knowing's benchmark infrastructure is unusually thorough for a solo-developer
project: pinned commits, paired statistical tests, bootstrap CIs, a
reproducibility script, and multiple competitor adapters. However, the evaluation
suffers from three structural problems that would prevent publication in a
rigorous venue: (a) dangerously permissive symbol matching inflates all reported
P@10 numbers, (b) ground truth is overwhelmingly author-written with no
inter-annotator agreement measurement, and (c) competitor adapters are not
given equivalent treatment, with some receiving crude output parsing that
systematically disadvantages them.

## 2. Critical Findings

### HIGH-1: Symbol Matching Is Excessively Permissive (Inflates All Metrics)

The `normalize.MatchesGroundTruth()` function at
`bench/cross-system/normalize/normalize.go` uses four cascading match
strategies: exact, case-insensitive, terminal name, and substring containment.
The terminal name matching is particularly dangerous. If the ground truth is
`django.template.library.Library.filter` and the system returns ANY symbol
ending in `.filter`, it matches as long as the terminal name is not in a short
blocklist of 15 common words. The word "filter" is NOT on that blocklist.

Even worse, the substring containment match
(`strings.Contains(r, g) || strings.Contains(g, r)`) means if a retrieved
symbol's normalized form contains or is contained by the ground truth's
normalized form, it matches.

The code explicitly acknowledges this: "favor recall over precision in matching."
But this directly inflates the P@10 metric that is the headline number.

**Impact:** Unknown without ablation, but could be substantial. Would need to
run with exact-match-only to quantify the inflation.

**Recommendation:** Run an ablation with exact-match-only normalization and
report the delta. At minimum, remove substring containment and tighten
terminal-name matching to require qualifier overlap.

### HIGH-2: Ground Truth Author Conflict of Interest

297 task fixtures exist, 243 are labeled `source: manual`, 29 from `swe-bench`,
25 `synthetic`. The manual fixtures were written by the same person who built
the retrieval engine. There is no inter-annotator agreement, no blind labeling,
and no external validation panel.

This creates two risks:
1. **Selection bias:** The author knows which symbols their RWR walk can reach
   and may unconsciously select ground truth that aligns with graph-reachable
   symbols.
2. **Vocabulary alignment:** Task descriptions may use vocabulary that aligns
   with knowing's BM25 keyword extraction because the same person wrote both.

The 29 swe-bench tasks partially mitigate this (externally sourced), but they
represent only 10% of the corpus.

**Recommendation:** Have 2-3 external developers label a subset of tasks blind.
Compute inter-annotator agreement (Fleiss' kappa). Even 50 tasks externally
labeled would strengthen the claims.

### HIGH-3: Competitor Adapters Receive Unequal Treatment

Several forms of unfairness:

**a) Aider adapter parses repo-map output with crude regex.** The Aider adapter
extracts symbols by searching for `def `, `class `, `func ` in text output.
Aider's RepoMap produces tree-context output, not a ranked symbol list. The
adapter caps input files at 2000. This is a strawman.

**b) codegraph gets `--max-nodes 50 --max-code 20` but knowing gets its full
pipeline.** Whether these parameters are optimal for codegraph is unexplored.
codegraph's maintainers were not consulted on adapter configuration.

**c) Token estimation differs across adapters.** knowing counts actual tokens.
codegraph estimates `len(output) / 4`. The token efficiency metric is comparing
apples to oranges.

**d) Competitors not tested on full corpus.** The competitive ratio (2.10x)
is computed from knowing's full-corpus P@10 (277 tasks) vs codegraph's
partial-corpus P@10 (107 tasks). The honest comparison would use only tasks
where both systems ran.

**Recommendation:** Report head-to-head on matched task sets only. Rewrite the
Aider adapter. Contact competitor maintainers for optimal configurations.

## 3. Medium Findings

### MEDIUM-1: filterAchievableGroundTruth Uses Knowing's Own Index

The `filterAchievableGroundTruth` function opens knowing's `.knowing/graph.db`
to check which ground truth symbols exist. Ground truth is defined by what
knowing indexed. If knowing fails to extract a symbol that codegraph can find,
that symbol is removed from ground truth, and codegraph gets no credit.

### MEDIUM-2: Inconsistent Numbers Across Documents

FINDINGS.md, METHODOLOGY.md, and MANIFEST.yaml report different repo and task
counts in various sections. Documents are not updated atomically.

### MEDIUM-3: No Multiple-Comparison Correction

24 pairwise tests (6 competitors x 4 metrics) with no Bonferroni or FDR
correction. At alpha=0.05 with 24 independent tests, the probability of at
least one false significant result is ~71%.

### MEDIUM-4: Wilcoxon Implementation Does Not Handle Tied Ranks

The implementation assigns sequential ranks after sorting, ignoring ties. With
P@10 quantized to {0.0, 0.1, 0.2, ...}, ties are extremely common. P-values
are unreliable.

### MEDIUM-5: Token Budget Favors Graph-Based Systems

The fixed 5000-token budget structurally favors symbol-level systems over
file-level context tools. This is not wrong (the benchmark measures symbol
retrieval) but should be stated more clearly.

### MEDIUM-6: Round 2 "Compounding" Test Is Circular

`TestCrossSystemRound2` records ground truth symbols as "useful" with score 1.0
in task memory, then re-runs the same tasks. The system is told the answers and
asked the same questions. This is memorization, not compounding.

**Recommendation:** Use a held-out task set or label it as "upper bound with
oracle feedback."

## 4. Low Findings

### LOW-1: Language and Framework Bias

No C/C++, Kotlin, Swift, PHP. Repos biased toward well-structured open source
projects.

### LOW-2: Enrichment Not Deterministic Across Platforms

Language server versions are not pinned. Different versions produce different
edges and different P@10.

### LOW-3: Parallel Mode Variance Not Quantified

+-0.009 P@10 variance acknowledged but not statistically characterized.

### LOW-4: Rails Has No Enrichment or Embeddings

Contributes 20 tasks that systematically score lower.

## 5. What They Got Right

1. **Pinned commits + verification script.** Better than 90% of published
   benchmarks.
2. **Paired statistical tests.** Wilcoxon on same tasks is correct (despite
   tie-handling bug).
3. **Effect size reporting.** Cohen's d alongside p-values.
4. **Bootstrap confidence intervals.** 10K resamples, deterministic seed.
5. **Multiple metrics.** P@10, R@10, NDCG@10, MRR, token efficiency.
6. **Failure transparency.** Crashes/timeouts score 0, not excluded.
7. **Ground truth validation.** Symbols verified against database.
8. **Self-criticism.** Honest "Known Limitations" section.
9. **Parameter sweep transparency.** Publishing that 26 configs produce
   identical P@10 is unusually honest.
10. **Corpus diversity.** 14 repos, 14K-3.5M LOC, 8 languages.
11. **No cherry-picking.** Selection policy documents that every repo attempted
    was included, with no exclusions based on performance.

## 6. Overall Verdict

**Would I accept these results for a peer-reviewed publication? No, not in
current form.**

The permissive symbol matching (HIGH-1) and single-author ground truth (HIGH-2)
are disqualifying for a venue like ICSE, FSE, or ASE. The unequal competitor
treatment (HIGH-3) would draw immediate reviewer objections. The Wilcoxon
tie-handling bug (MEDIUM-4) undermines the statistical claims.

However, the infrastructure is genuinely impressive for a pre-publication project.
The reproducibility apparatus exceeds most published benchmarks. The parameter
sweep honesty and self-critical limitations section show scientific integrity.
With fixes (tighter matching, external ground truth validation, fair competitor
adapters), this could become a credible evaluation.

**Bottom line:** The P@10 = 0.283 headline number is likely inflated by matching
leniency, and the 2.10x competitive ratio is inflated by comparing different task
sets. The true advantage over codegraph is probably real but smaller than claimed.
The engineering is solid; the experimental methodology needs tightening before
these numbers can be trusted by external parties.

## 7. Priority Fix List

| # | Finding | Effort | Impact |
|---|---------|--------|--------|
| 1 | Run exact-match ablation to quantify matching inflation | Low | Reveals true P@10 |
| 2 | Fix Wilcoxon tied-rank handling | Low | Correct p-values |
| 3 | Add Bonferroni correction | Low | Correct significance claims |
| 4 | Report head-to-head on matched task sets | Low | Honest competitive ratios |
| 5 | External ground truth labeling (50 tasks) | Medium | Inter-annotator agreement |
| 6 | Rewrite Aider adapter | Medium | Fair competitor treatment |
| 7 | Pin language server versions in manifest | Low | Reproducibility |
| 8 | Rename Round 2 or use held-out tasks | Low | Honest compounding claim |
