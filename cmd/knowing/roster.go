package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blackwell-systems/knowing/internal/roster"
)

// knowingDir delegates to the shared roster package.
func knowingDir() string { return roster.Dir() }

// defaultDB returns the database path for the current working directory.
// Checks the roster first, falls back to ~/.knowing/knowing.db.
func defaultDB() string {
	if db := roster.DBForCurrentDir(); db != "" {
		return db
	}
	if envDB := os.Getenv("KNOWING_DB"); envDB != "" {
		return envDB
	}
	return filepath.Join(knowingDir(), "knowing.db")
}

// cmdAdd registers a repository in the global roster.
func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if *repoURL == "" {
		gitRoot := detectGitRoot(absPath)
		if gitRoot != "" {
			*repoURL = detectRepoURL(gitRoot)
		}
	}
	if *repoURL == "" {
		*repoURL = absPath
	}

	dbPath, err := roster.Add(absPath, *repoURL)
	if err != nil {
		return fmt.Errorf("adding to roster: %w", err)
	}

	fmt.Printf("Added %s (%s) to roster\n", absPath, *repoURL)
	fmt.Printf("  Database: %s\n", dbPath)
	fmt.Println()
	fmt.Println("Run 'knowing index' from the repo directory to index it.")
	return nil
}

// cmdRemove removes a repository from the roster.
func cmdRemove(args []string) error {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	purge := fs.Bool("purge", false, "Also delete the database file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Get the DB path before removing from roster.
	dbPath := roster.DBForPath(absPath)

	if err := roster.Remove(absPath); err != nil {
		return err
	}

	fmt.Printf("Removed %s from roster\n", absPath)

	if *purge && dbPath != "" {
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("deleting database %s: %w", dbPath, err)
		}
		fmt.Printf("Deleted database: %s\n", dbPath)
	}

	return nil
}

// cmdList lists all tracked repositories.
func cmdList(args []string) error {
	r, err := roster.Load()
	if err != nil {
		return fmt.Errorf("loading roster: %w", err)
	}

	if len(r.Repos) == 0 {
		fmt.Println("No tracked repositories. Run 'knowing add .' to track a repo.")
		return nil
	}

	fmt.Printf("Tracked repositories (%s):\n\n", roster.Path())
	for _, entry := range r.Repos {
		exists := "ok"
		if _, err := os.Stat(entry.Path); err != nil {
			exists = "missing"
		}
		dbExists := "no data"
		if info, err := os.Stat(entry.DB); err == nil && info.Size() > 0 {
			dbExists = fmt.Sprintf("%.1f MB", float64(info.Size())/(1024*1024))
		}
		fmt.Printf("  %s\n", entry.Path)
		fmt.Printf("    url: %s\n", entry.URL)
		fmt.Printf("    db:  %s (%s)\n", entry.DB, dbExists)
		fmt.Printf("    status: %s\n", exists)
	}
	return nil
}
