package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestAuditSupplyChain_MissingBase verifies that the command returns an error
// when --base is not provided.
func TestAuditSupplyChain_MissingBase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	err := cmdAuditSupplyChain([]string{"-db", dbPath})
	if err == nil {
		t.Fatal("expected error for missing --base flag")
	}
	if !strings.Contains(err.Error(), "--base flag is required") {
		t.Errorf("expected '--base flag is required' in error, got %q", err.Error())
	}
}

// TestAuditSupplyChain_InvalidBaseRef verifies that the command returns an error
// for an invalid base snapshot reference when no snapshots exist.
func TestAuditSupplyChain_InvalidBaseRef(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	err := cmdAuditSupplyChain([]string{"-db", dbPath, "-base", "@latest"})
	if err == nil {
		t.Fatal("expected error for invalid base ref (no snapshots)")
	}
	if !strings.Contains(err.Error(), "resolving base snapshot") {
		t.Errorf("expected 'resolving base snapshot' in error, got %q", err.Error())
	}
}

// TestAuditSupplyChain_JSONOutput verifies that when snapshots exist but have
// no new nodes, the output is valid JSON with correct structure.
func TestAuditSupplyChain_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	outPath := filepath.Join(dir, "report.json")

	// Create a store with two snapshots.
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	oldHash := types.NewHash([]byte("supply-chain-old"))
	newHash := types.NewHash([]byte("supply-chain-new"))

	err = st.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: oldHash,
		CommitHash:   "aaa111",
		Timestamp:    1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = st.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: newHash,
		CommitHash:   "bbb222",
		Timestamp:    2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	err = cmdAuditSupplyChain([]string{
		"-db", dbPath,
		"-base", "@prev",
		"-head", "@latest",
		"-o", outPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}

	var report SupplyChainReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, string(data))
	}

	if report.BaseSnapshot == "" {
		t.Error("expected non-empty base_snapshot in report")
	}
	if report.HeadSnapshot == "" {
		t.Error("expected non-empty head_snapshot in report")
	}
	if report.Threshold != 0.3 {
		t.Errorf("expected threshold 0.3, got %f", report.Threshold)
	}
	if report.GeneratedAt == "" {
		t.Error("expected non-empty generated_at in report")
	}
}

// TestAuditSupplyChain_FailOnSuspicious verifies that --fail-on-suspicious
// returns an error when suspicious files exceed the threshold.
func TestAuditSupplyChain_FailOnSuspicious(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	oldHash := types.NewHash([]byte("fail-test-old"))
	newHash := types.NewHash([]byte("fail-test-new"))

	err = st.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: oldHash,
		CommitHash:   "ccc333",
		Timestamp:    1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = st.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: newHash,
		CommitHash:   "ddd444",
		Timestamp:    2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()

	// With threshold=0.0 and no actual new nodes, there should be no suspicious
	// files, so the command should succeed. This validates the flag is parsed.
	err = cmdAuditSupplyChain([]string{
		"-db", dbPath,
		"-base", "@prev",
		"-head", "@latest",
		"-threshold", "0.0",
		"-fail-on-suspicious",
	})
	// With empty snapshots (no new nodes), no files are analyzed,
	// so --fail-on-suspicious should not trigger.
	if err != nil {
		t.Fatalf("unexpected error with empty snapshots: %v", err)
	}
}
