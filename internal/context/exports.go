package context

import (
	stdctx "context"

	"github.com/blackwell-systems/knowing/internal/types"
)

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

// SeedEquivalenceClassesExported wraps seedEquivalenceClasses for external use.
func SeedEquivalenceClassesExported() []EquivalenceClass {
	return seedEquivalenceClasses()
}

// UniversalEquivalenceClassesExported wraps universalEquivalenceClasses for external use.
func UniversalEquivalenceClassesExported() []EquivalenceClass {
	return universalEquivalenceClasses()
}

// LanguageEquivalenceClassesExported wraps languageEquivalenceClasses for external use.
func LanguageEquivalenceClassesExported() []EquivalenceClass {
	return languageEquivalenceClasses()
}

// MatchEquivalenceClassesLangExported wraps matchEquivalenceClassesLang for external use.
// Returns exported match structs.
func MatchEquivalenceClassesLangExported(task string, classes []EquivalenceClass, lang string) []EquivMatchExported {
	matches := matchEquivalenceClassesLang(task, classes, lang)
	result := make([]EquivMatchExported, len(matches))
	for i, m := range matches {
		result[i] = EquivMatchExported{Class: m.class, Targets: m.targets}
	}
	return result
}

// EquivMatchExported is an exported version of equivalenceMatch for debug tools.
type EquivMatchExported struct {
	Class   EquivalenceClass
	Targets []string
}

// DetectRepoLanguageExported wraps detectRepoLanguage for external use.
func DetectRepoLanguageExported(ctx stdctx.Context, store types.GraphStore) string {
	return detectRepoLanguage(ctx, store)
}

// PackIntoBudget is the exported entry point for the packing algorithm.
// Used by the context packing benchmark to compare strategies.
func PackIntoBudget(ranked []RankedSymbol, budget int, format string) *ContextBlock {
	return packIntoBudget(ranked, budget, format)
}
