package rubyresolve

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseRuby is defined in eval_test.go (same package).

func TestResolveRequire_Standard(t *testing.T) {
	resolved, ok := ResolveRequire("json", "", false)
	assert.True(t, ok)
	assert.Equal(t, "json", resolved)
}

func TestResolveRequire_Nested(t *testing.T) {
	resolved, ok := ResolveRequire("active_record/base", "", false)
	assert.True(t, ok)
	assert.Equal(t, "active_record/base", resolved)
}

func TestResolveRequire_Relative(t *testing.T) {
	resolved, ok := ResolveRequire("../lib/helper", "app/models/user.rb", true)
	assert.True(t, ok)
	assert.Equal(t, "app/lib/helper", resolved)
}

func TestResolveRequire_RelativeWithExtension(t *testing.T) {
	resolved, ok := ResolveRequire("./foo.rb", "app/models/user.rb", true)
	assert.True(t, ok)
	assert.Equal(t, "app/models/foo", resolved)
}

func TestResolveRequire_Empty(t *testing.T) {
	_, ok := ResolveRequire("", "", false)
	assert.False(t, ok)
}

func TestBuildRequireMap(t *testing.T) {
	src := `require "json"
require_relative "../helper"
`
	content := []byte(src)
	root := parseRuby(t, src)

	result := BuildRequireMap(root, content, "app/models/user.rb")

	// require "json" -> map["json"] = "json"
	assert.Equal(t, "json", result["json"])

	// require_relative "../helper" -> resolved to "app/helper"
	assert.Equal(t, "app/helper", result["helper"])
	assert.Equal(t, "app/helper", result["app/helper"])
}

func TestBuildRequireMap_NestedRequire(t *testing.T) {
	src := `require "active_record/base"
`
	content := []byte(src)
	root := parseRuby(t, src)

	result := BuildRequireMap(root, content, "app/models/user.rb")

	assert.Equal(t, "active_record/base", result["base"])
	assert.Equal(t, "active_record/base", result["active_record/base"])
}

func TestParseScopeResolution_Simple(t *testing.T) {
	src := `A::B`
	content := []byte(src)
	root := parseRuby(t, src)

	// The root should have a scope_resolution child.
	node := findFirstNodeOfType(root, "scope_resolution")
	require.NotNil(t, node, "expected scope_resolution node in AST")

	result := ParseScopeResolution(node, content)
	assert.Equal(t, "A::B", result)
}

func TestParseScopeResolution_Nested(t *testing.T) {
	src := `A::B::C`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findFirstNodeOfType(root, "scope_resolution")
	require.NotNil(t, node, "expected scope_resolution node in AST")

	result := ParseScopeResolution(node, content)
	assert.Equal(t, "A::B::C", result)
}

func TestParseScopeResolution_TopLevel(t *testing.T) {
	src := `::TopLevel`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findFirstNodeOfType(root, "scope_resolution")
	require.NotNil(t, node, "expected scope_resolution node in AST")

	result := ParseScopeResolution(node, content)
	assert.Equal(t, "::TopLevel", result)
}

func TestParseScopeResolution_Nil(t *testing.T) {
	result := ParseScopeResolution(nil, nil)
	assert.Equal(t, "", result)
}

func TestResolveConstant_WithNesting(t *testing.T) {
	result := ResolveConstant("Base", []string{"MyApp", "Models"})
	assert.Equal(t, "MyApp::Models::Base", result)
}

func TestResolveConstant_Absolute(t *testing.T) {
	result := ResolveConstant("::ActiveRecord::Base", []string{"MyApp", "Models"})
	assert.Equal(t, "ActiveRecord::Base", result)
}

func TestResolveConstant_TopLevel(t *testing.T) {
	result := ResolveConstant("String", nil)
	assert.Equal(t, "String", result)
}

func TestResolveConstant_SingleNesting(t *testing.T) {
	result := ResolveConstant("Helper", []string{"MyApp"})
	assert.Equal(t, "MyApp::Helper", result)
}

// findFirstNodeOfType performs a depth-first search for the first node
// matching the given type string.
func findFirstNodeOfType(node *sitter.Node, nodeType string) *sitter.Node {
	if node == nil {
		return nil
	}
	if node.Type() == nodeType {
		return node
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		found := findFirstNodeOfType(node.Child(i), nodeType)
		if found != nil {
			return found
		}
	}
	return nil
}
