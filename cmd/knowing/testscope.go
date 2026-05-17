package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdTestScope computes which tests are affected by a set of changed files.
// It walks the call graph backward from changed symbols to find test functions
// that transitively call them.
//
// Usage:
//
//	knowing test-scope -db knowing.db                    # uses git diff HEAD
//	knowing test-scope -db knowing.db -files "a.go,b.go"  # explicit file list
//	knowing test-scope -db knowing.db -output packages     # output Go package paths
//	knowing test-scope -db knowing.db -output functions    # output test function names
//	knowing test-scope -db knowing.db -output run          # output -run regex for go test
func cmdTestScope(args []string) error {
	fs := flag.NewFlagSet("test-scope", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	files := fs.String("files", "", "Comma-separated changed files (default: git diff HEAD)")
	outputMode := fs.String("output", "packages", "Output mode: packages, functions, run")
	depth := fs.Int("depth", 3, "Maximum call-graph traversal depth")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found: %s (run 'knowing index' first)", *dbPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	// Get changed files.
	var changedFiles []string
	if *files != "" {
		for _, f := range strings.Split(*files, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				changedFiles = append(changedFiles, f)
			}
		}
	} else {
		changedFiles, err = gitChangedFiles()
		if err != nil {
			return fmt.Errorf("detecting changed files: %w", err)
		}
	}

	if len(changedFiles) == 0 {
		return nil // nothing changed, nothing to test
	}

	ctx := context.Background()

	// Find all symbols in changed files.
	changedSymbols, err := symbolsInFiles(ctx, st, changedFiles)
	if err != nil {
		return err
	}

	if len(changedSymbols) == 0 {
		return nil
	}

	// Walk backward from changed symbols to find test functions.
	tests, err := findAffectedTests(ctx, st, changedSymbols, *depth)
	if err != nil {
		return err
	}

	if len(tests) == 0 {
		return nil
	}

	// Output.
	switch *outputMode {
	case "packages":
		pkgs := uniqueTestPackages(tests)
		for _, p := range pkgs {
			fmt.Println(p)
		}
	case "functions":
		for _, t := range tests {
			fmt.Println(t.QualifiedName)
		}
	case "run":
		// Output a -run regex that matches all affected test functions.
		names := make([]string, 0, len(tests))
		for _, t := range tests {
			name := testFuncName(t.QualifiedName)
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			fmt.Printf("^(%s)$\n", strings.Join(names, "|"))
		}
	default:
		return fmt.Errorf("unknown output mode: %q (use: packages, functions, run)", *outputMode)
	}

	return nil
}

// gitChangedFiles returns files changed in the working tree (staged + unstaged vs HEAD).
func gitChangedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		// Try without HEAD (no commits yet).
		cmd = exec.Command("git", "diff", "--name-only")
		out, err = cmd.Output()
		if err != nil {
			return nil, err
		}
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasSuffix(line, ".go") || strings.HasSuffix(line, ".ts") ||
			strings.HasSuffix(line, ".py") || strings.HasSuffix(line, ".rs") ||
			strings.HasSuffix(line, ".java") || strings.HasSuffix(line, ".cs")) {
			files = append(files, line)
		}
	}
	return files, nil
}

// symbolsInFiles finds all graph nodes that belong to the given files.
func symbolsInFiles(ctx context.Context, st *store.SQLiteStore, files []string) ([]types.Node, error) {
	// Get repo hash (assume first repo).
	repos, err := st.AllRepos(ctx)
	if err != nil || len(repos) == 0 {
		return nil, err
	}
	repoHash := repos[0].RepoHash

	var symbols []types.Node
	seen := make(map[types.Hash]bool)

	for _, path := range files {
		nodes, err := st.NodesByFilePath(ctx, repoHash, path)
		if err != nil {
			return nil, err
		}
		for _, n := range nodes {
			if !seen[n.NodeHash] {
				seen[n.NodeHash] = true
				symbols = append(symbols, n)
			}
		}
	}

	return symbols, nil
}

// findAffectedTests walks the call graph backward from changed symbols
// to find test functions (nodes whose name starts with "Test").
func findAffectedTests(ctx context.Context, st *store.SQLiteStore, changedSymbols []types.Node, maxDepth int) ([]types.Node, error) {
	// BFS backward from changed symbols.
	visited := make(map[types.Hash]bool)
	var tests []types.Node

	// Seed with changed symbol hashes.
	frontier := make([]types.Hash, 0, len(changedSymbols))
	for _, s := range changedSymbols {
		frontier = append(frontier, s.NodeHash)
		visited[s.NodeHash] = true

		// Check if the changed symbol itself is a test.
		if isTestFunction(s) {
			tests = append(tests, s)
		}
	}

	// BFS: walk backward along "calls" edges (find callers).
	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []types.Hash
		for _, nodeHash := range frontier {
			// Find all callers of this node.
			callers, err := st.EdgesTo(ctx, nodeHash, "calls")
			if err != nil {
				continue
			}
			for _, edge := range callers {
				if visited[edge.SourceHash] {
					continue
				}
				visited[edge.SourceHash] = true

				caller, err := st.GetNode(ctx, edge.SourceHash)
				if err != nil || caller == nil {
					continue
				}

				if isTestFunction(*caller) {
					tests = append(tests, *caller)
				} else {
					// Continue traversing backward through non-test callers.
					nextFrontier = append(nextFrontier, edge.SourceHash)
				}
			}
		}
		frontier = nextFrontier
	}

	return tests, nil
}

// isTestFunction returns true if the node looks like a Go test function.
func isTestFunction(n types.Node) bool {
	if n.Kind != "function" {
		return false
	}
	name := testFuncName(n.QualifiedName)
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark")
}

// testFuncName extracts the function name from a qualified name.
func testFuncName(qname string) string {
	lastDot := strings.LastIndex(qname, ".")
	if lastDot < 0 {
		return qname
	}
	return qname[lastDot+1:]
}

// uniqueTestPackages extracts unique Go package paths from test nodes.
func uniqueTestPackages(tests []types.Node) []string {
	seen := make(map[string]bool)
	var pkgs []string

	for _, t := range tests {
		pkg := extractGoPackagePath(t.QualifiedName)
		if pkg != "" && !seen[pkg] {
			seen[pkg] = true
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}

// extractGoPackagePath extracts a "go test"-compatible package path from a qualified name.
// Format: repoURL://modulePath/pkg.FuncName -> ./pkg
func extractGoPackagePath(qname string) string {
	idx := strings.Index(qname, "://")
	if idx < 0 {
		return ""
	}
	repoURL := qname[:idx]
	rest := qname[idx+3:]
	// Remove the symbol name (last .Component).
	lastDot := strings.LastIndex(rest, ".")
	if lastDot < 0 {
		return rest
	}
	pkgPath := rest[:lastDot]

	// Strip the module path prefix (same as repoURL) to get a relative path.
	if strings.HasPrefix(pkgPath, repoURL+"/") {
		return "./" + pkgPath[len(repoURL)+1:]
	}
	if pkgPath == repoURL {
		return "./"
	}

	return "./" + pkgPath
}
