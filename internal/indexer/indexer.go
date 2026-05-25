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

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/indexer/authorship"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/indexer/ownership"
	"github.com/blackwell-systems/knowing/internal/snapshot"
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
	ComputeSnapshotFromEdges(ctx context.Context, repoHash types.Hash, commitHash string, edgeInputs []snapshot.EdgeInput, nodeCount int) (*types.Snapshot, error)
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
			// Note: go.work `use` directives do NOT override these skips.
			// Staging/third-party code is dependency code; indexing it dilutes
			// RWR probability and hurts retrieval quality (-20% P@10 on k8s).
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

				// Run extraction with a hard timeout using a watchdog goroutine.
				// CGO calls (tree-sitter parsing) are NOT interruptible by Go
				// context cancellation. If a file takes >10s, the watchdog fires
				// first and the worker moves on. The CGO call completes in the
				// background (fire-and-forget) without blocking the pipeline.
				type extractResult struct {
					result *types.ExtractResult
					file   *types.File
					err    error
				}
				done := make(chan extractResult, 1)
				go func(o types.ExtractOptions, relPath string) {
					r, f, err := idx.extractFile(ctx, o)
					done <- extractResult{result: r, file: f, err: err}
				}(opts, fw.relPath)

				timer := time.NewTimer(10 * time.Second)
				select {
				case er := <-done:
					timer.Stop()
					resultCh <- fileResult{result: er.result, file: er.file, err: er.err, relPath: fw.relPath}
				case <-timer.C:
					// Extraction stuck in CGO. Send empty result and move on.
					// The background goroutine will complete eventually and its
					// result will be discarded (nobody reads from done).
					fmt.Fprintf(os.Stderr, "\n  ⚠ timeout: %s (skipped)\n", fw.relPath)
					resultCh <- fileResult{result: &types.ExtractResult{}, relPath: fw.relPath}
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

	fmt.Fprintf(os.Stderr, "  CODEOWNERS...\n")
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

	// Propagate method inheritance: for each "extends" edge, find the parent's
	// methods and create edges from the child class to those methods. This lets
	// RWR walk from Flask -> Scaffold.before_request via inheritance.
	fmt.Fprintf(os.Stderr, "  Inheritance propagation...\n")
	inheritEdges := propagateInheritance(allNodes, allEdges)
	if len(inheritEdges) > 0 {
		allEdges = append(allEdges, inheritEdges...)
		if bs, ok := idx.store.(batchStore); ok {
			_ = bs.BatchPutEdges(ctx, inheritEdges)
		} else {
			for _, e := range inheritEdges {
				_ = idx.store.PutEdge(ctx, e)
			}
		}
		fmt.Fprintf(os.Stderr, "  Inheritance: %d edges propagated\n", len(inheritEdges))
	}

	// Propagate interface implementation: for each "implements" edge, find matching
	// method names between the implementing class and the interface, and create
	// "overrides" edges connecting them. This lets RWR walk from an interface method
	// to all concrete implementations.
	fmt.Fprintf(os.Stderr, "  Interface propagation...\n")
	ifaceEdges := propagateInterfaceMethods(allNodes, allEdges)
	if len(ifaceEdges) > 0 {
		allEdges = append(allEdges, ifaceEdges...)
		if bs, ok := idx.store.(batchStore); ok {
			_ = bs.BatchPutEdges(ctx, ifaceEdges)
		} else {
			for _, e := range ifaceEdges {
				_ = idx.store.PutEdge(ctx, e)
			}
		}
		fmt.Fprintf(os.Stderr, "  Interface: %d edges propagated\n", len(ifaceEdges))
	}

	// Generate structural "contains" edges from type/class nodes to their methods.
	// 77% of type nodes have zero edges; this connects them to their methods via QN
	// structure (Type.Method), enabling RWR to walk from type seeds to discover methods.
	fmt.Fprintf(os.Stderr, "  Contains edges...\n")
	containsEdges := generateContainsEdges(allNodes)
	if len(containsEdges) > 0 {
		allEdges = append(allEdges, containsEdges...)
		if bs, ok := idx.store.(batchStore); ok {
			_ = bs.BatchPutEdges(ctx, containsEdges)
		} else {
			for _, e := range containsEdges {
				_ = idx.store.PutEdge(ctx, e)
			}
		}
		fmt.Fprintf(os.Stderr, "  Contains: %d edges\n", len(containsEdges))
	}

	// Compute semantic similarity edges between functions in the same package.
	// This bridges disconnected subgraphs where two functions do the same work
	// but don't call each other.
	fmt.Fprintf(os.Stderr, "  Similarity edges...\n")
	similarEdges := ComputeSimilarityEdges(allNodes, 0.5)
	if len(similarEdges) > 0 {
		allEdges = append(allEdges, similarEdges...)
		if bs, ok := idx.store.(batchStore); ok {
			_ = bs.BatchPutEdges(ctx, similarEdges)
		} else {
			for _, e := range similarEdges {
				_ = idx.store.PutEdge(ctx, e)
			}
		}
		fmt.Fprintf(os.Stderr, "  Similarity: %d edges\n", len(similarEdges))
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

	// Finalize: compute snapshot first, THEN start background FTS.
	// FTS competes with snapshot for SQLite access (WAL reader/writer contention).
	// Running them concurrently causes both to be slow. Sequential: snapshot first
	// (fast, read-only query + small write), then FTS in background (slow, heavy writes).
	fmt.Fprintf(os.Stderr, "  Computing snapshot...\n")
	finalStart := time.Now()

	type ftsRebuilder interface {
		RebuildFTS(ctx context.Context) error
	}

	// Build edge inputs from in-memory edges (skip DB re-read).
	fmt.Fprintf(os.Stderr, "  Building edge inputs (%d edges, %d nodes)...\n", len(allEdges), len(allNodes))
	buildStart := time.Now()

	// Build a source-hash-to-package map from allNodes.
	sourcePackage := make(map[types.Hash]string, len(allNodes))
	for _, n := range allNodes {
		if qn := n.QualifiedName; qn != "" {
			pkgPath, _ := snapshot.ExtractPackagePath(qn)
			sourcePackage[n.NodeHash] = pkgPath
		}
	}
	fmt.Fprintf(os.Stderr, "  Package map built in %s (%d entries)\n", time.Since(buildStart).Truncate(time.Millisecond), len(sourcePackage))

	edgeInputs := make([]snapshot.EdgeInput, len(allEdges))
	for i, e := range allEdges {
		pkg := sourcePackage[e.SourceHash]
		if pkg == "" {
			pkg = "_unknown"
		}
		edgeInputs[i] = snapshot.EdgeInput{
			EdgeHash:    e.EdgeHash,
			PackagePath: pkg,
			EdgeType:    e.EdgeType,
		}
	}
	fmt.Fprintf(os.Stderr, "  Edge inputs ready in %s\n", time.Since(buildStart).Truncate(time.Millisecond))

	// Compute snapshot from in-memory edges (no DB re-read).
	snapStart := time.Now()
	snap, snapErr := idx.snapshot.ComputeSnapshotFromEdges(ctx, repoHash, commitHash, edgeInputs, len(allNodes))
	fmt.Fprintf(os.Stderr, "  Merkle tree + snapshot in %s\n", time.Since(snapStart).Truncate(time.Millisecond))
	fmt.Fprintf(os.Stderr, "  Total finalization in %s\n", time.Since(finalStart).Truncate(time.Millisecond))

	if snapErr != nil {
		return nil, fmt.Errorf("compute snapshot: %w", snapErr)
	}

	// Rebuild FTS AFTER snapshot (avoids SQLite contention).
	// Runs synchronously to ensure FTS is populated before the process exits.
	// Previously ran in a background goroutine, but CLI processes exit immediately
	// after IndexRepo returns, killing the goroutine before FTS completes.
	if fr, ok := idx.store.(ftsRebuilder); ok {
		fmt.Fprintf(os.Stderr, "  Rebuilding FTS index...\n")
		ftsStart := time.Now()
		_ = fr.RebuildFTS(context.Background())
		fmt.Fprintf(os.Stderr, "  FTS rebuilt in %s\n", time.Since(ftsStart).Truncate(time.Millisecond))
	}

	// Record edge events for the diff between old and new edges. These
	// events power SnapshotDiff: "removed" events for edges that disappeared
	// and "added" events for edges that are new in this snapshot.
	// Skip on first index (no previous snapshot = no diff to record, and recording
	// 229K "added" events for every edge is wasteful on initial index).
	if hasCleanup && snap != nil && snap.ParentHash != (types.Hash{}) {
		fmt.Fprintf(os.Stderr, "  Recording edge events...\n")
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

// IndexFilesIncremental indexes only the specified files (by relative path).
// This avoids the full directory walk of IndexRepo when the daemon already knows
// which files changed (from git diff or Merkle tree diff). Handles cleanup of
// old nodes/edges for changed files, extraction, batch storage, and snapshot.
func (idx *Indexer) IndexFilesIncremental(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
	if len(changedFiles) == 0 {
		return nil
	}

	repoHash := types.NewHash([]byte(repoURL))

	// Update repo record.
	repo := types.Repo{
		RepoHash:    repoHash,
		RepoURL:     repoURL,
		LastCommit:  commitHash,
		LastIndexed: time.Now().Unix(),
	}
	if err := idx.store.PutRepo(ctx, repo); err != nil {
		return fmt.Errorf("store repo: %w", err)
	}

	moduleToRepo := idx.buildModuleToRepoMap(ctx)
	cs, hasCleanup := idx.store.(cleanupStore)

	var allNodes []types.Node
	var allEdges []types.Edge
	var allFiles []types.File

	for _, relPath := range changedFiles {
		absPath := filepath.Join(repoPath, relPath)

		// Check if file was deleted.
		content, err := os.ReadFile(absPath)
		if err != nil {
			// File deleted: clean up its nodes and edges.
			if hasCleanup {
				if oldFile, err := idx.store.FileByPath(ctx, repoHash, relPath); err == nil && oldFile != nil {
					cs.DeleteEdgesBySourceFile(ctx, oldFile.FileHash) //nolint:errcheck
					cs.DeleteNodesByFile(ctx, oldFile.FileHash)       //nolint:errcheck
				}
			}
			continue
		}

		// Skip if no extractor handles this file.
		if idx.registry.FindExtractor(relPath) == nil {
			continue
		}

		// Skip generated files.
		if isGeneratedContent(content) {
			continue
		}

		contentHash := types.NewHash(content)
		fileHashInput := append(repoHash[:], []byte(relPath)...)
		fileHashInput = append(fileHashInput, contentHash[:]...)
		fileHash := types.NewHash(fileHashInput)

		// Clean up old nodes/edges for this file.
		if hasCleanup {
			if oldFile, err := idx.store.FileByPath(ctx, repoHash, relPath); err == nil && oldFile != nil {
				cs.DeleteEdgesBySourceFile(ctx, oldFile.FileHash) //nolint:errcheck
				cs.DeleteNodesByFile(ctx, oldFile.FileHash)       //nolint:errcheck
			}
		}

		// Extract.
		opts := types.ExtractOptions{
			RepoURL:         repoURL,
			RepoHash:        repoHash,
			CommitHash:      commitHash,
			FilePath:        relPath,
			FileHash:        fileHash,
			Content:         content,
			ModuleRoot:      repoPath,
			ModuleToRepoURL: moduleToRepo,
		}
		result, file, err := idx.extractFile(ctx, opts)
		if err != nil || result == nil {
			continue
		}

		allNodes = append(allNodes, result.Nodes...)
		allEdges = append(allEdges, result.Edges...)
		if file != nil {
			allFiles = append(allFiles, *file)
		}
	}

	// Batch store.
	bs, hasBatch := idx.store.(batchStore)
	if !hasBatch {
		// Fallback to individual puts.
		for _, f := range allFiles {
			_ = idx.store.PutFile(ctx, f)
		}
		for _, n := range allNodes {
			_ = idx.store.PutNode(ctx, n)
		}
		for _, e := range allEdges {
			_ = idx.store.PutEdge(ctx, e)
		}
	} else {
		if len(allFiles) > 0 {
			if err := bs.BatchPutFiles(ctx, allFiles); err != nil {
				return fmt.Errorf("batch put files: %w", err)
			}
		}
		if len(allNodes) > 0 {
			if err := bs.BatchPutNodes(ctx, allNodes); err != nil {
				return fmt.Errorf("batch put nodes: %w", err)
			}
		}
		if len(allEdges) > 0 {
			if err := bs.BatchPutEdges(ctx, allEdges); err != nil {
				return fmt.Errorf("batch put edges: %w", err)
			}
		}
	}

	// Record changed files for consumers.
	idx.lastChangedFiles = make([]string, len(changedFiles))
	copy(idx.lastChangedFiles, changedFiles)

	return nil
}

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

// propagateInheritance finds "extends" edges in the graph and creates
// additional edges from child classes to parent class methods. This allows
// RWR to walk from Flask -> Scaffold.before_request via inheritance.
//
// Uses name-based matching because extends edge target hashes are computed
// with a generic base name (e.g., "Scaffold") that doesn't match the actual
// Scaffold class node's hash (which includes the full file path context).
func propagateInheritance(allNodes []types.Node, allEdges []types.Edge) []types.Edge {
	// Build class terminal name -> class node hash + methods.
	// Terminal name = last component after the last dot in qualified name.
	type classInfo struct {
		hash    types.Hash
		methods []types.Hash // method node hashes
	}
	classByName := make(map[string]*classInfo)

	// First pass: identify class/type nodes and build name -> hash map.
	for i := range allNodes {
		n := &allNodes[i]
		if n.Kind != "type" {
			continue
		}
		name := terminalName(n.QualifiedName)
		if name == "" {
			continue
		}
		classByName[name] = &classInfo{hash: n.NodeHash}
	}

	// Second pass: associate methods with their parent class by name.
	// Method QN: "repo://path/file.py.ClassName.method_name"
	// We extract "ClassName" and find it in classByName.
	for i := range allNodes {
		n := &allNodes[i]
		if n.Kind != "method" {
			continue
		}
		qn := n.QualifiedName
		lastDot := strings.LastIndex(qn, ".")
		if lastDot < 0 {
			continue
		}
		prefix := qn[:lastDot]
		secondLastDot := strings.LastIndex(prefix, ".")
		if secondLastDot < 0 {
			continue
		}
		className := prefix[secondLastDot+1:]

		if ci, ok := classByName[className]; ok {
			ci.methods = append(ci.methods, n.NodeHash)
		}
	}

	// Find all "extends" edges and resolve parent by name.
	// The extends edge source is a child class hash; we look up the child's
	// terminal name, then for each extends edge find which parent class name
	// it targets. Since extends target hash uses a bare name, we need to find
	// the child class node and look at the AST to know the parent name.
	// Simpler approach: for each child class (type node), find ALL classes it
	// could inherit from by looking at extends edges FROM it.

	// Build child hash -> child class name.
	childHashToName := make(map[types.Hash]string)
	for i := range allNodes {
		n := &allNodes[i]
		if n.Kind == "type" {
			childHashToName[n.NodeHash] = terminalName(n.QualifiedName)
		}
	}

	// For each extends edge, find the parent class by trying all class names.
	// The target hash was computed as ComputeNodeHash(repoURL, moduleRoot, empty, baseName, "type")
	// We can't reverse-hash, but we CAN match by checking all known class names.
	// Build target hash -> parent class name candidates.
	targetHashToParent := make(map[types.Hash]string)
	for _, e := range allEdges {
		if e.EdgeType == "extends" {
			targetHashToParent[e.TargetHash] = "" // mark for resolution
		}
	}

	// Try to match each target hash to a known class.
	// Since we can't reverse the hash, we match by scanning: which class node's
	// hash equals the extends target hash?
	for name, ci := range classByName {
		if _, isTarget := targetHashToParent[ci.hash]; isTarget {
			targetHashToParent[ci.hash] = name
		}
	}

	// Create inherits edges: child class -> parent's methods.
	var inheritEdges []types.Edge
	seen := make(map[types.Hash]bool)

	for _, e := range allEdges {
		if e.EdgeType != "extends" {
			continue
		}
		childHash := e.SourceHash

		// Find parent class name from target hash.
		parentName := targetHashToParent[e.TargetHash]
		if parentName == "" {
			// Target hash doesn't match any known class node.
			// This is common for external base classes (Exception, object, etc.)
			continue
		}

		parentInfo, ok := classByName[parentName]
		if !ok || len(parentInfo.methods) == 0 {
			continue
		}

		for _, methodHash := range parentInfo.methods {
			edgeHash := types.ComputeEdgeHash(childHash, methodHash, "inherits", "ast_resolved")
			if seen[edgeHash] {
				continue
			}
			seen[edgeHash] = true
			inheritEdges = append(inheritEdges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: childHash,
				TargetHash: methodHash,
				EdgeType:   "inherits",
				Confidence: 0.9,
				Provenance: "ast_resolved",
			})
		}
	}

	return inheritEdges
}

// generateContainsEdges creates structural "contains" edges from type/class nodes
// to their method/field nodes. This is derived purely from QN structure:
//
//	Type QN:   "repo://path/file.py.ClassName"
//	Method QN: "repo://path/file.py.ClassName.method_name"
//
// If method QN == type QN + "." + something (single level), then type --contains--> method.
//
// This connects 77%+ of type nodes (which are otherwise completely disconnected)
// to their methods, enabling RWR to walk from a type seed to discover all its methods.
func generateContainsEdges(allNodes []types.Node) []types.Edge {
	// Build a map of all type/class/struct/interface node QNs to their hashes.
	typeQNs := make(map[string]types.Hash) // QN -> node hash
	for i := range allNodes {
		n := &allNodes[i]
		switch n.Kind {
		case "type", "class", "struct", "interface", "trait", "module", "object":
			typeQNs[n.QualifiedName] = n.NodeHash
		}
	}

	if len(typeQNs) == 0 {
		return nil
	}

	// For each non-type node, check if stripping its terminal name yields a type QN.
	var edges []types.Edge
	seen := make(map[types.Hash]bool)

	for i := range allNodes {
		n := &allNodes[i]
		// Skip type nodes themselves (they don't contain themselves).
		switch n.Kind {
		case "type", "class", "struct", "interface", "trait", "module", "object":
			continue
		}

		qn := n.QualifiedName
		lastDot := strings.LastIndex(qn, ".")
		if lastDot < 0 {
			continue
		}
		parentQN := qn[:lastDot]

		parentHash, ok := typeQNs[parentQN]
		if !ok {
			continue
		}

		edgeHash := types.ComputeEdgeHash(parentHash, n.NodeHash, edgetype.Contains, "structural")
		if seen[edgeHash] {
			continue
		}
		seen[edgeHash] = true

		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: parentHash,
			TargetHash: n.NodeHash,
			EdgeType:   edgetype.Contains,
			Confidence: 1.0,
			Provenance: "structural",
		})

		// Reverse edge: method -> type (enables RWR to walk from a matched method
		// to its parent type, then to sibling methods).
		revHash := types.ComputeEdgeHash(n.NodeHash, parentHash, edgetype.MemberOf, "structural")
		if !seen[revHash] {
			seen[revHash] = true
			edges = append(edges, types.Edge{
				EdgeHash:   revHash,
				SourceHash: n.NodeHash,
				TargetHash: parentHash,
				EdgeType:   edgetype.MemberOf,
				Confidence: 1.0,
				Provenance: "structural",
			})
		}
	}

	return edges
}

// propagateInterfaceMethods creates "overrides" edges from implementing class methods
// to the corresponding interface methods. When class C implements interface I, and both
// have a method named "Run", we create C.Run --overrides--> I.Run.
// This lets RWR walk from an interface method to all concrete implementations (and back).
func propagateInterfaceMethods(allNodes []types.Node, allEdges []types.Edge) []types.Edge {
	// Build type/interface name -> hash + methods map.
	type typeInfo struct {
		hash    types.Hash
		kind    string
		methods map[string]types.Hash // terminal method name -> hash
	}
	typeByHash := make(map[types.Hash]*typeInfo)
	typeByName := make(map[string]*typeInfo)

	// First pass: collect types and interfaces.
	for i := range allNodes {
		n := &allNodes[i]
		if n.Kind != "type" && n.Kind != "interface" && n.Kind != "class" && n.Kind != "struct" && n.Kind != "trait" {
			continue
		}
		name := terminalName(n.QualifiedName)
		if name == "" {
			continue
		}
		info := &typeInfo{hash: n.NodeHash, kind: n.Kind, methods: make(map[string]types.Hash)}
		typeByHash[n.NodeHash] = info
		typeByName[name] = info
	}

	// Second pass: associate methods with their parent type.
	for i := range allNodes {
		n := &allNodes[i]
		if n.Kind != "method" && n.Kind != "function" {
			continue
		}
		qn := n.QualifiedName
		lastDot := strings.LastIndex(qn, ".")
		if lastDot < 0 {
			continue
		}
		prefix := qn[:lastDot]
		secondLastDot := strings.LastIndex(prefix, ".")
		if secondLastDot < 0 {
			continue
		}
		parentName := prefix[secondLastDot+1:]
		if info, ok := typeByName[parentName]; ok {
			methodName := terminalName(qn)
			info.methods[methodName] = n.NodeHash
		}
	}

	// Find "implements" edges and create overrides for matching method names.
	var overrideEdges []types.Edge
	seen := make(map[types.Hash]bool)

	for _, e := range allEdges {
		if e.EdgeType != "implements" {
			continue
		}
		// implements: source = implementing class, target = interface
		implInfo := typeByHash[e.SourceHash]
		ifaceInfo := typeByHash[e.TargetHash]
		if implInfo == nil || ifaceInfo == nil {
			continue
		}

		// For each interface method, check if the implementing class has a method with the same name.
		for methodName, ifaceMethodHash := range ifaceInfo.methods {
			implMethodHash, ok := implInfo.methods[methodName]
			if !ok {
				continue
			}
			// Create: implMethod --overrides--> ifaceMethod
			edgeHash := types.ComputeEdgeHash(implMethodHash, ifaceMethodHash, edgetype.Overrides, "interface_propagation")
			if seen[edgeHash] {
				continue
			}
			seen[edgeHash] = true
			overrideEdges = append(overrideEdges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: implMethodHash,
				TargetHash: ifaceMethodHash,
				EdgeType:   edgetype.Overrides,
				Confidence: 0.9,
				Provenance: "interface_propagation",
			})
		}
	}

	return overrideEdges
}

// terminalName extracts the last dot-separated component of a qualified name.
// "repo://path/file.py.ClassName.method" -> "method"
// "repo://path/file.py.ClassName" -> "ClassName"
func terminalName(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}

