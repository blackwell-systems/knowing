package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/blackwell-systems/knowing/internal/diff"
	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// SupplyChainReport is the JSON output of the audit-supply-chain command.
type SupplyChainReport struct {
	GeneratedAt     string              `json:"generated_at"`
	BaseSnapshot    string              `json:"base_snapshot"`
	HeadSnapshot    string              `json:"head_snapshot"`
	Threshold       float64             `json:"threshold"`
	Summary         SupplyChainSummary  `json:"summary"`
	SuspiciousFiles []SuspiciousFile    `json:"suspicious_files,omitempty"`
	CapabilityPaths []CapabilityPath    `json:"capability_paths,omitempty"`
}

// SupplyChainSummary provides aggregate counts.
type SupplyChainSummary struct {
	FilesAnalyzed    int `json:"files_analyzed"`
	FilesSuspicious  int `json:"files_suspicious"`
	EnvReadsTotal    int `json:"env_reads_total"`
	ProcessExecTotal int `json:"process_exec_total"`
}

// SuspiciousFile describes a file that exceeds the isolation threshold.
type SuspiciousFile struct {
	File         string   `json:"file"`
	Score        float64  `json:"score"`
	InboundEdges int      `json:"inbound_edges"`
	OutboundEdges int     `json:"outbound_edges"`
	HookExecuted bool     `json:"hook_executed"`
	ReadsEnv     []string `json:"reads_env,omitempty"`
	ExecutesProc []string `json:"executes_process,omitempty"`
}

// CapabilityPath traces a reads_env -> executes_process chain.
type CapabilityPath struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	EdgeType string   `json:"edge_type"`
	Via      []string `json:"via,omitempty"`
}

// cmdAuditSupplyChain detects suspicious supply chain patterns in new code
// by combining snapshot diff with isolation analysis.
func cmdAuditSupplyChain(args []string) error {
	fs := flag.NewFlagSet("audit-supply-chain", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	base := fs.String("base", "", "Baseline snapshot hash or ref (@prev, @N)")
	head := fs.String("head", "@latest", "Current snapshot hash or ref (default: @latest)")
	threshold := fs.Float64("threshold", 0.3, "Isolation score threshold for suspicious files")
	failOnSuspicious := fs.Bool("fail-on-suspicious", false, "Exit non-zero if any file exceeds threshold")
	scanAll := fs.Bool("scan-all", false, "Scan all files (skip diff, useful when clean/compromised are in separate DBs)")
	outFile := fs.String("o", "", "Write JSON report to file (default: stdout)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: knowing audit-supply-chain --base <ref> [--head <ref>] [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Detect suspicious supply chain patterns in new code by combining\n")
		fmt.Fprintf(os.Stderr, "snapshot diff with isolation analysis.\n\n")
		fmt.Fprintf(os.Stderr, "Refs: @latest, @prev, @first, @N, or 64-char hex hash\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *base == "" {
		fs.Usage()
		return fmt.Errorf("--base flag is required")
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	baseHash, err := resolveSnapshotRef(st, *base)
	if err != nil {
		return fmt.Errorf("resolving base snapshot: %w", err)
	}

	headHash, err := resolveSnapshotRef(st, *head)
	if err != nil {
		return fmt.Errorf("resolving head snapshot: %w", err)
	}

	ctx := context.Background()

	var newFileHashes []types.Hash

	if *scanAll {
		// Scan all files in the DB (no diff needed).
		newFileHashes = collectAllFileHashes(ctx, st)
	} else {
		// Compute semantic diff to find new files.
		diffResult, err := diff.SemanticDiff(ctx, st, baseHash, headHash)
		if err != nil {
			return fmt.Errorf("computing semantic diff: %w", err)
		}
		newFileHashes = collectNewFileHashes(ctx, st, diffResult)
	}

	// Compute isolation scores for new files.
	isolationResults, err := diff.ComputeIsolation(ctx, st, newFileHashes)
	if err != nil {
		return fmt.Errorf("computing isolation: %w", err)
	}

	// Build report.
	report := buildSupplyChainReport(ctx, st, baseHash, headHash, *threshold, isolationResults)

	// Marshal and output.
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}

	if *outFile != "" {
		if err := os.WriteFile(*outFile, out, 0644); err != nil {
			return fmt.Errorf("writing output file: %w", err)
		}
	} else {
		fmt.Println(string(out))
	}

	if *failOnSuspicious && report.Summary.FilesSuspicious > 0 {
		return fmt.Errorf("found %d suspicious files exceeding threshold %.2f", report.Summary.FilesSuspicious, *threshold)
	}

	return nil
}

// collectAllFileHashes returns all unique file hashes in the database.
// Used with --scan-all when clean/compromised versions are in separate DBs.
func collectAllFileHashes(ctx context.Context, st *store.SQLiteStore) []types.Hash {
	nodes, err := st.NodesByName(ctx, "%")
	if err != nil {
		return nil
	}
	seen := make(map[types.Hash]struct{})
	var hashes []types.Hash
	for _, n := range nodes {
		if n.FileHash.IsZero() {
			continue
		}
		if _, ok := seen[n.FileHash]; !ok {
			seen[n.FileHash] = struct{}{}
			hashes = append(hashes, n.FileHash)
		}
	}
	return hashes
}

// collectNewFileHashes extracts unique file hashes from nodes added in the diff.
// It looks up each added node by its NodeHash to retrieve the FileHash.
func collectNewFileHashes(ctx context.Context, st *store.SQLiteStore, result *diff.SemanticDiffResult) []types.Hash {
	seen := make(map[types.Hash]struct{})
	var hashes []types.Hash

	for _, n := range result.NodesAdded {
		nodeHash, err := types.ParseHash(n.NodeHash)
		if err != nil {
			continue
		}
		node, err := st.GetNode(ctx, nodeHash)
		if err != nil || node == nil {
			continue
		}
		if node.FileHash.IsZero() {
			continue
		}
		if _, ok := seen[node.FileHash]; !ok {
			seen[node.FileHash] = struct{}{}
			hashes = append(hashes, node.FileHash)
		}
	}

	return hashes
}

// buildSupplyChainReport constructs the full report from isolation results.
func buildSupplyChainReport(ctx context.Context, st *store.SQLiteStore, baseHash, headHash types.Hash, threshold float64, results []diff.IsolationResult) *SupplyChainReport {
	report := &SupplyChainReport{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		BaseSnapshot: baseHash.String(),
		HeadSnapshot: headHash.String(),
		Threshold:    threshold,
	}

	var envTotal int
	var procTotal int

	for _, r := range results {
		envTotal += len(r.ReadsEnv)
		procTotal += len(r.ExecutesProc)

		if r.Score >= threshold {
			sf := SuspiciousFile{
				File:          r.File,
				Score:         r.Score,
				InboundEdges:  r.InboundEdges,
				OutboundEdges: r.OutboundEdges,
				HookExecuted:  r.HookExecuted,
				ReadsEnv:      r.ReadsEnv,
				ExecutesProc:  r.ExecutesProc,
			}
			report.SuspiciousFiles = append(report.SuspiciousFiles, sf)

			// Trace capability paths for suspicious files.
			paths := traceCapabilityPaths(ctx, st, r)
			report.CapabilityPaths = append(report.CapabilityPaths, paths...)
		}
	}

	report.Summary = SupplyChainSummary{
		FilesAnalyzed:    len(results),
		FilesSuspicious:  len(report.SuspiciousFiles),
		EnvReadsTotal:    envTotal,
		ProcessExecTotal: procTotal,
	}

	return report
}

// traceCapabilityPaths finds reads_env -> executes_process chains for a file.
func traceCapabilityPaths(ctx context.Context, st *store.SQLiteStore, result diff.IsolationResult) []CapabilityPath {
	var paths []CapabilityPath

	// For each env read, check if the same file also executes processes.
	// This is a direct chain: file reads env AND executes process.
	if len(result.ReadsEnv) > 0 && len(result.ExecutesProc) > 0 {
		for _, env := range result.ReadsEnv {
			for _, proc := range result.ExecutesProc {
				paths = append(paths, CapabilityPath{
					From:     env,
					To:       proc,
					EdgeType: edgetype.ReadsEnv + " -> " + edgetype.ExecutesProcess,
					Via:      []string{result.File},
				})
			}
		}
	}

	return paths
}
