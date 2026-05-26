// Package embedding provides semantic vector embeddings for symbols.
// Uses MiniLM-L6-v2 via hugot (pure-Go ONNX runtime) for offline embedding
// generation. Model auto-downloads from Hugging Face on first use (~30MB).
// Vectors are stored in an HNSW index (coder/hnsw) for nearest-neighbor search.
package embedding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/coder/hnsw"
	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// Model configuration. Override with KNOWING_EMBED_MODEL env var.
// Default: jina-code (best for code retrieval, +15% P@10 validated).
// Options: "jina-code" (default), "bge-small", "nomic-code"
var (
	modelRepo = "jinaai/jina-embeddings-v2-base-code"
	onnxFile  = "onnx/model.onnx"
	Dims      = 768
)

func init() {
	switch os.Getenv("KNOWING_EMBED_MODEL") {
	case "bge-small":
		modelRepo = "BAAI/bge-small-en-v1.5"
		onnxFile = "onnx/model.onnx"
		Dims = 384
	case "nomic-code":
		modelRepo = "nomic-ai/nomic-embed-text-v1.5"
		onnxFile = "onnx/model.onnx"
		Dims = 768
	case "jina-code":
		modelRepo = "jinaai/jina-embeddings-v2-base-code"
		onnxFile = "onnx/model.onnx"
		Dims = 768
	}
}

// Embedder generates embedding vectors and provides nearest-neighbor search.
type Embedder struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
	index    *hnsw.Graph[string]
	count    int
	mu       sync.RWMutex
}

// New creates an Embedder, downloading the model if needed.
// The model is cached at ~/.cache/knowing/models/.
func New() (*Embedder, error) {
	session, err := hugot.NewGoSession(context.Background())
	if err != nil {
		return nil, fmt.Errorf("create hugot session: %w", err)
	}

	modelPath, err := ensureModel()
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("download model: %w", err)
	}

	config := hugot.FeatureExtractionConfig{
		ModelPath:    modelPath,
		Name:         "knowing-embeddings",
		OnnxFilename: filepath.Base(onnxFile),
		Options: []hugot.FeatureExtractionOption{
			pipelines.WithNormalization(),
		},
	}

	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	g := hnsw.NewGraph[string]()
	g.Distance = hnsw.CosineDistance

	return &Embedder{
		session:  session,
		pipeline: pipeline,
		index:    g,
	}, nil
}

// Embed returns the embedding vector for a single text string.
func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return vecs[0], nil
}

// EmbedBatch returns embedding vectors for multiple texts.
func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	output, err := e.pipeline.RunPipeline(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("run pipeline: %w", err)
	}
	return output.Embeddings, nil
}

// AddVector indexes a symbol ID with its embedding vector for nearest-neighbor search.
func (e *Embedder) AddVector(id string, vec []float32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.index.Add(hnsw.Node[string]{
		Key:   id,
		Value: hnsw.Vector(vec),
	})
	e.count++
}

// Search returns the k nearest neighbor symbol IDs to the query vector.
func (e *Embedder) Search(query []float32, k int) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.count == 0 {
		return nil
	}
	results := e.index.Search(hnsw.Vector(query), k)
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.Key
	}
	return ids
}

// Count returns the number of indexed vectors.
func (e *Embedder) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.count
}

// Close releases resources.
func (e *Embedder) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}
	return nil
}

// ensureModel downloads the ONNX model if not already cached.
func ensureModel() (string, error) {
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "knowing", "models")
	modelDir := filepath.Join(cacheDir, "BAAI_bge-small-en-v1.5")

	// Check if already downloaded.
	if _, err := os.Stat(filepath.Join(modelDir, "tokenizer.json")); err == nil {
		if _, err := os.Stat(filepath.Join(modelDir, filepath.Base(onnxFile))); err == nil {
			return modelDir, nil
		}
	}

	opts := hugot.NewDownloadOptions()
	opts.OnnxFilePath = onnxFile
	path, err := hugot.DownloadModel(context.Background(), modelRepo, cacheDir, opts)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", modelRepo, err)
	}
	return path, nil
}
