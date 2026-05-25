package treesitter

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractEnvReadEdges(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantVars  []string
		wantCount int
	}{
		{
			name: "os.environ subscript",
			src: `def configure():
    key = os.environ["API_KEY"]
    secret = os.environ["SECRET"]
`,
			wantVars:  []string{"API_KEY", "SECRET"},
			wantCount: 2,
		},
		{
			name: "os.environ.get call",
			src: `def configure():
    host = os.environ.get("DB_HOST")
    port = os.environ.get("DB_PORT")
`,
			wantVars:  []string{"DB_HOST", "DB_PORT"},
			wantCount: 2,
		},
		{
			name: "os.getenv call",
			src: `def configure():
    home = os.getenv("HOME")
`,
			wantVars:  []string{"HOME"},
			wantCount: 1,
		},
		{
			name: "mixed patterns",
			src: `def configure():
    a = os.environ["KEY1"]
    b = os.environ.get("KEY2")
    c = os.getenv("KEY3")
`,
			wantVars:  []string{"KEY1", "KEY2", "KEY3"},
			wantCount: 3,
		},
		{
			name: "no matches",
			src: `def compute():
    x = 1 + 2
    return x
`,
			wantVars:  nil,
			wantCount: 0,
		},
		{
			name: "duplicate env var deduplicates",
			src: `def configure():
    a = os.environ["KEY"]
    b = os.getenv("KEY")
`,
			wantVars:  []string{"KEY"},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := makeOpts(tt.src)
			funcNode := parsePythonFunc(t, opts.Content)
			if funcNode == nil {
				if tt.wantCount == 0 {
					return
				}
				t.Fatal("no function_definition found in AST")
			}

			funcHash := types.NewHash([]byte("test-func"))
			nodes, edges := extractEnvReadEdges(funcNode, opts, "", funcHash)

			if len(edges) != tt.wantCount {
				t.Fatalf("expected %d edges, got %d", tt.wantCount, len(edges))
			}
			if len(nodes) != tt.wantCount {
				t.Fatalf("expected %d nodes, got %d", tt.wantCount, len(nodes))
			}

			// Verify edge properties.
			for _, e := range edges {
				if e.EdgeType != edgetype.ReadsEnv {
					t.Errorf("expected edge type %q, got %q", edgetype.ReadsEnv, e.EdgeType)
				}
				if e.Confidence != 0.9 {
					t.Errorf("expected confidence 0.9, got %f", e.Confidence)
				}
				if e.Provenance != "ast_inferred" {
					t.Errorf("expected provenance 'ast_inferred', got %q", e.Provenance)
				}
				if e.SourceHash != funcHash {
					t.Errorf("expected source hash to match funcHash")
				}
			}

			// Verify node properties.
			for _, n := range nodes {
				if n.Kind != types.KindEnvVar {
					t.Errorf("expected node kind %q, got %q", types.KindEnvVar, n.Kind)
				}
			}

			// Check expected var names are present via QN.
			qns := make(map[string]bool)
			for _, n := range nodes {
				qns[n.QualifiedName] = true
			}
			for _, varName := range tt.wantVars {
				expectedQN := "env://" + varName
				if !qns[expectedQN] {
					t.Errorf("missing node with QN %q, got %v", expectedQN, qns)
				}
			}
		})
	}
}

func TestExtractEnvReadEdges_MultipleInBody(t *testing.T) {
	// Test that env reads from nested control flow are captured.
	src := `def load_config():
    if True:
        db = os.environ["DATABASE_URL"]
    else:
        db = os.environ.get("FALLBACK_DB")
    home = os.getenv("HOME")
`
	opts := makeOpts(src)
	funcNode := parsePythonFunc(t, opts.Content)
	if funcNode == nil {
		t.Fatal("no function_definition found")
	}

	funcHash := types.NewHash([]byte("test-func"))
	nodes, edges := extractEnvReadEdges(funcNode, opts, "", funcHash)

	if len(edges) != 3 {
		t.Fatalf("expected 3 reads_env edges, got %d", len(edges))
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 env_var nodes, got %d", len(nodes))
	}
}

// parsePythonFunc parses Python source and returns the first function_definition node.
func parsePythonFunc(t *testing.T, content []byte) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Cleanup(func() { tree.Close() })
	return findFuncDef(tree.RootNode())
}

// findFuncDef recursively finds the first function_definition node.
func findFuncDef(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}
	if node.Type() == "function_definition" {
		return node
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		if found := findFuncDef(node.Child(i)); found != nil {
			return found
		}
	}
	return nil
}
