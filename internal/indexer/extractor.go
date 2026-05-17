// Package indexer orchestrates source code extraction and graph indexing.
//
// The indexer walks repository files, dispatches them to the appropriate
// language-specific Extractor, stores the resulting nodes and edges, computes
// Merkle snapshots, and runs cross-repo edge resolution. It supports
// incremental re-indexing by skipping files whose content hash has not changed.
//
// Key components:
//   - Indexer: top-level orchestrator (indexer.go)
//   - ExtractorRegistry: dispatches files to the first matching extractor (this file)
//   - parallelExtract: worker pool for concurrent file extraction (worker.go)
package indexer

import "github.com/blackwell-systems/knowing/internal/types"

// ExtractorRegistry maintains an ordered list of registered extractors and
// dispatches files to the first extractor whose CanHandle returns true.
// Extractors are checked in registration order, so more specific extractors
// should be registered before generic ones.
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

// FindAllExtractors returns all registered extractors that can handle
// the given file path. This enables multi-extractor dispatch: a .go file
// can be processed by both the Go extractor (functions, types) and the
// event extractor (Kafka/NATS patterns).
func (r *ExtractorRegistry) FindAllExtractors(path string) []types.Extractor {
	var matches []types.Extractor
	for _, ext := range r.extractors {
		if ext.CanHandle(path) {
			matches = append(matches, ext)
		}
	}
	return matches
}
