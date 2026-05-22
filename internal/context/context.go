package context

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
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

// ContextEngine queries the knowing knowledge graph to produce task-specific,
// token-budgeted context blocks ranked by graph relationships and runtime traffic.
type ContextEngine struct {
	store    types.GraphStore
	feedback FeedbackProvider    // nil if store doesn't support feedback
	bm25     BM25Searcher        // nil if store doesn't support BM25
	vector   VectorSearcher      // nil if embeddings not available
	session  *SessionTracker     // nil disables session-aware boosting
	memory   *TaskMemory         // nil disables passive task memory
	cache    *cache.SubgraphCache // nil disables subgraph result caching
}

// TaskOptions configures a task-based context query.
type TaskOptions struct {
	TaskDescription string
	TokenBudget     int    // default 50000
	Format          string // "xml", "markdown", "json"
	DBPath          string // path to knowing.db (for CLI usage)
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
	Node       types.Node
	Score      float64
	Components ScoreComponents
	Provenance string
	Distance   int
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
	if bs, ok := store.(BM25Searcher); ok {
		e.bm25 = bs
	}
	return e
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
	// Action words that describe intent but don't identify code
	"refactor": true, "fix": true, "update": true, "change": true,
	"modify": true, "implement": true, "create": true, "delete": true,
	"remove": true, "move": true, "rename": true, "improve": true,
	"optimize": true, "debug": true, "test": true, "review": true,
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
	ks := extractKeywordSet(opts.TaskDescription)
	if ks.IsEmpty() {
		return &ContextBlock{
			Format:      opts.Format,
			TokenBudget: budget,
		}, nil
	}
	keywords := ks.All()

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
	if note, err := e.store.GetNote(ctx, packNoteKey, "context_pack"); err == nil && note != nil {
		var persisted persistedContextPack
		if err := json.Unmarshal([]byte(note.Value), &persisted); err == nil {
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

	// Seed retrieval uses independent channels fused with Reciprocal Rank Fusion:
	//   Channel 1: Tiered keyword matching (compound-first: exact > prefix > substring > path)
	//   Channel 2: BM25 full-text search (compound-targeted symbol_name column)
	// RRF merges their ranked lists: score = 1/(k+rank_tier) + 1/(k+rank_bm25)
	// Symbols appearing in both lists are promoted; single-channel hits get less weight.

	// Channel 1: Compound-first tiered keyword matching.
	tieredResults, _, _ := e.tieredSearchSet(ctx, ks)

	// Channel 2: BM25 full-text search (always runs when available).
	var bm25Results []types.Node
	if e.bm25 != nil {
		ftsQuery := buildFTSQuery(keywords)
		if nodes, err := e.bm25.SearchBM25Nodes(ctx, ftsQuery, 30); err == nil {
			bm25Results = nodes
		}
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
	eqMatches := matchEquivalenceClasses(opts.TaskDescription, allClasses)

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
	for _, m := range eqMatches {
		for _, target := range m.targets {
			nodes, err := e.store.NodesByName(ctx, "%"+target)
			if err != nil {
				continue
			}
			for _, n := range nodes {
				if equivSeen[n.NodeHash] {
					continue
				}
				lastDot := strings.LastIndex(n.QualifiedName, ".")
				symName := n.QualifiedName
				if lastDot >= 0 {
					symName = n.QualifiedName[lastDot+1:]
				}
				if strings.EqualFold(symName, target) {
					equivSeen[n.NodeHash] = true
					equivResults = append(equivResults, n)
				}
			}
		}
	}

	// Fuse all channels with weighted Reciprocal Rank Fusion.
	// Weights: tiered 2x (exact/prefix, most precise), BM25 2x (relevance-ranked,
	// complementary to tiered via signature and multi-term matching),
	// equivalence 2x (concept-level), vector 0x (disabled, infrastructure preserved).
	// Equal weights for tiered+BM25: both find symbols by name but BM25 adds
	// signature matching and relevance ranking that tiered lacks.
	// k=60 is the standard RRF constant.
	candidates := rrfFuseMulti([]rankedChannel{
		{nodes: tieredResults, weight: 2.0},
		{nodes: bm25Results, weight: 2.0},
		{nodes: equivResults, weight: 2.0},
		{nodes: vectorResults, weight: 0.0},
	}, 60, 40)
	seen := make(map[types.Hash]bool, len(candidates))
	for _, c := range candidates {
		seen[c.NodeHash] = true
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

	// Determine candidate communities for scoped RWR.
	// If candidates cluster in 1-3 communities, constrain the walk.
	commCounts := make(map[int]int)
	type communityProvider interface {
		CommunitiesForNodes(ctx stdctx.Context, hashes []types.Hash) (map[types.Hash]int, error)
	}
	if cp, ok := e.store.(communityProvider); ok {
		hashes := make([]types.Hash, len(candidates))
		for i, c := range candidates {
			hashes[i] = c.NodeHash
		}
		if commMap, err := cp.CommunitiesForNodes(ctx, hashes); err == nil {
			for _, commID := range commMap {
				commCounts[commID]++
			}
		}
	}

	// Run Random Walk with Restart from seed nodes to compute relevance
	// scores across the entire reachable subgraph. This replaces manual
	// neighbor expansion with a principled graph-based relevance signal.
	seedHashes := make([]types.Hash, 0, len(candidates))
	seedSet := make(map[types.Hash]bool)
	for _, c := range candidates {
		seedHashes = append(seedHashes, c.NodeHash)
		seedSet[c.NodeHash] = true
	}

	// Community-aware RWR: if seeds cluster in 1-3 communities, constrain the
	// walk to those communities. This prevents drifting into unrelated packages.
	// If seeds span 4+ communities (diverse query), run unconstrained.
	var rwrScores map[types.Hash]float64
	var err error
	if len(commCounts) > 0 && len(commCounts) <= 3 {
		communityIDs := make(map[int]bool, len(commCounts))
		for cid := range commCounts {
			communityIDs[cid] = true
		}
		rwrScores, err = CommunityFilteredRWR(ctx, e.store, seedHashes, 0.2, 20, communityIDs)
	} else {
		rwrScores, err = RandomWalkWithRestart(ctx, e.store, seedHashes, 0.2, 20)
	}
	if err != nil {
		return nil, err
	}

	// Build scoring inputs from all nodes that received a non-trivial RWR score.
	// Threshold 0.02: balance between expanding the candidate pool for HITS/feedback
	// reranking and avoiding noise from distant, weakly-connected nodes.
	var inputs []ScoringInput
	for nodeHash, rwrScore := range rwrScores {
		if rwrScore < 0.02 {
			continue // skip negligible nodes
		}

		node, err := e.store.GetNode(ctx, nodeHash)
		if err != nil {
			return nil, err
		}
		if node == nil {
			continue
		}

		// Skip phantom external nodes reached by RWR walk.
		// These are unresolved targets from LSP enrichment with no source code.
		if node.Kind == "external" || strings.HasPrefix(node.QualifiedName, "external://") {
			continue
		}

		// Determine distance: 0 if seed (direct keyword match), 1 otherwise.
		distance := 1
		if seedSet[nodeHash] {
			distance = 0
		}

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

		// Use RWR score as the caller count proxy. Scale to an integer
		// range that the ranking algorithm can normalize (0-100).
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
	if repos, err := e.store.AllRepos(ctx); err == nil && len(repos) > 0 {
		if snap, err := e.store.LatestSnapshot(ctx, repos[0].RepoHash); err == nil && snap != nil {
			persisted := persistedContextPack{
				Block:        cachedBlock,
				SnapshotHash: snap.SnapshotHash,
			}
			if data, err := json.Marshal(persisted); err == nil {
				_ = e.store.PutNote(ctx, types.Note{
					ObjectHash: packNoteKey,
					Key:        "context_pack",
					Value:      string(data),
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
	return packIntoBudget(ranked, budget, opts.Format), nil
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
	return packIntoBudget(ranked, budget, opts.Format), nil
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
		"remove": true, "delete": true, "move": true, "rename": true,
		"find": true, "detect": true, "trace": true, "compute": true,
		"generate": true, "resolve": true, "wire": true, "connect": true,
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
	cleanWords := make([]string, 0, len(words))
	for _, w := range words {
		clean := strings.ToLower(strings.Trim(w, ".,;:!?\"'`()[]{}#"))
		if clean != "" && !stopWords[clean] && !fillerWords[clean] &&
			len(clean) >= 3 && !strings.ContainsAny(clean, "-/0123456789") {
			cleanWords = append(cleanWords, clean)
		}
	}
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
			ks.Compounds = append(ks.Compounds, camel)
		}
		snake := a + "_" + b
		if !seen[snake] {
			seen[snake] = true
			ks.Compounds = append(ks.Compounds, snake)
		}
	}

	// Assemble Components: priority terms first, then remaining by length descending.
	ks.Components = make([]string, 0, len(priorityTerms)+len(components))
	ks.Components = append(ks.Components, priorityTerms...)
	sort.Slice(components, func(i, j int) bool {
		return len(components[i]) > len(components[j])
	})
	ks.Components = append(ks.Components, components...)

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
			escaped := strings.ReplaceAll(kw, "\"", "")
			parts = append(parts, fmt.Sprintf("symbol_name:\"%s\"", escaped))
			// Also add unquoted for partial matches in other columns.
			parts = append(parts, escaped)
		} else {
			parts = append(parts, kw)
		}
	}
	return strings.Join(parts, " OR ")
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

	// Compute density (score per token) for each symbol.
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
		items[i] = densityItem{
			index:   i,
			density: sym.Score / float64(cost),
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

	// Greedily pack by density order.
	var tokensUsed int
	for _, item := range items {
		if tokensUsed+item.cost > budget {
			continue // skip this item, try smaller ones
		}
		tokensUsed += item.cost
		block.Symbols = append(block.Symbols, ranked[item.index])
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
		// Skip phantom external nodes created during LSP enrichment.
		// These are unresolved targets with no source code (kind "external"
		// or qualified_name starting with "external://").
		if n.Kind == "external" || strings.HasPrefix(n.QualifiedName, "external://") {
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
