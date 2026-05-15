package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate applies all pending SQL migrations in order. Each migration runs
// inside its own transaction and the schema_version table is updated after
// each successful migration.
func Migrate(db *sql.DB) error {
	// Ensure schema_version table exists.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("migrate: create schema_version: %w", err)
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("migrate: read version: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("migrate: read dir: %w", err)
	}

	// Sort entries by name so migrations apply in order.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		ver, err := parseVersion(name)
		if err != nil {
			return fmt.Errorf("migrate: parse version %q: %w", name, err)
		}
		if ver <= current {
			continue
		}

		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("migrate: begin tx for %s: %w", name, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: exec %s: %w", name, err)
		}

		if _, err := tx.Exec(`INSERT OR REPLACE INTO schema_version (version) VALUES (?)`, ver); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: update version for %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit %s: %w", name, err)
		}
	}

	return nil
}

func currentVersion(db *sql.DB) (int, error) {
	var v int
	err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// parseVersion extracts the leading numeric prefix from a migration filename
// like "001_initial_schema.sql" -> 1.
func parseVersion(name string) (int, error) {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("no version prefix in %q", name)
	}
	return strconv.Atoi(parts[0])
}
