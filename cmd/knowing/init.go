package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blackwell-systems/knowing/internal/store"
)

const generatedMarkerStart = "<!-- knowing:generated:start -->"
const generatedMarkerEnd = "<!-- knowing:generated:end -->"

// cmdInit generates a CLAUDE.md section with graph-derived project context.
// It is nondestructive and idempotent:
//   - If no CLAUDE.md exists, creates one with the generated section
//   - If CLAUDE.md exists without markers, appends the generated section
//   - If CLAUDE.md exists with markers, replaces only the section between markers
//   - Never touches content outside the markers
func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	output := fs.String("output", "CLAUDE.md", "Output file path (default: CLAUDE.md in current directory)")
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

	// Generate the context section.
	section, err := generateContextSection(st)
	if err != nil {
		return fmt.Errorf("generating context: %w", err)
	}

	// Write or update the file.
	return writeNondestructive(*output, section)
}

// generateContextSection produces the knowing-generated content.
// Follows progressive disclosure: minimal orientation in CLAUDE.md,
// breadcrumbs pointing to tools for on-demand detail.
func generateContextSection(st *store.SQLiteStore) (string, error) {
	ctx := context.Background()

	// Count stats for the one-line summary.
	allNodes, err := st.NodesByName(ctx, "")
	if err != nil {
		return "", err
	}

	packages := make(map[string]bool)
	for _, n := range allNodes {
		pkg := extractPackage(n.QualifiedName)
		if pkg != "" {
			packages[pkg] = true
		}
	}

	// Build a minimal section: orientation + breadcrumbs, not data dumps.
	var sb strings.Builder
	sb.WriteString(generatedMarkerStart + "\n")
	sb.WriteString(fmt.Sprintf("## Graph: %d symbols, %d packages (knowing)\n\n", len(allNodes), len(packages)))
	sb.WriteString("This project has a content-addressed knowledge graph. Before complex edits, call:\n\n")
	sb.WriteString("- `context_for_task` with a task description (graph-ranked context)\n")
	sb.WriteString("- `context_for_pr` with changed files (blast radius for review)\n")
	sb.WriteString("- `blast_radius` with a symbol hash (all callers)\n")
	sb.WriteString("\nUse `format: \"gcf\"` on all context calls for compact responses.\n")
	sb.WriteString(generatedMarkerEnd + "\n")

	return sb.String(), nil
}

// writeNondestructive writes the generated section without disrupting existing content.
func writeNondestructive(path, section string) error {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// No file exists: create with the section.
		return os.WriteFile(path, []byte(section), 0644)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(existing)

	// Check if markers already exist.
	startIdx := strings.Index(content, generatedMarkerStart)
	endIdx := strings.Index(content, generatedMarkerEnd)

	if startIdx >= 0 && endIdx >= 0 {
		// Replace between markers (inclusive).
		endIdx += len(generatedMarkerEnd)
		// Find the newline after the end marker.
		if endIdx < len(content) && content[endIdx] == '\n' {
			endIdx++
		}
		newContent := content[:startIdx] + section + content[endIdx:]
		return os.WriteFile(path, []byte(newContent), 0644)
	}

	// No markers: append the section with a separator.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + section
	return os.WriteFile(path, []byte(content), 0644)
}

// extractPackage gets the package path from a qualified name.
func extractPackage(qname string) string {
	// Format: repoURL://path.Symbol
	idx := strings.Index(qname, "://")
	if idx < 0 {
		return ""
	}
	rest := qname[idx+3:]
	dotIdx := strings.LastIndex(rest, ".")
	if dotIdx < 0 {
		return rest
	}
	return rest[:dotIdx]
}

