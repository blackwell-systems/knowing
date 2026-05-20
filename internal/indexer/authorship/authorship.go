// Package authorship extracts authored_by edges from git blame data.
// For each symbol (function, type, method) in a file, it determines the
// primary author (the author with the most lines in the symbol's range)
// and creates an authored_by edge from the symbol node to a synthetic
// author node.
//
// The git blame parsing logic is self-contained in this package (not
// imported from cmd/knowing) to avoid circular dependencies and keep
// the extraction logic modular.
package authorship

import (
	"bufio"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SymbolAuthorship holds the primary author for a symbol.
type SymbolAuthorship struct {
	NodeHash types.Hash
	Author   string
}

// blameLine holds parsed git blame output for a single line.
type blameLine struct {
	author string
}

// ExtractAuthorship runs git blame on a file and creates authored_by edges
// from each symbol to its primary author (most lines attributed).
//
// Parameters:
//   - repoURL: repository URL for node hash computation
//   - repoPath: absolute path to repo root (for running git blame)
//   - file: the File record
//   - nodes: all nodes in this file (need Line field for range computation)
//
// Returns: (authorNodes []types.Node, edges []types.Edge, error)
func ExtractAuthorship(repoURL, repoPath string, file types.File, nodes []types.Node) ([]types.Node, []types.Edge, error) {
	if len(nodes) == 0 {
		return nil, nil, nil
	}

	// Run git blame on the file.
	blameData, err := runGitBlame(repoPath, file.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("authorship: blame %s: %w", file.Path, err)
	}

	// Sort nodes by line number for range computation.
	sorted := make([]types.Node, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Line < sorted[j].Line
	})

	var authorNodes []types.Node
	var edges []types.Edge

	// Track seen author nodes by name to avoid duplicates.
	seenAuthors := make(map[string]types.Hash)

	for i, node := range sorted {
		if node.Line <= 0 || node.Line > len(blameData) {
			continue
		}

		// Determine line range: from this node's line to the next node's line (exclusive), or EOF.
		startLine := node.Line // 1-indexed
		var endLine int        // 1-indexed, inclusive
		if i+1 < len(sorted) && sorted[i+1].Line > startLine {
			endLine = sorted[i+1].Line - 1
		} else {
			endLine = len(blameData)
		}

		// Count lines per author in the range.
		authorCounts := make(map[string]int)
		for lineIdx := startLine - 1; lineIdx < endLine && lineIdx < len(blameData); lineIdx++ {
			author := blameData[lineIdx].author
			if author != "" {
				authorCounts[author]++
			}
		}

		if len(authorCounts) == 0 {
			continue
		}

		// Find the primary author (most lines).
		primaryAuthor := findPrimaryAuthor(authorCounts)

		// Get or create the author node.
		authorHash, exists := seenAuthors[primaryAuthor]
		if !exists {
			authorHash = types.ComputeNodeHash(repoURL, "authors", types.EmptyHash, primaryAuthor, "author")
			seenAuthors[primaryAuthor] = authorHash
			authorNodes = append(authorNodes, types.Node{
				NodeHash:      authorHash,
				QualifiedName: repoURL + "://authors/" + primaryAuthor,
				Kind:          "author",
			})
		}

		// Create the authored_by edge.
		provenance := "git_blame"
		edgeHash := types.ComputeEdgeHash(node.NodeHash, authorHash, "authored_by", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: node.NodeHash,
			TargetHash: authorHash,
			EdgeType:   "authored_by",
			Confidence: 1.0,
			Provenance: provenance,
		})
	}

	return authorNodes, edges, nil
}

// findPrimaryAuthor returns the author with the highest line count.
// In case of a tie, returns the lexicographically first author for
// deterministic results.
func findPrimaryAuthor(counts map[string]int) string {
	var best string
	var bestCount int
	for author, count := range counts {
		if count > bestCount || (count == bestCount && author < best) {
			best = author
			bestCount = count
		}
	}
	return best
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

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "author ") {
			currentAuthor = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "\t") {
			// Content line marks end of this blame block.
			lines = append(lines, blameLine{
				author: currentAuthor,
			})
		}
	}

	return lines, scanner.Err()
}
