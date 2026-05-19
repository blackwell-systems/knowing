package agent_efficiency

// Task is a benchmark task fixture with a ground truth answer.
type Task struct {
	ID          string
	Description string
	GroundTruth GroundTruth
	Complexity  string // "low", "medium", "high"
}

// GroundTruth describes what a correct agent response must contain.
type GroundTruth struct {
	// RelevantFiles are the files the agent should read or find to answer correctly.
	RelevantFiles []string
	// KeySymbols are the symbol names the agent should discover.
	KeySymbols []string
	// AnswerKeywords are substrings that must appear in the final response.
	AnswerKeywords []string
}

// Tasks is the canonical fixture set for the agent efficiency benchmark.
// Each task targets the knowing codebase and has been verified against the
// actual source.
var Tasks = []Task{
	{
		ID:          "blast-radius-handler",
		Description: "What function handles the blast_radius MCP tool in the knowing codebase? In which file is it defined?",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/mcp/handlers.go",
			},
			KeySymbols: []string{
				"handleBlastRadius",
			},
			AnswerKeywords: []string{
				"handleBlastRadius",
				"handlers.go",
			},
		},
		Complexity: "low",
	},
	{
		ID:          "context-engine-scoring",
		Description: "How does the context engine score symbols? What is the formula, and what weights are applied to each component?",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/context/ranking.go",
				"internal/context/hits.go",
			},
			KeySymbols: []string{
				"RankSymbols",
				"ScoringInput",
				"ScoreComponents",
				"HITSScores",
			},
			AnswerKeywords: []string{
				"RankSymbols",
				"weight",
				"feedback",
				"session",
				"recency",
			},
		},
		Complexity: "medium",
	},
	{
		ID:          "node-struct-blast-radius",
		Description: "If I change the Node struct in internal/types/types.go, what breaks? Which packages and callers depend on it?",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/types/types.go",
				"internal/store/knowing/",
				"internal/indexer/indexer.go",
				"internal/mcp/handlers.go",
			},
			KeySymbols: []string{
				"Node",
				"ComputeNodeHash",
				"PutNode",
			},
			AnswerKeywords: []string{
				"Node",
				"types.go",
				"store",
				"indexer",
			},
		},
		Complexity: "medium",
	},
	{
		ID:          "louvain-community-detection",
		Description: "How does the Louvain community detection work in the knowing codebase? Walk me through the algorithm implementation.",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/community/louvain.go",
				"internal/community/algorithm.go",
			},
			KeySymbols: []string{
				"Louvain",
				"Detect",
			},
			AnswerKeywords: []string{
				"modularity",
				"community",
				"louvain",
				"Detect",
			},
		},
		Complexity: "medium",
	},
	{
		ID:          "snapshot-package-coverage",
		Description: "What test files exist for the snapshot package, and what aspects of the package do they cover?",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/snapshot/manager_test.go",
				"internal/snapshot/hierarchical_test.go",
				"internal/snapshot/verify_test.go",
				"internal/snapshot/impact_test.go",
				"internal/snapshot/semantic_test.go",
			},
			KeySymbols: []string{
				"HierarchicalTree",
				"DiffHierarchicalTrees",
				"BuildHierarchicalTree",
			},
			AnswerKeywords: []string{
				"snapshot",
				"hierarchical",
				"test",
			},
		},
		Complexity: "low",
	},
	{
		ID:          "hierarchical-merkle-diff",
		Description: "How does the hierarchical Merkle tree improve diff performance in the knowing codebase? What is the algorithmic improvement over a flat diff?",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/snapshot/hierarchical.go",
				"internal/snapshot/merkle.go",
				"internal/cache/subgraph.go",
			},
			KeySymbols: []string{
				"HierarchicalTree",
				"BuildHierarchicalTree",
				"DiffHierarchicalTrees",
				"DiffHierarchicalTreesWithOptions",
				"SubgraphCache",
				"InvalidatePackages",
			},
			AnswerKeywords: []string{
				"O(packages)",
				"hierarchical",
				"PackageRoots",
				"EdgeTypeRoots",
				"diff",
			},
		},
		Complexity: "high",
	},
	{
		ID:          "edge-types",
		Description: "What edge types does the knowing graph support and where are they defined? List all supported edge type strings.",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/types/types.go",
			},
			KeySymbols: []string{
				"Edge",
				"EdgeType",
				"ComputeEdgeHash",
			},
			AnswerKeywords: []string{
				"calls",
				"imports",
				"implements",
				"references",
				"types.go",
			},
		},
		Complexity: "medium",
	},
	{
		ID:          "file-save-to-cache-invalidation",
		Description: "Trace the data flow in the knowing daemon from a git commit (file save) to cache invalidation. Which functions are involved and in what order?",
		GroundTruth: GroundTruth{
			RelevantFiles: []string{
				"internal/daemon/gitwatcher.go",
				"internal/daemon/daemon.go",
				"internal/snapshot/hierarchical.go",
				"internal/cache/subgraph.go",
			},
			KeySymbols: []string{
				"GitWatcher",
				"CommitEvent",
				"DiffHierarchicalTrees",
				"InvalidatePackages",
			},
			AnswerKeywords: []string{
				"GitWatcher",
				"CommitEvent",
				"reindex",
				"InvalidatePackages",
				"SubgraphCache",
			},
		},
		Complexity: "high",
	},
}
