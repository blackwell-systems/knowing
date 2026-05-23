// Package testutil provides shared test infrastructure for the knowing project.
package testutil

import (
	"context"

	"github.com/blackwell-systems/knowing/internal/types"
)

// MockGraphStore is a shared test double for types.GraphStore.
// All methods are no-ops returning nil/zero values by default.
// Embed this in test-specific mocks and override only needed methods.
type MockGraphStore struct {
	Nodes     map[types.Hash]*types.Node
	Edges     map[types.Hash]*types.Edge
	Snapshots map[types.Hash]*types.Snapshot
	Repos     map[types.Hash]*types.Repo
	Files     map[types.Hash]*types.File
}

// NewMockGraphStore creates a MockGraphStore with initialized maps.
func NewMockGraphStore() *MockGraphStore {
	return &MockGraphStore{
		Nodes:     make(map[types.Hash]*types.Node),
		Edges:     make(map[types.Hash]*types.Edge),
		Snapshots: make(map[types.Hash]*types.Snapshot),
		Repos:     make(map[types.Hash]*types.Repo),
		Files:     make(map[types.Hash]*types.File),
	}
}

func (m *MockGraphStore) PutNode(_ context.Context, n types.Node) error {
	m.Nodes[n.NodeHash] = &n
	return nil
}

func (m *MockGraphStore) PutEdge(_ context.Context, e types.Edge) error {
	m.Edges[e.EdgeHash] = &e
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

func (m *MockGraphStore) RecordEdgeEvent(_ context.Context, _ types.EdgeEvent) error {
	return nil
}

func (m *MockGraphStore) CreateSnapshot(_ context.Context, _ types.Snapshot) error {
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

func (m *MockGraphStore) NodesByName(_ context.Context, _ string) ([]types.Node, error) {
	return nil, nil
}

func (m *MockGraphStore) EdgesFrom(_ context.Context, _ types.Hash, _ string) ([]types.Edge, error) {
	return nil, nil
}

func (m *MockGraphStore) EdgesTo(_ context.Context, _ types.Hash, _ string) ([]types.Edge, error) {
	return nil, nil
}

func (m *MockGraphStore) DanglingEdges(_ context.Context) ([]types.Edge, error) {
	return nil, nil
}

func (m *MockGraphStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	return nil, nil
}

func (m *MockGraphStore) NodesByQualifiedName(_ context.Context, _ string) ([]types.Node, error) {
	return nil, nil
}

func (m *MockGraphStore) DeleteNodesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) {
	return 0, nil
}

func (m *MockGraphStore) DeleteEdgesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) {
	return 0, nil
}

func (m *MockGraphStore) DeleteEdge(_ context.Context, _ types.Hash) error {
	return nil
}

func (m *MockGraphStore) DeleteNodesByFile(_ context.Context, _ types.Hash) (int, error) {
	return 0, nil
}

func (m *MockGraphStore) DeleteEdgesBySourceFile(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}

func (m *MockGraphStore) EdgesBySourceFile(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}

func (m *MockGraphStore) DeleteSnapshot(_ context.Context, _ types.Hash) error {
	return nil
}

func (m *MockGraphStore) TransitiveCallers(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) {
	return nil, nil
}

func (m *MockGraphStore) TransitiveCallees(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) {
	return nil, nil
}

func (m *MockGraphStore) BlastRadius(_ context.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) {
	return nil, nil
}

func (m *MockGraphStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return nil, nil
}

func (m *MockGraphStore) StaleEdges(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}

func (m *MockGraphStore) LatestSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) {
	return nil, nil
}

func (m *MockGraphStore) FilesByRepo(_ context.Context, _ types.Hash) ([]types.File, error) {
	return nil, nil
}

func (m *MockGraphStore) FileByPath(_ context.Context, _ types.Hash, _ string) (*types.File, error) {
	return nil, nil
}

func (m *MockGraphStore) NodesByFilePath(_ context.Context, _ types.Hash, _ string) ([]types.Node, error) {
	return nil, nil
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
