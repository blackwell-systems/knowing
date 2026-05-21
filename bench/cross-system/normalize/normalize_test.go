package normalize

import "testing"

func TestSymbol(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// knowing format (repoURL://filepath.Symbol)
		{"github.com/pallets/flask://bench/cross-system/corpus/repos/flask/src/flask/sansio/scaffold.py.Scaffold.before_request", "Scaffold.before_request"},
		{"github.com/django/django://bench/cross-system/corpus/repos/django/django/db/models/query.py.QuerySet", "QuerySet"},
		{"github.com/rust-lang/cargo://bench/cross-system/corpus/repos/cargo/src/resolve.rs.resolve_dependencies", "resolve_dependencies"},
		{"github.com/org/repo://internal/store/sqlite.go.NewSQLiteStore", "NewSQLiteStore"},

		// knowing Go format
		{"github.com/blackwell-systems/knowing://github.com/blackwell-systems/knowing/internal/context.ContextEngine.ForTask", "ContextEngine.ForTask"},

		// SCIP trailing dot
		{"github.com/org/repo/pkg.FuncName.", "FuncName"},

		// grep format: file:line:content
		{"internal/auth/handler.go:42:func HandleLogin(", "HandleLogin"},
		{"app.py:10:def create_app(", "create_app"},
		{"src/index.ts:5:class Router {", "Router"},

		// Aider format: file:symbol
		{"handler.go:HandleLogin", "HandleLogin"},

		// file:line (no symbol, useless)
		{"handler.go:42", ""},

		// Python-style qualified names (ground truth format)
		{"flask.app.Flask.before_request", "Flask.before_request"},
		{"django.db.models.QuerySet", "QuerySet"},
		{"django.db.models.query.QuerySet.filter", "QuerySet.filter"},

		// Already simple
		{"FuncName", "FuncName"},
		{"Type.Method", "Type.Method"},

		// Edge cases
		{"", ""},
		{"   ", ""},
	}

	for _, tc := range cases {
		got := Symbol(tc.input)
		if got != tc.want {
			t.Errorf("Symbol(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMatchesGroundTruth(t *testing.T) {
	cases := []struct {
		retrieved   string
		groundTruth string
		want        bool
	}{
		// Exact match after normalization
		{"pkg.FuncName", "pkg.FuncName", true},

		// knowing vs Python-style ground truth (the critical case)
		{"github.com/pallets/flask://src/flask/sansio/scaffold.py.Scaffold.before_request",
			"flask.app.Flask.before_request", true}, // terminal "before_request" matches

		// Different class but same method (base class vs subclass)
		{"Scaffold.before_request", "Flask.before_request", true}, // terminal match, both have qualifiers

		// Simple terminal match
		{"HandleLogin", "auth.HandleLogin", true},
		{"auth.HandleLogin", "HandleLogin", true},

		// No match (different terminal names)
		{"Handler.Get", "Handler.Post", false},
		{"OtherFunc", "FuncName", false},

		// Substring match
		{"github.com/org/repo/pkg.FuncName", "FuncName", true},

		// Suffix match
		{"pkg.Type.Method", "Type.Method", true},

		// Empty
		{"", "pkg.FuncName", false},
		{"pkg.FuncName", "", false},

		// Same terminal but different qualifiers (should NOT match)
		{"User.save", "File.save", false},
	}

	for _, tc := range cases {
		got := MatchesGroundTruth(tc.retrieved, tc.groundTruth)
		if got != tc.want {
			t.Errorf("MatchesGroundTruth(%q, %q) = %v, want %v",
				tc.retrieved, tc.groundTruth, got, tc.want)
		}
	}
}
