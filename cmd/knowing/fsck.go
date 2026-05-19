package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/blackwell-systems/knowing/internal/roster"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdFsck verifies graph integrity: referential checks, hash recomputation,
// and snapshot chain continuity. It is read-only and never modifies data.
func cmdFsck(args []string) error {
	fs := flag.NewFlagSet("fsck", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	quick := fs.Bool("quick", false, "Run PRAGMA integrity_check only (fast, no graph checks)")
	repo := fs.String("repo", "", "Repository URL to verify (default: all repos)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	if *quick {
		if err := st.IntegrityCheck(ctx); err != nil {
			return fmt.Errorf("integrity check failed: %w", err)
		}
		fmt.Println("ok")
		return nil
	}

	// Load the global roster to classify dangling edges as cross-repo vs truly dangling.
	rosterDBPaths := collectRosterDBPaths()

	sm := snapshot.NewSnapshotManager(st)

	var repos []types.Repo
	if *repo != "" {
		repoHash := types.NewHash([]byte(*repo))
		r, err := st.GetRepo(ctx, repoHash)
		if err != nil {
			return fmt.Errorf("getting repo: %w", err)
		}
		if r == nil {
			return fmt.Errorf("repo not found: %s", *repo)
		}
		repos = []types.Repo{*r}
	} else {
		repos, err = st.AllRepos(ctx)
		if err != nil {
			return fmt.Errorf("listing repos: %w", err)
		}
	}

	var allErrors []snapshot.VerifyError
	for _, r := range repos {
		verifyErrs, err := sm.Verify(ctx, r.RepoHash, rosterDBPaths, *dbPath)
		if err != nil {
			return fmt.Errorf("verifying repo %s: %w", r.RepoURL, err)
		}
		allErrors = append(allErrors, verifyErrs...)
	}

	errorCount := 0
	warnCount := 0
	infoCount := 0
	crossRepoCount := 0
	stdlibCount := 0
	trulyDanglingCount := 0

	for _, ve := range allErrors {
		switch ve.Level {
		case "ERROR":
			errorCount++
			fmt.Fprintf(os.Stderr, "[%s] %s: %s (hash: %s)\n", ve.Level, ve.Kind, ve.Message, ve.Hash)
		case "WARN":
			warnCount++
			fmt.Fprintf(os.Stderr, "[%s] %s: %s (hash: %s)\n", ve.Level, ve.Kind, ve.Message, ve.Hash)
		case "INFO":
			infoCount++
		}

		// Count by classification.
		switch ve.Classification {
		case snapshot.ClassCrossRepo:
			crossRepoCount++
		case snapshot.ClassStdlib:
			stdlibCount++
		case snapshot.ClassTrulyDangling:
			trulyDanglingCount++
		}
	}

	fmt.Printf("Checked %d repos: %d errors, %d warnings, %d info\n", len(repos), errorCount, warnCount, infoCount)
	if crossRepoCount > 0 || stdlibCount > 0 || trulyDanglingCount > 0 {
		fmt.Printf("Dangling edges: %d cross-repo, %d stdlib, %d truly dangling\n", crossRepoCount, stdlibCount, trulyDanglingCount)
	}

	if errorCount > 0 {
		return fmt.Errorf("integrity check failed: %d errors found", errorCount)
	}
	return nil
}

// collectRosterDBPaths loads the global roster and returns the DB paths
// of all tracked repositories. Returns nil if the roster cannot be loaded.
func collectRosterDBPaths() []string {
	r, err := roster.Load()
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range r.Repos {
		if entry.DB != "" {
			paths = append(paths, entry.DB)
		}
	}
	return paths
}
