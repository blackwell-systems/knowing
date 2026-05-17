package main

import (
	"strings"
	"testing"
)

func TestIngestSCIP_MissingFileFlag(t *testing.T) {
	err := cmdIngestSCIP([]string{"-repo", "github.com/org/repo"})
	if err == nil {
		t.Fatal("expected error when -file is missing")
	}
	if !strings.Contains(err.Error(), "-file") {
		t.Errorf("error should mention -file flag, got: %v", err)
	}
}

func TestIngestSCIP_MissingRepoFlag(t *testing.T) {
	err := cmdIngestSCIP([]string{"-file", "/tmp/test.scip"})
	if err == nil {
		t.Fatal("expected error when -repo is missing")
	}
	if !strings.Contains(err.Error(), "-repo") {
		t.Errorf("error should mention -repo flag, got: %v", err)
	}
}

func TestIngestSCIP_NonExistentFile(t *testing.T) {
	// Use a temp dir for the database so it doesn't pollute the working directory.
	dbPath := t.TempDir() + "/test.db"
	err := cmdIngestSCIP([]string{
		"-file", "/tmp/nonexistent-scip-index-file.scip",
		"-repo", "github.com/org/repo",
		"-db", dbPath,
	})
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "ingest") || !strings.Contains(err.Error(), "SCIP") {
		t.Errorf("error should mention SCIP ingestion, got: %v", err)
	}
}

func TestIngestSCIP_NoFlags(t *testing.T) {
	err := cmdIngestSCIP([]string{})
	if err == nil {
		t.Fatal("expected error when no flags provided")
	}
}
