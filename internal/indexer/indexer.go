package indexer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/knowing/internal/resolver"
	"github.com/blackwell-systems/knowing/internal/types"
)

// SnapshotComputer computes a point-in-time snapshot for a repository.
// Defined locally to avoid importing internal/snapshot.
type SnapshotComputer interface {
	ComputeSnapshot(ctx context.Context, repoHash types.Hash, commitHash string) (*types.Snapshot, error)
}

// batchStore is an optional interface for stores that support batch inserts.
type batchStore interface {
	BatchPutNodes(ctx context.Context, nodes []types.Node) error
	BatchPutEdges(ctx context.Context, edges []types.Edge) error
	BatchPutFiles(ctx context.Context, files []types.File) error
}

// Indexer orchestrates extractors to index a repository's source code
// into the knowledge graph.
type Indexer struct {
	store    types.GraphStore
	snapshot SnapshotComputer
	registry *ExtractorRegistry
}

// NewIndexer creates an Indexer with the given store and snapshot computer.
func NewIndexer(store types.GraphStore, snapshot SnapshotComputer) *Indexer {
	return &Indexer{
		store:    store,
		snapshot: snapshot,
		registry: NewExtractorRegistry(),
	}
}

// Register adds an extractor to the indexer's registry.
func (idx *Indexer) Register(ext types.Extractor) {
	idx.registry.Register(ext)
}

// IndexFile extracts nodes and edges from a single file and stores them.
func (idx *Indexer) IndexFile(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	ext := idx.registry.FindExtractor(opts.FilePath)
	if ext == nil {
		return &types.ExtractResult{}, nil
	}

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", opts.FilePath, err)
	}

	// Sort nodes deterministically by qualified name then kind.
	sort.Slice(result.Nodes, func(i, j int) bool {
		if result.Nodes[i].QualifiedName != result.Nodes[j].QualifiedName {
			return result.Nodes[i].QualifiedName < result.Nodes[j].QualifiedName
		}
		return result.Nodes[i].Kind < result.Nodes[j].Kind
	})

	// Sort edges deterministically by source, target, edge type.
	sort.Slice(result.Edges, func(i, j int) bool {
		si, sj := result.Edges[i], result.Edges[j]
		if si.SourceHash != sj.SourceHash {
			return si.SourceHash.String() < sj.SourceHash.String()
		}
		if si.TargetHash != sj.TargetHash {
			return si.TargetHash.String() < sj.TargetHash.String()
		}
		return si.EdgeType < sj.EdgeType
	})

	// Store file record.
	contentHash := types.NewHash(opts.Content)
	file := types.File{
		FileHash:    opts.FileHash,
		RepoHash:    opts.RepoHash,
		Path:        opts.FilePath,
		ContentHash: contentHash,
	}
	if err := idx.store.PutFile(ctx, file); err != nil {
		return nil, fmt.Errorf("store file %s: %w", opts.FilePath, err)
	}

	// Store nodes.
	for _, n := range result.Nodes {
		if err := idx.store.PutNode(ctx, n); err != nil {
			return nil, fmt.Errorf("store node %s: %w", n.QualifiedName, err)
		}
	}

	// Store edges.
	for _, e := range result.Edges {
		if err := idx.store.PutEdge(ctx, e); err != nil {
			return nil, fmt.Errorf("store edge: %w", err)
		}
	}

	return result, nil
}

// extractFile extracts nodes and edges from a single file without storing them.
func (idx *Indexer) extractFile(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, *types.File, error) {
	ext := idx.registry.FindExtractor(opts.FilePath)
	if ext == nil {
		return &types.ExtractResult{}, nil, nil
	}

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("extract %s: %w", opts.FilePath, err)
	}

	contentHash := types.NewHash(opts.Content)
	file := &types.File{
		FileHash:    opts.FileHash,
		RepoHash:    opts.RepoHash,
		Path:        opts.FilePath,
		ContentHash: contentHash,
	}

	return result, file, nil
}

// IndexRepo indexes all source files in a repository, skipping files
// whose content hash has not changed since the last index.
func (idx *Indexer) IndexRepo(ctx context.Context, repoURL, repoPath, commitHash string) (*types.Snapshot, error) {
	// Compute repo hash from URL.
	repoHash := types.NewHash([]byte(repoURL))

	// Store repo record.
	repo := types.Repo{
		RepoHash:    repoHash,
		RepoURL:     repoURL,
		LastCommit:  commitHash,
		LastIndexed: time.Now().Unix(),
	}
	if err := idx.store.PutRepo(ctx, repo); err != nil {
		return nil, fmt.Errorf("store repo: %w", err)
	}

	// Get existing files for this repo to compare hashes.
	existingFiles, err := idx.store.FilesByRepo(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("get existing files: %w", err)
	}
	existingByPath := make(map[string]types.Hash, len(existingFiles))
	for _, f := range existingFiles {
		existingByPath[f.Path] = f.ContentHash
	}

	// Walk the repository directory, collecting file paths deterministically.
	var filePaths []string
	err = filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip hidden directories and vendor.
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		filePaths = append(filePaths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk repo: %w", err)
	}

	// Sort file paths for deterministic processing.
	sort.Strings(filePaths)

	// Accumulate all results for batch insertion.
	var allNodes []types.Node
	var allEdges []types.Edge
	var allFiles []types.File

	for _, absPath := range filePaths {
		relPath, err := filepath.Rel(repoPath, absPath)
		if err != nil {
			continue
		}

		// Check if any extractor can handle this file.
		if idx.registry.FindExtractor(relPath) == nil {
			continue
		}

		// Read file content and compute content hash.
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		contentHash := types.NewHash(content)

		// Skip unchanged files.
		if oldHash, exists := existingByPath[relPath]; exists && oldHash == contentHash {
			continue
		}

		// Compute FileHash as sha256(repoHash || path || contentHash).
		fileHashInput := append(repoHash[:], []byte(relPath)...)
		fileHashInput = append(fileHashInput, contentHash[:]...)
		fileHash := types.NewHash(fileHashInput)

		opts := types.ExtractOptions{
			RepoURL:    repoURL,
			RepoHash:   repoHash,
			CommitHash: commitHash,
			FilePath:   relPath,
			FileHash:   fileHash,
			Content:    content,
			ModuleRoot: repoPath,
		}

		result, file, err := idx.extractFile(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("extract file %s: %w", relPath, err)
		}

		allNodes = append(allNodes, result.Nodes...)
		allEdges = append(allEdges, result.Edges...)
		if file != nil {
			allFiles = append(allFiles, *file)
		}
	}

	// Batch insert if the store supports it, otherwise fall back to individual inserts.
	if bs, ok := idx.store.(batchStore); ok {
		if err := bs.BatchPutFiles(ctx, allFiles); err != nil {
			return nil, fmt.Errorf("batch store files: %w", err)
		}
		if err := bs.BatchPutNodes(ctx, allNodes); err != nil {
			return nil, fmt.Errorf("batch store nodes: %w", err)
		}
		if err := bs.BatchPutEdges(ctx, allEdges); err != nil {
			return nil, fmt.Errorf("batch store edges: %w", err)
		}
	} else {
		for _, f := range allFiles {
			if err := idx.store.PutFile(ctx, f); err != nil {
				return nil, fmt.Errorf("store file %s: %w", f.Path, err)
			}
		}
		for _, n := range allNodes {
			if err := idx.store.PutNode(ctx, n); err != nil {
				return nil, fmt.Errorf("store node %s: %w", n.QualifiedName, err)
			}
		}
		for _, e := range allEdges {
			if err := idx.store.PutEdge(ctx, e); err != nil {
				return nil, fmt.Errorf("store edge: %w", err)
			}
		}
	}

	// Compute and store snapshot.
	snap, err := idx.snapshot.ComputeSnapshot(ctx, repoHash, commitHash)
	if err != nil {
		return nil, fmt.Errorf("compute snapshot: %w", err)
	}

	// Resolve cross-repo dangling edges (best-effort, does not fail indexing).
	_, _ = idx.ResolveEdges(ctx)

	return snap, nil
}

// ResolveEdges runs the cross-repo edge resolver to retarget dangling edges
// whose target hashes were computed with the wrong repo URL.
func (idx *Indexer) ResolveEdges(ctx context.Context) (*resolver.ResolveStats, error) {
	r := resolver.NewResolver(idx.store)
	return r.Resolve(ctx)
}
