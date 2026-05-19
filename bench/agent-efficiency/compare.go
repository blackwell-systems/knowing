package agent_efficiency

import (
	"fmt"
	"strings"
)

// ComparisonResult holds the delta between a control (without knowing) and a
// treatment (with knowing) session for the same task.
type ComparisonResult struct {
	TaskID    string
	Control   SessionMetrics
	Treatment SessionMetrics

	// TokenSavings is the fraction of tokens saved:
	//   (control.TotalTokens - treatment.TotalTokens) / control.TotalTokens
	// Positive means treatment used fewer tokens.
	TokenSavings float64

	// ToolCallSavings is the fraction of tool calls saved:
	//   (control.ToolCalls - treatment.ToolCalls) / control.ToolCalls
	ToolCallSavings float64

	// TimeSavings is the fraction of wall-clock time saved.
	TimeSavings float64

	// CorrectnessDelta is treatment.AnswerCorrectness - control.AnswerCorrectness.
	// Positive means treatment gave a more correct answer.
	CorrectnessDelta float64
}

// Compare computes all deltas between control and treatment metrics.
func Compare(control, treatment SessionMetrics) ComparisonResult {
	r := ComparisonResult{
		TaskID:    control.TaskID,
		Control:   control,
		Treatment: treatment,
	}

	if control.TotalTokens > 0 {
		r.TokenSavings = float64(control.TotalTokens-treatment.TotalTokens) / float64(control.TotalTokens)
	}
	if control.ToolCalls > 0 {
		r.ToolCallSavings = float64(control.ToolCalls-treatment.ToolCalls) / float64(control.ToolCalls)
	}
	if control.WallClockMs > 0 {
		r.TimeSavings = float64(control.WallClockMs-treatment.WallClockMs) / float64(control.WallClockMs)
	}
	r.CorrectnessDelta = treatment.AnswerCorrectness - control.AnswerCorrectness

	return r
}

// FormatReport renders a slice of ComparisonResults as a Markdown report with
// a summary table and a per-task detail section.
func FormatReport(results []ComparisonResult) string {
	var sb strings.Builder

	sb.WriteString("# Agent Efficiency Benchmark Report\n\n")
	sb.WriteString("Positive savings values mean the treatment (with knowing MCP) used fewer resources.\n")
	sb.WriteString("Positive correctness delta means the treatment gave more correct answers.\n\n")

	// Summary table.
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Task | Token Savings | Tool Call Savings | Time Savings | Correctness Delta |\n")
	sb.WriteString("|------|--------------|------------------|--------------|-------------------|\n")

	var totalToken, totalTool, totalTime, totalCorrect float64
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %+.3f |\n",
			r.TaskID,
			formatPct(r.TokenSavings),
			formatPct(r.ToolCallSavings),
			formatPct(r.TimeSavings),
			r.CorrectnessDelta,
		))
		totalToken += r.TokenSavings
		totalTool += r.ToolCallSavings
		totalTime += r.TimeSavings
		totalCorrect += r.CorrectnessDelta
	}

	if len(results) > 0 {
		n := float64(len(results))
		sb.WriteString(fmt.Sprintf("| **Average** | **%s** | **%s** | **%s** | **%+.3f** |\n",
			formatPct(totalToken/n),
			formatPct(totalTool/n),
			formatPct(totalTime/n),
			totalCorrect/n,
		))
	}

	sb.WriteString("\n")

	// Per-task detail.
	sb.WriteString("## Per-Task Detail\n\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("### %s\n\n", r.TaskID))
		sb.WriteString("| Metric | Control | Treatment | Delta |\n")
		sb.WriteString("|--------|---------|-----------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Total tokens | %d | %d | %+d |\n",
			r.Control.TotalTokens, r.Treatment.TotalTokens,
			r.Treatment.TotalTokens-r.Control.TotalTokens))
		sb.WriteString(fmt.Sprintf("| Tool calls | %d | %d | %+d |\n",
			r.Control.ToolCalls, r.Treatment.ToolCalls,
			r.Treatment.ToolCalls-r.Control.ToolCalls))
		sb.WriteString(fmt.Sprintf("| Turns | %d | %d | %+d |\n",
			r.Control.Turns, r.Treatment.Turns,
			r.Treatment.Turns-r.Control.Turns))
		sb.WriteString(fmt.Sprintf("| Wall clock (ms) | %d | %d | %+d |\n",
			r.Control.WallClockMs, r.Treatment.WallClockMs,
			r.Treatment.WallClockMs-r.Control.WallClockMs))
		sb.WriteString(fmt.Sprintf("| Answer correctness | %.3f | %.3f | %+.3f |\n",
			r.Control.AnswerCorrectness, r.Treatment.AnswerCorrectness,
			r.CorrectnessDelta))
		sb.WriteString(fmt.Sprintf("| Relevant files found | %d | %d | %+d |\n",
			r.Control.FoundRelevantFiles, r.Treatment.FoundRelevantFiles,
			r.Treatment.FoundRelevantFiles-r.Control.FoundRelevantFiles))
		sb.WriteString(fmt.Sprintf("| Key symbols found | %d | %d | %+d |\n",
			r.Control.FoundKeySymbols, r.Treatment.FoundKeySymbols,
			r.Treatment.FoundKeySymbols-r.Control.FoundKeySymbols))

		// Tool call breakdown for treatment.
		if len(r.Treatment.ToolCallsByType) > 0 {
			sb.WriteString("\n**Treatment tool call breakdown:**\n\n")
			sb.WriteString("| Tool | Count |\n|------|-------|\n")
			for tool, count := range r.Treatment.ToolCallsByType {
				sb.WriteString(fmt.Sprintf("| %s | %d |\n", tool, count))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatPct(f float64) string {
	return fmt.Sprintf("%+.1f%%", f*100)
}
