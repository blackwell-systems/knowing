package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdEnrich runs offline enrichment passes that stamp per-symbol metadata.
func cmdEnrich(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: knowing enrich {blame} [flags] <repo-path>")
	}

	switch args[0] {
	case "blame":
		return cmdEnrichBlame(args[1:])
	default:
		return fmt.Errorf("unknown enrichment pass: %s (available: blame)", args[0])
	}
}

// blameLine holds parsed git blame output for a single line.
type blameLine struct {
	author   string
	commitAt int64 // unix timestamp
}

// cmdEnrichBlame stamps last-author and last-commit-at on every symbol
// by running git blame on each file that has nodes in the graph.
func cmdEnrichBlame(args []string) error {
	fs := flag.NewFlagSet("enrich blame", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing enrich blame [flags] <repo-path>")
	}
	repoPath, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if *repoURL == "" {
		*repoURL = detectRepoURL(repoPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()
	repoHash := types.NewHash([]byte(*repoURL))

	// Get all files for this repo.
	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("listing files: %w", err)
	}

	start := time.Now()
	stamped := 0
	skipped := 0
	errors := 0

	for _, f := range files {
		// Get nodes in this file.
		nodes, err := st.NodesByFilePath(ctx, repoHash, f.Path)
		if err != nil || len(nodes) == 0 {
			continue
		}

		// Run git blame on the file.
		absPath := filepath.Join(repoPath, f.Path)
		if _, err := os.Stat(absPath); err != nil {
			skipped++
			continue
		}

		blameData, err := runGitBlame(repoPath, f.Path)
		if err != nil {
			errors++
			continue
		}

		// Stamp each node with the blame data for its line.
		for _, n := range nodes {
			if n.Line <= 0 || n.Line > len(blameData) {
				continue
			}
			bl := blameData[n.Line-1] // 0-indexed array, 1-indexed lines
			if bl.author == "" {
				continue
			}
			if err := st.UpdateNodeBlame(ctx, n.NodeHash, bl.author, bl.commitAt); err != nil {
				errors++
				continue
			}
			stamped++
		}
	}

	log.Printf("Blame enrichment complete: %d symbols stamped, %d files skipped, %d errors (%v)",
		stamped, skipped, errors, time.Since(start).Round(time.Millisecond))
	return nil
}

// runGitBlame runs git blame with porcelain output and returns per-line data.
func runGitBlame(repoPath, filePath string) ([]blameLine, error) {
	cmd := exec.Command("git", "blame", "--porcelain", filePath)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git blame %s: %w", filePath, err)
	}

	return parseBlamePorcelain(string(out))
}

// parseBlamePorcelain parses git blame --porcelain output into per-line data.
// Porcelain format:
//
//	<sha1> <orig-line> <final-line> [<num-lines>]
//	author <name>
//	author-mail <email>
//	author-time <timestamp>
//	...
//	\t<line-content>
func parseBlamePorcelain(output string) ([]blameLine, error) {
	var lines []blameLine
	scanner := bufio.NewScanner(strings.NewReader(output))

	var currentAuthor string
	var currentTime int64

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "author ") {
			currentAuthor = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "author-time ") {
			ts, err := strconv.ParseInt(strings.TrimPrefix(line, "author-time "), 10, 64)
			if err == nil {
				currentTime = ts
			}
		} else if strings.HasPrefix(line, "\t") {
			// This is a content line, meaning the current blame block is complete for this line.
			lines = append(lines, blameLine{
				author:   currentAuthor,
				commitAt: currentTime,
			})
		}
	}

	return lines, scanner.Err()
}
