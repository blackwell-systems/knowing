package types

// Provenance tier constants.
const (
	ProvenanceASTInferred     = "ast_inferred"
	ProvenanceASTResolved     = "ast_resolved"
	ProvenanceLSPResolved     = "lsp_resolved"
	ProvenanceSCIPResolved    = "scip_resolved"
	ProvenanceRuntimeObserved = "runtime_observed"
)

// Confidence constants by provenance tier.
const (
	ConfidenceASTInferred  = 0.7
	ConfidenceASTResolved  = 0.85
	ConfidenceLSPResolved  = 0.9
	ConfidenceSCIPResolved = 1.0
)
