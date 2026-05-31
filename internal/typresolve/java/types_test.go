package javaresolve

import (
	"context"
	"fmt"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseJavaType parses a Java type string by wrapping it in a field declaration
// and extracting the type node from the AST.
func parseJavaType(t *testing.T, typeStr string) (*sitter.Node, []byte) {
	t.Helper()
	src := fmt.Sprintf("class X { %s x; }", typeStr)
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	require.NoError(t, err)
	t.Cleanup(func() { tree.Close() })

	root := tree.RootNode()
	classDecl := root.NamedChild(0)
	require.NotNil(t, classDecl, "expected class_declaration")
	body := classDecl.ChildByFieldName("body")
	require.NotNil(t, body, "expected class body")

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() == "field_declaration" {
			typeNode := child.ChildByFieldName("type")
			require.NotNil(t, typeNode, "expected type node in field_declaration")
			return typeNode, content
		}
	}
	t.Fatal("no field_declaration found")
	return nil, nil
}

// parseJavaSource parses a full Java source string and returns the root node.
func parseJavaSource(t *testing.T, src string) (*sitter.Node, []byte) {
	t.Helper()
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	require.NoError(t, err)
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode(), content
}

func TestParseTypeNode_PrimitiveInt(t *testing.T) {
	node, content := parseJavaType(t, "int")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "int", result.Name)
}

func TestParseTypeNode_PrimitiveBoolean(t *testing.T) {
	node, content := parseJavaType(t, "boolean")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "boolean", result.Name)
}

func TestParseTypeNode_PrimitiveLong(t *testing.T) {
	node, content := parseJavaType(t, "long")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "long", result.Name)
}

func TestParseTypeNode_PrimitiveDouble(t *testing.T) {
	node, content := parseJavaType(t, "double")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "double", result.Name)
}

func TestParseTypeNode_BuiltinString(t *testing.T) {
	node, content := parseJavaType(t, "String")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "String", result.Name)
}

func TestParseTypeNode_UserClass(t *testing.T) {
	node, content := parseJavaType(t, "MyClass")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindNamed, result.Kind)
	assert.Equal(t, "com.example.MyClass", result.Name)
}

func TestParseTypeNode_ImportedClass(t *testing.T) {
	node, content := parseJavaType(t, "UserService")
	imports := map[string]string{"UserService": "com.example.service"}
	result := ParseTypeNode(node, content, "com.example", imports)
	assert.Equal(t, typresolve.KindNamed, result.Kind)
	assert.Equal(t, "com.example.service.UserService", result.Name)
}

func TestParseTypeNode_GenericList(t *testing.T) {
	node, content := parseJavaType(t, "List<String>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindSlice, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, typresolve.KindBuiltin, result.Elem.Kind)
	assert.Equal(t, "String", result.Elem.Name)
}

func TestParseTypeNode_GenericMap(t *testing.T) {
	node, content := parseJavaType(t, "Map<String, Integer>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindMap, result.Kind)
	require.NotNil(t, result.Key)
	require.NotNil(t, result.Value)
	assert.Equal(t, "String", result.Key.Name)
	assert.Equal(t, "Integer", result.Value.Name)
}

func TestParseTypeNode_GenericOptional(t *testing.T) {
	node, content := parseJavaType(t, "Optional<String>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindOptional, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, "String", result.Elem.Name)
}

func TestParseTypeNode_Array(t *testing.T) {
	node, content := parseJavaType(t, "String[]")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindSlice, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, typresolve.KindBuiltin, result.Elem.Kind)
	assert.Equal(t, "String", result.Elem.Name)
}

func TestParseTypeNode_NestedGeneric(t *testing.T) {
	node, content := parseJavaType(t, "List<Map<String, Integer>>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindSlice, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, typresolve.KindMap, result.Elem.Kind)
	require.NotNil(t, result.Elem.Key)
	require.NotNil(t, result.Elem.Value)
	assert.Equal(t, "String", result.Elem.Key.Name)
	assert.Equal(t, "Integer", result.Elem.Value.Name)
}

func TestParseTypeNode_Nil(t *testing.T) {
	result := ParseTypeNode(nil, nil, "com.example", nil)
	assert.Equal(t, typresolve.KindUnknown, result.Kind)
}

func TestParseTypeNode_HashSet(t *testing.T) {
	node, content := parseJavaType(t, "HashSet<String>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindSlice, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, "String", result.Elem.Name)
}

func TestParseTypeNode_HashMap(t *testing.T) {
	node, content := parseJavaType(t, "HashMap<String, Integer>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindMap, result.Kind)
}

func TestParseTypeNode_CustomGeneric(t *testing.T) {
	node, content := parseJavaType(t, "Future<String>")
	result := ParseTypeNode(node, content, "com.example", nil)
	assert.Equal(t, typresolve.KindNamed, result.Kind)
	assert.Equal(t, "com.example.Future", result.Name)
	assert.Len(t, result.TypeParams, 1)
}

func TestBuildImportMap_RegularImport(t *testing.T) {
	src := `
import java.util.List;
class X {}
`
	root, content := parseJavaSource(t, src)
	imports := BuildImportMap(root, content)
	assert.Equal(t, "java.util", imports["List"])
}

func TestBuildImportMap_StaticImport(t *testing.T) {
	src := `
import static org.junit.Assert.assertEquals;
class X {}
`
	root, content := parseJavaSource(t, src)
	imports := BuildImportMap(root, content)
	assert.Equal(t, "org.junit", imports["Assert"])
}

func TestBuildImportMap_WildcardSkipped(t *testing.T) {
	src := `
import java.util.*;
class X {}
`
	root, content := parseJavaSource(t, src)
	imports := BuildImportMap(root, content)
	assert.Empty(t, imports)
}

func TestBuildImportMap_Multiple(t *testing.T) {
	src := `
import java.util.List;
import java.util.Map;
import java.io.*;
import static org.junit.Assert.assertEquals;
class X {}
`
	root, content := parseJavaSource(t, src)
	imports := BuildImportMap(root, content)
	assert.Equal(t, "java.util", imports["List"])
	assert.Equal(t, "java.util", imports["Map"])
	assert.Equal(t, "org.junit", imports["Assert"])
	// Wildcard should be skipped.
	_, found := imports["*"]
	assert.False(t, found)
	assert.Len(t, imports, 3)
}

func TestExtractPackage(t *testing.T) {
	src := `
package com.example.service;
class X {}
`
	root, content := parseJavaSource(t, src)
	pkg := ExtractPackage(root, content)
	assert.Equal(t, "com.example.service", pkg)
}

func TestExtractPackage_NoPackage(t *testing.T) {
	src := `class X {}`
	root, content := parseJavaSource(t, src)
	pkg := ExtractPackage(root, content)
	assert.Equal(t, "", pkg)
}

func TestResolveImport(t *testing.T) {
	imports := map[string]string{
		"List":        "java.util",
		"UserService": "com.example.service",
	}

	pkg, ok := ResolveImport(imports, "List")
	assert.True(t, ok)
	assert.Equal(t, "java.util", pkg)

	pkg, ok = ResolveImport(imports, "UserService")
	assert.True(t, ok)
	assert.Equal(t, "com.example.service", pkg)

	_, ok = ResolveImport(imports, "NotImported")
	assert.False(t, ok)
}

func TestIsBuiltinType(t *testing.T) {
	builtins := []string{
		"int", "long", "short", "byte", "char", "boolean", "float", "double", "void",
		"String", "Object", "Integer", "Long", "Boolean", "Double", "Float",
		"Class", "Void", "Byte", "Short", "Character",
	}
	for _, b := range builtins {
		assert.True(t, IsBuiltinType(b), "expected %s to be builtin", b)
	}
	assert.False(t, IsBuiltinType("MyClass"))
	assert.False(t, IsBuiltinType("List"))
}

func TestResolveBuiltinType(t *testing.T) {
	result := ResolveBuiltinType("int")
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "int", result.Name)

	result = ResolveBuiltinType("MyClass")
	assert.Nil(t, result)
}

func TestExtractBaseTypeName(t *testing.T) {
	// Simple type
	node, content := parseJavaType(t, "String")
	assert.Equal(t, "String", extractBaseTypeName(node, content))

	// Generic type
	node, content = parseJavaType(t, "List<String>")
	assert.Equal(t, "List", extractBaseTypeName(node, content))

	// Array type
	node, content = parseJavaType(t, "String[]")
	assert.Equal(t, "String", extractBaseTypeName(node, content))

	// Nil
	assert.Equal(t, "", extractBaseTypeName(nil, nil))
}

func TestQualifyTypeName(t *testing.T) {
	imports := map[string]string{"List": "java.util"}

	// Imported name
	assert.Equal(t, "java.util.List", qualifyTypeName("List", "com.example", imports))

	// Non-imported name with package
	assert.Equal(t, "com.example.MyClass", qualifyTypeName("MyClass", "com.example", nil))

	// Non-imported name without package
	assert.Equal(t, "MyClass", qualifyTypeName("MyClass", "", nil))
}
