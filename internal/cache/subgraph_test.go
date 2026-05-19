package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/types"
)

// makeHash builds a deterministic Hash from a single byte value for tests.
func makeHash(b byte) types.Hash {
	var h types.Hash
	h[0] = b
	return h
}

func TestGetPutBasic(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{})

	key := makeHash(1)
	val := []byte("result-data")

	// Miss before put.
	got, ok := c.Get(key)
	if ok || got != nil {
		t.Fatalf("expected miss, got hit with value %q", got)
	}

	// Hit after put.
	c.Put(key, val)
	got, ok = c.Get(key)
	if !ok {
		t.Fatal("expected hit after Put, got miss")
	}
	if string(got) != string(val) {
		t.Fatalf("value mismatch: got %q, want %q", got, val)
	}
}

func TestGetExpiry(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{
		TTL: 50 * time.Millisecond,
	})

	key := makeHash(2)
	c.Put(key, []byte("data"))

	// Immediate read should hit.
	if _, ok := c.Get(key); !ok {
		t.Fatal("expected hit immediately after Put")
	}

	// After TTL expires the entry should be gone.
	time.Sleep(100 * time.Millisecond)
	if _, ok := c.Get(key); ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestMaxEntriesEviction(t *testing.T) {
	const max = 5
	c := NewSubgraphCache(SubgraphCacheOptions{MaxEntries: max})

	// Fill to capacity.
	for i := 0; i < max; i++ {
		c.Put(makeHash(byte(i)), []byte{byte(i)})
	}
	if c.Stats().Size != max {
		t.Fatalf("expected size %d, got %d", max, c.Stats().Size)
	}

	// One more entry should evict one existing entry.
	c.Put(makeHash(byte(max)), []byte{byte(max)})
	if c.Stats().Size != max {
		t.Fatalf("expected size %d after eviction, got %d", max, c.Stats().Size)
	}
	if c.Stats().Evictions != 1 {
		t.Fatalf("expected 1 eviction, got %d", c.Stats().Evictions)
	}
}

func TestInvalidate(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{})

	key := makeHash(10)
	c.Put(key, []byte("v"))

	c.Invalidate(key)

	if _, ok := c.Get(key); ok {
		t.Fatal("expected miss after Invalidate")
	}
}

func TestInvalidatePackages(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{})

	// Build a minimal hierarchical tree with one package.
	pkg := "github.com/example/repo://internal/store"
	edgeHash := types.NewHash([]byte("edge1"))
	tree := snapshot.BuildHierarchicalTree([]snapshot.EdgeInput{
		{EdgeHash: edgeHash, PackagePath: pkg, EdgeType: "calls"},
	})

	// Store a result under the package root.
	root := tree.PackageRoots[pkg]
	c.Put(root, []byte("cached-result"))

	if _, ok := c.Get(root); !ok {
		t.Fatal("expected hit before invalidation")
	}

	// Invalidate the package.
	c.InvalidatePackages([]string{pkg}, tree)

	if _, ok := c.Get(root); ok {
		t.Fatal("expected miss after InvalidatePackages")
	}
}

func TestClear(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{})
	for i := 0; i < 10; i++ {
		c.Put(makeHash(byte(i)), []byte{byte(i)})
	}
	c.Clear()
	if c.Stats().Size != 0 {
		t.Fatalf("expected size 0 after Clear, got %d", c.Stats().Size)
	}
}

func TestStats(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{})

	key := makeHash(20)
	c.Get(key)        // miss
	c.Put(key, []byte("x"))
	c.Get(key)        // hit

	s := c.Stats()
	if s.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", s.Hits)
	}
	if s.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", s.Misses)
	}
	if s.Size != 1 {
		t.Errorf("expected size 1, got %d", s.Size)
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewSubgraphCache(SubgraphCacheOptions{MaxEntries: 100})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := makeHash(byte(i % 10))
			c.Put(key, []byte{byte(i)})
			c.Get(key)
			c.Stats()
		}(i)
	}
	wg.Wait()
}
