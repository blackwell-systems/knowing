package main

import (
	stdctx "context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/wire"
)

// cmdHook is the harness-agnostic context hook. It is the transport-neutral
// core that every agentic harness (Claude Code, Hermes, OpenCode, Codex, custom
// loops) can wire in as a config task rather than a bespoke code adapter.
//
// CONTRACT
//
// Input (stdin, JSON; every field optional):
//
//	{
//	  "event":   "session-start" | "pre-edit" | "pre-task" | "pre-files" | "post-task",
//	  "file":    "path/to/file.go",     // the file about to be edited
//	  "content": "<code being edited>",  // used to seed the query with edited symbols
//	  "task":    "<task or prompt text>",// used verbatim as the query
//	  "files":   ["a.go", "b.go"]        // changed-file set (pre-files)
//	}
//
// The event may also be supplied as the positional argument
// (`knowing hook pre-edit`), which takes precedence over the stdin "event".
//
// For interoperability the input parser is liberal: it also recognises the
// common Claude Code hook keys (hook_event_name, tool_name, tool_input.*,
// prompt) so a Claude Code hook can point straight at this command with
// `-emit claude-code` and no wrapper script.
//
// Output (stdout, JSON) with the default `-emit neutral`:
//
//	{
//	  "event":      "pre-edit",
//	  "query":      "<derived query>",
//	  "context":    "<GCF/text context, empty if none>",
//	  "symbols":    15,
//	  "tokens":     145,
//	  "latency_ms": 280,
//	  "source":     "knowing"
//	}
//
// With `-emit claude-code` the output is Claude Code's native hook envelope
// (hookSpecificOutput.additionalContext) instead, so the command can serve as
// the Claude Code hook directly. Additional per-harness adapters are documented
// in docs/architecture/hooks-integration.md and hooks/adapters/.
func cmdHook(args []string) error {
	// Pull a leading positional event (`knowing hook pre-edit -db ...`) before
	// flag parsing. Go's flag package stops at the first non-flag argument, so
	// without this the event would swallow every following flag.
	posEvent := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		posEvent = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	budget := fs.Int("budget", 400, "Token budget for injected context")
	format := fs.String("format", "gcf", "Context format (gcf/gcb/json/xml/markdown)")
	emit := fs.String("emit", "neutral", "Output envelope: neutral | claude-code")
	repo := fs.String("repo", "", "Repository URL for file resolution (pre-files)")
	eventFlag := fs.String("event", "", "Event name (overrides positional arg and stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	start := time.Now()

	in, err := readHookInput(os.Stdin)
	if err != nil {
		return fmt.Errorf("parsing hook input: %w", err)
	}
	event := firstNonEmpty(*eventFlag, posEvent, in.resolvedEvent())
	if event == "" {
		return fmt.Errorf("no event: pass it as `knowing hook <event>`, --event, or an \"event\" stdin field")
	}

	// Missing DB is not an error for a hook: harnesses invoke hooks on every
	// event, and a repo may simply not be indexed yet. Emit empty context so
	// the harness proceeds without noise.
	if _, statErr := os.Stat(*dbPath); os.IsNotExist(statErr) {
		return writeHookOutput(os.Stdout, *emit, hookResult{Event: event}, in, start)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := stdctx.Background()
	res := hookResult{Event: event}

	switch event {
	case "session-start", "pre-compact", "startup":
		res.Context = graphSummary(ctx, st)

	case "pre-edit", "pre-write", "pretooluse-edit":
		res.Query = deriveEditQuery(in.File, in.Content)
		if res.Query == "" {
			break
		}
		res.Context, res.Symbols, res.Tokens = runTaskContext(ctx, st, res.Query, *budget, *format)

	case "pre-task", "subagent", "prompt", "user-prompt":
		res.Query = strings.TrimSpace(in.Task)
		if res.Query == "" {
			break
		}
		res.Context, res.Symbols, res.Tokens = runTaskContext(ctx, st, res.Query, *budget, *format)

	case "pre-files":
		if len(in.Files) == 0 {
			break
		}
		res.Query = strings.Join(in.Files, ",")
		res.Context, res.Symbols, res.Tokens = runFileContext(ctx, st, in.Files, *repo, *budget, *format)

	case "post-task", "stop":
		// No injectable context. Feedback attribution is handled by the MCP
		// feedback tool / implicit-feedback layer, not by the hook path.

	default:
		return fmt.Errorf("unknown event %q (session-start, pre-edit, pre-task, pre-files, post-task)", event)
	}

	return writeHookOutput(os.Stdout, *emit, res, in, start)
}

// hookInput is the liberal on-the-wire input. Neutral keys are the contract;
// the Claude Code keys are accepted for direct wiring without a shell adapter.
type hookInput struct {
	// Neutral contract.
	Event   string   `json:"event"`
	File    string   `json:"file"`
	Content string   `json:"content"`
	Task    string   `json:"task"`
	Files   []string `json:"files"`

	// Claude Code compatibility.
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     *hookToolInput `json:"tool_input"`
	Input         *hookToolInput `json:"input"`
	Prompt        string         `json:"prompt"`
}

type hookToolInput struct {
	FilePath    string `json:"file_path"`
	Path        string `json:"path"`
	OldString   string `json:"old_string"`
	NewString   string `json:"new_string"`
	Content     string `json:"content"`
	Prompt      string `json:"prompt"`
	Description string `json:"description"`
}

// readHookInput reads stdin (if any) and normalises neutral + Claude Code keys
// into a single hookInput. Empty stdin is valid (session-start needs no input).
func readHookInput(r io.Reader) (hookInput, error) {
	var in hookInput
	raw, err := io.ReadAll(r)
	if err != nil {
		return in, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return in, nil
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, err
	}

	// Fold Claude Code tool_input / input into the neutral fields.
	ti := in.ToolInput
	if ti == nil {
		ti = in.Input
	}
	if ti != nil {
		if in.File == "" {
			in.File = firstNonEmpty(ti.FilePath, ti.Path)
		}
		if in.Content == "" {
			in.Content = firstNonEmpty(ti.OldString, ti.NewString, ti.Content)
		}
		if in.Task == "" {
			in.Task = firstNonEmpty(ti.Prompt, ti.Description)
		}
	}
	if in.Task == "" {
		in.Task = in.Prompt
	}
	return in, nil
}

// resolvedEvent maps a Claude Code hook_event_name (+ tool_name) onto a neutral
// event when the neutral "event" field is absent.
func (in hookInput) resolvedEvent() string {
	if in.Event != "" {
		return in.Event
	}
	switch in.HookEventName {
	case "SessionStart":
		return "session-start"
	case "PreCompact":
		return "pre-compact"
	case "Stop", "SubagentStop":
		return "post-task"
	case "UserPromptSubmit":
		return "pre-task"
	case "PreToolUse":
		switch in.ToolName {
		case "Edit", "Write", "MultiEdit":
			return "pre-edit"
		case "Task", "Agent":
			return "pre-task"
		}
	}
	return ""
}

// hookResult is the neutral output payload.
type hookResult struct {
	Event   string
	Query   string
	Context string
	Symbols int
	Tokens  int
}

func runTaskContext(ctx stdctx.Context, st *store.SQLiteStore, query string, budget int, format string) (string, int, int) {
	engine := knowingctx.NewContextEngine(st)
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: query,
		TokenBudget:     budget,
		Format:          format,
	})
	if err != nil || block == nil {
		return "", 0, 0
	}
	return encodeBlock(ctx, st, block, "context_for_task", format), len(block.Symbols), block.TokensUsed
}

func runFileContext(ctx stdctx.Context, st *store.SQLiteStore, files []string, repo string, budget int, format string) (string, int, int) {
	engine := knowingctx.NewContextEngine(st)
	block, err := engine.ForFiles(ctx, knowingctx.FileOptions{
		Files:       files,
		RepoURL:     repo,
		TokenBudget: budget,
		Format:      format,
	})
	if err != nil || block == nil {
		return "", 0, 0
	}
	return encodeBlock(ctx, st, block, "context_for_files", format), len(block.Symbols), block.TokensUsed
}

// encodeBlock renders a context block, matching the encoder split used by the
// `context` command (wire codecs for gcf/gcb/json, formatter otherwise).
func encodeBlock(ctx stdctx.Context, st *store.SQLiteStore, block *knowingctx.ContextBlock, tool, format string) string {
	switch format {
	case "gcf", "gcb", "json":
		payload, err := wire.FromContextBlock(ctx, block, tool, st)
		if err != nil {
			return ""
		}
		out, err := wire.EncodeWith(format, payload)
		if err != nil {
			return ""
		}
		return out
	default:
		out, err := knowingctx.FormatContextBlock(block, format)
		if err != nil {
			return ""
		}
		return out
	}
}

// graphSummary is the ambient capability blurb injected at session start so the
// agent knows a graph exists and which tools to reach for.
func graphSummary(ctx stdctx.Context, st *store.SQLiteStore) string {
	nodes, _ := st.RealNodeCount(ctx)
	edges, _ := st.EdgeCount(ctx)
	if nodes == 0 {
		return ""
	}
	return fmt.Sprintf("[knowing graph available] Content-addressed code graph indexed with "+
		"%d symbols and %d relationships. Context tools: context_for_task (graph-ranked context "+
		"for a task), context_for_files (blast radius for changed files), context_for_pr (PR impact), "+
		"blast_radius (callers of a symbol), cross_repo_callers (callers across repos). "+
		"Request format=gcf for ~84%% fewer tokens on responses.", nodes, edges)
}

// deriveEditQuery builds a retrieval query from an impending edit: the symbols
// named in the edited code (best signal), falling back to the file's basename.
// Ported from the Claude Code pre-edit shell hook so the logic is harness-neutral.
func deriveEditQuery(file, content string) string {
	syms := extractEditSymbols(content)
	if len(syms) > 0 {
		if len(syms) > 5 {
			syms = syms[:5]
		}
		return strings.Join(syms, " ")
	}
	if file == "" {
		return ""
	}
	base := filepath.Base(file)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

var (
	reFunc    = regexp.MustCompile(`func\s+(?:\([^)]*\)\s+)?(\w+)\s*\(`)
	reType    = regexp.MustCompile(`type\s+(\w+)\s+(?:struct|interface)`)
	reVarDecl = regexp.MustCompile(`(?:var|const)\s+(\w+)`)
	reMethod  = regexp.MustCompile(`\.(\w+)\s*\(`)
	rePkgCall = regexp.MustCompile(`(\w+)\.(\w+)\s*\(`)
	reUpper   = regexp.MustCompile(`\b([A-Z]\w{2,})\b`)
)

// editSymbolSkip mirrors the Go-keyword filter in the reference shell hook:
// generic identifiers that add BM25 noise rather than seeding the walk.
var editSymbolSkip = map[string]bool{
	"Context": true, "Error": true, "String": true, "Close": true,
	"Start": true, "Stop": true, "New": true, "Get": true, "Set": true,
}

func extractEditSymbols(content string) []string {
	if content == "" {
		return nil
	}
	set := map[string]bool{}
	add := func(s string) {
		if s != "" && !editSymbolSkip[s] {
			set[s] = true
		}
	}
	for _, m := range reFunc.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	for _, m := range reType.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	for _, m := range reVarDecl.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	for _, m := range reMethod.FindAllStringSubmatch(content, -1) {
		name := m[1]
		if len(name) > 2 && name[0] >= 'A' && name[0] <= 'Z' {
			add(name)
		}
	}
	for _, m := range rePkgCall.FindAllStringSubmatch(content, -1) {
		add(m[2])
	}
	// Fallback: significant CamelCase identifiers if nothing structural matched.
	if len(set) == 0 {
		for _, m := range reUpper.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	}

	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// writeHookOutput renders the result in the requested envelope.
func writeHookOutput(w io.Writer, emit string, res hookResult, in hookInput, start time.Time) error {
	latency := time.Since(start).Milliseconds()

	switch emit {
	case "claude-code", "claude":
		return writeClaudeCodeEnvelope(w, res, in)
	case "neutral", "":
		out := map[string]any{
			"event":      res.Event,
			"context":    res.Context,
			"symbols":    res.Symbols,
			"tokens":     res.Tokens,
			"latency_ms": latency,
			"source":     "knowing",
		}
		if res.Query != "" {
			out["query"] = res.Query
		}
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(out)
	default:
		return fmt.Errorf("unknown --emit %q (neutral, claude-code)", emit)
	}
}

// writeClaudeCodeEnvelope emits Claude Code's native hook schema. Empty context
// produces empty output (exit 0), which Claude Code treats as a no-op.
func writeClaudeCodeEnvelope(w io.Writer, res hookResult, in hookInput) error {
	if res.Context == "" {
		return nil
	}

	var additional string
	var hookEventName string
	switch res.Event {
	case "session-start", "startup":
		hookEventName = "SessionStart"
		additional = res.Context
	case "pre-compact":
		hookEventName = "PreCompact"
		additional = res.Context
	case "pre-edit", "pre-write":
		hookEventName = "PreToolUse"
		target := in.File
		if target == "" {
			target = "this file"
		}
		additional = fmt.Sprintf("[knowing context for %s] Graph-ranked symbols related to this file:\n%s", target, res.Context)
	default:
		hookEventName = "PreToolUse"
		additional = fmt.Sprintf("[knowing context] Graph-ranked symbols related to this task:\n%s", res.Context)
	}

	envelope := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     hookEventName,
			"additionalContext": additional,
		},
	}
	// PreToolUse also carries an explicit allow decision (matches the legacy hook).
	if hookEventName == "PreToolUse" {
		envelope["hookSpecificOutput"].(map[string]any)["permissionDecision"] = "allow"
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(envelope)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
