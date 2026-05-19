package snapshot

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// DanglingClassification describes why an edge target is missing from the
// local database. Only "truly_dangling" indicates potential corruption.
const (
	ClassCrossRepo    = "cross_repo"
	ClassStdlib       = "stdlib"
	ClassTrulyDangling = "truly_dangling"
)

// VerifyError represents a single integrity check finding.
type VerifyError struct {
	Level          string     // "ERROR" or "WARN"
	Kind           string     // "dangling_edge", "hash_mismatch", "broken_chain", "missing_file"
	Hash           types.Hash // hash of the entity with the issue
	Message        string     // human-readable description
	Classification string     // for dangling_edge: "cross_repo", "stdlib", or "truly_dangling"
}

// stdlibPackages is the set of Go standard library top-level package names.
// Used to classify dangling edges that target stdlib symbols.
var stdlibPackages = map[string]bool{
	"archive": true, "bufio": true, "bytes": true, "cmp": true,
	"compress": true, "container": true, "context": true, "crypto": true,
	"database": true, "debug": true, "embed": true, "encoding": true,
	"errors": true, "expvar": true, "flag": true, "fmt": true,
	"go": true, "hash": true, "html": true, "image": true,
	"index": true, "io": true, "iter": true, "log": true,
	"maps": true, "math": true, "mime": true, "net": true,
	"os": true, "path": true, "plugin": true, "reflect": true,
	"regexp": true, "runtime": true, "slices": true, "sort": true,
	"strconv": true, "strings": true, "structs": true, "sync": true,
	"syscall": true, "testing": true, "text": true, "time": true,
	"unicode": true, "unsafe": true, "builtin": true,
}

// isStdlibTarget returns true if the edge likely targets a standard library
// or builtin symbol. It checks the edge provenance and attempts to infer
// the target's package from any available context.
func isStdlibTarget(edge types.Edge) bool {
	// Edges to "stdlib" repo URL are always stdlib.
	// We check this via the target hash by looking at known patterns.
	// Since we don't have the target node (it's dangling), we rely on
	// the edge type and any metadata hints.

	// Import and throws edges to builtin/stdlib targets. Since the target
	// node is dangling (not in any DB), we infer from the edge's callsite
	// file path. If the edge has no callsite info, fall back to edge type
	// only for throws (which always target error types).
	if edge.EdgeType == "throws" {
		return true // throws targets are always builtin error types
	}

	// For imports, check if the import target looks like a stdlib path.
	// We can't check the target node (it's dangling), but the callsite_file
	// gives us the import path for some extractors.
	if edge.EdgeType == "imports" && edge.CallSiteFile != "" {
		firstSlash := strings.Index(edge.CallSiteFile, "/")
		topPkg := edge.CallSiteFile
		if firstSlash > 0 {
			topPkg = edge.CallSiteFile[:firstSlash]
		}
		return stdlibPackages[topPkg]
	}

	// References edges to missing targets in non-indexed repos.
	if edge.EdgeType == "references" || edge.EdgeType == "imports" {
		// Conservative: if we can't determine stdlib, don't assume it.
		return false
	}

	return false
}

// rosterNodeLookup holds pre-opened stores for cross-repo node lookups.
// Created once per Verify call, closed when done.
type rosterNodeLookup struct {
	stores []*store.SQLiteStore
}

func newRosterNodeLookup(ctx context.Context, rosterDBPaths []string, localDBPath string) *rosterNodeLookup {
	rl := &rosterNodeLookup{}
	for _, dbPath := range rosterDBPaths {
		if dbPath == localDBPath {
			continue
		}
		st, err := store.NewSQLiteStore(dbPath)
		if err != nil {
			continue
		}
		rl.stores = append(rl.stores, st)
	}
	return rl
}

func (rl *rosterNodeLookup) close() {
	for _, st := range rl.stores {
		st.Close()
	}
}

func (rl *rosterNodeLookup) existsInRoster(ctx context.Context, hash types.Hash) bool {
	for _, st := range rl.stores {
		node, err := st.GetNode(ctx, hash)
		if err == nil && node != nil {
			return true
		}
	}
	return false
}

// classifyDanglingEdge determines whether a dangling edge target is in
// another roster database, is a stdlib reference, or is truly dangling.
func classifyDanglingEdge(ctx context.Context, edge types.Edge, targetHash types.Hash, rosterLookup *rosterNodeLookup) string {
	// Check if the target exists in any other roster database first.
	if rosterLookup != nil && rosterLookup.existsInRoster(ctx, targetHash) {
		return ClassCrossRepo
	}

	// Stdlib/builtin heuristics for edges whose targets are not in any
	// roster database (since stdlib is never indexed).
	if isStdlibTarget(edge) {
		return ClassStdlib
	}

	return ClassTrulyDangling
}

// Verify performs integrity verification on a repo's graph.
// Checks: edge referential integrity, hash recomputation, snapshot chain continuity.
//
// When rosterDBPaths is non-empty, dangling edges are classified as cross_repo,
// stdlib, or truly_dangling. When empty, all dangling edges are truly_dangling.
// localDBPath identifies this store's database so it is skipped in roster lookups.
func (sm *SnapshotManager) Verify(ctx context.Context, repoHash types.Hash, rosterDBPaths []string, localDBPath string) ([]VerifyError, error) {
	var errs []VerifyError

	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return nil, fmt.Errorf("repo not found: %s", repoHash)
	}

	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("querying nodes by name: %w", err)
	}

	// Open roster databases once for cross-repo lookups.
	rosterLookup := newRosterNodeLookup(ctx, rosterDBPaths, localDBPath)
	defer rosterLookup.close()

	// Check edge referential integrity and hash recomputation.
	edgeSeen := make(map[types.Hash]struct{})
	for _, node := range nodes {
		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return nil, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, edge := range edges {
			if _, ok := edgeSeen[edge.EdgeHash]; ok {
				continue
			}
			edgeSeen[edge.EdgeHash] = struct{}{}

			// a. Edge referential integrity: verify source and target nodes exist.
			src, err := sm.store.GetNode(ctx, edge.SourceHash)
			if err != nil {
				return nil, fmt.Errorf("getting source node: %w", err)
			}
			if src == nil {
				classification := classifyDanglingEdge(ctx, edge, edge.SourceHash, rosterLookup)
				errs = append(errs, VerifyError{
					Level:          levelForClassification(classification),
					Kind:           "dangling_edge",
					Hash:           edge.EdgeHash,
					Message:        fmt.Sprintf("edge %s references non-existent source node %s", edge.EdgeHash, edge.SourceHash),
					Classification: classification,
				})
			}

			tgt, err := sm.store.GetNode(ctx, edge.TargetHash)
			if err != nil {
				return nil, fmt.Errorf("getting target node: %w", err)
			}
			if tgt == nil {
				classification := classifyDanglingEdge(ctx, edge, edge.TargetHash, rosterLookup)
				errs = append(errs, VerifyError{
					Level:          levelForClassification(classification),
					Kind:           "dangling_edge",
					Hash:           edge.EdgeHash,
					Message:        fmt.Sprintf("edge %s references non-existent target node %s (%s)", edge.EdgeHash, edge.TargetHash, classification),
					Classification: classification,
				})
			}

			// b. Hash recomputation (edges).
			if hashErr := types.VerifyEdgeHash(edge); hashErr != nil {
				errs = append(errs, VerifyError{
					Level:   "ERROR",
					Kind:    "hash_mismatch",
					Hash:    edge.EdgeHash,
					Message: hashErr.Error(),
				})
			}
		}

		// c. Node hash verification.
		pkgPath, pkgErr := ExtractPackagePath(node.QualifiedName)
		if pkgErr == nil {
			if hashErr := types.VerifyNodeHash(node, repo.RepoURL, pkgPath); hashErr != nil {
				errs = append(errs, VerifyError{
					Level:   "WARN",
					Kind:    "hash_mismatch",
					Hash:    node.NodeHash,
					Message: hashErr.Error(),
				})
			}
		}
	}

	// d. Snapshot chain continuity.
	chain, err := sm.walkChain(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("walking snapshot chain: %w", err)
	}

	// Build a set of known snapshot hashes.
	snapshotSet := make(map[types.Hash]struct{}, len(chain))
	for _, snap := range chain {
		snapshotSet[snap.SnapshotHash] = struct{}{}
	}

	for _, snap := range chain {
		if snap.ParentHash.IsZero() {
			continue
		}
		parent, err := sm.store.GetSnapshot(ctx, snap.ParentHash)
		if err != nil {
			return nil, fmt.Errorf("getting parent snapshot: %w", err)
		}
		if parent == nil {
			errs = append(errs, VerifyError{
				Level:   "ERROR",
				Kind:    "broken_chain",
				Hash:    snap.SnapshotHash,
				Message: fmt.Sprintf("snapshot %s parent %s not found", snap.SnapshotHash, snap.ParentHash),
			})
		}
	}

	return errs, nil
}

// levelForClassification returns the error level for a dangling edge classification.
// Only truly_dangling edges are errors; cross_repo and stdlib are informational warnings.
func levelForClassification(classification string) string {
	switch classification {
	case ClassCrossRepo, ClassStdlib:
		return "INFO"
	default:
		return "ERROR"
	}
}

// isStdlibPackagePath returns true if the given package path starts with a
// known Go standard library package prefix.
func isStdlibPackagePath(pkgPath string) bool {
	if pkgPath == "" {
		return false
	}
	parts := strings.SplitN(pkgPath, "/", 2)
	return stdlibPackages[parts[0]]
}
