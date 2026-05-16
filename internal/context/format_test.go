package context

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeRankedSymbol(name, kind, sig, prov string, score float64, dist int, comps ScoreComponents) RankedSymbol {
	return RankedSymbol{
		Node: types.Node{
			QualifiedName: name,
			Kind:          kind,
			Signature:     sig,
		},
		Score:      score,
		Provenance: prov,
		Distance:   dist,
		Components: comps,
	}
}

func TestFormatXML_Basic(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  1200,
		TokenBudget: 50000,
		Symbols: []RankedSymbol{
			makeRankedSymbol("github.com/example/pkg.DoSomething", "function", "func DoSomething(ctx context.Context) error", "runtime_observed", 0.95, 0, ScoreComponents{BlastRadius: 0.40, Confidence: 0.25, Recency: 0.20, Distance: 0.15}),
			makeRankedSymbol("github.com/example/pkg.Helper", "function", "func Helper() string", "ast_resolved", 0.72, 1, ScoreComponents{BlastRadius: 0.30, Confidence: 0.20, Recency: 0.12, Distance: 0.10}),
		},
	}

	output, err := FormatContextBlock(block, "xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "<context") {
		t.Error("output missing <context> element")
	}
	if !strings.Contains(output, `tokens_used="1200"`) {
		t.Error("output missing tokens_used attribute")
	}
	if !strings.Contains(output, `token_budget="50000"`) {
		t.Error("output missing token_budget attribute")
	}
	if !strings.Contains(output, "<target_symbols>") {
		t.Error("output missing <target_symbols> section")
	}
	if !strings.Contains(output, "<related_symbols>") {
		t.Error("output missing <related_symbols> section")
	}
	if !strings.Contains(output, "DoSomething") {
		t.Error("output missing target symbol name")
	}
	if !strings.Contains(output, "Helper") {
		t.Error("output missing related symbol name")
	}
	if !strings.Contains(output, "<relationship_summary>") {
		t.Error("output missing <relationship_summary> section")
	}
}

func TestFormatMarkdown_Basic(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  800,
		TokenBudget: 50000,
		Symbols: []RankedSymbol{
			makeRankedSymbol("github.com/example/pkg.DoSomething", "function", "func DoSomething(ctx context.Context) error", "runtime_observed", 0.95, 0, ScoreComponents{}),
			makeRankedSymbol("github.com/example/pkg.Helper", "function", "", "ast_resolved", 0.72, 1, ScoreComponents{}),
		},
	}

	output, err := FormatContextBlock(block, "markdown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "# Context (800/50000 tokens)") {
		t.Error("output missing context header")
	}
	if !strings.Contains(output, "## Target Symbols") {
		t.Error("output missing Target Symbols section")
	}
	if !strings.Contains(output, "`github.com/example/pkg.DoSomething`") {
		t.Error("output missing target symbol")
	}
	if !strings.Contains(output, "## Related Symbols (distance: 1)") {
		t.Error("output missing Related Symbols section")
	}
	if !strings.Contains(output, "`github.com/example/pkg.Helper`") {
		t.Error("output missing related symbol")
	}
}

func TestFormatJSON_Basic(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  500,
		TokenBudget: 50000,
		Symbols: []RankedSymbol{
			makeRankedSymbol("github.com/example/pkg.DoSomething", "function", "func DoSomething() error", "runtime_observed", 0.95, 0, ScoreComponents{BlastRadius: 0.40, Confidence: 0.25, Recency: 0.20, Distance: 0.15}),
		},
	}

	output, err := FormatContextBlock(block, "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed jsonOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if parsed.TokensUsed != 500 {
		t.Errorf("tokens_used = %d, want 500", parsed.TokensUsed)
	}
	if parsed.TokenBudget != 50000 {
		t.Errorf("token_budget = %d, want 50000", parsed.TokenBudget)
	}
	if len(parsed.Symbols) != 1 {
		t.Fatalf("symbols count = %d, want 1", len(parsed.Symbols))
	}
	if parsed.Symbols[0].QualifiedName != "github.com/example/pkg.DoSomething" {
		t.Errorf("qualified_name = %q, want github.com/example/pkg.DoSomething", parsed.Symbols[0].QualifiedName)
	}
	if parsed.Symbols[0].Components.BlastRadius != 0.40 {
		t.Errorf("blast_radius = %f, want 0.40", parsed.Symbols[0].Components.BlastRadius)
	}
}

func TestFormatUnknown(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  100,
		TokenBudget: 50000,
	}

	_, err := FormatContextBlock(block, "invalid")
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Errorf("error = %q, want it to contain 'unknown format'", err.Error())
	}
}

func TestFormatEmpty(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  0,
		TokenBudget: 50000,
		Symbols:     nil,
	}

	xmlOut, err := FormatContextBlock(block, "xml")
	if err != nil {
		t.Fatalf("xml format error on empty block: %v", err)
	}
	if !strings.Contains(xmlOut, "<context") {
		t.Error("empty XML output missing <context> element")
	}
	if !strings.Contains(xmlOut, "<total_symbols>0</total_symbols>") {
		t.Error("empty XML output should report 0 total symbols")
	}

	mdOut, err := FormatContextBlock(block, "markdown")
	if err != nil {
		t.Fatalf("markdown format error on empty block: %v", err)
	}
	if !strings.Contains(mdOut, "# Context") {
		t.Error("empty markdown output missing header")
	}

	jsonOut, err := FormatContextBlock(block, "json")
	if err != nil {
		t.Fatalf("json format error on empty block: %v", err)
	}
	var parsed jsonOutput
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("empty JSON output is not valid: %v", err)
	}
	if len(parsed.Symbols) != 0 {
		t.Errorf("empty JSON symbols count = %d, want 0", len(parsed.Symbols))
	}
}

func TestFormatXML_Grouping(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  2000,
		TokenBudget: 50000,
		Symbols: []RankedSymbol{
			makeRankedSymbol("pkg.Target", "function", "func Target()", "runtime_observed", 0.99, 0, ScoreComponents{}),
			makeRankedSymbol("pkg.DirectRelation", "method", "func (r *R) DirectRelation()", "ast_resolved", 0.75, 1, ScoreComponents{}),
			makeRankedSymbol("pkg.Extended", "type", "type Extended struct{}", "ast_inferred", 0.50, 2, ScoreComponents{}),
		},
	}

	output, err := FormatContextBlock(block, "xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "<target_symbols>") {
		t.Error("missing <target_symbols> section")
	}
	if !strings.Contains(output, "<related_symbols>") {
		t.Error("missing <related_symbols> section")
	}
	if !strings.Contains(output, "<extended_context>") {
		t.Error("missing <extended_context> section")
	}

	targetIdx := strings.Index(output, "<target_symbols>")
	relatedIdx := strings.Index(output, "<related_symbols>")
	extendedIdx := strings.Index(output, "<extended_context>")

	targetSymIdx := strings.Index(output, "pkg.Target")
	relatedSymIdx := strings.Index(output, "pkg.DirectRelation")
	extendedSymIdx := strings.Index(output, "pkg.Extended")

	if targetSymIdx < targetIdx || targetSymIdx > relatedIdx {
		t.Error("Target symbol not in <target_symbols> section")
	}
	if relatedSymIdx < relatedIdx || relatedSymIdx > extendedIdx {
		t.Error("DirectRelation symbol not in <related_symbols> section")
	}
	if extendedSymIdx < extendedIdx {
		t.Error("Extended symbol not in <extended_context> section")
	}

	if !strings.Contains(output, `<distance hop="0" count="1"/>`) {
		t.Error("missing distance hop=0 count")
	}
	if !strings.Contains(output, `<distance hop="1" count="1"/>`) {
		t.Error("missing distance hop=1 count")
	}
	if !strings.Contains(output, `<distance hop="2" count="1"/>`) {
		t.Error("missing distance hop=2 count")
	}
}

func TestFormatXML_DefaultFormat(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  100,
		TokenBudget: 50000,
		Symbols: []RankedSymbol{
			makeRankedSymbol("pkg.Func", "function", "", "", 0.80, 0, ScoreComponents{}),
		},
	}

	output, err := FormatContextBlock(block, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "<context") {
		t.Error("empty format should default to XML")
	}
}

func TestFormatXML_EscapesSpecialChars(t *testing.T) {
	block := &ContextBlock{
		TokensUsed:  100,
		TokenBudget: 50000,
		Symbols: []RankedSymbol{
			makeRankedSymbol("pkg.Func", "function", "func Func(a <T>, b &U) bool", "", 0.80, 0, ScoreComponents{}),
		},
	}

	output, err := FormatContextBlock(block, "xml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(output, "<T>") && !strings.Contains(output, "&lt;T&gt;") {
		t.Error("signature with angle brackets not properly escaped")
	}
}
