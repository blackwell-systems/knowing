package mcp

// context_handlers.go implements the MCP tool handlers for graph-aware context
// packing: context_for_task and context_for_files. These handlers delegate to
// the internal/context package for ranking and formatting.

import (
	"context"
	"fmt"
	"strings"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleContextForTask handles the context_for_task MCP tool.
func (s *Server) handleContextForTask(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskDesc, errResult := requireStringArg(req, "task_description")
	if errResult != nil {
		return errResult, nil
	}
	tokenBudget := getIntArg(req, "token_budget", 50000)

	engine := knowingctx.NewContextEngine(s.store)
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: taskDesc,
		TokenBudget:     tokenBudget,
		Format:          "xml",
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("context_for_task failed: %v", err)), nil
	}

	output, err := knowingctx.FormatContextBlock(block, "xml")
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

	output, err := knowingctx.FormatContextBlock(block, "xml")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("format failed: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

// --- Tool definitions ---

func contextForTaskTool() mcp.Tool {
	return mcp.NewTool("context_for_task",
		mcp.WithDescription("Generate graph-ranked, token-budgeted context for a task description. Returns symbols ranked by blast radius, confidence, recency, and graph distance in XML format."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("task_description", mcp.Required(), mcp.Description("Description of the task to generate context for (e.g. add caching to the user lookup endpoint)")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 50000)")),
	)
}

func contextForFilesTool() mcp.Tool {
	return mcp.NewTool("context_for_files",
		mcp.WithDescription("Generate blast-radius context weighted by runtime observations for a set of changed files. Returns related symbols ranked by graph proximity and runtime traffic in XML format."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("files", mcp.Required(), mcp.Description("Comma-separated list of changed file paths relative to repo root (e.g. internal/mcp/handlers.go,internal/store/sqlite.go)")),
		mcp.WithString("repo_url", mcp.Description("Repository URL for resolving file hashes (e.g. https://github.com/org/repo)")),
		mcp.WithInteger("token_budget", mcp.Description("Maximum token budget for the context block (default 50000)")),
	)
}
