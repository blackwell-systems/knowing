package crosssystem_test

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	"github.com/blackwell-systems/knowing/internal/store"
)

// filterAchievableGroundTruth removes ground truth symbols that don't exist
// in the indexed repo's database. This prevents penalizing systems for not
// finding symbols that were never extracted.
//
// Standard IR evaluation practice: you can't penalize a system for not
// retrieving documents that aren't in the corpus.
func filterAchievableGroundTruth(tasks []benchtype.Task, corpusDir string) []benchtype.Task {
	filtered := make([]benchtype.Task, len(tasks))
	copy(filtered, tasks)

	ctx := context.Background()
	stores := make(map[string]*store.SQLiteStore)

	for i := range filtered {
		repo := filtered[i].Repo
		if repo == "" {
			continue
		}

		// Open or reuse store for this repo.
		s, ok := stores[repo]
		if !ok {
			dbPath := filepath.Join(corpusDir, repo, ".knowing", "graph.db")
			var err error
			s, err = store.NewSQLiteStore(dbPath)
			if err != nil {
				continue
			}
			stores[repo] = s
		}

		var achievable []string
		for _, gt := range filtered[i].GroundTruth {
			// Extract terminal name (last dot-separated component)
			parts := strings.Split(gt, ".")
			terminal := parts[len(parts)-1]
			if terminal == "" {
				continue
			}

			// Check if any node's qualified name contains this terminal name.
			// Using NodesByName which does a LIKE query.
			nodes, err := s.NodesByName(ctx, "%"+terminal+"%")
			if err == nil && len(nodes) > 0 {
				achievable = append(achievable, gt)
			}
		}

		if len(achievable) > 0 {
			filtered[i].GroundTruth = achievable
		}
	}

	// Close all stores.
	for _, s := range stores {
		s.Close()
	}

	return filtered
}
