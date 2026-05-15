package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Hash is a content-addressed identifier (SHA-256).
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

// Node represents a symbol in the knowledge graph.
type Node struct {
	NodeHash      Hash   // sha256(repo || package_path || content_hash || symbol_name || symbol_kind)
	FileHash      Hash   // reference to containing file
	QualifiedName string // {repo}://{module_path}/{package_path}.{TypeName}.{MemberName}
	Kind          string // function, type, method, interface, const, var
	Line          int    // source line number
	Signature     string // type signature for display
}

// Edge represents a relationship between two nodes.
type Edge struct {
	EdgeHash   Hash    // sha256(source_node_hash || target_node_hash || edge_type || provenance_json)
	SourceHash Hash    // source node hash
	TargetHash Hash    // target node hash
	EdgeType   string  // calls, imports, implements, references, etc.
	Confidence float64 // 0.0 to 1.0
	Provenance string  // provenance source (ast_resolved, scip_imported, etc.)
}

// File represents a tracked source file.
type File struct {
	FileHash    Hash   // sha256(repo_hash || path || content_hash)
	RepoHash    Hash   // reference to containing repo
	Path        string // relative path within repo
	ContentHash Hash   // sha256(file_contents)
}

// Repo represents a tracked repository.
type Repo struct {
	RepoHash    Hash   // sha256(repo_url)
	RepoURL     string
	LastCommit  string
	LastIndexed int64  // unix timestamp
}

// Snapshot represents a point-in-time graph state (Merkle root).
type Snapshot struct {
	SnapshotHash Hash   // merkle_root(sorted(all_edge_hashes))
	ParentHash   Hash   // previous snapshot in chain
	RepoHash     Hash   // which repo this snapshot is for
	CommitHash   string // git commit hash
	Timestamp    int64  // unix timestamp
	NodeCount    int
	EdgeCount    int
}

// EdgeEvent represents an append-only edge event (event sourcing).
type EdgeEvent struct {
	EventID      int64
	EdgeHash     Hash
	EventType    string // "added" or "removed"
	SnapshotHash Hash
	SourceCommit string
	IndexerVer   string
	Timestamp    int64
}

// EdgeProvenance captures how an edge was derived.
type EdgeProvenance struct {
	Source         string  // ast_resolved, scip_imported, runtime_trace, etc.
	Confidence     float64
	IndexerVersion string
	SourceCommit   string
	SourceFileHash Hash
	Timestamp      int64
}

// ComputeNodeHash computes the content-addressed hash for a node.
// The contentHash parameter is accepted for API compatibility but is
// not included in the hash computation. Node identity depends on
// (repo, package, name, kind) only.
func ComputeNodeHash(repoURL, packagePath string, _ Hash, symbolName, symbolKind string) Hash {
	data := fmt.Sprintf("%s\x00%s\x00%s\x00%s", repoURL, packagePath, symbolName, symbolKind)
	return NewHash([]byte(data))
}

// ComputeEdgeHash computes the content-addressed hash for an edge.
func ComputeEdgeHash(sourceHash, targetHash Hash, edgeType, provenanceJSON string) Hash {
	data := fmt.Sprintf("%s\x00%s\x00%s\x00%s", sourceHash, targetHash, edgeType, provenanceJSON)
	return NewHash([]byte(data))
}
