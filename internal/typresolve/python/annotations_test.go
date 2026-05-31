package pyresolve

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	python "github.com/smacker/go-tree-sitter/python"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestParseAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		ann      string
		moduleQN string
		want     *typresolve.Type
	}{
		{
			name: "builtin int",
			ann:  "int",
			want: typresolve.Builtin("int"),
		},
		{
			name: "builtin str",
			ann:  "str",
			want: typresolve.Builtin("str"),
		},
		{
			name: "builtin None",
			ann:  "None",
			want: typresolve.Builtin("None"),
		},
		{
			name:     "quoted forward ref",
			ann:      `"MyClass"`,
			moduleQN: "mymodule",
			want:     typresolve.Named("mymodule.MyClass"),
		},
		{
			name: "generic list",
			ann:  "list[int]",
			want: typresolve.Slice(typresolve.Builtin("int")),
		},
		{
			name: "generic dict",
			ann:  "dict[str, int]",
			want: typresolve.Map(typresolve.Builtin("str"), typresolve.Builtin("int")),
		},
		{
			name: "tuple",
			ann:  "tuple[int, str]",
			want: typresolve.Tuple([]*typresolve.Type{
				typresolve.Builtin("int"),
				typresolve.Builtin("str"),
			}),
		},
		{
			name: "Optional",
			ann:  "Optional[str]",
			want: typresolve.Optional(typresolve.Builtin("str")),
		},
		{
			name: "pipe union first non-None",
			ann:  "int | str",
			want: typresolve.Builtin("int"),
		},
		{
			name: "pipe optional",
			ann:  "str | None",
			want: typresolve.Optional(typresolve.Builtin("str")),
		},
		{
			name: "Callable",
			ann:  "Callable[[int, str], bool]",
			want: typresolve.Func(
				[]typresolve.Param{
					{Type: typresolve.Builtin("int")},
					{Type: typresolve.Builtin("str")},
				},
				[]*typresolve.Type{typresolve.Builtin("bool")},
			),
		},
		{
			name: "ClassVar wrapper",
			ann:  "ClassVar[int]",
			want: typresolve.Builtin("int"),
		},
		{
			name:     "bare name qualified",
			ann:      "MyClass",
			moduleQN: "mymod",
			want:     typresolve.Named("mymod.MyClass"),
		},
		{
			name: "typing name not qualified",
			ann:  "Iterator",
			want: typresolve.Named("Iterator"),
		},
		{
			name: "nested generic",
			ann:  "dict[str, list[int]]",
			want: typresolve.Map(
				typresolve.Builtin("str"),
				typresolve.Slice(typresolve.Builtin("int")),
			),
		},
		{
			name: "typing.Optional",
			ann:  "typing.Optional[str]",
			want: typresolve.Optional(typresolve.Builtin("str")),
		},
		{
			name: "Final wrapper",
			ann:  "Final[int]",
			want: typresolve.Builtin("int"),
		},
		{
			name: "Annotated extracts first arg",
			ann:  "Annotated[int, Field(default=0)]",
			want: typresolve.Builtin("int"),
		},
		{
			name: "Type[T] returns T",
			ann:  "Type[MyClass]",
			want: typresolve.Named("MyClass"),
		},
		{
			name: "Union first member",
			ann:  "Union[int, str]",
			want: typresolve.Builtin("int"),
		},
		{
			name:     "single quoted forward ref",
			ann:      "'Foo'",
			moduleQN: "mymodule",
			want:     typresolve.Named("mymodule.Foo"),
		},
		{
			name: "None | str is Optional",
			ann:  "None | str",
			want: typresolve.Optional(typresolve.Builtin("str")),
		},
		{
			name: "empty string",
			ann:  "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAnnotation(tt.ann, tt.moduleQN)
			if !typresolve.TypesEqual(got, tt.want) {
				t.Errorf("ParseAnnotation(%q, %q)\n  got  %+v\n  want %+v", tt.ann, tt.moduleQN, got, tt.want)
			}
		})
	}
}

func TestSplitSubscriptArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"int, str", []string{"int", "str"}},
		{"str, list[int]", []string{"str", "list[int]"}},
		{"int, dict[str, list[int]]", []string{"int", "dict[str, list[int]]"}},
		{"", nil},
		{"int", []string{"int"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitSubscriptArgs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitSubscriptArgs(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitSubscriptArgs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func parsePython(t *testing.T, code string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(nil, nil, []byte(code))
	if err != nil {
		t.Fatalf("failed to parse Python: %v", err)
	}
	return tree.RootNode()
}

func TestBuildImportMap(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		checks map[string]ImportInfo
	}{
		{
			name: "import os",
			code: "import os\n",
			checks: map[string]ImportInfo{
				"os": {ModulePath: "os", IsFromStyle: false},
			},
		},
		{
			name: "import os.path binds both",
			code: "import os.path\n",
			checks: map[string]ImportInfo{
				"os":      {ModulePath: "os", IsFromStyle: false},
				"os.path": {ModulePath: "os.path", IsFromStyle: false},
			},
		},
		{
			name: "from flask import Flask",
			code: "from flask import Flask\n",
			checks: map[string]ImportInfo{
				"Flask": {ModulePath: "flask.Flask", IsFromStyle: true},
			},
		},
		{
			name: "from flask import Flask as F",
			code: "from flask import Flask as F\n",
			checks: map[string]ImportInfo{
				"F": {ModulePath: "flask.Flask", IsFromStyle: true},
			},
		},
		{
			name: "import numpy as np",
			code: "import numpy as np\n",
			checks: map[string]ImportInfo{
				"np": {ModulePath: "numpy", IsFromStyle: false},
			},
		},
		{
			name: "from X import multiple",
			code: "from django.db import models, connection\n",
			checks: map[string]ImportInfo{
				"models":     {ModulePath: "django.db.models", IsFromStyle: true},
				"connection": {ModulePath: "django.db.connection", IsFromStyle: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.code)
			root := parsePython(t, tt.code)
			imports := BuildImportMap(root, content)

			for name, wantInfo := range tt.checks {
				got, ok := imports[name]
				if !ok {
					t.Errorf("import %q not found in map; got keys: %v", name, importKeys(imports))
					continue
				}
				if got.ModulePath != wantInfo.ModulePath {
					t.Errorf("import %q: ModulePath = %q, want %q", name, got.ModulePath, wantInfo.ModulePath)
				}
				if got.IsFromStyle != wantInfo.IsFromStyle {
					t.Errorf("import %q: IsFromStyle = %v, want %v", name, got.IsFromStyle, wantInfo.IsFromStyle)
				}
			}
		})
	}
}

func TestResolveImport(t *testing.T) {
	imports := map[string]ImportInfo{
		"Flask": {ModulePath: "flask.Flask", IsFromStyle: true},
		"os":    {ModulePath: "os", IsFromStyle: false},
	}

	info, ok := ResolveImport(imports, "Flask")
	if !ok {
		t.Fatal("expected Flask to be found")
	}
	if info.ModulePath != "flask.Flask" {
		t.Errorf("Flask ModulePath = %q, want %q", info.ModulePath, "flask.Flask")
	}

	_, ok = ResolveImport(imports, "nonexistent")
	if ok {
		t.Error("expected nonexistent to not be found")
	}
}

func importKeys(m map[string]ImportInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
