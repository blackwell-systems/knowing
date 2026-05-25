// Package testutil provides shared test infrastructure for the knowing project.
package testutil

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

// MockGraphStore is a shared test double for types.GraphStore.
// All methods are no-ops returning nil/zero values by default.
// Embed this in test-specific mocks and override only needed methods.
//
// Exported map fields allow tests to pre-populate data for point lookups.
// Override result fields (e.g., NodesByNameResult) control what query methods return.
// Mutation tracking fields (e.g., PutEdges, DeletedEdges) capture write operations.
type MockGraphStore struct {
	// Data maps for point lookups.
	Nodes     map[types.Hash]*types.Node
	Edges     map[types.Hash]*types.Edge
	Snapshots map[types.Hash]*types.Snapshot
	Repos     map[types.Hash]*types.Repo
	Files     map[types.Hash]*types.File

	// Indexed data for query methods.
	NodesByNameResult  map[string][]types.Node
	EdgesFromResult    map[types.Hash][]types.Edge
	EdgesToResult      map[types.Hash][]types.Edge
	FilesByRepoResult  map[types.Hash][]types.File

	// Results for traversal methods.
	TransitiveCallersResult []types.CallerResult
	TransitiveCalleesResult []types.CalleeResult
	BlastRadiusResult       *types.BlastRadiusResult
	SnapshotDiffResult      *types.DiffResult
	StaleEdgesResult        []types.Edge
	LatestSnapshotResult    *types.Snapshot

	// Mutation tracking.
	PutEdges         []types.Edge
	PutNodes         []types.Node
	DeletedEdges     []types.Hash
	CreatedSnapshots []types.Snapshot
	EdgeEvents       []types.EdgeEvent
}

// NewMockGraphStore creates a MockGraphStore with initialized maps.
func NewMockGraphStore() *MockGraphStore {
	return &MockGraphStore{
		Nodes:             make(map[types.Hash]*types.Node),
		Edges:             make(map[types.Hash]*types.Edge),
		Snapshots:         make(map[types.Hash]*types.Snapshot),
		Repos:             make(map[types.Hash]*types.Repo),
		Files:             make(map[types.Hash]*types.File),
		NodesByNameResult: make(map[string][]types.Node),
		EdgesFromResult:   make(map[types.Hash][]types.Edge),
		EdgesToResult:     make(map[types.Hash][]types.Edge),
		FilesByRepoResult: make(map[types.Hash][]types.File),
	}
}

func (m *MockGraphStore) PutNode(_ context.Context, n types.Node) error {
	m.Nodes[n.NodeHash] = &n
	m.PutNodes = append(m.PutNodes, n)
	return nil
}

func (m *MockGraphStore) PutEdge(_ context.Context, e types.Edge) error {
	m.Edges[e.EdgeHash] = &e
	m.PutEdges = append(m.PutEdges, e)
	return nil
}

func (m *MockGraphStore) PutFile(_ context.Context, f types.File) error {
	m.Files[f.FileHash] = &f
	return nil
}

func (m *MockGraphStore) PutRepo(_ context.Context, r types.Repo) error {
	m.Repos[r.RepoHash] = &r
	return nil
}

func (m *MockGraphStore) RecordEdgeEvent(_ context.Context, ev types.EdgeEvent) error {
	m.EdgeEvents = append(m.EdgeEvents, ev)
	return nil
}

func (m *MockGraphStore) CreateSnapshot(_ context.Context, s types.Snapshot) error {
	m.Snapshots[s.SnapshotHash] = &s
	m.CreatedSnapshots = append(m.CreatedSnapshots, s)
	m.LatestSnapshotResult = &s
	return nil
}

func (m *MockGraphStore) GetNode(_ context.Context, hash types.Hash) (*types.Node, error) {
	return m.Nodes[hash], nil
}

func (m *MockGraphStore) GetEdge(_ context.Context, hash types.Hash) (*types.Edge, error) {
	return m.Edges[hash], nil
}

func (m *MockGraphStore) GetSnapshot(_ context.Context, hash types.Hash) (*types.Snapshot, error) {
	return m.Snapshots[hash], nil
}

func (m *MockGraphStore) GetRepo(_ context.Context, hash types.Hash) (*types.Repo, error) {
	return m.Repos[hash], nil
}

func (m *MockGraphStore) NodesByName(_ context.Context, prefix string) ([]types.Node, error) {
	if nodes, ok := m.NodesByNameResult[prefix]; ok {
		return nodes, nil
	}
	return nil, nil
}

func (m *MockGraphStore) EdgesFrom(_ context.Context, sourceHash types.Hash, _ string) ([]types.Edge, error) {
	return m.EdgesFromResult[sourceHash], nil
}

func (m *MockGraphStore) EdgesTo(_ context.Context, targetHash types.Hash, _ string) ([]types.Edge, error) {
	return m.EdgesToResult[targetHash], nil
}

func (m *MockGraphStore) DanglingEdges(_ context.Context) ([]types.Edge, error) {
	var dangling []types.Edge
	for _, e := range m.Edges {
		if _, ok := m.Nodes[e.TargetHash]; !ok {
			dangling = append(dangling, *e)
		}
	}
	return dangling, nil
}

func (m *MockGraphStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	var result []types.Repo
	for _, r := range m.Repos {
		result = append(result, *r)
	}
	return result, nil
}

func (m *MockGraphStore) NodesByQualifiedName(_ context.Context, qualifiedName string) ([]types.Node, error) {
	var result []types.Node
	for _, n := range m.Nodes {
		if n.QualifiedName == qualifiedName {
			result = append(result, *n)
		}
	}
	return result, nil
}

func (m *MockGraphStore) DeleteNodesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) {
	return 0, nil
}

func (m *MockGraphStore) DeleteEdgesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) {
	return 0, nil
}

func (m *MockGraphStore) DeleteEdge(_ context.Context, hash types.Hash) error {
	m.DeletedEdges = append(m.DeletedEdges, hash)
	delete(m.Edges, hash)
	return nil
}

func (m *MockGraphStore) DeleteNodesByFile(_ context.Context, fileHash types.Hash) (int, error) {
	count := 0
	for h, n := range m.Nodes {
		if n.FileHash == fileHash {
			delete(m.Nodes, h)
			count++
		}
	}
	return count, nil
}

func (m *MockGraphStore) DeleteEdgesBySourceFile(_ context.Context, fileHash types.Hash) ([]types.Edge, error) {
	var removed []types.Edge
	for _, e := range m.Edges {
		if src, ok := m.Nodes[e.SourceHash]; ok && src.FileHash == fileHash {
			removed = append(removed, *e)
		}
	}
	for _, e := range removed {
		delete(m.Edges, e.EdgeHash)
	}
	return removed, nil
}

func (m *MockGraphStore) EdgesBySourceFile(_ context.Context, fileHash types.Hash) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.Edges {
		if src, ok := m.Nodes[e.SourceHash]; ok && src.FileHash == fileHash {
			result = append(result, *e)
		}
	}
	return result, nil
}

func (m *MockGraphStore) DeleteSnapshot(_ context.Context, hash types.Hash) error {
	delete(m.Snapshots, hash)
	return nil
}

func (m *MockGraphStore) TransitiveCallers(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) {
	return m.TransitiveCallersResult, nil
}

func (m *MockGraphStore) TransitiveCallees(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) {
	return m.TransitiveCalleesResult, nil
}

func (m *MockGraphStore) BlastRadius(_ context.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) {
	return m.BlastRadiusResult, nil
}

func (m *MockGraphStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return m.SnapshotDiffResult, nil
}

func (m *MockGraphStore) StaleEdges(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return m.StaleEdgesResult, nil
}

func (m *MockGraphStore) LatestSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) {
	return m.LatestSnapshotResult, nil
}

func (m *MockGraphStore) FilesByRepo(_ context.Context, repoHash types.Hash) ([]types.File, error) {
	return m.FilesByRepoResult[repoHash], nil
}

func (m *MockGraphStore) FileByPath(_ context.Context, repoHash types.Hash, path string) (*types.File, error) {
	for _, f := range m.FilesByRepoResult[repoHash] {
		if f.Path == path {
			return &f, nil
		}
	}
	// Also check the Files map.
	for _, f := range m.Files {
		if f.RepoHash == repoHash && f.Path == path {
			return f, nil
		}
	}
	return nil, nil
}

func (m *MockGraphStore) NodesByFilePath(_ context.Context, repoHash types.Hash, path string) ([]types.Node, error) {
	file, _ := m.FileByPath(nil, repoHash, path)
	if file == nil {
		return nil, nil
	}
	var nodes []types.Node
	for _, n := range m.Nodes {
		if n.FileHash == file.FileHash {
			nodes = append(nodes, *n)
		}
	}
	return nodes, nil
}

func (m *MockGraphStore) NodesByFileHash(_ context.Context, fileHash types.Hash) ([]types.Node, error) {
	var nodes []types.Node
	for _, n := range m.Nodes {
		if n.FileHash == fileHash {
			nodes = append(nodes, *n)
		}
	}
	return nodes, nil
}

func (m *MockGraphStore) StaleNodesByFiles(_ context.Context, _ types.Hash, _ []string) ([]types.Node, error) {
	return nil, nil
}

func (m *MockGraphStore) PutNote(_ context.Context, _ types.Note) error {
	return nil
}

func (m *MockGraphStore) GetNote(_ context.Context, _ types.Hash, _ string) (*types.Note, error) {
	return nil, nil
}

func (m *MockGraphStore) GetNotes(_ context.Context, _ types.Hash) ([]types.Note, error) {
	return nil, nil
}

func (m *MockGraphStore) GetNotesByKey(_ context.Context, _ string) ([]types.Note, error) {
	return nil, nil
}

func (m *MockGraphStore) DeleteNote(_ context.Context, _ types.Hash, _ string) error {
	return nil
}

func (m *MockGraphStore) DeleteNotesByObject(_ context.Context, _ types.Hash) error {
	return nil
}

func (m *MockGraphStore) Close() error {
	return nil
}

// Helper methods for test setup (non-interface methods).

// AddNode adds a node to the store for testing.
func (m *MockGraphStore) AddNode(n types.Node) {
	m.Nodes[n.NodeHash] = &n
}

// AddEdge adds an edge to the store for testing.
func (m *MockGraphStore) AddEdge(e types.Edge) {
	m.Edges[e.EdgeHash] = &e
}

// AddRepo adds a repo to the store for testing.
func (m *MockGraphStore) AddRepo(r types.Repo) {
	m.Repos[r.RepoHash] = &r
}

// String implements fmt.Stringer for debugging.
func (m *MockGraphStore) String() string {
	return fmt.Sprintf("MockGraphStore{nodes:%d, edges:%d, repos:%d}",
		len(m.Nodes), len(m.Edges), len(m.Repos))
}
