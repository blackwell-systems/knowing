// Package roster manages the global registry of tracked repositories.
// The roster is stored at ~/.knowing/roster.json and maps repo paths to
// database paths and repo URLs. It is the canonical source of cross-repo
// identity: the indexer reads it to compute correct target hashes for
// cross-repo edges.
package roster

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Entry represents a tracked repository.
type Entry struct {
	Path string `json:"path"` // absolute path to repo root
	URL  string `json:"url"`  // repo URL (for graph identity)
	DB   string `json:"db"`   // path to this repo's database file
}

// Roster is the list of tracked repositories.
type Roster struct {
	Repos []Entry `json:"repos"`
}

// Dir returns the ~/.knowing directory, creating it if needed.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".knowing"
	}
	dir := filepath.Join(home, ".knowing")
	os.MkdirAll(dir, 0755)
	return dir
}

// Path returns the path to the roster file.
func Path() string {
	return filepath.Join(Dir(), "roster.json")
}

// DBPath returns the per-repo database path for a repo URL.
func DBPath(repoURL string) string {
	safe := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(repoURL)
	dir := filepath.Join(Dir(), "repos")
	os.MkdirAll(dir, 0755)
	return filepath.Join(dir, safe+".db")
}

// Load reads the roster from disk. Returns empty roster if file doesn't exist.
func Load() (*Roster, error) {
	data, err := os.ReadFile(Path())
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

// Save writes the roster to disk.
func Save(r *Roster) error {
	p := Path()
	os.MkdirAll(filepath.Dir(p), 0755)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

// Add registers a repo if not already present. Returns the DB path.
func Add(absPath, repoURL string) (string, error) {
	r, err := Load()
	if err != nil {
		return "", err
	}
	for _, entry := range r.Repos {
		if entry.Path == absPath {
			return entry.DB, nil // already tracked
		}
	}
	dbPath := DBPath(repoURL)
	r.Repos = append(r.Repos, Entry{Path: absPath, URL: repoURL, DB: dbPath})
	return dbPath, Save(r)
}

// Remove removes a repo from the roster by path.
func Remove(absPath string) error {
	r, err := Load()
	if err != nil {
		return err
	}
	filtered := make([]Entry, 0, len(r.Repos))
	for _, entry := range r.Repos {
		if entry.Path != absPath {
			filtered = append(filtered, entry)
		}
	}
	r.Repos = filtered
	return Save(r)
}

// DBForPath looks up the DB path for a repo by its absolute path.
func DBForPath(absPath string) string {
	r, err := Load()
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

// DBForCurrentDir finds the DB for the current directory by walking up to find
// a registered repo root.
func DBForCurrentDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	r, err := Load()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		for _, entry := range r.Repos {
			if entry.Path == dir {
				return entry.DB
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ModuleMap returns a map from Go module paths to repo URLs for all
// registered repos. This is the cross-repo identity map: it tells the
// indexer which repo URL to use when computing target hashes for
// cross-repo edges.
//
// The module path is read from go.mod in each repo's path. Repos without
// a go.mod (non-Go repos) are included with their URL as the key.
func ModuleMap() map[string]string {
	r, err := Load()
	if err != nil {
		return nil
	}
	result := make(map[string]string, len(r.Repos))
	for _, entry := range r.Repos {
		modPath := readGoMod(entry.Path)
		if modPath != "" {
			result[modPath] = entry.URL
		} else {
			result[entry.URL] = entry.URL
		}
	}
	return result
}

// readGoMod reads the module path from go.mod in the given directory.
func readGoMod(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module"))
		}
	}
	return ""
}
