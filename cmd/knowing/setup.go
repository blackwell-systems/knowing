package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/roster"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdSetup is the full "get started in one command" experience.
// It indexes the repo, enriches with LSP, generates CLAUDE.md,
// and configures the MCP server for Claude Code.
//
// Usage: knowing init [flags] [repo-path]
//
// Steps:
//   1. Detect git root and repo URL
//   2. Index the repository
//   3. Detect and run LSP enrichment
//   4. Generate CLAUDE.md
//   5. Configure Claude Code MCP server (if detected)
//   6. Print summary
func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dbPath := fs.String("db", "", "Database path (default: knowing.db in repo root)")
	repoURL := fs.String("url", "", "Repository URL (default: auto-detect from git remote)")
	skipMCP := fs.Bool("skip-mcp", false, "Skip Claude Code MCP configuration")
	skipEnrich := fs.Bool("skip-enrich", false, "Skip LSP enrichment")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Determine repo path.
	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\n  knowing init\n\n")

	// Step 1: Detect git root and repo URL.
	gitRoot := detectGitRoot(absPath)
	if gitRoot == "" {
		gitRoot = absPath
		fmt.Fprintf(os.Stderr, "  [1/6] Not a git repo, using %s\n", absPath)
	} else {
		fmt.Fprintf(os.Stderr, "  [1/6] Git root: %s\n", gitRoot)
	}

	if *repoURL == "" {
		*repoURL = detectRepoURL(gitRoot)
	}
	if *repoURL == "" {
		*repoURL = gitRoot // fallback to local path
	}
	fmt.Fprintf(os.Stderr, "        Repo URL: %s\n", *repoURL)

	if *dbPath == "" {
		*dbPath = defaultDB() // global DB for cross-repo edges
	}

	// Register in roster for cross-repo tracking.
	if _, err := roster.Add(gitRoot, *repoURL); err != nil {
		fmt.Fprintf(os.Stderr, "        Roster warning: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "        Registered in roster: %s\n", roster.Path())
	}

	// Step 2: Index the repository.
	fmt.Fprintf(os.Stderr, "  [2/6] Indexing repository...\n")
	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	registerAllExtractors(idx, false)

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, *repoURL, gitRoot, "HEAD")
	if err != nil {
		return fmt.Errorf("index: %w", err)
	}
	if snap != nil {
		fmt.Fprintf(os.Stderr, "        %d nodes, %d edges indexed\n", snap.NodeCount, snap.EdgeCount)
	}

	// Step 3: LSP enrichment.
	if !*skipEnrich {
		fmt.Fprintf(os.Stderr, "  [3/6] Detecting language servers...\n")
		lspCfg := enrichment.DetectLSPServers(gitRoot)
		if len(lspCfg.Servers) > 0 {
			for _, s := range lspCfg.Servers {
				fmt.Fprintf(os.Stderr, "        Found: %s (%s)\n", s.Command[0], s.LanguageID)
			}
			fmt.Fprintf(os.Stderr, "        Running enrichment...\n")
			enricher := enrichment.NewEnricher(st, gitRoot)
			enricher.SetLSPConfig(lspCfg)
			repoHash := types.NewHash([]byte(*repoURL))
			if err := enricher.Run(ctx, repoHash); err != nil {
				fmt.Fprintf(os.Stderr, "        Enrichment error (non-fatal): %v\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "        No language servers found on PATH (install gopls, typescript-language-server, etc.)\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  [3/6] LSP enrichment skipped (--skip-enrich)\n")
	}

	// Step 4: Generate CLAUDE.md.
	fmt.Fprintf(os.Stderr, "  [4/6] Generating CLAUDE.md...\n")
	section, err := generateContextSection(st)
	if err != nil {
		return fmt.Errorf("generate context: %w", err)
	}
	claudeMDPath := filepath.Join(gitRoot, "CLAUDE.md")
	if err := writeNondestructive(claudeMDPath, section); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}
	fmt.Fprintf(os.Stderr, "        Wrote %s\n", claudeMDPath)

	// Step 5: Configure Claude Code MCP server.
	if !*skipMCP {
		fmt.Fprintf(os.Stderr, "  [5/6] Configuring Claude Code MCP server...\n")
		if err := configureMCPServer(*dbPath); err != nil {
			fmt.Fprintf(os.Stderr, "        MCP config skipped: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "        MCP server configured in ~/.claude.json\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  [5/6] MCP configuration skipped (--skip-mcp)\n")
	}

	// Step 6: Summary.
	allNodes, _ := st.NodesByName(ctx, "")
	fmt.Fprintf(os.Stderr, "\n  [6/6] Setup complete!\n\n")
	fmt.Fprintf(os.Stderr, "  knowing is ready for this repository:\n")
	fmt.Fprintf(os.Stderr, "    Database: %s\n", *dbPath)
	fmt.Fprintf(os.Stderr, "    Symbols:  %d\n", len(allNodes))
	fmt.Fprintf(os.Stderr, "    CLAUDE.md: %s\n", claudeMDPath)
	fmt.Fprintf(os.Stderr, "\n  Next steps:\n")
	fmt.Fprintf(os.Stderr, "    1. Start using Claude Code in this repo\n")
	fmt.Fprintf(os.Stderr, "    2. Call context_for_task to get graph-ranked context\n")
	fmt.Fprintf(os.Stderr, "    3. Run 'knowing init' again anytime to update\n\n")

	return nil
}

// detectGitRoot finds the git root directory from a starting path.
func detectGitRoot(startPath string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = startPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// detectRepoURL gets the remote URL from git.
func detectRepoURL(gitRoot string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	// Normalize: strip .git suffix and convert SSH to HTTPS-style.
	url = strings.TrimSuffix(url, ".git")
	if strings.HasPrefix(url, "git@") {
		// git@github.com:org/repo -> github.com/org/repo
		url = strings.TrimPrefix(url, "git@")
		url = strings.Replace(url, ":", "/", 1)
	}
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	return url
}

// configureMCPServer adds knowing to ~/.claude.json MCP server config.
func configureMCPServer(dbPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home directory: %w", err)
	}

	claudeJSON := filepath.Join(home, ".claude.json")
	knowingBin, err := exec.LookPath("knowing")
	if err != nil {
		return fmt.Errorf("knowing not on PATH")
	}

	absDB, _ := filepath.Abs(dbPath)

	// Read existing config.
	var config map[string]any
	data, err := os.ReadFile(claudeJSON)
	if err != nil {
		if os.IsNotExist(err) {
			config = make(map[string]any)
		} else {
			return fmt.Errorf("read ~/.claude.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("parse ~/.claude.json: %w", err)
		}
	}

	// Navigate to mcpServers.
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
		config["mcpServers"] = mcpServers
	}

	// Check if knowing is already configured.
	if _, exists := mcpServers["knowing"]; exists {
		log.Printf("[info] knowing MCP server already configured in ~/.claude.json")
		return nil
	}

	// Add knowing server config.
	mcpServers["knowing"] = map[string]any{
		"type":    "stdio",
		"command": knowingBin,
		"args":    []string{"mcp"},
		"env": map[string]string{
			"KNOWING_DB": absDB,
		},
	}

	// Write back.
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(claudeJSON, out, 0644)
}
