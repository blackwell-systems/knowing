package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReadHookInput_Neutral(t *testing.T) {
	in, err := readHookInput(strings.NewReader(`{"event":"pre-edit","file":"a.go","content":"func Foo() {}","task":"do x"}`))
	if err != nil {
		t.Fatalf("readHookInput: %v", err)
	}
	if in.Event != "pre-edit" || in.File != "a.go" || in.Task != "do x" {
		t.Fatalf("neutral fields not parsed: %+v", in)
	}
}

func TestReadHookInput_Empty(t *testing.T) {
	in, err := readHookInput(strings.NewReader("   \n"))
	if err != nil {
		t.Fatalf("empty stdin should be valid: %v", err)
	}
	if in.resolvedEvent() != "" {
		t.Fatalf("empty input should resolve to no event, got %q", in.resolvedEvent())
	}
}

func TestReadHookInput_ClaudeCodeFolding(t *testing.T) {
	// A raw Claude Code PreToolUse(Edit) payload must fold into neutral fields.
	raw := `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"pkg/x.go","old_string":"func Bar(){}"}}`
	in, err := readHookInput(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("readHookInput: %v", err)
	}
	if in.File != "pkg/x.go" {
		t.Errorf("file not folded from tool_input.file_path: %q", in.File)
	}
	if in.Content != "func Bar(){}" {
		t.Errorf("content not folded from tool_input.old_string: %q", in.Content)
	}
	if got := in.resolvedEvent(); got != "pre-edit" {
		t.Errorf("PreToolUse(Edit) should resolve to pre-edit, got %q", got)
	}
}

func TestResolvedEvent_Mapping(t *testing.T) {
	cases := []struct {
		name, hookEvent, tool, want string
	}{
		{"session", "SessionStart", "", "session-start"},
		{"compact", "PreCompact", "", "pre-compact"},
		{"stop", "Stop", "", "post-task"},
		{"prompt", "UserPromptSubmit", "", "pre-task"},
		{"edit", "PreToolUse", "Edit", "pre-edit"},
		{"write", "PreToolUse", "Write", "pre-edit"},
		{"task", "PreToolUse", "Task", "pre-task"},
		{"unknown-tool", "PreToolUse", "Bash", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := hookInput{HookEventName: c.hookEvent, ToolName: c.tool}
			if got := in.resolvedEvent(); got != c.want {
				t.Errorf("resolvedEvent(%s,%s) = %q, want %q", c.hookEvent, c.tool, got, c.want)
			}
		})
	}
}

func TestResolvedEvent_NeutralTakesPrecedence(t *testing.T) {
	in := hookInput{Event: "pre-task", HookEventName: "PreToolUse", ToolName: "Edit"}
	if got := in.resolvedEvent(); got != "pre-task" {
		t.Errorf("neutral event should win, got %q", got)
	}
}

func TestExtractEditSymbols(t *testing.T) {
	content := `func (s *Server) HandleRequest(req *Request) error {
	type Payload struct{}
	var counter int
	return s.processPayload(req)
}`
	syms := extractEditSymbols(content)
	joined := strings.Join(syms, " ")
	for _, want := range []string{"HandleRequest", "Payload", "counter"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected symbol %q in %v", want, syms)
		}
	}
	// Generic identifiers are filtered as noise.
	for _, s := range syms {
		if editSymbolSkip[s] {
			t.Errorf("skip-listed symbol %q should not appear", s)
		}
	}
}

func TestExtractEditSymbols_FallbackToCamelCase(t *testing.T) {
	// No structural matches; fall back to significant CamelCase identifiers.
	syms := extractEditSymbols("someValue := AnotherThing")
	if len(syms) == 0 {
		t.Fatal("expected fallback CamelCase extraction")
	}
	if strings.Join(syms, " ") == "" || !strings.Contains(strings.Join(syms, " "), "AnotherThing") {
		t.Errorf("expected AnotherThing in fallback, got %v", syms)
	}
}

func TestDeriveEditQuery_FilenameFallback(t *testing.T) {
	if got := deriveEditQuery("internal/store/sqlite.go", ""); got != "sqlite" {
		t.Errorf("expected basename-without-ext 'sqlite', got %q", got)
	}
	if got := deriveEditQuery("", ""); got != "" {
		t.Errorf("no file and no content should yield empty query, got %q", got)
	}
}

func TestDeriveEditQuery_PrefersSymbols(t *testing.T) {
	q := deriveEditQuery("x.go", "func ComputeScore() {}")
	if !strings.Contains(q, "ComputeScore") {
		t.Errorf("expected symbol-derived query, got %q", q)
	}
}

func TestWriteHookOutput_Neutral(t *testing.T) {
	var buf strings.Builder
	res := hookResult{Event: "pre-edit", Query: "Foo", Context: "@abc func Foo", Symbols: 3, Tokens: 42}
	if err := writeHookOutput(&buf, "neutral", res, hookInput{}, time.Now()); err != nil {
		t.Fatalf("writeHookOutput: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if out["event"] != "pre-edit" || out["query"] != "Foo" || out["source"] != "knowing" {
		t.Errorf("unexpected neutral output: %v", out)
	}
	if out["symbols"].(float64) != 3 {
		t.Errorf("symbols not reported: %v", out["symbols"])
	}
}

func TestWriteHookOutput_ClaudeCodePreEdit(t *testing.T) {
	var buf strings.Builder
	res := hookResult{Event: "pre-edit", Context: "@abc func Foo"}
	in := hookInput{File: "pkg/x.go"}
	if err := writeHookOutput(&buf, "claude-code", res, in, time.Now()); err != nil {
		t.Fatalf("writeHookOutput: %v", err)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName      string `json:"hookEventName"`
			PermissionDecision string `json:"permissionDecision"`
			AdditionalContext  string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("expected PreToolUse, got %q", out.HookSpecificOutput.HookEventName)
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("expected allow decision, got %q", out.HookSpecificOutput.PermissionDecision)
	}
	if !strings.Contains(out.HookSpecificOutput.AdditionalContext, "pkg/x.go") {
		t.Errorf("expected file path in additionalContext, got %q", out.HookSpecificOutput.AdditionalContext)
	}
}

func TestWriteHookOutput_ClaudeCodeEmptyIsNoOp(t *testing.T) {
	var buf strings.Builder
	res := hookResult{Event: "pre-edit", Context: ""}
	if err := writeHookOutput(&buf, "claude-code", res, hookInput{}, time.Now()); err != nil {
		t.Fatalf("writeHookOutput: %v", err)
	}
	if buf.String() != "" {
		t.Errorf("empty context should produce no output, got %q", buf.String())
	}
}
