package context

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ExplainResult is the full scoring breakdown for a symbol in the context of
// a task query. Every field that contributed to the final score is exposed.
type ExplainResult struct {
	Symbol        types.Node
	Rank          int     // 1-indexed position in the ranked results
	TotalScore    float64 // final score after all components
	TotalSymbols  int     // total symbols considered
	Components    ScoreComponents
	HITSAuthority float64 // raw HITS authority score (0 if HITS not run)
	HITSHub       float64 // raw HITS hub score
	HITSAdjust    float64 // net HITS adjustment applied to total
	RWRScore      float64 // raw Random Walk with Restart score
	IsSeed        bool    // was this a direct keyword match (distance=0)?
	SeedChannel   string  // which channel found this symbol ("tiered", "bm25", "equiv", "rwr")
	SeedTier      string  // for tiered matches: "exact", "prefix", "substring", "path"
	EquivMatches  []string // equivalence classes that matched (concept names)
	Keywords      []string // extracted keywords from the task description
	MaxCallers    int      // max caller count in the candidate set (normalization denominator)
	CallerProxy   int      // RWR-derived caller proxy for this symbol
}

// ExplainSymbol runs the full retrieval pipeline for a task and returns
// a detailed scoring breakdown for a specific symbol. If the symbol is not
// in the results, it still returns whatever information is available (e.g.,
// "not found in seed set, not reached by RWR").
func (e *ContextEngine) ExplainSymbol(ctx stdctx.Context, task string, symbolQuery string) (*ExplainResult, error) {
	keywords := extractKeywords(task)
	if len(keywords) == 0 {
		return nil, fmt.Errorf("no keywords extracted from task description")
	}

	// Run the same seed retrieval as ForTask.
	tieredResults, tieredSeen, tierMap := e.tieredSearch(ctx, keywords)
	bm25Results := e.bm25Search(ctx, keywords)
	vectorResults := e.vectorSearch(ctx, task)
	equivResults, equivMatchNames := e.equivSearch(ctx, task, tieredResults)

	// Fuse channels.
	candidates := rrfFuseMulti([]rankedChannel{
		{nodes: tieredResults, weight: 3.0},
		{nodes: equivResults, weight: 2.0},
		{nodes: bm25Results, weight: 1.0},
		{nodes: vectorResults, weight: 0.0},
	}, 60, 200) // larger limit to include more symbols for explain

	// Add interface seeds.
	seen := make(map[types.Hash]bool, len(candidates))
	for _, c := range candidates {
		seen[c.NodeHash] = true
	}
	for _, c := range candidates {
		if c.Kind == "interface" || c.Kind == "type" {
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
				candidates = append(candidates, *implNode)
			}
		}
	}

	candidates = filterNoisySymbols(candidates)

	// RWR.
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

	// Build scoring inputs.
	var inputs []ScoringInput
	rwrMap := make(map[types.Hash]float64)
	for nodeHash, rwrScore := range rwrScores {
		rwrMap[nodeHash] = rwrScore
		if rwrScore < 0.02 {
			continue
		}
		node, err := e.store.GetNode(ctx, nodeHash)
		if err != nil || node == nil {
			continue
		}
		distance := 1
		if seedSet[nodeHash] {
			distance = 0
		}
		edges, err := e.store.EdgesTo(ctx, nodeHash, "")
		if err != nil {
			continue
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

	// Apply feedback boosts.
	if e.feedback != nil && len(inputs) > 0 {
		hashes := make([]types.Hash, len(inputs))
		for i, inp := range inputs {
			hashes[i] = inp.Node.NodeHash
		}
		// TODO: Pass neighborhood roots for merkleized expiration once hierarchical tree is available.
		if boosts, err := e.feedback.FeedbackBoosts(ctx, hashes, nil); err == nil {
			for i := range inputs {
				if boost, ok := boosts[inputs[i].Node.NodeHash]; ok {
					inputs[i].FeedbackBoost = boost
				}
			}
		}
	}

	// Apply session boosts.
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

	// Apply task memory boosts.
	if e.memory != nil && len(inputs) > 0 {
		memoryBoosts, err := e.memory.Recall(ctx, keywords)
		if err == nil && len(memoryBoosts) > 0 {
			for i := range inputs {
				if boost, ok := memoryBoosts[inputs[i].Node.NodeHash]; ok {
					inputs[i].FeedbackBoost += boost * 0.3
				}
			}
		}
	}

	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].CallerCount > inputs[j].CallerCount
	})

	// HITS.
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

	// Rank.
	ranked := RankSymbols(inputs, hitsResult)

	// Find the target symbol.
	symbolQueryLower := strings.ToLower(symbolQuery)
	var target *RankedSymbol
	targetRank := -1
	for i, r := range ranked {
		qnLower := strings.ToLower(r.Node.QualifiedName)
		lastDot := strings.LastIndex(qnLower, ".")
		symName := qnLower
		if lastDot >= 0 {
			symName = qnLower[lastDot+1:]
		}
		if symName == symbolQueryLower || strings.Contains(qnLower, symbolQueryLower) {
			target = &ranked[i]
			targetRank = i + 1
			break
		}
	}

	if target == nil {
		// Symbol not in ranked results. Check if it exists at all.
		nodes, err := e.store.NodesByName(ctx, "%"+symbolQuery)
		if err != nil || len(nodes) == 0 {
			return nil, fmt.Errorf("symbol %q not found in graph", symbolQuery)
		}
		// It exists but wasn't reached by retrieval.
		node := nodes[0]
		result := &ExplainResult{
			Symbol:       node,
			Rank:         -1,
			TotalSymbols: len(ranked),
			Keywords:     keywords,
			SeedChannel:  "none",
			SeedTier:     "not matched",
		}
		if rwr, ok := rwrMap[node.NodeHash]; ok {
			result.RWRScore = rwr
			result.SeedChannel = "rwr (below threshold)"
		}
		return result, nil
	}

	// Determine seed channel and tier.
	seedChannel := "rwr"
	seedTier := ""
	if tieredSeen[target.Node.NodeHash] {
		seedChannel = "tiered"
		seedTier = tierMap[target.Node.NodeHash]
	}
	bm25Set := make(map[types.Hash]bool)
	for _, n := range bm25Results {
		bm25Set[n.NodeHash] = true
	}
	if bm25Set[target.Node.NodeHash] && seedChannel == "rwr" {
		seedChannel = "bm25"
	}
	equivSet := make(map[types.Hash]bool)
	for _, n := range equivResults {
		equivSet[n.NodeHash] = true
	}
	if equivSet[target.Node.NodeHash] && seedChannel == "rwr" {
		seedChannel = "equiv"
	}

	// Compute HITS adjustment for explain.
	var hitsAdj float64
	var hitsAuth, hitsHub float64
	if hitsResult != nil {
		h := hitsResult[target.Node.NodeHash]
		hitsAuth = h.Authority
		hitsHub = h.Hub
		isSeed := target.Distance == 0
		if isSeed && h.Authority > 0.05 {
			hitsAdj = h.Authority * 0.25
		} else if !isSeed && h.Authority > 0.2 {
			hitsAdj = -h.Authority * 0.15
		}
		if isSeed && h.Hub > 0.1 {
			hitsAdj += h.Hub * 0.10
		}
	}

	// Find max callers for normalization context.
	maxCallers := 1
	for _, inp := range inputs {
		if inp.CallerCount > maxCallers {
			maxCallers = inp.CallerCount
		}
	}

	// Find the input for caller proxy.
	callerProxy := 0
	for _, inp := range inputs {
		if inp.Node.NodeHash == target.Node.NodeHash {
			callerProxy = inp.CallerCount
			break
		}
	}

	// Collect equivalence class names that matched this symbol.
	var matchedEquivNames []string
	for _, name := range equivMatchNames {
		matchedEquivNames = append(matchedEquivNames, name)
	}

	return &ExplainResult{
		Symbol:        target.Node,
		Rank:          targetRank,
		TotalScore:    target.Score,
		TotalSymbols:  len(ranked),
		Components:    target.Components,
		HITSAuthority: hitsAuth,
		HITSHub:       hitsHub,
		HITSAdjust:    hitsAdj,
		RWRScore:      rwrMap[target.Node.NodeHash],
		IsSeed:        target.Distance == 0,
		SeedChannel:   seedChannel,
		SeedTier:      seedTier,
		EquivMatches:  matchedEquivNames,
		Keywords:      keywords,
		MaxCallers:    maxCallers,
		CallerProxy:   callerProxy,
	}, nil
}

// tieredSearch runs the 4-tier keyword matching and returns results plus
// a map of which tier each node was found in.
func (e *ContextEngine) tieredSearch(ctx stdctx.Context, keywords []string) ([]types.Node, map[types.Hash]bool, map[types.Hash]string) {
	var results []types.Node
	seen := make(map[types.Hash]bool)
	tierMap := make(map[types.Hash]string) // hash -> "exact", "prefix", "substring", "path"

	// Tier 1: exact matches.
	for _, kw := range keywords {
		nodes, err := e.store.NodesByName(ctx, "%"+kw)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			lastDot := strings.LastIndex(n.QualifiedName, ".")
			symbolName := n.QualifiedName
			if lastDot >= 0 {
				symbolName = n.QualifiedName[lastDot+1:]
			}
			if strings.EqualFold(symbolName, kw) && !seen[n.NodeHash] {
				seen[n.NodeHash] = true
				results = append(results, n)
				tierMap[n.NodeHash] = "exact"
			}
		}
	}

	// Tier 2: prefix matches.
	if len(results) < 15 {
		for _, kw := range keywords {
			nodes, err := e.store.NodesByName(ctx, "%"+kw)
			if err != nil {
				continue
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
					results = append(results, n)
					tierMap[n.NodeHash] = "prefix"
				}
				if len(results) >= 30 {
					break
				}
			}
			if len(results) >= 30 {
				break
			}
		}
	}

	// Tier 3: substring fallback.
	if len(results) < 5 {
		for _, kw := range keywords {
			if len(kw) < 4 {
				continue
			}
			nodes, err := e.store.NodesByName(ctx, "%"+kw)
			if err != nil {
				continue
			}
			for _, n := range nodes {
				if !seen[n.NodeHash] {
					seen[n.NodeHash] = true
					results = append(results, n)
					tierMap[n.NodeHash] = "substring"
				}
				if len(results) >= 20 {
					break
				}
			}
			if len(results) >= 20 {
				break
			}
		}
	}

	// Tier 4: file-path keyword matching.
	if len(results) < 30 {
		for _, kw := range keywords {
			if len(kw) < 3 {
				continue
			}
			nodes, err := e.store.NodesByName(ctx, "%"+kw+"%")
			if err != nil {
				continue
			}
			for _, n := range nodes {
				if seen[n.NodeHash] {
					continue
				}
				qLower := strings.ToLower(n.QualifiedName)
				kwLower := strings.ToLower(kw)
				if strings.Contains(qLower, "/"+kwLower) || strings.Contains(qLower, kwLower+"/") {
					seen[n.NodeHash] = true
					results = append(results, n)
					tierMap[n.NodeHash] = "path"
					if len(results) >= 40 {
						break
					}
				}
			}
			if len(results) >= 40 {
				break
			}
		}
	}

	return results, seen, tierMap
}

// bm25Search runs BM25 full-text search.
func (e *ContextEngine) bm25Search(ctx stdctx.Context, keywords []string) []types.Node {
	if e.bm25 == nil {
		return nil
	}
	ftsQuery := strings.Join(keywords, " OR ")
	nodes, err := e.bm25.SearchBM25Nodes(ctx, ftsQuery, 30)
	if err != nil {
		return nil
	}
	return nodes
}

// vectorSearch runs embedding-based search.
func (e *ContextEngine) vectorSearch(ctx stdctx.Context, task string) []types.Node {
	if e.vector == nil {
		return nil
	}
	hashes, err := e.vector.EmbedAndSearch(ctx, task, 30)
	if err != nil || len(hashes) == 0 {
		return nil
	}
	var results []types.Node
	for _, h := range hashes {
		if node, err := e.store.GetNode(ctx, h); err == nil && node != nil {
			results = append(results, *node)
		}
	}
	return results
}

// equivSearch runs equivalence class matching and returns matched nodes
// plus the names of matched concepts.
func (e *ContextEngine) equivSearch(ctx stdctx.Context, task string, tieredResults []types.Node) ([]types.Node, []string) {
	allClasses := append(seedEquivalenceClasses(), universalEquivalenceClasses()...)
	allClasses = append(allClasses, languageEquivalenceClasses()...)
	eqMatches := matchEquivalenceClasses(task, allClasses)

	var candidateHashes []types.Hash
	topN := 10
	if len(tieredResults) < topN {
		topN = len(tieredResults)
	}
	for i := 0; i < topN; i++ {
		candidateHashes = append(candidateHashes, tieredResults[i].NodeHash)
	}
	graphClasses := graphDerivedAliases(ctx, e.store, candidateHashes)
	graphMatches := matchEquivalenceClasses(task, graphClasses)
	eqMatches = append(eqMatches, graphMatches...)

	var results []types.Node
	equivSeen := make(map[types.Hash]bool)
	var matchNames []string

	for _, m := range eqMatches {
		matchNames = append(matchNames, m.class.Concept)
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
					results = append(results, n)
				}
			}
		}
	}

	return results, matchNames
}
