package embedding

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// EmbeddingStore persists embedding vectors keyed by node hash and model name.
// Implemented by store.SQLiteStore.
type EmbeddingStore interface {
	BatchPutEmbeddings(ctx context.Context, model string, hashes []types.Hash, vectors [][]byte) error
	GetEmbeddings(ctx context.Context, model string, hashes []types.Hash) (map[types.Hash][]byte, error)
	GetAllEmbeddings(ctx context.Context, model string) (map[types.Hash][]byte, error)
}

// Searcher wraps an Embedder and provides the VectorSearcher interface
// expected by the context engine. It resolves HNSW string keys (hex-encoded
// node hashes) back to types.Hash values.
type Searcher struct {
	embedder    *Embedder
	store       EmbeddingStore // nil = no persistence (in-memory only)
	model       string         // model identifier for cache key
	vecCache    map[types.Hash][]float32 // lazy-loaded from store on first gap-fill
	vecCacheSet bool                     // true after first load attempt
}

// NewSearcher creates a Searcher from an initialized Embedder.
func NewSearcher(e *Embedder) *Searcher {
	return &Searcher{embedder: e, model: modelRepo}
}

// SetStore attaches a persistent embedding store for vector caching.
// When set, IndexBatch writes vectors to the store, and ReRankByHashes
// reads cached vectors instead of re-embedding candidates.
func (s *Searcher) SetStore(store EmbeddingStore) {
	s.store = store
}

// Model returns the model identifier used for cache keys.
func (s *Searcher) Model() string {
	return s.model
}

// EmbedAndSearch embeds the query text and returns the k nearest symbol hashes.
func (s *Searcher) EmbedAndSearch(ctx context.Context, query string, k int) ([]types.Hash, error) {
	if s.embedder.Count() == 0 {
		return nil, nil
	}

	vec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	ids := s.embedder.Search(vec, k)
	hashes := make([]types.Hash, 0, len(ids))
	for _, id := range ids {
		h, err := hashFromHex(id)
		if err != nil {
			continue
		}
		hashes = append(hashes, h)
	}
	return hashes, nil
}

// IndexNode embeds a node's text representation and adds it to the HNSW index.
// The text format is: "kind name signature filepath"
func (s *Searcher) IndexNode(ctx context.Context, node types.Node, filePath string) error {
	text := fmt.Sprintf("%s %s %s %s", node.Kind, node.QualifiedName, node.Signature, filePath)
	vec, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return err
	}
	s.embedder.AddVector(hex.EncodeToString(node.NodeHash[:]), vec)
	return nil
}

// IndexBatch embeds multiple nodes and adds them to the HNSW index.
// More efficient than individual IndexNode calls due to batched embedding.
// When a store is attached, vectors are also persisted to SQLite.
func (s *Searcher) IndexBatch(ctx context.Context, nodes []types.Node, filePaths []string) error {
	if len(nodes) == 0 {
		return nil
	}

	texts := make([]string, len(nodes))
	for i, n := range nodes {
		texts[i] = buildEmbedText(n)
	}

	vecs, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("batch embed: %w", err)
	}

	for i, vec := range vecs {
		s.embedder.AddVector(hex.EncodeToString(nodes[i].NodeHash[:]), vec)
	}

	// Persist to store if available.
	if s.store != nil {
		hashes := make([]types.Hash, len(nodes))
		raw := make([][]byte, len(nodes))
		for i, vec := range vecs {
			hashes[i] = nodes[i].NodeHash
			raw[i] = float32sToBytes(vec)
		}
		if err := s.store.BatchPutEmbeddings(ctx, s.model, hashes, raw); err != nil {
			return fmt.Errorf("persist embeddings: %w", err)
		}
	}
	return nil
}

// Count returns the number of indexed vectors.
func (s *Searcher) Count() int {
	return s.embedder.Count()
}

// ReRank embeds the query and each candidate text, returns indices sorted by
// descending cosine similarity to the query. Used as a post-RWR re-ranking step.
func (s *Searcher) ReRank(ctx context.Context, query string, candidates []string) ([]int, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// Embed query + all candidates in one batch.
	all := make([]string, 0, 1+len(candidates))
	all = append(all, query)
	all = append(all, candidates...)

	vecs, err := s.embedder.EmbedBatch(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("rerank embed: %w", err)
	}
	if len(vecs) < 1+len(candidates) {
		return nil, fmt.Errorf("rerank: expected %d vectors, got %d", 1+len(candidates), len(vecs))
	}

	queryVec := vecs[0]

	// Compute cosine similarity for each candidate.
	type scored struct {
		idx   int
		score float64
	}
	scores := make([]scored, len(candidates))
	for i := range candidates {
		scores[i] = scored{idx: i, score: cosine(queryVec, vecs[i+1])}
	}

	// Sort by descending similarity.
	sort.Slice(scores, func(a, b int) bool {
		return scores[a].score > scores[b].score
	})

	result := make([]int, len(scores))
	for i, s := range scores {
		result[i] = s.idx
	}
	return result, nil
}

// ReRankScores embeds the query and each candidate, returns cosine similarity
// scores for each candidate at its original index position.
func (s *Searcher) ReRankScores(ctx context.Context, query string, candidates []string) ([]float64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	all := make([]string, 0, 1+len(candidates))
	all = append(all, query)
	all = append(all, candidates...)

	vecs, err := s.embedder.EmbedBatch(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("rerank-scores embed: %w", err)
	}
	if len(vecs) < 1+len(candidates) {
		return nil, fmt.Errorf("rerank-scores: expected %d vectors, got %d", 1+len(candidates), len(vecs))
	}

	queryVec := vecs[0]
	scores := make([]float64, len(candidates))
	for i := range candidates {
		scores[i] = cosine(queryVec, vecs[i+1])
	}
	return scores, nil
}

// cosine computes cosine similarity between two vectors.
func cosine(a, b []float32) float64 {
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float64(dot) / (float64(sqrt32(normA)) * float64(sqrt32(normB)))
}

func sqrt32(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}

// Close releases the underlying embedder resources.
func (s *Searcher) Close() error {
	return s.embedder.Close()
}

// buildEmbedText creates a rich natural-language text representation of a node
// for embedding. The richer the text, the more semantic signal the embedding model
// has to work with. Includes:
//   - Symbol kind in natural language ("function", "type", "method")
//   - Symbol name split from CamelCase/snake_case into words
//   - Package name as context
//   - Signature with parameter and return types expanded
//   - File path components as additional context
func buildEmbedText(n types.Node) string {
	var parts []string

	// Doc comment is the richest semantic signal: written by humans in natural language.
	// Prioritize it as the first component of embed text.
	if n.Doc != "" {
		parts = append(parts, n.Doc)
	}

	// Extract package and symbol name from qualified name.
	// "github.com/org/repo://internal/context.ContextEngine.ForTask" -> package "context", symbol "ForTask"
	qn := n.QualifiedName
	pkg := ""
	symbolName := qn

	// Find the last path component before the symbol.
	if idx := strings.LastIndex(qn, "/"); idx >= 0 {
		rest := qn[idx+1:]
		if dot := strings.Index(rest, "."); dot >= 0 {
			pkg = rest[:dot]
			symbolName = rest[dot+1:]
		}
	}

	// Kind as natural language.
	switch n.Kind {
	case "function":
		parts = append(parts, "function")
	case "method":
		parts = append(parts, "method on a type")
	case "type":
		parts = append(parts, "type definition")
	case "interface":
		parts = append(parts, "interface contract")
	case "const":
		parts = append(parts, "constant value")
	case "var":
		parts = append(parts, "variable")
	default:
		parts = append(parts, n.Kind)
	}

	// Symbol name split into words for semantic matching.
	// "TransitiveCallers" -> "Transitive Callers"
	// "blast_radius" -> "blast radius"
	splitName := splitSymbolToWords(symbolName)
	parts = append(parts, splitName)

	// Package context.
	if pkg != "" {
		parts = append(parts, "in package "+pkg)
	}

	// Signature provides parameter and return type context.
	if n.Signature != "" {
		// Clean up the signature for readability.
		sig := n.Signature
		// Remove "func " prefix, type receiver.
		sig = strings.TrimPrefix(sig, "func ")
		if paren := strings.Index(sig, "("); paren > 0 {
			sig = sig[paren:]
		}
		if sig != "" && sig != "()" {
			parts = append(parts, "with signature "+sig)
		}
	}

	return strings.Join(parts, " ")
}

// splitSymbolToWords converts a CamelCase or snake_case symbol into space-separated words.
// "TransitiveCallers" -> "Transitive Callers"
// "blast_radius" -> "blast radius"
// "ContextEngine.ForTask" -> "Context Engine For Task"
func splitSymbolToWords(s string) string {
	// Replace dots, underscores with spaces.
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")

	// Split CamelCase.
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(s[i-1])
			if prev >= 'a' && prev <= 'z' {
				result = append(result, ' ')
			} else if prev >= 'A' && prev <= 'Z' && i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z' {
				result = append(result, ' ')
			}
		}
		result = append(result, r)
	}
	return string(result)
}

// ReRankByHashes re-ranks candidates using cached vectors from the store.
// Only embeds the query text (1 inference call). Candidates with no cached
// vector fall back to on-the-fly embedding. Returns cosine similarity scores
// at original index positions, same contract as ReRankScores.
func (s *Searcher) ReRankByHashes(ctx context.Context, query string, hashes []types.Hash, fallbackTexts []string) ([]float64, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	// Embed just the query (1 text, ~115ms).
	queryVec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Look up cached vectors.
	var cached map[types.Hash][]byte
	if s.store != nil {
		cached, err = s.store.GetEmbeddings(ctx, s.model, hashes)
		if err != nil {
			cached = nil // degrade gracefully
		}
	}

	// Identify cache misses that need embedding.
	var missIdxs []int
	var missTexts []string
	candidateVecs := make([][]float32, len(hashes))

	for i, h := range hashes {
		if raw, ok := cached[h]; ok && len(raw) > 0 {
			candidateVecs[i] = bytesToFloat32s(raw)
		} else {
			missIdxs = append(missIdxs, i)
			if i < len(fallbackTexts) {
				missTexts = append(missTexts, fallbackTexts[i])
			} else {
				missTexts = append(missTexts, "")
			}
		}
	}

	// Batch-embed cache misses.
	if len(missTexts) > 0 {
		missVecs, err := s.embedder.EmbedBatch(ctx, missTexts)
		if err != nil {
			return nil, fmt.Errorf("embed misses: %w", err)
		}
		// Persist newly computed vectors.
		if s.store != nil && len(missVecs) == len(missIdxs) {
			persistHashes := make([]types.Hash, len(missIdxs))
			persistRaw := make([][]byte, len(missIdxs))
			for j, idx := range missIdxs {
				persistHashes[j] = hashes[idx]
				persistRaw[j] = float32sToBytes(missVecs[j])
			}
			_ = s.store.BatchPutEmbeddings(ctx, s.model, persistHashes, persistRaw)
		}
		for j, idx := range missIdxs {
			candidateVecs[idx] = missVecs[j]
		}
	}

	// Compute cosine similarities.
	scores := make([]float64, len(hashes))
	for i, vec := range candidateVecs {
		if vec != nil {
			scores[i] = cosine(queryVec, vec)
		}
	}
	return scores, nil
}

// float32sToBytes serializes a float32 slice to little-endian bytes.
func float32sToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32s deserializes little-endian bytes to a float32 slice.
func bytesToFloat32s(b []byte) []float32 {
	n := len(b) / 4
	v := make([]float32, n)
	for i := range n {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// LoadAndSearchFromStore loads all cached vectors from the embedding store and
// performs brute-force cosine similarity search against the query. This bypasses
// the HNSW index entirely: no index build, no in-memory graph, just read vectors
// from SQLite and compute cosine. O(n) per query but eliminates the multi-minute
// HNSW rebuild that dominates startup time on large repos.
//
// Returns the k nearest node hashes sorted by descending similarity.
// If the store has no vectors, returns nil (not an error).
func (s *Searcher) LoadAndSearchFromStore(ctx context.Context, query string, k int) ([]types.Hash, error) {
	if s.store == nil {
		return nil, nil
	}

	// Lazy-load vectors on first call. Converts bytes to float32 once and
	// caches for all subsequent searches on this repo. Only loads when
	// gap-fill actually fires (< 5 candidates), not at startup.
	if !s.vecCacheSet {
		s.vecCacheSet = true
		raw, err := s.store.GetAllEmbeddings(ctx, s.model)
		if err == nil && len(raw) > 0 {
			s.vecCache = make(map[types.Hash][]float32, len(raw))
			for h, b := range raw {
				v := bytesToFloat32s(b)
				if len(v) > 0 {
					s.vecCache[h] = v
				}
			}
		}
	}

	if len(s.vecCache) == 0 {
		return nil, nil
	}

	// Embed the query (1 inference call, ~120ms).
	queryVec, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Brute-force cosine similarity against cached vectors.
	type scored struct {
		hash  types.Hash
		score float64
	}
	results := make([]scored, 0, len(s.vecCache))
	for h, vec := range s.vecCache {
		sim := cosine(queryVec, vec)
		results = append(results, scored{hash: h, score: sim})
	}

	// Sort by descending similarity.
	sort.Slice(results, func(a, b int) bool {
		return results[a].score > results[b].score
	})

	// Take top-k.
	if k > len(results) {
		k = len(results)
	}
	hashes := make([]types.Hash, k)
	for i := 0; i < k; i++ {
		hashes[i] = results[i].hash
	}
	return hashes, nil
}

func hashFromHex(h string) (types.Hash, error) {
	var hash types.Hash
	b, err := hex.DecodeString(h)
	if err != nil {
		return hash, err
	}
	if len(b) != 32 {
		return hash, fmt.Errorf("invalid hash length: %d", len(b))
	}
	copy(hash[:], b)
	return hash, nil
}
