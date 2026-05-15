package daemon

import (
	"fmt"
	"os/exec"
	"strings"
)

// gitdiff.go resolves the set of changed, added, and deleted files between
// two git commits. This powers the daemon's incremental indexing: after
// detecting a new commit via GitWatcher, the daemon calls GitDiffFiles to
// determine which files need re-extraction.

// GitDiffFiles resolves changed, added, and deleted files between two
// commits using git diff --name-status. If oldCommit is empty, all
// tracked files are returned as added (initial index).
// Returns paths relative to repoPath.
func GitDiffFiles(repoPath, oldCommit, newCommit string) (changed, added, deleted []string, err error) {
	if oldCommit == "" {
		// No previous commit: treat all tracked files as added.
		cmd := exec.Command("git", "-C", repoPath, "ls-files")
		out, err := cmd.Output()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("git ls-files: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line != "" {
				added = append(added, line)
			}
		}
		return nil, added, nil, nil
	}

	cmd := exec.Command("git", "-C", repoPath, "diff", "--name-status", oldCommit, newCommit)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("git diff: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status, path := parts[0], parts[1]
		switch {
		case strings.HasPrefix(status, "M"):
			changed = append(changed, path)
		case strings.HasPrefix(status, "A"):
			added = append(added, path)
		case strings.HasPrefix(status, "D"):
			deleted = append(deleted, path)
		case strings.HasPrefix(status, "R"):
			// Renamed: treat as delete old + add new.
			renameParts := strings.SplitN(path, "\t", 2)
			if len(renameParts) == 2 {
				deleted = append(deleted, renameParts[0])
				added = append(added, renameParts[1])
			}
		}
	}
	return changed, added, deleted, nil
}
