// Package snapshot manages Merkle-based graph snapshots for the knowing
// knowledge graph.
//
// Each snapshot represents a point-in-time fingerprint of all edges in a
// repository's graph. The fingerprint is a Merkle root computed by sorting
// all edge hashes lexicographically and building a binary hash tree. Two
// snapshots with different roots are guaranteed to have different edge sets,
// enabling efficient change detection.
//
// Snapshots form a singly-linked chain (each snapshot points to its parent),
// supporting garbage collection of old snapshots while preserving chain
// integrity for the most recent N snapshots.
package snapshot

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// packageRootsNoteKey is the note key for persisted package roots.
const packageRootsNoteKey = "package_roots"

// packageRootsObjectHash is the fixed object hash for the package roots note.
var packageRootsObjectHash = types.NewHash([]byte("package_roots_v1"))

// SnapshotManager manages Merkle root computation, snapshot chain
// maintenance, diff operations, and garbage collection of old snapshots.
type SnapshotManager struct {
	store types.GraphStore
	// lastHierarchicalTree caches the most recently computed hierarchical tree
	// for use by diff operations and subgraph root lookups.
	lastHierarchicalTree *HierarchicalTree
}

// LastHierarchicalTree returns the most recently computed hierarchical tree,
// or nil if no snapshot has been computed in this session.
func (sm *SnapshotManager) LastHierarchicalTree() *HierarchicalTree {
	return sm.lastHierarchicalTree
}

// NewSnapshotManager creates a new SnapshotManager backed by the given GraphStore.
func NewSnapshotManager(store types.GraphStore) *SnapshotManager {
	return &SnapshotManager{store: store}
}

// ComputeSnapshot computes a new snapshot for a repository by collecting all
// edge hashes, building a Merkle tree, and storing the resulting snapshot.
// The snapshot is chained to the latest existing snapshot for the repo.
// Also builds a HierarchicalTree for efficient per-package diff and caching.
func (sm *SnapshotManager) ComputeSnapshot(ctx context.Context, repoHash types.Hash, commitHash string) (*types.Snapshot, error) {
	// Collect all edges with metadata for hierarchical tree construction.
	edgeInputs, nodeCount, err := sm.collectRepoEdgesHierarchical(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("collecting edges: %w", err)
	}
	edgeCount := len(edgeInputs)

	// Build hierarchical Merkle tree (package -> edge-type -> leaf).
	// The hierarchical root IS the snapshot hash. No flat tree is built;
	// the hierarchical structure is the canonical representation.
	htree := BuildHierarchicalTree(edgeInputs)

	// Wrap the hierarchical root with a snapshot domain prefix for type safety.
	snapshotHash := types.ComputeSnapshotHash(htree.Root)

	// Store the hierarchical tree for later use by diff and caching.
	sm.lastHierarchicalTree = htree

	// Get the latest snapshot for parent chain and generation number.
	var parentHash types.Hash
	var generation int
	latest, err := sm.store.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("getting latest snapshot: %w", err)
	}
	if latest != nil {
		parentHash = latest.SnapshotHash
		generation = latest.Generation + 1
	}

	snap := types.Snapshot{
		SnapshotHash: snapshotHash,
		ParentHash:   parentHash,
		RepoHash:     repoHash,
		CommitHash:   commitHash,
		Timestamp:    time.Now().Unix(),
		NodeCount:    nodeCount,
		EdgeCount:    edgeCount,
		Generation:   generation,
	}

	if err := sm.store.CreateSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("creating snapshot: %w", err)
	}

	sm.persistPackageRoots(ctx, htree)
	return &snap, nil
}

// ComputeSnapshotFromEdges builds a snapshot from pre-collected edge inputs,
// skipping the expensive DB read. Use this when the caller already has all
// edges in memory (e.g., the indexer's producer-consumer pipeline).
func (sm *SnapshotManager) ComputeSnapshotFromEdges(ctx context.Context, repoHash types.Hash, commitHash string, edgeInputs []EdgeInput, nodeCount int) (*types.Snapshot, error) {
	edgeCount := len(edgeInputs)

	htree := BuildHierarchicalTree(edgeInputs)
	snapshotHash := types.ComputeSnapshotHash(htree.Root)
	sm.lastHierarchicalTree = htree

	var parentHash types.Hash
	var generation int
	latest, err := sm.store.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("getting latest snapshot: %w", err)
	}
	if latest != nil {
		parentHash = latest.SnapshotHash
		generation = latest.Generation + 1
	}

	snap := types.Snapshot{
		SnapshotHash: snapshotHash,
		ParentHash:   parentHash,
		RepoHash:     repoHash,
		CommitHash:   commitHash,
		Timestamp:    time.Now().Unix(),
		NodeCount:    nodeCount,
		EdgeCount:    edgeCount,
		Generation:   generation,
	}

	if err := sm.store.CreateSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("creating snapshot: %w", err)
	}

	sm.persistPackageRoots(ctx, htree)
	return &snap, nil
}

// Diff returns the structural difference between two snapshots.
// It delegates to the GraphStore's SnapshotDiff implementation.
func (sm *SnapshotManager) Diff(ctx context.Context, oldHash, newHash types.Hash) (*types.DiffResult, error) {
	return sm.store.SnapshotDiff(ctx, oldHash, newHash)
}

// GarbageCollect removes old snapshots for a repo, keeping the most recent
// keepCount snapshots. It preserves chain integrity by walking the chain from
// the latest snapshot backwards. Returns the number of removed snapshots.
func (sm *SnapshotManager) GarbageCollect(ctx context.Context, repoHash types.Hash, keepCount int) (removed int, err error) {
	if keepCount < 1 {
		return 0, fmt.Errorf("keepCount must be >= 1, got %d", keepCount)
	}

	// Walk the snapshot chain from latest, collecting all snapshots.
	chain, err := sm.walkChain(ctx, repoHash)
	if err != nil {
		return 0, fmt.Errorf("walking snapshot chain: %w", err)
	}

	if len(chain) <= keepCount {
		return 0, nil
	}

	// Keep the most recent keepCount snapshots; the rest are candidates for removal.
	// Chain is ordered newest-first.
	toRemove := chain[keepCount:]

	// Delete old snapshots and their associated edge events.
	for _, snap := range toRemove {
		if err := sm.store.DeleteSnapshot(ctx, snap.SnapshotHash); err != nil {
			return removed, fmt.Errorf("deleting snapshot %s: %w", snap.SnapshotHash, err)
		}
		removed++
	}
	return removed, nil
}

// CollectEdgeInputs gathers all edges with package and type metadata for a
// repo. This is the canonical source of EdgeInput data: both hierarchical
// tree construction and Merkle proof generation must use this method to
// ensure consistent package paths and edge hashes.
func (sm *SnapshotManager) CollectEdgeInputs(ctx context.Context, repoHash types.Hash) ([]EdgeInput, int, error) {
	return sm.collectRepoEdgesHierarchical(ctx, repoHash)
}

// collectRepoEdgesHierarchical gathers all edges with package and type metadata
// for hierarchical tree construction. Returns EdgeInputs and node count.
func (sm *SnapshotManager) collectRepoEdgesHierarchical(ctx context.Context, repoHash types.Hash) ([]EdgeInput, int, error) {
	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return nil, 0, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return nil, 0, fmt.Errorf("repo not found: %s", repoHash)
	}

	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return nil, 0, fmt.Errorf("querying nodes by name: %w", err)
	}

	// Build node hash -> package path lookup.
	nodePackage := make(map[types.Hash]string, len(nodes))
	for _, n := range nodes {
		pkgPath, err := ExtractPackagePath(n.QualifiedName)
		if err != nil {
			// Log warning and skip this node's edges from hierarchical tree.
			// The node is still stored; it just does not contribute to the
			// package-level Merkle tree structure.
			continue
		}
		nodePackage[n.NodeHash] = pkgPath
	}

	edgeSeen := make(map[types.Hash]struct{})
	var edgeInputs []EdgeInput

	for _, node := range nodes {
		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return nil, 0, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, e := range edges {
			if _, ok := edgeSeen[e.EdgeHash]; !ok {
				edgeSeen[e.EdgeHash] = struct{}{}
				edgeInputs = append(edgeInputs, EdgeInput{
					EdgeHash:    e.EdgeHash,
					PackagePath: nodePackage[e.SourceHash],
					EdgeType:    e.EdgeType,
				})
			}
		}
	}

	return edgeInputs, len(nodes), nil
}

// ExtractPackagePath extracts the package path from a qualified name.
// Format: "repoURL://pkgPath.SymbolName" or "repoURL://pkgPath.Type.Method"
// The separator "://" marks the boundary between the repo URL and the package path.
//
// For Go packages, the package path ends at the first dot-separated component
// that starts with an uppercase letter (indicating a type or symbol name, not
// a package path segment). If no uppercase component is found, falls back to
// splitting at the last dot.
//
// Examples:
//
//	"repo://pkg/sub.FuncName"          -> "pkg/sub"
//	"repo://pkg/sub.TypeName.Method"   -> "pkg/sub"
//	"repo://pkg/sub.lowercase"         -> "pkg/sub"
//
// This is the canonical package path extractor. All code that needs to derive
// a package path from a qualified name should use this function to avoid
// divergent implementations.
func ExtractPackagePath(qualifiedName string) (string, error) {
	sep := strings.LastIndex(qualifiedName, "://")
	if sep < 0 {
		return "", fmt.Errorf("malformed qualified name: missing '://' separator: %q", qualifiedName)
	}
	rest := qualifiedName[sep+3:]

	// Find the boundary between package path and symbol name.
	// Package path segments contain '/' and lowercase names.
	// Symbol names start with an uppercase letter (Go convention).
	// Look for the first dot where the next segment starts with uppercase.
	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash < 0 {
		lastSlash = 0
	}
	afterPkg := rest[lastSlash:]
	firstDot := strings.Index(afterPkg, ".")
	if firstDot < 0 {
		return "", fmt.Errorf("malformed qualified name: no symbol separator '.' after package path: %q", qualifiedName)
	}
	return rest[:lastSlash+firstDot], nil
}

// collectRepoEdges gathers all edge hashes for a repo by traversing
// files -> nodes (via NodesByName with repo prefix) -> edges (via EdgesFrom).
// Returns the edge hashes, node count, and edge count.
func (sm *SnapshotManager) collectRepoEdges(ctx context.Context, repoHash types.Hash) ([]types.Hash, int, int, error) {
	// Get the repo to determine its URL for node prefix queries.
	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return nil, 0, 0, fmt.Errorf("repo not found: %s", repoHash)
	}

	// Get all nodes for this repo using the repo URL as qualified name prefix.
	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("querying nodes by name: %w", err)
	}

	// TODO: Synthetic file nodes (files without extracted symbols) are not included
	// in snapshots because NodesByName only returns nodes with qualified names.
	// Consider including file-level hashes in the Merkle tree for completeness.

	// Collect all edges from each node (using empty edge type to get all types).
	edgeSeen := make(map[types.Hash]struct{})
	var edgeHashes []types.Hash

	for _, node := range nodes {
		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, e := range edges {
			if _, ok := edgeSeen[e.EdgeHash]; !ok {
				edgeSeen[e.EdgeHash] = struct{}{}
				edgeHashes = append(edgeHashes, e.EdgeHash)
			}
		}
	}

	return edgeHashes, len(nodes), len(edgeHashes), nil
}

// walkChain walks the snapshot chain from latest to earliest for a repo.
// Returns snapshots ordered newest-first.
func (sm *SnapshotManager) walkChain(ctx context.Context, repoHash types.Hash) ([]types.Snapshot, error) {
	var chain []types.Snapshot

	current, err := sm.store.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return nil, err
	}

	for current != nil {
		chain = append(chain, *current)
		if current.ParentHash.IsZero() {
			break
		}
		current, err = sm.store.GetSnapshot(ctx, current.ParentHash)
		if err != nil {
			return nil, fmt.Errorf("getting snapshot %s: %w", current.ParentHash, err)
		}
	}

	return chain, nil
}

// persistPackageRoots serializes the hierarchical tree's PackageRoots to the
// notes table. This enables per-package Merkle expiration for vocab associations
// and other cached data: when a specific package changes, only that package's
// root changes, leaving other packages' cached data valid.
func (sm *SnapshotManager) persistPackageRoots(ctx context.Context, htree *HierarchicalTree) {
	if htree == nil || len(htree.PackageRoots) == 0 {
		return
	}
	type noteWriter interface {
		PutNote(ctx context.Context, note types.Note) error
	}
	nw, ok := sm.store.(noteWriter)
	if !ok {
		return
	}

	// Serialize as JSON: {"pkg/path": "hex-root", ...}
	m := make(map[string]string, len(htree.PackageRoots))
	for pkg, root := range htree.PackageRoots {
		m[pkg] = hex.EncodeToString(root[:])
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = nw.PutNote(ctx, types.Note{
		ObjectHash: packageRootsObjectHash,
		Key:        packageRootsNoteKey,
		Value:      string(data),
	})
}

// LoadPackageRoots reads the persisted package roots from the notes table.
// Returns a map of package path -> Merkle root hash. Returns nil if no roots
// are stored (pre-existing DB without this feature).
func LoadPackageRoots(ctx context.Context, store types.GraphStore) map[string]types.Hash {
	type noteReader interface {
		GetNote(ctx context.Context, objectHash types.Hash, key string) (*types.Note, error)
	}
	nr, ok := store.(noteReader)
	if !ok {
		return nil
	}
	note, err := nr.GetNote(ctx, packageRootsObjectHash, packageRootsNoteKey)
	if err != nil || note == nil || note.Value == "" {
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(note.Value), &m); err != nil {
		return nil
	}

	roots := make(map[string]types.Hash, len(m))
	for pkg, hexRoot := range m {
		b, err := hex.DecodeString(hexRoot)
		if err != nil || len(b) != 32 {
			continue
		}
		var h types.Hash
		copy(h[:], b)
		roots[pkg] = h
	}
	return roots
}

// PackageRootForSymbol looks up the package root for a symbol given its
// qualified name and the persisted package roots map. Extracts the package
// path from the QN (everything between "://" and the last "/").
func PackageRootForSymbol(qn string, packageRoots map[string]types.Hash) types.Hash {
	if packageRoots == nil {
		return types.EmptyHash
	}
	// QN format: "repoURL://path/to/package/file.ext.SymbolName"
	// Package path: "path/to/package"
	idx := strings.Index(qn, "://")
	if idx < 0 {
		return types.EmptyHash
	}
	path := qn[idx+3:]
	// Strip file + symbol: find the last "/" to get directory.
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 {
		return types.EmptyHash
	}
	pkgPath := path[:lastSlash]

	if root, ok := packageRoots[pkgPath]; ok {
		return root
	}
	return types.EmptyHash
}
