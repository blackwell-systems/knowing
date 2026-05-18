package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RosterEntry represents a tracked repository.
type RosterEntry struct {
	Path string `json:"path"` // absolute path to repo root
	URL  string `json:"url"`  // repo URL (for graph identity)
	DB   string `json:"db"`   // path to this repo's database file
}

// Roster is the list of tracked repositories, stored at ~/.knowing/roster.json.
type Roster struct {
	Repos []RosterEntry `json:"repos"`
}

// knowingDir returns the ~/.knowing directory, creating it if needed.
func knowingDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".knowing"
	}
	dir := filepath.Join(home, ".knowing")
	os.MkdirAll(dir, 0755)
	return dir
}

// rosterPath returns the path to the roster file.
func rosterPath() string {
	return filepath.Join(knowingDir(), "roster.json")
}

// repoDBPath returns the per-repo database path.
// Converts the repo URL into a safe filename: github.com/org/repo -> github.com-org-repo.db
func repoDBPath(repoURL string) string {
	safe := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(repoURL)
	dir := filepath.Join(knowingDir(), "repos")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, safe+".db")
}

// dbForRepo looks up the DB path for a repo in the roster. Returns empty string if not found.
func dbForRepo(absPath string) string {
	r, err := loadRoster()
	if err != nil {
		return ""
	}
	for _, entry := range r.Repos {
		if entry.Path == absPath {
			return entry.DB
		}
	}
	return ""
}

// dbForCurrentDir looks up the DB path for the current directory (or a parent that's a repo root).
func dbForCurrentDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	// Walk up to find a registered repo root.
	dir := cwd
	for {
		if db := dbForRepo(dir); db != "" {
			return db
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// loadRoster reads the roster from disk. Returns empty roster if file doesn't exist.
func loadRoster() (*Roster, error) {
	path := rosterPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Roster{}, nil
		}
		return nil, err
	}
	var r Roster
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// saveRoster writes the roster to disk.
func saveRoster(r *Roster) error {
	path := rosterPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// addToRoster adds a repo to the roster if not already present.
// Each repo gets its own DB file at ~/.knowing/repos/<safe-name>.db.
func addToRoster(absPath, repoURL string) error {
	r, err := loadRoster()
	if err != nil {
		return err
	}
	for _, entry := range r.Repos {
		if entry.Path == absPath {
			return nil // already tracked
		}
	}
	dbPath := repoDBPath(repoURL)
	r.Repos = append(r.Repos, RosterEntry{Path: absPath, URL: repoURL, DB: dbPath})
	return saveRoster(r)
}

// removeFromRoster removes a repo from the roster by path.
func removeFromRoster(absPath string) error {
	r, err := loadRoster()
	if err != nil {
		return err
	}
	filtered := make([]RosterEntry, 0, len(r.Repos))
	found := false
	for _, entry := range r.Repos {
		if entry.Path == absPath {
			found = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !found {
		return fmt.Errorf("repo not in roster: %s", absPath)
	}
	r.Repos = filtered
	return saveRoster(r)
}

// cmdAdd registers a repository in the global roster and indexes it.
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
		*repoURL = detectRepoURL(absPath)
	}
	if *repoURL == "" {
		*repoURL = absPath
	}

	if err := addToRoster(absPath, *repoURL); err != nil {
		return fmt.Errorf("adding to roster: %w", err)
	}

	dbPath := dbForRepo(absPath)
	fmt.Printf("Added %s (%s) to roster\n", absPath, *repoURL)
	fmt.Printf("  Database: %s\n", dbPath)
	fmt.Println()
	fmt.Println("Run 'knowing index' from the repo directory to index it.")
	return nil
}

// cmdRemove removes a repository from the roster.
func cmdRemove(args []string) error {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
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

	if err := removeFromRoster(absPath); err != nil {
		return err
	}

	fmt.Printf("Removed %s from roster\n", absPath)
	return nil
}

// cmdList lists all tracked repositories.
func cmdList(args []string) error {
	r, err := loadRoster()
	if err != nil {
		return fmt.Errorf("loading roster: %w", err)
	}

	if len(r.Repos) == 0 {
		fmt.Println("No tracked repositories. Run 'knowing add .' to track a repo.")
		return nil
	}

	fmt.Printf("Tracked repositories (%s):\n\n", rosterPath())
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
