package context

// Exported wrappers for debugging tools (cmd/knowing/debug_seeds.go).
// These functions are internal but need to be callable from the CLI.

// ExtractKeywordSetExported wraps extractKeywordSet for external use.
func ExtractKeywordSetExported(desc string) KeywordSet {
	return extractKeywordSet(desc)
}

// ExtractPathTermsExported wraps extractPathTerms for external use.
func ExtractPathTermsExported(desc string) []string {
	return extractPathTerms(desc)
}

// BuildFTSQueryExported wraps buildFTSQuery for external use.
func BuildFTSQueryExported(keywords []string) string {
	return buildFTSQuery(keywords)
}

// DecomposeCompoundsExported wraps decomposeCompounds for external use.
func DecomposeCompoundsExported(keywords []string) string {
	return decomposeCompounds(keywords)
}

// DecomposeCompoundsTargetedExported wraps decomposeCompoundsTargeted for external use.
func DecomposeCompoundsTargetedExported(keywords []string) string {
	return decomposeCompoundsTargeted(keywords)
}
