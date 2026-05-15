package daemon

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// CommitEvent describes a commit-level change detected in a repository.
type CommitEvent struct {
	RepoPath     string
	OldCommit    string
	NewCommit    string
	ChangedFiles []string // modified files (relative paths)
	AddedFiles   []string // new files (relative paths)
	DeletedFiles []string // removed files (relative paths)
}

// GitWatcher monitors repositories for git commits by watching .git/HEAD
// and .git/refs/heads/*. It uses fsnotify on these specific paths (one or
// two file descriptors per repo) rather than watching all source files,
// keeping the descriptor count low even for large repositories.
//
// The watch flow: fsnotify fires on .git/HEAD or ref file writes -> debounce
// coalesces rapid writes (e.g., rebase) -> re-read HEAD -> if commit hash
// changed, run git diff to determine changed files -> emit CommitEvent.
type GitWatcher struct {
	debounce time.Duration       // quiet period before emitting an event
	watcher  *fsnotify.Watcher
	events   chan CommitEvent    // outbound commit events
	done     chan struct{}       // closed when the event loop exits

	mu    sync.Mutex
	repos map[string]string     // repoPath -> last known commit hash
}

// NewGitWatcher creates a GitWatcher with the given debounce duration.
// Events are emitted on the channel returned by Events().
func NewGitWatcher(debounce time.Duration) (*GitWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	gw := &GitWatcher{
		debounce: debounce,
		watcher:  w,
		events:   make(chan CommitEvent, 64),
		done:     make(chan struct{}),
		repos:    make(map[string]string),
	}
	go gw.loop()
	return gw, nil
}

// Add registers a repository for commit watching. It reads the current HEAD
// commit hash as the baseline and watches .git/HEAD and .git/refs/heads/.
func (gw *GitWatcher) Add(repoPath string) error {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}

	commit, err := GitHeadCommit(abs)
	if err != nil {
		return err
	}

	gw.mu.Lock()
	gw.repos[abs] = commit
	gw.mu.Unlock()

	// Watch .git/HEAD for branch switches and commits.
	headPath := filepath.Join(abs, ".git", "HEAD")
	if err := gw.watcher.Add(headPath); err != nil {
		return err
	}

	// Watch .git/refs/heads/ for ref updates (new commits on branches).
	refsPath := filepath.Join(abs, ".git", "refs", "heads")
	if err := gw.watcher.Add(refsPath); err != nil {
		// Non-fatal: refs/heads may not exist yet in a fresh repo.
		log.Printf("gitwatcher: warning: cannot watch %s: %v", refsPath, err)
	}

	return nil
}

// Remove stops watching a repository.
func (gw *GitWatcher) Remove(repoPath string) error {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}

	gw.mu.Lock()
	delete(gw.repos, abs)
	gw.mu.Unlock()

	headPath := filepath.Join(abs, ".git", "HEAD")
	_ = gw.watcher.Remove(headPath)

	refsPath := filepath.Join(abs, ".git", "refs", "heads")
	_ = gw.watcher.Remove(refsPath)

	return nil
}

// Events returns a read-only channel of commit events.
func (gw *GitWatcher) Events() <-chan CommitEvent {
	return gw.events
}

// Close shuts down the watcher and waits for the event loop to exit.
func (gw *GitWatcher) Close() error {
	err := gw.watcher.Close()
	<-gw.done // wait for loop to exit
	return err
}

// repoForPath resolves which tracked repository a watched file belongs to.
// Watched paths are inside .git/ subdirectories, so we walk up to find the
// repo root.
func (gw *GitWatcher) repoForPath(watchedPath string) string {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	for repoPath := range gw.repos {
		gitDir := filepath.Join(repoPath, ".git")
		if strings.HasPrefix(watchedPath, gitDir) {
			return repoPath
		}
	}
	return ""
}

// loop is the main event loop. It collects fsnotify events, debounces them
// per repository using per-repo timers, then resolves the commit diff and
// emits CommitEvents. Debouncing is necessary because a single git operation
// (commit, rebase, merge) can trigger multiple file writes to .git/ in rapid
// succession; the debounce timer ensures we process the final state once.
func (gw *GitWatcher) loop() {
	defer close(gw.done)
	defer close(gw.events)

	type pending struct {
		timer *time.Timer
	}
	state := make(map[string]*pending)

	for {
		select {
		case ev, ok := <-gw.watcher.Events:
			if !ok {
				// Watcher closed; stop all pending timers.
				for _, p := range state {
					if p.timer != nil {
						p.timer.Stop()
					}
				}
				return
			}

			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			absFile, err := filepath.Abs(ev.Name)
			if err != nil {
				continue
			}

			repo := gw.repoForPath(absFile)
			if repo == "" {
				continue
			}

			p, ok := state[repo]
			if !ok {
				p = &pending{}
				state[repo] = p
			}

			// Reset debounce timer.
			if p.timer != nil {
				p.timer.Stop()
			}
			repoPath := repo
			p.timer = time.AfterFunc(gw.debounce, func() {
				gw.handleCommitChange(repoPath)
			})

		case _, ok := <-gw.watcher.Errors:
			if !ok {
				return
			}
			// Errors are logged but not fatal.
		}
	}
}

// handleCommitChange reads the current HEAD commit hash and, if it differs
// from the last known hash, resolves the diff and emits a CommitEvent.
func (gw *GitWatcher) handleCommitChange(repoPath string) {
	newCommit, err := GitHeadCommit(repoPath)
	if err != nil {
		log.Printf("gitwatcher: failed to read HEAD for %s: %v", repoPath, err)
		return
	}

	gw.mu.Lock()
	oldCommit := gw.repos[repoPath]
	if newCommit == oldCommit {
		gw.mu.Unlock()
		return
	}
	gw.repos[repoPath] = newCommit
	gw.mu.Unlock()

	changed, added, deleted, err := GitDiffFiles(repoPath, oldCommit, newCommit)
	if err != nil {
		log.Printf("gitwatcher: failed to diff %s..%s in %s: %v", oldCommit[:8], newCommit[:8], repoPath, err)
		return
	}

	event := CommitEvent{
		RepoPath:     repoPath,
		OldCommit:    oldCommit,
		NewCommit:    newCommit,
		ChangedFiles: changed,
		AddedFiles:   added,
		DeletedFiles: deleted,
	}

	// Non-blocking send; drop if consumer is too slow.
	select {
	case gw.events <- event:
	default:
	}
}
