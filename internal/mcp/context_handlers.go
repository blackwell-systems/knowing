package mcp

// context_handlers.go implements the MCP tool handlers for graph-aware context
// packing: context_for_task and context_for_files. These handlers delegate to
// the internal/context package for ranking and formatting.

import (
	"context"
	"fmt"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/wire"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleContextForTask handles the context_for_task MCP tool.
func (s *Server) handleContextForTask(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskDesc, errResult := requireStringArg(req, "task_description")
	if errResult != nil {
		return errResult, nil
	}
	tokenBudget := getIntArg(req, "token_budget", 50000)
	format := getStringArg(req, "format")
	priorPackRoot := getStringArg(req, "pack_root")

	// Store task keywords for vocab association recording in ObserveToolUse.
	s.lastTaskKeywords = knowingctx.ExtractKeywordSetExported(taskDesc).All()

	engine := knowingctx.NewContextEngine(s.store)
	engine.SetSession(s.ctxSession)
	if s.vecSearch != nil {
		engine.SetVector(s.vecSearch)
	}
	// Task memory disabled (session 24, confirmed neutral). Implicit feedback only.
	// if s.taskMemory != nil {
	// 	engine.SetTaskMemory(s.taskMemory)
	// }
	if s.resultCache != nil {
		engine.SetCache(s.resultCache)
	}
	if s.implicit != nil {
		engine.SetImplicitFeedback(s.implicit)
	}
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: taskDesc,
		TokenBudget:     tokenBudget,
		Format:          "xml",
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("context_for_task failed: %v", err)), nil
	}

	// Context pack deduplication (P5): if the agent sent a prior PackRoot
	// and it matches the current result, return an "unchanged" signal.
	// The agent already has this context; no need to resend.
	if priorPackRoot != "" && block.PackRoot.String() == priorPackRoot {
		return mcp.NewToolResultText(fmt.Sprintf(
			"unchanged pack_root=%s symbols=%d\nContext is identical to your prior request. No retransmission needed.",
			block.PackRoot, len(block.Symbols),
		)), nil
	}

	// Track session metrics for the knowing://session resource.
	s.contextCalls.Add(1)
	s.symbolsServed.Add(int64(len(block.Symbols)))

	// Implicit feedback (flush/register) is now handled by the context engine
	// in ForTask via recordImplicitFeedback. The engine calls FlushUnused on
	// previous symbols and RegisterReturned on new ones. DetectUsed is still
	// called from the MCP server's tool call handler (server.go:detectImplicitUsage)
	// since it needs tool call content that only the MCP layer has.

	// Task memory DISABLED (session 24). Keyword->symbol recording confirmed
	// neutral on honest measurement (5 rounds, P@10 flat). The implicit feedback
	// system (noise demotion via FlushUnused/DetectUsed) is the active learning
	// mechanism. Task memory infrastructure preserved for future redesign (7a-d).

	output, err := formatBlock(ctx, block, format, "context_for_task", s)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("format failed: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleContextForFiles handles the context_for_files MCP tool.
func (s *Server) handleContextForFiles(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filesStr, errResult := requireStringArg(req, "files")
	if errResult != nil {
		return errResult, nil
	}
	repoURL := getStringArg(req, "repo_url")
	tokenBudget := getIntArg(req, "token_budget", 50000)
	format := getStringArg(req, "format")

	// Split comma-separated file paths and trim whitespace.
	parts := strings.Split(filesStr, ",")
	fileList := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			fileList = append(fileList, trimmed)
		}
	}

	engine := knowingctx.NewContextEngine(s.store)
	block, err := engine.ForFiles(ctx, knowingctx.FileOptions{
		Files:       fileList,
		RepoURL:     repoURL,
		TokenBudget: tokenBudget,
		Format:      "xml",
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("context_for_files failed: %v", err)), nil
	}

	// Track session metrics for the knowing://session resource.
	s.contextCalls.Add(1)
	s.symbolsServed.Add(int64(len(block.Symbols)))

	output, err := formatBlock(ctx, block, format, "context_for_files", s)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("format failed: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// handleContextForPR handles the context_for_pr MCP tool.
func (s *Server) handleContextForPR(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filesStr, errResult := requireStringArg(req, "files")
	if errResult != nil {
		return errResult, nil
	}
	repoURL := getStringArg(req, "repo_url")
	tokenBudget := getIntArg(req, "token_budget", 8000)
	format := getStringArg(req, "format")

	// Split comma-separated file paths.
	parts := strings.Split(filesStr, ",")
	fileList := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			fileList = append(fileList, trimmed)
		}
	}

	engine := knowingctx.NewContextEngine(s.store)
	block, err := engine.ForPR(ctx, knowingctx.PROptions{
		Files:       fileList,
		RepoURL:     repoURL,
		TokenBudget: tokenBudget,
		Format:      "xml",
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("context_for_pr failed: %v", err)), nil
	}

	output, err := formatBlock(ctx, block, format, "context_for_pr", s)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("format failed: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// formatBlock routes output through the wire codec registry for gcf/gcb/json,
// or falls back to the legacy context formatter for xml/markdown.
// For GCF, uses the server's session state for cross-call deduplication.
func formatBlock(ctx context.Context, block *knowingctx.ContextBlock, format, tool string, s *Server) (string, error) {
	switch format {
	case "gcf":
		payload, err := wire.FromContextBlock(ctx, block, tool, s.store)
		if err != nil {
			return "", fmt.Errorf("building wire payload: %w", err)
		}
		return wire.EncodeWithSession(payload, s.session), nil
	case "gcb", "json":
		payload, err := wire.FromContextBlock(ctx, block, tool, s.store)
		if err != nil {
			return "", fmt.Errorf("building wire payload: %w", err)
		}
		return wire.EncodeWith(format, payload)
	case "xml", "markdown", "":
		if format == "" {
			format = "xml"
		}
		return knowingctx.FormatContextBlock(block, format)
	default:
		return "", fmt.Errorf("unknown format %q (available: gcf, gcb, json, xml, markdown)", format)
	}
}

// handleExplainSymbol handles the explain_symbol MCP tool.
func (s *Server) handleExplainSymbol(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskDesc, errResult := requireStringArg(req, "task_description")
	if errResult != nil {
		return errResult, nil
	}
	symbol, errResult := requireStringArg(req, "symbol")
	if errResult != nil {
		return errResult, nil
	}

	// Implicit feedback: asking "why did this symbol rank here?" strongly implies
	// the agent is actively using it.
	s.ObserveToolUse(ctx, symbol)

	engine := knowingctx.NewContextEngine(s.store)
	engine.SetSession(s.ctxSession)
	if s.vecSearch != nil {
		engine.SetVector(s.vecSearch)
	}
	if s.taskMemory != nil {
		engine.SetTaskMemory(s.taskMemory)
	}

	result, err := engine.ExplainSymbol(ctx, taskDesc, symbol)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("explain_symbol failed: %v", err)), nil
	}

	output := formatExplain(result)
	return mcp.NewToolResultText(output), nil
}

// formatExplain renders an ExplainResult as a readable text block.
func formatExplain(r *knowingctx.ExplainResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# knowing why: %s\n", r.Symbol.QualifiedName)
	fmt.Fprintf(&b, "Kind: %s", r.Symbol.Kind)
	if r.Symbol.Line > 0 {
		fmt.Fprintf(&b, " (line %d)", r.Symbol.Line)
	}
	b.WriteString("\n\n")

	if r.Rank < 0 {
		b.WriteString("**NOT RANKED** (not in top results for this task)\n")
		if r.RWRScore > 0 {
			fmt.Fprintf(&b, "RWR score: %.4f (below 0.02 threshold)\n", r.RWRScore)
		} else {
			b.WriteString("Not reached by seed matching or RWR walk\n")
		}
		fmt.Fprintf(&b, "Keywords: %s\n", strings.Join(r.Keywords, ", "))
		return b.String()
	}

	fmt.Fprintf(&b, "**Rank #%d** of %d symbols (score: %.4f)\n\n", r.Rank, r.TotalSymbols, r.TotalScore)

	// Discovery
	b.WriteString("## Discovery\n")
	if r.IsSeed {
		b.WriteString("- Seed: yes (direct keyword match)\n")
	} else {
		b.WriteString("- Seed: no (reached via graph walk)\n")
	}
	fmt.Fprintf(&b, "- Channel: %s\n", r.SeedChannel)
	if r.SeedTier != "" {
		fmt.Fprintf(&b, "- Tier: %s\n", r.SeedTier)
	}
	if len(r.EquivMatches) > 0 {
		fmt.Fprintf(&b, "- Equivalence classes: %s\n", strings.Join(r.EquivMatches, ", "))
	}
	fmt.Fprintf(&b, "- Keywords: %s\n\n", strings.Join(r.Keywords, ", "))

	// Score components
	b.WriteString("## Score Breakdown\n")
	b.WriteString("| Component | Score | Detail |\n")
	b.WriteString("|-----------|-------|--------|\n")
	fmt.Fprintf(&b, "| Blast radius | %.4f | caller proxy %d, max %d |\n",
		r.Components.BlastRadius, r.CallerProxy, r.MaxCallers)
	fmt.Fprintf(&b, "| Confidence | %.4f | |\n", r.Components.Confidence)
	fmt.Fprintf(&b, "| Recency | %.4f | |\n", r.Components.Recency)
	dist := 0
	if !r.IsSeed {
		dist = 1
	}
	fmt.Fprintf(&b, "| Distance | %.4f | distance=%d |\n", r.Components.Distance, dist)
	if r.Components.Feedback != 0 {
		fmt.Fprintf(&b, "| Feedback | %+.4f | |\n", r.Components.Feedback)
	}
	if r.Components.Session != 0 {
		fmt.Fprintf(&b, "| Session | %.4f | |\n", r.Components.Session)
	}
	b.WriteString("\n")

	// Graph signals
	b.WriteString("## Graph Signals\n")
	fmt.Fprintf(&b, "- RWR score: %.4f\n", r.RWRScore)
	if r.HITSAuthority > 0 || r.HITSHub > 0 {
		fmt.Fprintf(&b, "- HITS authority: %.4f\n", r.HITSAuthority)
		fmt.Fprintf(&b, "- HITS hub: %.4f\n", r.HITSHub)
		if r.HITSAdjust != 0 {
			fmt.Fprintf(&b, "- HITS adjustment: %+.4f\n", r.HITSAdjust)
		}
	}

	return b.String()
}

// --- Tool definitions ---

func contextForTaskTool() mcp.Tool {
	return mcp.NewTool("context_for_task",
		mcp.WithDescription("Generate graph-ranked, token-budgeted context for a task description. Returns symbols ranked by blast radius, confidence, recency, and graph distance. Supports multiple output formats. Pass pack_root from a prior call to check if context changed; returns 'unchanged' signal if identical, saving retransmission."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("task_description", mcp.Required(), mcp.Description("Description of the task to generate context for (e.g. add caching to the user lookup endpoint)"), Examples("add caching to the user lookup endpoint")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 50000)")),
		mcp.WithString("format", mcp.Description("Output format: gcf (compact graph-native, 75%+ token savings), gcb (binary), json, xml (default), markdown"), Examples("gcf")),
		mcp.WithString("pack_root", mcp.Description("PackRoot from a prior context_for_task call (64-char hex). If the current result has the same PackRoot, returns 'unchanged' instead of resending context."), Examples("a1b2c3d4e5f6...")),
	)
}

func contextForPRTool() mcp.Tool {
	return mcp.NewTool("context_for_pr",
		mcp.WithDescription("Generate relationship-aware context for a pull request. Identifies all symbols in changed files, runs graph-based relevance scoring (RWR) from them, and surfaces the full structural impact neighborhood including callers, callees, and related types. One call at PR-open time replaces multiple manual context queries."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("files", mcp.Required(), mcp.Description("Comma-separated list of changed file paths relative to repo root (from the PR diff)"), Examples("internal/mcp/server.go,internal/context/context.go")),
		mcp.WithString("repo_url", mcp.Description("Repository URL for resolving file hashes"), Examples("https://github.com/org/repo")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 8000, larger than per-edit calls)")),
		mcp.WithString("format", mcp.Description("Output format: gcf (compact, 75%+ token savings), gcb (binary), json, xml (default), markdown"), Examples("gcf")),
	)
}

func explainSymbolTool() mcp.Tool {
	return mcp.NewTool("explain_symbol",
		mcp.WithDescription("Explain why a symbol ranked where it did for a given task. Shows the full scoring breakdown: seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback weight, session boost, and equivalence class matches. Use this to debug unexpected rankings or understand why a symbol was (or wasn't) included in context results."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("task_description", mcp.Required(), mcp.Description("Task description to evaluate the symbol against"), Examples("refactor auth middleware")),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Symbol name or qualified name to explain"), Examples("RankSymbols", "context.ForTask")),
	)
}

func contextForFilesTool() mcp.Tool {
	return mcp.NewTool("context_for_files",
		mcp.WithDescription("Generate blast-radius context weighted by runtime observations for a set of changed files. Returns related symbols ranked by graph proximity and runtime traffic. Supports multiple output formats."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("files", mcp.Required(), mcp.Description("Comma-separated list of changed file paths relative to repo root (e.g. internal/mcp/handlers.go,internal/store/sqlite.go)"), Examples("internal/mcp/handlers.go,internal/store/sqlite.go")),
		mcp.WithString("repo_url", mcp.Description("Repository URL for resolving file hashes (e.g. https://github.com/org/repo)"), Examples("https://github.com/org/repo")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 50000)")),
		mcp.WithString("format", mcp.Description("Output format: gcf (compact graph-native, 75%+ token savings), gcb (binary), json, xml (default), markdown"), Examples("gcf")),
	)
}
