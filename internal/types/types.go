// Package types defines the core domain model for the knowing knowledge graph.
//
// All entities (nodes, edges, files, repos, snapshots) are content-addressed
// using SHA-256 hashes, enabling deterministic identity, deduplication, and
// Merkle-based snapshot diffing. The hash functions in this package define
// the canonical identity computations used throughout the system.
//
// Key types:
//   - Node: a symbol (function, type, method, etc.) in the knowledge graph
//   - Edge: a relationship (calls, imports, implements, references) between nodes
//   - File and Repo: tracked source artifacts
//   - Snapshot: a point-in-time Merkle root over all edges in a repo
//   - EdgeEvent: an append-only event for event-sourced diff tracking
//
// The GraphStore interface (interfaces.go) and Extractor interface define
// the contracts that concrete implementations must satisfy.
package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Hash is a content-addressed identifier (SHA-256 digest, 32 bytes).
// Used as the primary key for all graph entities: nodes, edges, files,
// repos, and snapshots. Two entities with identical content always
// produce the same Hash.
type Hash [32]byte

// EmptyHash is the zero-value hash.
var EmptyHash Hash

// NewHash computes a SHA-256 hash from the given data.
func NewHash(data []byte) Hash {
	return sha256.Sum256(data)
}

// String returns the hex-encoded hash.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// IsZero returns true if the hash is the zero value.
func (h Hash) IsZero() bool {
	return h == EmptyHash
}

// Node represents a symbol in the knowledge graph. A node is a function,
// type, method, interface, const, or var declaration extracted from source
// code. Nodes are identified by a content-addressed hash computed from
// (repo, package, name, kind), so two nodes in different files with the
// same qualified identity will share a hash.
type Node struct {
	// NodeHash is the content-addressed identity: sha256(repoURL || packagePath || symbolName || symbolKind).
	// Note: contentHash was removed from the computation; node identity
	// depends only on (repo, package, name, kind).
	NodeHash      Hash
	FileHash      Hash   // reference to the containing File record
	QualifiedName string // fully qualified name: "{repoURL}://{pkgPath}.{TypeName}.{SymbolName}"
	Kind          string // one of: function, type, method, interface, const, var
	Line          int    // 1-indexed source line number of the declaration
	Signature     string // human-readable type signature for display (e.g., "func (SQLiteStore) PutNode()")
	Doc           string // doc comment preceding the declaration (first 200 chars, language-agnostic)
	LastAuthor    string  // git blame: author of the last commit that touched this symbol's line
	LastCommitAt  int64   // git blame: unix timestamp of the last commit that touched this symbol's line
	CoveragePct   float64 // test coverage percentage for this symbol's lines (0.0-100.0, -1 = not measured)
}

// Edge represents a directed relationship between two nodes in the knowledge
// graph. Edge types include "calls", "imports", "implements", and "references".
// Each edge carries a confidence score and provenance tag indicating how it
// was derived (ast_resolved, ast_inferred, lsp_resolved, etc.).
type Edge struct {
	EdgeHash   Hash    // content-addressed identity: sha256(sourceHash || targetHash || edgeType || provenance)
	SourceHash Hash    // hash of the source node (caller, importer, implementor)
	TargetHash Hash    // hash of the target node (callee, imported package, interface)
	EdgeType   string  // relationship kind: "calls", "imports", "implements", "references"
	Confidence float64 // quality score from 0.0 to 1.0; ast_inferred=0.7, lsp_resolved=0.9, ast_resolved=1.0
	Provenance string  // how the edge was derived: "ast_resolved", "ast_inferred", "lsp_resolved", etc.
	// CallSite fields store the source location of the call expression (not the
	// target declaration). These positions are used by LSP enrichment: the enricher
	// sends GetDefinition at (CallSiteFile, CallSiteLine, CallSiteCol) to confirm
	// or correct the target. Zero values mean no call-site info is available.
	CallSiteLine int    // 1-indexed line of the call expression in the source file
	CallSiteCol  int    // 0-indexed column of the call expression
	CallSiteFile string // relative file path (within the repo) containing the call expression
	// Runtime observation fields. Zero values for static edges.
	ObservationCount int   // total observations in current window (0 for static edges)
	LastObserved     int64 // unix timestamp of last observation (0 for static edges)
}

// File represents a tracked source file within a repository. The FileHash
// incorporates the repo, path, and content, so a file's identity changes
// whenever its content changes (enabling content-based change detection).
type File struct {
	FileHash    Hash   // sha256(repoHash || relativePath || contentHash)
	RepoHash    Hash   // hash of the containing Repo
	Path        string // path relative to the repository root
	ContentHash Hash   // sha256 of the raw file contents; used for skip-if-unchanged checks
}

// Repo represents a tracked repository. The RepoURL can be either a remote
// URL (e.g., "github.com/org/repo") or a local filesystem path, depending
// on how the repo was registered.
type Repo struct {
	RepoHash    Hash   // sha256(repoURL); canonical identity for the repo
	RepoURL     string // the URL or path that was passed to IndexRepo
	LastCommit  string // git commit hash from the most recent index run
	LastIndexed int64  // unix timestamp of the most recent index run
}

// Snapshot represents a point-in-time graph state for a single repository.
// The SnapshotHash is the Merkle root computed over all sorted edge hashes,
// providing a tamper-evident fingerprint of the entire graph at a given commit.
// Snapshots form a singly-linked chain via ParentHash, enabling efficient
// diffing and garbage collection.
type Snapshot struct {
	SnapshotHash Hash   // Merkle root: merkle_root(sorted(all_edge_hashes))
	ParentHash   Hash   // hash of the previous snapshot in the chain; zero for the first
	RepoHash     Hash   // hash of the repository this snapshot belongs to
	CommitHash   string // git commit hash at the time of snapshotting
	Timestamp    int64  // unix timestamp when the snapshot was created
	NodeCount    int    // total number of nodes in the graph at snapshot time
	EdgeCount    int    // total number of edges in the graph at snapshot time
}

// EdgeEvent represents an append-only edge mutation event for event sourcing.
// Each time an edge is added or removed during an index run, an EdgeEvent is
// recorded. These events power the SnapshotDiff query by tracking which edges
// changed between snapshots.
type EdgeEvent struct {
	EventID      int64  // auto-increment primary key
	EdgeHash     Hash   // hash of the edge that was added or removed
	EventType    string // "added" or "removed"
	SnapshotHash Hash   // the snapshot during which this event occurred
	SourceCommit string // git commit that triggered the event
	IndexerVer   string // version of the indexer that produced this event (e.g., "v1")
	Timestamp    int64  // unix timestamp of the event
}

// EdgeProvenance captures the full derivation history of an edge.
// Used in BlastRadiusResult to show the provenance chain from a caller
// back to the target, so consumers can assess trustworthiness.
type EdgeProvenance struct {
	Source         string  // derivation method: "ast_resolved", "ast_inferred", "lsp_resolved", etc.
	Confidence     float64 // confidence score of this provenance step (0.0 to 1.0)
	IndexerVersion string  // version of the indexer that produced this edge
	SourceCommit   string  // git commit hash at the time of extraction
	SourceFileHash Hash    // hash of the source file from which the edge was extracted
	Timestamp      int64   // unix timestamp of extraction
}

// ComputeNodeHash computes the content-addressed hash for a node.
// The contentHash parameter is accepted for API compatibility but is
// not included in the hash computation. Node identity depends on
// (repo, package, name, kind) only.
//
// The hash formula is: SHA-256("node" + NUL + repoURL + NUL + packagePath + NUL + symbolName + NUL + symbolKind).
// The "node\0" domain prefix distinguishes node hashes from edge, snapshot,
// and Merkle interior node hashes, preventing cross-domain hash collisions.
// NUL bytes are used as field separators to prevent ambiguous concatenation
// (e.g., "a/b" + "c" vs "a" + "b/c").
//
// WARNING: This formula changed to include the "node\0" prefix. Existing
// databases must be re-indexed.
func ComputeNodeHash(repoURL, packagePath string, _ Hash, symbolName, symbolKind string) Hash {
	data := fmt.Sprintf("node\x00%s\x00%s\x00%s\x00%s", repoURL, packagePath, symbolName, symbolKind)
	return NewHash([]byte(data))
}

// ComputeEdgeHash computes the content-addressed hash for an edge.
// The hash formula is: SHA-256("edge" + NUL + sourceHash + NUL + targetHash + NUL + edgeType + NUL + provenance).
// The "edge\0" domain prefix distinguishes edge hashes from node, snapshot,
// and Merkle interior node hashes, preventing cross-domain hash collisions.
// Because provenance is included, upgrading an edge from "ast_inferred" to
// "lsp_resolved" produces a new hash (the old edge must be deleted first).
//
// WARNING: This formula changed to include the "edge\0" prefix. Existing
// databases must be re-indexed.
func ComputeEdgeHash(sourceHash, targetHash Hash, edgeType, provenanceJSON string) Hash {
	data := fmt.Sprintf("edge\x00%s\x00%s\x00%s\x00%s", sourceHash, targetHash, edgeType, provenanceJSON)
	return NewHash([]byte(data))
}

// ComputeSnapshotHash wraps a Merkle root hash with a "snapshot" domain
// prefix, distinguishing snapshot identity from raw Merkle interior nodes.
func ComputeSnapshotHash(merkleRoot Hash) Hash {
	data := append([]byte("snapshot\x00"), merkleRoot[:]...)
	return NewHash(data)
}

// ComputeMerkleNodeHash computes a Merkle interior node hash with a "merkle"
// domain prefix. This distinguishes interior tree nodes from leaf hashes
// and snapshot root hashes.
func ComputeMerkleNodeHash(left, right Hash) Hash {
	var buf [71]byte
	copy(buf[:7], "merkle\x00")
	copy(buf[7:39], left[:])
	copy(buf[39:71], right[:])
	return NewHash(buf[:])
}
