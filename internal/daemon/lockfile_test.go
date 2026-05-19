package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAcquireLockfile_Success(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	if err := AcquireLockfile(dbPath); err != nil {
		t.Fatalf("AcquireLockfile: %v", err)
	}
	defer ReleaseLockfile(dbPath)

	lockPath := dbPath + ".lock"
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		t.Fatalf("parse PID from lockfile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("lockfile PID = %d, want %d", pid, os.Getpid())
	}
}

func TestAcquireLockfile_BlocksSecondInstance(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	if err := AcquireLockfile(dbPath); err != nil {
		t.Fatalf("first AcquireLockfile: %v", err)
	}
	defer ReleaseLockfile(dbPath)

	err := AcquireLockfile(dbPath)
	if err == nil {
		t.Fatal("expected error on second AcquireLockfile, got nil")
	}
	if !strings.Contains(err.Error(), "another knowing daemon") {
		t.Errorf("error = %q, want it to contain \"another knowing daemon\"", err.Error())
	}
}

func TestAcquireLockfile_CleansStale(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	lockPath := dbPath + ".lock"

	// Write a lockfile with a PID that almost certainly does not exist.
	stalePID := 999999999
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", stalePID)), 0644); err != nil {
		t.Fatalf("write stale lockfile: %v", err)
	}

	// AcquireLockfile should detect the stale lock, remove it, and succeed.
	if err := AcquireLockfile(dbPath); err != nil {
		t.Fatalf("AcquireLockfile with stale lock: %v", err)
	}
	defer ReleaseLockfile(dbPath)

	// Verify the lockfile now contains the current PID.
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("read lockfile after stale cleanup: %v", err)
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		t.Fatalf("parse PID from lockfile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("lockfile PID = %d, want %d", pid, os.Getpid())
	}
}

func TestReleaseLockfile_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nonexistent.db")

	// Should not panic or error even if no lockfile exists.
	ReleaseLockfile(dbPath)
	ReleaseLockfile(dbPath)
}
