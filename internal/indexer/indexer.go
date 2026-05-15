package indexer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SnapshotComputer computes a point-in-time snapshot for a repository.
// Defined locally to avoid importing internal/snapshot.
type SnapshotComputer interface {
	ComputeSnapshot(ctx context.Context, repoHash types.Hash, commitHash string) (*types.Snapshot, error)
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
	file := types.File{
		FileHash:    opts.FileHash,
		RepoHash:    opts.RepoHash,
		Path:        opts.FilePath,
		ContentHash: opts.FileHash,
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

	for _, absPath := range filePaths {
		relPath, err := filepath.Rel(repoPath, absPath)
		if err != nil {
			continue
		}

		// Check if any extractor can handle this file.
		if idx.registry.FindExtractor(relPath) == nil {
			continue
		}

		// Read file content and compute hash.
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		contentHash := sha256.Sum256(content)
		fileHash := types.Hash(contentHash)

		// Skip unchanged files.
		if oldHash, exists := existingByPath[relPath]; exists && oldHash == fileHash {
			continue
		}

		opts := types.ExtractOptions{
			RepoURL:    repoURL,
			RepoHash:   repoHash,
			CommitHash: commitHash,
			FilePath:   relPath,
			FileHash:   fileHash,
			Content:    content,
			ModuleRoot: repoPath,
		}

		if _, err := idx.IndexFile(ctx, opts); err != nil {
			return nil, fmt.Errorf("index file %s: %w", relPath, err)
		}
	}

	// Compute and store snapshot.
	snap, err := idx.snapshot.ComputeSnapshot(ctx, repoHash, commitHash)
	if err != nil {
		return nil, fmt.Errorf("compute snapshot: %w", err)
	}

	return snap, nil
}
