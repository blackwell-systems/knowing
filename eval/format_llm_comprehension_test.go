// Package eval: LLM format comprehension benchmark.
//
// Sends the same context payload in GCF, JSON, XML, and TOON formats to an
// actual LLM and measures whether it can answer structured questions correctly.
// Each question has an objectively verifiable answer derived from the payload.
//
// This is the definitive test of whether GCF's 84% token savings come at the
// cost of comprehension accuracy. If GCF accuracy matches JSON, it can be the
// default format for all agent workflows.
//
// Two backends:
//
//	EVAL_BACKEND=cli  (default) - shells out to `claude -p "..."`.
//	                              Uses whatever model Claude Code is configured for.
//	                              No API key needed; requires claude on PATH.
//	EVAL_BACKEND=api            - calls Anthropic Messages API directly.
//	                              Requires ANTHROPIC_API_KEY env var.
//	                              EVAL_MODEL overrides the model (default: claude-haiku-4-5-20251001).
//
// Run (CLI backend, default):
//
//	GOWORK=off go test ./eval/ -run TestLLMFormatComprehension -v -timeout 30m
//
// Run (API backend):
//
//	EVAL_BACKEND=api ANTHROPIC_API_KEY=sk-... GOWORK=off go test ./eval/ -run TestLLMFormatComprehension -v -timeout 30m
package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/wire"
)

// llmQuestion defines a question with a deterministic expected answer.
type llmQuestion struct {
	Name     string
	Task     string // task description to generate context
	Question string // question sent to the LLM after the context
	// Extract computes the expected answer from the structured block.
	Extract func(block *knowingctx.ContextBlock) string
	// Verify checks whether the LLM's response contains the expected answer.
	// Returns (correct bool, detail string).
	Verify func(expected, llmResponse string) (bool, string)
}

var llmQuestions = []llmQuestion{
	{
		Name:     "top_symbol",
		Task:     "improve context retrieval ranking",
		Question: "What is the qualified name of the highest-scored symbol in the context? Reply with ONLY the qualified name, nothing else.",
		Extract: func(b *knowingctx.ContextBlock) string {
			if len(b.Symbols) == 0 {
				return ""
			}
			return b.Symbols[0].Node.QualifiedName
		},
		Verify: func(expected, resp string) (bool, string) {
			resp = strings.TrimSpace(resp)
			// Accept if the response contains the expected QN (LLM may add backticks or context).
			if strings.Contains(resp, expected) {
				return true, "exact match"
			}
			// Accept short name match.
			short := shortName(expected)
			if strings.Contains(resp, short) {
				return true, "short name match"
			}
			return false, fmt.Sprintf("expected %q, got %q", expected, resp)
		},
	},
	{
		Name:     "symbol_count",
		Task:     "add a new MCP tool",
		Question: "How many symbols are in the context? Reply with ONLY a number, nothing else.",
		Extract: func(b *knowingctx.ContextBlock) string {
			return fmt.Sprintf("%d", len(b.Symbols))
		},
		Verify: func(expected, resp string) (bool, string) {
			resp = strings.TrimSpace(resp)
			if resp == expected {
				return true, "exact"
			}
			// Accept if the number appears in the response.
			if strings.Contains(resp, expected) {
				return true, "contains"
			}
			return false, fmt.Sprintf("expected %s, got %q", expected, resp)
		},
	},
	{
		Name:     "edge_count",
		Task:     "refactor the snapshot manager",
		Question: "How many edges (relationships between symbols) are in the context? Reply with ONLY a number, nothing else.",
		Extract: func(b *knowingctx.ContextBlock) string {
			return fmt.Sprintf("%d", len(b.Edges))
		},
		Verify: func(expected, resp string) (bool, string) {
			resp = strings.TrimSpace(resp)
			if resp == expected {
				return true, "exact"
			}
			if strings.Contains(resp, expected) {
				return true, "contains"
			}
			return false, fmt.Sprintf("expected %s, got %q", expected, resp)
		},
	},
	{
		Name:     "top_kind",
		Task:     "improve context retrieval ranking",
		Question: "What is the kind (function, method, type, etc.) of the highest-scored symbol? Reply with ONLY the kind, nothing else.",
		Extract: func(b *knowingctx.ContextBlock) string {
			if len(b.Symbols) == 0 {
				return ""
			}
			return b.Symbols[0].Node.Kind
		},
		Verify: func(expected, resp string) (bool, string) {
			resp = strings.TrimSpace(strings.ToLower(resp))
			expected = strings.ToLower(expected)
			if resp == expected {
				return true, "exact"
			}
			// GCF uses abbreviations (fn, iface). Accept both.
			abbrevMap := map[string]string{
				"fn": "function", "function": "fn",
				"iface": "interface", "interface": "iface",
				"method": "method", "type": "type",
			}
			if alt, ok := abbrevMap[expected]; ok && resp == alt {
				return true, "abbreviation match"
			}
			if strings.Contains(resp, expected) {
				return true, "contains"
			}
			return false, fmt.Sprintf("expected %q, got %q", expected, resp)
		},
	},
	{
		Name:     "seed_count",
		Task:     "add a new extractor for Zig",
		Question: "How many symbols are in the 'targets' or distance-0 group (the direct matches)? Reply with ONLY a number, nothing else.",
		Extract: func(b *knowingctx.ContextBlock) string {
			count := 0
			for _, s := range b.Symbols {
				if s.Distance == 0 {
					count++
				}
			}
			return fmt.Sprintf("%d", count)
		},
		Verify: func(expected, resp string) (bool, string) {
			resp = strings.TrimSpace(resp)
			if resp == expected {
				return true, "exact"
			}
			if strings.Contains(resp, expected) {
				return true, "contains"
			}
			return false, fmt.Sprintf("expected %s, got %q", expected, resp)
		},
	},
	{
		Name:     "edge_type_list",
		Task:     "trace data flow through the indexer",
		Question: "List all unique edge types that appear in the context, comma-separated, in alphabetical order. Reply with ONLY the comma-separated list, nothing else.",
		Extract: func(b *knowingctx.ContextBlock) string {
			types := make(map[string]bool)
			for _, e := range b.Edges {
				types[e.EdgeType] = true
			}
			sorted := make([]string, 0, len(types))
			for t := range types {
				sorted = append(sorted, t)
			}
			// Sort manually to avoid import.
			for i := 0; i < len(sorted); i++ {
				for j := i + 1; j < len(sorted); j++ {
					if sorted[j] < sorted[i] {
						sorted[i], sorted[j] = sorted[j], sorted[i]
					}
				}
			}
			return strings.Join(sorted, ", ")
		},
		Verify: func(expected, resp string) (bool, string) {
			resp = strings.TrimSpace(resp)
			// Normalize: lowercase, remove backticks, trim spaces around commas.
			normalize := func(s string) string {
				s = strings.ToLower(s)
				s = strings.ReplaceAll(s, "`", "")
				parts := strings.Split(s, ",")
				for i, p := range parts {
					parts[i] = strings.TrimSpace(p)
				}
				return strings.Join(parts, ", ")
			}
			if normalize(resp) == normalize(expected) {
				return true, "exact"
			}
			// Partial credit: count how many expected types appear.
			expectedTypes := strings.Split(expected, ", ")
			found := 0
			for _, et := range expectedTypes {
				if strings.Contains(strings.ToLower(resp), strings.ToLower(et)) {
					found++
				}
			}
			if found == len(expectedTypes) {
				return true, "all types present"
			}
			if found > 0 {
				return false, fmt.Sprintf("%d/%d types found", found, len(expectedTypes))
			}
			return false, fmt.Sprintf("expected %q, got %q", expected, resp)
		},
	},
}

// llmBackend abstracts the two ways to call an LLM.
type llmBackend interface {
	Name() string
	Call(prompt string) (string, error)
}

// cliBackend shells out to `claude -c "..."`.
type cliBackend struct{}

func (cliBackend) Name() string { return "cli (claude -c)" }

func (cliBackend) Call(prompt string) (string, error) {
	cmd := exec.Command("claude", "-p", prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude -p failed: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// apiBackend calls the Anthropic Messages API directly.
type apiBackend struct {
	apiKey string
	model  string
}

func (a apiBackend) Name() string { return fmt.Sprintf("api (%s)", a.model) }

func (a apiBackend) Call(prompt string) (string, error) {
	return callClaude(a.apiKey, a.model, prompt)
}

func TestLLMFormatComprehension(t *testing.T) {
	// Select backend: cli (default) or api.
	backendName := os.Getenv("EVAL_BACKEND")
	if backendName == "" {
		backendName = "cli"
	}

	var backend llmBackend
	switch backendName {
	case "cli":
		if _, err := exec.LookPath("claude"); err != nil {
			t.Skip("claude not on PATH; install Claude Code or set EVAL_BACKEND=api with ANTHROPIC_API_KEY")
		}
		backend = cliBackend{}
	case "api":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			t.Skip("EVAL_BACKEND=api requires ANTHROPIC_API_KEY")
		}
		model := os.Getenv("EVAL_MODEL")
		if model == "" {
			model = "claude-haiku-4-5-20251001"
		}
		backend = apiBackend{apiKey: apiKey, model: model}
	default:
		t.Fatalf("unknown EVAL_BACKEND %q (use cli or api)", backendName)
	}

	dbPath := os.Getenv("KNOWING_DB")
	if dbPath == "" {
		dbPath = "knowing.db"
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbPath = "../knowing.db"
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Skip("no knowing.db found; run knowing index first")
		}
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	formats := []string{"json", "xml", "gcf"}

	type result struct {
		Question string
		Format   string
		Correct  bool
		Detail   string
		Tokens   int
	}
	var results []result

	t.Log("")
	t.Log("=== LLM Format Comprehension Eval ===")
	t.Logf("Backend: %s", backend.Name())
	t.Log("")

	for _, q := range llmQuestions {
		// Generate context once.
		engine := knowingctx.NewContextEngine(st)
		engine.DisablePersistentCache()
		ctx := context.Background()

		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: q.Task,
			TokenBudget:     5000,
			Format:          "xml",
		})
		if err != nil || len(block.Symbols) == 0 {
			t.Logf("%s: skipped (no symbols)", q.Name)
			continue
		}

		// Enrich block with edges from the store so ground truth matches
		// what the formatted output actually contains. ForTask doesn't
		// populate block.Edges, but renderContext -> FromContextBlock
		// discovers edges between included symbols.
		if len(block.Edges) == 0 {
			payload, err := wire.FromContextBlock(ctx, block, "eval", st)
			if err == nil && len(payload.Edges) > 0 {
				edges := make([]knowingctx.ContextEdge, len(payload.Edges))
				for i, e := range payload.Edges {
					edges[i] = knowingctx.ContextEdge{
						Source:   e.Source,
						Target:   e.Target,
						EdgeType: e.EdgeType,
					}
				}
				block.Edges = edges
			}
		}

		expected := q.Extract(block)
		if expected == "" {
			t.Logf("%s: skipped (empty expected)", q.Name)
			continue
		}

		for _, format := range formats {
			// Render context in this format.
			contextStr, err := renderContext(block, format, st)
			if err != nil {
				t.Logf("%s/%s: render error: %v", q.Name, format, err)
				continue
			}

			tokens := estimateTokens(contextStr)

			// Ask the LLM.
			prompt := fmt.Sprintf("Here is a code context payload in %s format:\n\n%s\n\nQuestion: %s",
				strings.ToUpper(format), contextStr, q.Question)

			resp, err := backend.Call(prompt)
			if err != nil {
				t.Logf("%s/%s: API error: %v", q.Name, format, err)
				results = append(results, result{q.Name, format, false, "api_error", tokens})
				continue
			}

			correct, detail := q.Verify(expected, resp)
			results = append(results, result{q.Name, format, correct, detail, tokens})

			mark := "PASS"
			if !correct {
				mark = "FAIL"
			}
			t.Logf("  %s %-13s %-6s [%s] expected=%q got=%q (%s)",
				mark, q.Name, format, detail, expected, strings.TrimSpace(resp), fmt.Sprintf("%d tok", tokens))
		}
	}

	// Summary table.
	t.Log("")
	t.Log("=== Summary ===")
	t.Log("")

	formatCorrect := make(map[string]int)
	formatTotal := make(map[string]int)
	formatTokens := make(map[string]int)

	for _, r := range results {
		formatTotal[r.Format]++
		if r.Correct {
			formatCorrect[r.Format]++
		}
		formatTokens[r.Format] += r.Tokens
	}

	t.Logf("%-8s %8s %10s %12s", "Format", "Accuracy", "Avg Tokens", "vs JSON")
	t.Logf("%-8s %8s %10s %12s", "------", "--------", "----------", "-------")

	jsonTokens := 0
	if formatTotal["json"] > 0 {
		jsonTokens = formatTokens["json"] / formatTotal["json"]
	}

	for _, f := range formats {
		total := formatTotal[f]
		if total == 0 {
			continue
		}
		correct := formatCorrect[f]
		avgTokens := formatTokens[f] / total
		accuracy := 100.0 * float64(correct) / float64(total)

		vsJSON := "baseline"
		if f != "json" && jsonTokens > 0 {
			vsJSON = fmt.Sprintf("%.0f%%", 100.0*float64(avgTokens)/float64(jsonTokens))
		}

		t.Logf("%-8s %7.1f%% %10d %12s", f, accuracy, avgTokens, vsJSON)
	}

	t.Log("")
	t.Logf("If GCF accuracy >= JSON accuracy, GCF can be the default format.")
}

// renderContext converts a ContextBlock to a string in the given format.
func renderContext(block *knowingctx.ContextBlock, format string, st *store.SQLiteStore) (string, error) {
	switch format {
	case "gcf":
		payload, err := wire.FromContextBlock(context.Background(), block, "format_comprehension_eval", st)
		if err != nil {
			return "", err
		}
		return wire.Encode(payload), nil
	case "json":
		payload, err := wire.FromContextBlock(context.Background(), block, "format_comprehension_eval", st)
		if err != nil {
			return "", err
		}
		return wire.EncodeWith(format, payload)
	case "xml", "markdown":
		return knowingctx.FormatContextBlock(block, format)
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}

// estimateTokens uses the ~4 chars per token heuristic.
func estimateTokens(s string) int {
	return len(s) / 4
}

// callClaude sends a prompt to the Anthropic Messages API and returns the response text.
func callClaude(apiKey, model, prompt string) (string, error) {
	body := map[string]any{
		"model":      model,
		"max_tokens": 200,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return result.Content[0].Text, nil
}
