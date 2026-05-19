// Package community provides pluggable graph community detection algorithms.
//
// Each algorithm implements the Algorithm interface. The registry maps string
// names to implementations so callers can select by name (CLI flag, MCP param,
// export option).
//
// To add a new algorithm:
//  1. Create a file (e.g. leiden.go) implementing Algorithm
//  2. Register it in init() or Registry
package community

import "github.com/blackwell-systems/knowing/internal/types"

// WeightedEdge represents a graph edge with a weight (typically confidence).
type WeightedEdge struct {
	Target types.Hash
	Weight float64
}

// Graph holds the adjacency list and node set for community detection.
type Graph struct {
	Nodes     []types.Hash
	Adj       map[types.Hash][]WeightedEdge
	NodeSet   map[types.Hash]bool
	EdgeCount int
}

// Algorithm detects communities in a graph. Returns a map from node hash to
// community ID (int). Community IDs should be contiguous starting from 0.
type Algorithm interface {
	Name() string
	Detect(g *Graph) map[types.Hash]int
}

// Registry maps algorithm names to implementations.
var Registry = map[string]Algorithm{
	"louvain":           &Louvain{Resolution: 1.0, MaxPasses: 20},
	"louvain-fine":      &Louvain{Resolution: 0.3, MaxPasses: 20},
	"label-propagation": &LabelPropagation{MaxIterations: 50},
}

// Default is the algorithm used when none is specified.
const Default = "louvain"

// Get returns the algorithm for the given name, or nil if not found.
func Get(name string) Algorithm {
	return Registry[name]
}

// Names returns all registered algorithm names.
func Names() []string {
	names := make([]string, 0, len(Registry))
	for k := range Registry {
		names = append(names, k)
	}
	return names
}
