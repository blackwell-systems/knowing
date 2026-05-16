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

	engine := knowingctx.NewContextEngine(s.store)
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: taskDesc,
		TokenBudget:     tokenBudget,
		Format:          "xml",
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("context_for_task failed: %v", err)), nil
	}

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

	output, err := formatBlock(ctx, block, format, "context_for_files", s)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("format failed: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// formatBlock routes output through the wire codec registry for kwf/kwb/json,
// or falls back to the legacy context formatter for xml/markdown.
// For KWF, uses the server's session state for cross-call deduplication.
func formatBlock(ctx context.Context, block *knowingctx.ContextBlock, format, tool string, s *Server) (string, error) {
	switch format {
	case "kwf":
		payload, err := wire.FromContextBlock(ctx, block, tool, s.store)
		if err != nil {
			return "", fmt.Errorf("building wire payload: %w", err)
		}
		return wire.EncodeWithSession(payload, s.session), nil
	case "kwb", "json":
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
		return "", fmt.Errorf("unknown format %q (available: kwf, kwb, json, xml, markdown)", format)
	}
}

// --- Tool definitions ---

func contextForTaskTool() mcp.Tool {
	return mcp.NewTool("context_for_task",
		mcp.WithDescription("Generate graph-ranked, token-budgeted context for a task description. Returns symbols ranked by blast radius, confidence, recency, and graph distance. Supports multiple output formats."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("task_description", mcp.Required(), mcp.Description("Description of the task to generate context for (e.g. add caching to the user lookup endpoint)"), Examples("add caching to the user lookup endpoint")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 50000)")),
		mcp.WithString("format", mcp.Description("Output format: kwf (compact graph-native, 75%+ token savings), kwb (binary), json, xml (default), markdown"), Examples("kwf")),
	)
}

func contextForFilesTool() mcp.Tool {
	return mcp.NewTool("context_for_files",
		mcp.WithDescription("Generate blast-radius context weighted by runtime observations for a set of changed files. Returns related symbols ranked by graph proximity and runtime traffic. Supports multiple output formats."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("files", mcp.Required(), mcp.Description("Comma-separated list of changed file paths relative to repo root (e.g. internal/mcp/handlers.go,internal/store/sqlite.go)"), Examples("internal/mcp/handlers.go,internal/store/sqlite.go")),
		mcp.WithString("repo_url", mcp.Description("Repository URL for resolving file hashes (e.g. https://github.com/org/repo)"), Examples("https://github.com/org/repo")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 50000)")),
		mcp.WithString("format", mcp.Description("Output format: kwf (compact graph-native, 75%+ token savings), kwb (binary), json, xml (default), markdown"), Examples("kwf")),
	)
}
