package community

import (
	"math/rand"

	"github.com/blackwell-systems/knowing/internal/types"
)

// LabelPropagation implements the label propagation community detection algorithm.
// Each node adopts the most common label among its neighbors. Fast and simple
// but non-deterministic (results vary between runs).
type LabelPropagation struct {
	MaxIterations int
}

func (lp *LabelPropagation) Name() string { return "label-propagation" }

func (lp *LabelPropagation) Detect(g *Graph) map[types.Hash]int {
	nodes := g.Nodes
	adj := g.Adj
	maxIter := lp.MaxIterations
	if maxIter <= 0 {
		maxIter = 50
	}

	// Initialize: each node gets its own label.
	label := make(map[types.Hash]int, len(nodes))
	for i, n := range nodes {
		label[n] = i
	}

	// Shuffle order for each iteration (non-deterministic).
	order := make([]int, len(nodes))
	for i := range order {
		order[i] = i
	}

	for iter := 0; iter < maxIter; iter++ {
		changed := false
		rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

		for _, idx := range order {
			node := nodes[idx]
			neighbors := adj[node]
			if len(neighbors) == 0 {
				continue
			}

			// Count weighted votes for each label.
			votes := make(map[int]float64)
			for _, e := range neighbors {
				votes[label[e.Target]] += e.Weight
			}

			// Pick the label with the highest weight.
			bestLabel := label[node]
			bestWeight := 0.0
			for l, w := range votes {
				if w > bestWeight {
					bestWeight = w
					bestLabel = l
				}
			}

			if bestLabel != label[node] {
				label[node] = bestLabel
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	return renumber(nodes, label)
}
