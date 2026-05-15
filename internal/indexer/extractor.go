// Package indexer orchestrates source code extraction and graph indexing.
package indexer

import "github.com/blackwell-systems/knowing/internal/types"

// ExtractorRegistry maintains a list of registered extractors and finds
// the appropriate extractor for a given file path.
type ExtractorRegistry struct {
	extractors []types.Extractor
}

// NewExtractorRegistry creates an empty ExtractorRegistry.
func NewExtractorRegistry() *ExtractorRegistry {
	return &ExtractorRegistry{}
}

// Register adds an extractor to the registry.
func (r *ExtractorRegistry) Register(ext types.Extractor) {
	r.extractors = append(r.extractors, ext)
}

// FindExtractor returns the first registered extractor that can handle
// the given file path, or nil if none matches.
func (r *ExtractorRegistry) FindExtractor(path string) types.Extractor {
	for _, ext := range r.extractors {
		if ext.CanHandle(path) {
			return ext
		}
	}
	return nil
}
