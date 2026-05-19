package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// AcquireLockfile creates a lockfile at dbPath+".lock" containing the current
// process PID, using O_CREAT|O_EXCL for atomic creation (mirrors git's
// lockfile.c pattern). If the lockfile already exists and the recorded PID is
// alive, an error is returned. If the recorded PID is no longer alive, the
// stale lockfile is removed and creation is retried once.
func AcquireLockfile(dbPath string) error {
	lockPath := dbPath + ".lock"

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("create lockfile %s: %w", lockPath, err)
		}

		// Lockfile exists: check if the recorded PID is still alive.
		pid, pidErr := readLockfilePID(lockPath)
		if pidErr == nil && pid > 0 {
			proc, procErr := os.FindProcess(pid)
			if procErr == nil {
				signalErr := proc.Signal(syscall.Signal(0))
				if signalErr == nil {
					// Process is alive.
					return fmt.Errorf("another knowing daemon (PID %d) is using database %s", pid, dbPath)
				}
			}
		}

		// Process is gone (or PID unreadable): remove stale lockfile and retry.
		_ = os.Remove(lockPath)
		f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("create lockfile after removing stale %s: %w", lockPath, err)
		}
	}

	defer f.Close()
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		_ = os.Remove(lockPath)
		return fmt.Errorf("write lockfile %s: %w", lockPath, err)
	}
	return nil
}

// ReleaseLockfile removes the lockfile at dbPath+".lock". Safe to call even
// if the lockfile does not exist.
func ReleaseLockfile(dbPath string) {
	_ = os.Remove(dbPath + ".lock")
}

// readLockfilePID reads the PID written by AcquireLockfile from the given
// lockfile path. Returns 0 and a non-nil error if the file cannot be read or
// does not contain a valid PID.
func readLockfilePID(lockPath string) (int, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID %q in lockfile: %w", pidStr, err)
	}
	return pid, nil
}
