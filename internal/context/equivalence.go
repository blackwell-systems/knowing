// Package context provides equivalence class retrieval for bridging the
// vocabulary gap between natural-language task descriptions and code symbol names.
//
// An equivalence class maps a concept (like "TRANSITIVE_IMPACT") to multiple
// phrases that developers use to describe it ("blast radius", "impact analysis",
// "downstream callers") and the specific symbols/tools those phrases should
// resolve to ("TransitiveCallers", "BlastRadius", "blast_radius").
package context

import "strings"

// EquivalenceClass maps a concept to its natural-language phrases and code targets.
type EquivalenceClass struct {
	Concept    string   // canonical concept ID (e.g., "TRANSITIVE_IMPACT")
	Phrases    []string // natural-language phrases that refer to this concept
	Targets    []string // symbol/tool identifiers to boost when phrases match
	TargetType string   // "symbol", "mcp_tool", "edge_type", "workflow", "file"
	Weight     float64  // source strength (seed: 1.0, graph: 0.7, feedback: 0.5)
	Source     string   // "seed", "graph", "feedback", "generated"
}

// actionVerbs are common developer action words that combine with concept nouns
// to form searchable phrases. Used for cross-product phrase generation.
var actionVerbs = []string{
	"find", "get", "compute", "show", "list",
	"trace", "check", "run", "detect", "analyze",
}

// seedEquivalenceClasses returns the hand-curated seed dictionary of universal
// software engineering concepts. These bootstrap the system before graph-derived
// aliases and feedback take over.
func seedEquivalenceClasses() []EquivalenceClass {
	seeds := []EquivalenceClass{
		{
			Concept:    "TRANSITIVE_IMPACT",
			Phrases:    []string{"blast radius", "impact analysis", "downstream callers", "affected code", "ripple effect", "what breaks", "who calls"},
			Targets:    []string{"TransitiveCallers", "BlastRadius", "blastRadiusTool", "handleBlastRadius"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "SYMBOL_LOOKUP",
			Phrases:    []string{"find symbol", "symbol definition", "symbol references", "symbol usage", "where is", "who uses"},
			Targets:    []string{"GetNode", "NodesByName", "NodesByQualifiedName", "graphQueryTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "DATAFLOW_TRACE",
			Phrases:    []string{"trace flow", "call path", "flow between", "call chain", "data flow", "execution path"},
			Targets:    []string{"TransitiveCallees", "traceDataflowTool", "flow_between"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "TEST_SELECTION",
			Phrases:    []string{"affected tests", "test scope", "what tests to run", "which tests", "test impact", "tests for change"},
			Targets:    []string{"cmdTestScope", "findAffectedTests", "test_scope"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "SNAPSHOT_DIFF",
			Phrases:    []string{"what changed", "graph diff", "semantic diff", "structural diff", "changes between", "added removed"},
			Targets:    []string{"SnapshotDiff", "SemanticDiff", "PRImpact", "semanticDiffTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "RUNTIME_USAGE",
			Phrases:    []string{"production traffic", "observed calls", "dead routes", "runtime edges", "trace ingestion", "telemetry"},
			Targets:    []string{"Ingestor", "IngestSpans", "OTLPReceiver", "runtimeTrafficTool", "deadRoutesTool", "ConfidenceFromCount"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "INDEXING",
			Phrases:    []string{"reindex", "index repo", "refresh graph", "update graph", "parse codebase", "extract symbols"},
			Targets:    []string{"IndexRepo", "NewIndexer", "Register", "IncrementalReindex", "indexRepoTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "STALE_GRAPH",
			Phrases:    []string{"stale edges", "invalidated relationships", "outdated graph", "graph drift", "expired edges"},
			Targets:    []string{"StaleEdges", "DeleteNodesByFile", "DeleteEdgesBySourceFile", "staleEdgesTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "CONTEXT_PACKING",
			Phrases:    []string{"relevant context", "task context", "files context", "context for task", "ranked context", "token budget"},
			Targets:    []string{"ContextEngine", "ForTask", "ForFiles", "ForPR", "RankSymbols", "packIntoBudget", "contextForTaskTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "COMMUNITY_DETECTION",
			Phrases:    []string{"community detection", "graph clustering", "module boundaries", "subsystem grouping", "related symbols"},
			Targets:    []string{"louvain", "handleCommunities", "communitiesTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "CROSS_REPO",
			Phrases:    []string{"cross repo", "multi repo", "external dependencies", "dangling edges", "resolve imports"},
			Targets:    []string{"Resolver", "Resolve", "DanglingEdges", "crossRepoCallersTool"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "WIRE_FORMAT",
			Phrases:    []string{"wire format", "token savings", "compact encoding", "serialize context", "encode payload"},
			Targets:    []string{"Payload", "EncodeWith", "EncodeWithSession", "Session", "GCF"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "FEEDBACK_LOOP",
			Phrases:    []string{"feedback", "symbol usefulness", "was this helpful", "ranking improvement", "learning from usage"},
			Targets:    []string{"FeedbackBoosts", "RecordFeedback", "feedbackTool", "FeedbackProvider"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "LSP_ENRICHMENT",
			Phrases:    []string{"lsp enrichment", "type resolution", "gopls", "upgrade edges", "resolve types"},
			Targets:    []string{"Enricher", "Run", "RunScoped", "enrichEdge"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "DAEMON_LIFECYCLE",
			Phrases:    []string{"daemon", "background process", "file watcher", "git watcher", "auto reindex", "persistent service"},
			Targets:    []string{"Daemon", "NewDaemon", "GitWatcher", "traceIngestLoop"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "EMBEDDING_SEARCH",
			Phrases:    []string{"semantic search", "vector search", "embeddings", "similar symbols", "nearest neighbor"},
			Targets:    []string{"Embedder", "Searcher", "EmbedAndSearch", "IndexBatch"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "SESSION_TRACKING",
			Phrases:    []string{"session tracking", "session state", "session boost", "working memory", "recent context"},
			Targets:    []string{"SessionTracker", "SessionBoosts", "Record", "RecordBatch"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "GRAPH_WALK",
			Phrases:    []string{"random walk", "graph traversal", "rwr", "walk with restart", "explore graph", "graph expansion"},
			Targets:    []string{"RandomWalkWithRestart", "buildAdjacencyMap", "ComputeHITS"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "PR_REVIEW",
			Phrases:    []string{"pull request", "pr review", "pr impact", "changed files", "code review context"},
			Targets:    []string{"PRImpact", "ForPR", "prImpactTool", "SemanticDiff"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
		{
			Concept:    "SNAPSHOT_MANAGEMENT",
			Phrases:    []string{"snapshot", "graph version", "graph state", "merkle root", "commit snapshot"},
			Targets:    []string{"SnapshotManager", "ComputeSnapshot", "NewSnapshotManager", "LatestSnapshot"},
			TargetType: "symbol",
			Weight:     1.0,
			Source:     "seed",
		},
	}

	// Expand each seed with cross-product of action verbs x concept nouns.
	for i := range seeds {
		seeds[i].Phrases = expandWithVerbs(seeds[i].Phrases)
	}

	return seeds
}

// expandWithVerbs generates additional phrase variants by prepending action verbs
// to existing noun phrases. "blast radius" -> "find blast radius", "compute blast radius".
// Only generates for phrases that look like nouns (no existing verb prefix).
func expandWithVerbs(phrases []string) []string {
	expanded := make([]string, len(phrases))
	copy(expanded, phrases)

	for _, phrase := range phrases {
		// Skip phrases that already start with a verb.
		hasVerb := false
		for _, v := range actionVerbs {
			if len(phrase) > len(v) && phrase[:len(v)] == v {
				hasVerb = true
				break
			}
		}
		if hasVerb {
			continue
		}
		// Generate "verb + phrase" variants.
		for _, verb := range actionVerbs {
			expanded = append(expanded, verb+" "+phrase)
		}
	}

	return expanded
}

// matchEquivalenceClasses finds equivalence classes whose phrases appear in the
// query text. Returns matching classes with their targets for seed boosting.
func matchEquivalenceClasses(query string, classes []EquivalenceClass) []equivalenceMatch {
	queryLower := strings.ToLower(query)
	var matches []equivalenceMatch

	for _, cls := range classes {
		for _, phrase := range cls.Phrases {
			if strings.Contains(queryLower, strings.ToLower(phrase)) {
				matches = append(matches, equivalenceMatch{
					class:   cls,
					phrase:  phrase,
					targets: cls.Targets,
					weight:  cls.Weight,
				})
				break // one match per class is enough
			}
		}
	}

	return matches
}

type equivalenceMatch struct {
	class   EquivalenceClass
	phrase  string
	targets []string
	weight  float64
}
