package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// community represents a detected graph community with metadata.
type community struct {
	ID              int      `json:"id"`
	Size            int      `json:"size"`
	TopSymbols      []string `json:"top_symbols"`
	Cohesion        float64  `json:"cohesion"`
	DominantPackage string   `json:"dominant_package"`
}

// communityResult is the response for action="list".
type communityResult struct {
	Communities []community `json:"communities"`
	NodeCount   int         `json:"node_count"`
	EdgeCount   int         `json:"edge_count"`
}

// symbolCommunityResult is the response for action="for_symbol".
type symbolCommunityResult struct {
	Symbol    string      `json:"symbol"`
	Community community   `json:"community"`
	Neighbors []community `json:"neighbors"`
}

// weightedEdge represents an edge with a weight for the Louvain algorithm.
type weightedEdge struct {
	Target types.Hash
	Weight float64
}

// communitiesTool defines the "communities" MCP tool.
func communitiesTool() mcp.Tool {
	return mcp.NewTool("communities",
		mcp.WithDescription("Detect communities in the knowledge graph using Louvain modularity clustering. Returns densely-connected groups of symbols."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("action", mcp.Description("Action to perform: 'list' returns all communities, 'for_symbol' returns the community containing a specific symbol (default: list)"), Examples("list", "for_symbol")),
		mcp.WithString("repo_url", mcp.Description("Filter nodes to a specific repository URL prefix"), Examples("https://github.com/org/repo")),
		mcp.WithString("symbol", mcp.Description("Qualified symbol name, required when action=for_symbol"), Examples("github.com/org/repo://pkg.FunctionName")),
	)
}

// handleCommunities implements the communities tool handler.
func (s *Server) handleCommunities(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := getStringArg(req, "action")
	if action == "" {
		action = "list"
	}

	repoURL := getStringArg(req, "repo_url")
	symbol := getStringArg(req, "symbol")

	if action == "for_symbol" && symbol == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewTextContent("error: symbol is required when action=for_symbol")},
			IsError: true,
		}, nil
	}

	// Load nodes.
	nodes, err := s.loadCommunityNodes(ctx, repoURL)
	if err != nil {
		return nil, fmt.Errorf("loading nodes: %w", err)
	}
	if len(nodes) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewTextContent("{\"communities\":[], \"node_count\":0, \"edge_count\":0}")},
		}, nil
	}

	// Build adjacency list.
	nodeSet := make(map[types.Hash]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n.NodeHash] = true
	}

	adj, edgeCount := s.buildAdjacencyList(ctx, nodes, nodeSet)

	// Run Louvain clustering.
	nodeHashes := make([]types.Hash, len(nodes))
	for i, n := range nodes {
		nodeHashes[i] = n.NodeHash
	}
	membership := louvain(nodeHashes, adj)

	// Build community metadata.
	communities := buildCommunities(nodes, membership, adj, nodeSet)

	switch action {
	case "for_symbol":
		return s.handleCommunitiesForSymbol(ctx, symbol, nodes, membership, communities, adj, nodeSet)
	default:
		return handleCommunitiesList(communities, len(nodes), edgeCount)
	}
}

// loadCommunityNodes loads nodes, optionally filtered by repo_url prefix.
func (s *Server) loadCommunityNodes(ctx context.Context, repoURL string) ([]types.Node, error) {
	if repoURL != "" {
		return s.store.NodesByName(ctx, repoURL)
	}
	// Load all nodes by querying with empty prefix.
	return s.store.NodesByName(ctx, "")
}

// buildAdjacencyList builds an undirected weighted adjacency list from edges
// between nodes in the set. Returns the adjacency map and total edge count.
func (s *Server) buildAdjacencyList(ctx context.Context, nodes []types.Node, nodeSet map[types.Hash]bool) (map[types.Hash][]weightedEdge, int) {
	adj := make(map[types.Hash][]weightedEdge, len(nodes))
	edgeCount := 0

	// For large graphs, sample to avoid loading too many edges.
	maxNodes := len(nodes)
	if maxNodes > 5000 {
		maxNodes = 5000
	}

	for i := 0; i < maxNodes; i++ {
		n := nodes[i]
		edges, err := s.store.EdgesFrom(ctx, n.NodeHash, "")
		if err != nil {
			continue
		}
		for _, e := range edges {
			if !nodeSet[e.TargetHash] {
				continue
			}
			adj[n.NodeHash] = append(adj[n.NodeHash], weightedEdge{
				Target: e.TargetHash,
				Weight: e.Confidence,
			})
			// Add reverse edge for undirected graph.
			adj[e.TargetHash] = append(adj[e.TargetHash], weightedEdge{
				Target: n.NodeHash,
				Weight: e.Confidence,
			})
			edgeCount++
		}
	}
	return adj, edgeCount
}

// louvain partitions nodes into communities maximizing modularity.
// This is a simplified single-pass (Phase 1 only) implementation.
// Uses the standard modularity gain formula:
//
//	deltaQ = [k_i_in / m - (sigma_tot * k_i) / (2 * m^2)]
//
// where k_i_in = sum of weights from node i to community C,
// sigma_tot = sum of all weights of edges incident to nodes in C,
// k_i = weighted degree of node i, m = sum of all edge weights / 2.
func louvain(nodes []types.Hash, adj map[types.Hash][]weightedEdge) map[types.Hash]int {
	// Initialize: each node in its own community.
	comm := make(map[types.Hash]int, len(nodes))
	for i, n := range nodes {
		comm[n] = i
	}

	// Compute total edge weight. Since adj is undirected (each edge stored twice),
	// summing all gives 2m.
	var twoM float64
	for _, edges := range adj {
		for _, e := range edges {
			twoM += e.Weight
		}
	}
	if twoM == 0 {
		return comm
	}
	m := twoM / 2.0

	// Compute node strengths (weighted degree = sum of edge weights).
	ki := make(map[types.Hash]float64, len(nodes))
	for _, n := range nodes {
		for _, e := range adj[n] {
			ki[n] += e.Weight
		}
	}

	// Maintain sigma_tot per community (sum of ki for all nodes in community).
	sigmaTot := make(map[int]float64, len(nodes))
	for _, n := range nodes {
		sigmaTot[comm[n]] += ki[n]
	}

	// Iterate until no improvement.
	improved := true
	for pass := 0; improved && pass < 20; pass++ {
		improved = false
		for _, node := range nodes {
			currentComm := comm[node]
			bestComm := currentComm
			bestGain := 0.0

			// Compute weight from node to each neighboring community.
			kiIn := make(map[int]float64)
			for _, e := range adj[node] {
				kiIn[comm[e.Target]] += e.Weight
			}

			// Remove node from its current community for gain calculation.
			// sigma_tot of current community without this node:
			sigCurr := sigmaTot[currentComm] - ki[node]
			kiInCurr := kiIn[currentComm]

			for c, w := range kiIn {
				if c == currentComm {
					continue
				}
				sigC := sigmaTot[c]

				// Gain = moving from current to c.
				// gain_remove = ki_in_curr/m - (sigCurr * ki[node]) / (2*m*m)
				// gain_add = w/m - (sigC * ki[node]) / (2*m*m)
				// net gain = gain_add - gain_remove... but standard Louvain uses:
				// deltaQ = [w - sigC*ki[node]/(2m)] / m
				//        - [kiInCurr - sigCurr*ki[node]/(2m)] / m ... (for removal)
				// Simplified: compare gain of adding to c vs staying.
				gainAdd := w/m - (sigC*ki[node])/(2*m*m)
				gainRemove := kiInCurr/m - (sigCurr*ki[node])/(2*m*m)
				gain := gainAdd - gainRemove

				if gain > bestGain {
					bestGain = gain
					bestComm = c
				}
			}

			if bestComm != currentComm {
				// Move node.
				sigmaTot[currentComm] -= ki[node]
				sigmaTot[bestComm] += ki[node]
				comm[node] = bestComm
				improved = true
			}
		}
	}

	// Renumber communities to be contiguous starting from 0.
	remap := make(map[int]int)
	nextID := 0
	for _, n := range nodes {
		c := comm[n]
		if _, ok := remap[c]; !ok {
			remap[c] = nextID
			nextID++
		}
		comm[n] = remap[c]
	}

	return comm
}

// buildCommunities computes metadata for each community.
func buildCommunities(nodes []types.Node, membership map[types.Hash]int, adj map[types.Hash][]weightedEdge, nodeSet map[types.Hash]bool) []community {
	// Group nodes by community.
	commNodes := make(map[int][]types.Node)
	for _, n := range nodes {
		cid := membership[n.NodeHash]
		commNodes[cid] = append(commNodes[cid], n)
	}

	communities := make([]community, 0, len(commNodes))
	for cid, cnodes := range commNodes {
		c := community{
			ID:   cid,
			Size: len(cnodes),
		}

		// Compute degree within community for top symbols.
		type nodeDegree struct {
			name   string
			degree int
		}
		degrees := make([]nodeDegree, 0, len(cnodes))
		commSet := make(map[types.Hash]bool, len(cnodes))
		for _, n := range cnodes {
			commSet[n.NodeHash] = true
		}

		var internalEdges, totalEdges int
		for _, n := range cnodes {
			deg := 0
			for _, e := range adj[n.NodeHash] {
				if nodeSet[e.Target] {
					totalEdges++
					if commSet[e.Target] {
						internalEdges++
						deg++
					}
				}
			}
			degrees = append(degrees, nodeDegree{name: n.QualifiedName, degree: deg})
		}

		// Top symbols by degree.
		sort.Slice(degrees, func(i, j int) bool {
			return degrees[i].degree > degrees[j].degree
		})
		topN := 5
		if len(degrees) < topN {
			topN = len(degrees)
		}
		c.TopSymbols = make([]string, topN)
		for i := 0; i < topN; i++ {
			c.TopSymbols[i] = degrees[i].name
		}

		// Cohesion.
		if totalEdges > 0 {
			c.Cohesion = math.Round(float64(internalEdges)/float64(totalEdges)*100) / 100
		}

		// Dominant package.
		c.DominantPackage = dominantPackage(cnodes)

		communities = append(communities, c)
	}

	// Sort by size descending.
	sort.Slice(communities, func(i, j int) bool {
		return communities[i].Size > communities[j].Size
	})

	// Cap at 50 communities, merge smallest into "other".
	if len(communities) > 50 {
		other := community{
			ID:         -1,
			Size:       0,
			TopSymbols: []string{},
			Cohesion:   0,
		}
		for _, c := range communities[49:] {
			other.Size += c.Size
		}
		other.DominantPackage = "other"
		communities = append(communities[:49], other)
	}

	// Re-assign IDs after sorting.
	for i := range communities {
		communities[i].ID = i
	}

	return communities
}

// dominantPackage finds the most common package prefix among nodes.
func dominantPackage(nodes []types.Node) string {
	pkgCount := make(map[string]int)
	for _, n := range nodes {
		pkg := extractPackage(n.QualifiedName)
		if pkg != "" {
			pkgCount[pkg]++
		}
	}
	var bestPkg string
	var bestCount int
	for pkg, count := range pkgCount {
		if count > bestCount {
			bestCount = count
			bestPkg = pkg
		}
	}
	return bestPkg
}

// handleCommunitiesList handles action="list".
func handleCommunitiesList(communities []community, nodeCount, edgeCount int) (*mcp.CallToolResult, error) {
	result := communityResult{
		Communities: communities,
		NodeCount:   nodeCount,
		EdgeCount:   edgeCount,
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(string(data))},
	}, nil
}

// handleCommunitiesForSymbol handles action="for_symbol".
func (s *Server) handleCommunitiesForSymbol(ctx context.Context, symbol string, nodes []types.Node, membership map[types.Hash]int, communities []community, adj map[types.Hash][]weightedEdge, nodeSet map[types.Hash]bool) (*mcp.CallToolResult, error) {
	// Resolve symbol to node.
	resolved, err := s.store.NodesByQualifiedName(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("resolving symbol: %w", err)
	}
	if len(resolved) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewTextContent(fmt.Sprintf("error: symbol %q not found", symbol))},
			IsError: true,
		}, nil
	}

	targetNode := resolved[0]
	targetMembership, ok := membership[targetNode.NodeHash]
	if !ok {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.NewTextContent(fmt.Sprintf("error: symbol %q not in community graph", symbol))},
			IsError: true,
		}, nil
	}

	// Build mapping from membership ID to community index.
	// Group nodes by their membership value.
	commGroups := make(map[int][]types.Hash)
	for _, n := range nodes {
		commGroups[membership[n.NodeHash]] = append(commGroups[membership[n.NodeHash]], n.NodeHash)
	}

	// Map membership IDs to community objects by matching group members.
	membershipToComm := make(map[int]int) // membership ID -> community.ID
	for memID, members := range commGroups {
		memberSet := make(map[types.Hash]bool, len(members))
		for _, h := range members {
			memberSet[h] = true
		}
		for _, c := range communities {
			if c.Size == len(members) && len(c.TopSymbols) > 0 {
				// Check if a top symbol's node is in this group.
				for _, n := range nodes {
					if memberSet[n.NodeHash] && contains(c.TopSymbols, n.QualifiedName) {
						membershipToComm[memID] = c.ID
						break
					}
				}
				if _, found := membershipToComm[memID]; found {
					break
				}
			}
		}
	}

	// Find the community for our target symbol.
	targetCommID := membershipToComm[targetMembership]
	var symbolComm community
	for _, c := range communities {
		if c.ID == targetCommID {
			symbolComm = c
			break
		}
	}

	// Find neighbor communities (connected via cross-community edges).
	neighborCommIDs := make(map[int]bool)
	targetMembers := commGroups[targetMembership]
	for _, h := range targetMembers {
		for _, e := range adj[h] {
			if !nodeSet[e.Target] {
				continue
			}
			neighborMem := membership[e.Target]
			if neighborMem != targetMembership {
				if cID, ok := membershipToComm[neighborMem]; ok {
					neighborCommIDs[cID] = true
				}
			}
		}
	}

	var neighbors []community
	for _, c := range communities {
		if neighborCommIDs[c.ID] && c.ID != symbolComm.ID {
			neighbors = append(neighbors, c)
		}
	}

	result := symbolCommunityResult{
		Symbol:    symbol,
		Community: symbolComm,
		Neighbors: neighbors,
	}
	if result.Neighbors == nil {
		result.Neighbors = []community{}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshaling result: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(string(data))},
	}, nil
}

// contains checks if a string slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
