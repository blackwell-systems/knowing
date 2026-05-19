# Equivalence Classes

Equivalence classes are knowing's primary mechanism for bridging the vocabulary
gap between natural-language task descriptions and code symbol names. When a
developer writes "find the blast radius of this change," they mean
`TransitiveCallers`. When they say "what tests should I run," they mean
`cmdTestScope` and `findAffectedTests`. Equivalence classes encode these
mappings explicitly.

## The vocabulary gap problem

Code retrieval systems face a fundamental mismatch: developers describe work in
natural language, but the targets they need are symbol names in CamelCase or
snake_case. Standard text search (BM25, substring matching) cannot bridge this
gap because the words developers use often share zero lexical overlap with the
symbols they need.

Examples of zero-overlap queries:

| Task description | Target symbol | Lexical overlap |
|-----------------|---------------|-----------------|
| "blast radius" | `TransitiveCallers` | none |
| "what tests to run" | `cmdTestScope` | none |
| "semantic diff" | `SnapshotDiff` | none |
| "dead routes" | `ConfidenceFromCount` | none |

Off-the-shelf embeddings (MiniLM-L6-v2, BGE-small-en-v1.5) were tested
extensively (experiments 9-12) and failed to bridge this gap. General-purpose
models do not understand that "blast radius" maps to `TransitiveCallers`
without domain-specific training. Equivalence classes solve this with explicit,
deterministic, zero-dependency mappings.

## The EquivalenceClass struct

Defined in `internal/context/equivalence.go`:

```go
type EquivalenceClass struct {
    Concept    string   // canonical concept ID (e.g., "TRANSITIVE_IMPACT")
    Phrases    []string // natural-language phrases that refer to this concept
    Targets    []string // symbol/tool identifiers to boost when phrases match
    TargetType string   // "symbol", "mcp_tool", "edge_type", "workflow", "file"
    Weight     float64  // source strength (seed: 1.0, universal: 0.8, graph: 0.7)
    Source     string   // "seed", "universal", "graph", "feedback", "generated"
}
```

Each field serves a distinct purpose:

- **Concept**: A unique identifier for the concept (e.g., `TRANSITIVE_IMPACT`,
  `TEST_SELECTION`). Used for debugging and deduplication.
- **Phrases**: The natural-language phrases that trigger this class. Matched
  case-insensitively as substrings against the task description.
- **Targets**: The symbol or tool names to boost when a phrase matches. These
  are resolved against the graph to find actual nodes.
- **TargetType**: Categorizes what the targets represent. Currently all seed
  classes use `"symbol"`.
- **Weight**: Controls the confidence of the source. Higher weight means
  stronger boost in the RRF fusion.
- **Source**: Tracks provenance for debugging and potential future decay.

## Three layers

The equivalence class system operates in three layers, each with different
confidence levels and generation methods.

### Layer 1: Seed classes (repo-specific, weight 1.0)

Defined in `seedEquivalenceClasses()` in `internal/context/equivalence.go`.
These are hand-curated mappings specific to the knowing codebase. There are
currently 21 seed concepts covering the core domain:

| Concept | Example phrases | Example targets |
|---------|----------------|-----------------|
| `TRANSITIVE_IMPACT` | "blast radius", "downstream callers" | `TransitiveCallers`, `BlastRadius` |
| `TEST_SELECTION` | "affected tests", "what tests to run" | `cmdTestScope`, `findAffectedTests` |
| `DATAFLOW_TRACE` | "trace flow", "call chain" | `TransitiveCallees`, `traceDataflowTool` |
| `SNAPSHOT_DIFF` | "what changed", "semantic diff" | `SnapshotDiff`, `SemanticDiff` |
| `CONTEXT_PACKING` | "relevant context", "token budget" | `ContextEngine`, `ForTask`, `RankSymbols` |
| `EXTRACTOR` | "language extractor", "tree-sitter" | `Extractor`, `GoTreeSitterExtractor` |

Seed classes carry weight 1.0 (highest confidence) because a human verified
each phrase-to-target mapping.

### Layer 2: Universal classes (any-repo, weight 0.8)

Defined in `universalEquivalenceClasses()` in
`internal/context/universal_seeds.go`. These capture concepts common to
virtually all software projects (Go, TypeScript, Python, Rust, Java) and
require no repo-specific knowledge. There are currently 63 universal concepts:

| Concept | Example phrases | Example targets |
|---------|----------------|-----------------|
| `ENTRY_POINT` | "main function", "startup" | `main`, `Init`, `Bootstrap` |
| `ERROR_HANDLING` | "error wrapping", "panic recovery" | `Error`, `Wrap`, `HandleError` |
| `DATABASE` | "db connection", "persistence" | `DB`, `Store`, `Query`, `Migrate` |
| `HTTP_SERVER` | "rest api", "router", "routes" | `Server`, `Router`, `HandleFunc` |
| `AUTHENTICATION` | "auth", "jwt", "credentials" | `Auth`, `Token`, `ValidateToken` |
| `CONCURRENCY` | "goroutine", "worker pool", "mutex" | `Worker`, `Pool`, `WaitGroup` |

Universal classes carry weight 0.8 (slightly lower than seed) because their
targets are generic symbol names that may match unrelated code. They ship as
the default for any repo, providing baseline retrieval quality before
repo-specific seeds are added.

The cross-repo eval on gortex (an external Go codebase with no knowing-specific
seeds) achieved 46.7% R@10 overall, with 60% on exact-match queries and 60% on
multi-hop queries, demonstrating the value of universal classes.

### Layer 3: Graph-derived aliases (auto-generated, weight 0.7)

Defined in `graphDerivedAliases()` in `internal/context/graph_aliases.go`.
These are generated automatically from the graph structure at query time. For
each candidate seed node, the system inspects callers and callees, splits their
names into component words, filters out generic terms, and creates targeted
phrase mappings back to the original node.

Example: `TransitiveCallers` is called by `handleBlastRadius`. Splitting
`handleBlastRadius` yields `["handle", "Blast", "Radius"]`. Filtering out the
generic word `"handle"` leaves `["blast", "radius"]`. These become phrases that
map back to `TransitiveCallers`.

Graph-derived aliases carry weight 0.7 (lowest) because they are
auto-generated and inherently noisier. The system limits input to the top 10
tiered results (highest quality seeds) to avoid amplifying noise from
loosely-related nodes.

The generation process:

1. For each seed hash, retrieve the node from the store
2. Query `EdgesTo` (callers) and `EdgesFrom` (callees) with edge type `"calls"`
3. For each neighbor, split the qualified name via CamelCase boundaries
4. Filter against a generic word list (`handle`, `new`, `get`, `set`, `test`, etc.)
5. Remove words shorter than 3 characters
6. Remove words already present in the target name (redundant)
7. Generate bigram phrases from consecutive word pairs
8. Create an `EquivalenceClass` with concept `"GRAPH_" + targetName`

## How phrase matching works

Implemented in `matchEquivalenceClasses()` in
`internal/context/equivalence.go`:

```go
func matchEquivalenceClasses(query string, classes []EquivalenceClass) []equivalenceMatch {
    queryLower := strings.ToLower(query)
    var matches []equivalenceMatch

    for _, cls := range classes {
        for _, phrase := range cls.Phrases {
            if strings.Contains(queryLower, strings.ToLower(phrase)) {
                matches = append(matches, equivalenceMatch{
                    class:   cls,
                    phrase:  phrase,
                    targets: cls.Targets,
                    weight:  cls.Weight,
                })
                break // one match per class is enough
            }
        }
    }

    return matches
}
```

Key properties:

- **Case-insensitive**: Both the query and phrases are lowercased before
  comparison.
- **Substring matching**: A phrase matches if it appears anywhere in the task
  description. "Find the blast radius" matches the phrase "blast radius".
- **One match per class**: Once any phrase in a class matches, the class is
  included and remaining phrases are skipped. This prevents a single class from
  dominating results when multiple of its phrases appear.
- **All targets included**: When a class matches, all of its targets are
  resolved against the graph.

## Cross-product expansion with action verbs

Before matching, all phrase lists are expanded with action verb prefixes.
The `expandWithVerbs()` function prepends each of 10 common developer verbs to
every noun phrase:

```go
var actionVerbs = []string{
    "find", "get", "compute", "show", "list",
    "trace", "check", "run", "detect", "analyze",
}
```

This turns a phrase like "blast radius" into 10 additional variants: "find
blast radius", "get blast radius", "compute blast radius", and so on.

The expansion only applies to phrases that do not already start with a verb,
preventing double-prefixing. A phrase like "find symbol" (already verb-prefixed)
is left unchanged.

This cross-product approach is why a small number of seed concepts (21)
produces 200+ matchable phrases. Each concept with 8 noun phrases generates
approximately 88 total phrases (8 original + 80 verb-expanded).

## How equivalence results enter the RRF pipeline

In `ForTask()` in `internal/context/context.go`, equivalence matching runs as
Channel 4 in the Reciprocal Rank Fusion pipeline:

```go
candidates := rrfFuseMulti([]rankedChannel{
    {nodes: tieredResults, weight: 3.0},   // Channel 1: keyword tiers
    {nodes: equivResults, weight: 2.0},    // Channel 4: equivalence classes
    {nodes: bm25Results, weight: 1.0},     // Channel 2: BM25 full-text
    {nodes: vectorResults, weight: 0.0},   // Channel 3: vector (disabled)
}, 60, 40)
```

The equivalence channel has weight 2.0, making it the second-strongest signal
after tiered keyword matching (3.0) and twice as strong as BM25 (1.0). This
weighting reflects measured precision: equivalence matches are concept-level
(high confidence) while BM25 is lexical (lower confidence).

The fusion process for equivalence results:

1. Seed + universal classes are combined and matched against the task description
2. The top 10 tiered results serve as input to `graphDerivedAliases()`, which
   generates additional classes from graph structure
3. Graph-derived matches are appended to seed/universal matches
4. For each match, all targets are resolved by querying `NodesByName` and
   filtering for exact symbol name matches (case-insensitive)
5. The resolved nodes form the `equivResults` list, which enters RRF fusion

Symbols that appear in both the equivalence channel and another channel
(tiered or BM25) receive accumulated scores from both, promoting them higher in
the final ranking.

## Measured impact

### Experiment 18: Initial equivalence classes (+8pp hard tier)

Adding 20 seed equivalence classes with 200+ phrases was the single biggest
retrieval improvement measured:

| Tier | Before | After | Delta |
|------|--------|-------|-------|
| Easy | 36.5% | 38.5% | +2.0pp |
| Medium | 29.5% | 32.0% | +2.5pp |
| Hard | 10.0% | 18.0% | +8.0pp |
| Overall | 26.7% | 30.5% | +3.8pp |
| MRR | 0.46 | 0.53 | +0.07 |

### Experiment 19: Expanded phrases + EXTRACTOR concept (+3.3pp hard tier)

Adding missing phrases to existing classes and a new concept:

| Tier | Before | After | Delta |
|------|--------|-------|-------|
| Hard | 18.0% | 21.3% | +3.3pp |
| Overall | 30.5% | 31.6% | +1.1pp |
| MRR | 0.53 | 0.58 | +0.05 |

### Cross-repo eval: gortex (+6.7pp overall from universal seeds)

On an external Go codebase with no knowing-specific seeds, the pipeline
(including universal equivalence classes and graph-derived aliases) achieved
46.7% R@10 overall. The concept-tier queries (the ones most dependent on
equivalence classes) scored 20%, while exact-match and multi-hop queries
reached 60%.

### What failed: untargeted alternatives

Experiment 20 tested BM25 neighbor enrichment (appending caller/callee names
to the FTS index) as an alternative to graph-derived equivalence classes.
Result: -1.8pp overall regression. The key insight from that experiment:
equivalence classes work because they are targeted (specific phrases to specific
targets with explicit intent). Untargeted text expansion into BM25 cannot
distinguish "this neighbor is conceptually relevant" from "this neighbor
happens to be connected."

Auto-generated concepts from CamelCase-split symbol names also tested neutral
(referenced as experiment 21 in code comments). CamelCase splitting already
makes symbol names searchable via BM25, so auto-concepts only add value when
they generate conceptual aliases that differ from the name, which requires
domain understanding.

## How to add new seed concepts

To add a new seed concept to the knowing-specific dictionary, edit
`seedEquivalenceClasses()` in `internal/context/equivalence.go`.

Example: adding a concept for the ownership/CODEOWNERS system:

```go
{
    Concept:    "OWNERSHIP",
    Phrases:    []string{"code owner", "ownership", "who owns", "maintainer", "responsible for"},
    Targets:    []string{"OwnershipTool", "handleOwnership", "ownershipTool"},
    TargetType: "symbol",
    Weight:     1.0,
    Source:     "seed",
},
```

Guidelines for writing good seed concepts:

1. **Phrases should be what developers actually type.** Look at real task
   descriptions and agent prompts. "blast radius" is good; "compute the set of
   transitively reachable callers" is not.
2. **Targets should be the actual symbol names in the codebase.** Check that
   each target exists as a symbol; misspelled targets silently fail.
3. **Include both noun forms and common misspellings.** "reindex" and
   "re-indexing" are both valid phrases for the INDEXING concept.
4. **Do not include verb prefixes in phrases.** The `expandWithVerbs()` function
   adds "find X", "get X", etc. automatically. Write noun phrases only.
5. **Keep targets focused.** 3-8 targets per concept is typical. Too many
   targets dilutes the boost.

Adding phrases to existing concepts is cheap and safe. Near-zero risk of
regression, consistent returns (experiment 19).

## How to add universal concepts

To add a concept that works across any codebase, edit
`universalEquivalenceClasses()` in `internal/context/universal_seeds.go`.

Universal concepts differ from seeds in three ways:

1. **Weight is 0.8**, not 1.0, because targets are generic symbol names.
2. **Source is `"universal"`**, not `"seed"`.
3. **Targets must be common naming conventions.** `Config`, `Server`, `Handler`
   are good because most codebases use them. `SQLiteStore` is too specific.

Example: adding a concept for metrics/observability:

```go
{
    Concept:    "METRICS",
    Phrases:    []string{"metrics", "prometheus", "counter", "gauge", "histogram", "observability", "instrumentation"},
    Targets:    []string{"Metrics", "Counter", "Gauge", "Histogram", "Record", "Observe", "Instrument"},
    TargetType: "symbol",
    Weight:     0.8,
    Source:     "universal",
},
```

## Graph-derived alias generation

Graph-derived aliases are generated at query time in `graphDerivedAliases()`
(`internal/context/graph_aliases.go`). The process extracts meaningful words
from symbol names using `extractMeaningfulWords()`, which:

1. Takes the last component of a qualified name (after the last `/`)
2. Strips the package prefix (before the first `.`)
3. Splits CamelCase at uppercase boundaries and snake_case at underscores
4. Filters against a generic word list of ~35 terms (`handle`, `new`, `get`,
   `set`, `test`, `mock`, `err`, `ctx`, `server`, `store`, etc.)
5. Removes words shorter than 3 characters

The alias generation then:

1. Collects meaningful words from all callers and callees of each seed node
2. Deduplicates and removes words already in the target name
3. Generates bigram phrases from consecutive word pairs (e.g., `["blast",
   "radius"]` produces `"blast radius"`)
4. Creates an `EquivalenceClass` with the `"graph"` source and weight 0.7

This is targeted (specific phrases to specific targets) rather than untargeted
(dumping text into BM25). Only nodes already in the candidate pool (top 10
tiered results) serve as input, and only their direct neighbors contribute
words. This limits noise propagation from high-degree generic nodes like
`types.Hash` or `GraphStore`.

## Design principles

Three principles govern the equivalence class system, validated by experiments
18-20:

**1. Targeted beats untargeted.** Explicit (phrase, target) mappings with
declared intent outperform adding text to search indices. BM25 neighbor
enrichment (experiment 20) and doc comment indexing (experiment 17) both
regressed because they cannot distinguish conceptual relevance from incidental
connection. Equivalence classes encode the distinction directly.

**2. Local-first.** Seed classes are deterministic, inspectable, and have zero
external dependencies. No API calls, no model inference, no network round-trips.
This is why equivalence classes outperformed all embedding approaches tested
(experiments 9-12): the vocabulary mappings for a specific codebase are finite
and enumerable; a general model is an expensive way to approximate what a lookup
table does exactly.

**3. Compounds with feedback.** The three layers are designed to compound:

- Seed classes provide high-confidence bootstrap (weight 1.0)
- Universal classes extend coverage to common patterns (weight 0.8)
- Graph-derived aliases auto-generate from structure (weight 0.7)
- Feedback (future, weight 0.5) will accumulate (task, useful_symbol) pairs
  from real usage, reinforcing or extending existing concepts

Each layer adds value independently, and they combine through RRF fusion without
interfering with each other. Adding a new layer cannot degrade existing layers
because RRF only promotes nodes that appear in multiple channels.
