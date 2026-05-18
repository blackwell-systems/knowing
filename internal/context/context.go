package context

import (
	stdctx "context"
	"sort"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// FeedbackProvider is implemented by stores that support feedback queries.
type FeedbackProvider interface {
	FeedbackBoosts(ctx stdctx.Context, hashes []types.Hash) (map[types.Hash]float64, error)
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
	feedback FeedbackProvider // nil if store doesn't support feedback
	bm25     BM25Searcher    // nil if store doesn't support BM25
	vector   VectorSearcher   // nil if embeddings not available
	session  *SessionTracker  // nil disables session-aware boosting
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

// SetVector attaches a vector search backend to the engine. When set,
// query embeddings are compared against indexed symbol embeddings via
// nearest-neighbor search, providing a third RRF channel alongside
// tiered keyword matching and BM25.
func (e *ContextEngine) SetVector(vs VectorSearcher) {
	e.vector = vs
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

	// Extract keywords from the task description.
	keywords := extractKeywords(opts.TaskDescription)
	if len(keywords) == 0 {
		return &ContextBlock{
			Format:      opts.Format,
			TokenBudget: budget,
		}, nil
	}

	// Seed retrieval uses two independent channels fused with Reciprocal Rank Fusion:
	//   Channel 1: Tiered keyword matching (exact > prefix > substring > path)
	//   Channel 2: BM25 full-text search (CamelCase-split qualified names + signatures)
	// RRF merges their ranked lists: score = 1/(k+rank_tier) + 1/(k+rank_bm25)
	// Symbols appearing in both lists are promoted; single-channel hits get less weight.
	// This avoids the dilution problem of naive concatenation while letting BM25 always run.

	// Channel 1: Tiered keyword matching.
	var tieredResults []types.Node
	tieredSeen := make(map[types.Hash]bool)

	// Tier 1: exact matches (keyword matches the symbol name exactly).
	for _, kw := range keywords {
		nodes, err := e.store.NodesByName(ctx, "%"+kw)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			lastDot := strings.LastIndex(n.QualifiedName, ".")
			symbolName := n.QualifiedName
			if lastDot >= 0 {
				symbolName = n.QualifiedName[lastDot+1:]
			}
			if strings.EqualFold(symbolName, kw) && !tieredSeen[n.NodeHash] {
				tieredSeen[n.NodeHash] = true
				tieredResults = append(tieredResults, n)
			}
		}
	}

	// Tier 2: prefix matches (symbol name starts with keyword).
	if len(tieredResults) < 15 {
		for _, kw := range keywords {
			nodes, err := e.store.NodesByName(ctx, "%"+kw)
			if err != nil {
				return nil, err
			}
			for _, n := range nodes {
				if tieredSeen[n.NodeHash] {
					continue
				}
				lastDot := strings.LastIndex(n.QualifiedName, ".")
				symbolName := n.QualifiedName
				if lastDot >= 0 {
					symbolName = n.QualifiedName[lastDot+1:]
				}
				if strings.HasPrefix(strings.ToLower(symbolName), strings.ToLower(kw)) {
					tieredSeen[n.NodeHash] = true
					tieredResults = append(tieredResults, n)
				}
				if len(tieredResults) >= 30 {
					break
				}
			}
			if len(tieredResults) >= 30 {
				break
			}
		}
	}

	// Tier 3: substring fallback (only if we still have very few candidates).
	if len(tieredResults) < 5 {
		for _, kw := range keywords {
			if len(kw) < 4 {
				continue
			}
			nodes, err := e.store.NodesByName(ctx, "%"+kw)
			if err != nil {
				return nil, err
			}
			for _, n := range nodes {
				if !tieredSeen[n.NodeHash] {
					tieredSeen[n.NodeHash] = true
					tieredResults = append(tieredResults, n)
				}
				if len(tieredResults) >= 20 {
					break
				}
			}
			if len(tieredResults) >= 20 {
				break
			}
		}
	}

	// Tier 4: file-path keyword matching.
	if len(tieredResults) < 30 {
		for _, kw := range keywords {
			if len(kw) < 3 {
				continue
			}
			nodes, err := e.store.NodesByName(ctx, "%"+kw+"%")
			if err != nil {
				continue
			}
			for _, n := range nodes {
				if tieredSeen[n.NodeHash] {
					continue
				}
				qLower := strings.ToLower(n.QualifiedName)
				kwLower := strings.ToLower(kw)
				if strings.Contains(qLower, "/"+kwLower) || strings.Contains(qLower, kwLower+"/") {
					tieredSeen[n.NodeHash] = true
					tieredResults = append(tieredResults, n)
					if len(tieredResults) >= 40 {
						break
					}
				}
			}
			if len(tieredResults) >= 40 {
				break
			}
		}
	}

	// Channel 2: BM25 full-text search (always runs when available).
	var bm25Results []types.Node
	if e.bm25 != nil {
		ftsQuery := strings.Join(keywords, " OR ")
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

	// Fuse all channels with weighted Reciprocal Rank Fusion.
	// Weights: tiered 3x (highest precision), vector 2x (concept-level), BM25 1x (broadest).
	// k=60 is the standard RRF constant.
	candidates := rrfFuseMulti([]rankedChannel{
		{nodes: tieredResults, weight: 3.0},
		{nodes: bm25Results, weight: 1.0},
		{nodes: vectorResults, weight: 2.0},
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

	rwrScores, err := RandomWalkWithRestart(ctx, e.store, seedHashes, 0.2, 20)
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
		})
	}

	// Apply feedback boosts if feedback provider is available.
	if e.feedback != nil && len(inputs) > 0 {
		hashes := make([]types.Hash, len(inputs))
		for i, inp := range inputs {
			hashes[i] = inp.Node.NodeHash
		}
		if boosts, err := e.feedback.FeedbackBoosts(ctx, hashes); err == nil && len(boosts) > 0 {
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

	return block, nil
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

// extractKeywords splits a task description into deduplicated, lowercase keywords
// with stop words removed.
// extractKeywords processes a task description into searchable terms.
// It splits CamelCase/snake_case identifiers, expands abbreviations,
// filters stop words, and returns terms ordered by specificity (longer first).
func extractKeywords(desc string) []string {
	words := strings.Fields(desc)
	seen := make(map[string]bool)
	var result []string
	var priorityTerms []string // nouns following action verbs get boosted

	// Verb-pattern detection: "Add a new X", "Implement X", "Build X"
	// The object noun after a filtered verb is the most important keyword.
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

	// Find the first non-filler noun after the opening verb.
	if len(words) > 0 {
		firstWord := strings.ToLower(strings.Trim(words[0], ".,;:!?\"'`()[]{}#"))
		if actionVerbs[firstWord] {
			for _, w := range words[1:] {
				clean := strings.ToLower(strings.Trim(w, ".,;:!?\"'`()[]{}#"))
				if clean == "" || fillerWords[clean] {
					continue
				}
				// This is the primary object noun. Boost it.
				priorityTerms = append(priorityTerms, clean)
				// Also try CamelCase version as a symbol name.
				if len(clean) > 2 {
					capitalized := strings.ToUpper(clean[:1]) + clean[1:]
					priorityTerms = append(priorityTerms, capitalized)
				}
				break
			}
		}
	}

	for _, w := range words {
		// Strip punctuation from edges.
		w = strings.Trim(w, ".,;:!?\"'`()[]{}#")
		if w == "" {
			continue
		}

		// Split CamelCase and snake_case into components.
		parts := splitIdentifier(w)
		for _, p := range parts {
			lower := strings.ToLower(p)
			if len(lower) < 2 {
				continue
			}
			if stopWords[lower] {
				continue
			}
			if seen[lower] {
				continue
			}
			seen[lower] = true
			result = append(result, lower)

			// Also add abbreviation expansion if available.
			if expanded, ok := abbreviations[lower]; ok && !seen[expanded] {
				seen[expanded] = true
				result = append(result, expanded)
			}
		}

		// Keep the original compound term too (if multi-part) for exact matching.
		lower := strings.ToLower(w)
		if strings.Contains(lower, "_") || len(parts) > 1 {
			if !seen[lower] && !stopWords[lower] {
				seen[lower] = true
				result = append(result, lower)
			}
		}
	}

	// Bigram compound detection: adjacent non-stop-words are joined into
	// compound terms (CamelCase and snake_case variants). This catches
	// multi-word concepts that map to single symbol names:
	//   "blast radius" -> "BlastRadius", "blast_radius"
	//   "transitive callers" -> "TransitiveCallers", "transitive_callers"
	//   "edge events" -> "EdgeEvents", "edge_events"
	var compounds []string
	cleanWords := make([]string, 0, len(words))
	for _, w := range words {
		clean := strings.ToLower(strings.Trim(w, ".,;:!?\"'`()[]{}#"))
		// Only include clean alpha words (no hyphens, no numbers) that are meaningful.
		if clean != "" && !stopWords[clean] && !fillerWords[clean] &&
			len(clean) >= 3 && !strings.ContainsAny(clean, "-/0123456789") {
			cleanWords = append(cleanWords, clean)
		}
	}
	for i := 0; i < len(cleanWords)-1; i++ {
		a, b := cleanWords[i], cleanWords[i+1]
		// Skip if either word is an action verb (these describe intent, not symbols).
		if actionVerbs[a] || actionVerbs[b] {
			continue
		}
		// Skip if both words are very short (too likely to be noise).
		if len(a) < 4 && len(b) < 4 {
			continue
		}
		// CamelCase compound: "BlastRadius"
		camel := strings.ToUpper(a[:1]) + a[1:] + strings.ToUpper(b[:1]) + b[1:]
		if !seen[camel] {
			seen[camel] = true
			compounds = append(compounds, camel)
		}
		// snake_case compound: "blast_radius"
		snake := a + "_" + b
		if !seen[snake] {
			seen[snake] = true
			compounds = append(compounds, snake)
		}
	}

	// Prepend priority terms (object nouns from verb patterns) so they seed first.
	var final []string
	for _, pt := range priorityTerms {
		if !seen[pt] {
			seen[pt] = true
			final = append(final, pt)
		}
	}
	// Compounds next (most specific, multi-word matches).
	final = append(final, compounds...)
	// Then individual keywords.
	final = append(final, result...)

	// Sort by length descending: longer (more specific) terms first.
	// But keep priority terms at the front.
	priorityLen := len(priorityTerms)
	if len(final) > priorityLen {
		tail := final[priorityLen:]
		sort.Slice(tail, func(i, j int) bool {
			return len(tail[i]) > len(tail[j])
		})
	}

	return final
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
		cost := EstimateNodeTokens(sym.Node)
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
// vendor/, and node_modules/ paths that pollute retrieval results.
func filterNoisySymbols(nodes []types.Node) []types.Node {
	var filtered []types.Node
	for _, n := range nodes {
		qn := strings.ToLower(n.QualifiedName)
		// Skip minified JS bundles and build artifacts.
		if strings.Contains(qn, "/dist/") || strings.Contains(qn, "/build/") ||
			strings.Contains(qn, "/vendor/") || strings.Contains(qn, "/node_modules/") ||
			strings.Contains(qn, ".min.") || strings.Contains(qn, ".bundle.") {
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
