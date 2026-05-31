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

	"github.com/blackwell-systems/knowing/internal/embedding"
	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdEnrich runs offline enrichment passes that stamp per-symbol metadata.
func cmdEnrich(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: knowing enrich {lsp|blame|coverage} [flags] <repo-path>")
	}

	switch args[0] {
	case "blame":
		return cmdEnrichBlame(args[1:])
	case "coverage":
		return cmdEnrichCoverage(args[1:])
	case "lsp":
		return cmdEnrichLSP(args[1:])
	case "embeddings":
		return cmdEnrichEmbeddings(args[1:])
	case "resolver":
		return cmdEnrichResolver(args[1:])
	default:
		return fmt.Errorf("unknown enrichment pass: %s (available: blame, coverage, lsp, embeddings, resolver)", args[0])
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

// coverageBlock represents a block of code with coverage data from a Go cover profile.
type coverageBlock struct {
	startLine  int
	startCol   int
	endLine    int
	endCol     int
	statements int
	count      int // 0 = not covered, >0 = covered
}

// cmdEnrichCoverage stamps coverage percentage on symbols from a Go cover profile.
func cmdEnrichCoverage(args []string) error {
	fs := flag.NewFlagSet("enrich coverage", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	profile := fs.String("profile", "cover.out", "Path to Go cover profile")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing enrich coverage [flags] <repo-path>")
	}
	repoPath, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if *repoURL == "" {
		*repoURL = detectRepoURL(repoPath)
	}

	// Parse cover profile.
	profilePath := *profile
	if !filepath.IsAbs(profilePath) {
		profilePath = filepath.Join(repoPath, profilePath)
	}
	blocks, err := parseCoverProfile(profilePath)
	if err != nil {
		return fmt.Errorf("parsing cover profile: %w", err)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()
	repoHash := types.NewHash([]byte(*repoURL))

	files, err := st.FilesByRepo(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("listing files: %w", err)
	}

	start := time.Now()
	stamped := 0
	skipped := 0

	for _, f := range files {
		nodes, err := st.NodesByFilePath(ctx, repoHash, f.Path)
		if err != nil || len(nodes) == 0 {
			continue
		}

		// Find coverage blocks for this file.
		// The cover profile uses module-relative paths (e.g., "github.com/org/repo/internal/store/sqlite.go").
		// We need to match against our file path.
		fileBlocks := findBlocksForFile(blocks, f.Path, *repoURL)
		if len(fileBlocks) == 0 {
			skipped++
			continue
		}

		for _, n := range nodes {
			if n.Line <= 0 {
				continue
			}
			pct := computeCoverageForLine(fileBlocks, n.Line)
			if err := st.UpdateNodeCoverage(ctx, n.NodeHash, pct); err != nil {
				continue
			}
			stamped++
		}
	}

	log.Printf("Coverage enrichment complete: %d symbols stamped, %d files without coverage data (%v)",
		stamped, skipped, time.Since(start).Round(time.Millisecond))
	return nil
}

// parseCoverProfile reads a Go cover profile and returns coverage blocks
// grouped by file path.
func parseCoverProfile(path string) (map[string][]coverageBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	blocks := make(map[string][]coverageBlock)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		// Skip mode line: "mode: set" or "mode: count" or "mode: atomic"
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		// Format: file:startLine.startCol,endLine.endCol numStatements count
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}
		filePath := line[:colonIdx]
		rest := line[colonIdx+1:]

		// Parse "startLine.startCol,endLine.endCol numStatements count"
		var startLine, startCol, endLine, endCol, stmts, count int
		n, err := fmt.Sscanf(rest, "%d.%d,%d.%d %d %d",
			&startLine, &startCol, &endLine, &endCol, &stmts, &count)
		if err != nil || n != 6 {
			continue
		}

		blocks[filePath] = append(blocks[filePath], coverageBlock{
			startLine:  startLine,
			startCol:   startCol,
			endLine:    endLine,
			endCol:     endCol,
			statements: stmts,
			count:      count,
		})
	}

	return blocks, scanner.Err()
}

// findBlocksForFile finds coverage blocks matching a relative file path.
// Cover profiles use full module paths (e.g., "github.com/org/repo/internal/foo.go"),
// so we match by suffix.
func findBlocksForFile(allBlocks map[string][]coverageBlock, relPath, repoURL string) []coverageBlock {
	// Try exact module path match first.
	fullPath := repoURL + "/" + relPath
	if blocks, ok := allBlocks[fullPath]; ok {
		return blocks
	}
	// Fallback: suffix match.
	for filePath, blocks := range allBlocks {
		if strings.HasSuffix(filePath, "/"+relPath) {
			return blocks
		}
	}
	return nil
}

// computeCoverageForLine computes the coverage percentage for a symbol at
// a given line. Looks at all blocks that overlap the line and returns
// the percentage of those blocks that are covered.
func computeCoverageForLine(blocks []coverageBlock, line int) float64 {
	covered := 0
	total := 0
	for _, b := range blocks {
		if line >= b.startLine && line <= b.endLine {
			total += b.statements
			if b.count > 0 {
				covered += b.statements
			}
		}
	}
	if total == 0 {
		return -1 // no coverage data for this line
	}
	return float64(covered) / float64(total) * 100
}

// cmdEnrichEmbeddings batch-embeds all nodes in the database. Persists vectors
// to the embeddings table so brute-force vector search (gap-fill) works without
// the HNSW index rebuild. Incremental: skips nodes that already have vectors.
func cmdEnrichEmbeddings(args []string) error {
	fs := flag.NewFlagSet("enrich embeddings", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	batchSize := fs.Int("batch", 32, "Embedding batch size")
	model := fs.String("model", "", "Embedding model (default: jina-code)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *model != "" {
		os.Setenv("KNOWING_EMBED_MODEL", *model)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Load all nodes.
	nodes, err := st.NodesByName(ctx, "%")
	if err != nil || len(nodes) == 0 {
		return fmt.Errorf("no nodes found in database (run 'knowing index' first)")
	}

	// Check which nodes already have embeddings.
	allHashes := make([]types.Hash, len(nodes))
	for i, n := range nodes {
		allHashes[i] = n.NodeHash
	}

	// Initialize embedder.
	embedder, err := embedding.New()
	if err != nil {
		return fmt.Errorf("initializing embedder: %w", err)
	}
	defer embedder.Close()

	searcher := embedding.NewSearcher(embedder)
	searcher.SetStore(st)

	// Get existing embeddings to skip. Use the searcher's model key
	// (matches the model name used when persisting vectors).
	existing, err := st.GetAllEmbeddings(ctx, searcher.Model())
	if err != nil {
		existing = nil // proceed without skip optimization
	}

	// Filter to nodes that need embedding. Skip phantom external nodes
	// (meaningless names like "external://calls.target" waste embedding inference
	// and gap-fill already filters them from results).
	var toEmbed []types.Node
	skippedPhantoms := 0
	for _, n := range nodes {
		if n.Kind == "external" || strings.HasPrefix(n.QualifiedName, "external://") {
			skippedPhantoms++
			continue
		}
		if _, ok := existing[n.NodeHash]; !ok {
			toEmbed = append(toEmbed, n)
		}
	}

	realNodes := len(nodes) - skippedPhantoms
	if len(toEmbed) == 0 {
		fmt.Fprintf(os.Stderr, "All %d real nodes already have embeddings (%d phantoms skipped). Nothing to do.\n",
			realNodes, skippedPhantoms)
		return nil
	}

	fmt.Fprintf(os.Stderr, "Embedding %d nodes (%d already cached, %d phantoms skipped, %d total)...\n",
		len(toEmbed), realNodes-len(toEmbed), skippedPhantoms, len(nodes))

	start := time.Now()
	embedded := 0

	for i := 0; i < len(toEmbed); i += *batchSize {
		end := i + *batchSize
		if end > len(toEmbed) {
			end = len(toEmbed)
		}
		batch := toEmbed[i:end]
		filePaths := make([]string, len(batch))

		if err := searcher.IndexBatch(ctx, batch, filePaths); err != nil {
			log.Printf("warning: batch %d failed: %v", i / *batchSize, err)
			continue
		}
		embedded += len(batch)

		if embedded%1000 == 0 || end == len(toEmbed) {
			elapsed := time.Since(start)
			rate := float64(embedded) / elapsed.Seconds()
			remaining := float64(len(toEmbed)-embedded) / rate
			fmt.Fprintf(os.Stderr, "  [%d/%d] %.0f/sec, ETA %.0fs\n",
				embedded, len(toEmbed), rate, remaining)
		}
	}

	fmt.Fprintf(os.Stderr, "Embedding complete: %d nodes in %v\n",
		embedded, time.Since(start).Round(time.Millisecond))
	return nil
}

// cmdEnrichLSP runs LSP enrichment on an already-indexed database.
// Opens the existing DB, detects language servers, upgrades edge confidence
// from ast_inferred to lsp_resolved, and discovers new cross-module edges.
func cmdEnrichLSP(args []string) error {
	fs := flag.NewFlagSet("enrich lsp", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	concurrency := fs.Int("concurrency", 8, "Number of parallel LSP requests")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing enrich lsp [flags] <repo-path>")
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

	// Verify the DB has nodes (enrichment needs an already-indexed graph).
	nodes, err := st.NodesByName(ctx, "%")
	if err != nil || len(nodes) == 0 {
		return fmt.Errorf("no nodes found in database (run 'knowing index' first)")
	}
	fmt.Fprintf(os.Stderr, "LSP enrichment: %d nodes in %s\n", len(nodes), *dbPath)

	enricher := enrichment.NewEnricher(st, repoPath)
	enricher.SetConcurrency(*concurrency)

	// Load knowing-lsp.json if present in the repo root.
	if lspCfg, err := enrichment.LoadLSPConfig(filepath.Join(repoPath, "knowing-lsp.json")); err == nil {
		enricher.SetLSPConfig(lspCfg)
	}

	if err := enricher.Run(ctx, repoHash); err != nil {
		return fmt.Errorf("enrichment failed: %w", err)
	}
	enricher.Close(ctx)

	fmt.Fprintf(os.Stderr, "LSP enrichment complete\n")
	return nil
}

// cmdEnrichResolver runs in-process type resolvers against an existing DB.
// Adds resolver_resolved edges without re-extracting or running external LSPs.
// Usage: knowing enrich resolver [-db path] <repo-path>
func cmdEnrichResolver(args []string) error {
	fs := flag.NewFlagSet("enrich resolver", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing enrich resolver [flags] <repo-path>")
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

	fmt.Fprintf(os.Stderr, "In-process resolver enrichment: %s\n", repoPath)
	if err := runInProcessResolver(ctx, st, repoPath, repoHash); err != nil {
		return fmt.Errorf("resolver enrichment failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Resolver enrichment complete\n")
	return nil
}
