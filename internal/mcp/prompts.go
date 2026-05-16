package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// registerPrompts registers all MCP prompts on the server.
// Prompts are pre-built reasoning templates that structure multi-step workflows.
// Any MCP client discovers them automatically during initialization.
func (s *Server) registerPrompts() {
	s.mcpServer.AddPrompt(refactorSafelyPrompt(), s.handleRefactorSafely)
	s.mcpServer.AddPrompt(reviewPRPrompt(), s.handleReviewPR)
	s.mcpServer.AddPrompt(investigateDeadCodePrompt(), s.handleInvestigateDeadCode)
}

// --- Prompt Definitions ---

func refactorSafelyPrompt() mcp.Prompt {
	return mcp.Prompt{
		Name:        "refactor_safely",
		Description: "Structured workflow for safe refactoring: check blast radius, get context, plan changes, verify no broken callers. Use before modifying any exported symbol.",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "symbol",
				Description: "The symbol or file you intend to refactor (e.g., 'HandleRequest', 'internal/mcp/handlers.go')",
				Required:    true,
			},
			{
				Name:        "intent",
				Description: "What you plan to change (e.g., 'rename to HandleHTTPRequest', 'change return type', 'extract helper')",
				Required:    false,
			},
		},
	}
}

func reviewPRPrompt() mcp.Prompt {
	return mcp.Prompt{
		Name:        "review_pr",
		Description: "Structured PR review workflow: get semantic diff, identify risk level, check runtime traffic on changed routes, summarize relationship-level impact.",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "files",
				Description: "Comma-separated list of changed files in the PR",
				Required:    true,
			},
			{
				Name:        "base_snapshot",
				Description: "Base snapshot hash (from main branch). If omitted, uses latest snapshot.",
				Required:    false,
			},
			{
				Name:        "head_snapshot",
				Description: "Head snapshot hash (from PR branch). If omitted, uses latest snapshot.",
				Required:    false,
			},
		},
	}
}

func investigateDeadCodePrompt() mcp.Prompt {
	return mcp.Prompt{
		Name:        "investigate_dead_code",
		Description: "Find potentially dead symbols: zero static callers AND no runtime observations. Distinguishes 'unused' from 'uninstrumented' by checking service coverage.",
		Arguments: []mcp.PromptArgument{
			{
				Name:        "scope",
				Description: "Package or file path to scope the investigation (e.g., 'internal/mcp', 'cmd/knowing/main.go'). Omit for full repo.",
				Required:    false,
			},
			{
				Name:        "stale_days",
				Description: "Number of days without runtime observation to consider a route dead (default: 30)",
				Required:    false,
			},
		},
	}
}

// --- Prompt Handlers ---

func (s *Server) handleRefactorSafely(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	symbol := ""
	intent := ""
	if req.Params.Arguments != nil {
		symbol = req.Params.Arguments["symbol"]
		intent = req.Params.Arguments["intent"]
	}

	instructions := "You are about to refactor code in a knowing-indexed codebase. Follow these steps in order:\n\n"
	instructions += "## Step 1: Check Blast Radius\n\n"
	instructions += "Call the `context_for_files` tool with the file(s) containing `" + symbol + "` to understand what depends on it.\n"
	instructions += "Look at the scores: symbols scoring > 0.7 are highly connected and changes will propagate widely.\n\n"
	instructions += "## Step 2: Identify All Callers\n\n"
	instructions += "Call `blast_radius` with the target symbol's node hash. This returns every transitive caller.\n"
	instructions += "Count them. If > 20 callers, this is a high-risk refactor. Consider a deprecation path instead of a breaking change.\n\n"
	instructions += "## Step 3: Check Runtime Traffic\n\n"
	instructions += "Call `runtime_traffic` with the service name. If this symbol handles production traffic (observation_count > 0), "
	instructions += "coordinate the change with a deployment plan. Dead routes (no observations) are safer to modify.\n\n"
	instructions += "## Step 4: Plan the Change\n\n"
	if intent != "" {
		instructions += "Stated intent: " + intent + "\n\n"
	}
	instructions += "Based on the blast radius and runtime data:\n"
	instructions += "- If callers are in the same package: proceed with direct modification\n"
	instructions += "- If callers span packages: consider adding the new version alongside the old, migrating callers, then removing the old\n"
	instructions += "- If callers span repos: coordinate via semantic PR diff to show cross-repo impact\n\n"
	instructions += "## Step 5: Verify After Change\n\n"
	instructions += "After making the change, call `context_for_files` again on the modified files.\n"
	instructions += "Compare the graph structure before and after. New edges should appear where expected.\n"
	instructions += "No unexpected edge removals should be present (these indicate broken callers).\n"

	return mcp.NewGetPromptResult(
		"Safe refactoring workflow for: "+symbol,
		[]mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: instructions,
				},
			},
		},
	), nil
}

func (s *Server) handleReviewPR(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	files := ""
	if req.Params.Arguments != nil {
		files = req.Params.Arguments["files"]
	}

	instructions := "You are reviewing a PR that changes the following files: " + files + "\n\n"
	instructions += "Follow this structured review workflow:\n\n"
	instructions += "## Step 1: Get Semantic Diff\n\n"
	instructions += "Call `semantic_diff` with the base and head snapshot hashes.\n"
	instructions += "This shows relationship-level changes (not just file diffs): which symbols were added, removed, or had their edges modified.\n\n"
	instructions += "## Step 2: Assess Impact\n\n"
	instructions += "Call `pr_impact` with the same snapshot hashes.\n"
	instructions += "Look at:\n"
	instructions += "- `risk_level`: low (0-5 affected callers), medium (6-20), high (>20)\n"
	instructions += "- `changed_symbols`: each symbol that was modified and its blast radius\n"
	instructions += "- `affected_edges`: relationships that were created or broken\n\n"
	instructions += "## Step 3: Check Runtime Traffic\n\n"
	instructions += "For each changed file that contains HTTP route handlers:\n"
	instructions += "Call `runtime_traffic` to see if these routes handle production traffic.\n"
	instructions += "Call `dead_routes` to identify any routes that haven't been called recently.\n"
	instructions += "High-traffic routes require more scrutiny. Dead routes are safer to modify.\n\n"
	instructions += "## Step 4: Summarize\n\n"
	instructions += "Produce a review summary with:\n"
	instructions += "- Risk level (from pr_impact)\n"
	instructions += "- Number of symbols changed and their blast radius\n"
	instructions += "- Whether any production routes are affected\n"
	instructions += "- Recommendation: approve, request changes, or needs discussion\n"

	return mcp.NewGetPromptResult(
		"PR review workflow for changed files",
		[]mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: instructions,
				},
			},
		},
	), nil
}

func (s *Server) handleInvestigateDeadCode(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	scope := "the entire repository"
	staleDays := "30"
	if req.Params.Arguments != nil {
		if s := req.Params.Arguments["scope"]; s != "" {
			scope = s
		}
		if d := req.Params.Arguments["stale_days"]; d != "" {
			staleDays = d
		}
	}

	instructions := "Investigate potentially dead code in: " + scope + "\n\n"
	instructions += "## Step 1: Find Symbols with Zero Callers\n\n"
	instructions += "Call `graph_query` with a prefix matching the scope to enumerate symbols.\n"
	instructions += "For each exported symbol, call `blast_radius` to check if it has any callers.\n"
	instructions += "Symbols with 0 callers are dead code candidates.\n\n"
	instructions += "## Step 2: Check Runtime Observations\n\n"
	instructions += "Call `dead_routes` with stale_days=" + staleDays + ".\n"
	instructions += "Routes with no observations in " + staleDays + " days are dead route candidates.\n"
	instructions += "Call `trace_stats` to see overall runtime edge health.\n\n"
	instructions += "## Step 3: Distinguish Dead from Uninstrumented\n\n"
	instructions += "A symbol with 0 callers and 0 runtime observations could be:\n"
	instructions += "- **Actually dead**: no code calls it, no traffic reaches it. Safe to remove.\n"
	instructions += "- **Uninstrumented**: called by external systems not tracked by knowing (e.g., cron jobs, CLI tools, third-party integrations).\n"
	instructions += "- **Test-only**: called only from test files (check if callers are in *_test.go).\n"
	instructions += "- **Interface implementation**: required by an interface but never called directly.\n\n"
	instructions += "Check the symbol's kind and package before recommending removal.\n"
	instructions += "Public API endpoints, interface implementations, and main() entry points are often \"dead\" by graph analysis but alive by design.\n\n"
	instructions += "## Step 4: Report\n\n"
	instructions += "Produce a categorized list:\n"
	instructions += "- Confirmed dead (safe to remove)\n"
	instructions += "- Likely dead (needs human verification)\n"
	instructions += "- Uninstrumented (needs trace coverage, not removal)\n"

	return mcp.NewGetPromptResult(
		"Dead code investigation for: "+scope,
		[]mcp.PromptMessage{
			{
				Role: mcp.RoleUser,
				Content: mcp.TextContent{
					Type: "text",
					Text: instructions,
				},
			},
		},
	), nil
}
