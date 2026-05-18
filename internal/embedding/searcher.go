package embedding

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

// Searcher wraps an Embedder and provides the VectorSearcher interface
// expected by the context engine. It resolves HNSW string keys (hex-encoded
// node hashes) back to types.Hash values.
type Searcher struct {
	embedder *Embedder
}

// NewSearcher creates a Searcher from an initialized Embedder.
func NewSearcher(e *Embedder) *Searcher {
	return &Searcher{embedder: e}
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
func (s *Searcher) IndexBatch(ctx context.Context, nodes []types.Node, filePaths []string) error {
	if len(nodes) == 0 {
		return nil
	}

	texts := make([]string, len(nodes))
	for i, n := range nodes {
		fp := ""
		if i < len(filePaths) {
			fp = filePaths[i]
		}
		texts[i] = fmt.Sprintf("%s %s %s %s", n.Kind, n.QualifiedName, n.Signature, fp)
	}

	vecs, err := s.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return fmt.Errorf("batch embed: %w", err)
	}

	for i, vec := range vecs {
		s.embedder.AddVector(hex.EncodeToString(nodes[i].NodeHash[:]), vec)
	}
	return nil
}

// Count returns the number of indexed vectors.
func (s *Searcher) Count() int {
	return s.embedder.Count()
}

// Close releases the underlying embedder resources.
func (s *Searcher) Close() error {
	return s.embedder.Close()
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
