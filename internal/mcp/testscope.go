package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// testScopeTool defines the "test_scope" MCP tool, which finds tests affected
// by changes to the given file paths using backward BFS through call edges.
func testScopeTool() mcp.Tool {
	return mcp.NewTool("test_scope",
		mcp.WithDescription("Find tests affected by changes to the given files. Performs backward BFS through call edges to discover test functions that transitively depend on symbols in the specified files."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("files", mcp.Required(), mcp.Description("Comma-separated file paths relative to repo root (e.g. pkg/store/sqlite.go,pkg/types/types.go)"), Examples("pkg/store/sqlite.go,internal/mcp/server.go")),
		mcp.WithString("output", mcp.Description("Output format: 'packages' (unique package paths), 'functions' (qualified test function names), or 'run' (go test -run regex). Default: packages"), mcp.Enum("packages", "functions", "run")),
		mcp.WithNumber("depth", mcp.Description("Maximum BFS traversal depth (default 3)")),
	)
}

// testScopeResult is the JSON response for the test_scope tool.
type testScopeResult struct {
	Mode    string   `json:"mode"`
	Results []string `json:"results"`
	Count   int      `json:"count"`
}

// handleTestScope implements the test_scope tool handler.
func (s *Server) handleTestScope(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil {
		return mcp.NewToolResultError("test_scope requires a SQLiteStore backend"), nil
	}

	// Parse files argument.
	filesArg, errResult := requireStringArg(req, "files")
	if errResult != nil {
		return errResult, nil
	}

	rawPaths := strings.Split(filesArg, ",")
	var paths []string
	for _, p := range rawPaths {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			paths = append(paths, trimmed)
		}
	}

	// Parse output mode.
	outputMode := getStringArg(req, "output")
	if outputMode == "" {
		outputMode = "packages"
	}

	// Parse depth.
	maxDepth := getIntArg(req, "depth", 3)

	// Step 1: Find all symbols in the specified files.
	repos, err := s.sqlStore.AllRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("AllRepos: %w", err)
	}

	seen := make(map[types.Hash]bool)
	var changedNodes []types.Hash

	for _, repo := range repos {
		for _, path := range paths {
			nodes, err := s.sqlStore.NodesByFilePath(ctx, repo.RepoHash, path)
			if err != nil {
				return nil, fmt.Errorf("NodesByFilePath(%s): %w", path, err)
			}
			for _, n := range nodes {
				if !seen[n.NodeHash] {
					seen[n.NodeHash] = true
					changedNodes = append(changedNodes, n.NodeHash)
				}
			}
		}
	}

	// Step 2: BFS backward through call edges to find affected test functions.
	type bfsEntry struct {
		hash  types.Hash
		depth int
	}

	visited := make(map[types.Hash]bool)
	for _, h := range changedNodes {
		visited[h] = true
	}

	queue := make([]bfsEntry, 0, len(changedNodes))
	for _, h := range changedNodes {
		queue = append(queue, bfsEntry{hash: h, depth: 0})
	}

	var testNodes []types.Node

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		// Get all callers of this node.
		edges, err := s.sqlStore.EdgesTo(ctx, entry.hash, "calls")
		if err != nil {
			return nil, fmt.Errorf("EdgesTo: %w", err)
		}

		for _, edge := range edges {
			callerHash := edge.SourceHash
			if visited[callerHash] {
				continue
			}
			visited[callerHash] = true

			// Look up the caller node.
			node, err := s.sqlStore.GetNode(ctx, callerHash)
			if err != nil {
				return nil, fmt.Errorf("GetNode: %w", err)
			}
			if node == nil {
				continue
			}

			// Check if this is a test function.
			if isTestFunction(node) {
				testNodes = append(testNodes, *node)
			}

			// Continue BFS if within depth.
			if entry.depth+1 < maxDepth {
				queue = append(queue, bfsEntry{hash: callerHash, depth: entry.depth + 1})
			}
		}
	}

	// Step 3: Format output.
	var results []string

	switch outputMode {
	case "packages":
		pkgSet := make(map[string]bool)
		for _, n := range testNodes {
			pkg := extractPackage(n.QualifiedName)
			if pkg != "" {
				pkgSet[pkg] = true
			}
		}
		for pkg := range pkgSet {
			results = append(results, pkg)
		}
	case "functions":
		for _, n := range testNodes {
			results = append(results, n.QualifiedName)
		}
	case "run":
		nameSet := make(map[string]bool)
		for _, n := range testNodes {
			name := extractFuncName(n.QualifiedName)
			if name != "" {
				nameSet[name] = true
			}
		}
		var names []string
		for name := range nameSet {
			names = append(names, name)
		}
		if len(names) > 0 {
			results = []string{"^(" + strings.Join(names, "|") + ")$"}
		}
	}

	out := testScopeResult{
		Mode:    outputMode,
		Results: results,
		Count:   len(results),
	}

	data, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("json.Marshal: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
	}, nil
}

// isTestFunction returns true if the node represents a Go test or benchmark function.
func isTestFunction(n *types.Node) bool {
	if n.Kind != "function" {
		return false
	}
	name := extractFuncName(n.QualifiedName)
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark")
}

// extractPackage extracts the package path from a qualified name.
// Format: "repoURL://pkg/path.FuncName" -> "pkg/path"
func extractPackage(qualifiedName string) string {
	// Split on "://" to separate repo URL from package path.
	parts := strings.SplitN(qualifiedName, "://", 2)
	if len(parts) < 2 {
		return ""
	}
	pkgAndSymbol := parts[1]
	// The last dot separates the package from the symbol name.
	// Handle method receivers: "pkg/path.Type.Method" -> "pkg/path"
	lastDot := strings.LastIndex(pkgAndSymbol, ".")
	if lastDot < 0 {
		return pkgAndSymbol
	}
	pkg := pkgAndSymbol[:lastDot]
	// If pkg still contains a dot (e.g. "pkg/path.Type"), strip again.
	if idx := strings.LastIndex(pkg, "."); idx >= 0 {
		// Check if the part after the dot starts with uppercase (method receiver).
		after := pkg[idx+1:]
		if len(after) > 0 && after[0] >= 'A' && after[0] <= 'Z' {
			pkg = pkg[:idx]
		}
	}
	return pkg
}

// extractFuncName extracts the bare function name from a qualified name.
// Format: "repoURL://pkg/path.FuncName" -> "FuncName"
func extractFuncName(qualifiedName string) string {
	parts := strings.SplitN(qualifiedName, "://", 2)
	if len(parts) < 2 {
		return ""
	}
	pkgAndSymbol := parts[1]
	lastDot := strings.LastIndex(pkgAndSymbol, ".")
	if lastDot < 0 {
		return pkgAndSymbol
	}
	return pkgAndSymbol[lastDot+1:]
}
