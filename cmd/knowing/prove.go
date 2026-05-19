package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdProve generates a Merkle proof that a specific edge exists in the
// current snapshot. The proof is a JSON object that can be verified
// offline with `knowing verify`.
func cmdProve(args []string) error {
	fs := flag.NewFlagSet("prove", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	source := fs.String("source", "", "Qualified name of the source symbol")
	target := fs.String("target", "", "Qualified name of the target symbol")
	edgeType := fs.String("type", "calls", "Edge type (calls, imports, implements, etc.)")
	repo := fs.String("repo", "", "Repository URL (default: auto-detect from current directory)")
	outFile := fs.String("o", "", "Write proof to file instead of stdout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: knowing prove -source <symbol> -target <symbol> [-type calls] [-repo url]\n\n")
		fmt.Fprintf(os.Stderr, "Generate a Merkle proof that a relationship exists in the current snapshot.\n")
		fmt.Fprintf(os.Stderr, "The proof can be verified offline with `knowing verify`.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *source == "" || *target == "" {
		fs.Usage()
		return fmt.Errorf("both -source and -target are required")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Resolve repo.
	repoURL := *repo
	if repoURL == "" {
		cwd, _ := os.Getwd()
		gitRoot := detectGitRoot(cwd)
		if gitRoot == "" {
			return fmt.Errorf("not in a git repository; use -repo flag")
		}
		repoURL = detectRepoURL(gitRoot)
		if repoURL == "" {
			return fmt.Errorf("could not detect repo URL; use -repo flag")
		}
	}
	repoHash := types.NewHash([]byte(repoURL))

	// Find the edge.
	sourceNodes, err := st.NodesByName(ctx, *source)
	if err != nil {
		return fmt.Errorf("looking up source: %w", err)
	}
	if len(sourceNodes) == 0 {
		return fmt.Errorf("source symbol %q not found", *source)
	}

	targetNodes, err := st.NodesByName(ctx, *target)
	if err != nil {
		return fmt.Errorf("looking up target: %w", err)
	}
	if len(targetNodes) == 0 {
		return fmt.Errorf("target symbol %q not found", *target)
	}

	// Find the edge hash.
	var edgeHash types.Hash
	var found bool
	for _, sn := range sourceNodes {
		edges, err := st.EdgesFrom(ctx, sn.NodeHash, *edgeType)
		if err != nil {
			continue
		}
		for _, e := range edges {
			for _, tn := range targetNodes {
				if e.TargetHash == tn.NodeHash {
					edgeHash = e.EdgeHash
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return fmt.Errorf("no %s edge found from %q to %q", *edgeType, *source, *target)
	}

	// Get the hierarchical tree and edge inputs.
	snapMgr := snapshot.NewSnapshotManager(st)
	latestSnap, err := st.LatestSnapshot(ctx, repoHash)
	if err != nil || latestSnap == nil {
		return fmt.Errorf("no snapshot found for repo %s", repoURL)
	}

	edgeInputs, _, err := snapMgr.CollectEdgeInputs(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("collecting edge inputs: %w", err)
	}

	tree := snapshot.BuildHierarchicalTree(edgeInputs)

	// Find the package path for this edge.
	pkgPath, err := snapshot.ExtractPackagePath(sourceNodes[0].QualifiedName)
	if err != nil {
		return fmt.Errorf("extracting package path: %w", err)
	}

	// Generate proof.
	proof, err := snapshot.GenerateProof(tree, edgeHash, pkgPath, *edgeType, edgeInputs)
	if err != nil {
		return fmt.Errorf("generating proof: %w", err)
	}

	// Output.
	output := struct {
		Source       string              `json:"source"`
		Target       string              `json:"target"`
		EdgeType     string              `json:"edge_type"`
		SnapshotHash string              `json:"snapshot_hash"`
		Proof        *snapshot.MerkleProof `json:"proof"`
	}{
		Source:       *source,
		Target:       *target,
		EdgeType:     *edgeType,
		SnapshotHash: latestSnap.SnapshotHash.String(),
		Proof:        proof,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling proof: %w", err)
	}

	totalSteps := len(proof.EdgeToEdgeTypeRoot) + len(proof.EdgeTypeToPackageRoot) + len(proof.PackageToRepoRoot)

	if *outFile != "" {
		if err := os.WriteFile(*outFile, data, 0644); err != nil {
			return fmt.Errorf("writing proof to %s: %w", *outFile, err)
		}
		fmt.Fprintf(os.Stderr, "\n  ✓ relationship found\n\n")
		fmt.Fprintf(os.Stderr, "  proof:   %s\n", *outFile)
		fmt.Fprintf(os.Stderr, "  size:    %d bytes (%d cryptographic steps)\n", len(data), totalSteps)
		fmt.Fprintf(os.Stderr, "  verify:  knowing verify %s\n\n", *outFile)
	} else {
		fmt.Println(string(data))
	}

	return nil
}
