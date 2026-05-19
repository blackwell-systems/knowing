package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// AuditReport is the structured output of knowing audit.
type AuditReport struct {
	GeneratedAt  string          `json:"generated_at"`
	RepoURL      string          `json:"repo_url"`
	SnapshotHash string          `json:"snapshot_hash"`
	CommitHash   string          `json:"commit_hash"`
	Integrity    AuditIntegrity  `json:"integrity"`
	Summary      AuditSummary    `json:"summary"`
	CrossPackage []AuditEdge     `json:"cross_package_edges,omitempty"`
	Proofs       []AuditProof    `json:"proofs,omitempty"`
}

// AuditIntegrity is the result of fsck.
type AuditIntegrity struct {
	Status     string `json:"status"` // "clean" or "errors_found"
	Errors     int    `json:"errors"`
	Warnings   int    `json:"warnings"`
	DurationMs int64  `json:"duration_ms"`
}

// AuditSummary counts graph entities.
type AuditSummary struct {
	Nodes             int            `json:"nodes"`
	Edges             int            `json:"edges"`
	Packages          int            `json:"packages"`
	EdgeTypes         map[string]int `json:"edge_types"`
	CrossPackageCount int            `json:"cross_package_count"`
}

// AuditEdge is a cross-package edge included in the report.
type AuditEdge struct {
	SourcePackage string `json:"source_package"`
	TargetPackage string `json:"target_package"`
	EdgeType      string `json:"edge_type"`
	SourceSymbol  string `json:"source_symbol"`
	TargetSymbol  string `json:"target_symbol"`
	EdgeHash      string `json:"edge_hash"`
	Confidence    float64 `json:"confidence"`
	Provenance    string `json:"provenance"`
}

// AuditProof is a Merkle proof attached to a cross-package edge.
type AuditProof struct {
	EdgeHash string              `json:"edge_hash"`
	Steps    int                 `json:"steps"`
	Proof    *snapshot.MerkleProof `json:"proof"`
}

// cmdAudit generates a structured compliance report for the current snapshot.
func cmdAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	repo := fs.String("repo", "", "Repository URL (default: auto-detect)")
	outFile := fs.String("o", "", "Write report to file (default: stdout)")
	withProofs := fs.Bool("proofs", false, "Include Merkle proofs for all cross-package edges")
	maxEdges := fs.Int("max-edges", 500, "Maximum cross-package edges to include")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: knowing audit [-proofs] [-o report.json] [-repo url]\n\n")
		fmt.Fprintf(os.Stderr, "Generate a structured compliance report for the current snapshot.\n")
		fmt.Fprintf(os.Stderr, "Includes: integrity check, edge summary, cross-package edges,\n")
		fmt.Fprintf(os.Stderr, "and optionally Merkle proofs for every cross-package relationship.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
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

	report := AuditReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		RepoURL:     repoURL,
	}

	// Get latest snapshot.
	latestSnap, err := st.LatestSnapshot(ctx, repoHash)
	if err != nil || latestSnap == nil {
		return fmt.Errorf("no snapshot found for %s", repoURL)
	}
	report.SnapshotHash = latestSnap.SnapshotHash.String()
	report.CommitHash = latestSnap.CommitHash

	// Integrity check.
	fmt.Fprintf(os.Stderr, "Running integrity check...\n")
	fsckStart := time.Now()
	snapMgr := snapshot.NewSnapshotManager(st)
	errs, err := snapMgr.Verify(ctx, repoHash, collectRosterDBPaths(), *dbPath)
	fsckDuration := time.Since(fsckStart)
	if err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}

	errorCount, warnCount := 0, 0
	for _, e := range errs {
		if e.Level == "ERROR" {
			errorCount++
		} else {
			warnCount++
		}
	}
	report.Integrity = AuditIntegrity{
		Status:     "clean",
		Errors:     errorCount,
		Warnings:   warnCount,
		DurationMs: fsckDuration.Milliseconds(),
	}
	if errorCount > 0 {
		report.Integrity.Status = "errors_found"
	}
	fmt.Fprintf(os.Stderr, "Integrity: %s (%d errors, %d warnings, %dms)\n",
		report.Integrity.Status, errorCount, warnCount, fsckDuration.Milliseconds())

	// Collect all nodes and edges.
	fmt.Fprintf(os.Stderr, "Collecting edges...\n")
	nodes, err := st.NodesByName(ctx, repoURL)
	if err != nil {
		return fmt.Errorf("querying nodes: %w", err)
	}

	nodeQN := make(map[types.Hash]string, len(nodes))
	for _, n := range nodes {
		nodeQN[n.NodeHash] = n.QualifiedName
	}

	edgeTypeCounts := make(map[string]int)
	var crossPkgEdges []AuditEdge
	pkgSet := make(map[string]bool)

	for _, n := range nodes {
		srcPkg, _ := snapshot.ExtractPackagePath(n.QualifiedName)
		if srcPkg != "" {
			pkgSet[srcPkg] = true
		}

		edges, err := st.EdgesFrom(ctx, n.NodeHash, "")
		if err != nil {
			continue
		}
		for _, e := range edges {
			edgeTypeCounts[e.EdgeType]++

			targetQN, ok := nodeQN[e.TargetHash]
			if !ok {
				continue
			}
			tgtPkg, _ := snapshot.ExtractPackagePath(targetQN)

			if srcPkg != "" && tgtPkg != "" && srcPkg != tgtPkg && len(crossPkgEdges) < *maxEdges {
				crossPkgEdges = append(crossPkgEdges, AuditEdge{
					SourcePackage: srcPkg,
					TargetPackage: tgtPkg,
					EdgeType:      e.EdgeType,
					SourceSymbol:  n.QualifiedName,
					TargetSymbol:  targetQN,
					EdgeHash:      e.EdgeHash.String(),
					Confidence:    e.Confidence,
					Provenance:    e.Provenance,
				})
			}
		}
	}

	report.Summary = AuditSummary{
		Nodes:             len(nodes),
		Edges:             latestSnap.EdgeCount,
		Packages:          len(pkgSet),
		EdgeTypes:         edgeTypeCounts,
		CrossPackageCount: len(crossPkgEdges),
	}
	report.CrossPackage = crossPkgEdges

	fmt.Fprintf(os.Stderr, "Found %d nodes, %d packages, %d cross-package edges\n",
		len(nodes), len(pkgSet), len(crossPkgEdges))

	// Generate proofs if requested.
	if *withProofs && len(crossPkgEdges) > 0 {
		fmt.Fprintf(os.Stderr, "Generating proofs for %d edges...\n", len(crossPkgEdges))
		edgeInputs, _, err := snapMgr.CollectEdgeInputs(ctx, repoHash)
		if err != nil {
			return fmt.Errorf("collecting edge inputs for proofs: %w", err)
		}
		tree := snapshot.BuildHierarchicalTree(edgeInputs)

		proofStart := time.Now()
		for _, ae := range crossPkgEdges {
			var edgeHash types.Hash
			b, _ := types.ParseHash(ae.EdgeHash)
			edgeHash = b

			proof, err := snapshot.GenerateProof(tree, edgeHash, ae.SourcePackage, ae.EdgeType, edgeInputs)
			if err != nil {
				continue // some edges may not be in the tree
			}
			steps := len(proof.EdgeToEdgeTypeRoot) + len(proof.EdgeTypeToPackageRoot) + len(proof.PackageToRepoRoot)
			report.Proofs = append(report.Proofs, AuditProof{
				EdgeHash: ae.EdgeHash,
				Steps:    steps,
				Proof:    proof,
			})
		}
		fmt.Fprintf(os.Stderr, "Generated %d proofs in %v\n", len(report.Proofs), time.Since(proofStart))
	}

	// Output.
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}

	if *outFile != "" {
		if err := os.WriteFile(*outFile, data, 0644); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Report written to %s (%d bytes)\n", *outFile, len(data))
	} else {
		fmt.Println(string(data))
	}

	return nil
}

// cmdAuditDiff compares two snapshots and produces a structured change report.
func cmdAuditDiff(args []string) error {
	fs := flag.NewFlagSet("audit-diff", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	repo := fs.String("repo", "", "Repository URL (default: auto-detect)")
	outFile := fs.String("o", "", "Write report to file (default: stdout)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: knowing audit-diff <old-snapshot> <new-snapshot> [-o report.json]\n\n")
		fmt.Fprintf(os.Stderr, "Compare two audit point snapshots. Shows changed packages,\n")
		fmt.Fprintf(os.Stderr, "change classification (behavioral/structural/runtime/metadata),\n")
		fmt.Fprintf(os.Stderr, "and added/removed edges.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 2 {
		fs.Usage()
		return fmt.Errorf("two snapshot hashes required")
	}

	oldHashStr := fs.Arg(0)
	newHashStr := fs.Arg(1)

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Resolve repo for edge collection.
	repoURL := *repo
	if repoURL == "" {
		cwd, _ := os.Getwd()
		gitRoot := detectGitRoot(cwd)
		if gitRoot != "" {
			repoURL = detectRepoURL(gitRoot)
		}
	}

	// Get snapshots.
	oldHash, err := types.ParseHash(oldHashStr)
	if err != nil {
		return fmt.Errorf("parsing old snapshot hash: %w", err)
	}
	newHash, err := types.ParseHash(newHashStr)
	if err != nil {
		return fmt.Errorf("parsing new snapshot hash: %w", err)
	}

	oldSnap, err := st.GetSnapshot(ctx, oldHash)
	if err != nil || oldSnap == nil {
		return fmt.Errorf("old snapshot not found: %s", oldHashStr)
	}
	newSnap, err := st.GetSnapshot(ctx, newHash)
	if err != nil || newSnap == nil {
		return fmt.Errorf("new snapshot not found: %s", newHashStr)
	}

	// Compute diff.
	diffResult, err := st.SnapshotDiff(ctx, oldHash, newHash)
	if err != nil {
		return fmt.Errorf("computing diff: %w", err)
	}

	type DiffReport struct {
		GeneratedAt    string              `json:"generated_at"`
		OldSnapshot    string              `json:"old_snapshot"`
		OldCommit      string              `json:"old_commit"`
		NewSnapshot    string              `json:"new_snapshot"`
		NewCommit      string              `json:"new_commit"`
		AddedEdges     int                 `json:"added_edges"`
		RemovedEdges   int                 `json:"removed_edges"`
		Classification string              `json:"classification,omitempty"`
		AddedDetails   []string            `json:"added_edge_types,omitempty"`
		RemovedDetails []string            `json:"removed_edge_types,omitempty"`
	}

	dr := DiffReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		OldSnapshot: oldHashStr,
		OldCommit:   oldSnap.CommitHash,
		NewSnapshot: newHashStr,
		NewCommit:   newSnap.CommitHash,
		AddedEdges:  len(diffResult.EdgesAdded),
		RemovedEdges: len(diffResult.EdgesRemoved),
	}

	// Classify changes if we have the hierarchical trees.
	repoHash := types.NewHash([]byte(repoURL))
	if repoURL != "" {
		snapMgr := snapshot.NewSnapshotManager(st)
		edgeInputs, _, err := snapMgr.CollectEdgeInputs(ctx, repoHash)
		if err == nil && len(edgeInputs) > 0 {
			tree := snapshot.BuildHierarchicalTree(edgeInputs)
			if tree != nil {
				// Build edge type summary.
				addedTypes := make(map[string]int)
				removedTypes := make(map[string]int)
				for _, e := range diffResult.EdgesAdded {
					addedTypes[e.EdgeType]++
				}
				for _, e := range diffResult.EdgesRemoved {
					removedTypes[e.EdgeType]++
				}
				for et, count := range addedTypes {
					dr.AddedDetails = append(dr.AddedDetails, fmt.Sprintf("+%d %s", count, et))
				}
				for et, count := range removedTypes {
					dr.RemovedDetails = append(dr.RemovedDetails, fmt.Sprintf("-%d %s", count, et))
				}

				// Classify based on edge types that changed.
				hasBehavioral := addedTypes["calls"] > 0 || removedTypes["calls"] > 0 ||
					addedTypes["throws"] > 0 || removedTypes["throws"] > 0
				hasRuntime := addedTypes["runtime_calls"] > 0 || removedTypes["runtime_calls"] > 0
				hasStructural := addedTypes["imports"] > 0 || removedTypes["imports"] > 0 ||
					addedTypes["implements"] > 0 || removedTypes["implements"] > 0

				switch {
				case hasBehavioral:
					dr.Classification = "behavioral"
				case hasRuntime:
					dr.Classification = "runtime_drift"
				case hasStructural:
					dr.Classification = "structural"
				default:
					dr.Classification = "metadata_only"
				}
			}
		}
	}

	data, err := json.MarshalIndent(dr, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling diff report: %w", err)
	}

	if *outFile != "" {
		if err := os.WriteFile(*outFile, data, 0644); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Diff report written to %s\n", *outFile)
	} else {
		fmt.Println(string(data))
	}

	return nil
}
