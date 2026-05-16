package cssextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeOpts(content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "src/styles/main.css",
		FileHash: types.NewHash([]byte("test-file-hash")),
		Content:  []byte(content),
	}
}

func TestCSSExtractor_Name(t *testing.T) {
	e := NewCSSExtractor()
	if got := e.Name(); got != "treesitter-css" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-css")
	}
}

func TestCSSExtractor_CanHandle(t *testing.T) {
	e := NewCSSExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"src/styles/main.css", true},
		{"src/styles/theme.scss", true},
		{"src/app.js", false},
		{"src/index.ts", false},
		{"node_modules/bootstrap/dist/css/bootstrap.css", false},
		{"vendor/node_modules/lib/style.css", false},
		{"src/styles/README.md", false},
		{"styles.CSS", true}, // case insensitive extension
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCSSExtractor_ExtractClassSelectors(t *testing.T) {
	e := NewCSSExtractor()
	css := `.container {
  display: flex;
}

.header {
  color: red;
}
`
	opts := makeOpts(css)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should find .container and .header class selectors.
	classNodes := filterNodesByKind(result.Nodes, "class_selector")
	if len(classNodes) < 2 {
		t.Fatalf("expected at least 2 class_selector nodes, got %d", len(classNodes))
	}

	wantNames := map[string]bool{
		"github.com/example/repo://src/styles/main.css.class.container": true,
		"github.com/example/repo://src/styles/main.css.class.header":    true,
	}
	for _, n := range classNodes {
		if !wantNames[n.QualifiedName] {
			t.Errorf("unexpected class selector QN: %q", n.QualifiedName)
		}
		delete(wantNames, n.QualifiedName)
	}
	for qn := range wantNames {
		t.Errorf("missing expected class selector: %q", qn)
	}
}

func TestCSSExtractor_ExtractIDSelectors(t *testing.T) {
	e := NewCSSExtractor()
	css := `#main-content {
  width: 100%;
}

#sidebar {
  width: 300px;
}
`
	opts := makeOpts(css)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	idNodes := filterNodesByKind(result.Nodes, "id_selector")
	if len(idNodes) < 2 {
		t.Fatalf("expected at least 2 id_selector nodes, got %d", len(idNodes))
	}

	wantNames := map[string]bool{
		"github.com/example/repo://src/styles/main.css.id.main-content": true,
		"github.com/example/repo://src/styles/main.css.id.sidebar":      true,
	}
	for _, n := range idNodes {
		if !wantNames[n.QualifiedName] {
			t.Errorf("unexpected id selector QN: %q", n.QualifiedName)
		}
		delete(wantNames, n.QualifiedName)
	}
	for qn := range wantNames {
		t.Errorf("missing expected id selector: %q", qn)
	}
}

func TestCSSExtractor_ExtractCustomProperties(t *testing.T) {
	e := NewCSSExtractor()
	css := `:root {
  --color-primary: #3498db;
  --font-size-base: 16px;
}
`
	opts := makeOpts(css)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	propNodes := filterNodesByKind(result.Nodes, "custom_property")
	if len(propNodes) < 2 {
		t.Fatalf("expected at least 2 custom_property nodes, got %d", len(propNodes))
	}

	wantNames := map[string]bool{
		"github.com/example/repo://src/styles/main.css.property.--color-primary":  true,
		"github.com/example/repo://src/styles/main.css.property.--font-size-base": true,
	}
	for _, n := range propNodes {
		if !wantNames[n.QualifiedName] {
			t.Errorf("unexpected custom property QN: %q", n.QualifiedName)
		}
		delete(wantNames, n.QualifiedName)
	}
	for qn := range wantNames {
		t.Errorf("missing expected custom property: %q", qn)
	}
}

func TestCSSExtractor_ExtractImportEdges(t *testing.T) {
	e := NewCSSExtractor()
	css := `@import "reset.css";
@import 'typography.css';

.body {
  margin: 0;
}
`
	opts := makeOpts(css)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	importEdges := filterEdgesByType(result.Edges, "imports")
	if len(importEdges) < 2 {
		t.Fatalf("expected at least 2 import edges, got %d", len(importEdges))
	}

	// Verify edge properties.
	for _, edge := range importEdges {
		if edge.EdgeType != "imports" {
			t.Errorf("expected edge type 'imports', got %q", edge.EdgeType)
		}
		if edge.Confidence != 0.7 {
			t.Errorf("expected confidence 0.7, got %f", edge.Confidence)
		}
		if edge.Provenance != "ast_inferred" {
			t.Errorf("expected provenance 'ast_inferred', got %q", edge.Provenance)
		}
	}
}

func TestCSSExtractor_ExtractVarDependsOn(t *testing.T) {
	e := NewCSSExtractor()
	css := `:root {
  --color-primary: #3498db;
}

.button {
  background-color: var(--color-primary);
}
`
	opts := makeOpts(css)
	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	depEdges := filterEdgesByType(result.Edges, "depends_on")
	if len(depEdges) < 1 {
		t.Fatalf("expected at least 1 depends_on edge, got %d", len(depEdges))
	}

	edge := depEdges[0]
	if edge.EdgeType != "depends_on" {
		t.Errorf("expected edge type 'depends_on', got %q", edge.EdgeType)
	}
	if edge.Confidence != 0.7 {
		t.Errorf("expected confidence 0.7, got %f", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("expected provenance 'ast_inferred', got %q", edge.Provenance)
	}

	// The source should be the .button class selector hash.
	expectedSourceHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "button", "class_selector")
	if edge.SourceHash != expectedSourceHash {
		t.Errorf("expected source hash for .button, got different hash")
	}

	// The target should be the --color-primary custom property hash.
	expectedTargetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "--color-primary", "custom_property")
	if edge.TargetHash != expectedTargetHash {
		t.Errorf("expected target hash for --color-primary, got different hash")
	}
}

func TestCSSExtractor_ExcludesNodeModules(t *testing.T) {
	e := NewCSSExtractor()

	paths := []string{
		"node_modules/bootstrap/dist/css/bootstrap.css",
		"frontend/node_modules/lib/style.scss",
		"packages/node_modules/component/index.css",
	}

	for _, path := range paths {
		if e.CanHandle(path) {
			t.Errorf("CanHandle(%q) = true, want false (should exclude node_modules)", path)
		}
	}
}

func TestCSSExtractor_EmptyFile(t *testing.T) {
	e := NewCSSExtractor()
	opts := makeOpts("")
	result, err := e.Extract(context.Background(), opts)
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

// filterNodesByKind returns nodes matching the given kind.
func filterNodesByKind(nodes []types.Node, kind string) []types.Node {
	var result []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			result = append(result, n)
		}
	}
	return result
}

// filterEdgesByType returns edges matching the given type.
func filterEdgesByType(edges []types.Edge, edgeType string) []types.Edge {
	var result []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			result = append(result, e)
		}
	}
	return result
}
