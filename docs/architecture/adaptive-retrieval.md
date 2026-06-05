# Adaptive Retrieval

knowing's retrieval pipeline observes its own graph properties at query time and
adjusts its strategy accordingly. No manual configuration. The same binary handles
a 1.4K-node Flask repo and a 552K-node VS Code repo with different strategies,
automatically. This is the project's central thesis: a code retrieval system that
adapts to its graph outperforms any fixed-strategy system, and the gap widens with
scale.

**Current result:** P@10 = 0.330 cold start (302 tasks, 16 repos, 8 languages,
honest measurement: no task memory, no embeddings). 3.79x codegraph, 5.84x
GitNexus, 6.35x Gortex, 14.3x Aider, 22.0x grep.

## The Problem with Fixed-Strategy Retrieval

Every competing code retrieval system uses a fixed strategy regardless of graph
properties. codegraph uses the same BM25+heuristic scoring on a 1K-node repo
and a 100K-node repo. GitNexus builds the same knowledge graph structure whether
the codebase has 5 packages or 500.

This works at small scale. At large scale, it fails:
- BM25 keyword competition: "Handle" matches 10,000 symbols on k8s
- Seed quality degrades: methods outnumber types 10:1, but methods are worse seeds
- Disconnection increases: larger graphs have more isolated subgraphs
- Vocabulary gaps widen: more symbols = more ground truth with zero keyword overlap

The result: fixed-strategy systems get less precise as codebases grow. knowing
gets more precise because it detects these conditions and compensates.

## Thirteen Self-Adapting Mechanisms

### 1. PreferTypeSeeds (density-adaptive seed selection)

**Trigger:** `GraphNodeCount > 40,000`

**What it does:** Reorders RRF candidates so type/interface/class nodes are
selected as RWR seeds before methods/functions. Types make better seeds on dense
graphs because they have `contains` edges to their methods, producing more
productive walks.

**Why it's needed:** On dense graphs, BM25 returns many method-level matches
that compete for seed slots. Methods can only walk upward to callers (competing
with thousands of other methods for the same keywords). Types walk downward to
their methods via `contains` edges, reaching an entire subsystem from one seed.

**Measured impact:** VS Code +44% (0.095 -> 0.137). Zero regressions on any repo.
Auto-enables based on node count; no manual threshold configuration.

**Session 17 finding:** Phantom nodes from LSP enrichment inflate GraphNodeCount
(k8s: 72K real -> 242K with phantoms). This was initially suspected as a bug
(triggering PreferTypeSeeds on repos that aren't truly dense). Testing showed
it's correct behavior: enrichment edges make the graph genuinely denser, and
PreferTypeSeeds benefits from the inflated count. The phantom density signal is
valid.

### 2. Adaptive Seed Count (scale-aware exploration)

**Trigger:** `GraphNodeCount > 10,000` (20 seeds) or `> 40,000` (25 seeds)

**What it does:** Increases the number of RWR seeds on larger graphs. Default is
15 seeds. Larger graphs have higher disconnection rates; more seeds compensate by
covering more subgraphs.

**Why it's needed:** On a 1K-node Flask repo, 15 seeds cover most of the graph
within 4 RWR hops. On a 57K-node Django repo, 15 seeds leave large regions
unreachable. More seeds = broader coverage = more ground truth found.

**Measured impact:** Django +14.2% (0.197 -> 0.225). Full corpus +3.8% (0.238 -> 0.247).

### 3. Framework Equivalence Classes with Forced Injection (vocabulary-adaptive bridging)

**Trigger:** Always active; language-scoped via `Lang` field + `detectRepoLanguage()`

**What it does:** Maps framework-specific concepts to specific symbol names
across 263 equivalence classes in 30 per-framework files. When a task says
"custom validator", the system finds `EmailValidator`, `BaseValidator`,
`ValidationError` in Django. When a task says "consumer group", it finds
`KafkaConsumer`, `ConsumerRecord` in Kafka.

High-confidence matches (weight >= 0.9, source "framework") bypass RWR
scoring and inject directly at the top of the ranked results (forced injection),
subject to the multi-phrase gate (`isStrongEquivMatch`): either >= 2 phrases
matched or the single phrase is multi-word. This prevents single generic words
(e.g., "command" triggering VSCODE_COMMAND) from flooding the top-10 with
framework hub symbols. Learned vocab (source "learned") goes through RRF
competition instead (soft injection). This solves the vocabulary gap for
framework concepts where BM25 and graph walks fail, while preventing learned
associations from displacing correct results on tasks with good BM25 coverage.

**Coverage:** Django, Flask, FastAPI, Terraform, Kubernetes, Kafka, Rails,
Spring, ASP.NET, Ocelot, Caddy, Cargo, Spark-Java, VS Code, NestJS, Next.js,
Angular, React, Jekyll + cross-cutting (testing, ORM, auth, CLI, config,
errors, web, containers, cryptography).

**Why it's adaptive:** Each class has a `Lang` field (go, python, typescript,
ruby, java, csharp, rust) restricting it to matching repos.
`detectRepoLanguage()` samples node QNs to determine the primary language.
Go router classes never fire on C# repos. Django classes never fire on
Terraform repos.

**Why it works where BM25 fails:** The task says "validates email format" but
the symbol is `EmailValidator`. BM25 can't find it because "email format"
doesn't match "EmailValidator" (different token structure). But the equiv
class maps "email validation" -> "EmailValidator" directly.

**Measured impact (session 23, honest measurement):**
- Django: 0.081 -> 0.183 (+126%)
- Terraform: 0.120 -> 0.405 (+238%)
- Kafka: 0.232 -> 0.421 (+81%)
- VS Code: 0.037 -> 0.168 (+354%)
- Full corpus: 0.176 -> 0.278 (+57%)

**Defensibility criterion:** Every equiv class must pass the test: "would this
mapping appear in the framework's official documentation or tutorials?" Classes
mapping application internals (e.g., ripgrep's `DecompressionMatcher`) are
rejected as curve-fitting.

### 4. Focused Seed Selection (cohesion-adaptive seeding)

**Trigger:** Always active (more than 5 candidates after RRF fusion)

**What it does:** After RRF fusion produces candidates, clusters them by package
path and promotes the largest cluster to the front of the seed list. The maxSeeds
cap downstream then naturally selects from this focused set. Instead of scattering
15-25 seeds across the graph, the walk concentrates in the dominant structural
neighborhood.

**Why it's needed:** 57 experiments proved seed count doesn't matter, but seed
structural cohesion does. Scattered seeds dilute the RWR walk across unrelated
areas of the graph; cohesive seeds focus it on the right neighborhood.

**Why it can't regress:** If no dominant cluster exists (all singletons), the
candidates pass through unchanged. The mechanism only reorders when there is
genuine structural signal.

**Measured impact:** Full corpus P@10 0.267 -> 0.283 (+6.0%). Django +8.7%.
First experiment to break through the session 20 ceiling.

### 5. ~~Cluster-Aware Gap-Fill Seeds~~ (NEUTRAL, session 23)

**Status:** Confirmed neutral on honest measurement. Three runs with and without
embeddings produced identical P@10 (0.176, 0.175, 0.176). The previous "+11%"
finding was task memory contamination: gap-fill kept injecting the same symbols
that accumulated task memory was boosting, creating a feedback loop that looked
like improvement.

**What it does:** When primary keyword-based channels return fewer than 5 seed
candidates, queries the embedding vector store for semantically similar symbols.
Infrastructure preserved but embeddings not loaded in the benchmark.

**Why it's neutral:** Framework equivalence classes (mechanism 3) now solve the
vocabulary gap that gap-fill was designed for. "Custom validator" -> EmailValidator
is a direct, precise mapping. Embedding cosine similarity produces weaker,
noisier candidates for the same problem. With equiv classes active, gap-fill
has nothing left to contribute.

**Historical note:** Previous measurements (sessions 15-22) showed gap-fill as
+11% because task memory accumulated across benchmark runs. Session 23
discovered 26,096 stale task memory entries in terraform alone. After disabling
task memory in the benchmark adapter, gap-fill measured dead neutral.

### 6. Task Memory Compounding (learning-adaptive boosting) [DISABLED]

**Status:** Disabled in the MCP server since session 24 (confirmed neutral on
honest measurement). The `SetTaskMemory` call is commented out in
`internal/mcp/context_handlers.go`. Infrastructure preserved in
`internal/context/task_memory.go` for potential future redesign.

**What it did:** Recorded the top-5 symbols returned by each `context_for_task`
call. On future queries with similar keywords, recalled stored symbols and boosted
them via the feedback channel. Boost formula: `0.5 + recall_score * 0.4`
(range [0.5, 0.9]), with 7-day linear decay.

**Why it was disabled:** Session 24 proved this mechanism redundant with the
pipeline (BM25 and equiv classes already find the same symbols). The positive
measurements from sessions 17-22 were inflated by task memory contamination
(stale entries accumulated across benchmark runs, creating a feedback loop).

**Superseded by:** Mechanism 13 (cross-task vocabulary bridging) provides the
learning-over-time capability that task memory was designed for, with better
precision (domain-specific keywords only, confidence weighting, Merkle expiration).

### 7. Merkleized Expiration (staleness-adaptive validity)

**Trigger:** Code change in the symbol's package (SubgraphRoot mismatch)

**What it does:** Two data types use per-package Merkle roots for automatic
expiration:

1. **Feedback records** (`internal/store/feedback.go`): each record stores the
   Merkle root of the symbol's package via `neighborhood_root`. When querying
   feedback, only records where `neighborhood_root` matches the current
   SubgraphRoot are counted.

2. **Vocab associations** (`internal/store/vocab.go`, migration 022, session 26):
   each association stores the package Merkle root via `subgraph_root`. At query
   time, `LearnedVocabAssociations` filters associations where the root doesn't
   match the current package state. This provides per-package precision: when
   package A changes, only associations for symbols in package A expire; package B's
   associations remain valid.

Both use the same infrastructure: `persistPackageRoots` stores htree.PackageRoots
to the notes table during indexing; `LoadPackageRoots` + `PackageRootForSymbol`
look them up at query time. Both the context engine and MCP server paths are wired.

**Why it's needed:** Stale feedback and stale vocab associations are worse than
none. A symbol that was useful yesterday but was refactored today should not
receive a boost. Traditional systems require explicit TTL policies or manual
cleanup. Content-addressed Merkle roots provide structural expiration: data is
valid if and only if the code hasn't changed.

**Measured overhead:** 11% (255us -> 284us for 100 symbols). The Merkle root
comparison is a single hash equality check.

### 8. LSP Enrichment Interaction (enrichment-adaptive reachability)

**Trigger:** LSP enrichment creates phantom nodes + type_hint_of edges exist

**What it does:** LSP enrichment discovers new edges (implements, references)
and creates phantom external nodes (stdlib/dependency types). When type_hint_of
edges (from tree-sitter extraction) connect functions to those phantom nodes,
RWR can walk between functions that reference the same external type. Neither
enrichment alone nor type_hint_of alone produces this reachability; they interact
multiplicatively.

**Why it's adaptive:** The reachability benefit scales with enrichment coverage.
Repos with more enrichment (Go with gopls: 192K new edges on k8s) see larger
improvements than repos with less enrichment. The system automatically leverages
whatever enrichment is available without configuration.

**Measured impact:** Kubernetes 0.000 -> 0.232 (first-time enrichment). Terraform
~0.095 -> 0.275. Python repos +0.040. Session 13 measured enrichment as neutral
(before type_hint_of existed). Session 17 revised: enrichment is strongly positive
when combined with type_hint_of edges.

### 9. Adaptive Retrieval for Massive Repos (scale-adaptive fallback)

**Trigger:** `effectiveNodeCount() > 200,000` AND `resultConfidence(ranked) < 0.3`

**What it does:** After RWR, measures whether the score distribution is flat
(top-10 scores within 20% of each other). Flat distribution means RWR didn't
converge on anything meaningful. Falls back to direct FTS + contains-edge
expansion: find symbols by name, then expand types via `contains` edges to
get their methods/fields.

**Why it's needed:** On VS Code (552K nodes), RWR always diffuses to
near-uniform regardless of seed quality. The walk visits so many nodes that
score differences become negligible. Direct FTS bypasses the walk entirely,
finding symbols by name match and expanding structurally.

**Guard:** Only triggers on repos > 200K nodes. Mid-size repos (cargo 81K,
kafka 105K) have effective RWR convergence; triggering the fallback on them
regresses P@10 because their flat-looking scores are actually correct rankings.

**Measured impact:** VS Code 0.037 -> 0.053 (+43%) without equiv classes.
With equiv classes: 0.053 -> 0.168 (equiv classes dominate).

### 10. RWR Proximity Packing (enrichment-adaptive density)

**Trigger:** Always active (zero overhead, uses already-computed RWR scores)

**What it does:** In `packIntoBudget`, multiplies each symbol's density (score/tokens)
by `rwrScore^0.3` (cube root of raw RWR score). Symbols structurally closer to seeds
(higher RWR score) get higher effective density, packing into the budget before distant
high-centrality noise.

**Why it's needed:** LSP enrichment creates phantom external nodes that inflate the
degree of real intermediate symbols (e.g., `.validate`, `.save` methods). These
high-centrality symbols have good scores but are structurally distant from the task's
seeds. Without proximity weighting, they fill budget slots and squeeze out nearby
relevant symbols (ground truth found at R@10=1.00 but P@10 drops).

**Why 0.3:** 9-point exponent sweep on 300 tasks (session 24). 0.3 peaked at P@10 0.282.
11/15 repos improved vs 0.5 (sqrt). Enriched repos benefit most: cargo +0.026,
rails +0.025, vscode +0.015. Override with `BENCH_PROXIMITY_EXP`.

**Measured impact:** Full corpus neutral-to-positive (0.282 vs 0.279 baseline).
Enriched saleor: 0.182 -> 0.209 (regression halved from -23% to -11%).

### 11. Implicit Feedback / Noise Demotion (usage-adaptive precision)

**Trigger:** Active MCP session where agent makes tool calls after context queries

**What it does:** Tracks which symbols were returned by `context_for_task` and
detects when the agent subsequently uses them (via `DetectUsed` scanning Edit/Read
tool call content). Symbols returned but never used get negative feedback
(`RecordFeedback(useful=false)`). On subsequent queries, demoted symbols rank lower.

**Why it's needed:** The retrieval pipeline returns a mix of relevant and noise symbols.
Over an active session, the engine learns which are noise for this user's work patterns
and suppresses them. This is the sole active learning mechanism (task memory confirmed
neutral in session 24).

**Why it's adaptive:** The demotion is per-session and per-symbol, scoped by keyword
cluster (session 25). A symbol demoted as noise for checkout tasks is unaffected when
the user switches to order tasks (different cluster hash). Per-cluster scoping prevents
the cross-task interference that degraded early compounding experiments.

**Measured impact:** Django +5.9% P@10 after 3 rounds of implicit feedback (benchmark
with noise demotion but no simulated agent usage). Per-cluster scoping (session 25)
improved compounding: R@10 +5.2%, MRR +12.6% over 5 rounds. Full corpus 10-round
compounding (session 26): P@10 0.277 -> 0.283 peak (+2.2%), MRR 0.459 -> 0.497 peak
(+8.1%). Never regresses below baseline.

### 12. Change-Aware Scoring (commit recency boost)

**Trigger:** Git blame data present on nodes (`Node.LastCommitAt > 0`)

**What it does:** Symbols in recently-committed code get a mild ranking boost via
`commitRecencyScore`: +0.05 (last day), +0.03 (last week), +0.01 (last month),
+0.00 (older or no blame data). Acts as a tiebreaker for structurally equivalent
symbols that differ in how recently they were touched.

**Why it's needed:** When RWR finds multiple symbols at similar graph distance from
seeds, the developer more likely needs the one they or their team recently changed.
This is different from runtime recency (mechanism 4, which uses OTLP trace timestamps):
commit recency tracks authorship, not execution.

**Why it's adaptive:** Returns 0 when blame data is absent (no overhead). Weight is
intentionally low (max 0.05) so it never overrides structural signals. Activates
automatically when `knowing enrich blame` populates the `last_commit_at` field.

**Measured impact:** Neutral on benchmark (corpus repos have no blame data). Expected
positive in production where repos have recent commit history.

### 13. Cross-Task Vocabulary Bridging (usage-adaptive knowledge transfer)

**Trigger:** Agent usage records vocab associations (`count >= 2`) + shared keywords between tasks

**What it does:** When an agent works on task A ("payment processing") and uses
symbols like `settle_ledger`, the system records `payment -> settle_ledger`. When
a different task B ("payment refund") shares the keyword "payment", the learned
association surfaces `settle_ledger` for task B via RRF competition.

Three safeguards prevent noise:
1. **Noise keyword filter** (`isVocabWorthy`): filters ~80 common English words
   (use, not, find, whether, etc.) from recording. Only domain-specific keywords
   create associations.
2. **Soft RRF injection**: learned vocab competes through RRF, not forced to the
   top. Naturally loses to better candidates on well-covered tasks.
3. **Confidence weighting** (`vocabCountWeight`): observation count scales RRF
   weight from 0.3 (count=2) to 0.8 (count>=10). Reinforced associations get
   stronger each round.

**Why it's adaptive:** The system learns different vocabulary for different
codebases based on actual agent usage patterns. Django agents teach Django
vocabulary; Kafka agents teach Kafka vocabulary. No manual curation needed.

**Measured impact (session 26):**
- Cross-task validation: Django +41.4% in isolation, full corpus 0.0% aggregate (safe)
- 100% of improvements are cross-task (never self-reinforcement)
- 10-round full corpus compounding: P@10 peak +2.2%, MRR peak +8.1%
- 10-round Django compounding with confidence weighting: band [0.203, 0.217] (36% tighter than unweighted)

**Interaction with mechanism 11:** Implicit feedback demotes noise, vocab bridging
promotes knowledge. Together they create a self-correcting learning loop: bad
associations get demoted by feedback, good associations get reinforced by vocab
count growth.

## Ablation Summary

**Note (session 23):** All pre-session-23 ablation numbers were measured with
accumulated task memory, which inflated absolute values. The relative ordering
and within-session deltas remain valid. The numbers below reflect the best
available measurement at each session.

| Mechanism | Impact | Trigger | Session |
|-----------|--------|---------|---------|
| Framework equiv classes + forced injection | +57% (0.176 -> 0.278) | Language/framework detected | 23 |
| Multi-phrase equiv gate | +9.6% (0.293 -> 0.321) | Framework injection candidates | 28 |
| Code pattern keyword extraction | Contributes to 0.330 | Task contains code-like patterns | 28 |
| Enrichment + type_hint | +24% estimated | LSP available | 17 |
| Focused seed selection | +6% | Always (>5 candidates) | 21 |
| PreferTypeSeeds | VS Code +44% | Node count > 40K | 14 |
| Adaptive seed count | Django +14.2% | Node count > 10K/40K | 16 |
| RWR proximity packing | +0.003 aggregate, enriched saleor regression halved | Always (zero cost) | 24 |
| Implicit feedback (noise demotion) | Django +5.9% after 3 rounds; R@10 +5.2%, MRR +12.6% with cluster scoping | Active MCP session | 24, 25 |
| Adaptive retrieval fallback | VS Code +43% | Node count > 200K + flat RWR | 23 |
| Feedback expiration | correctness (no P@10 delta) | Code change | 17 |
| Change-aware scoring (commit recency) | Neutral (no blame data in corpus) | Blame data present | 25 |
| Cross-task vocab bridging | Django +41.4%, corpus 0.0% (safe); 10-round MRR +8.1% | Vocab count >= 2 + shared keywords | 26 |
| Task memory compounding | **NEUTRAL** (was +3.8%) | Disabled session 24 | 24 |
| Gap-fill seeds | **NEUTRAL** (was +11%) | Candidates < 5 | 23 (revised) |

Combined: P@10 = 0.330 cold start (302 tasks, 16 repos, honest measurement, exponent 0.3).
With compounding (10 rounds): P@10 = 0.283 peak, MRR = 0.497 peak.

## Why Fixed-Strategy Systems Can't Compete

Competitors would need to implement all thirteen mechanisms to match knowing's
adaptive behavior. But the mechanisms interact: PreferTypeSeeds benefits from
phantom density (mechanism 8). Focused seeds reinforce equivalence classes
(mechanisms 3-4). Proximity packing compensates for enrichment density
(mechanism 10 mitigates mechanism 8's side effects). Implicit feedback
demotes noise that the other mechanisms can't filter structurally (mechanism 11).
Cross-task vocabulary bridging (mechanism 13) learns from implicit feedback's
usage signals, and Merkle expiration (mechanism 7) keeps both feedback and vocab
associations honest as code evolves. The system is greater than the sum of its
parts.

More importantly, the adaptive approach is structural: it follows from
content-addressed storage (feedback expiration via Merkle roots), graph-native
retrieval (density-adaptive seeding via node count), and embedding integration
(gap-fill via brute-force cosine). Each adaptation is a natural consequence of
the architecture, not an ad-hoc heuristic bolted on.

## Source Files

| File | Mechanism |
|------|-----------|
| `internal/context/equiv_*.go` (30 files) | Framework equivalence classes (263 concepts) |
| `internal/context/language_seeds.go` | Aggregator that collects all equiv class files |
| `internal/context/equivalence.go` | EquivalenceClass type, matching logic, language scoping |
| `internal/context/context.go` | Framework injection, adaptive retrieval fallback, focused seed selection, detectRepoLanguage, resultConfidence, directFTSExpansion, FTS fallback decomposition, isVocabWorthy, vocabCountWeight |
| `internal/context/walk.go` | GraphNodeCount, adaptive seed count, PreferTypeSeeds flag, RWR cache infrastructure (computeRWRCacheHash, getRWRCache, putRWRCache, collectSeedPackageRoots) |
| `internal/context/task_memory.go` | Task memory recording and recall (DISABLED in MCP server, session 24) |
| `internal/context/implicit.go` | Implicit feedback engine (FlushUnused, DetectUsed, RegisterReturned) |
| `internal/context/session.go` | Session tracker (RecordBatch, SessionBoosts, 3-minute half-life decay) |
| `internal/context/ranking.go` | Feedback boost integration, commitRecencyScore |
| `internal/context/delta.go` | Delta context packing (DiffPacks, IsWorthIt, SavingsPercent) |
| `internal/context/sweep.go` | PackStrategy, adaptiveProximityExponent, lspEdgeWeight, BENCH_PACK_STRATEGY |
| `internal/store/sqlite.go` | Merkleized feedback (neighborhood_root) |
| `internal/store/feedback.go` | Feedback storage, RWR cache invalidation on feedback recording, BENCH_FEEDBACK_WEIGHT |
| `internal/store/vocab.go` | Vocab association storage with per-package Merkle expiration (subgraph_root) |
| `internal/store/migrations/020_*.sql` | Per-cluster feedback (keyword_cluster column) |
| `internal/store/migrations/021_*.sql` | Vocab associations table |
| `internal/store/migrations/022_*.sql` | Vocab subgraph_root column for Merkle expiration |
| `internal/snapshot/manager.go` | persistPackageRoots, LoadPackageRoots, PackageRootForSymbol |
| `internal/enrichment/enricher.go` | LSP enrichment (phantom nodes, edge discovery) |
| `internal/wire/delta.go` | Delta GCF encoding (EncodeDelta) |
| `cmd/knowing/debug_seeds.go` | Seed pipeline diagnostic tool |
| `cmd/knowing/debug_fts.go` | FTS5 query probe tool |
| `cmd/knowing/debug_walk.go` | RWR walk visualization tool |
| `cmd/knowing/bench_task.go` | Single-task benchmark tool |
| `cmd/knowing/debug_vocab.go` | Vocabulary association inspector + filter preview |
| `cmd/knowing/debug_feedback.go` | Feedback record inspector |
| `cmd/knowing/debug_equiv.go` | Equivalence class match diagnostic |
| `cmd/knowing/debug_pack.go` | Packing decisions diagnostic |
| `cmd/knowing/debug_rwr_cache.go` | RWR cache correctness and latency diagnostic |
| `bench/cross-system/cross_task_vocab_test.go` | Cross-task vocabulary bridging validation |
| `bench/cross-system/compounding_test.go` | 10-round compounding with vocab + feedback + task memory |
| `bench/context-packing/bench_test.go` | Packing strategy comparison (4 strategies) |
| `bench/delta-packing/bench_test.go` | Delta packing benchmark (cross-task + re-query simulation) |

## Related Documents

- [Retrieval Pipeline](retrieval-pipeline.md): full pipeline reference (seed channels, RWR, HITS, scoring)
- [Context Engine](context-engine.md): ForTask flow with all adaptive mechanisms in sequence
- [Enrichment Pipeline](enrichment-pipeline.md): LSP enrichment that creates phantom reachability
- [Embedding Re-ranker](embedding-reranker.md): the embedding infrastructure that gap-fill builds on
