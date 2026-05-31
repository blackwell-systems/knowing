package tsresolve

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	typescript "github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestParseTypeText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		moduleQN string
		want     *typresolve.Type
	}{
		// Builtin primitives
		{
			name: "builtin string",
			text: "string",
			want: typresolve.Builtin("string"),
		},
		{
			name: "builtin number",
			text: "number",
			want: typresolve.Builtin("number"),
		},
		{
			name: "builtin boolean",
			text: "boolean",
			want: typresolve.Builtin("boolean"),
		},
		{
			name: "builtin void",
			text: "void",
			want: typresolve.Builtin("void"),
		},
		{
			name: "builtin any",
			text: "any",
			want: typresolve.Builtin("any"),
		},
		{
			name: "builtin never",
			text: "never",
			want: typresolve.Builtin("never"),
		},

		// Leading colon stripping
		{
			name: "leading colon",
			text: ": string",
			want: typresolve.Builtin("string"),
		},

		// Array shorthand
		{
			name: "array shorthand",
			text: "string[]",
			want: typresolve.Slice(typresolve.Builtin("string")),
		},

		// Generic Array
		{
			name: "generic Array",
			text: "Array<number>",
			want: typresolve.Slice(typresolve.Builtin("number")),
		},

		// Generic Map
		{
			name: "generic Map",
			text: "Map<string, number>",
			want: typresolve.Map(typresolve.Builtin("string"), typresolve.Builtin("number")),
		},

		// Promise
		{
			name: "Promise generic",
			text: "Promise<string>",
			want: &typresolve.Type{
				Kind: typresolve.KindNamed,
				Name: "Promise",
				TypeParams: []typresolve.TypeParam{
					{Name: "string", Constraint: typresolve.Builtin("string")},
				},
			},
		},

		// Union
		{
			name: "union first member",
			text: "string | number",
			want: typresolve.Builtin("string"),
		},

		// Nullable union
		{
			name: "nullable union string|null",
			text: "string | null",
			want: typresolve.Optional(typresolve.Builtin("string")),
		},
		{
			name: "nullable union null|string",
			text: "null | string",
			want: typresolve.Optional(typresolve.Builtin("string")),
		},

		// Tuple
		{
			name: "tuple",
			text: "[string, number]",
			want: typresolve.Tuple([]*typresolve.Type{
				typresolve.Builtin("string"),
				typresolve.Builtin("number"),
			}),
		},

		// Function type
		{
			name: "function type",
			text: "(x: string) => number",
			want: typresolve.Func(
				[]typresolve.Param{{Name: "x", Type: typresolve.Builtin("string")}},
				[]*typresolve.Type{typresolve.Builtin("number")},
			),
		},

		// Bare name with module qualification
		{
			name:     "bare name qualified",
			text:     "MyClass",
			moduleQN: "src/app",
			want:     typresolve.Named("src/app.MyClass"),
		},

		// Stdlib name stays bare
		{
			name:     "stdlib name bare",
			text:     "Promise",
			moduleQN: "src/app",
			want:     typresolve.Named("Promise"),
		},

		// Qualified name
		{
			name: "qualified name",
			text: "express.Request",
			want: typresolve.Named("express.Request"),
		},

		// Nested generic
		{
			name: "nested generic",
			text: "Array<Map<string, number>>",
			want: typresolve.Slice(
				typresolve.Map(typresolve.Builtin("string"), typresolve.Builtin("number")),
			),
		},

		// Record utility type
		{
			name: "Record utility",
			text: "Record<string, number>",
			want: typresolve.Map(typresolve.Builtin("string"), typresolve.Builtin("number")),
		},

		// Partial unwrap
		{
			name:     "Partial unwrap",
			text:     "Partial<MyType>",
			moduleQN: "src/app",
			want:     typresolve.Named("src/app.MyType"),
		},

		// Object literal
		{
			name: "object literal",
			text: "{ name: string }",
			want: typresolve.Unknown(),
		},

		// Intersection
		{
			name: "intersection first member",
			text: "Foo & Bar",
			want: typresolve.Named("Foo"),
		},

		// Trailing semicolon
		{
			name: "trailing semicolon",
			text: "string;",
			want: typresolve.Builtin("string"),
		},

		// Trailing comma
		{
			name: "trailing comma",
			text: "string,",
			want: typresolve.Builtin("string"),
		},

		// Function type void return
		{
			name: "function void return",
			text: "(x: number) => void",
			want: typresolve.Func(
				[]typresolve.Param{{Name: "x", Type: typresolve.Builtin("number")}},
				nil,
			),
		},

		// Set generic
		{
			name: "Set generic",
			text: "Set<string>",
			want: &typresolve.Type{
				Kind: typresolve.KindNamed,
				Name: "Set",
				TypeParams: []typresolve.TypeParam{
					{Name: "string", Constraint: typresolve.Builtin("string")},
				},
			},
		},

		// ReturnType utility
		{
			name: "ReturnType utility",
			text: "ReturnType<typeof fn>",
			want: typresolve.Unknown(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTypeText(tt.text, tt.moduleQN)
			if !typresolve.TypesEqual(got, tt.want) {
				t.Errorf("ParseTypeText(%q, %q):\ngot  %+v\nwant %+v",
					tt.text, tt.moduleQN, got, tt.want)
			}
		})
	}
}

func TestParseTypeText_EmptyInput(t *testing.T) {
	got := ParseTypeText("", "")
	if got == nil || got.Kind != typresolve.KindUnknown {
		t.Errorf("expected Unknown for empty input, got %+v", got)
	}
}

func TestIsBuiltinType(t *testing.T) {
	// Primitives
	for _, name := range []string{"string", "number", "boolean", "void", "any", "never", "null", "undefined"} {
		if !IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = false, want true", name)
		}
	}
	// Stdlib names
	for _, name := range []string{"Array", "Promise", "Map", "Set", "Error", "Date"} {
		if !IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = false, want true", name)
		}
	}
	// Non-builtin
	if IsBuiltinType("MyClass") {
		t.Error("IsBuiltinType(MyClass) = true, want false")
	}
}

func TestResolveBuiltinType(t *testing.T) {
	got := ResolveBuiltinType("string")
	if got == nil || got.Kind != typresolve.KindBuiltin || got.Name != "string" {
		t.Errorf("ResolveBuiltinType(string) = %+v, want Builtin(string)", got)
	}
	if ResolveBuiltinType("MyClass") != nil {
		t.Error("ResolveBuiltinType(MyClass) should return nil")
	}
}

func TestBuiltinWrapperClass(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"string", "String"},
		{"number", "Number"},
		{"boolean", "Boolean"},
		{"bigint", "BigInt"},
		{"symbol", "Symbol"},
		{"object", ""},
		{"void", ""},
	}
	for _, tt := range tests {
		got := BuiltinWrapperClass(tt.input)
		if got != tt.want {
			t.Errorf("BuiltinWrapperClass(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLiteralType(t *testing.T) {
	tests := []struct {
		nodeType string
		wantKind typresolve.TypeKind
		wantName string
	}{
		{"string", typresolve.KindBuiltin, "string"},
		{"template_string", typresolve.KindBuiltin, "string"},
		{"number", typresolve.KindBuiltin, "number"},
		{"true", typresolve.KindBuiltin, "boolean"},
		{"false", typresolve.KindBuiltin, "boolean"},
		{"null", typresolve.KindBuiltin, "null"},
		{"undefined", typresolve.KindBuiltin, "undefined"},
		{"regex", typresolve.KindNamed, "RegExp"},
	}
	for _, tt := range tests {
		got := LiteralType(tt.nodeType)
		if got == nil {
			t.Errorf("LiteralType(%q) = nil, want non-nil", tt.nodeType)
			continue
		}
		if got.Kind != tt.wantKind || got.Name != tt.wantName {
			t.Errorf("LiteralType(%q) = {Kind:%v, Name:%q}, want {Kind:%v, Name:%q}",
				tt.nodeType, got.Kind, got.Name, tt.wantKind, tt.wantName)
		}
	}
	if LiteralType("identifier") != nil {
		t.Error("LiteralType(identifier) should return nil")
	}
}

func TestUnwrapPromise(t *testing.T) {
	// Promise with type param unwraps.
	promise := typresolve.Named("Promise")
	promise.TypeParams = []typresolve.TypeParam{
		{Name: "string", Constraint: typresolve.Builtin("string")},
	}
	got := UnwrapPromise(promise)
	if got.Kind != typresolve.KindBuiltin || got.Name != "string" {
		t.Errorf("UnwrapPromise(Promise<string>) = %+v, want Builtin(string)", got)
	}

	// Non-promise passes through.
	num := typresolve.Builtin("number")
	if UnwrapPromise(num) != num {
		t.Error("UnwrapPromise should pass through non-Promise types")
	}

	// Nil passes through.
	if UnwrapPromise(nil) != nil {
		t.Error("UnwrapPromise(nil) should return nil")
	}
}

func TestSplitAtDepthZero(t *testing.T) {
	tests := []struct {
		text string
		sep  byte
		want []string
	}{
		{"a, b, c", ',', []string{"a", "b", "c"}},
		{"Map<string, number>, boolean", ',', []string{"Map<string, number>", "boolean"}},
		{"A | B | C", '|', []string{"A", "B", "C"}},
		{"A & B", '&', []string{"A", "B"}},
		{"single", ',', []string{"single"}},
	}
	for _, tt := range tests {
		got := splitAtDepthZero(tt.text, tt.sep)
		if len(got) != len(tt.want) {
			t.Errorf("splitAtDepthZero(%q, %q): got %v, want %v", tt.text, string(tt.sep), got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitAtDepthZero(%q, %q)[%d] = %q, want %q",
					tt.text, string(tt.sep), i, got[i], tt.want[i])
			}
		}
	}
}

func parseTS(t *testing.T, code string) (*sitter.Node, []byte) {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(code))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return tree.RootNode(), []byte(code)
}

func TestParseTypeNode_TypeAnnotation(t *testing.T) {
	code := `const x: string = "hello";`
	root, content := parseTS(t, code)

	// Navigate to the type_annotation node.
	// lexical_declaration -> variable_declarator -> type_annotation
	decl := root.NamedChild(0)
	if decl == nil {
		t.Fatal("expected lexical_declaration")
	}
	declarator := decl.NamedChild(0)
	if declarator == nil {
		t.Fatal("expected variable_declarator")
	}
	typeAnnotation := declarator.ChildByFieldName("type")
	if typeAnnotation == nil {
		t.Fatal("expected type_annotation")
	}

	got := ParseTypeNode(typeAnnotation, content, "", nil)
	if got == nil || got.Kind != typresolve.KindBuiltin || got.Name != "string" {
		t.Errorf("ParseTypeNode for 'string' annotation: got %+v, want Builtin(string)", got)
	}
}

func TestBuildImportMap(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantLen int
		checks  map[string]ImportInfo
	}{
		{
			name:    "named import",
			code:    `import { readFile } from 'fs';`,
			wantLen: 1,
			checks: map[string]ImportInfo{
				"readFile": {ModulePath: "fs", OriginalName: "readFile"},
			},
		},
		{
			name:    "namespace import",
			code:    `import * as path from 'path';`,
			wantLen: 1,
			checks: map[string]ImportInfo{
				"path": {ModulePath: "path", OriginalName: "path", IsNamespace: true},
			},
		},
		{
			name:    "default import",
			code:    `import express from 'express';`,
			wantLen: 1,
			checks: map[string]ImportInfo{
				"express": {ModulePath: "express", OriginalName: "express", IsDefault: true},
			},
		},
		{
			name:    "aliased import",
			code:    `import { Component as Comp } from 'react';`,
			wantLen: 1,
			checks: map[string]ImportInfo{
				"Comp": {ModulePath: "react", OriginalName: "Component"},
			},
		},
		{
			name:    "multiple named imports",
			code:    `import { useState, useEffect } from 'react';`,
			wantLen: 2,
			checks: map[string]ImportInfo{
				"useState":  {ModulePath: "react", OriginalName: "useState"},
				"useEffect": {ModulePath: "react", OriginalName: "useEffect"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, content := parseTS(t, tt.code)
			got := BuildImportMap(root, content)
			if len(got) != tt.wantLen {
				t.Errorf("BuildImportMap: got %d entries, want %d: %+v", len(got), tt.wantLen, got)
			}
			for name, want := range tt.checks {
				info, ok := got[name]
				if !ok {
					t.Errorf("missing import %q in map: %+v", name, got)
					continue
				}
				if info.ModulePath != want.ModulePath {
					t.Errorf("import %q: ModulePath = %q, want %q", name, info.ModulePath, want.ModulePath)
				}
				if info.OriginalName != want.OriginalName {
					t.Errorf("import %q: OriginalName = %q, want %q", name, info.OriginalName, want.OriginalName)
				}
				if info.IsNamespace != want.IsNamespace {
					t.Errorf("import %q: IsNamespace = %v, want %v", name, info.IsNamespace, want.IsNamespace)
				}
				if info.IsDefault != want.IsDefault {
					t.Errorf("import %q: IsDefault = %v, want %v", name, info.IsDefault, want.IsDefault)
				}
			}
		})
	}
}

func TestResolveImport(t *testing.T) {
	imports := map[string]ImportInfo{
		"readFile": {ModulePath: "fs", OriginalName: "readFile"},
	}

	info, ok := ResolveImport(imports, "readFile")
	if !ok {
		t.Fatal("expected to find readFile")
	}
	if info.ModulePath != "fs" {
		t.Errorf("ModulePath = %q, want 'fs'", info.ModulePath)
	}

	_, ok = ResolveImport(imports, "nonexistent")
	if ok {
		t.Error("expected not to find nonexistent")
	}
}

func TestResolveModulePath(t *testing.T) {
	tests := []struct {
		importSource string
		currentFile  string
		want         string
	}{
		{"./utils", "src/app.ts", "src/utils"},
		{"../shared/types", "src/components/button.tsx", "src/shared/types"},
		{"express", "src/app.ts", ""},
		{"./helper.ts", "src/app.ts", "src/helper"},
		{"./component.tsx", "src/views/main.tsx", "src/views/component"},
	}

	for _, tt := range tests {
		got := ResolveModulePath(tt.importSource, tt.currentFile)
		if got != tt.want {
			t.Errorf("ResolveModulePath(%q, %q) = %q, want %q",
				tt.importSource, tt.currentFile, got, tt.want)
		}
	}
}
