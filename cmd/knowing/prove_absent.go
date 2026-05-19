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

// cmdProveAbsent generates a Merkle proof that a specific edge does NOT exist
// in the current snapshot. The proof shows the two adjacent leaves that bracket
// where the missing edge would be, proving there is no room for it.
func cmdProveAbsent(args []string) error {
	fs := flag.NewFlagSet("prove-absent", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	source := fs.String("source", "", "Qualified name of the source symbol")
	target := fs.String("target", "", "Qualified name of the target symbol")
	edgeType := fs.String("type", "calls", "Edge type (calls, imports, implements, etc.)")
	repo := fs.String("repo", "", "Repository URL (default: auto-detect)")
	outFile := fs.String("o", "", "Write proof to file instead of stdout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: knowing prove-absent -source <symbol> -target <symbol> [-type calls]\n\n")
		fmt.Fprintf(os.Stderr, "Prove that a relationship does NOT exist in the current snapshot.\n")
		fmt.Fprintf(os.Stderr, "The proof is cryptographic: it shows the two adjacent edges that\n")
		fmt.Fprintf(os.Stderr, "bracket where the missing edge would be, proving there is no room.\n\n")
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
		if gitRoot != "" {
			repoURL = detectRepoURL(gitRoot)
		}
		if repoURL == "" {
			return fmt.Errorf("could not detect repo; use -repo flag")
		}
	}
	repoHash := types.NewHash([]byte(repoURL))

	// Find source nodes to determine the package.
	sourceNodes, err := st.NodesByName(ctx, *source)
	if err != nil || len(sourceNodes) == 0 {
		return fmt.Errorf("source symbol %q not found", *source)
	}

	pkgPath, err := snapshot.ExtractPackagePath(sourceNodes[0].QualifiedName)
	if err != nil {
		return fmt.Errorf("extracting package path: %w", err)
	}

	// Compute the edge hash that would exist if this relationship were real.
	targetNodes, err := st.NodesByName(ctx, *target)
	if err != nil || len(targetNodes) == 0 {
		return fmt.Errorf("target symbol %q not found", *target)
	}

	edgeHash := types.ComputeEdgeHash(
		sourceNodes[0].NodeHash,
		targetNodes[0].NodeHash,
		*edgeType,
		"ast_inferred",
	)

	// Verify the edge actually doesn't exist.
	existingEdges, err := st.EdgesFrom(ctx, sourceNodes[0].NodeHash, *edgeType)
	if err == nil {
		for _, e := range existingEdges {
			if e.TargetHash == targetNodes[0].NodeHash {
				return fmt.Errorf("cannot prove absence: %s -%s-> %s EXISTS in the graph", *source, *edgeType, *target)
			}
		}
	}

	// Build tree and generate absence proof.
	snapMgr := snapshot.NewSnapshotManager(st)
	latestSnap, err := st.LatestSnapshot(ctx, repoHash)
	if err != nil || latestSnap == nil {
		return fmt.Errorf("no snapshot found for %s", repoURL)
	}

	edgeInputs, _, err := snapMgr.CollectEdgeInputs(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("collecting edge inputs: %w", err)
	}

	tree := snapshot.BuildHierarchicalTree(edgeInputs)

	proof, err := snapshot.GenerateAbsenceProof(tree, edgeHash, pkgPath, *edgeType, edgeInputs)
	if err != nil {
		return fmt.Errorf("generating absence proof: %w", err)
	}

	output := struct {
		Source       string                  `json:"source"`
		Target       string                  `json:"target"`
		EdgeType     string                  `json:"edge_type"`
		Absent       bool                    `json:"absent"`
		SnapshotHash string                  `json:"snapshot_hash"`
		Proof        *snapshot.AbsenceProof  `json:"proof"`
	}{
		Source:       *source,
		Target:       *target,
		EdgeType:     *edgeType,
		Absent:       true,
		SnapshotHash: latestSnap.SnapshotHash.String(),
		Proof:        proof,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling proof: %w", err)
	}

	if *outFile != "" {
		if err := os.WriteFile(*outFile, data, 0644); err != nil {
			return fmt.Errorf("writing proof: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Absence proof written to %s (%d bytes)\n", *outFile, len(data))
	} else {
		fmt.Println(string(data))
	}

	return nil
}
