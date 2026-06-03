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
    Concept    string   // canonical concept ID (e.g., "DJANGO_VALIDATORS")
    Phrases    []string // natural-language phrases that refer to this concept
    Targets    []string // symbol/tool identifiers to boost when phrases match
    TargetType string   // "symbol", "mcp_tool", "edge_type", "workflow", "file"
    Weight     float64  // source strength (seed: 1.0, framework: 0.9, universal: 0.8, graph: 0.7)
    Source     string   // "seed", "framework", "universal", "language", "graph", "feedback"
    Lang       string   // language scope: "go", "python", "typescript", "ruby", "java", "csharp", "rust", "" (universal)
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
- **Weight**: Controls the confidence of the source. Seed classes carry 1.0,
  framework-specific carry 0.9 (triggers forced injection), universal and
  language-specific carry 0.8, graph-derived carry 0.7.
- **Source**: Tracks provenance and determines injection behavior.
  Values: `"seed"`, `"framework"`, `"universal"`, `"language"`, `"graph"`,
  `"learned"`. Classes with `Source == "framework"` AND `Weight >= 0.9`,
  
  bypass RWR scoring and inject directly into ranked results (forced injection). Learned vocab goes through RRF competition (soft injection).
- **Lang**: Language scope. When non-empty, the class only fires on repos
  where `detectRepoLanguage()` returns a matching language. Prevents
  cross-language false positives (Go router classes on C# repos).

## Six layers

The equivalence class system operates in six layers, each with different
confidence levels, generation methods, and injection behaviors.

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

### Layer 3: Framework-specific classes (forced injection, weight 0.9)

Defined in 30 per-framework files (`internal/context/equiv_*.go`), aggregated
by `languageEquivalenceClasses()` in `internal/context/language_seeds.go`.
There are currently **263 framework-specific classes** covering:

| Framework | File | Classes | Example |
|-----------|------|---------|---------|
| Django | `equiv_django.go` | 13 | "custom validator" -> EmailValidator, BaseValidator |
| Flask | `equiv_flask.go` | 11 | "flask blueprint" -> Blueprint, register_blueprint |
| FastAPI | `equiv_fastapi.go` | 14 | "dependency injection" -> Depends, solve_dependencies |
| Terraform | `equiv_terraform.go` | 18 | "graph transformer" -> GraphTransformer, AcyclicGraph |
| Kubernetes | `equiv_kubernetes.go` | 14 | "deployment controller" -> DeploymentController |
| Kafka | `equiv_java.go` | 11 | "consumer group" -> KafkaConsumer, ConsumerRecord |
| Rails | `equiv_rails.go` | 16 | "active storage" -> has_one_attached, Variant |
| Spring | `equiv_java.go` | 4 | "spring controller" -> RestController, GetMapping |
| ASP.NET/Ocelot | `equiv_csharp.go` | 30 | "security policy" -> SecurityMiddleware, ISecurityPolicy |
| Caddy | `equiv_caddy.go` | 9 | "automatic https" -> App.automaticHTTPSPhase1 |
| VS Code | `equiv_vscode.go` | 14 | "code action" -> CodeActionOracle, ManagedCodeActionSet |
| Cargo | `equiv_cargo.go` | 9 | "dependency resolution" -> FeatureResolver, resolve |
| Spark-Java | `equiv_sparkjava.go` | 8 | "route handler" -> Routes.find, RouteEntry |
| NestJS | `equiv_nestjs.go` | 4 | "guard" -> CanActivate, AuthGuard |
| Next.js | `equiv_nextjs.go` | 3 | "server-side rendering" -> getServerSideProps |
| Angular | `equiv_angular.go` | 4 | "angular component" -> Component, OnInit |
| React | `equiv_react.go` | 3 | "custom hook" -> useState, useEffect |
| Jekyll | `equiv_jekyll.go` | 2 | "jekyll plugin" -> Generator, Converter |
| Cross-cutting | `equiv_testing.go` etc. | 40 | "unit test" -> TestCase; "jwt token" -> JWT, Claims |

Framework classes carry weight 0.9 and source `"framework"`. This triggers
**forced injection**: matched symbols bypass RWR scoring and inject directly
at the top of the ranked results. This is the mechanism that solves the
vocabulary gap: when the task says "custom validator" but the symbol is
`EmailValidator`, BM25 can't find it, but the equiv class maps directly.

Each class has a `Lang` field restricting it to matching repos.
`detectRepoLanguage()` samples node QNs to determine the primary language.
This prevents Go router classes from firing on C# repos.

**Defensibility criterion:** Every framework class must pass: "would this
mapping appear in the framework's official documentation or tutorials?"
Application-specific internals are rejected as curve-fitting.

**Measured impact (session 23):** P@10 0.176 -> 0.278 (+57%).

### Layer 4: Language-specific classes (vocabulary bridging, weight 0.8)

Also defined in the `equiv_*.go` files and aggregated by
`languageEquivalenceClasses()`. These are broader language-level vocabulary
bridges (not framework-specific) with weight 0.8. They enter the RRF pipeline
as a normal channel, not through forced injection.

| Language | Concepts | Examples |
|----------|----------|---------|
| Python | entry point, routing, middleware, ORM, serialization, auth, template, errors, config, testing | 10 |
| TypeScript | components, state, routing, API, validation, type system, compiler, modules | 8 |
| Rust | error/Result, async/tokio, traits/impl, build/cargo, builder pattern, testing | 6 |
| Go | HTTP handler, router, middleware, server | 4 |

### Layer 5: Graph-derived aliases (auto-generated, weight 0.7)

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

Note: verb expansion applies to seed and universal classes only. Language-specific
classes (Layer 3) do not expand with verbs because their phrases are typically
short single-word triggers (e.g., "route", "component", "trait") where verb
prefixes would produce low-quality matches.

## How equivalence results enter the RRF pipeline

In `ForTask()` in `internal/context/context.go`, equivalence matching runs as
one of the active channels in the Reciprocal Rank Fusion pipeline:

```go
candidates := rrfFuseMulti([]rankedChannel{
    {nodes: tieredResults, weight: 2.0},   // Channel 1: keyword tiers (exact/prefix)
    {nodes: bm25Results, weight: 2.0},     // Channel 2: BM25 full-text
    {nodes: equivResults, weight: 2.0},    // Channel 3: equivalence classes
    {nodes: vectorResults, weight: 0.0},   // Channel 4: vector (disabled)
}, 60, 40)
```

All three active channels (tiered, BM25, equivalence) carry equal weight 2.0.
Tiered matching finds exact/prefix symbol hits; BM25 adds relevance-ranked
signature matching and multi-term queries; equivalence provides concept-level
bridging for zero-overlap vocabulary gaps. Equal weighting lets RRF promote
symbols that appear in multiple channels without any single channel dominating.

The fusion process for equivalence results:

1. Seed + universal + language-specific classes are combined and matched against
   the task description
2. The top 10 tiered results serve as input to `graphDerivedAliases()`, which
   generates additional classes from graph structure
3. Graph-derived matches are appended to seed/universal/language matches
4. For each match, targets are filtered to remove generic symbols (<=3 chars or
   common method names like "get", "set", "do", "new", "run", "put", "post",
   "call", "add", "pop") that produce excessive false positives
5. Surviving targets are resolved by querying `NodesByName` and filtering for
   exact symbol name matches (case-insensitive)
6. The resolved nodes form the `equivResults` list, capped at
   2x(tiered+BM25 count) to prevent channel domination in RRF fusion
7. The capped list enters RRF fusion

Symbols that appear in both the equivalence channel and another channel
(tiered or BM25) receive accumulated scores from both, promoting them higher in
the final ranking.

### Channel balance constraint (Run 22 finding)

On small graphs (< 3000 non-external nodes), unbounded equivalence results
dominate RRF fusion. When equiv returns 66 results against 8 tiered + 3 BM25,
the equiv channel controls most of the RRF rank positions. After fusion, RWR
gives all seeds identical scores (~0.38) because the graph is too small for
the random walk to differentiate. The fix: cap equiv results at
2x(tiered+BM25) and filter generic targets. This produced +136% P@10
improvement (0.101 -> 0.226).

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

## How to add framework equivalence classes

To add a new framework class, create or edit the appropriate file in
`internal/context/equiv_<framework>.go`, then add it to the aggregator
in `internal/context/language_seeds.go`.

Example: adding Django cache support:

```go
// in internal/context/equiv_django.go
{
    Concept:    "DJANGO_CACHE",
    Phrases:    []string{"cache backend", "cache framework", "django cache", "cache stampede"},
    Targets:    []string{"BaseCache", "BaseCache.get", "BaseCache.set", "RedisCache", "CacheHandler"},
    TargetType: "symbol",
    Weight:     0.9,
    Source:     "framework",
    Lang:       "python",
},
```

Then add to the aggregator in `language_seeds.go`:
```go
all = append(all, djangoEquivalenceClasses()...)
```

Guidelines for writing framework classes:

1. **Phrases must be what developers actually type.** "custom validator" is
   good; "instantiate a BaseValidator subclass" is not. The test: would this
   phrase appear in a Stack Overflow question title?
2. **Targets must be documented framework symbols.** Check the framework's
   official docs. `EmailValidator` appears in Django docs. `_internal_helper`
   does not.
3. **Set `Lang` to scope by language.** Prevents cross-language false positives.
4. **Use `Weight: 0.9` and `Source: "framework"`.** This triggers forced
   injection (bypass RWR).
5. **Defensibility test:** "Would this mapping appear in the framework's
   official documentation or tutorials?" If no, it's curve-fitting.
6. **Keep targets focused.** 3-8 targets per concept. Too many floods results.
7. **Use `bench-task` to verify.** Run the specific task before and after
   adding the class to confirm improvement.

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

**3. Compounds with learning.** The six layers are designed to compound:

- Seed classes provide high-confidence bootstrap (weight 1.0)
- Universal classes extend coverage to common patterns (weight 0.8)
- Framework-specific classes bridge framework vocabulary with forced injection (weight 0.9)
- Language-specific classes bridge non-Go vocabulary (weight 0.8)
- Graph-derived aliases auto-generate from structure (weight 0.7)
- Learned vocab associations accumulate from agent usage via RRF (confidence-weighted 0.3-0.8, source "learned")

Each layer adds value independently, and they combine through RRF fusion without
interfering with each other. Learned vocab associations (Layer 6) are the active
learning mechanism: when agents use symbols after context queries, the keyword ->
symbol association is recorded (`vocab_associations` table, migration 021). After
2+ observations, the association becomes a learned equivalence class with soft
injection. Per-cluster scoping (migration 020) prevents cross-task interference.

### Layer 6: Learned vocabulary associations (from usage, weight 0.5)

Generated at query time from the `vocab_associations` table
(`internal/store/vocab.go`). When an agent uses a symbol after a
`context_for_task` query, the keyword -> symbol association is recorded.
After 2+ observations (`count >= 2`), the association activates as a learned
equivalence class.

Learned vocab classes go through **RRF competition** (soft injection, unlike Layer 3
framework classes which use forced injection). This prevents displacement of correct results on tasks with good BM25 coverage.
Confidence weighting scales RRF weight from 0.3 (count=2) to 0.8 (count>=10), rewarding reinforced associations.

Per-cluster scoping: feedback and vocab associations are scoped to keyword
clusters (`keyword_cluster` column on feedback table, migration 020). The
cluster is derived from sorted primary keywords. Noise demotion for "checkout"
queries doesn't affect "order" queries.

**Measured impact (session 25):** Cross-task validation on Django: simulated
prior usage of "inlines" -> `get_inlines`, `get_fieldsets` moved
django-swe-003 from P@10=0.000 to P@10=0.200. Compounding test: R@10 peak
0.378 over 5 rounds.
