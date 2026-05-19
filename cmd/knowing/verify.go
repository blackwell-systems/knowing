package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/blackwell-systems/knowing/internal/snapshot"
)

// cmdVerify checks a Merkle proof JSON against its claimed root.
// No database is needed: verification is purely cryptographic.
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	proofFile := fs.String("proof", "", "Path to proof JSON file (or - for stdin)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: knowing verify -proof <file>\n\n")
		fmt.Fprintf(os.Stderr, "Verify a Merkle proof offline. No database needed.\n")
		fmt.Fprintf(os.Stderr, "The proof is checked cryptographically against the claimed root hash.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *proofFile == "" {
		// Check if there's a positional arg.
		if fs.NArg() > 0 {
			*proofFile = fs.Arg(0)
		} else {
			fs.Usage()
			return fmt.Errorf("-proof is required")
		}
	}

	// Read proof.
	var data []byte
	var err error
	if *proofFile == "-" {
		data, err = os.ReadFile("/dev/stdin")
	} else {
		data, err = os.ReadFile(*proofFile)
	}
	if err != nil {
		return fmt.Errorf("reading proof: %w", err)
	}

	// Parse the proof envelope.
	var envelope struct {
		Source       string                `json:"source"`
		Target       string                `json:"target"`
		EdgeType     string                `json:"edge_type"`
		SnapshotHash string                `json:"snapshot_hash"`
		Proof        *snapshot.MerkleProof `json:"proof"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parsing proof JSON: %w", err)
	}

	if envelope.Proof == nil {
		return fmt.Errorf("no proof field in JSON")
	}

	// Verify.
	valid := snapshot.VerifyProof(envelope.Proof)

	totalSteps := len(envelope.Proof.EdgeToEdgeTypeRoot) +
		len(envelope.Proof.EdgeTypeToPackageRoot) +
		len(envelope.Proof.PackageToRepoRoot)

	if valid {
		fmt.Printf("VERIFIED\n")
		fmt.Printf("  Edge:      %s\n", envelope.Proof.EdgeHash)
		fmt.Printf("  Package:   %s\n", envelope.Proof.PackagePath)
		fmt.Printf("  Edge type: %s\n", envelope.Proof.EdgeType)
		fmt.Printf("  Repo root: %s\n", envelope.Proof.RepoRoot)
		if envelope.SnapshotHash != "" {
			fmt.Printf("  Snapshot:  %s\n", envelope.SnapshotHash)
		}
		if envelope.Source != "" {
			fmt.Printf("  Claim:     %s -%s-> %s\n", envelope.Source, envelope.EdgeType, envelope.Target)
		}
		fmt.Printf("  Proof:     %d steps\n", totalSteps)
		return nil
	}

	fmt.Fprintf(os.Stderr, "FAILED\n")
	fmt.Fprintf(os.Stderr, "  The proof does not verify against the claimed root.\n")
	fmt.Fprintf(os.Stderr, "  Either the edge was not in this snapshot, or the proof was tampered with.\n")
	os.Exit(1)
	return nil
}
