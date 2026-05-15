// Package daemon provides file watching, reindex coordination, and daemon
// lifecycle management for the knowing system of record.
package daemon

// Deprecated: FileWatcher is replaced by GitWatcher (gitwatcher.go).
// This file is retained for backward compatibility during transition.
// Remove after all references are updated.

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchEvent describes a batch of file changes within a repository.
type WatchEvent struct {
	RepoPath string
	Files    []string
}

// FileWatcher monitors repositories for file changes, debouncing rapid
// successive writes into single events with a configurable quiet threshold.
type FileWatcher struct {
	debounce time.Duration
	watcher  *fsnotify.Watcher
	events   chan WatchEvent
	done     chan struct{}

	mu       sync.Mutex
	repoDirs map[string]bool // tracked repo directories
}

// NewFileWatcher creates a watcher with the given debounce duration.
// Events are emitted on the channel returned by Events().
func NewFileWatcher(debounce time.Duration) (*FileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	fw := &FileWatcher{
		debounce: debounce,
		watcher:  w,
		events:   make(chan WatchEvent, 64),
		done:     make(chan struct{}),
		repoDirs: make(map[string]bool),
	}
	go fw.loop()
	return fw, nil
}

// Add registers a repository directory for watching.
func (fw *FileWatcher) Add(repoPath string) error {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	fw.mu.Lock()
	fw.repoDirs[abs] = true
	fw.mu.Unlock()
	return fw.watcher.Add(abs)
}

// Remove stops watching a repository directory.
func (fw *FileWatcher) Remove(repoPath string) error {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}
	fw.mu.Lock()
	delete(fw.repoDirs, abs)
	fw.mu.Unlock()
	return fw.watcher.Remove(abs)
}

// Events returns a read-only channel of debounced watch events.
func (fw *FileWatcher) Events() <-chan WatchEvent {
	return fw.events
}

// Close shuts down the watcher and closes the events channel.
func (fw *FileWatcher) Close() error {
	err := fw.watcher.Close()
	<-fw.done // wait for loop to exit
	return err
}

// loop is the main event loop that collects fsnotify events and debounces
// them into WatchEvent batches per repository.
func (fw *FileWatcher) loop() {
	defer close(fw.done)
	defer close(fw.events)

	// pending tracks accumulated file changes per repo path, keyed by
	// the absolute repo directory.
	type pending struct {
		files map[string]bool
		timer *time.Timer
	}
	state := make(map[string]*pending)

	for {
		select {
		case ev, ok := <-fw.watcher.Events:
			if !ok {
				// Watcher closed; flush any pending events.
				for repo, p := range state {
					p.timer.Stop()
					fw.flush(repo, p.files)
				}
				return
			}

			// Only care about writes and creates.
			if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			absFile, err := filepath.Abs(ev.Name)
			if err != nil {
				continue
			}

			repo := fw.repoFor(absFile)
			if repo == "" {
				continue
			}

			p, ok := state[repo]
			if !ok {
				p = &pending{files: make(map[string]bool)}
				state[repo] = p
			}
			p.files[absFile] = true

			// Reset debounce timer.
			if p.timer != nil {
				p.timer.Stop()
			}
			p.timer = time.AfterFunc(fw.debounce, func() {
				fw.mu.Lock()
				files := p.files
				p.files = make(map[string]bool)
				fw.mu.Unlock()
				fw.flush(repo, files)
			})

		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Errors are logged but not fatal.
		}
	}
}

// repoFor finds which tracked repo directory contains the given file path.
func (fw *FileWatcher) repoFor(absFile string) string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	for dir := range fw.repoDirs {
		rel, err := filepath.Rel(dir, absFile)
		if err == nil && len(rel) > 0 && rel[0] != '.' {
			return dir
		}
	}
	return ""
}

// flush sends a WatchEvent for the given repo with accumulated files.
func (fw *FileWatcher) flush(repo string, files map[string]bool) {
	if len(files) == 0 {
		return
	}
	list := make([]string, 0, len(files))
	for f := range files {
		list = append(list, f)
	}
	// Non-blocking send; drop if consumer is too slow.
	select {
	case fw.events <- WatchEvent{RepoPath: repo, Files: list}:
	default:
	}
}
