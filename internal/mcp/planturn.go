package mcp

// planturn.go implements a lightweight task-to-tool recommender. Given a task
// description, it extracts keywords and returns ranked suggestions of which
// knowing MCP tools to call with pre-filled arguments.

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/mark3labs/mcp-go/mcp"
)

// toolRule defines a mapping from keywords to a tool suggestion.
type toolRule struct {
	Keywords []string
	Tool     string
	Reason   string
	Args     map[string]string // pre-filled or placeholder args
}

// toolRules is the static rule table for keyword-to-tool matching.
var toolRules = []toolRule{
	{
		Keywords: []string{"test", "affected", "scope"},
		Tool:     "test_scope",
		Reason:   "task relates to testing or affected test scope",
		Args:     map[string]string{"files": "<fill: comma-separated changed file paths>", "output": "run"},
	},
	{
		Keywords: []string{"changed", "files", "diff", "modified"},
		Tool:     "context_for_files",
		Reason:   "task involves changed or modified files",
		Args:     map[string]string{"files": "<fill: comma-separated changed file paths>"},
	},
	{
		Keywords: []string{"blast", "radius", "impact", "callers"},
		Tool:     "blast_radius",
		Reason:   "task involves understanding impact or callers of a symbol",
		Args:     map[string]string{"symbol": "<fill: qualified symbol name>"},
	},
	{
		Keywords: []string{"flow", "path", "between", "reach"},
		Tool:     "flow_between",
		Reason:   "task involves finding paths or flow between symbols",
		Args:     map[string]string{"source_symbol": "<fill: source symbol>", "target_symbol": "<fill: target symbol>"},
	},
	{
		Keywords: []string{"community", "cluster", "module", "group"},
		Tool:     "communities",
		Reason:   "task involves community detection or module grouping",
		Args:     map[string]string{},
	},
	{
		Keywords: []string{"feedback", "useful", "report"},
		Tool:     "feedback",
		Reason:   "task involves recording or querying feedback",
		Args:     map[string]string{"action": "query"},
	},
	{
		Keywords: []string{"index", "reindex"},
		Tool:     "index_repo",
		Reason:   "task involves indexing or reindexing a repository",
		Args:     map[string]string{"repo_url": "<fill: repository URL>"},
	},
	{
		Keywords: []string{"stale", "dead", "unused"},
		Tool:     "stale_edges",
		Reason:   "task involves finding stale, dead, or unused code",
		Args:     map[string]string{"repo_url": "<fill: repository URL>"},
	},
	{
		Keywords: []string{"diff", "snapshot", "compare"},
		Tool:     "semantic_diff",
		Reason:   "task involves comparing snapshots or diffs",
		Args:     map[string]string{"before_snapshot": "<fill: before snapshot ID>", "after_snapshot": "<fill: after snapshot ID>"},
	},
	{
		Keywords: []string{"context", "task", "relevant"},
		Tool:     "context_for_task",
		Reason:   "task involves gathering relevant context",
		Args:     map[string]string{"task_description": "<fill: task description>"},
	},
	{
		Keywords: []string{"dataflow", "callees", "downstream"},
		Tool:     "trace_dataflow",
		Reason:   "task involves tracing dataflow or downstream callees",
		Args:     map[string]string{"symbol": "<fill: qualified symbol name>"},
	},
	{
		Keywords: []string{"pr", "pull request", "review"},
		Tool:     "context_for_pr",
		Reason:   "task involves pull request review or context",
		Args:     map[string]string{"files": "<fill: comma-separated changed file paths>"},
	},
}

// stopWords are common words to filter out during keyword extraction.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true, "need": true,
	"to": true, "of": true, "in": true, "for": true, "on": true,
	"with": true, "at": true, "by": true, "from": true, "as": true,
	"into": true, "through": true, "during": true, "before": true, "after": true,
	"above": true, "below": true, "and": true, "but": true, "or": true,
	"not": true, "no": true, "so": true, "if": true, "then": true,
	"that": true, "this": true, "it": true, "its": true, "i": true,
	"me": true, "my": true, "we": true, "our": true, "you": true,
	"your": true, "what": true, "which": true, "who": true, "how": true,
	"all": true, "each": true, "every": true, "any": true, "some": true,
}

// extractKeywords splits a task description into lowercase keywords,
// removing stop words and punctuation.
func extractKeywords(task string) []string {
	task = strings.ToLower(task)
	// Split on non-letter, non-digit characters
	words := strings.FieldsFunc(task, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var keywords []string
	for _, w := range words {
		if !stopWords[w] && len(w) > 1 {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// planTurnTool defines the "plan_turn" MCP tool.
func planTurnTool() mcp.Tool {
	return mcp.NewTool("plan_turn",
		mcp.WithDescription("Given a task description, suggests which knowing MCP tools to call with pre-filled arguments. Returns up to 4 ranked suggestions."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("task", mcp.Required(), mcp.Description("Description of the task you want to accomplish")),
	)
}

// suggestion represents a single tool suggestion in the response.
type suggestion struct {
	Tool   string            `json:"tool"`
	Reason string            `json:"reason"`
	Args   map[string]string `json:"args"`
}

// handlePlanTurn handles the plan_turn MCP tool.
func (s *Server) handlePlanTurn(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	task, errResult := requireStringArg(req, "task")
	if errResult != nil {
		return errResult, nil
	}

	keywords := extractKeywords(task)

	type scored struct {
		rule  toolRule
		score int
	}

	var matches []scored
	for _, rule := range toolRules {
		score := 0
		for _, kw := range rule.Keywords {
			// Handle multi-word keywords (e.g. "pull request")
			if strings.Contains(kw, " ") {
				if strings.Contains(strings.ToLower(task), kw) {
					score++
				}
			} else {
				for _, tk := range keywords {
					if tk == kw {
						score++
						break
					}
				}
			}
		}
		if score > 0 {
			matches = append(matches, scored{rule: rule, score: score})
		}
	}

	// Sort by score descending (simple insertion sort for small slice)
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].score > matches[j-1].score; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	// Take top 4
	maxSuggestions := 4
	if len(matches) < maxSuggestions {
		maxSuggestions = len(matches)
	}

	suggestions := make([]suggestion, 0, maxSuggestions)

	// Deduplicate tools (same tool might match multiple rules)
	seen := make(map[string]bool)
	for _, m := range matches {
		if seen[m.rule.Tool] {
			continue
		}
		seen[m.rule.Tool] = true
		suggestions = append(suggestions, suggestion{
			Tool:   m.rule.Tool,
			Reason: m.rule.Reason,
			Args:   m.rule.Args,
		})
		if len(suggestions) >= 4 {
			break
		}
	}

	// Fallback: if no matches, suggest context_for_task
	if len(suggestions) == 0 {
		suggestions = append(suggestions, suggestion{
			Tool:   "context_for_task",
			Reason: "no specific tool matched; use context_for_task as a general starting point",
			Args:   map[string]string{"task_description": task},
		})
	}

	resp := struct {
		Suggestions []suggestion `json:"suggestions"`
	}{Suggestions: suggestions}

	data, err := json.Marshal(resp)
	if err != nil {
		return mcp.NewToolResultError("failed to marshal response"), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
