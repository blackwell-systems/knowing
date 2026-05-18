// Package ownership parses CODEOWNERS files and emits owned_by edges from
// file nodes to synthetic team/user nodes. The CODEOWNERS format (GitHub
// standard) maps glob patterns to one or more owners (teams or users).
//
// Supported locations for CODEOWNERS:
//   - CODEOWNERS (repo root)
//   - .github/CODEOWNERS
//   - docs/CODEOWNERS
//
// Each parsed rule produces owned_by edges with confidence 1.0 and
// provenance "codeowners" because ownership is deterministic.
package ownership

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// Rule represents a single CODEOWNERS rule: a glob pattern mapped to one
// or more owners.
type Rule struct {
	Pattern string
	Owners  []string
}

// ParseCodeowners reads a CODEOWNERS file and returns the parsed rules.
// Rules are returned in file order; later rules take precedence (matching
// GitHub's semantics). Blank lines and comment lines (starting with #) are
// skipped.
func ParseCodeowners(path string) ([]Rule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []Rule
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		rules = append(rules, Rule{
			Pattern: fields[0],
			Owners:  fields[1:],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// FindCodeowners searches for a CODEOWNERS file in the standard locations
// within the given repo root. Returns the path to the first found file,
// or empty string if none exists.
func FindCodeowners(repoRoot string) string {
	candidates := []string{
		filepath.Join(repoRoot, "CODEOWNERS"),
		filepath.Join(repoRoot, ".github", "CODEOWNERS"),
		filepath.Join(repoRoot, "docs", "CODEOWNERS"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// matchPattern checks if a file path matches a CODEOWNERS glob pattern.
// CODEOWNERS patterns follow these conventions:
//   - Patterns without a "/" match against the file name only.
//   - Patterns starting with "/" are anchored to the repo root.
//   - Patterns ending with "/" match directories (and all files under them).
//   - Standard glob characters (*, ?) are supported.
//   - "**" matches across directory boundaries.
func matchPattern(pattern, filePath string) bool {
	// Normalize path separators.
	filePath = filepath.ToSlash(filePath)

	// Patterns ending with "/" match the directory and everything under it.
	if strings.HasSuffix(pattern, "/") {
		dir := strings.TrimSuffix(pattern, "/")
		dir = strings.TrimPrefix(dir, "/")
		if strings.HasPrefix(filePath, dir+"/") || filePath == dir {
			return true
		}
		return false
	}

	// Replace "**" with a match-all placeholder for filepath.Match compatibility.
	if strings.Contains(pattern, "**") {
		// Convert "**/" to match any number of directories.
		p := strings.TrimPrefix(pattern, "/")
		// Simple approach: check if the suffix after the last "**/" matches.
		parts := strings.SplitN(p, "**/", 2)
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			if prefix != "" && !strings.HasPrefix(filePath, prefix) {
				return false
			}
			// Try matching the suffix against every possible sub-path.
			segments := strings.Split(filePath, "/")
			for i := range segments {
				subPath := strings.Join(segments[i:], "/")
				if matched, _ := filepath.Match(suffix, subPath); matched {
					return true
				}
			}
			return false
		}
	}

	// Patterns starting with "/" are anchored to root.
	if strings.HasPrefix(pattern, "/") {
		matched, _ := filepath.Match(strings.TrimPrefix(pattern, "/"), filePath)
		return matched
	}

	// Patterns without "/" match against the filename only (unless they contain /).
	if !strings.Contains(pattern, "/") {
		matched, _ := filepath.Match(pattern, filepath.Base(filePath))
		return matched
	}

	// Otherwise, try matching against the full path.
	matched, _ := filepath.Match(pattern, filePath)
	if matched {
		return true
	}

	// Also try matching without leading directory context.
	segments := strings.Split(filePath, "/")
	for i := range segments {
		subPath := strings.Join(segments[i:], "/")
		if matched, _ := filepath.Match(pattern, subPath); matched {
			return true
		}
	}
	return false
}

// ExtractOwnership generates owned_by edges for all file nodes in the graph
// based on CODEOWNERS rules. For each file, it finds the last matching rule
// (GitHub semantics: last match wins) and emits edges from the file node to
// synthetic owner nodes.
//
// Parameters:
//   - repoURL: the repository URL used for node hash computation
//   - repoRoot: absolute path to the repository root on disk
//   - files: the list of File records from the current index run
//   - rules: parsed CODEOWNERS rules
//
// Returns nodes for owners (kind "team" or "user") and owned_by edges.
func ExtractOwnership(repoURL string, files []types.File, rules []Rule) ([]types.Node, []types.Edge) {
	if len(rules) == 0 || len(files) == 0 {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge

	// Track which owner nodes we have already created to avoid duplicates.
	seenOwners := make(map[string]types.Hash)

	for _, file := range files {
		// Find the last matching rule (GitHub semantics).
		var matchedRule *Rule
		for i := range rules {
			if matchPattern(rules[i].Pattern, file.Path) {
				matchedRule = &rules[i]
			}
		}
		if matchedRule == nil {
			continue
		}

		for _, owner := range matchedRule.Owners {
			// Determine if this is a team or user.
			// Teams use the format @org/team-name, users are @username.
			kind := "user"
			if strings.Contains(owner, "/") {
				kind = "team"
			}
			ownerName := strings.TrimPrefix(owner, "@")

			// Get or create the owner node hash.
			ownerHash, exists := seenOwners[ownerName]
			if !exists {
				ownerHash = types.ComputeNodeHash(repoURL, "owners", types.EmptyHash, ownerName, kind)
				seenOwners[ownerName] = ownerHash
				nodes = append(nodes, types.Node{
					NodeHash:      ownerHash,
					QualifiedName: repoURL + "://owners/" + ownerName,
					Kind:          kind,
				})
			}

			provenance := "codeowners"
			edgeHash := types.ComputeEdgeHash(file.FileHash, ownerHash, "owned_by", provenance)
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: file.FileHash,
				TargetHash: ownerHash,
				EdgeType:   "owned_by",
				Confidence: 1.0,
				Provenance: provenance,
			})
		}
	}

	return nodes, edges
}
