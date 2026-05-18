package eval

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

func TestVerifyFixtures(t *testing.T) {
	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	dbPath := t.TempDir() + "/verify.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")

	allNodes, _ := st.NodesByName(ctx, "%")

	fixtures := loadFixtures(t, filepath.Join(repoRoot, "eval", "fixtures"))
	for _, fix := range fixtures {
		short := fix.Task
		if len(short) > 50 {
			short = short[:50] + "..."
		}
		for _, gt := range fix.GroundTruth {
			found := false
			for _, n := range allNodes {
				if isRelevant(n.QualifiedName, []string{gt}) {
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("MISSING [%s] %s: %s\n", fix.Difficulty, short, gt)
			}
		}
	}
}
