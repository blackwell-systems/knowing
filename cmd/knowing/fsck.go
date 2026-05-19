package main

import (
	"context"
	"flag"
	"fmt"
	"os"

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
		verifyErrs, err := sm.Verify(ctx, r.RepoHash)
		if err != nil {
			return fmt.Errorf("verifying repo %s: %w", r.RepoURL, err)
		}
		allErrors = append(allErrors, verifyErrs...)
	}

	errorCount := 0
	warnCount := 0
	for _, ve := range allErrors {
		fmt.Fprintf(os.Stderr, "[%s] %s: %s (hash: %s)\n", ve.Level, ve.Kind, ve.Message, ve.Hash)
		switch ve.Level {
		case "ERROR":
			errorCount++
		case "WARN":
			warnCount++
		}
	}

	fmt.Printf("Checked %d repos: %d errors, %d warnings\n", len(repos), errorCount, warnCount)

	if errorCount > 0 {
		return fmt.Errorf("integrity check failed: %d errors found", errorCount)
	}
	return nil
}
