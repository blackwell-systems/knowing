package context

import (
	stdctx "context"
	"sort"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ContextEngine queries the knowing knowledge graph to produce task-specific,
// token-budgeted context blocks ranked by graph relationships and runtime traffic.
type ContextEngine struct {
	store types.GraphStore
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
}

// NewContextEngine creates a ContextEngine backed by the given GraphStore.
func NewContextEngine(store types.GraphStore) *ContextEngine {
	return &ContextEngine{store: store}
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

	// Find candidate nodes using tiered matching:
	// Tier 1: exact symbol name match (highest quality seeds)
	// Tier 2: prefix match on the last path component (symbol name starts with keyword)
	// Tier 3: substring match (fallback, capped at 20 per keyword)
	//
	// Fewer, better seeds produce sharper RWR distributions.
	seen := make(map[types.Hash]bool)
	var candidates []types.Node

	// Tier 1: exact matches (keyword matches the symbol name exactly).
	// These are the highest-quality seeds.
	for _, kw := range keywords {
		nodes, err := e.store.NodesByName(ctx, "%"+kw)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			// Check if the keyword matches the last component of the qualified name.
			lastDot := strings.LastIndex(n.QualifiedName, ".")
			symbolName := n.QualifiedName
			if lastDot >= 0 {
				symbolName = n.QualifiedName[lastDot+1:]
			}
			if strings.EqualFold(symbolName, kw) && !seen[n.NodeHash] {
				seen[n.NodeHash] = true
				candidates = append(candidates, n)
			}
		}
	}

	// Tier 2: prefix matches (symbol name starts with keyword).
	if len(candidates) < 15 {
		for _, kw := range keywords {
			nodes, err := e.store.NodesByName(ctx, "%"+kw)
			if err != nil {
				return nil, err
			}
			for _, n := range nodes {
				if seen[n.NodeHash] {
					continue
				}
				lastDot := strings.LastIndex(n.QualifiedName, ".")
				symbolName := n.QualifiedName
				if lastDot >= 0 {
					symbolName = n.QualifiedName[lastDot+1:]
				}
				if strings.HasPrefix(strings.ToLower(symbolName), strings.ToLower(kw)) {
					seen[n.NodeHash] = true
					candidates = append(candidates, n)
				}
				if len(candidates) >= 30 {
					break
				}
			}
			if len(candidates) >= 30 {
				break
			}
		}
	}

	// Tier 3: substring fallback (only if we still have very few candidates).
	if len(candidates) < 5 {
		for _, kw := range keywords {
			if len(kw) < 4 {
				continue // skip short keywords in substring mode (too broad)
			}
			nodes, err := e.store.NodesByName(ctx, "%"+kw)
			if err != nil {
				return nil, err
			}
			for _, n := range nodes {
				if !seen[n.NodeHash] {
					seen[n.NodeHash] = true
					candidates = append(candidates, n)
				}
				if len(candidates) >= 20 {
					break
				}
			}
			if len(candidates) >= 20 {
				break
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
	var inputs []ScoringInput
	for nodeHash, rwrScore := range rwrScores {
		if rwrScore < 0.05 {
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
	return packIntoBudget(ranked, budget, opts.Format), nil
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

	// Sort by length descending: longer (more specific) terms first.
	sort.Slice(result, func(i, j int) bool {
		return len(result[i]) > len(result[j])
	})

	return result
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
