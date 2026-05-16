package context

import (
	stdctx "context"
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
// that fit within a token budget.
type ContextBlock struct {
	Symbols     []RankedSymbol
	Format      string
	TokensUsed  int
	TokenBudget int
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
var stopWords = map[string]bool{
	"the": true, "a": true, "in": true, "to": true, "for": true,
	"is": true, "of": true, "and": true, "or": true,
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

	// Find candidate nodes by keyword prefix search.
	seen := make(map[types.Hash]bool)
	var candidates []types.Node
	for _, kw := range keywords {
		nodes, err := e.store.NodesByName(ctx, kw)
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

	// Build scoring inputs for direct matches and their neighbors.
	var inputs []ScoringInput
	inputSeen := make(map[types.Hash]bool)

	for _, node := range candidates {
		callers, err := e.store.EdgesTo(ctx, node.NodeHash, "calls")
		if err != nil {
			return nil, err
		}
		callees, err := e.store.EdgesFrom(ctx, node.NodeHash, "calls")
		if err != nil {
			return nil, err
		}

		// Determine confidence and last observed from edges.
		confidence := 0.5
		var lastObserved int64
		allEdges := append(callers, callees...)
		for _, edge := range allEdges {
			if edge.Confidence > confidence {
				confidence = edge.Confidence
			}
			if edge.LastObserved > lastObserved {
				lastObserved = edge.LastObserved
			}
		}

		if !inputSeen[node.NodeHash] {
			inputSeen[node.NodeHash] = true
			inputs = append(inputs, ScoringInput{
				Node:               node,
				CallerCount:        len(callers),
				Confidence:         confidence,
				LastObserved:       lastObserved,
				DistanceFromTarget: 0,
			})
		}

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

		// Add callees as distance-1 candidates.
		for _, edge := range callees {
			if inputSeen[edge.TargetHash] {
				continue
			}
			calleeNode, err := e.store.GetNode(ctx, edge.TargetHash)
			if err != nil {
				return nil, err
			}
			if calleeNode == nil {
				continue
			}
			inputSeen[edge.TargetHash] = true
			inputs = append(inputs, ScoringInput{
				Node:               *calleeNode,
				CallerCount:        0,
				Confidence:         edge.Confidence,
				LastObserved:       edge.LastObserved,
				DistanceFromTarget: 1,
			})
		}
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
func extractKeywords(desc string) []string {
	words := strings.Fields(desc)
	seen := make(map[string]bool)
	var result []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if stopWords[lower] {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		result = append(result, lower)
	}
	return result
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
