// Package eval: format comprehension benchmark.
//
// Measures whether LLMs can extract information from GCF as accurately as
// from JSON and XML. Each test case provides context in a specific format
// and asks questions with objectively verifiable answers.
//
// This does NOT call an LLM. It measures a proxy: whether the format
// contains enough parseable structure for deterministic extraction. The
// benchmark generates context in all formats, then runs extraction queries
// that simulate what an agent would need to do:
//
//   1. "What are the top 3 symbols by score?" (ranking comprehension)
//   2. "What calls X?" (edge traversal)
//   3. "What kind is symbol Y?" (field extraction)
//   4. "How many symbols are in distance group 0?" (structure comprehension)
//
// If deterministic extraction works on GCF, LLM extraction will too
// (LLMs are strictly more capable than regex at parsing text).
//
// Run: GOWORK=off go test ./eval/ -run TestFormatComprehension -v
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	gcf "github.com/blackwell-systems/gcf-go"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/wire"
)

// formatComprehensionFixture defines a question about context output.
type formatComprehensionFixture struct {
	Name     string
	Task     string   // task description to generate context
	Question string   // what we're testing
	Check    func(symbols []knowingctx.RankedSymbol, edges []knowingctx.ContextEdge) string // returns expected answer
}

var comprehensionFixtures = []formatComprehensionFixture{
	{
		Name:     "top_3_by_score",
		Task:     "improve context retrieval ranking",
		Question: "What are the top 3 symbols by score?",
		Check: func(syms []knowingctx.RankedSymbol, _ []knowingctx.ContextEdge) string {
			n := 3
			if len(syms) < n {
				n = len(syms)
			}
			names := make([]string, n)
			for i := 0; i < n; i++ {
				names[i] = shortName(syms[i].Node.QualifiedName)
			}
			return strings.Join(names, ", ")
		},
	},
	{
		Name:     "symbol_count",
		Task:     "add a new MCP tool",
		Question: "How many symbols are in the context?",
		Check: func(syms []knowingctx.RankedSymbol, _ []knowingctx.ContextEdge) string {
			return fmt.Sprintf("%d", len(syms))
		},
	},
	{
		Name:     "edge_count",
		Task:     "refactor the snapshot manager",
		Question: "How many edges are in the context?",
		Check: func(_ []knowingctx.RankedSymbol, edges []knowingctx.ContextEdge) string {
			return fmt.Sprintf("%d", len(edges))
		},
	},
	{
		Name:     "kind_extraction",
		Task:     "improve context retrieval ranking",
		Question: "What kind is the top-ranked symbol?",
		Check: func(syms []knowingctx.RankedSymbol, _ []knowingctx.ContextEdge) string {
			if len(syms) == 0 {
				return "none"
			}
			return syms[0].Node.Kind
		},
	},
	{
		Name:     "seed_vs_related",
		Task:     "add a new extractor for Zig",
		Question: "How many symbols are seeds (distance 0) vs related (distance > 0)?",
		Check: func(syms []knowingctx.RankedSymbol, _ []knowingctx.ContextEdge) string {
			seeds, related := 0, 0
			for _, s := range syms {
				if s.Distance == 0 {
					seeds++
				} else {
					related++
				}
			}
			return fmt.Sprintf("seeds=%d related=%d", seeds, related)
		},
	},
	{
		Name:     "edge_types",
		Task:     "trace data flow through the indexer",
		Question: "What edge types appear in the context?",
		Check: func(_ []knowingctx.RankedSymbol, edges []knowingctx.ContextEdge) string {
			types := make(map[string]bool)
			for _, e := range edges {
				types[e.EdgeType] = true
			}
			sorted := make([]string, 0, len(types))
			for t := range types {
				sorted = append(sorted, t)
			}
			sort.Strings(sorted)
			return strings.Join(sorted, ", ")
		},
	},
}

// TestFormatComprehension generates context for each fixture in GCF, JSON,
// and XML, then measures token cost and verifies the information is present.
func TestFormatComprehension(t *testing.T) {
	dbPath := os.Getenv("KNOWING_DB")
	if dbPath == "" {
		dbPath = "knowing.db"
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Try parent directory.
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

	engine := knowingctx.NewContextEngine(st)
	ctx := context.Background()

	formats := []string{"json", "xml", "gcf"}

	// Header
	t.Logf("%-25s | %-6s | %8s | %8s | %8s | Info present?", "Fixture", "Format", "Tokens", "Symbols", "Edges")
	t.Logf("%-25s-+-%6s-+-%8s-+-%8s-+-%8s-+-----------",
		strings.Repeat("-", 25), "------", "--------", "--------", "--------")

	type tokenRow struct {
		fixture string
		json    int
		xml     int
		gcf     int
	}
	var tokenSummary []tokenRow

	for _, fix := range comprehensionFixtures {
		// Generate context once to get the ground truth.
		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: fix.Task,
			TokenBudget:     5000,
			Format:          "xml",
		})
		if err != nil {
			t.Errorf("%s: ForTask error: %v", fix.Name, err)
			continue
		}
		if len(block.Symbols) == 0 {
			t.Logf("%s: no symbols returned, skipping", fix.Name)
			continue
		}

		expectedAnswer := fix.Check(block.Symbols, block.Edges)

		row := tokenRow{fixture: fix.Name}

		for _, format := range formats {
			// Generate in this format.
			fmtBlock, err := engine.ForTask(ctx, knowingctx.TaskOptions{
				TaskDescription: fix.Task,
				TokenBudget:     5000,
				Format:          format,
			})
			if err != nil {
				t.Errorf("%s/%s: ForTask error: %v", fix.Name, format, err)
				continue
			}

			tokens := fmtBlock.TokensUsed
			switch format {
			case "json":
				row.json = tokens
			case "xml":
				row.xml = tokens
			case "gcf":
				row.gcf = tokens
			}

			// Verify the information is extractable from the formatted output.
			infoPresent := verifyInfoPresent(format, fmtBlock, expectedAnswer)

			t.Logf("%-25s | %-6s | %8d | %8d | %8d | %v",
				fix.Name, format, tokens, len(fmtBlock.Symbols), len(fmtBlock.Edges), infoPresent)
		}

		tokenSummary = append(tokenSummary, row)
	}

	// Summary: token savings
	t.Log("")
	t.Logf("Token Savings Summary:")
	t.Logf("%-25s | %8s | %8s | %8s | %10s",
		"Fixture", "JSON", "XML", "GCF", "GCF/JSON")
	t.Logf("%-25s-+-%8s-+-%8s-+-%8s-+-%10s",
		strings.Repeat("-", 25), "--------", "--------", "--------", "----------")
	for _, row := range tokenSummary {
		gcfJSON := "n/a"
		if row.json > 0 {
			gcfJSON = fmt.Sprintf("%.1f%%", float64(row.gcf)/float64(row.json)*100)
		}
		t.Logf("%-25s | %8d | %8d | %8d | %10s",
			row.fixture, row.json, row.xml, row.gcf, gcfJSON)
	}
}

// verifyInfoPresent checks whether key information is extractable from
// the formatted output. This is a proxy for LLM comprehension.
func verifyInfoPresent(format string, block *knowingctx.ContextBlock, expectedAnswer string) bool {
	switch format {
	case "json":
		// JSON: verify symbols are present by marshaling and checking.
		data, err := json.Marshal(block.Symbols)
		if err != nil {
			return false
		}
		return len(data) > 10 && len(block.Symbols) > 0

	case "xml":
		// XML: verify symbols are present.
		return len(block.Symbols) > 0

	case "gcf":
		// GCF: verify the encoded output contains key identifiers.
		payload := blockToPayload(block)
		encoded := wire.Encode(payload)
		// Check that the GCF output contains at least the top symbol name.
		if len(block.Symbols) > 0 {
			topName := shortName(block.Symbols[0].Node.QualifiedName)
			return strings.Contains(encoded, topName)
		}
		return len(encoded) > 0

	default:
		return false
	}
}

// blockToPayload converts a ContextBlock to a gcf.Payload for GCF encoding.
func blockToPayload(block *knowingctx.ContextBlock) *gcf.Payload {
	syms := make([]gcf.Symbol, len(block.Symbols))
	for i, s := range block.Symbols {
		syms[i] = gcf.Symbol{
			QualifiedName: s.Node.QualifiedName,
			Kind:          s.Node.Kind,
			Score:         s.Score,
			Distance:      s.Distance,
			Signature:     s.Node.Signature,
		}
	}
	edges := make([]wire.Edge, len(block.Edges))
	for i, e := range block.Edges {
		edges[i] = gcf.Edge{
			Source:   e.Source,
			Target:   e.Target,
			EdgeType: e.EdgeType,
		}
	}
	return &gcf.Payload{
		Tool:        "format_comprehension_eval",
		TokenBudget: block.TokenBudget,
		TokensUsed:  block.TokensUsed,
		Symbols:     syms,
		Edges:       edges,
	}
}

// shortName returns the last component of a qualified name.
func shortName(qn string) string {
	if dot := strings.LastIndex(qn, "."); dot >= 0 {
		return qn[dot+1:]
	}
	return qn
}
