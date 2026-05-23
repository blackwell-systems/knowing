package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// PIDFilePath returns the path to the daemon PID file: ~/.knowing/daemon.pid.
// If the user's home directory cannot be determined, falls back to /tmp.
func PIDFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/knowing-daemon.pid"
	}
	dir := filepath.Join(home, ".knowing")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "daemon.pid")
}

// WritePIDFile writes the current process PID to the PID file.
func WritePIDFile() error {
	path := PIDFilePath()
	content := fmt.Sprintf("%d\n", os.Getpid())
	return os.WriteFile(path, []byte(content), 0644)
}

// ReadPIDFile reads the PID from the PID file.
// Returns (0, error) if the file does not exist or contains invalid data.
func ReadPIDFile() (int, error) {
	path := PIDFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID %q in %s: %w", pidStr, path, err)
	}
	return pid, nil
}

// RemovePIDFile removes the PID file. Safe to call even if the file does not exist.
func RemovePIDFile() {
	_ = os.Remove(PIDFilePath())
}

// IsDaemonRunning checks whether a daemon process is alive by reading the PID
// file and sending signal 0 to the recorded process. Returns (pid, true) if
// the process is running, (0, false) otherwise.
func IsDaemonRunning() (int, bool) {
	pid, err := ReadPIDFile()
	if err != nil || pid <= 0 {
		return 0, false
	}

	// Signal 0 does not actually send a signal; it checks if the process exists
	// and the caller has permission to signal it.
	err = syscall.Kill(pid, 0)
	if err == nil {
		return pid, true
	}
	// ESRCH means no such process; EPERM means process exists but we cannot signal it
	// (still means it's running).
	if err == syscall.EPERM {
		return pid, true
	}
	return 0, false
}
