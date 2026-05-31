package goresolve

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// parseGoSource parses Go source code and returns the root AST node.
func parseGoSource(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return tree.RootNode()
}

// findTypeNode locates the type node of the first var_declaration in
// the AST. It navigates: source_file -> var_declaration -> var_spec -> type.
func findTypeNode(t *testing.T, root *sitter.Node) *sitter.Node {
	t.Helper()
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "var_declaration" {
			for j := 0; j < int(child.ChildCount()); j++ {
				spec := child.Child(j)
				if spec.Type() == "var_spec" {
					typeNode := spec.ChildByFieldName("type")
					if typeNode != nil {
						return typeNode
					}
				}
			}
		}
	}
	t.Fatal("no type node found in var declaration")
	return nil
}

func TestParseTypeNode(t *testing.T) {
	const pkg = "mypackage"

	tests := []struct {
		name     string
		src      string
		imports  map[string]string
		expected *typresolve.Type
	}{
		{
			name:     "simple named type",
			src:      "package p\nvar x MyType",
			expected: typresolve.Named("mypackage.MyType"),
		},
		{
			name:     "builtin int",
			src:      "package p\nvar x int",
			expected: typresolve.Builtin("int"),
		},
		{
			name:     "builtin string",
			src:      "package p\nvar x string",
			expected: typresolve.Builtin("string"),
		},
		{
			name:     "builtin bool",
			src:      "package p\nvar x bool",
			expected: typresolve.Builtin("bool"),
		},
		{
			name:     "builtin error",
			src:      "package p\nvar x error",
			expected: typresolve.Builtin("error"),
		},
		{
			name:     "builtin byte",
			src:      "package p\nvar x byte",
			expected: typresolve.Builtin("byte"),
		},
		{
			name:     "builtin rune",
			src:      "package p\nvar x rune",
			expected: typresolve.Builtin("rune"),
		},
		{
			name:     "builtin any",
			src:      "package p\nvar x any",
			expected: typresolve.Builtin("any"),
		},
		{
			name:     "builtin uintptr",
			src:      "package p\nvar x uintptr",
			expected: typresolve.Builtin("uintptr"),
		},
		{
			name:     "pointer to named type",
			src:      "package p\nvar x *MyType",
			expected: typresolve.Pointer(typresolve.Named("mypackage.MyType")),
		},
		{
			name:     "pointer to builtin",
			src:      "package p\nvar x *int",
			expected: typresolve.Pointer(typresolve.Builtin("int")),
		},
		{
			name:     "slice of builtin",
			src:      "package p\nvar x []string",
			expected: typresolve.Slice(typresolve.Builtin("string")),
		},
		{
			name:     "slice of named type",
			src:      "package p\nvar x []MyType",
			expected: typresolve.Slice(typresolve.Named("mypackage.MyType")),
		},
		{
			name:     "map of builtins",
			src:      "package p\nvar x map[string]int",
			expected: typresolve.Map(typresolve.Builtin("string"), typresolve.Builtin("int")),
		},
		{
			name:    "qualified type with imports",
			src:     "package p\nvar x http.Request",
			imports: map[string]string{"http": "net/http"},
			expected: typresolve.Named("net/http.Request"),
		},
		{
			name:     "function type",
			src:      "package p\nvar x func(int) string",
			expected: typresolve.Func(nil, nil),
		},
		{
			name:     "interface type",
			src:      "package p\nvar x interface{}",
			expected: &typresolve.Type{Kind: typresolve.KindInterface},
		},
		{
			name:     "struct type",
			src:      "package p\nvar x struct{}",
			expected: &typresolve.Type{Kind: typresolve.KindStruct},
		},
		{
			name:     "nil node returns nil",
			src:      "",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "nil node returns nil" {
				got := ParseTypeNode(nil, nil, pkg, nil)
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}

			content := []byte(tc.src)
			root := parseGoSource(t, tc.src)
			node := findTypeNode(t, root)

			imports := tc.imports
			if imports == nil {
				imports = make(map[string]string)
			}

			got := ParseTypeNode(node, content, pkg, imports)
			if !typresolve.TypesEqual(got, tc.expected) {
				t.Errorf("ParseTypeNode() = %+v, want %+v", got, tc.expected)
			}
		})
	}
}

func TestParseTypeNode_MultiReturn(t *testing.T) {
	// Multi-return parameter_list: find the result field of a function type.
	src := `package p
func foo() (int, error) { return 0, nil }
`
	content := []byte(src)
	root := parseGoSource(t, src)

	// Find the function_declaration, then its result parameter_list.
	var funcDecl *sitter.Node
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_declaration" {
			funcDecl = child
			break
		}
	}
	if funcDecl == nil {
		t.Fatal("no function_declaration found")
	}

	resultNode := funcDecl.ChildByFieldName("result")
	if resultNode == nil {
		t.Fatal("no result field on function_declaration")
	}

	got := ParseTypeNode(resultNode, content, "pkg", make(map[string]string))
	expected := typresolve.Tuple([]*typresolve.Type{
		typresolve.Builtin("int"),
		typresolve.Builtin("error"),
	})

	if !typresolve.TypesEqual(got, expected) {
		t.Errorf("multi-return: got %+v, want %+v", got, expected)
	}
}

func TestParseTypeNode_SingleReturn(t *testing.T) {
	src := `package p
func foo() (int) { return 0 }
`
	content := []byte(src)
	root := parseGoSource(t, src)

	var funcDecl *sitter.Node
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_declaration" {
			funcDecl = child
			break
		}
	}
	if funcDecl == nil {
		t.Fatal("no function_declaration found")
	}

	resultNode := funcDecl.ChildByFieldName("result")
	if resultNode == nil {
		t.Fatal("no result field on function_declaration")
	}

	got := ParseTypeNode(resultNode, content, "pkg", make(map[string]string))
	// Single element parameter_list should unwrap to the element itself.
	expected := typresolve.Builtin("int")

	if !typresolve.TypesEqual(got, expected) {
		t.Errorf("single return: got %+v, want %+v", got, expected)
	}
}

func TestParseTypeNode_Channel(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected *typresolve.Type
	}{
		{
			name:     "bidirectional channel",
			src:      "package p\nvar x chan int",
			expected: typresolve.Channel(typresolve.Builtin("int"), typresolve.ChanBidi),
		},
		{
			name:     "send-only channel",
			src:      "package p\nvar x chan<- int",
			expected: typresolve.Channel(typresolve.Builtin("int"), typresolve.ChanSend),
		},
		{
			name:     "receive-only channel",
			src:      "package p\nvar x <-chan int",
			expected: typresolve.Channel(typresolve.Builtin("int"), typresolve.ChanRecv),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte(tc.src)
			root := parseGoSource(t, tc.src)
			node := findTypeNode(t, root)
			got := ParseTypeNode(node, content, "pkg", make(map[string]string))
			if !typresolve.TypesEqual(got, tc.expected) {
				t.Errorf("got %+v, want %+v", got, tc.expected)
			}
		})
	}
}

func TestBuildImportMap(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected map[string]string
	}{
		{
			name: "single import",
			src:  `package p; import "fmt"`,
			expected: map[string]string{
				"fmt": "fmt",
			},
		},
		{
			name: "grouped imports",
			src: `package p
import (
	"fmt"
	"net/http"
	"os"
)`,
			expected: map[string]string{
				"fmt":  "fmt",
				"http": "net/http",
				"os":   "os",
			},
		},
		{
			name: "aliased import",
			src: `package p
import (
	myhttp "net/http"
)`,
			expected: map[string]string{
				"myhttp": "net/http",
			},
		},
		{
			name: "dot import skipped",
			src: `package p
import (
	. "testing"
	"fmt"
)`,
			expected: map[string]string{
				"fmt": "fmt",
			},
		},
		{
			name: "blank import skipped",
			src: `package p
import (
	_ "net/http/pprof"
	"fmt"
)`,
			expected: map[string]string{
				"fmt": "fmt",
			},
		},
		{
			name:     "no imports",
			src:      `package p`,
			expected: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte(tc.src)
			root := parseGoSource(t, tc.src)
			got := BuildImportMap(root, content)

			if len(got) != len(tc.expected) {
				t.Fatalf("len(got)=%d, len(expected)=%d; got=%v", len(got), len(tc.expected), got)
			}
			for k, v := range tc.expected {
				if got[k] != v {
					t.Errorf("imports[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestResolveImport(t *testing.T) {
	imports := map[string]string{
		"fmt":  "fmt",
		"http": "net/http",
	}

	t.Run("found", func(t *testing.T) {
		pkg, ok := ResolveImport(imports, "http")
		if !ok {
			t.Fatal("expected found")
		}
		if pkg != "net/http" {
			t.Errorf("got %q, want %q", pkg, "net/http")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := ResolveImport(imports, "os")
		if ok {
			t.Fatal("expected not found")
		}
	})
}
