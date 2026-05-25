package tsextractor

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractEnvReadEdges_DotNotation(t *testing.T) {
	source := `function getToken() {
  const token = process.env.GITHUB_TOKEN;
  return token;
}
`
	opts := makeOpts(t, "config.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "config"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "getToken", "function")

	nodes, edges := ExtractEnvReadEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	if edges[0].EdgeType != edgetype.ReadsEnv {
		t.Errorf("expected edge type %q, got %q", edgetype.ReadsEnv, edges[0].EdgeType)
	}
	if edges[0].Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", edges[0].Confidence)
	}
	if nodes[0].Kind != types.KindEnvVar {
		t.Errorf("expected node kind %q, got %q", types.KindEnvVar, nodes[0].Kind)
	}
	if nodes[0].QualifiedName != "env://GITHUB_TOKEN" {
		t.Errorf("expected QN %q, got %q", "env://GITHUB_TOKEN", nodes[0].QualifiedName)
	}
}

func TestExtractEnvReadEdges_BracketNotation(t *testing.T) {
	source := `function getSecret() {
  const secret = process.env["NPM_TOKEN"];
  return secret;
}
`
	opts := makeOpts(t, "config.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "config"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "getSecret", "function")

	nodes, edges := ExtractEnvReadEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if nodes[0].QualifiedName != "env://NPM_TOKEN" {
		t.Errorf("expected QN %q, got %q", "env://NPM_TOKEN", nodes[0].QualifiedName)
	}
}

func TestExtractEnvReadEdges_MultipleVars(t *testing.T) {
	source := `function loadConfig() {
  const a = process.env.DB_HOST;
  const b = process.env["DB_PORT"];
  const c = process.env.DB_HOST;
}
`
	opts := makeOpts(t, "config.ts", source)
	root, tree := parseTSSource(t, source)
	defer tree.Close()

	body := findFunctionBody(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	qnamePrefix := "config"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "loadConfig", "function")

	nodes, edges := ExtractEnvReadEdges(body, opts, qnamePrefix, sourceHash)

	// DB_HOST appears twice but should be deduplicated. DB_PORT is separate.
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges (deduplicated), got %d", len(edges))
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestExtractEnvReadEdges_NoMatches(t *testing.T) {
	source := `function doWork() {
  const x = 1 + 2;
  console.log(x);
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
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "doWork", "function")

	nodes, edges := ExtractEnvReadEdges(body, opts, qnamePrefix, sourceHash)

	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(nodes))
	}
}

func TestExtractEnvReadEdges_NilBody(t *testing.T) {
	opts := makeOpts(t, "empty.ts", "")
	qnamePrefix := "empty"
	sourceHash := types.ComputeNodeHash(opts.RepoURL, qnamePrefix, types.EmptyHash, "fn", "function")

	nodes, edges := ExtractEnvReadEdges(nil, opts, qnamePrefix, sourceHash)

	if nodes != nil || edges != nil {
		t.Fatalf("expected nil results for nil body")
	}
}
