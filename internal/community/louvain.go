package community

import "github.com/blackwell-systems/knowing/internal/types"

// Louvain implements single-pass Louvain modularity optimization.
// Resolution controls granularity: lower values produce fewer, larger communities.
type Louvain struct {
	Resolution float64
	MaxPasses  int
}

func (l *Louvain) Name() string { return "louvain" }

func (l *Louvain) Detect(g *Graph) map[types.Hash]int {
	nodes := g.Nodes
	adj := g.Adj
	resolution := l.Resolution
	if resolution <= 0 {
		resolution = 1.0
	}
	maxPasses := l.MaxPasses
	if maxPasses <= 0 {
		maxPasses = 20
	}

	// Initialize: each node in its own community.
	comm := make(map[types.Hash]int, len(nodes))
	for i, n := range nodes {
		comm[n] = i
	}

	// Compute total edge weight (adj is undirected, each edge stored twice).
	var twoM float64
	for _, edges := range adj {
		for _, e := range edges {
			twoM += e.Weight
		}
	}
	if twoM == 0 {
		return comm
	}
	m := twoM / 2.0

	// Node strengths (weighted degree).
	ki := make(map[types.Hash]float64, len(nodes))
	for _, n := range nodes {
		for _, e := range adj[n] {
			ki[n] += e.Weight
		}
	}

	// sigma_tot per community.
	sigmaTot := make(map[int]float64, len(nodes))
	for _, n := range nodes {
		sigmaTot[comm[n]] += ki[n]
	}

	// Iterate until no improvement.
	improved := true
	for pass := 0; improved && pass < maxPasses; pass++ {
		improved = false
		for _, node := range nodes {
			currentComm := comm[node]
			bestComm := currentComm
			bestGain := 0.0

			// Weight from node to each neighboring community.
			kiIn := make(map[int]float64)
			for _, e := range adj[node] {
				kiIn[comm[e.Target]] += e.Weight
			}

			sigCurr := sigmaTot[currentComm] - ki[node]
			kiInCurr := kiIn[currentComm]

			for c, w := range kiIn {
				if c == currentComm {
					continue
				}
				sigC := sigmaTot[c]

				// Standard Louvain gain with resolution parameter.
				gainAdd := w/m - resolution*(sigC*ki[node])/(2*m*m)
				gainRemove := kiInCurr/m - resolution*(sigCurr*ki[node])/(2*m*m)
				gain := gainAdd - gainRemove

				if gain > bestGain {
					bestGain = gain
					bestComm = c
				}
			}

			if bestComm != currentComm {
				sigmaTot[currentComm] -= ki[node]
				sigmaTot[bestComm] += ki[node]
				comm[node] = bestComm
				improved = true
			}
		}
	}

	// Renumber to contiguous IDs.
	return renumber(nodes, comm)
}

// DetectIncremental runs Louvain seeded from previous community assignments.
// Only nodes in changedNodes are allowed to move; all other nodes keep their
// previous assignment. Falls back to full Detect if previous is nil/empty.
func (l *Louvain) DetectIncremental(g *Graph, previous map[types.Hash]int, changedNodes map[types.Hash]bool) map[types.Hash]int {
	if len(previous) == 0 {
		return l.Detect(g)
	}

	nodes := g.Nodes
	adj := g.Adj
	resolution := l.Resolution
	if resolution <= 0 {
		resolution = 1.0
	}
	maxPasses := l.MaxPasses
	if maxPasses <= 0 {
		maxPasses = 20
	}

	// Seed: use previous assignments where available, assign new IDs for new nodes.
	comm := make(map[types.Hash]int, len(nodes))
	maxID := 0
	for _, id := range previous {
		if id >= maxID {
			maxID = id + 1
		}
	}
	for _, n := range nodes {
		if prevID, ok := previous[n]; ok {
			comm[n] = prevID
		} else {
			comm[n] = maxID
			maxID++
		}
	}

	// Compute total edge weight.
	var twoM float64
	for _, edges := range adj {
		for _, e := range edges {
			twoM += e.Weight
		}
	}
	if twoM == 0 {
		return renumber(nodes, comm)
	}
	m := twoM / 2.0

	// Node strengths.
	ki := make(map[types.Hash]float64, len(nodes))
	for _, n := range nodes {
		for _, e := range adj[n] {
			ki[n] += e.Weight
		}
	}

	// sigma_tot per community.
	sigmaTot := make(map[int]float64, len(nodes))
	for _, n := range nodes {
		sigmaTot[comm[n]] += ki[n]
	}

	// Iterate, but only move changedNodes.
	improved := true
	for pass := 0; improved && pass < maxPasses; pass++ {
		improved = false
		for _, node := range nodes {
			if !changedNodes[node] {
				continue // frozen
			}

			currentComm := comm[node]
			bestComm := currentComm
			bestGain := 0.0

			kiIn := make(map[int]float64)
			for _, e := range adj[node] {
				kiIn[comm[e.Target]] += e.Weight
			}

			sigCurr := sigmaTot[currentComm] - ki[node]
			kiInCurr := kiIn[currentComm]

			for c, w := range kiIn {
				if c == currentComm {
					continue
				}
				sigC := sigmaTot[c]
				gainAdd := w/m - resolution*(sigC*ki[node])/(2*m*m)
				gainRemove := kiInCurr/m - resolution*(sigCurr*ki[node])/(2*m*m)
				gain := gainAdd - gainRemove

				if gain > bestGain {
					bestGain = gain
					bestComm = c
				}
			}

			if bestComm != currentComm {
				sigmaTot[currentComm] -= ki[node]
				sigmaTot[bestComm] += ki[node]
				comm[node] = bestComm
				improved = true
			}
		}
	}

	return renumber(nodes, comm)
}

// renumber reassigns community IDs to be contiguous starting from 0.
func renumber(nodes []types.Hash, comm map[types.Hash]int) map[types.Hash]int {
	remap := make(map[int]int)
	nextID := 0
	for _, n := range nodes {
		c := comm[n]
		if _, ok := remap[c]; !ok {
			remap[c] = nextID
			nextID++
		}
		comm[n] = remap[c]
	}
	return comm
}
