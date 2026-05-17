package scipingest

import "testing"

func TestParseSCIPSymbol_GoType(t *testing.T) {
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("scip-go gomod github.com/org/repo v1.0.0 pkg/MyType.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "github.com/org/repo" {
		t.Errorf("repo = %q, want %q", repo, "github.com/org/repo")
	}
	if pkgPath != "pkg" {
		t.Errorf("pkgPath = %q, want %q", pkgPath, "pkg")
	}
	if name != "MyType" {
		t.Errorf("name = %q, want %q", name, "MyType")
	}
	if kind != "type" {
		t.Errorf("kind = %q, want %q", kind, "type")
	}
}

func TestParseSCIPSymbol_Method(t *testing.T) {
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("scip-go gomod github.com/org/repo v1.0.0 pkg/MyType.Method().")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "github.com/org/repo" {
		t.Errorf("repo = %q, want %q", repo, "github.com/org/repo")
	}
	if pkgPath != "pkg" {
		t.Errorf("pkgPath = %q, want %q", pkgPath, "pkg")
	}
	if name != "Method" {
		t.Errorf("name = %q, want %q", name, "Method")
	}
	if kind != "method" {
		t.Errorf("kind = %q, want %q", kind, "method")
	}
}

func TestParseSCIPSymbol_Function(t *testing.T) {
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("scip-go gomod github.com/org/repo v1.0.0 pkg/DoThing().")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "github.com/org/repo" {
		t.Errorf("repo = %q, want %q", repo, "github.com/org/repo")
	}
	if pkgPath != "pkg" {
		t.Errorf("pkgPath = %q, want %q", pkgPath, "pkg")
	}
	if name != "DoThing" {
		t.Errorf("name = %q, want %q", name, "DoThing")
	}
	// "()." suffix is method pattern
	if kind != "method" {
		t.Errorf("kind = %q, want %q", kind, "method")
	}
}

func TestParseSCIPSymbol_TopLevelFunction(t *testing.T) {
	// A function at package level without type prefix: ends with "()"
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("scip-go gomod github.com/org/repo v1.0.0 pkg/DoThing()")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "github.com/org/repo" {
		t.Errorf("repo = %q, want %q", repo, "github.com/org/repo")
	}
	if pkgPath != "pkg" {
		t.Errorf("pkgPath = %q, want %q", pkgPath, "pkg")
	}
	if name != "DoThing" {
		t.Errorf("name = %q, want %q", name, "DoThing")
	}
	if kind != "function" {
		t.Errorf("kind = %q, want %q", kind, "function")
	}
}

func TestParseSCIPSymbol_Local(t *testing.T) {
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("local 42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "" {
		t.Errorf("repo = %q, want empty", repo)
	}
	if pkgPath != "" {
		t.Errorf("pkgPath = %q, want empty", pkgPath)
	}
	if name != "42" {
		t.Errorf("name = %q, want %q", name, "42")
	}
	if kind != "function" {
		t.Errorf("kind = %q, want %q", kind, "function")
	}
}

func TestParseSCIPSymbol_TypeScript(t *testing.T) {
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("scip-typescript npm @types/node 16.0.0 path/posix.resolve().")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "@types/node" {
		t.Errorf("repo = %q, want %q", repo, "@types/node")
	}
	if pkgPath != "path" {
		t.Errorf("pkgPath = %q, want %q", pkgPath, "path")
	}
	if name != "resolve" {
		t.Errorf("name = %q, want %q", name, "resolve")
	}
	if kind != "method" {
		t.Errorf("kind = %q, want %q", kind, "method")
	}
}

func TestParseSCIPSymbol_Empty(t *testing.T) {
	_, _, _, _, err := ParseSCIPSymbol("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseSCIPSymbol_Invalid(t *testing.T) {
	_, _, _, _, err := ParseSCIPSymbol("not enough fields")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestParseSCIPSymbol_Field(t *testing.T) {
	repo, pkgPath, name, kind, err := ParseSCIPSymbol("scip-go gomod github.com/org/repo v1.0.0 pkg/MyType.FieldName#")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo != "github.com/org/repo" {
		t.Errorf("repo = %q, want %q", repo, "github.com/org/repo")
	}
	if pkgPath != "pkg" {
		t.Errorf("pkgPath = %q, want %q", pkgPath, "pkg")
	}
	if name != "FieldName" {
		t.Errorf("name = %q, want %q", name, "FieldName")
	}
	if kind != "var" {
		t.Errorf("kind = %q, want %q", kind, "var")
	}
}
