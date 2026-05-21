package indexer

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/indexer/authorship"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/ownership"
	"github.com/blackwell-systems/knowing/internal/resolver"
	"github.com/blackwell-systems/knowing/internal/roster"
	"github.com/blackwell-systems/knowing/internal/types"
)

// The Indexer is the top-level orchestrator for building the knowledge graph.
// It walks a repository's files, dispatches each to the appropriate Extractor,
// stores the resulting nodes and edges, computes a Merkle snapshot, records
// edge events for diffing, and runs the cross-repo edge resolver.
//
// Incremental indexing: on re-index, files whose content hash has not changed
// are skipped entirely. For changed files, old nodes and edges are deleted
// before new ones are inserted, and "removed"/"added" edge events are recorded
// for the snapshot diff.

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

// cleanupStore is an optional interface for stores that support
// file-level cleanup operations.
type cleanupStore interface {
	DeleteNodesByFile(ctx context.Context, fileHash types.Hash) (int, error)
	DeleteEdgesBySourceFile(ctx context.Context, fileHash types.Hash) ([]types.Edge, error)
	EdgesBySourceFile(ctx context.Context, fileHash types.Hash) ([]types.Edge, error)
}

// Indexer orchestrates extractors to index a repository's source code
// into the knowledge graph. It manages the full lifecycle: file discovery,
// content-based change detection, extraction dispatch, batch storage,
// snapshot computation, edge event recording, and cross-repo resolution.
type Indexer struct {
	store       types.GraphStore
	snapshot    SnapshotComputer
	registry    *ExtractorRegistry
	Concurrency int  // number of parallel extraction workers; 0 means runtime.GOMAXPROCS
	SkipBlame   bool // skip git blame authorship extraction (expensive on large repos)

	// changedMu protects lastChangedFiles, which is read by the daemon to
	// determine which files need LSP enrichment after an index run.
	changedMu        sync.Mutex
	lastChangedFiles []string
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
// It runs ALL matching extractors (not just the first) and merges results.
// When multiple extractors handle the same file, the first extractor that
// parses with tree-sitter sets opts.ParsedTree; subsequent extractors reuse it.
func (idx *Indexer) extractFile(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, *types.File, error) {
	extractors := idx.registry.FindAllExtractors(opts.FilePath)
	if len(extractors) == 0 {
		return &types.ExtractResult{}, nil, nil
	}

	merged := &types.ExtractResult{}
	var sharedTree interface{ Close() } // track tree for cleanup

	for _, ext := range extractors {
		result, err := ext.Extract(ctx, opts)
		if err != nil {
			continue
		}
		if result != nil {
			merged.Nodes = append(merged.Nodes, result.Nodes...)
			merged.Edges = append(merged.Edges, result.Edges...)
		}
		// If the extractor shared a parsed tree, extract the root node for
		// subsequent extractors and track the tree for closing.
		if opts.ParsedTree == nil && result != nil && result.ParsedTree != nil {
			if st, ok := result.ParsedTree.(*gotsextractor.SharedTree); ok {
				opts.ParsedTree = st.Root // pass root node to subsequent extractors
				sharedTree = st.Tree      // track for closing
			}
		}
	}

	// Close shared tree after all extractors have used it.
	if sharedTree != nil {
		sharedTree.Close()
	}

	contentHash := types.NewHash(opts.Content)
	file := &types.File{
		FileHash:    opts.FileHash,
		RepoHash:    opts.RepoHash,
		Path:        opts.FilePath,
		ContentHash: contentHash,
	}

	return merged, file, nil
}

// IndexRepo indexes all source files in a repository, skipping files
// whose content hash has not changed since the last index.
func (idx *Indexer) IndexRepo(ctx context.Context, repoURL, repoPath, commitHash string) (*types.Snapshot, error) {
	// Repo identity is sha256(repoURL); this is the same computation used
	// by ComputeNodeHash and everywhere else that needs the repo hash.
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

	// Build a module-to-repo-URL map by reading go.mod from each indexed repo.
	// This lets extractors resolve cross-repo call targets to the correct stored
	// repo URL instead of using heuristic inference from the import path.
	moduleToRepo := idx.buildModuleToRepoMap(ctx)

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
			name := d.Name()
			// Skip hidden dirs, dependency dirs, and common non-source dirs.
			switch name {
			case ".git", ".claude", ".knowing", ".polywave-state",
				"vendor", "node_modules", "testdata",
				"__pycache__", ".mypy_cache", ".pytest_cache",
				"target", "build", "dist", "out",
				"corpus",
				// Large monorepo dirs that are mirrors/generated/third-party.
				"staging", "third_party", "_output", "hack":
				return filepath.SkipDir
			}
			// Skip hidden directories (dot-prefixed), except .github (contains CI workflows).
			if len(name) > 1 && name[0] == '.' && name != ".github" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only collect files an extractor can handle (avoids reading binary/media files).
		// Also skip generated files (protobuf, deepcopy, mocks).
		name := d.Name()
		if strings.HasPrefix(name, "zz_generated") || strings.HasSuffix(name, ".pb.go") ||
			strings.HasSuffix(name, ".pb.gw.go") || strings.HasSuffix(name, "_generated.go") {
			return nil
		}
		relP, _ := filepath.Rel(repoPath, path)
		if relP != "" && idx.registry.FindExtractor(relP) != nil {
			filePaths = append(filePaths, path)
		}
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

	// Track removed edges for event recording and changed file paths.
	var removedEdges []types.Edge
	var changedFilePaths []string
	cs, hasCleanup := idx.store.(cleanupStore)

	// Detect deleted files: files in the store that no longer exist on disk.
	// Build set of current relative paths for O(1) lookup.
	if hasCleanup {
		currentPaths := make(map[string]bool, len(filePaths))
		for _, absPath := range filePaths {
			relPath, err := filepath.Rel(repoPath, absPath)
			if err == nil {
				currentPaths[relPath] = true
			}
		}
		for oldPath := range existingByPath {
			if !currentPaths[oldPath] {
				// File was deleted: clean up its nodes and edges.
				changedFilePaths = append(changedFilePaths, oldPath)
				oldFile, err := idx.store.FileByPath(ctx, repoHash, oldPath)
				if err == nil && oldFile != nil {
					removed, err := cs.DeleteEdgesBySourceFile(ctx, oldFile.FileHash)
					if err == nil {
						removedEdges = append(removedEdges, removed...)
					}
					_, _ = cs.DeleteNodesByFile(ctx, oldFile.FileHash)
				}
			}
		}
	}

	// Phase 1: Filter to extractable, changed files and do cleanup (sequential, needs DB).
	type fileWork struct {
		absPath  string
		relPath  string
		content  []byte
		fileHash types.Hash
	}
	var work []fileWork

	for _, absPath := range filePaths {
		relPath, err := filepath.Rel(repoPath, absPath)
		if err != nil {
			continue
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		// Skip generated files by checking for "Code generated" or "DO NOT EDIT" in first 512 bytes.
		if isGeneratedContent(content) {
			continue
		}


		contentHash := types.NewHash(content)

		if oldHash, exists := existingByPath[relPath]; exists && oldHash == contentHash {
			continue
		}

		changedFilePaths = append(changedFilePaths, relPath)

		// Clean up old nodes/edges for changed files (must be sequential, touches DB).
		if hasCleanup {
			if _, exists := existingByPath[relPath]; exists {
				oldFile, err := idx.store.FileByPath(ctx, repoHash, relPath)
				if err == nil && oldFile != nil {
					removed, err := cs.DeleteEdgesBySourceFile(ctx, oldFile.FileHash)
					if err == nil {
						removedEdges = append(removedEdges, removed...)
					}
					_, _ = cs.DeleteNodesByFile(ctx, oldFile.FileHash)
				}
			}
		}

		fileHashInput := append(repoHash[:], []byte(relPath)...)
		fileHashInput = append(fileHashInput, contentHash[:]...)
		fileHash := types.NewHash(fileHashInput)

		work = append(work, fileWork{
			absPath:  absPath,
			relPath:  relPath,
			content:  content,
			fileHash: fileHash,
		})
	}

	// Phase 2+3: Producer-consumer pipeline.
	// Extract workers produce results into a channel. A storage writer consumes
	// them in batches and commits to SQLite. Extraction and storage run concurrently.
	// DB gets data within seconds of starting. Kill at any time = partial data persists.

	type fileResult struct {
		result *types.ExtractResult
		file   *types.File
		err    error
		relPath string
	}

	numWorkers := idx.Concurrency
	if numWorkers <= 0 {
		numWorkers = runtime.GOMAXPROCS(0)
	}
	if numWorkers > len(work) {
		numWorkers = len(work)
	}

	resultCh := make(chan fileResult, numWorkers*2) // buffered to avoid blocking workers
	totalFiles := len(work)
	startTime := time.Now()

	// --- Producer: extraction workers ---
	var extractWg sync.WaitGroup
	workCh := make(chan int, len(work))

	for w := 0; w < numWorkers; w++ {
		extractWg.Add(1)
		go func() {
			defer extractWg.Done()
			for i := range workCh {
				fw := work[i]
				opts := types.ExtractOptions{
					RepoURL:         repoURL,
					RepoHash:        repoHash,
					CommitHash:      commitHash,
					FilePath:        fw.relPath,
					FileHash:        fw.fileHash,
					Content:         fw.content,
					ModuleRoot:      repoPath,
					ModuleToRepoURL: moduleToRepo,
				}

				fileCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				result, file, err := idx.extractFile(fileCtx, opts)
				cancel()
				if fileCtx.Err() == context.DeadlineExceeded {
					resultCh <- fileResult{result: &types.ExtractResult{}, relPath: fw.relPath}
				} else {
					resultCh <- fileResult{result: result, file: file, err: err, relPath: fw.relPath}
				}
			}
		}()
	}

	// Feed work to extractors.
	go func() {
		for i := range work {
			workCh <- i
		}
		close(workCh)
		extractWg.Wait()
		close(resultCh) // signal storage writer that extraction is done
	}()

	// --- Consumer: storage writer (single goroutine, owns all DB writes) ---
	const batchSize = 500
	var batchNodes []types.Node
	var batchEdges []types.Edge
	var batchFiles []types.File
	completed := 0
	lastReport := time.Now()
	var storeErr error

	bs, hasBatch := idx.store.(batchStore)

	for fr := range resultCh {
		if fr.err != nil {
			storeErr = fmt.Errorf("extract file %s: %w", fr.relPath, fr.err)
			break
		}
		if fr.result != nil {
			batchNodes = append(batchNodes, fr.result.Nodes...)
			batchEdges = append(batchEdges, fr.result.Edges...)
			allNodes = append(allNodes, fr.result.Nodes...)
			allEdges = append(allEdges, fr.result.Edges...)
		}
		if fr.file != nil {
			batchFiles = append(batchFiles, *fr.file)
			allFiles = append(allFiles, *fr.file)
		}
		completed++

		// Flush batch to DB every batchSize files.
		if len(batchFiles) >= batchSize || completed == totalFiles {
			if hasBatch {
				if len(batchFiles) > 0 {
					if err := bs.BatchPutFiles(ctx, batchFiles); err != nil {
						storeErr = fmt.Errorf("batch store files: %w", err)
						break
					}
				}
				if len(batchNodes) > 0 {
					if err := bs.BatchPutNodes(ctx, batchNodes); err != nil {
						storeErr = fmt.Errorf("batch store nodes: %w", err)
						break
					}
				}
				if len(batchEdges) > 0 {
					if err := bs.BatchPutEdges(ctx, batchEdges); err != nil {
						storeErr = fmt.Errorf("batch store edges: %w", err)
						break
					}
				}
			} else {
				for _, f := range batchFiles {
					_ = idx.store.PutFile(ctx, f)
				}
				for _, n := range batchNodes {
					_ = idx.store.PutNode(ctx, n)
				}
				for _, e := range batchEdges {
					_ = idx.store.PutEdge(ctx, e)
				}
			}
			batchNodes = batchNodes[:0]
			batchEdges = batchEdges[:0]
			batchFiles = batchFiles[:0]
		}

		// Progress report.
		if time.Since(lastReport) > 2*time.Second || completed == totalFiles {
			elapsed := time.Since(startTime)
			filesPerSec := float64(completed) / elapsed.Seconds()
			remaining := time.Duration(float64(totalFiles-completed)/filesPerSec) * time.Second
			fmt.Fprintf(os.Stderr, "\r  [%d/%d] %.0f files/s, %d edges, ETA %s",
				completed, totalFiles, filesPerSec, len(allEdges), remaining.Truncate(time.Second))
			lastReport = time.Now()
		}
	}

	fmt.Fprintf(os.Stderr, "\r  [%d/%d] done, %d edges in %s%40s\n",
		completed, totalFiles, len(allEdges), time.Since(startTime).Truncate(time.Millisecond), "")

	if storeErr != nil {
		return nil, storeErr
	}

	// Extract CODEOWNERS ownership edges if a CODEOWNERS file exists.
	if coPath := ownership.FindCodeowners(repoPath); coPath != "" {
		rules, err := ownership.ParseCodeowners(coPath)
		if err == nil && len(rules) > 0 {
			ownerNodes, ownerEdges := ownership.ExtractOwnership(repoURL, allFiles, rules)
			if bs, ok := idx.store.(batchStore); ok {
				if len(ownerNodes) > 0 {
					_ = bs.BatchPutNodes(ctx, ownerNodes)
				}
				if len(ownerEdges) > 0 {
					_ = bs.BatchPutEdges(ctx, ownerEdges)
				}
			} else {
				for _, n := range ownerNodes {
					_ = idx.store.PutNode(ctx, n)
				}
				for _, e := range ownerEdges {
					_ = idx.store.PutEdge(ctx, e)
				}
			}
		}
	}

	// Extract authored_by edges from git blame (parallel, best-effort).
	// This is expensive (one git blame subprocess per file) so it runs in parallel
	// and is skippable via idx.SkipBlame.
	if !idx.SkipBlame && len(changedFilePaths) > 0 {
		fmt.Fprintf(os.Stderr, "  Authorship extraction (%d files)...\n", len(allFiles))
		blameStart := time.Now()

		type blameResult struct {
			nodes []types.Node
			edges []types.Edge
		}

		blameResults := make([]blameResult, len(allFiles))
		var blameWg sync.WaitGroup
		blameCh := make(chan int, len(allFiles))

		blameWorkers := numWorkers
		if blameWorkers > len(allFiles) {
			blameWorkers = len(allFiles)
		}

		for w := 0; w < blameWorkers; w++ {
			blameWg.Add(1)
			go func() {
				defer blameWg.Done()
				for i := range blameCh {
					f := allFiles[i]
					nodes, err := idx.store.NodesByFilePath(ctx, repoHash, f.Path)
					if err != nil || len(nodes) == 0 {
						continue
					}
					an, ae, err := authorship.ExtractAuthorship(repoURL, repoPath, f, nodes)
					if err != nil {
						continue
					}
					blameResults[i] = blameResult{nodes: an, edges: ae}
				}
			}()
		}

		for i := range allFiles {
			blameCh <- i
		}
		close(blameCh)
		blameWg.Wait()

		// Collect and store authorship results.
		var authorNodes []types.Node
		var authorEdges []types.Edge
		for _, br := range blameResults {
			authorNodes = append(authorNodes, br.nodes...)
			authorEdges = append(authorEdges, br.edges...)
		}
		if bs, ok := idx.store.(batchStore); ok {
			if len(authorNodes) > 0 {
				_ = bs.BatchPutNodes(ctx, authorNodes)
			}
			if len(authorEdges) > 0 {
				_ = bs.BatchPutEdges(ctx, authorEdges)
			}
		} else {
			for _, n := range authorNodes {
				_ = idx.store.PutNode(ctx, n)
			}
			for _, e := range authorEdges {
				_ = idx.store.PutEdge(ctx, e)
			}
		}
		fmt.Fprintf(os.Stderr, "  Authorship: %d edges in %s\n", len(authorEdges), time.Since(blameStart).Truncate(time.Millisecond))
	}

	// Rebuild FTS and compute snapshot concurrently (independent operations).
	fmt.Fprintf(os.Stderr, "  Finalizing (FTS + snapshot)...\n")
	finalStart := time.Now()

	type ftsRebuilder interface {
		RebuildFTS(ctx context.Context) error
	}

	var ftsErr error
	var snap *types.Snapshot
	var snapErr error
	var finalWg sync.WaitGroup

	// FTS rebuild (parallel string split + sequential INSERT + rebuild).
	if fr, ok := idx.store.(ftsRebuilder); ok {
		finalWg.Add(1)
		go func() {
			defer finalWg.Done()
			ftsErr = fr.RebuildFTS(ctx)
		}()
	}

	// Snapshot computation (hierarchical Merkle tree).
	finalWg.Add(1)
	go func() {
		defer finalWg.Done()
		snap, snapErr = idx.snapshot.ComputeSnapshot(ctx, repoHash, commitHash)
	}()

	finalWg.Wait()
	fmt.Fprintf(os.Stderr, "  Finalized in %s\n", time.Since(finalStart).Truncate(time.Millisecond))

	if ftsErr != nil {
		return nil, fmt.Errorf("rebuild fts: %w", ftsErr)
	}
	if snapErr != nil {
		return nil, fmt.Errorf("compute snapshot: %w", snapErr)
	}

	// Record edge events for the diff between old and new edges. These
	// events power SnapshotDiff: "removed" events for edges that disappeared
	// and "added" events for edges that are new in this snapshot.
	if hasCleanup && snap != nil {
		// Build set of removed edge hashes for O(1) lookup.
		removedSet := make(map[types.Hash]bool, len(removedEdges))
		for _, e := range removedEdges {
			removedSet[e.EdgeHash] = true
		}

		// Collect new edges from changed files.
		newEdgeSet := make(map[types.Hash]bool)
		for _, f := range allFiles {
			newEdges, err := cs.EdgesBySourceFile(ctx, f.FileHash)
			if err != nil {
				continue
			}
			for _, e := range newEdges {
				newEdgeSet[e.EdgeHash] = true
			}
		}

		now := time.Now().Unix()

		// Record "removed" events with full edge data. The edge itself has been
		// deleted from the edges table, so the event must be self-contained:
		// SnapshotDiff reads these columns directly without joining to edges.
		for _, e := range removedEdges {
			if !newEdgeSet[e.EdgeHash] {
				_ = idx.store.RecordEdgeEvent(ctx, types.EdgeEvent{
					EdgeHash:     e.EdgeHash,
					EventType:    "removed",
					SnapshotHash: snap.SnapshotHash,
					SourceCommit: commitHash,
					IndexerVer:   "v1",
					Timestamp:    now,
					SourceHash:   e.SourceHash,
					TargetHash:   e.TargetHash,
					EdgeType:     e.EdgeType,
					Confidence:   e.Confidence,
					Provenance:   e.Provenance,
				})
			}
		}

		// Record "added" events with full edge data for consistency.
		for _, e := range allEdges {
			if !removedSet[e.EdgeHash] {
				_ = idx.store.RecordEdgeEvent(ctx, types.EdgeEvent{
					EdgeHash:     e.EdgeHash,
					EventType:    "added",
					SnapshotHash: snap.SnapshotHash,
					SourceCommit: commitHash,
					IndexerVer:   "v1",
					Timestamp:    now,
					SourceHash:   e.SourceHash,
					TargetHash:   e.TargetHash,
					EdgeType:     e.EdgeType,
					Confidence:   e.Confidence,
					Provenance:   e.Provenance,
				})
			}
		}
	}

	// Store changed files for downstream consumers (the daemon reads this
	// list to scope LSP enrichment to only the files that actually changed).
	idx.changedMu.Lock()
	idx.lastChangedFiles = changedFilePaths
	idx.changedMu.Unlock()

	// Resolve cross-repo dangling edges. Best-effort: failures here do not
	// fail the overall index run because the graph is still usable with
	// dangling edges (they just won't link across repos).
	_, _ = idx.ResolveEdges(ctx)

	// Auto-GC: if edge_events exceed threshold, run incremental GC.
	// Inspired by git's gc_auto_threshold (6700 loose objects triggers gc).
	idx.maybeAutoGC(ctx, repoHash)

	return snap, nil
}

// autoGCThreshold is the number of edge_events that triggers automatic GC.
// Edge events accumulate on every re-index; when they exceed this threshold,
// old snapshots are pruned to prevent unbounded growth.
const autoGCThreshold = 5000

// autoGCKeepCount is the number of recent snapshots preserved by auto-GC.
const autoGCKeepCount = 10

// gcCapable is an optional interface for snapshot managers that support GC.
type gcCapable interface {
	GarbageCollect(ctx context.Context, repoHash types.Hash, keepCount int) (int, error)
}

// maybeAutoGC checks if edge_events have exceeded the threshold and runs
// incremental GC if so. Best-effort: failures are logged but don't fail indexing.
func (idx *Indexer) maybeAutoGC(ctx context.Context, repoHash types.Hash) {
	sqlStore, ok := idx.store.(interface {
		DB() *sql.DB
	})
	if !ok {
		return
	}
	db := sqlStore.DB()

	var eventCount int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM edge_events").Scan(&eventCount)
	if err != nil {
		return
	}

	if eventCount < autoGCThreshold {
		return
	}

	// Trigger GC via snapshot manager.
	if gc, ok := idx.snapshot.(gcCapable); ok {
		_, _ = gc.GarbageCollect(ctx, repoHash, autoGCKeepCount)
	}
}

// ResolveEdges runs the cross-repo edge resolver to retarget dangling edges
// whose target hashes were computed with the wrong repo URL.
func (idx *Indexer) ResolveEdges(ctx context.Context) (*resolver.ResolveStats, error) {
	r := resolver.NewResolver(idx.store)
	return r.Resolve(ctx)
}

// buildModuleToRepoMap reads go.mod from each indexed repo to build a mapping
// from Go module paths to stored repo URLs. This allows extractors to resolve
// cross-repo call targets to the correct stored repo URL.
func (idx *Indexer) buildModuleToRepoMap(ctx context.Context) map[string]string {
	// Start with repos in the local database.
	result := make(map[string]string)
	repos, err := idx.store.AllRepos(ctx)
	if err == nil {
		for _, repo := range repos {
			modulePath := readModulePath(repo.RepoURL)
			if modulePath != "" {
				result[modulePath] = repo.RepoURL
			}
		}
	}

	// Merge the global roster's module map. This is critical for cross-repo
	// edge resolution with per-repo isolation: the local database only has
	// one repo, but the roster knows all registered repos and their module
	// paths. Without this, cross-repo target hashes use the wrong repo URL
	// and never match the actual nodes in the target repo's database.
	rosterMap := roster.ModuleMap()
	for modPath, repoURL := range rosterMap {
		if _, exists := result[modPath]; !exists {
			result[modPath] = repoURL
		}
	}

	return result
}

// LastChangedFiles returns the file paths that changed in the most
// recent IndexRepo call.
func (idx *Indexer) LastChangedFiles() []string {
	idx.changedMu.Lock()
	defer idx.changedMu.Unlock()
	result := make([]string, len(idx.lastChangedFiles))
	copy(result, idx.lastChangedFiles)
	return result
}

// readModulePath reads the module path from go.mod in the given directory.
// Returns empty string if go.mod doesn't exist or can't be parsed.
func readModulePath(repoPath string) string {
	f, err := os.Open(filepath.Join(repoPath, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}

// isGeneratedContent checks the first 512 bytes of a file for generated-code markers.
// Returns true if the file should be skipped.
func isGeneratedContent(content []byte) bool {
	// Check first 512 bytes (covers all standard generated-code headers).
	header := content
	if len(header) > 512 {
		header = header[:512]
	}
	s := string(header)

	// Standard Go generated markers.
	if strings.Contains(s, "Code generated") || strings.Contains(s, "DO NOT EDIT") ||
		strings.Contains(s, "AUTO-GENERATED") || strings.Contains(s, "auto-generated") ||
		strings.Contains(s, "Autogenerated") || strings.Contains(s, "GENERATED FILE") {
		return true
	}

	// Python generated markers.
	if strings.Contains(s, "# Generated by") || strings.Contains(s, "# This file is generated") {
		return true
	}

	// TypeScript/JS generated markers.
	if strings.Contains(s, "// This file was automatically generated") ||
		strings.Contains(s, "/* tslint:disable */") && strings.Contains(s, "auto-generated") {
		return true
	}

	return false
}
