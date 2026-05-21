package normalize

import "testing"

func TestSymbol(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// knowing format
		{"github.com/org/repo/pkg.FuncName", "pkg.FuncName"},
		{"github.com/org/repo/internal/auth.Handler", "auth.Handler"},

		// SCIP trailing dot
		{"github.com/org/repo/pkg.FuncName.", "pkg.FuncName"},

		// grep format: file:line:content
		{"internal/auth/handler.go:42:func HandleLogin(", "HandleLogin"},
		{"app.py:10:def create_app(", "create_app"},
		{"src/index.ts:5:class Router {", "Router"},

		// Aider format: file:symbol
		{"handler.go:HandleLogin", "HandleLogin"},

		// file:line (no symbol, useless)
		{"handler.go:42", ""},

		// Already normalized
		{"pkg.FuncName", "pkg.FuncName"},
		{"FuncName", "FuncName"},
		{"Type.Method", "Type.Method"},

		// Path-qualified
		{"internal/store/sqlite.SQLiteStore", "sqlite.SQLiteStore"},

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

		// Different qualification levels
		{"github.com/org/repo/pkg.FuncName", "pkg.FuncName", true},
		{"FuncName", "pkg.FuncName", true},

		// Suffix match
		{"pkg.Type.Method", "Type.Method", true},
		{"auth.Handler.ServeHTTP", "Handler.ServeHTTP", true},

		// No match
		{"OtherFunc", "pkg.FuncName", false},
		{"Handler.Get", "Handler.Post", false},

		// Empty
		{"", "pkg.FuncName", false},
		{"pkg.FuncName", "", false},
	}

	for _, tc := range cases {
		got := MatchesGroundTruth(tc.retrieved, tc.groundTruth)
		if got != tc.want {
			t.Errorf("MatchesGroundTruth(%q, %q) = %v, want %v",
				tc.retrieved, tc.groundTruth, got, tc.want)
		}
	}
}
