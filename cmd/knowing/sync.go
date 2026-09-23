package main

import (
	stdctx "context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/internal/daemon"
	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/roster"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdSync reindexes a tracked repository so the graph stays current after a
// `git pull` / branch switch, with zero agent action. It is the command a git
// post-merge / post-checkout hook invokes (install them with `knowing sync
// install`). It reindexes the repo and scopes LSP enrichment to the files that
// changed since the last indexed commit (read from the latest snapshot), so a
// routine pull refreshes in seconds instead of a full re-enrichment.
//
//	knowing sync              reindex the repo at the cwd
//	knowing sync --roster     reindex every repo in the roster
//	knowing sync install      install git post-merge/post-checkout hooks
//	knowing sync uninstall    remove the installed hooks
func cmdSync(args []string) error {
	// Subcommands: install / uninstall the git hooks.
	if len(args) > 0 {
		switch args[0] {
		case "install":
			return cmdSyncInstall(args[1:])
		case "uninstall":
			return cmdSyncUninstall(args[1:])
		}
	}

	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	allRoster := fs.Bool("roster", false, "Sync every repository in the roster, not just the cwd")
	noEnrich := fs.Bool("no-enrich", false, "Skip LSP enrichment (tree-sitter edges only)")
	enrichConcurrency := fs.Int("enrich-concurrency", 0, "Parallel LSP enrichment requests (0 = default)")
	quiet := fs.Bool("quiet", false, "Suppress per-repo output (git hooks run quiet by default)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := stdctx.Background()

	if *allRoster {
		r, err := roster.Load()
		if err != nil {
			return fmt.Errorf("loading roster: %w", err)
		}
		if len(r.Repos) == 0 {
			if !*quiet {
				fmt.Fprintln(os.Stderr, "knowing sync: no tracked repositories (run 'knowing add')")
			}
			return nil
		}
		var firstErr error
		for _, e := range r.Repos {
			if err := syncRepo(ctx, e.Path, e.URL, e.DB, !*noEnrich, *enrichConcurrency, *quiet); err != nil {
				fmt.Fprintf(os.Stderr, "knowing sync: %s: %v\n", e.Path, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
		return firstErr
	}

	// Single-repo sync: resolve the git root at the cwd and its roster entry.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving cwd: %w", err)
	}
	gitRoot := detectGitRoot(cwd)
	if gitRoot == "" {
		if !*quiet {
			fmt.Fprintln(os.Stderr, "knowing sync: not inside a git repository")
		}
		return nil
	}

	entry := rosterEntryForPath(gitRoot)
	if entry == nil {
		// Untracked repo: stay a no-op so an installed hook never errors on a
		// repo the user did not opt into. `sync install` adds it to the roster.
		if !*quiet {
			fmt.Fprintf(os.Stderr, "knowing sync: %s is not tracked (run 'knowing add' or 'knowing sync install')\n", gitRoot)
		}
		return nil
	}

	return syncRepo(ctx, entry.Path, entry.URL, entry.DB, !*noEnrich, *enrichConcurrency, *quiet)
}

// syncRepo reindexes a single repository and scope-enriches the files that
// changed since its last indexed commit.
func syncRepo(ctx stdctx.Context, repoRoot, repoURL, dbPath string, enrich bool, enrichConcurrency int, quiet bool) error {
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	repoHash := types.NewHash([]byte(repoURL))

	// Determine the changed-file set BEFORE reindexing, by diffing the last
	// indexed commit (recorded in the latest snapshot) against HEAD. This
	// scopes enrichment to what actually moved on the pull.
	head, _ := daemon.GitHeadCommit(repoRoot)
	var lastCommit string
	if snap, err := st.LatestSnapshot(ctx, repoHash); err == nil && snap != nil {
		lastCommit = snap.CommitHash
	}
	changed, determined := changedFilesSince(repoRoot, lastCommit, head)

	// Reindex. IndexRepo re-extracts the whole tree, but content-addressing
	// makes unchanged files cheap. Pass the same repoURL used for repoHash so
	// enrichment targets the snapshot IndexRepo just wrote (avoids the
	// index/enrich repo-hash mismatch, roadmap #14).
	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	registerAllExtractors(idx, false)

	start := time.Now()
	snap, err := idx.IndexRepo(ctx, repoURL, repoRoot, head)
	if err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	if enrich {
		if err := runInProcessResolver(ctx, st, repoRoot, repoHash); err != nil {
			fmt.Fprintf(os.Stderr, "  in-process resolver warning: %v\n", err)
		}
		enricher := enrichment.NewEnricher(st, repoRoot)
		if enrichConcurrency > 0 {
			enricher.SetConcurrency(enrichConcurrency)
		}
		var enrichErr error
		switch {
		case lastCommit == "" || !determined:
			// First index, or the changed set couldn't be resolved (missing HEAD,
			// or the old commit is gone after a rebase/gc): enrich the whole repo.
			// Falling back to full is the safe choice: skipping here would leave
			// genuinely-changed files un-enriched.
			enrichErr = enricher.Run(ctx, repoHash)
		case len(changed) > 0:
			enrichErr = enricher.RunScoped(ctx, repoHash, changed)
		default:
			// determined AND no files changed: nothing to enrich.
		}
		if enrichErr != nil {
			fmt.Fprintf(os.Stderr, "  enrichment warning: %v\n", enrichErr)
		}
		enricher.Close(ctx)
	}

	if !quiet {
		var scope string
		switch {
		case lastCommit == "" || !determined:
			scope = "full"
		case len(changed) > 0:
			scope = fmt.Sprintf("%d changed file(s)", len(changed))
		default:
			scope = "no changes"
		}
		fmt.Printf("synced %s in %v (nodes: %d, edges: %d, %s)\n",
			repoURL, time.Since(start).Round(time.Millisecond),
			snap.NodeCount, snap.EdgeCount, scope)
	}
	return nil
}

// rosterEntryForPath returns the roster entry whose path matches gitRoot, or nil.
func rosterEntryForPath(gitRoot string) *roster.Entry {
	abs, err := filepath.Abs(gitRoot)
	if err != nil {
		abs = gitRoot
	}
	r, err := roster.Load()
	if err != nil {
		return nil
	}
	for i := range r.Repos {
		if r.Repos[i].Path == abs {
			return &r.Repos[i]
		}
	}
	return nil
}

// changedFilesSince returns repo-relative paths changed between two commits and
// whether the range could be resolved. The distinction matters: determined ==
// false (with nil files) means the diff could NOT be computed (missing HEAD, or
// the old commit is gone after a rebase/gc), so the caller must fall back to a
// full-repo operation rather than assume nothing changed. determined == true
// with an empty slice means the commits are identical: genuinely no changes.
func changedFilesSince(repoRoot, oldCommit, newCommit string) (files []string, determined bool) {
	if oldCommit == "" || newCommit == "" {
		return nil, false
	}
	if oldCommit == newCommit {
		return nil, true // identical commits: nothing changed
	}
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", oldCommit, newCommit)
	out, err := cmd.Output()
	if err != nil {
		return nil, false // old commit may be gone (rebase/gc); fall back to full
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, true
}

// --- git hook installation ---

const (
	syncHookMarkerStart = "# knowing:sync:start"
	syncHookMarkerEnd   = "# knowing:sync:end"
)

// postMergeHookBody reindexes after `git pull` / `git merge`.
const postMergeHookBody = syncHookMarkerStart + `
# Refresh the knowing graph after a pull/merge. Non-blocking, never fails the hook.
command -v knowing >/dev/null 2>&1 && knowing sync --quiet >/dev/null 2>&1 &
` + syncHookMarkerEnd

// postCheckoutHookBody reindexes on branch switch only ($3 == 1), not file checkouts.
const postCheckoutHookBody = syncHookMarkerStart + `
# Refresh the knowing graph on branch switch. Non-blocking, never fails the hook.
[ "$3" = "1" ] && command -v knowing >/dev/null 2>&1 && knowing sync --quiet >/dev/null 2>&1 &
true
` + syncHookMarkerEnd

func cmdSyncInstall(args []string) error {
	fs := flag.NewFlagSet("sync install", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving cwd: %w", err)
	}
	gitRoot := detectGitRoot(cwd)
	if gitRoot == "" {
		return fmt.Errorf("not inside a git repository")
	}

	// Ensure the repo is tracked so `knowing sync` can resolve its DB.
	url := detectRepoURL(gitRoot)
	if url == "" {
		url = gitRoot
	}
	if _, err := roster.Add(gitRoot, url); err != nil {
		return fmt.Errorf("registering repo in roster: %w", err)
	}

	hooksDir, err := gitHooksDir(gitRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}

	for _, h := range []struct {
		name, body string
	}{
		{"post-merge", postMergeHookBody},
		{"post-checkout", postCheckoutHookBody},
	} {
		path := filepath.Join(hooksDir, h.name)
		if err := installGitHook(path, h.body); err != nil {
			return fmt.Errorf("installing %s: %w", h.name, err)
		}
		fmt.Printf("installed %s hook: %s\n", h.name, path)
	}

	fmt.Printf("knowing will now reindex %s on pull and branch switch.\n", url)
	return nil
}

func cmdSyncUninstall(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving cwd: %w", err)
	}
	gitRoot := detectGitRoot(cwd)
	if gitRoot == "" {
		return fmt.Errorf("not inside a git repository")
	}
	hooksDir, err := gitHooksDir(gitRoot)
	if err != nil {
		return err
	}
	for _, name := range []string{"post-merge", "post-checkout"} {
		path := filepath.Join(hooksDir, name)
		removed, err := uninstallGitHook(path)
		if err != nil {
			return fmt.Errorf("uninstalling %s: %w", name, err)
		}
		if removed {
			fmt.Printf("removed knowing block from %s\n", path)
		}
	}
	return nil
}

// gitHooksDir resolves the hooks directory, honoring worktrees, submodules, and
// core.hooksPath via `git rev-parse --git-path hooks`.
func gitHooksDir(gitRoot string) (string, error) {
	cmd := exec.Command("git", "-C", gitRoot, "rev-parse", "--git-path", "hooks")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolving git hooks dir: %w", err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(gitRoot, p)
	}
	return p, nil
}

// installGitHook writes our marker-delimited block into a hook file
// non-destructively: creates the file with a shebang if absent, replaces an
// existing knowing block, or appends after any pre-existing hook content.
func installGitHook(path, body string) error {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		content := "#!/bin/sh\n" + body + "\n"
		return os.WriteFile(path, []byte(content), 0o755)
	}
	if err != nil {
		return err
	}

	content := string(existing)
	start := strings.Index(content, syncHookMarkerStart)
	end := strings.Index(content, syncHookMarkerEnd)
	if start >= 0 && end >= 0 {
		end += len(syncHookMarkerEnd)
		content = content[:start] + body + content[end:]
	} else {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + body + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	// Preserve executability for a hook that pre-existed without it.
	return os.Chmod(path, 0o755)
}

// uninstallGitHook strips our marker block. If the file becomes just a shebang
// (it was ours alone), it is removed entirely. Returns whether anything changed.
func uninstallGitHook(path string) (bool, error) {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	content := string(existing)
	start := strings.Index(content, syncHookMarkerStart)
	end := strings.Index(content, syncHookMarkerEnd)
	if start < 0 || end < 0 {
		return false, nil
	}
	end += len(syncHookMarkerEnd)
	stripped := strings.TrimRight(content[:start], "\n") + content[end:]

	if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stripped), "#!/bin/sh")) == "" {
		// Nothing but the shebang remains: the hook was ours alone.
		return true, os.Remove(path)
	}
	if !strings.HasSuffix(stripped, "\n") {
		stripped += "\n"
	}
	return true, os.WriteFile(path, []byte(stripped), 0o755)
}
