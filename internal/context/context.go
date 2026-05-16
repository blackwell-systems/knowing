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

	// Find candidate nodes by keyword substring search.
	// NodesByName uses LIKE prefix%, so prepending % gives LIKE %keyword%.
	seen := make(map[types.Hash]bool)
	var candidates []types.Node
	for _, kw := range keywords {
		nodes, err := e.store.NodesByName(ctx, "%"+kw)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			if !seen[n.NodeHash] {
				seen[n.NodeHash] = true
				candidates = append(candidates, n)
			}
			if len(candidates) >= 100 {
				break
			}
		}
		if len(candidates) >= 100 {
			break
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

	// Rank and pack into budget.
	ranked := RankSymbols(inputs)
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
		file, err := e.store.FileByPath(ctx, repoHash, path)
		if err != nil {
			return nil, err
		}
		if file == nil {
			continue
		}

		// Find nodes in this file by searching all nodes and filtering by FileHash.
		allNodes, err := e.store.NodesByName(ctx, "")
		if err != nil {
			return nil, err
		}

		for _, node := range allNodes {
			if node.FileHash != file.FileHash {
				continue
			}
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

	ranked := RankSymbols(inputs)
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

// packIntoBudget iterates over ranked symbols and accumulates them until the
// token budget would be exceeded.
func packIntoBudget(ranked []RankedSymbol, budget int, format string) *ContextBlock {
	block := &ContextBlock{
		Format:      format,
		TokenBudget: budget,
	}

	var tokensUsed int
	for _, sym := range ranked {
		cost := EstimateNodeTokens(sym.Node)
		if tokensUsed+cost > budget {
			break
		}
		tokensUsed += cost
		block.Symbols = append(block.Symbols, sym)
	}
	block.TokensUsed = tokensUsed
	return block
}
