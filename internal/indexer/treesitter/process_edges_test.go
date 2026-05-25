package treesitter

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractProcessExecEdges(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantCmds  []string
		wantCount int
	}{
		{
			name: "subprocess.run with list",
			src: `def deploy():
    subprocess.run(["git", "push", "origin"])
`,
			wantCmds:  []string{"git"},
			wantCount: 1,
		},
		{
			name: "subprocess.Popen with string",
			src: `def start():
    subprocess.Popen("nginx")
`,
			wantCmds:  []string{"nginx"},
			wantCount: 1,
		},
		{
			name: "subprocess.call with list",
			src: `def build():
    subprocess.call(["make", "all"])
`,
			wantCmds:  []string{"make"},
			wantCount: 1,
		},
		{
			name: "os.system with string",
			src: `def cleanup():
    os.system("rm -rf /tmp/build")
`,
			wantCmds:  []string{"rm"},
			wantCount: 1,
		},
		{
			name: "dynamic variable args",
			src: `def run_cmd(cmd):
    subprocess.run(cmd)
`,
			wantCmds:  []string{"dynamic"},
			wantCount: 1,
		},
		{
			name: "no matches",
			src: `def compute():
    return 1 + 2
`,
			wantCmds:  nil,
			wantCount: 0,
		},
		{
			name: "multiple process calls",
			src: `def pipeline():
    subprocess.run(["git", "pull"])
    subprocess.run(["make", "build"])
    os.system("docker compose up")
`,
			wantCmds:  []string{"git", "make", "docker"},
			wantCount: 3,
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
			nodes, edges := extractProcessExecEdges(funcNode, opts, "", funcHash)

			if len(edges) != tt.wantCount {
				t.Fatalf("expected %d edges, got %d", tt.wantCount, len(edges))
			}
			if len(nodes) != tt.wantCount {
				t.Fatalf("expected %d nodes, got %d", tt.wantCount, len(nodes))
			}

			// Verify edge properties.
			for _, e := range edges {
				if e.EdgeType != edgetype.ExecutesProcess {
					t.Errorf("expected edge type %q, got %q", edgetype.ExecutesProcess, e.EdgeType)
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
				if n.Kind != types.KindProcess {
					t.Errorf("expected node kind %q, got %q", types.KindProcess, n.Kind)
				}
			}

			// Check expected commands are present via QN.
			qns := make(map[string]bool)
			for _, n := range nodes {
				qns[n.QualifiedName] = true
			}
			for _, cmd := range tt.wantCmds {
				expectedQN := "process://" + cmd
				if !qns[expectedQN] {
					t.Errorf("missing node with QN %q, got %v", expectedQN, qns)
				}
			}
		})
	}
}

func TestExtractProcessExecEdges_NestedCalls(t *testing.T) {
	// Test that process calls inside control flow are captured.
	src := `def deploy(env):
    if env == "prod":
        subprocess.run(["docker", "push"])
    else:
        os.system("echo skipping")
`
	opts := makeOpts(src)
	funcNode := parsePythonFunc(t, opts.Content)
	if funcNode == nil {
		t.Fatal("no function_definition found")
	}

	funcHash := types.NewHash([]byte("test-func"))
	nodes, edges := extractProcessExecEdges(funcNode, opts, "", funcHash)

	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}
