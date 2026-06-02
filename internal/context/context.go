package context

import (
	"bytes"
	"compress/zlib"
	stdctx "context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/internal/cache"
	"github.com/blackwell-systems/knowing/internal/types"
)

// FeedbackProvider is implemented by stores that support feedback queries.
type FeedbackProvider interface {
	FeedbackBoosts(ctx stdctx.Context, hashes []types.Hash, neighborhoodRoots map[types.Hash]types.Hash) (map[types.Hash]float64, error)
}

// BM25Searcher is implemented by stores that support full-text BM25 search.
// Returns nodes ordered by BM25 relevance (best matches first).
type BM25Searcher interface {
	SearchBM25Nodes(ctx stdctx.Context, query string, limit int) ([]types.Node, error)
}

// VectorSearcher provides semantic nearest-neighbor search over symbol embeddings.
type VectorSearcher interface {
	// EmbedAndSearch embeds the query text and returns the k nearest symbol node hashes.
	EmbedAndSearch(ctx stdctx.Context, query string, k int) ([]types.Hash, error)
}

// StoreSearcher provides brute-force cosine search from persisted vectors.
// Optional interface; when the HNSW index is empty (no embedding phase),
// the gap-fill falls back to this for O(n) search from SQLite-cached vectors.
type StoreSearcher interface {
	LoadAndSearchFromStore(ctx stdctx.Context, query string, k int) ([]types.Hash, error)
}

// VectorReRanker re-ranks candidates by embedding similarity to a query.
// Optional interface; if the VectorSearcher also implements this, the engine
// uses it to re-rank RWR output before packing.
type VectorReRanker interface {
	// ReRank embeds the query and each candidate text, returns indices sorted by
	// descending cosine similarity to the query.
	ReRank(ctx stdctx.Context, query string, candidates []string) ([]int, error)
	// ReRankScores embeds the query and each candidate, returns cosine similarity
	// scores (0.0-1.0) for each candidate at its original index position.
	ReRankScores(ctx stdctx.Context, query string, candidates []string) ([]float64, error)
	// ReRankByHashes re-ranks using cached vectors looked up by node hash.
	// Only embeds the query (1 inference call). Falls back to embedding
	// candidates on cache miss. Returns scores at original index positions.
	ReRankByHashes(ctx stdctx.Context, query string, hashes []types.Hash, fallbackTexts []string) ([]float64, error)
}

// FeedbackRecorder writes feedback to persistent storage.
// Separated from FeedbackProvider (reads) so engines can record implicit
// feedback without depending on the full store interface.
type FeedbackRecorder interface {
	RecordFeedback(ctx stdctx.Context, symbolHash types.Hash, sessionID string, useful bool, neighborhoodRoot types.Hash) error
}

// ContextEngine queries the knowing knowledge graph to produce task-specific,
// token-budgeted context blocks ranked by graph relationships and runtime traffic.
type ContextEngine struct {
	store    types.GraphStore
	feedback FeedbackProvider    // nil if store doesn't support feedback
	recorder FeedbackRecorder    // nil if store doesn't support recording
	implicit *ImplicitFeedback   // nil disables implicit feedback (noise demotion)
	bm25     BM25Searcher        // nil if store doesn't support BM25
	vector   VectorSearcher      // nil if embeddings not available
	session  *SessionTracker     // nil disables session-aware boosting
	memory   *TaskMemory         // nil disables passive task memory
	cache    *cache.SubgraphCache // nil disables subgraph result caching
	noPersistentCache bool       // skip notes-table pack cache (for benchmarks)
	nodeCount int                // per-engine node count for density-adaptive retrieval (0 = use global)
}

// SetNodeCount sets the per-engine node count for density-adaptive retrieval.
// When set (> 0), overrides the global GraphNodeCount. Thread-safe: each engine
// has its own field, no global mutation needed.
func (e *ContextEngine) SetNodeCount(n int) {
	e.nodeCount = n
}

// effectiveNodeCount returns the node count to use for density-adaptive decisions.
// Prefers per-engine field, falls back to global.
func (e *ContextEngine) effectiveNodeCount() int {
	if e.nodeCount > 0 {
		return e.nodeCount
	}
	return GraphNodeCount
}

// TaskOptions configures a task-based context query.
type TaskOptions struct {
	TaskDescription string
	TokenBudget     int    // default 50000
	Format          string // "xml", "markdown", "json"
	DBPath          string // path to knowing.db (for CLI usage)
	RepoURL         string // optional: scope search to this repo (filters out cross-repo noise)
}

// FileOptions configures a file-based context query.
type FileOptions struct {
	Files       []string // relative file paths
	RepoURL     string   // repo URL for resolving file hashes
	TokenBudget int      // default 50000
	Format      string   // "xml", "markdown", "json"
}

// PROptions configures a PR context query.
type PROptions struct {
	Files       []string // changed file paths (relative to repo root)
	RepoURL     string   // repo URL for resolving file hashes
	TokenBudget int      // default 8000 (larger than per-edit, used once per PR)
	Format      string   // "xml", "markdown", "json", "gcf"
}

// ContextBlock is the result of a context query: a ranked list of symbols
// that fit within a token budget, plus the edges between them.
type ContextBlock struct {
	Symbols     []RankedSymbol
	Edges       []ContextEdge
	Format      string
	TokensUsed  int
	TokenBudget int
	// PackRoot is the content-addressed identity of this context pack.
	// Computed from hash(task_normalized, snapshot_root, selected_node_hashes).
	// Two identical queries against the same graph state produce the same PackRoot,
	// enabling deduplication, citation, and cross-session replay.
	PackRoot types.Hash
}

// ContextEdge is an edge between two symbols in the context block.
type ContextEdge struct {
	Source   string // qualified name of source
	Target   string // qualified name of target
	EdgeType string
}

// RankedSymbol is a graph node paired with its computed relevance score
// and score breakdown.
type RankedSymbol struct {
	Node        types.Node
	Score       float64
	Components  ScoreComponents
	Provenance  string
	Distance    int     // binary 0/1 for scoring
	BFSDistance  int     // actual BFS hop count for proximity-weighted packing
	RWRScore    float64 // raw normalized RWR score (0-1), proxy for seed proximity in packing
}

// ScoreComponents breaks down a symbol's score into its weighted components.
type ScoreComponents struct {
	BlastRadius float64
	Confidence  float64
	Recency     float64
	Distance    float64
	Feedback    float64
	Session     float64
}

// NewContextEngine creates a ContextEngine backed by the given GraphStore.
// If the store implements FeedbackProvider, feedback-aware reranking is enabled.
func NewContextEngine(store types.GraphStore) *ContextEngine {
	e := &ContextEngine{store: store}
	if fp, ok := store.(FeedbackProvider); ok {
		e.feedback = fp
	}
	if fr, ok := store.(FeedbackRecorder); ok {
		e.recorder = fr
	}
	if bs, ok := store.(BM25Searcher); ok {
		e.bm25 = bs
	}
	return e
}

// SetImplicitFeedback attaches an implicit feedback tracker to the engine.
// When set, ForTask automatically flushes unused symbols from the previous
// call (recording negative feedback) and registers new returned symbols
// for attribution. This enables noise demotion: symbols returned but never
// used by the agent get penalized on future queries.
func (e *ContextEngine) SetImplicitFeedback(f *ImplicitFeedback) {
	e.implicit = f
}

// SetSession attaches a session tracker to the engine. When set, symbols
// returned by previous queries in this session receive a boost on subsequent
// queries. Pass nil to disable session-aware boosting.
func (e *ContextEngine) SetSession(st *SessionTracker) {
	e.session = st
}

// SetVector attaches a vector search backend to the engine.
func (e *ContextEngine) SetVector(vs VectorSearcher) {
	e.vector = vs
}

// DisablePersistentCache prevents the engine from reading/writing cached packs
// in the notes table. Used in benchmarks to ensure fresh retrieval on every query.
func (e *ContextEngine) DisablePersistentCache() {
	e.noPersistentCache = true
}

// reRankWithEmbeddings re-orders the top-N ranked symbols using embedding similarity
// to the task description. Takes the top 50 candidates, embeds them alongside the query,
// and reorders by cosine similarity. Symbols below top-50 retain their original order.
func (e *ContextEngine) reRankWithEmbeddings(ctx stdctx.Context, reranker VectorReRanker, ranked []RankedSymbol, task string) []RankedSymbol {
	// Only re-rank top 50 (embedding cost is proportional to count).
	reRankN := 50
	if len(ranked) < reRankN {
		reRankN = len(ranked)
	}
	if reRankN == 0 || task == "" {
		return ranked
	}

	// Build candidate hashes and fallback texts.
	hashes := make([]types.Hash, reRankN)
	candidates := make([]string, reRankN)
	for i := 0; i < reRankN; i++ {
		n := ranked[i].Node
		hashes[i] = n.NodeHash
		// Rich text: kind + name + signature + doc (used as fallback on cache miss)
		parts := []string{n.Kind, n.QualifiedName}
		if n.Signature != "" {
			parts = append(parts, n.Signature)
		}
		if n.Doc != "" {
			parts = append(parts, n.Doc)
		}
		candidates[i] = strings.Join(parts, " ")
	}

	// Try hash-based re-rank first (uses cached vectors, only embeds query).
	// Falls back to full re-embedding on cache miss.
	scores, err := reranker.ReRankByHashes(ctx, task, hashes, candidates)
	if err != nil || len(scores) != reRankN {
		return ranked // fallback to original order on error
	}

	// Blended scoring: weight=0.0 means pure re-rank by embedding (validated default).
	// Tunable via ReRankOriginalWeight. Higher = more conservative (preserves MRR).
	originalWeight := ReRankOriginalWeight
	embedWeight := 1.0 - originalWeight

	type blended struct {
		idx   int
		score float64
	}
	items := make([]blended, reRankN)
	for i := 0; i < reRankN; i++ {
		// Normalize original score to [0,1] relative to the top-N range.
		origNorm := ranked[i].Score / ranked[0].Score // top symbol = 1.0
		items[i] = blended{
			idx:   i,
			score: originalWeight*origNorm + embedWeight*scores[i],
		}
	}

	// Sort by blended score descending.
	sort.Slice(items, func(a, b int) bool {
		return items[a].score > items[b].score
	})

	// Rebuild ranked slice: blended top-N + unchanged tail.
	result := make([]RankedSymbol, len(ranked))
	for i, item := range items {
		result[i] = ranked[item.idx]
	}
	copy(result[reRankN:], ranked[reRankN:])
	return result
}

// SetTaskMemory attaches a task memory for passive retrieval learning.
// When set, past task-symbol associations boost future queries with
// similar keywords.
func (e *ContextEngine) SetTaskMemory(tm *TaskMemory) {
	e.memory = tm
}

// SetCache attaches a SubgraphCache for result memoization. When set,
// ForTask checks the cache before running retrieval and stores the result
// after a cache miss. Cache keys are derived from the normalized task
// description so that identical queries skip the full retrieval pipeline.
// Pass nil to disable caching.
func (e *ContextEngine) SetCache(c *cache.SubgraphCache) {
	e.cache = c
}

// stopWords is the set of words filtered from task descriptions during keyword extraction.
// Includes both English stop words and common Go/programming terms that match too broadly.
var stopWords = map[string]bool{
	// English
	"the": true, "a": true, "an": true, "in": true, "to": true, "for": true,
	"is": true, "of": true, "and": true, "or": true, "on": true, "at": true,
	"by": true, "it": true, "be": true, "as": true, "do": true, "no": true,
	"if": true, "so": true, "up": true, "my": true, "we": true, "all": true,
	"this": true, "that": true, "with": true, "from": true, "what": true,
	"how": true, "can": true, "will": true, "should": true, "would": true,
	"about": true, "into": true, "when": true, "where": true, "which": true,
	// Programming terms that match too many symbols
	"new": true, "get": true, "set": true, "add": true, "make": true,
	"init": true, "has": true, "err": true, "func": true, "type": true,
	"var": true, "const": true, "return": true, "import": true,
	"package": true, "struct": true, "interface": true, "string": true,
	"int": true, "bool": true, "error": true, "nil": true,
	// Pure action words: describe intent, never identify code.
	// NOTE: words that are ALSO feature names (rename, debug, trace, cache)
	// must NOT be in stopWords. They belong in actionVerbs only (for verb-pattern
	// detection) so they still appear in Components for BM25/tiered matching.
	"refactor": true, "fix": true, "update": true, "change": true,
	"modify": true, "implement": true, "create": true, "delete": true,
	"remove": true, "move": true, "improve": true,
	"optimize": true, "review": true,
}

// abbreviations maps common Go/programming abbreviations to their expansions.
// Used during keyword extraction to improve matching against qualified names.
var abbreviations = map[string]string{
	"ctx":    "context",
	"fmt":    "format",
	"req":    "request",
	"resp":   "response",
	"res":    "response",
	"cfg":    "config",
	"conf":   "config",
	"msg":    "message",
	"db":     "database",
	"srv":    "server",
	"svc":    "service",
	"mgr":    "manager",
	"pkg":    "package",
	"idx":    "index",
	"iter":   "iterator",
	"buf":    "buffer",
	"conn":   "connection",
	"addr":   "address",
	"auth":   "auth",
	"mux":    "multiplexer",
	"cmd":    "command",
	"args":   "arguments",
	"opts":   "options",
	"params": "parameters",
	"desc":   "description",
	"info":   "information",
	"stat":   "statistics",
	"stats":  "statistics",
}

// ForTask produces ranked context for a task description by finding relevant
// symbols in the knowledge graph, scoring them, and packing them within the
// token budget.
func (e *ContextEngine) ForTask(ctx stdctx.Context, opts TaskOptions) (*ContextBlock, error) {
	budget := opts.TokenBudget
	if budget == 0 {
		budget = 50000
	}

	// Extract structured keywords from the task description.
	// Compound-first: query "before_request" as a unit before splitting into
	// "before" and "request" (which match too broadly on small graphs).
	ks := extractKeywordSet(opts.TaskDescription)
	if ks.IsEmpty() {
		return &ContextBlock{
			Format:      opts.Format,
			TokenBudget: budget,
		}, nil
	}
	// Use Primary (compounds + exact) for tiered search. Fall back to All
	// only if Primary yields too few results (<5 seeds after tiered matching).
	keywords := ks.Primary()
	fallbackKeywords := ks.All()

	// Cache lookup: if a SubgraphCache is attached, check for a cached result
	// keyed by the normalized task description. On a hit, deserialize and return
	// immediately, skipping all retrieval. The Format field is not cached because
	// it is a caller-specified rendering hint, not part of the result content.
	normalized := strings.ToLower(strings.TrimSpace(opts.TaskDescription))
	var cacheKey types.Hash
	if e.cache != nil {
		cacheKey = types.NewHash([]byte("task\x00" + normalized))
		if data, ok := e.cache.Get(cacheKey); ok {
			var cached cachedContextBlock
			if err := json.Unmarshal(data, &cached); err == nil {
				block := cached.toContextBlock()
				block.Format = opts.Format
				return block, nil
			}
		}
	}

	// Persistent cache: check notes table for a stored context pack.
	// This survives process restarts, enabling cross-session replay.
	// The note is keyed by task hash; the value includes a snapshot hash
	// for staleness detection. If the latest snapshot changed, the cached
	// pack is stale and we recompute.
	packNoteKey := types.NewHash([]byte("context_pack\x00" + normalized))
	if !e.noPersistentCache {
		if note, err := e.store.GetNote(ctx, packNoteKey, "context_pack"); err == nil && note != nil {
			var persisted persistedContextPack
			// Try zlib-compressed base64 first (new format), fall back to raw JSON (legacy).
			noteData := []byte(note.Value)
			if raw, b64Err := base64.StdEncoding.DecodeString(note.Value); b64Err == nil {
				if decompressed, zErr := decompressZlib(raw); zErr == nil {
					noteData = decompressed
				}
			}
			if err := json.Unmarshal(noteData, &persisted); err == nil {
				// Staleness check: compare stored snapshot hash against current.
				stale := false
				if repos, err := e.store.AllRepos(ctx); err == nil && len(repos) > 0 {
					if snap, err := e.store.LatestSnapshot(ctx, repos[0].RepoHash); err == nil && snap != nil {
						if persisted.SnapshotHash != snap.SnapshotHash {
							stale = true
						}
					}
				}
				if !stale {
					block := persisted.Block.toContextBlock()
					block.Format = opts.Format
					if e.cache != nil && !cacheKey.IsZero() {
						if data, err := json.Marshal(persisted.Block); err == nil {
							e.cache.Put(cacheKey, data)
						}
					}
					return block, nil
				}
			}
		}
	}

	// Seed retrieval uses independent channels fused with Reciprocal Rank Fusion:
	//   Channel 1: Tiered keyword matching (compound-first: exact > prefix > substring > path)
	//   Channel 2: BM25 full-text search (compound-targeted symbol_name column)
	// RRF merges their ranked lists: score = 1/(k+rank_tier) + 1/(k+rank_bm25)
	// Symbols appearing in both lists are promoted; single-channel hits get less weight.

	// Channel 1: Compound-first tiered keyword matching.
	tieredResults, _, _ := e.tieredSearchSet(ctx, ks)

	// Extract path terms once (used by both BM25 and Channel 5 path seeding).
	pathTerms := extractPathTerms(opts.TaskDescription)

	// Channel 2: BM25 full-text search (compound-targeted, always runs when available).
	var bm25Results []types.Node
	if e.bm25 != nil {
		ftsQuery := buildFTSQuery(keywords)
		if ftsQuery == "" && len(fallbackKeywords) > 0 {
			ftsQuery = buildFTSQuery(fallbackKeywords)
		}
		// FTS fallback decomposition: if the primary compound query yields 0
		// results, decompose compounds into targeted symbol_name OR terms.
		// "ModelAdmin.get_inlines" -> symbol_name:"ModelAdmin" OR symbol_name:"get_inlines"
		// "html.escape" -> symbol_name:"escape" (skip short/common segments)
		// The decomposed terms still target symbol_name for precision; generic
		// words like "django" or "utils" are filtered by length threshold.
		if ftsQuery != "" {
			if probe, err := e.bm25.SearchBM25Nodes(ctx, ftsQuery, 1); err == nil && len(probe) == 0 {
				decomposed := decomposeCompoundsTargeted(keywords)
				if decomposed == "" {
					decomposed = decomposeCompoundsTargeted(fallbackKeywords)
				}
				if decomposed != "" {
					ftsQuery = decomposed
				}
			}
		} else {
			// Primary was empty (no compounds at all). Decompose fallback.
			decomposed := decomposeCompoundsTargeted(fallbackKeywords)
			if decomposed != "" {
				ftsQuery = decomposed
			}
		}
		// Phrase-boost: generate adjacent-word phrases from Components for
		// higher-precision BM25 matching on dense graphs. "code actions" as a
		// phrase matches only symbols where those words appear adjacent (215 matches)
		// vs "code OR actions" (3284+5000 matches individually).
		if len(ks.Components) >= 2 {
			for i := 0; i < len(ks.Components)-1 && i < 5; i++ {
				a, b := ks.Components[i], ks.Components[i+1]
				if len(a) >= 3 && len(b) >= 3 {
					phrase := fmt.Sprintf("\"%s %s\"", a, b)
					if ftsQuery == "" {
						ftsQuery = phrase
					} else {
						ftsQuery += " OR " + phrase
					}
				}
			}
		}
		// Append file_path-targeted terms from path extraction.
		// Uses prefix matching (term*) to handle singular/plural directory names
		// (e.g., "migration*" matches both "migration.py" and "migrations/").
		if ftsQuery != "" && len(pathTerms) > 0 {
			for _, pt := range pathTerms {
				if len(pt) >= 4 {
					ftsQuery += fmt.Sprintf(" OR file_path:%s*", pt)
				}
			}
		}
		// Concept expansion: add related code vocabulary as supplemental OR terms.
		// "consumer" also searches "subscriber", "listener", "handler", etc.
		// Uses all keywords (including components) since domain terms like
		// "consumer", "scheduler" are typically single words in Components.
		// Capped at 10 expansions to avoid flooding BM25 with noise.
		if expanded := expandKeywords(fallbackKeywords, 10); len(expanded) > 0 {
			for _, exp := range expanded {
				ftsQuery += fmt.Sprintf(" OR %s", exp)
			}
		}
		if nodes, err := e.bm25.SearchBM25Nodes(ctx, ftsQuery, 30); err == nil {
			bm25Results = nodes
		}
	}

	// Repo-scoped filtering: when RepoURL is set, remove nodes from other repos.
	// This prevents cross-repo noise in multi-module DBs (e.g., k8s staging modules
	// diluting results for the root module).
	if opts.RepoURL != "" {
		repoPrefix := opts.RepoURL + "://"
		tieredResults = filterByRepoPrefix(tieredResults, repoPrefix)
		bm25Results = filterByRepoPrefix(bm25Results, repoPrefix)
	}

	// Channel 3: Vector (embedding) search (runs when embedder is available).
	// Embeds the task description and finds semantically similar symbols.
	var vectorResults []types.Node
	if e.vector != nil {
		hashes, err := e.vector.EmbedAndSearch(ctx, opts.TaskDescription, 30)
		if err == nil && len(hashes) > 0 {
			for _, h := range hashes {
				if node, err := e.store.GetNode(ctx, h); err == nil && node != nil {
					vectorResults = append(vectorResults, *node)
				}
			}
		}
	}

	// Channel 4: Equivalence class matching.
	// Two sources: seed dictionary (hand-curated concepts) + graph-derived aliases
	// (auto-generated from caller/callee names of tiered+BM25 candidates).
	var equivResults []types.Node
	equivSeen := make(map[types.Hash]bool)

	// Source 1: Seed + universal equivalence classes.
	// Auto-generated concepts from symbol names tested neutral (experiment 21):
	// CamelCase splitting already makes symbol names searchable via BM25.
	// Auto-concepts only add value if they generate conceptual aliases that
	// differ from the name, which requires domain understanding.
	allClasses := append(seedEquivalenceClasses(), universalEquivalenceClasses()...)
	allClasses = append(allClasses, languageEquivalenceClasses()...)
	// Detect repo language for scoping framework equiv classes.
	repoLang := detectRepoLanguage(ctx, e.store)
	eqMatches := matchEquivalenceClassesLang(opts.TaskDescription, allClasses, repoLang)

	// Source 2: Graph-derived aliases from the candidates we already found.
	// Uses tiered+BM25 results as input, generates targeted phrase mappings
	// from their callers/callees, and matches against the task description.
	// Only use top tiered results (highest quality seeds) for graph alias generation.
	// Using all candidates adds too much noise from loosely-related nodes.
	var candidateHashes []types.Hash
	topN := 10
	if len(tieredResults) < topN {
		topN = len(tieredResults)
	}
	for i := 0; i < topN; i++ {
		candidateHashes = append(candidateHashes, tieredResults[i].NodeHash)
	}
	graphClasses := graphDerivedAliases(ctx, e.store, candidateHashes)
	graphMatches := matchEquivalenceClasses(opts.TaskDescription, graphClasses)
	eqMatches = append(eqMatches, graphMatches...)

	// Resolve all equivalence targets to actual nodes.
	// Filter: skip generic targets (<=3 chars or common method names) that produce
	// too many false positives on small graphs. These targets match hundreds of
	// unrelated symbols (e.g., "Get" matches every getter in the codebase).
	genericTargets := map[string]bool{
		"get": true, "set": true, "do": true, "new": true, "run": true,
		"put": true, "post": true, "call": true, "add": true, "pop": true,
	}
	// Track high-confidence framework matches for forced injection post-RWR.
	var frameworkInjections []types.Node
	for _, m := range eqMatches {
		for _, target := range m.targets {
			targetLower := strings.ToLower(target)
			if len(target) <= 3 || genericTargets[targetLower] {
				continue
			}
			nodes, err := e.store.NodesByName(ctx, "%"+target)
			if err != nil {
				continue
			}
			for _, n := range nodes {
				lastDot := strings.LastIndex(n.QualifiedName, ".")
				symName := n.QualifiedName
				if lastDot >= 0 {
					symName = n.QualifiedName[lastDot+1:]
				}
				if !strings.EqualFold(symName, target) {
					continue
				}
				// Framework injection: always inject regardless of equivSeen.
				// Earlier lower-weight classes may have already added this node
				// to equivResults, but injection needs to happen separately.
				if m.weight >= 0.9 && m.class.Source == "framework" {
					frameworkInjections = append(frameworkInjections, n)
				}
				if equivSeen[n.NodeHash] {
					continue
				}
				equivSeen[n.NodeHash] = true
				equivResults = append(equivResults, n)
			}
		}
	}
	// Cap equiv results: too many low-quality equiv seeds dilute RRF fusion.
	// On small graphs (< 3000 non-external nodes), equiv results that outnumber
	// tiered+BM25 by >3x produce flat RWR scores. Cap at 2x the primary channels.
	maxEquiv := (len(tieredResults) + len(bm25Results)) * 2
	if maxEquiv < 10 {
		maxEquiv = 10
	}
	if len(equivResults) > maxEquiv {
		equivResults = equivResults[:maxEquiv]
	}

	// Channel 5: Path-context seeding.
	// Extracts package/directory-like terms from the task description and finds
	// TYPE/CLASS nodes whose qualified name PATH contains those terms. Types are
	// structural anchors: with contains edges, RWR walks from types to their methods.
	// This bridges the gap between conceptual task descriptions and implementations
	// (e.g., "migration" -> django.db.migrations.operations.base.Operation -> .state_forwards).
	var pathResults []types.Node
	pathSeen := make(map[types.Hash]bool)

	// Type-method matching: find types in packages matching one path term,
	// then check if their methods/children match another keyword. This bridges
	// "migration operation" -> Type in migrations/ package with method containing "forward".
	// Results go into pathResults (at front) so they flow through both RRF and seed injection.
	if len(pathTerms) >= 1 && len(keywords) > 0 {
		var typeMethodHits []types.Node
		for _, pathTerm := range pathTerms {
			typeNodes, _ := e.store.NodesByName(ctx, "%/"+pathTerm+"%")
			if len(typeNodes) == 0 {
				typeNodes, _ = e.store.NodesByName(ctx, "%."+pathTerm+".%")
			}
			for _, tn := range typeNodes {
				if pathSeen[tn.NodeHash] {
					continue
				}
				if tn.Kind != "type" && tn.Kind != "class" && tn.Kind != "struct" && tn.Kind != "interface" {
					continue
				}
				children, _ := e.store.EdgesFrom(ctx, tn.NodeHash, "contains")
				if len(children) == 0 {
					continue
				}
				for _, edge := range children {
					child, _ := e.store.GetNode(ctx, edge.TargetHash)
					if child == nil {
						continue
					}
					childLower := strings.ToLower(child.QualifiedName)
					for _, kw := range keywords {
						if strings.Contains(childLower, strings.ToLower(kw)) {
							pathSeen[tn.NodeHash] = true
							typeMethodHits = append(typeMethodHits, tn)
							break
						}
					}
					if pathSeen[tn.NodeHash] {
						break
					}
				}
				if len(typeMethodHits) >= 10 {
					break
				}
			}
			if len(typeMethodHits) >= 10 {
				break
			}
		}
		// Prepend to pathResults so they get priority in RRF and seed injection.
		pathResults = append(typeMethodHits, pathResults...)
	}

	// Per-term OR pass: for each term individually.
	for _, term := range pathTerms {
		// Search for nodes whose qualified name contains this path segment.
		nodes, err := e.store.NodesByName(ctx, "%/"+term+"%")
		if err != nil {
			continue
		}
		// Also try dot-separated paths (Python: django.db.migrations.*)
		if len(nodes) == 0 {
			nodes, _ = e.store.NodesByName(ctx, "%."+term+".%")
		}
		// Collect matching type/class nodes, prioritized by connectivity.
		// Types with outgoing contains edges (i.e., with methods) are framework code;
		// types without are often individual instances (e.g., 0001_initial.py.Migration).
		var richTypes, leanTypes, otherMatches []types.Node
		for _, n := range nodes {
			if pathSeen[n.NodeHash] {
				continue
			}
			if !isPathMatch(n.QualifiedName, term) {
				continue
			}
			pathSeen[n.NodeHash] = true
			switch n.Kind {
			case "type", "class", "struct", "interface", "trait":
				// Check if this type has contains edges (methods).
				edges, _ := e.store.EdgesFrom(ctx, n.NodeHash, "contains")
				if len(edges) > 0 {
					richTypes = append(richTypes, n)
				} else {
					leanTypes = append(leanTypes, n)
				}
			default:
				otherMatches = append(otherMatches, n)
			}
			// Stop scanning after finding enough candidates.
			if len(richTypes) >= 5 {
				break
			}
			if len(richTypes)+len(leanTypes)+len(otherMatches) >= 100 {
				break
			}
		}
		// Priority: rich types (with methods) > lean types > other nodes.
		taken := 0
		for _, n := range richTypes {
			if taken >= 5 {
				break
			}
			pathResults = append(pathResults, n)
			taken++
		}
		for _, n := range leanTypes {
			if taken >= 10 {
				break
			}
			pathResults = append(pathResults, n)
			taken++
		}
		for _, n := range otherMatches {
			if taken >= 15 {
				break
			}
			pathResults = append(pathResults, n)
			taken++
		}
	}
	// Cap total path results.
	if len(pathResults) > 30 {
		pathResults = pathResults[:30]
	}
	if opts.RepoURL != "" {
		repoPrefix := opts.RepoURL + "://"
		pathResults = filterByRepoPrefix(pathResults, repoPrefix)
	}

	// Fuse all channels with weighted Reciprocal Rank Fusion.
	// Weights: tiered 2x (exact/prefix, most precise), BM25 2x (relevance-ranked,
	// complementary to tiered via signature and multi-term matching),
	// equivalence 2x (concept-level), path 1.5x (directory-based, broader),
	// vector 0x (disabled, infrastructure preserved).
	// Path channel gets 1.5x: valuable for bridging concepts to packages but
	// less precise than name-matching channels (may return many symbols from a package).
	// k=60 is the standard RRF constant.
	candidates := rrfFuseMulti([]rankedChannel{
		{nodes: tieredResults, weight: 2.0},
		{nodes: bm25Results, weight: 2.0},
		{nodes: equivResults, weight: 2.0},
		{nodes: pathResults, weight: 1.5},
		{nodes: vectorResults, weight: 0.0},
	}, int(sweepRRFk()), 40)

	seen := make(map[types.Hash]bool, len(candidates))
	for _, c := range candidates {
		seen[c.NodeHash] = true
	}

	// Embedding gap-fill: when BM25/tiered/equiv return fewer than 5 seeds,
	// use vector search to find semantically similar symbols that keyword matching
	// missed. This targets the vocabulary gap problem (42% of Django tasks score
	// zero because ground truth shares no keywords with the task description).
	// Gap-fill seeds enter at weight 1.0 (same as primary seeds) but only when
	// primary channels are weak. Cannot regress repos where BM25 already works.
	//
	// Two search strategies:
	// 1. HNSW (EmbedAndSearch): fast O(log n), requires in-memory index built at startup
	// 2. Store brute-force (LoadAndSearchFromStore): O(n) cosine, no index build needed,
	//    works with pre-embedded vectors from SQLite. Falls back to this when HNSW is empty.
	gapThreshold := 5
	if v := os.Getenv("BENCH_GAP_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			gapThreshold = n
		}
	}
	// Focused seed selection: cluster candidates by package path and prefer
	// seeds from the same structural neighborhood. On by default; disable
	// with BENCH_FOCUSED_SEEDS=0 for A/B testing.
	focusedSeeds := os.Getenv("BENCH_FOCUSED_SEEDS") != "0"
	var focusPkg string
	if focusedSeeds {
		focusPkg = dominantPkg(candidates)
	}
	if len(candidates) < gapThreshold && e.vector != nil {
		gapHashes, gapErr := e.vector.EmbedAndSearch(ctx, opts.TaskDescription, 20)
		// If HNSW returned nothing (empty index), try brute-force from store.
		if (gapErr != nil || len(gapHashes) == 0) {
			if storeSearcher, ok := e.vector.(StoreSearcher); ok {
				gapHashes, gapErr = storeSearcher.LoadAndSearchFromStore(ctx, opts.TaskDescription, 20)
			}
		}
		if gapErr == nil {
			for _, h := range gapHashes {
				if seen[h] {
					continue
				}
				n, err := e.store.GetNode(ctx, h)
				if err != nil || n == nil {
					continue
				}
				// Skip phantom/external nodes
				if n.Kind == "external" {
					continue
				}
				// Cluster-aware gap-fill: when focused seeds is active,
				// only accept gap-fill nodes from the dominant package.
				if focusPkg != "" && qualifiedNamePkg(n.QualifiedName) != focusPkg {
					continue
				}
				seen[h] = true
				candidates = append(candidates, *n)
				if len(candidates) >= 15 {
					break
				}
			}
		}
	}

	// Tier 5: interface-aware seeding.
	// If any candidate is an interface type, add all implementors as seeds.
	// "Build a new extractor" -> finds types.Extractor -> adds all existing extractors.
	var interfaceSeeds []types.Node
	for _, c := range candidates {
		if c.Kind == "interface" || c.Kind == "type" {
			// Find all "implements" edges pointing TO this type.
			implEdges, err := e.store.EdgesTo(ctx, c.NodeHash, "implements")
			if err != nil {
				continue
			}
			for _, edge := range implEdges {
				if seen[edge.SourceHash] {
					continue
				}
				implNode, err := e.store.GetNode(ctx, edge.SourceHash)
				if err != nil || implNode == nil {
					continue
				}
				seen[implNode.NodeHash] = true
				interfaceSeeds = append(interfaceSeeds, *implNode)
			}
		}
	}
	candidates = append(candidates, interfaceSeeds...)

	// Filter out noise: minified bundles, dist/, vendor/, node_modules/
	candidates = filterNoisySymbols(candidates)

	// PreferTypeSeeds (H8): on dense graphs, reorder candidates so type/interface/class
	// nodes come before methods/functions. Types are better RWR seeds because they have
	// contains edges to their methods, making the walk more productive. On sparse graphs
	// this is irrelevant (all candidates are already good seeds).
	// Self-adapting: auto-enable when graph has >50K nodes (dense graph threshold).
	preferTypes := PreferTypeSeeds
	if !preferTypes && e.effectiveNodeCount() > 40000 {
		preferTypes = true
	}
	if preferTypes && len(candidates) > 0 {
		sort.SliceStable(candidates, func(i, j int) bool {
			iIsType := candidates[i].Kind == "type" || candidates[i].Kind == "class" ||
				candidates[i].Kind == "interface" || candidates[i].Kind == "struct"
			jIsType := candidates[j].Kind == "type" || candidates[j].Kind == "class" ||
				candidates[j].Kind == "interface" || candidates[j].Kind == "struct"
			if iIsType && !jIsType {
				return true
			}
			return false
		})
	}

	// Run Random Walk with Restart from seed nodes to compute relevance
	// scores across the entire reachable subgraph. This replaces manual
	// neighbor expansion with a principled graph-based relevance signal.
	//
	// Seed weights: candidates earlier in the list (higher RRF rank) get
	// higher restart probability. This prevents generic seeds (low specificity)
	// from diluting the walk on small, densely-connected graphs.
	// Cap seeds at top-15 by RRF rank. On small graphs (< 3000 nodes), too many
	// seeds cause RWR to converge to near-uniform (everything is 2 hops from
	// everything). Limiting seeds keeps the walk focused on the best candidates.
	//
	// Focused seed selection (#36): cluster candidates by package path, pick seeds
	// from the most cohesive cluster. Quality over quantity: 3-5 seeds in the same
	// structural neighborhood vs 15-25 scattered across the graph.
	// Package-path boosting: DISABLED (experiment showed regression on terraform).
	// The issue: when Primary keywords are empty, BM25 doesn't fire, so there are
	// no good candidates to boost. The path boost reorders tiered results which are
	// already in the wrong neighborhood. Fix needed: improve keyword extraction
	// for long task descriptions so BM25 always fires.
	// TODO: re-enable after fixing extractKeywordSet for sentence-style tasks.
	// if e.effectiveNodeCount() > 10000 && len(pathTerms) > 0 && len(candidates) > 10 {
	// 	candidates = boostByPathMatch(candidates, pathTerms)
	// }

	if focusedSeeds && len(candidates) > 5 {
		candidates = focusedSeedSelect(candidates)
	}
	maxSeeds := sweepMaxSeeds()
	// Adaptive seed count: on large graphs, increase seeds to compensate for
	// higher disconnection rates. More seeds = wider net = more symbols reachable.
	// Auto-enabled based on node count. Django +14.2% P@10 with 25 seeds.
	if e.effectiveNodeCount() > 40000 {
		maxSeeds = max(maxSeeds, 25) // large graphs: 25 seeds
	} else if e.effectiveNodeCount() > 10000 {
		maxSeeds = max(maxSeeds, 20) // medium graphs: 20 seeds
	}
	if len(candidates) < maxSeeds {
		maxSeeds = len(candidates)
	}
	seedHashes := make([]types.Hash, 0, maxSeeds+10)
	seedWeights := make(map[types.Hash]float64, maxSeeds+10)
	seedSet := make(map[types.Hash]bool)
	for i := 0; i < maxSeeds; i++ {
		c := candidates[i]
		seedHashes = append(seedHashes, c.NodeHash)
		seedSet[c.NodeHash] = true
		// Weight decays by rank: rank 1 = 1.0, rank 10 = 0.55, rank 15 = 0.40
		seedWeights[c.NodeHash] = 1.0 / (1.0 + float64(i)*0.1)
	}

	// Inject path-seeded type nodes as supplemental RWR seeds.
	// These don't compete in RRF (where they'd lose to name-matched results)
	// but still contribute to RWR with lower restart weight (0.3).
	// With contains edges, RWR walks from these types to their methods.
	for _, n := range pathResults {
		if seedSet[n.NodeHash] {
			continue // already a seed from main channels
		}
		seedHashes = append(seedHashes, n.NodeHash)
		seedSet[n.NodeHash] = true
		seedWeights[n.NodeHash] = 0.3
		if len(seedHashes) >= maxSeeds+10 {
			break
		}
	}

	rwrScores, _, err := RandomWalkWithRestartWeighted(ctx, e.store, seedHashes, seedWeights, sweepAlpha(), sweepMaxIter())
	if err != nil {
		return nil, err
	}

	// Build scoring inputs from all nodes that received a non-trivial RWR score.
	// Score cutoff: balance between expanding the candidate pool for HITS/feedback
	// reranking and avoiding noise from distant, weakly-connected nodes.
	scoreCutoff := sweepScoreCutoff()
	var inputs []ScoringInput
	for nodeHash, rwrScore := range rwrScores {
		if rwrScore < scoreCutoff {
			continue // skip negligible nodes
		}

		node, err := e.store.GetNode(ctx, nodeHash)
		if err != nil {
			return nil, err
		}
		if node == nil {
			continue
		}

		// Skip phantom external nodes and stdlib nodes reached by RWR walk.
		// External: unresolved targets from LSP enrichment with no source code.
		// Stdlib: standard library functions (fmt.Errorf, etc.) that accumulate
		// disproportionate RWR probability due to high in-degree but are never
		// useful as context for a coding task.
		if node.Kind == "external" || strings.HasPrefix(node.QualifiedName, "external://") || strings.HasPrefix(node.QualifiedName, "stdlib://") {
			continue
		}

		// Scoring distance: binary 0/1 (BFS proximity in scoring confirmed
		// neutral/harmful in session 24; the enrichment dilution problem is
		// in packing, not scoring).
		distance := 1
		if seedSet[nodeHash] {
			distance = 0
		}

		// Raw RWR score for proximity-weighted packing. Already computed,
		// zero extra cost. Higher score = closer to seeds.
		rawRWR := rwrScores[nodeHash]

		// Get edge metadata for confidence and recency.
		edges, err := e.store.EdgesTo(ctx, nodeHash, "")
		if err != nil {
			return nil, err
		}
		confidence := 0.5
		var lastObserved int64
		for _, edge := range edges {
			if edge.Confidence > confidence {
				confidence = edge.Confidence
			}
			if edge.LastObserved > lastObserved {
				lastObserved = edge.LastObserved
			}
		}

		// Hub dampening (H1): penalize nodes with very high in-degree.
		// These are utility types that absorb RWR probability regardless of query.
		if HubDampeningThreshold > 0 && len(edges) > HubDampeningThreshold {
			dampFactor := math.Sqrt(float64(len(edges)) / float64(HubDampeningThreshold))
			rwrScore /= dampFactor
		}

		// Use RWR score as the caller count proxy. Scale to an integer
		// range that the ranking algorithm can normalize (0-100).
		// For seeds: boost by RRF rank position to break ties when RWR is flat.
		// Seed #1 gets +50, seed #15 gets +3. Non-seeds get pure RWR score.
		callerProxy := int(rwrScore * 100)
		if w, ok := seedWeights[nodeHash]; ok && w > 0 {
			// seedWeights decay from 1.0 (rank 1) to 0.4 (rank 15).
			// Scale to 0-50 bonus range.
			callerProxy += int(w * 50)
		}

		inputs = append(inputs, ScoringInput{
			Node:               *node,
			CallerCount:        callerProxy,
			Confidence:         confidence,
			LastObserved:       lastObserved,
			DistanceFromTarget: distance,
			RWRScore:           rawRWR,
			IsTestFile:         isTestFilePath(node.QualifiedName),
		})
	}

	// Apply feedback boosts if feedback provider is available.
	if e.feedback != nil && len(inputs) > 0 {
		hashes := make([]types.Hash, len(inputs))
		for i, inp := range inputs {
			hashes[i] = inp.Node.NodeHash
		}
		// TODO: Pass neighborhood roots for merkleized expiration once hierarchical tree is available.
		if boosts, err := e.feedback.FeedbackBoosts(ctx, hashes, nil); err == nil && len(boosts) > 0 {
			for i := range inputs {
				if boost, ok := boosts[inputs[i].Node.NodeHash]; ok {
					inputs[i].FeedbackBoost = boost
				}
			}
		}
	}

	// Apply session boosts: symbols seen earlier this session get priority.
	if e.session != nil && len(inputs) > 0 {
		hashes := make([]types.Hash, len(inputs))
		for i, inp := range inputs {
			hashes[i] = inp.Node.NodeHash
		}
		boosts := e.session.SessionBoosts(hashes)
		for i := range inputs {
			if boost, ok := boosts[inputs[i].Node.NodeHash]; ok {
				inputs[i].SessionBoost = boost
			}
		}
	}

	// Apply task memory boosts: symbols useful for similar past tasks get priority.
	// Memory recall returns scores on [0,1] scale (0.6 = returned before, 1.0 = explicit feedback).
	// FeedbackBoost uses [0,1] where >0.5 = positive, <0.5 = negative.
	// Set to 0.5 + (recall_score * 0.4) so memory always produces a positive boost
	// (range [0.5, 0.9]) without overwhelming explicit feedback (which can reach 1.0).
	if e.memory != nil && len(inputs) > 0 {
		memoryBoosts, err := e.memory.Recall(ctx, keywords)
		if err == nil && len(memoryBoosts) > 0 {
			for i := range inputs {
				if boost, ok := memoryBoosts[inputs[i].Node.NodeHash]; ok {
					memoryScore := 0.5 + (boost * 0.4) // [0.5, 0.9] range (always positive)
					if inputs[i].FeedbackBoost < memoryScore {
						inputs[i].FeedbackBoost = memoryScore
					}
				}
			}
		}
	}

	// If the task is about testing, don't penalize test file symbols.
	// Detect by checking if task description contains test-related keywords.
	taskIsAboutTesting := isTestRelatedTask(opts.TaskDescription)
	if taskIsAboutTesting {
		for i := range inputs {
			inputs[i].IsTestFile = false
		}
	}

	// Sort inputs by RWR score (CallerCount proxy) so HITS runs on the most
	// relevant subgraph, not a random map iteration order.
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].CallerCount > inputs[j].CallerCount
	})

	// Run HITS on the top-200 nodes for authority/hub scoring.
	// This separates structurally important symbols from merely proximate ones.
	var hitsResult map[types.Hash]HITSScores
	if len(inputs) > 5 {
		topN := 200
		if len(inputs) < topN {
			topN = len(inputs)
		}
		hitsNodes := make([]types.Hash, topN)
		for i := 0; i < topN; i++ {
			hitsNodes[i] = inputs[i].Node.NodeHash
		}
		hitsResult, _ = ComputeHITS(ctx, e.store, hitsNodes, 10)
	}

	// Rank and pack into budget.
	ranked := RankSymbols(inputs, hitsResult)

	// Re-rank disabled: per-repo A/B test (session 19) showed the embedding re-ranker
	// is net negative on P@10 (9/13 repos hurt, net -0.050 vs +0.015). The "+17%"
	// measured in session 15 was from gap-fill seeds, not re-rank. Gap-fill remains
	// active via the vector searcher. Re-rank code preserved for future investigation.
	// if reranker, ok := e.vector.(VectorReRanker); ok && len(ranked) > 0 {
	// 	ranked = e.reRankWithEmbeddings(ctx, reranker, ranked, opts.TaskDescription)
	// }

	// Framework injection: high-confidence framework equiv matches get inserted
	// at the front of the ranked list. These are specific class names from
	// domain-specific concept mappings (e.g., "email validation" -> EmailValidator).
	// They bypass RWR scoring because they're semantically certain matches that
	// the graph structure can't discover (no inheritance edges, no keyword overlap).
	if len(frameworkInjections) > 0 {
		injectedSeen := make(map[types.Hash]bool)
		var injected []RankedSymbol
		for _, n := range frameworkInjections {
			if injectedSeen[n.NodeHash] {
				continue
			}
			injectedSeen[n.NodeHash] = true
			// Give injected symbols a score just above the current max.
			topScore := 0.0
			if len(ranked) > 0 {
				topScore = ranked[0].Score
			}
			injected = append(injected, RankedSymbol{
				Node:  n,
				Score: topScore + 0.1,
			})
		}
		// Remove duplicates from ranked (if already present from RWR).
		var deduped []RankedSymbol
		for _, r := range ranked {
			if !injectedSeen[r.Node.NodeHash] {
				deduped = append(deduped, r)
			}
		}
		ranked = append(injected, deduped...)
	}

	// Adaptive strategy: if RWR produced flat results (low confidence), fall back
	// to direct FTS + contains-edge expansion. This helps massive repos (vscode 552K
	// nodes) where RWR always diffuses to near-uniform and the top-10 is noise.
	if len(ranked) >= 10 && e.effectiveNodeCount() > 200000 && resultConfidence(ranked) < 0.3 {
		// RWR didn't converge. Try direct symbol name search + 1-hop expansion.
		directResults := directFTSExpansion(ctx, e.store, e.bm25, ks, 10)
		if len(directResults) > 0 {
			// Merge: keep framework injections at top, add direct results, then RWR.
			var merged []RankedSymbol
			mergedSeen := make(map[types.Hash]bool)
			// Framework injections stay at top.
			for _, r := range ranked {
				if r.Score > ranked[0].Score-0.05 { // top cluster (injections)
					merged = append(merged, r)
					mergedSeen[r.Node.NodeHash] = true
				}
			}
			// Add direct FTS results with high score.
			baseScore := 0.7
			if len(merged) > 0 {
				baseScore = merged[len(merged)-1].Score - 0.01
			}
			for i, n := range directResults {
				if mergedSeen[n.NodeHash] {
					continue
				}
				mergedSeen[n.NodeHash] = true
				merged = append(merged, RankedSymbol{
					Node:  n,
					Score: baseScore - float64(i)*0.01,
				})
			}
			// Fill rest from RWR.
			for _, r := range ranked {
				if mergedSeen[r.Node.NodeHash] {
					continue
				}
				merged = append(merged, r)
			}
			ranked = merged
		}
	}

	block := packIntoBudget(ranked, budget, opts.Format)

	// Record returned symbols in session tracker for future query boosts.
	if e.session != nil && len(block.Symbols) > 0 {
		hashes := make([]types.Hash, len(block.Symbols))
		for i, s := range block.Symbols {
			hashes[i] = s.Node.NodeHash
		}
		e.session.RecordBatch(hashes)
	}

	// Record in task memory: persist (keywords, symbols) for future recall.
	// Only records top-5 symbols (highest score, most likely to be useful).
	if e.memory != nil && len(block.Symbols) > 0 {
		kws := NormalizeKeywords(opts.TaskDescription)
		topN := 5
		if len(block.Symbols) < topN {
			topN = len(block.Symbols)
		}
		hashes := make([]types.Hash, topN)
		for i := 0; i < topN; i++ {
			hashes[i] = block.Symbols[i].Node.NodeHash
		}
		e.memory.RecordBatch(ctx, kws, hashes, 1.0) //nolint:errcheck
	}

	// Compute content-addressed PackRoot for this context result.
	// Two identical queries against the same graph state produce the same PackRoot,
	// enabling deduplication, citation, and cross-session replay.
	block.PackRoot = computePackRoot(opts.TaskDescription, block.Symbols)

	// Cache store: persist the result for future identical queries. The Format
	// field is excluded from the cached payload because it is caller-specified.
	cachedBlock := newCachedContextBlock(block)
	if data, err := json.Marshal(cachedBlock); err == nil {
		if e.cache != nil && !cacheKey.IsZero() {
			e.cache.Put(cacheKey, data)
		}
	}
	// Persistent cache: store in notes table with snapshot hash for staleness.
	// Uses zlib compression (~6x smaller than raw JSON for typical context packs).
	if repos, err := e.store.AllRepos(ctx); err == nil && len(repos) > 0 {
		if snap, err := e.store.LatestSnapshot(ctx, repos[0].RepoHash); err == nil && snap != nil {
			persisted := persistedContextPack{
				Block:        cachedBlock,
				SnapshotHash: snap.SnapshotHash,
			}
			if data, err := json.Marshal(persisted); err == nil {
				compressed := compressZlib(data)
				_ = e.store.PutNote(ctx, types.Note{
					ObjectHash: packNoteKey,
					Key:        "context_pack",
					Value:      base64.StdEncoding.EncodeToString(compressed),
					UpdatedAt:  time.Now().Unix(),
				})
			}
		}
	}

	return block, nil
}

// persistedContextPack wraps a cached context block with a snapshot hash for
// staleness detection. Stored in the notes table for cross-session replay.
// If the snapshot hash differs from the current latest, the pack is stale.
type persistedContextPack struct {
	Block        cachedContextBlock `json:"block"`
	SnapshotHash types.Hash         `json:"snapshot_hash"`
}

// cachedContextBlock is the JSON-serializable form of ContextBlock stored in the
// SubgraphCache. Format is excluded because it is a caller-specified rendering hint.
type cachedContextBlock struct {
	Symbols     []RankedSymbol `json:"symbols"`
	Edges       []ContextEdge  `json:"edges"`
	TokensUsed  int            `json:"tokens_used"`
	TokenBudget int            `json:"token_budget"`
	PackRoot    types.Hash     `json:"pack_root"`
}

func newCachedContextBlock(b *ContextBlock) cachedContextBlock {
	return cachedContextBlock{
		Symbols:     b.Symbols,
		Edges:       b.Edges,
		TokensUsed:  b.TokensUsed,
		TokenBudget: b.TokenBudget,
		PackRoot:    b.PackRoot,
	}
}

func (c *cachedContextBlock) toContextBlock() *ContextBlock {
	return &ContextBlock{
		Symbols:     c.Symbols,
		Edges:       c.Edges,
		TokensUsed:  c.TokensUsed,
		TokenBudget: c.TokenBudget,
		PackRoot:    c.PackRoot,
	}
}

// computePackRoot produces a content-addressed hash for a context pack.
// The root is deterministic: same task + same selected symbols = same hash.
func computePackRoot(task string, symbols []RankedSymbol) types.Hash {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(task))))
	// Sort node hashes for determinism (symbols may be in score order).
	nodeHashes := make([]types.Hash, len(symbols))
	for i, s := range symbols {
		nodeHashes[i] = s.Node.NodeHash
	}
	sort.Slice(nodeHashes, func(i, j int) bool {
		return string(nodeHashes[i][:]) < string(nodeHashes[j][:])
	})
	for _, nh := range nodeHashes {
		h.Write(nh[:])
	}
	var result types.Hash
	copy(result[:], h.Sum(nil))
	return result
}

// ForFiles produces blast-radius context weighted by runtime observations
// for a set of changed files.
func (e *ContextEngine) ForFiles(ctx stdctx.Context, opts FileOptions) (*ContextBlock, error) {
	budget := opts.TokenBudget
	if budget == 0 {
		budget = 50000
	}

	if len(opts.Files) == 0 {
		return &ContextBlock{
			Format:      opts.Format,
			TokenBudget: budget,
		}, nil
	}

	repoHash := types.NewHash([]byte(opts.RepoURL))

	var inputs []ScoringInput
	inputSeen := make(map[types.Hash]bool)

	for _, path := range opts.Files {
		nodes, err := e.store.NodesByFilePath(ctx, repoHash, path)
		if err != nil {
			return nil, err
		}

		for _, node := range nodes {
			if inputSeen[node.NodeHash] {
				continue
			}

			callers, err := e.store.EdgesTo(ctx, node.NodeHash, "calls")
			if err != nil {
				return nil, err
			}

			confidence := 0.5
			var lastObserved int64
			for _, edge := range callers {
				if edge.Confidence > confidence {
					confidence = edge.Confidence
				}
				if edge.LastObserved > lastObserved {
					lastObserved = edge.LastObserved
				}
			}

			inputSeen[node.NodeHash] = true
			inputs = append(inputs, ScoringInput{
				Node:               node,
				CallerCount:        len(callers),
				Confidence:         confidence,
				LastObserved:       lastObserved,
				DistanceFromTarget: 0,
			})

			// Add callers as distance-1 candidates.
			for _, edge := range callers {
				if inputSeen[edge.SourceHash] {
					continue
				}
				callerNode, err := e.store.GetNode(ctx, edge.SourceHash)
				if err != nil {
					return nil, err
				}
				if callerNode == nil {
					continue
				}
				inputSeen[edge.SourceHash] = true
				inputs = append(inputs, ScoringInput{
					Node:               *callerNode,
					CallerCount:        0,
					Confidence:         edge.Confidence,
					LastObserved:       edge.LastObserved,
					DistanceFromTarget: 1,
				})
			}
		}
	}

	// Run HITS on the file-based candidates for authority/hub differentiation.
	var hitsResult map[types.Hash]HITSScores
	if len(inputs) > 5 {
		topN := 200
		if len(inputs) < topN {
			topN = len(inputs)
		}
		sort.Slice(inputs, func(i, j int) bool {
			return inputs[i].CallerCount > inputs[j].CallerCount
		})
		hitsNodes := make([]types.Hash, topN)
		for i := 0; i < topN; i++ {
			hitsNodes[i] = inputs[i].Node.NodeHash
		}
		hitsResult, _ = ComputeHITS(ctx, e.store, hitsNodes, 10)
	}

	ranked := RankSymbols(inputs, hitsResult)
	block := packIntoBudget(ranked, budget, opts.Format)
	e.recordImplicitFeedback(ctx, block)
	return block, nil
}

// recordImplicitFeedback flushes unused symbols from the previous ForTask call
// (recording negative feedback for noise demotion) and registers newly returned
// symbols for attribution. This moves implicit feedback from MCP-only to the
// engine level, making it available to all consumers (MCP, benchmark, CLI).
func (e *ContextEngine) recordImplicitFeedback(ctx stdctx.Context, block *ContextBlock) {
	if e.implicit == nil || block == nil {
		return
	}

	// Flush previous: symbols returned by last ForTask but never used = negative.
	if e.recorder != nil {
		unused := e.implicit.FlushUnused()
		for _, h := range unused {
			_ = e.recorder.RecordFeedback(ctx, h, "implicit", false, types.EmptyHash)
		}
	}

	// Register new returned symbols for attribution tracking.
	if len(block.Symbols) > 0 {
		e.implicit.RegisterReturned(block.Symbols)
	}
}

// ForPR produces relationship-aware context for a pull request. It identifies
// all symbols in the changed files, runs RWR from them to find the broader
// impact neighborhood, and includes blast radius (callers of changed symbols)
// as distance-1 context. This is the highest-value context call: one invocation
// at PR-open time surfaces the full structural impact.
func (e *ContextEngine) ForPR(ctx stdctx.Context, opts PROptions) (*ContextBlock, error) {
	budget := opts.TokenBudget
	if budget == 0 {
		budget = 8000
	}

	if len(opts.Files) == 0 {
		return &ContextBlock{
			Format:      opts.Format,
			TokenBudget: budget,
		}, nil
	}

	repoHash := types.NewHash([]byte(opts.RepoURL))

	// Step 1: Find all symbols in the changed files (these are the PR's direct changes).
	var seeds []types.Hash
	seedSet := make(map[types.Hash]bool)

	for _, path := range opts.Files {
		nodes, err := e.store.NodesByFilePath(ctx, repoHash, path)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			if !seedSet[node.NodeHash] {
				seeds = append(seeds, node.NodeHash)
				seedSet[node.NodeHash] = true
			}
		}
	}

	if len(seeds) == 0 {
		return &ContextBlock{
			Format:      opts.Format,
			TokenBudget: budget,
		}, nil
	}

	// Step 2: Run RWR from the changed symbols to find the impact neighborhood.
	rwrScores, err := RandomWalkWithRestart(ctx, e.store, seeds, 0.2, 20)
	if err != nil {
		return nil, err
	}

	// Step 3: Build scoring inputs from all nodes with non-trivial RWR scores.
	var inputs []ScoringInput
	for nodeHash, rwrScore := range rwrScores {
		if rwrScore < 0.05 {
			continue
		}

		node, err := e.store.GetNode(ctx, nodeHash)
		if err != nil || node == nil {
			continue
		}

		// Distance: 0 if directly changed (in a PR file), 1+ otherwise.
		distance := 1
		if seedSet[nodeHash] {
			distance = 0
		}

		edges, err := e.store.EdgesTo(ctx, nodeHash, "")
		if err != nil {
			return nil, err
		}
		confidence := 0.5
		var lastObserved int64
		for _, edge := range edges {
			if edge.Confidence > confidence {
				confidence = edge.Confidence
			}
			if edge.LastObserved > lastObserved {
				lastObserved = edge.LastObserved
			}
		}

		callerProxy := int(rwrScore * 100)

		inputs = append(inputs, ScoringInput{
			Node:               *node,
			CallerCount:        callerProxy,
			Confidence:         confidence,
			LastObserved:       lastObserved,
			DistanceFromTarget: distance,
			IsTestFile:         isTestFilePath(node.QualifiedName),
		})
	}

	// Sort by RWR score before selecting HITS subgraph.
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].CallerCount > inputs[j].CallerCount
	})

	// Run HITS for better authority/hub differentiation.
	var hitsResult map[types.Hash]HITSScores
	if len(inputs) > 5 {
		topN := 200
		if len(inputs) < topN {
			topN = len(inputs)
		}
		hitsNodes := make([]types.Hash, topN)
		for i := 0; i < topN; i++ {
			hitsNodes[i] = inputs[i].Node.NodeHash
		}
		hitsResult, _ = ComputeHITS(ctx, e.store, hitsNodes, 10)
	}

	ranked := RankSymbols(inputs, hitsResult)
	block := packIntoBudget(ranked, budget, opts.Format)
	e.recordImplicitFeedback(ctx, block)
	return block, nil
}

// KeywordSet separates extracted keywords by specificity tier.
// Tiered search queries these in priority order: Exact first, then Compounds,
// then Components only as fallback. This prevents split components like "before"
// and "request" from drowning out the actual compound "before_request".
type KeywordSet struct {
	// Exact: backtick-quoted identifiers from the task description.
	// These are explicit symbol references (e.g., `before_request`).
	Exact []string
	// Compounds: multi-part identifiers detected by structure (snake_case,
	// CamelCase, dotted) or generated from bigram joining.
	Compounds []string
	// Components: individual words split from identifiers, abbreviation
	// expansions, and priority terms. Used as fallback when compounds
	// yield insufficient results.
	Components []string
}

// All returns all keywords in priority order (exact, compounds, components).
// Used by callers that don't need structured access.
func (ks KeywordSet) All() []string {
	result := make([]string, 0, len(ks.Exact)+len(ks.Compounds)+len(ks.Components))
	result = append(result, ks.Exact...)
	result = append(result, ks.Compounds...)
	result = append(result, ks.Components...)
	return result
}

// Primary returns the highest-priority keywords (exact + compounds).
// These should be queried first by tiered search.
func (ks KeywordSet) Primary() []string {
	result := make([]string, 0, len(ks.Exact)+len(ks.Compounds))
	result = append(result, ks.Exact...)
	result = append(result, ks.Compounds...)
	return result
}

// IsEmpty returns true if no keywords were extracted.
func (ks KeywordSet) IsEmpty() bool {
	return len(ks.Exact) == 0 && len(ks.Compounds) == 0 && len(ks.Components) == 0
}

// ExtractKeywordSet is the exported entry point for keyword extraction.
// Used by benchmarks and tooling that need structured access to extracted keywords.
func ExtractKeywordSet(desc string) KeywordSet {
	return extractKeywordSet(desc)
}

// extractKeywords is a backward-compatible wrapper that returns all keywords
// as a flat list in priority order. Use extractKeywordSet for structured access.
func extractKeywords(desc string) []string {
	return extractKeywordSet(desc).All()
}

// extractKeywordSet processes a task description into a structured keyword set.
// It detects backtick-quoted identifiers as exact symbols, preserves compound
// identifiers (snake_case, CamelCase, dotted), splits identifiers into components
// for fallback, and generates bigram compounds from adjacent words.
func extractKeywordSet(desc string) KeywordSet {
	var ks KeywordSet
	seen := make(map[string]bool)

	// Phase 1: Extract backtick-quoted identifiers as exact symbols.
	// These are explicit references to symbol names (e.g., `before_request`).
	remaining := desc
	for {
		start := strings.Index(remaining, "`")
		if start == -1 {
			break
		}
		end := strings.Index(remaining[start+1:], "`")
		if end == -1 {
			break
		}
		quoted := remaining[start+1 : start+1+end]
		remaining = remaining[start+1+end+1:]
		// Only accept identifiers (no spaces, reasonable length).
		quoted = strings.TrimSpace(quoted)
		if quoted != "" && len(quoted) <= 100 && !strings.Contains(quoted, " ") {
			if !seen[quoted] {
				seen[quoted] = true
				ks.Exact = append(ks.Exact, quoted)
			}
			// Also add lowercase variant if different.
			lower := strings.ToLower(quoted)
			if lower != quoted && !seen[lower] {
				seen[lower] = true
				ks.Exact = append(ks.Exact, lower)
			}
		}
	}

	// Phase 2: Standard word extraction.
	words := strings.Fields(desc)
	var priorityTerms []string
	var components []string

	actionVerbs := map[string]bool{
		"add": true, "implement": true, "create": true, "build": true,
		"fix": true, "refactor": true, "update": true, "change": true,
		"remove": true, "delete": true, "move": true,
		"find": true, "detect": true, "compute": true,
		"generate": true, "resolve": true, "wire": true, "connect": true,
		"extend": true, "introduce": true, "integrate": true,
	}
	fillerWords := map[string]bool{
		"a": true, "an": true, "the": true, "new": true, "existing": true,
		"all": true, "some": true, "each": true, "that": true, "which": true,
	}

	// Verb-pattern detection: the first noun after an action verb is the primary target.
	// If the target word is a compound identifier, it goes directly to Compounds
	// (highest specificity); otherwise to priorityTerms (Components).
	if len(words) > 0 {
		firstWord := strings.ToLower(strings.Trim(words[0], ".,;:!?\"'`()[]{}#"))
		if actionVerbs[firstWord] {
			for _, w := range words[1:] {
				clean := strings.Trim(w, ".,;:!?\"'`()[]{}#")
				cleanLower := strings.ToLower(clean)
				if cleanLower == "" || fillerWords[cleanLower] {
					continue
				}
				// Route compound identifiers to Compounds directly.
				isCompound := strings.Contains(cleanLower, "_") || strings.Contains(cleanLower, ".") || hasMixedCase(clean)
				if isCompound {
					if !seen[cleanLower] {
						seen[cleanLower] = true
						ks.Compounds = append(ks.Compounds, cleanLower)
					}
					if hasMixedCase(clean) && !seen[clean] {
						seen[clean] = true
						ks.Compounds = append(ks.Compounds, clean)
					}
				} else {
					if !seen[cleanLower] {
						seen[cleanLower] = true
						priorityTerms = append(priorityTerms, cleanLower)
					}
					if len(cleanLower) > 2 {
						capitalized := strings.ToUpper(cleanLower[:1]) + cleanLower[1:]
						if !seen[capitalized] {
							seen[capitalized] = true
							priorityTerms = append(priorityTerms, capitalized)
						}
					}
				}
				break
			}
		}
	}

	// Process each word: split identifiers, collect compounds and components.
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#")
		if w == "" {
			continue
		}

		parts := splitIdentifier(w)

		// If the word is a compound identifier (contains _ or splits into multiple parts),
		// add it to Compounds.
		lower := strings.ToLower(w)
		if strings.Contains(lower, "_") || strings.Contains(lower, ".") || len(parts) > 1 {
			if !seen[lower] && !stopWords[lower] {
				seen[lower] = true
				ks.Compounds = append(ks.Compounds, lower)
			}
			// Also preserve the original case if it has mixed case (CamelCase symbol).
			if hasMixedCase(w) && !seen[w] {
				seen[w] = true
				ks.Compounds = append(ks.Compounds, w)
			}
		}

		// Split into components for fallback.
		for _, p := range parts {
			pl := strings.ToLower(p)
			if len(pl) < 2 || stopWords[pl] || seen[pl] {
				continue
			}
			seen[pl] = true
			components = append(components, pl)

			if expanded, ok := abbreviations[pl]; ok && !seen[expanded] {
				seen[expanded] = true
				components = append(components, expanded)
			}
		}
	}

	// Phase 3: Bigram compound generation from adjacent non-stop-words.
	// These are SPECULATIVE (may not correspond to real symbols), so they go
	// to Components as fallback, not Compounds (which are only for identifiers
	// actually found in the task text). This prevents junk bigrams like
	// "TrackingThrough" from filling the tiered search cap before real terms.
	cleanWords := make([]string, 0, len(words))
	for _, w := range words {
		clean := strings.ToLower(strings.Trim(w, ".,;:!?\"'`()[]{}#"))
		if clean != "" && !stopWords[clean] && !fillerWords[clean] &&
			len(clean) >= 3 && !strings.ContainsAny(clean, "-/0123456789") {
			cleanWords = append(cleanWords, clean)
		}
	}
	var bigrams []string
	for i := 0; i < len(cleanWords)-1; i++ {
		a, b := cleanWords[i], cleanWords[i+1]
		if actionVerbs[a] || actionVerbs[b] {
			continue
		}
		if len(a) < 4 && len(b) < 4 {
			continue
		}
		camel := strings.ToUpper(a[:1]) + a[1:] + strings.ToUpper(b[:1]) + b[1:]
		if !seen[camel] {
			seen[camel] = true
			bigrams = append(bigrams, camel)
		}
		snake := a + "_" + b
		if !seen[snake] {
			seen[snake] = true
			bigrams = append(bigrams, snake)
		}
	}

	// Assemble Components: priority terms first, then individual words by length
	// descending, then bigrams LAST (speculative compounds that often don't match
	// real symbols and can dilute tiered search with junk like "ExtendRefactoring").
	ks.Components = make([]string, 0, len(priorityTerms)+len(bigrams)+len(components))
	ks.Components = append(ks.Components, priorityTerms...)
	sort.Slice(components, func(i, j int) bool {
		return len(components[i]) > len(components[j])
	})
	ks.Components = append(ks.Components, components...)
	ks.Components = append(ks.Components, bigrams...)

	return ks
}

// buildFTSQuery constructs an FTS5 query from extracted keywords.
// Compound identifiers (snake_case, CamelCase, dotted) are quoted as phrases
// and targeted at the symbol_name column for maximum BM25 relevance.
// Simple words are joined with OR for broad matching.
func buildFTSQuery(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}

	var parts []string
	for _, kw := range keywords {
		// Detect compound identifiers: contains underscore, dot, or mixed case.
		isCompound := strings.Contains(kw, "_") || strings.Contains(kw, ".") || hasMixedCase(kw)
		if isCompound {
			// Target the symbol_name column with a quoted phrase.
			// FTS5 syntax: {column_name} : "phrase"
			// The symbol_name column (index 0 in the virtual table) gets 10x weight,
			// so matches there dominate the ranking.
			// Only search symbol_name: unquoted compounds in all-column search
			// produce false positives because splitForFTS stores both the compound
			// and its split components (e.g., "before_request before request"),
			// so unrelated symbols match on the component tokens.
			escaped := strings.ReplaceAll(kw, "\"", "")
			parts = append(parts, fmt.Sprintf("symbol_name:\"%s\"", escaped))
		} else {
			parts = append(parts, kw)
		}
	}
	return strings.Join(parts, " OR ")
}

// decomposeCompounds breaks compound keywords (dotted, snake_case, CamelCase)
// into simple OR'd terms for FTS fallback when phrase matching yields 0 results.
// "ModelAdmin.get_inlines" -> ["ModelAdmin", "get_inlines"]
// "html.escape" -> ["html", "escape"]
// "before_request" -> ["before", "request"]
func decomposeCompounds(keywords []string) string {
	var parts []string
	seen := make(map[string]bool)
	for _, kw := range keywords {
		isCompound := strings.Contains(kw, "_") || strings.Contains(kw, ".") || hasMixedCase(kw)
		if !isCompound {
			if !seen[kw] {
				seen[kw] = true
				parts = append(parts, kw)
			}
			continue
		}
		// Split on dots first, then split each segment further.
		segments := strings.Split(kw, ".")
		for _, seg := range segments {
			for _, part := range splitIdentifier(seg) {
				lower := strings.ToLower(part)
				if len(lower) >= 3 && !seen[lower] {
					seen[lower] = true
					parts = append(parts, lower)
				}
				// Also add original case for CamelCase symbol matching.
				if part != lower && !seen[part] {
					seen[part] = true
					parts = append(parts, part)
				}
			}
		}
	}
	return strings.Join(parts, " OR ")
}

// decomposeCompoundsTargeted breaks compound keywords into symbol_name-targeted
// OR terms for FTS fallback. For dotted paths like "django.utils.html.escape",
// only the last two meaningful segments are kept (the class/method names, not
// the package path). For snake_case and CamelCase, all segments are kept.
// All terms target the symbol_name column for precision.
func decomposeCompoundsTargeted(keywords []string) string {
	var parts []string
	seen := make(map[string]bool)

	addPart := func(s string) {
		lower := strings.ToLower(s)
		if len(lower) < 3 || seen[lower] {
			return
		}
		seen[lower] = true
		parts = append(parts, fmt.Sprintf("symbol_name:%s", lower))
		if s != lower && !seen[s] {
			seen[s] = true
			parts = append(parts, fmt.Sprintf("symbol_name:%s", s))
		}
	}

	for _, kw := range keywords {
		isCompound := strings.Contains(kw, "_") || strings.Contains(kw, ".") || hasMixedCase(kw)
		if !isCompound {
			if len(kw) >= 3 && !seen[kw] {
				seen[kw] = true
				parts = append(parts, kw)
			}
			continue
		}

		if strings.Contains(kw, ".") {
			// Dotted path: take only the last 2 segments (leaf identifiers).
			// "django.utils.html.escape" -> ["html", "escape"] -> keep "escape"
			// "ModelAdmin.get_inlines" -> ["ModelAdmin", "get_inlines"] -> keep both
			dotParts := strings.Split(kw, ".")
			start := len(dotParts) - 2
			if start < 0 {
				start = 0
			}
			for _, seg := range dotParts[start:] {
				// Skip dunder methods: __getitem__ -> "getitem" is too generic.
				if strings.HasPrefix(seg, "__") && strings.HasSuffix(seg, "__") {
					continue
				}
				for _, part := range splitIdentifier(seg) {
					addPart(part)
				}
			}
		} else {
			// snake_case or CamelCase: split and keep all segments.
			for _, part := range splitIdentifier(kw) {
				addPart(part)
			}
		}
	}
	return strings.Join(parts, " OR ")
}

// resultConfidence measures how concentrated the RWR scores are.
// Returns 0.0 (completely flat, no signal) to 1.0 (strongly peaked).
// A flat distribution means RWR didn't converge on anything meaningful.
func resultConfidence(ranked []RankedSymbol) float64 {
	if len(ranked) < 10 {
		return 1.0 // too few results to judge
	}
	top := ranked[0].Score
	tenth := ranked[9].Score
	if top <= 0 {
		return 0.0
	}
	// Ratio: how much does #1 stand out from #10?
	// High ratio (>3x) = strong convergence. Low ratio (<1.5x) = flat.
	ratio := top / tenth
	if ratio > 3.0 {
		return 1.0
	}
	if ratio < 1.2 {
		return 0.0
	}
	// Linear interpolation between 1.2 and 3.0.
	return (ratio - 1.2) / 1.8
}

// directFTSExpansion finds symbols by direct FTS name matching and expands
// via contains edges (1-hop). Used as fallback when RWR produces flat results
// on massive repos. Returns up to limit nodes.
func directFTSExpansion(ctx stdctx.Context, store types.GraphStore, bm25 BM25Searcher, ks KeywordSet, limit int) []types.Node {
	if bm25 == nil {
		return nil
	}
	// Build a targeted query from the most specific keywords.
	// Use Compounds first (most specific), then longest Components.
	var queryTerms []string
	for _, c := range ks.Compounds {
		queryTerms = append(queryTerms, fmt.Sprintf("symbol_name:\"%s\"", c))
		if len(queryTerms) >= 3 {
			break
		}
	}
	if len(queryTerms) < 3 {
		for _, c := range ks.Components {
			if len(c) >= 6 && !strings.Contains(c, "_") {
				// Skip CamelCase bigrams.
				if len(c) > 1 && c[0] >= 'A' && c[0] <= 'Z' {
					hasLower := false
					skip := false
					for i := 1; i < len(c); i++ {
						if c[i] >= 'a' && c[i] <= 'z' {
							hasLower = true
						}
						if hasLower && c[i] >= 'A' && c[i] <= 'Z' {
							skip = true
							break
						}
					}
					if skip {
						continue
					}
				}
				queryTerms = append(queryTerms, fmt.Sprintf("symbol_name:\"%s\"", strings.ToLower(c)))
				if len(queryTerms) >= 5 {
					break
				}
			}
		}
	}
	if len(queryTerms) == 0 {
		return nil
	}
	query := strings.Join(queryTerms, " OR ")
	ftsResults, err := bm25.SearchBM25Nodes(ctx, query, limit)
	if err != nil || len(ftsResults) == 0 {
		return nil
	}

	// Expand: for each FTS result that's a type/class, add its members via contains edges.
	var expanded []types.Node
	seen := make(map[types.Hash]bool)
	for _, n := range ftsResults {
		if seen[n.NodeHash] {
			continue
		}
		seen[n.NodeHash] = true
		expanded = append(expanded, n)
		// If it's a type, add its contained methods/fields.
		if n.Kind == "type" || n.Kind == "class" || n.Kind == "interface" {
			members, _ := store.EdgesFrom(ctx, n.NodeHash, "contains")
			for _, e := range members {
				if seen[e.TargetHash] {
					continue
				}
				if member, err := store.GetNode(ctx, e.TargetHash); err == nil && member != nil {
					seen[member.NodeHash] = true
					expanded = append(expanded, *member)
				}
				if len(expanded) >= limit*2 {
					break
				}
			}
		}
	}
	if len(expanded) > limit {
		expanded = expanded[:limit]
	}
	return expanded
}

// detectRepoLanguage determines the primary language of a repo by sampling node QNs.
// Returns: "go", "python", "typescript", "ruby", "java", "csharp", "rust", or "" (unknown).
// Uses a fast heuristic: samples 500 nodes and checks file extensions in QNs.
func detectRepoLanguage(ctx stdctx.Context, store types.GraphStore) string {
	// Sample nodes across the full range to catch all file types.
	allNodes, err := store.NodesByName(ctx, "%")
	if err != nil || len(allNodes) == 0 {
		return ""
	}
	counts := map[string]int{}
	// Sample evenly across the node list to avoid bias from alphabetical ordering.
	step := len(allNodes) / 500
	if step < 1 {
		step = 1
	}
	start := 0
	end := len(allNodes)
	for i := start; i < end; i += step {
		qn := allNodes[i].QualifiedName
		switch {
		case strings.Contains(qn, ".go."):
			counts["go"]++
		case strings.Contains(qn, ".py."):
			counts["python"]++
		case strings.Contains(qn, ".ts.") || strings.Contains(qn, ".tsx."):
			counts["typescript"]++
		case strings.Contains(qn, ".rb."):
			counts["ruby"]++
		case strings.Contains(qn, ".java."):
			counts["java"]++
		case strings.Contains(qn, ".cs."):
			counts["csharp"]++
		case strings.Contains(qn, ".rs."):
			counts["rust"]++
		// Java fallback: dotted package names (org.apache.kafka, com.example)
		case strings.Contains(qn, "://org.") || strings.Contains(qn, "://com.") || strings.Contains(qn, "://io.") || strings.Contains(qn, "://net."):
			counts["java"]++
		}
	}
	best := ""
	bestCount := 0
	for lang, count := range counts {
		if count > bestCount {
			best = lang
			bestCount = count
		}
	}
	return best
}

// hasMixedCase returns true if the string has both uppercase and lowercase letters
// (indicating a CamelCase identifier like "SessionInterface" or "QuerySet").
func hasMixedCase(s string) bool {
	hasUpper, hasLower := false, false
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
		if r >= 'a' && r <= 'z' {
			hasLower = true
		}
		if hasUpper && hasLower {
			return true
		}
	}
	return false
}

// splitIdentifier splits a CamelCase or snake_case identifier into component words.
func splitIdentifier(s string) []string {
	// Handle snake_case.
	if strings.Contains(s, "_") {
		parts := strings.Split(s, "_")
		var result []string
		for _, p := range parts {
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	}

	// Handle CamelCase.
	var parts []string
	current := strings.Builder{}
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// packIntoBudget selects symbols to maximize total relevance within the token budget.
// It uses density-ranked packing: symbols are sorted by score/cost ratio so that
// small high-value symbols (types, constants) are preferred over large medium-value
// symbols (long functions) when budget is tight. This is a greedy fractional knapsack
// approximation that outperforms pure score-order packing on constrained budgets.
func packIntoBudget(ranked []RankedSymbol, budget int, format string) *ContextBlock {
	block := &ContextBlock{
		Format:      format,
		TokenBudget: budget,
	}

	if len(ranked) == 0 {
		return block
	}

	// Compute density (score per token) with RWR proximity boost.
	// Symbols with higher raw RWR scores are closer to seeds in the graph.
	// Boosting their packing density ensures structurally proximate symbols
	// fill budget slots before distant high-centrality noise. Zero extra
	// computation: RWR scores are already available from the walk.
	type densityItem struct {
		index   int
		density float64
		cost    int
	}
	items := make([]densityItem, len(ranked))
	for i, sym := range ranked {
		cost := EstimateNodeTokensForFormat(sym.Node, format)
		if cost < 1 {
			cost = 1
		}
		baseDensity := sym.Score / float64(cost)
		// RWR proximity boost: rwrScore^exponent smooths the decay curve.
		// Seeds (~1.0) get full density, distant nodes (~0.01) get reduced density.
		// Exponent controls aggressiveness: 0.5=sqrt (default), 0.3=gentle, 0.7=aggressive, 1.0=linear.
		// Sweep via BENCH_PROXIMITY_EXP env var.
		proximityFactor := 1.0
		if sym.RWRScore > 0 {
			proximityFactor = math.Pow(sym.RWRScore, proximityExponent())
		}
		items[i] = densityItem{
			index:   i,
			density: baseDensity * proximityFactor,
			cost:    cost,
		}
	}

	// Sort by density descending. Ties broken by raw score (higher first).
	sort.Slice(items, func(a, b int) bool {
		if items[a].density != items[b].density {
			return items[a].density > items[b].density
		}
		return ranked[items[a].index].Score > ranked[items[b].index].Score
	})

	// Greedily pack by density order with optional coherence bonus.
	// When CoherenceBonus > 0, each iteration picks the best unpacked item
	// considering a density boost for symbols sharing a file with already-packed
	// symbols. This favors coherent subgraphs over scattered singletons.
	var tokensUsed int
	if CoherenceBonus > 0 {
		packedFiles := make(map[types.Hash]bool)
		packed := make([]bool, len(items))
		for range items {
			bestIdx := -1
			bestDensity := -1.0
			for i, item := range items {
				if packed[i] || tokensUsed+item.cost > budget {
					continue
				}
				d := item.density
				if packedFiles[ranked[item.index].Node.FileHash] {
					d *= (1.0 + CoherenceBonus)
				}
				if d > bestDensity {
					bestDensity = d
					bestIdx = i
				}
			}
			if bestIdx < 0 {
				break
			}
			packed[bestIdx] = true
			tokensUsed += items[bestIdx].cost
			sym := ranked[items[bestIdx].index]
			block.Symbols = append(block.Symbols, sym)
			packedFiles[sym.Node.FileHash] = true
		}
	} else {
		// Original greedy pack (no coherence).
		for _, item := range items {
			if tokensUsed+item.cost > budget {
				continue
			}
			tokensUsed += item.cost
			block.Symbols = append(block.Symbols, ranked[item.index])
		}
	}

	// Re-sort the packed symbols by score descending for output ordering.
	sort.Slice(block.Symbols, func(i, j int) bool {
		return block.Symbols[i].Score > block.Symbols[j].Score
	})

	block.TokensUsed = tokensUsed
	return block
}

// filterNoisySymbols removes symbols from minified bundles, dist/,
// vendor/, node_modules/, and phantom external nodes that pollute retrieval results.
func filterNoisySymbols(nodes []types.Node) []types.Node {
	var filtered []types.Node
	for _, n := range nodes {
		// Skip phantom external nodes and stdlib nodes.
		// External: unresolved targets from LSP enrichment with no source code.
		// Stdlib: standard library functions that have extreme in-degree but are
		// never useful as context for a coding task.
		if n.Kind == "external" || strings.HasPrefix(n.QualifiedName, "external://") || strings.HasPrefix(n.QualifiedName, "stdlib://") {
			continue
		}
		qn := strings.ToLower(n.QualifiedName)
		// Skip minified JS bundles and build artifacts.
		if strings.Contains(qn, "/dist/") || strings.Contains(qn, "/build/") ||
			strings.Contains(qn, "/vendor/") || strings.Contains(qn, "/node_modules/") ||
			strings.Contains(qn, ".min.") || strings.Contains(qn, ".bundle.") {
			continue
		}
		// Skip test fixtures and test helpers (they have high call-edge counts
		// from being called by many tests, but are noise for most tasks).
		if strings.Contains(qn, "conftest.py.") || strings.Contains(qn, "fixtures.py.") ||
			strings.Contains(qn, "/testutil") || strings.Contains(qn, "/testhelper") ||
			strings.Contains(qn, "test_helper") {
			continue
		}
		// Skip test mock implementations (they duplicate real interface methods).
		// Match patterns like "mockStore.PutEdge" or "walkMockStore.EdgesFrom".
		lastDot := strings.LastIndex(n.QualifiedName, ".")
		if lastDot >= 0 {
			// Get the type name (between the second-to-last and last dot).
			prefix := n.QualifiedName[:lastDot]
			typeStart := strings.LastIndex(prefix, ".")
			if typeStart >= 0 {
				typeName := strings.ToLower(prefix[typeStart+1:])
				if strings.Contains(typeName, "mock") || strings.Contains(typeName, "fake") ||
					strings.Contains(typeName, "stub") {
					continue
				}
			}
		}
		// Skip symbols with very short names that look minified (e.g. "nR", "v4").
		if lastDot >= 0 {
			symName := n.QualifiedName[lastDot+1:]
			if len(symName) <= 2 && !isCommonShortName(symName) {
				continue
			}
		}
		filtered = append(filtered, n)
	}
	return filtered
}

// isCommonShortName returns true for legitimate short symbol names.
func isCommonShortName(name string) bool {
	common := map[string]bool{
		"ID": true, "OK": true, "Go": true, "Do": true,
		"DB": true, "IP": true, "IO": true,
	}
	return common[name]
}

// isTestRelatedTask returns true if the task description is about testing,
// in which case test file symbols should NOT be penalized.
func isTestRelatedTask(desc string) bool {
	lower := strings.ToLower(desc)
	testKeywords := []string{"test", "testing", "spec", "unit test", "integration test", "mock", "fixture", "assert"}
	for _, kw := range testKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isTestFilePath detects whether a qualified name refers to a symbol in a test file.
// Uses file path patterns (not symbol names) to avoid false positives on production
// code that legitimately contains "test" in a name (e.g., TestConnection, contest).
func isTestFilePath(qualifiedName string) bool {
	lower := strings.ToLower(qualifiedName)
	// Go test files.
	if strings.Contains(lower, "_test.go") {
		return true
	}
	// Python test files and directories.
	if strings.Contains(lower, "/tests/") || strings.Contains(lower, "/test_") ||
		strings.Contains(lower, "tests/test_") {
		return true
	}
	// TypeScript/JavaScript test files and directories.
	if strings.Contains(lower, ".test.") || strings.Contains(lower, ".spec.") ||
		strings.Contains(lower, "/__tests__/") || strings.Contains(lower, "/test/") {
		return true
	}
	// Rust test modules (tests/ directory at crate root).
	if strings.Contains(lower, "/tests/") {
		return true
	}
	return false
}

// rankedChannel is one retrieval channel's output with its RRF weight.
type rankedChannel struct {
	nodes  []types.Node
	weight float64
}

// rrfFuseMulti merges N ranked node lists using weighted Reciprocal Rank Fusion.
// Each channel contributes: weight * 1/(k + rank + 1) for each node it contains.
// Nodes appearing in multiple channels accumulate scores from all of them.
// k=60 is the standard RRF constant. Returns up to limit nodes sorted by fused score.
func rrfFuseMulti(channels []rankedChannel, k, limit int) []types.Node {
	type scored struct {
		node  types.Node
		score float64
	}
	scores := make(map[types.Hash]*scored)

	for _, ch := range channels {
		if ch.weight == 0 || len(ch.nodes) == 0 {
			continue
		}
		for rank, n := range ch.nodes {
			s, ok := scores[n.NodeHash]
			if !ok {
				s = &scored{node: n}
				scores[n.NodeHash] = s
			}
			s.score += ch.weight / float64(k+rank+1)
		}
	}

	// Sort by fused score descending.
	results := make([]scored, 0, len(scores))
	for _, s := range scores {
		results = append(results, *s)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	nodes := make([]types.Node, len(results))
	for i, r := range results {
		nodes[i] = r.node
	}
	return nodes
}

// compressZlib compresses data using zlib (level 6, good balance of speed/ratio).
func compressZlib(data []byte) []byte {
	var buf bytes.Buffer
	w, _ := zlib.NewWriterLevel(&buf, zlib.DefaultCompression)
	w.Write(data) //nolint:errcheck
	w.Close()     //nolint:errcheck
	return buf.Bytes()
}

// decompressZlib decompresses zlib-compressed data.
func decompressZlib(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// filterByRepoPrefix returns only nodes whose QualifiedName starts with prefix.
// Used to scope search results to a single repo in multi-repo DBs.
func filterByRepoPrefix(nodes []types.Node, prefix string) []types.Node {
	var result []types.Node
	for _, n := range nodes {
		if strings.HasPrefix(n.QualifiedName, prefix) {
			result = append(result, n)
		}
	}
	return result
}

// extractPathTerms extracts package/directory-like terms from a task description.
// These are lowercase words (>= 4 chars) that could be directory or package names,
// excluding common English words that would match too broadly.
//
// Examples:
//   - "custom migration operation" -> ["migration"]
//   - "EndpointSlice controller reconciliation" -> ["endpointslice", "controller", "reconciliation"]
//   - "scheduler framework plugin" -> ["scheduler", "framework", "plugin"]
//   - "CSI driver provisioning" -> ["driver", "provisioning"]
func extractPathTerms(desc string) []string {
	// Words that appear commonly in task descriptions but rarely as package names.
	// These would match too many unrelated nodes if used as path queries.
	pathStopWords := map[string]bool{
		"about": true, "above": true, "after": true, "again": true, "also": true,
		"always": true, "based": true, "because": true, "been": true, "before": true,
		"being": true, "between": true, "both": true, "called": true, "change": true,
		"check": true, "code": true, "could": true, "create": true, "custom": true,
		"data": true, "debug": true, "default": true, "does": true, "doing": true,
		"done": true, "during": true, "each": true, "ensure": true, "every": true,
		"existing": true, "extend": true, "field": true, "file": true, "find": true,
		"first": true, "following": true, "from": true, "function": true, "given": true,
		"have": true, "here": true, "high": true, "implement": true, "into": true,
		"just": true, "keep": true, "know": true, "large": true, "like": true,
		"list": true, "look": true, "make": true, "many": true, "method": true,
		"more": true, "most": true, "much": true, "must": true, "name": true,
		"need": true, "never": true, "node": true, "only": true, "open": true,
		"other": true, "over": true, "part": true, "pass": true, "path": true,
		"place": true, "point": true, "read": true, "request": true, "result": true,
		"return": true, "right": true, "same": true, "should": true, "show": true,
		"since": true, "single": true, "some": true, "specific": true, "start": true,
		"state": true, "step": true, "still": true, "support": true, "system": true,
		"take": true, "test": true, "text": true, "that": true, "then": true,
		"there": true, "these": true, "they": true, "thing": true, "this": true,
		"through": true, "time": true, "trace": true, "tracing": true, "type": true,
		"under": true, "update": true, "used": true, "using": true, "value": true,
		"very": true, "want": true, "well": true, "what": true, "when": true,
		"where": true, "which": true, "while": true, "will": true, "with": true,
		"within": true, "without": true, "work": true, "would": true, "write": true,
		"zero": true, "adding": true, "added": true, "calls": true,
		"handle": true, "handling": true, "level": true, "logic": true, "main": true,
		"module": true, "object": true, "operation": true, "option": true, "order": true,
		"perform": true, "process": true, "provide": true, "running": true, "service": true,
		"setting": true, "source": true, "string": true, "class": true, "model": true,
		"view": true, "base": true, "error": true, "response": true, "down": true,
	}

	words := strings.Fields(desc)
	var terms []string
	seen := make(map[string]bool)

	for _, w := range words {
		// Strip punctuation.
		clean := strings.Trim(w, ".,;:!?\"'`()[]{}#<>")
		if clean == "" {
			continue
		}

		// Split CamelCase into components for path matching.
		// "EndpointSlice" -> also check "endpointslice" (as-is, lowercased)
		lower := strings.ToLower(clean)

		// Skip short words and stop words.
		if len(lower) < 4 || pathStopWords[lower] {
			continue
		}

		// Skip if it contains special chars (URLs, flags, etc.)
		if strings.ContainsAny(lower, "=/<>{}[]()@#$%^&*+") {
			continue
		}

		// Skip pure numbers.
		allDigit := true
		for _, r := range lower {
			if r < '0' || r > '9' {
				allDigit = false
				break
			}
		}
		if allDigit {
			continue
		}

		if !seen[lower] {
			seen[lower] = true
			terms = append(terms, lower)
		}

		// For compound identifiers like "EndpointSlice" or "endpoint_slice",
		// also extract sub-components that are >= 4 chars.
		if strings.Contains(clean, "_") {
			parts := strings.Split(clean, "_")
			for _, p := range parts {
				pl := strings.ToLower(p)
				if len(pl) >= 4 && !pathStopWords[pl] && !seen[pl] {
					seen[pl] = true
					terms = append(terms, pl)
				}
			}
		} else if hasMixedCase(clean) {
			// CamelCase split.
			parts := splitCamelWords(clean)
			for _, p := range parts {
				pl := strings.ToLower(p)
				if len(pl) >= 4 && !pathStopWords[pl] && !seen[pl] {
					seen[pl] = true
					terms = append(terms, pl)
				}
			}
		}
	}

	// Limit to top 5 most specific terms (longer = more specific).
	sort.Slice(terms, func(i, j int) bool {
		return len(terms[i]) > len(terms[j])
	})
	if len(terms) > 5 {
		terms = terms[:5]
	}

	return terms
}

// isPathMatch checks if a term appears in the path portion of a qualified name,
// not just in the terminal symbol name. This prevents false positives where
// a package term accidentally matches a method name.
//
// QualifiedName formats:
//   - Go: "github.com/org/repo://pkg/controller/endpointslice/reconciler.reconcile"
//   - Python: "github.com/org/repo://django/db/migrations/operations/base.py.Operation"
//   - Rust: "github.com/org/repo://src/cargo/core/resolver/mod.rs.resolve"
//
// The term must start at a path boundary (after /, ., _, -, :) but may be a
// prefix of the segment (e.g., "migration" matches "migrations/").
func isPathMatch(qualifiedName, term string) bool {
	lower := strings.ToLower(qualifiedName)
	termLower := strings.ToLower(term)

	// Find the path portion: everything up to the terminal symbol name.
	// Terminal name starts after the last file extension marker (.py., .go., .rs., .ts., .java.)
	// or if none, after the last dot following a slash.
	pathPortion := lower
	exts := []string{".py.", ".go.", ".rs.", ".ts.", ".java.", ".cs.", ".rb.", ".js."}
	for _, ext := range exts {
		if idx := strings.LastIndex(lower, ext); idx >= 0 {
			pathPortion = lower[:idx+len(ext)]
			break
		}
	}
	if pathPortion == lower {
		// No file extension found. Use everything before last dot after last slash.
		lastSlash := strings.LastIndex(lower, "/")
		if lastSlash >= 0 {
			afterSlash := lower[lastSlash:]
			if dotIdx := strings.Index(afterSlash, "."); dotIdx >= 0 {
				pathPortion = lower[:lastSlash+dotIdx]
			}
		}
	}

	// Check if term appears in the path starting at a boundary.
	// Allow term to be a PREFIX of a path segment (e.g., "migration" matches in
	// "/migrations/" because "migration" starts at a boundary and is a prefix).
	idx := strings.Index(pathPortion, termLower)
	if idx < 0 {
		return false
	}

	// Verify left boundary: character before should be a separator or start.
	if idx > 0 {
		prev := pathPortion[idx-1]
		if prev != '/' && prev != '.' && prev != '_' && prev != '-' && prev != ':' {
			return false
		}
	}

	// Right boundary: the term can be a prefix of the segment. No right-boundary
	// check needed because we already confirmed it's in the path portion (not the
	// terminal symbol name). The path portion guarantee prevents false positives
	// like "cache" matching "CacheHandler" (which would be in the symbol portion).

	return true
}

// qualifiedNamePkg extracts the package path from a QualifiedName.
// Format: "{repoURL}://{pkgPath}.{TypeName}.{SymbolName}"
// e.g. "github.com/foo/bar/pkg.Type.Method" -> "github.com/foo/bar/pkg"
func qualifiedNamePkg(qn string) string {
	idx := strings.Index(qn, "://")
	if idx < 0 {
		return ""
	}
	path := qn[idx+3:]
	parts := strings.Split(path, ".")
	var pkgParts []string
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		// Package components are lowercase; symbol components start with upper or are keywords.
		if p[0] >= 'A' && p[0] <= 'Z' {
			break
		}
		pkgParts = append(pkgParts, p)
	}
	if len(pkgParts) == 0 {
		return path
	}
	return strings.Join(pkgParts, ".")
}

// dominantPkg returns the package path that appears most among candidates.
func dominantPkg(candidates []types.Node) string {
	counts := make(map[string]int)
	best, bestN := "", 0
	for _, c := range candidates {
		pkg := qualifiedNamePkg(c.QualifiedName)
		counts[pkg]++
		if counts[pkg] > bestN {
			bestN = counts[pkg]
			best = pkg
		}
	}
	if bestN < 2 {
		return ""
	}
	return best
}

// boostByPathMatch reorders candidates so those whose file path contains task keywords
// are promoted. On large repos (terraform 76K nodes), BM25 returns symbols from many
// packages. A candidate from "internal/command/validate.go" should rank above one from
// "internal/rpcapi/cli.go" when the task mentions "validate" and "command".
func boostByPathMatch(candidates []types.Node, pathTerms []string) []types.Node {
	if len(pathTerms) == 0 {
		return candidates
	}

	type scored struct {
		node  types.Node
		score int
		idx   int
	}

	items := make([]scored, len(candidates))
	for i, c := range candidates {
		// Extract file path from QualifiedName (between "://" and the last file extension dot)
		pathScore := 0
		qn := strings.ToLower(c.QualifiedName)
		for _, term := range pathTerms {
			if strings.Contains(qn, strings.ToLower(term)) {
				pathScore++
			}
		}
		items[i] = scored{node: c, score: pathScore, idx: i}
	}

	// Stable sort: higher path score first, preserve original order within same score.
	sort.SliceStable(items, func(a, b int) bool {
		return items[a].score > items[b].score
	})

	result := make([]types.Node, len(candidates))
	for i, item := range items {
		result[i] = item.node
	}
	return result
}

// focusedSeedSelect reorders candidates so structurally cohesive seeds come first.
// Instead of scattering 15-25 seeds across the graph, it clusters by package path
// and promotes the largest cluster to the front. The maxSeeds cap downstream then
// naturally selects from this focused set.
func focusedSeedSelect(candidates []types.Node) []types.Node {
	// Cluster candidates by package.
	type cluster struct {
		pkg     string
		indices []int
	}
	clusterMap := make(map[string]*cluster)
	var clusterOrder []string
	for i, c := range candidates {
		pkg := qualifiedNamePkg(c.QualifiedName)
		if cl, ok := clusterMap[pkg]; ok {
			cl.indices = append(cl.indices, i)
		} else {
			clusterMap[pkg] = &cluster{pkg: pkg, indices: []int{i}}
			clusterOrder = append(clusterOrder, pkg)
		}
	}

	// Find the largest cluster.
	bestPkg := ""
	bestSize := 0
	for _, pkg := range clusterOrder {
		if len(clusterMap[pkg].indices) > bestSize {
			bestSize = len(clusterMap[pkg].indices)
			bestPkg = pkg
		}
	}

	// If no dominant cluster (all singletons), return as-is.
	if bestSize < 2 {
		return candidates
	}

	// Reorder: largest cluster first (preserving RRF rank within), then rest.
	result := make([]types.Node, 0, len(candidates))
	inBest := make(map[int]bool, bestSize)
	for _, idx := range clusterMap[bestPkg].indices {
		result = append(result, candidates[idx])
		inBest[idx] = true
	}
	for i, c := range candidates {
		if !inBest[i] {
			result = append(result, c)
		}
	}
	return result
}
