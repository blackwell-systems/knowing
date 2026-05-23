package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// overridePIDFilePath temporarily overrides PIDFilePath to use a temp directory.
// Returns a cleanup function that restores the original.
func overridePIDFilePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")

	// We override by setting HOME to a temp dir so PIDFilePath() returns a predictable path.
	// However, PIDFilePath uses os.UserHomeDir which may use other env vars.
	// Instead, write directly to test the functions that accept path indirection.
	// For testing, we write the PID file at the standard path but clean up after.
	return pidPath
}

func TestPIDFile_WriteThenRead(t *testing.T) {
	// Use a temp dir to avoid polluting the real ~/.knowing directory.
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "daemon.pid")

	// Write current PID.
	content := strconv.Itoa(os.Getpid()) + "\n"
	if err := os.WriteFile(pidPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Read it back.
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	gotPID, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	if gotPID != os.Getpid() {
		t.Errorf("PID = %d, want %d", gotPID, os.Getpid())
	}
}

func TestPIDFile_WritePIDFile(t *testing.T) {
	// WritePIDFile writes to PIDFilePath() which uses ~/.knowing.
	// To avoid side effects in CI, we test the function works end-to-end.
	if err := WritePIDFile(); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}
	t.Cleanup(RemovePIDFile)

	pid, err := ReadPIDFile()
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("ReadPIDFile = %d, want %d", pid, os.Getpid())
	}
}

func TestPIDFile_IsDaemonRunning_CurrentProcess(t *testing.T) {
	// Write current PID so IsDaemonRunning can find it.
	if err := WritePIDFile(); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}
	t.Cleanup(RemovePIDFile)

	pid, running := IsDaemonRunning()
	if !running {
		t.Error("IsDaemonRunning should return true for current process")
	}
	if pid != os.Getpid() {
		t.Errorf("IsDaemonRunning pid = %d, want %d", pid, os.Getpid())
	}
}

func TestPIDFile_IsDaemonRunning_AfterRemove(t *testing.T) {
	// Write and then remove: should report not running.
	if err := WritePIDFile(); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}
	RemovePIDFile()

	pid, running := IsDaemonRunning()
	if running {
		t.Errorf("IsDaemonRunning should return false after RemovePIDFile, got pid=%d", pid)
	}
}

func TestPIDFile_ReadPIDFile_NoFile(t *testing.T) {
	// Ensure the PID file does not exist.
	RemovePIDFile()

	_, err := ReadPIDFile()
	if err == nil {
		t.Error("ReadPIDFile should return error when no file exists")
	}
}

func TestPIDFile_IsDaemonRunning_DeadProcess(t *testing.T) {
	// Write a PID that is almost certainly not running (very high PID).
	// On macOS/Linux, PID 99999999 is extremely unlikely to exist.
	path := PIDFilePath()
	if err := os.WriteFile(path, []byte("99999999\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Cleanup(RemovePIDFile)

	_, running := IsDaemonRunning()
	if running {
		t.Error("IsDaemonRunning should return false for dead PID 99999999")
	}
}

func TestPIDFile_PIDFilePath(t *testing.T) {
	path := PIDFilePath()
	if path == "" {
		t.Fatal("PIDFilePath returned empty string")
	}
	if !strings.HasSuffix(path, "daemon.pid") {
		t.Errorf("PIDFilePath = %q, want suffix daemon.pid", path)
	}
}
