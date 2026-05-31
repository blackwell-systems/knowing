package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdDebugWalk shows the RWR walk from specific seed nodes: edge traversal,
// score distribution, and final top-N ranking.
// Usage: knowing debug-walk -seed "NodeName" [-db path] [-top N] [-alpha 0.2]
func cmdDebugWalk(args []string) error {
	fs := flag.NewFlagSet("debug-walk", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database")
	seedName := fs.String("seed", "", "Seed symbol name or prefix (required)")
	topN := fs.Int("top", 20, "Number of top results to show")
	alpha := fs.Float64("alpha", 0.2, "RWR restart probability")
	maxIter := fs.Int("iter", 20, "Max RWR iterations")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *seedName == "" {
		return fmt.Errorf("usage: knowing debug-walk -seed \"SymbolName\" [-db path] [-top N]")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Find seed nodes by name prefix.
	nodes, err := st.NodesByName(ctx, "%"+*seedName)
	if err != nil {
		return fmt.Errorf("searching for seed: %w", err)
	}

	// Filter to exact or close matches (check both terminal name and full QN).
	var seeds []types.Node
	seedLower := strings.ToLower(*seedName)
	for _, n := range nodes {
		sym := terminalSymbol(n.QualifiedName)
		qnLower := strings.ToLower(n.QualifiedName)
		if strings.EqualFold(sym, *seedName) ||
			strings.HasPrefix(strings.ToLower(sym), seedLower) ||
			strings.Contains(qnLower, "."+seedLower) ||
			strings.HasSuffix(qnLower, seedLower) {
			seeds = append(seeds, n)
		}
		if len(seeds) >= 5 {
			break
		}
	}

	if len(seeds) == 0 {
		fmt.Printf("No nodes found matching %q\n", *seedName)
		if len(nodes) > 0 {
			fmt.Printf("\nDid you mean:\n")
			for i, n := range nodes {
				if i >= 10 {
					break
				}
				fmt.Printf("  %s\n", terminalSymbol(n.QualifiedName))
			}
		}
		return nil
	}

	fmt.Printf("=== WALK DEBUG ===\n")
	fmt.Printf("Seeds: %d nodes matching %q\n", len(seeds), *seedName)
	fmt.Printf("Alpha: %.2f  MaxIter: %d\n", *alpha, *maxIter)
	fmt.Printf("\n")

	// Show seed nodes.
	seedHashes := make([]types.Hash, len(seeds))
	fmt.Printf("--- Seeds ---\n")
	for i, s := range seeds {
		seedHashes[i] = s.NodeHash
		fmt.Printf("  [%d] %s (%s)\n", i+1, terminalSymbol(s.QualifiedName), s.Kind)
	}
	fmt.Printf("\n")

	// Show edges from seeds (1-hop).
	fmt.Printf("--- Seed Edges (1-hop) ---\n")
	for _, s := range seeds {
		outEdges, _ := st.EdgesFrom(ctx, s.NodeHash, "")
		inEdges, _ := st.EdgesTo(ctx, s.NodeHash, "")
		edgeTypes := make(map[string]int)
		for _, e := range outEdges {
			edgeTypes[e.EdgeType+" (out)"]++
		}
		for _, e := range inEdges {
			edgeTypes[e.EdgeType+" (in)"]++
		}
		fmt.Printf("  %s: %d outgoing, %d incoming\n", terminalSymbol(s.QualifiedName), len(outEdges), len(inEdges))
		// Show edge type breakdown.
		type etCount struct {
			et    string
			count int
		}
		var ets []etCount
		for et, c := range edgeTypes {
			ets = append(ets, etCount{et, c})
		}
		sort.Slice(ets, func(i, j int) bool { return ets[i].count > ets[j].count })
		for _, et := range ets {
			fmt.Printf("    %s: %d\n", et.et, et.count)
		}
	}
	fmt.Printf("\n")

	// Run RWR.
	fmt.Printf("--- RWR Walk ---\n")
	scores, err := knowingctx.RandomWalkWithRestart(ctx, st, seedHashes, *alpha, *maxIter)
	if err != nil {
		return fmt.Errorf("RWR failed: %w", err)
	}

	fmt.Printf("  Nodes reached: %d\n", len(scores))

	// Sort by score.
	type scored struct {
		hash  types.Hash
		score float64
	}
	var ranked []scored
	for h, s := range scores {
		ranked = append(ranked, scored{h, s})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	// Show top N.
	fmt.Printf("\n--- Top %d by RWR Score ---\n", *topN)
	for i, r := range ranked {
		if i >= *topN {
			break
		}
		node, _ := st.GetNode(ctx, r.hash)
		name := r.hash.String()[:8]
		kind := "?"
		if node != nil {
			name = terminalSymbol(node.QualifiedName)
			kind = node.Kind
		}
		isSeed := ""
		for _, sh := range seedHashes {
			if sh == r.hash {
				isSeed = " [SEED]"
				break
			}
		}
		fmt.Printf("  [%2d] %.6f  %s (%s)%s\n", i+1, r.score, name, kind, isSeed)
	}

	// Score distribution summary.
	if len(ranked) > 0 {
		fmt.Printf("\n--- Score Distribution ---\n")
		fmt.Printf("  Max: %.6f  Min: %.6f\n", ranked[0].score, ranked[len(ranked)-1].score)
		top10Score := 0.0
		for i := 0; i < min(10, len(ranked)); i++ {
			top10Score += ranked[i].score
		}
		totalScore := 0.0
		for _, r := range ranked {
			totalScore += r.score
		}
		fmt.Printf("  Top-10 mass: %.4f (%.1f%% of total)\n", top10Score, 100*top10Score/totalScore)
		fmt.Printf("  Total mass: %.4f across %d nodes\n", totalScore, len(ranked))
	}

	return nil
}
