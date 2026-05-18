package rubyextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// makeOpts creates ExtractOptions for testing with the given Ruby source.
func makeOpts(source string, filePath string) types.ExtractOptions {
	fileHash := types.NewHash([]byte(source))
	repoHash := types.NewHash([]byte("test://repo"))
	return types.ExtractOptions{
		RepoURL:    "test://repo",
		RepoHash:   repoHash,
		CommitHash: "abc123",
		FilePath:   filePath,
		FileHash:   fileHash,
		Content:    []byte(source),
		ModuleRoot: "/tmp/testruby",
	}
}

func TestRubyExtractor_Name(t *testing.T) {
	ext := NewRubyExtractor()
	if got := ext.Name(); got != "treesitter-ruby" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-ruby")
	}
}

func TestRubyExtractor_CanHandle(t *testing.T) {
	ext := NewRubyExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"app/models/user.rb", true},
		{"lib/tasks/deploy.rb", true},
		{"config/routes.rb", true},
		{"main.go", false},
		{"script.py", false},
		{"vendor/bundle/gems/foo.rb", false},
		{"some/vendor/cache/bar.rb", false},
		{"README.md", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ext.CanHandle(tt.path)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestRubyExtractor_BasicClassWithMethods(t *testing.T) {
	ext := NewRubyExtractor()
	source := `class User
  def initialize(name)
    @name = name
  end

  def greet
    puts "Hello, #{@name}"
  end

  def self.find_by_name(name)
    # lookup logic
  end
end
`
	opts := makeOpts(source, "app/models/user.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have 1 class (type) node.
	typeNodes := filterNodes(result.Nodes, "type")
	if len(typeNodes) != 1 {
		t.Fatalf("expected 1 type node, got %d", len(typeNodes))
	}
	if !containsName(typeNodes, "User") {
		t.Error("expected type node for User")
	}

	// Should have method nodes for initialize and greet.
	methodNodes := filterNodes(result.Nodes, "method")
	if len(methodNodes) < 3 {
		t.Fatalf("expected at least 3 method nodes (initialize, greet, find_by_name), got %d", len(methodNodes))
	}

	names := nodeNames(methodNodes)
	assertContains(t, names, "initialize")
	assertContains(t, names, "greet")
	assertContains(t, names, "find_by_name")

	// Verify singleton method has correct signature.
	for _, n := range methodNodes {
		if containsStr(n.QualifiedName, "find_by_name") {
			if n.Kind != "method" {
				t.Errorf("singleton method kind = %q, want 'method'", n.Kind)
			}
		}
	}
}

func TestRubyExtractor_InheritanceAndIncludes(t *testing.T) {
	ext := NewRubyExtractor()
	source := `class Admin < User
  include Authenticatable
  include Serializable
  extend ClassMethods

  def admin_action
    perform_task
  end
end
`
	opts := makeOpts(source, "app/models/admin.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have 1 class node.
	typeNodes := filterNodes(result.Nodes, "type")
	if len(typeNodes) != 1 {
		t.Fatalf("expected 1 type node, got %d", len(typeNodes))
	}

	// Should have an "extends" edge from Admin to User.
	extendsEdges := filterEdges(result.Edges, "extends")
	if len(extendsEdges) != 1 {
		t.Fatalf("expected 1 extends edge, got %d", len(extendsEdges))
	}

	// Should have "implements" edges for include/extend calls.
	implEdges := filterEdges(result.Edges, "implements")
	if len(implEdges) < 2 {
		t.Fatalf("expected at least 2 implements edges (Authenticatable, Serializable), got %d", len(implEdges))
	}

	// Verify provenance and confidence on all edges.
	for _, e := range result.Edges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("edge confidence = %f, want 0.7", e.Confidence)
		}
	}

	// Should have a method node for admin_action.
	methodNodes := filterNodes(result.Nodes, "method")
	if len(methodNodes) < 1 {
		t.Fatalf("expected at least 1 method node, got %d", len(methodNodes))
	}

	// Should have a call edge for perform_task.
	callEdges := filterEdges(result.Edges, "calls")
	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge for perform_task, got %d", len(callEdges))
	}
}

func TestRubyExtractor_RailsModelWithDecorators(t *testing.T) {
	ext := NewRubyExtractor()
	source := `class Post < ApplicationRecord
  belongs_to :author
  has_many :comments
  has_one :featured_image

  validates :title, presence: true
  validate :custom_validation

  before_save :normalize_title
  after_create :send_notification

  scope :published, -> { where(published: true) }

  attr_accessor :temp_field

  def publish!
    update(published: true)
  end
end
`
	opts := makeOpts(source, "app/models/post.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have 1 class node.
	typeNodes := filterNodes(result.Nodes, "type")
	if len(typeNodes) != 1 {
		t.Fatalf("expected 1 type node, got %d", len(typeNodes))
	}

	// Should have an extends edge to ApplicationRecord.
	extendsEdges := filterEdges(result.Edges, "extends")
	if len(extendsEdges) != 1 {
		t.Fatalf("expected 1 extends edge, got %d", len(extendsEdges))
	}

	// Should have multiple decorates edges for Rails DSL calls.
	decoratesEdges := filterEdges(result.Edges, "decorates")
	if len(decoratesEdges) < 5 {
		t.Fatalf("expected at least 5 decorates edges (belongs_to, has_many, has_one, validates, validate, before_save, after_create, scope, attr_accessor), got %d", len(decoratesEdges))
	}

	// Should have a method node for publish!.
	methodNodes := filterNodes(result.Nodes, "method")
	if len(methodNodes) < 1 {
		t.Fatalf("expected at least 1 method node for publish!, got %d", len(methodNodes))
	}
}

func TestRubyExtractor_RequireStatements(t *testing.T) {
	ext := NewRubyExtractor()
	source := `require 'json'
require_relative './helpers/formatter'

class Parser
  def parse(data)
    JSON.parse(data)
  end
end
`
	opts := makeOpts(source, "lib/parser.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	importEdges := filterEdges(result.Edges, "imports")
	if len(importEdges) < 2 {
		t.Fatalf("expected at least 2 import edges, got %d", len(importEdges))
	}

	for _, e := range importEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("import edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("import edge confidence = %f, want 0.7", e.Confidence)
		}
	}
}

func TestRubyExtractor_Module(t *testing.T) {
	ext := NewRubyExtractor()
	source := `module Concerns
  module Trackable
    def track_changes
      # tracking logic
    end
  end
end
`
	opts := makeOpts(source, "app/models/concerns/trackable.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	typeNodes := filterNodes(result.Nodes, "type")
	if len(typeNodes) < 2 {
		t.Fatalf("expected at least 2 type nodes (Concerns, Trackable), got %d", len(typeNodes))
	}

	// Verify type node names (using QualifiedName containment).
	if !containsName(typeNodes, "Concerns") {
		t.Error("expected type node for Concerns")
	}
	if !containsName(typeNodes, "Trackable") {
		t.Error("expected type node for Trackable")
	}
}

func TestRubyExtractor_RoutesFile(t *testing.T) {
	ext := NewRubyExtractor()
	source := `Rails.application.routes.draw do
  get '/health', to: 'health#index'
  post '/users', to: 'users#create'
  resources :articles
  namespace :api do
  end
end
`
	opts := makeOpts(source, "config/routes.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	routeNodes := filterNodes(result.Nodes, "route_handler")
	if len(routeNodes) < 2 {
		t.Fatalf("expected at least 2 route_handler nodes, got %d", len(routeNodes))
	}

	routeEdges := filterEdges(result.Edges, "handles_route")
	if len(routeEdges) < 2 {
		t.Fatalf("expected at least 2 handles_route edges, got %d", len(routeEdges))
	}
}

func TestRubyExtractor_EmptyFile(t *testing.T) {
	ext := NewRubyExtractor()
	source := ""
	opts := makeOpts(source, "lib/empty.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty file, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges for empty file, got %d", len(result.Edges))
	}
}

func TestRubyExtractor_TopLevelFunction(t *testing.T) {
	ext := NewRubyExtractor()
	source := `def hello
  puts "hello world"
end

def goodbye
  puts "goodbye"
end
`
	opts := makeOpts(source, "lib/greeting.rb")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	funcNodes := filterNodes(result.Nodes, "function")
	if len(funcNodes) < 2 {
		t.Fatalf("expected at least 2 function nodes, got %d", len(funcNodes))
	}

	names := nodeNames(funcNodes)
	assertContains(t, names, "hello")
	assertContains(t, names, "goodbye")

	// Top-level methods should have kind "function", not "method".
	for _, n := range funcNodes {
		if n.Kind != "function" {
			t.Errorf("top-level method %q has kind %q, want 'function'", n.QualifiedName, n.Kind)
		}
	}
}

// --- helpers ---

func filterNodes(nodes []types.Node, kind string) []types.Node {
	var out []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func filterEdges(edges []types.Edge, edgeType string) []types.Edge {
	var out []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			out = append(out, e)
		}
	}
	return out
}

func nodeNames(nodes []types.Node) []string {
	var names []string
	for _, n := range nodes {
		parts := splitQualified(n.QualifiedName)
		if len(parts) > 0 {
			names = append(names, parts[len(parts)-1])
		}
	}
	return names
}

func splitQualified(qname string) []string {
	idx := lastDotIndex(qname)
	if idx < 0 {
		return []string{qname}
	}
	return []string{qname[:idx], qname[idx+1:]}
}

func lastDotIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func containsName(nodes []types.Node, name string) bool {
	for _, n := range nodes {
		if containsStr(n.QualifiedName, name) {
			return true
		}
	}
	return false
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || containsSubstr(haystack, needle))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func assertContains(t *testing.T, strs []string, target string) {
	t.Helper()
	for _, s := range strs {
		if s == target {
			return
		}
	}
	t.Errorf("expected %v to contain %q", strs, target)
}
