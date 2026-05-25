package treesitter_test

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractTypeHints(t *testing.T) {
	content := []byte(`from django.core.cache.backends.base import BaseCache
from django.http import HttpRequest, HttpResponse

def process_request(request: HttpRequest, cache: BaseCache) -> HttpResponse:
    data = cache.get("key")
    return HttpResponse(data)

class Handler:
    def handle(self, cache: BaseCache, timeout: int) -> None:
        cache.set("key", "val")
`)
	ext, _ := treesitter.NewTreeSitterExtractor("python")
	result, err := ext.Extract(context.Background(), types.ExtractOptions{
		RepoURL:    "test-repo",
		FilePath:   "app.py",
		FileHash:   types.NewHash([]byte("test-file")),
		Content:    content,
		ModuleRoot: "/tmp",
	})
	if err != nil {
		t.Fatal(err)
	}

	typeHintEdges := 0
	for _, e := range result.Edges {
		if e.EdgeType == "type_hint_of" {
			typeHintEdges++
			t.Logf("type_hint_of: %s -> %s", e.SourceHash, e.TargetHash)
		}
	}
	// process_request has: HttpRequest, BaseCache, HttpResponse (3 type hints)
	// Handler.handle has: BaseCache (1, int skipped, None skipped)
	// Total expected: 4
	if typeHintEdges < 3 {
		t.Errorf("expected at least 3 type_hint_of edges, got %d", typeHintEdges)
	}
	t.Logf("Total type_hint_of edges: %d", typeHintEdges)
}
