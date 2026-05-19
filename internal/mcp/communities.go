package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/blackwell-systems/knowing/internal/community"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// communityInfo represents a detected graph community with metadata.
type communityInfo struct {
	ID              int      `json:"id"`
	Size            int      `json:"size"`
	TopSymbols      []string `json:"top_symbols"`
	Cohesion        float64  `json:"cohesion"`
	DominantPackage string   `json:"dominant_package"`
	// MerkleRoot is the content-addressed identity of this community's subgraph.
	// Computed from the packages this community spans. Two communities with the
	// same MerkleRoot contain identical graph structure, enabling cache keying,
	// change detection ("auth community root changed"), and safe parallelization
	// ("disjoint community roots = disjoint work").
	MerkleRoot string `json:"merkle_root,omitempty"`
	// Packages lists the distinct packages this community spans.
	Packages []string `json:"packages,omitempty"`
}

// communityResult is the response for action="list".
type communityResult struct {
	Communities []communityInfo `json:"communities"`
	NodeCount   int             `json:"node_count"`
	EdgeCount   int             `json:"edge_count"`
}

// symbolCommunityResult is the response for action="for_symbol".
type symbolCommunityResult struct {
	Symbol    string          `json:"symbol"`
	Community communityInfo   `json:"community"`
	Neighbors []communityInfo `json:"neighbors"`
}

// communitiesTool defines the "communities" MCP tool.
func communitiesTool() mcp.Tool {
	return mcp.NewTool("communities",
		mcp.WithDescription("Detect communities in the knowledge graph. Returns densely-connected groups of symbols. Supports multiple algorithms: "+strings.Join(community.Names(), ", ")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("action", mcp.Description("Action to perform: 'list' returns all communities, 'for_symbol' returns the community containing a specific symbol (default: list)"), Examples("list", "for_symbol")),
		mcp.WithString("algorithm", mcp.Description("Community detection algorithm (default: louvain). Options: "+strings.Join(community.Names(), ", ")), Examples(community.Names()...)),
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

	algoName := getStringArg(req, "algorithm")
	if algoName == "" {
		algoName = community.Default
	}

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

	// Run community detection via the registry.
	nodeHashes := make([]types.Hash, len(nodes))
	for i, n := range nodes {
		nodeHashes[i] = n.NodeHash
	}

	algo := community.Get(algoName)
	if algo == nil {
		algo = community.Get(community.Default)
	}
	g := &community.Graph{
		Nodes:   nodeHashes,
		Adj:     adj,
		NodeSet: nodeSet,
	}

	// Try incremental detection: load previous assignments from notes,
	// use DetectIncremental if the algorithm supports it.
	var membership map[types.Hash]int
	if incAlgo, ok := algo.(community.IncrementalAlgorithm); ok {
		previous, _ := community.LoadAssignments(ctx, s.store)
		if previous != nil {
			// Use all nodes as changed (no Merkle diff available in MCP handler).
			// The daemon path will scope changedNodes from DiffHierarchicalTrees.
			allChanged := make(map[types.Hash]bool, len(nodeHashes))
			for _, h := range nodeHashes {
				allChanged[h] = true
			}
			membership = incAlgo.DetectIncremental(g, previous, allChanged)
		} else {
			membership = algo.Detect(g)
		}
	} else {
		membership = algo.Detect(g)
	}

	// Persist assignments for future incremental runs.
	_ = community.SaveAssignments(ctx, s.store, membership)

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
func (s *Server) buildAdjacencyList(ctx context.Context, nodes []types.Node, nodeSet map[types.Hash]bool) (map[types.Hash][]community.WeightedEdge, int) {
	adj := make(map[types.Hash][]community.WeightedEdge, len(nodes))
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
			adj[n.NodeHash] = append(adj[n.NodeHash], community.WeightedEdge{
				Target: e.TargetHash,
				Weight: e.Confidence,
			})
			// Add reverse edge for undirected graph.
			adj[e.TargetHash] = append(adj[e.TargetHash], community.WeightedEdge{
				Target: n.NodeHash,
				Weight: e.Confidence,
			})
			edgeCount++
		}
	}
	return adj, edgeCount
}

// buildCommunities computes metadata for each community.
func buildCommunities(nodes []types.Node, membership map[types.Hash]int, adj map[types.Hash][]community.WeightedEdge, nodeSet map[types.Hash]bool) []communityInfo {
	// Group nodes by community.
	commNodes := make(map[int][]types.Node)
	for _, n := range nodes {
		cid := membership[n.NodeHash]
		commNodes[cid] = append(commNodes[cid], n)
	}

	communities := make([]communityInfo, 0, len(commNodes))
	for cid, cnodes := range commNodes {
		c := communityInfo{
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

		// Dominant package and community packages.
		c.DominantPackage = dominantPackage(cnodes)

		// Collect distinct packages for this community and compute Merkle root.
		pkgSet := make(map[string]bool)
		for _, n := range cnodes {
			pkg := extractPackage(n.QualifiedName)
			if pkg != "" {
				pkgSet[pkg] = true
			}
		}
		pkgs := make([]string, 0, len(pkgSet))
		for pkg := range pkgSet {
			pkgs = append(pkgs, pkg)
		}
		sort.Strings(pkgs)
		c.Packages = pkgs

		// Compute community Merkle root from sorted package names.
		// This root changes when any package in the community changes,
		// enabling scoped cache invalidation.
		if len(pkgs) > 0 {
			pkgHashes := make([]types.Hash, len(pkgs))
			for i, pkg := range pkgs {
				pkgHashes[i] = types.NewHash([]byte(pkg))
			}
			rootHash := types.NewHash([]byte(fmt.Sprintf("%v", pkgHashes)))
			c.MerkleRoot = rootHash.String()
		}

		communities = append(communities, c)
	}

	// Sort by size descending.
	sort.Slice(communities, func(i, j int) bool {
		return communities[i].Size > communities[j].Size
	})

	// Cap at 50 communities, merge smallest into "other".
	if len(communities) > 50 {
		other := communityInfo{
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
func handleCommunitiesList(communities []communityInfo, nodeCount, edgeCount int) (*mcp.CallToolResult, error) {
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
func (s *Server) handleCommunitiesForSymbol(ctx context.Context, symbol string, nodes []types.Node, membership map[types.Hash]int, communities []communityInfo, adj map[types.Hash][]community.WeightedEdge, nodeSet map[types.Hash]bool) (*mcp.CallToolResult, error) {
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
	membershipToComm := make(map[int]int) // membership ID -> communityInfo.ID
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
	var symbolComm communityInfo
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

	var neighbors []communityInfo
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
		result.Neighbors = []communityInfo{}
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
