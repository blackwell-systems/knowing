package tsextractor

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractProcessExecEdges_Spawn(t *testing.T) {
	source := `function startWorker() {
  child_process.spawn("node", ["worker.js"]);
}
`
	opts := makeOpts(t, "worker.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "worker"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "startWorker", "function")

	nodes, edges := ExtractProcessExecEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	if edges[0].EdgeType != edgetype.ExecutesProcess {
		t.Errorf("expected edge type %q, got %q", edgetype.ExecutesProcess, edges[0].EdgeType)
	}
	if edges[0].Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", edges[0].Confidence)
	}
	if nodes[0].Kind != types.KindProcess {
		t.Errorf("expected node kind %q, got %q", types.KindProcess, nodes[0].Kind)
	}
	if nodes[0].QualifiedName != "process://node" {
		t.Errorf("expected QN %q, got %q", "process://node", nodes[0].QualifiedName)
	}
}

func TestExtractProcessExecEdges_Exec(t *testing.T) {
	source := `function install() {
  child_process.exec("npm install");
}
`
	opts := makeOpts(t, "build.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "build"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "install", "function")

	nodes, edges := ExtractProcessExecEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	// "npm install" should extract just "npm" as the command.
	if nodes[0].QualifiedName != "process://npm" {
		t.Errorf("expected QN %q, got %q", "process://npm", nodes[0].QualifiedName)
	}
}

func TestExtractProcessExecEdges_ExecSync(t *testing.T) {
	source := `function runCmd() {
  execSync("cmd");
}
`
	opts := makeOpts(t, "run.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "run"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "runCmd", "function")

	nodes, edges := ExtractProcessExecEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if nodes[0].QualifiedName != "process://cmd" {
		t.Errorf("expected QN %q, got %q", "process://cmd", nodes[0].QualifiedName)
	}
}

func TestExtractProcessExecEdges_DynamicArg(t *testing.T) {
	source := `function runDynamic() {
  const cmd = getCommand();
  child_process.exec(cmd);
}
`
	opts := makeOpts(t, "dynamic.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "dynamic"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "runDynamic", "function")

	nodes, edges := ExtractProcessExecEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if nodes[0].QualifiedName != "process://dynamic" {
		t.Errorf("expected QN %q, got %q", "process://dynamic", nodes[0].QualifiedName)
	}
}

func TestExtractProcessExecEdges_NoMatches(t *testing.T) {
	source := `function doMath() {
  const x = 1 + 2;
  return x;
}
`
	opts := makeOpts(t, "math.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "math"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "doMath", "function")

	nodes, edges := ExtractProcessExecEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestExtractProcessExecEdges_NilBody(t *testing.T) {
	opts := makeOpts(t, "empty.ts", "")
	qnamePrefix := "empty"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "fn", "function")

	nodes, edges := ExtractProcessExecEdges(nil, opts, qnamePrefix, sourceHash)

	if nodes != nil || edges != nil {
		t.Fatalf("expected nil results for nil body")
	}
}
