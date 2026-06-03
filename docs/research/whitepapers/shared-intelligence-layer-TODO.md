# Shared Intelligence Layer: Expansion Plan for Peer Review

The current preprint is Zenodo-ready (~2,500 words, empirical evidence, reproducibility).
Expanding to a full conference paper (ICSE, FSE, SIGIR) requires the following.

## What's ready

- Abstract, architecture, learning loop, empirical evidence table
- Related work section (PageRank, RAG, SWE-bench, community detection)
- Limitations (5 items, honest)
- Reproducibility (exact commands, corpus manifest)
- Reference to published prior work (Hierarchical Identity Architecture)

## What's needed for a full paper

### 1. Formal problem definition
- Define the vocabulary gap mathematically (keyword set K, symbol set S, overlap function)
- Define cross-task transfer (task A's learned mapping M_A applied to task B where K_A intersects K_B)
- Define structural expiration (SubgraphRoot function, validity predicate)
- This gives reviewers precise claims to evaluate

### 2. Algorithm pseudocode
- Learning loop: record, filter (isVocabWorthy), threshold (count >= 2), inject (soft RRF)
- RRF fusion with confidence-weighted vocab channel
- Merkle expiration: record(association, packageRoot), lookup(association, currentRoot), filter(stored != current)
- Cross-task bridging: keywordOverlap(taskA, taskB) -> sharedAssociations

### 3. Ablation study
Remove each component independently, measure delta on full corpus:
- No noise filter (all keywords recorded)
- No confidence weighting (flat 0.5 weight)
- Forced injection instead of soft RRF
- No per-cluster scoping (global feedback)
- No Merkle expiration (all associations persist forever)

Session 26 has partial data (forced vs soft: -3.9% vs 0.0%; all keywords vs filtered: -23% vs +41%). Need systematic single-variable ablations.

### 4. Per-repo breakdown
Show cross-task bridging results for each of the 16 repos, not just Django aggregate:
- Which repos benefit most? (hypothesis: repos with vocabulary gaps)
- Which repos are neutral? (hypothesis: repos with good BM25 coverage)
- Which repos degrade? (hypothesis: none, but prove it)

### 5. Comparison with learned embeddings
- Fine-tune CodeBERT or UniXcoder for vocabulary bridging (same task pairs)
- Compare: does learned embedding similarity bridge the same gaps as keyword-based vocab?
- Hypothesis: embeddings are weaker because they generalize poorly to framework-specific vocabulary

### 6. Extended compounding analysis
- Full corpus 10-round compounding with per-task attribution
- Show which tasks compound (vocabulary gap tasks) vs which plateau (already-covered tasks)
- Statistical significance: Wilcoxon on per-round P@10 vectors

### 7. Threats to validity
- Task fixture selection bias (hand-curated by knowing's developer)
- Single implementation (results may not transfer to other graph architectures)
- Django overrepresentation (36/308 tasks, highest zero-rate)
- Compounding noise floor on small repos
- Benchmark simulates agent usage; real MCP usage patterns may differ

### 8. Expanded related work
- Detailed comparison with Sourcegraph's code intelligence (SCIP-based, no learning)
- GitHub Copilot's retrieval layer (opaque, no published methodology)
- Continue.dev's context engine (embedding-based, no graph)
- Academic: code completion with retrieval augmentation (RepoFusion, CrossCodeEval)

## Estimated effort

| Section | Effort | Blocking? |
|---------|--------|-----------|
| Formal definitions | 1 session | No (writing) |
| Algorithm pseudocode | 1 session | No (writing) |
| Ablation study | 1 session | Yes (benchmark runs) |
| Per-repo breakdown | 0.5 session | Yes (benchmark run) |
| Embedding comparison | 2 sessions | Yes (fine-tuning + eval) |
| Extended compounding | 0.5 session | Yes (benchmark run) |
| Threats to validity | 0.5 session | No (writing) |
| Expanded related work | 0.5 session | No (writing) |

**Total: ~6 sessions for a full conference paper.**

## Target venues

| Venue | Deadline | Fit |
|-------|----------|-----|
| ICSE 2027 | ~Oct 2026 | SE audience, systems + empirical |
| FSE 2027 | ~Sep 2026 | SE audience, tool papers track |
| SIGIR 2027 | ~Jan 2027 | IR audience, novel retrieval method |
| NeurIPS D&B 2026 | ~Jun 2026 | Benchmark track (CRET extraction) |
| MSR 2027 | ~Nov 2026 | Mining repos, empirical study |

## Immediate next step

Publish preprint on Zenodo. Get DOI. Cite in blog, README, LinkedIn. Expand if a venue deadline motivates it.
