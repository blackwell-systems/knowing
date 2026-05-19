// Package cache provides a thread-safe, TTL-bounded subgraph result cache
// keyed by Merkle subgraph roots.
//
// When the hierarchical tree shows a package root has not changed, cached
// results for queries scoped to that package are still valid. The daemon
// invalidates entries after each index run by comparing old and new
// hierarchical trees and evicting entries for changed packages.
package cache

import (
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	// DefaultMaxEntries is the default maximum number of cache entries.
	DefaultMaxEntries = 10_000

	// DefaultTTL is the default time-to-live for cache entries.
	DefaultTTL = 1 * time.Hour
)

// CacheStats holds hit/miss counters and size information for a SubgraphCache.
type CacheStats struct {
	Hits       int64
	Misses     int64
	Size       int
	Evictions  int64
	MaxEntries int
}

// entry is a single cache record.
type entry struct {
	value     []byte
	expiresAt time.Time
}

// SubgraphCache caches query results keyed by Merkle subgraph roots.
// When the hierarchical tree shows a package root has not changed,
// cached results for queries scoped to that package are still valid.
//
// The cache is fully thread-safe via a sync.RWMutex. Reads use a shared
// lock; writes (Put, Invalidate, Clear) use an exclusive lock.
type SubgraphCache struct {
	mu         sync.RWMutex
	entries    map[types.Hash]*entry
	maxEntries int
	ttl        time.Duration

	// metrics (updated under mu)
	hits      int64
	misses    int64
	evictions int64
}

// SubgraphCacheOptions configures a SubgraphCache.
type SubgraphCacheOptions struct {
	// MaxEntries is the maximum number of entries to hold before eviction.
	// Defaults to DefaultMaxEntries when zero.
	MaxEntries int

	// TTL is the time-to-live for each entry.
	// Defaults to DefaultTTL when zero.
	TTL time.Duration
}

// NewSubgraphCache creates a SubgraphCache with the given options.
// Zero-value options use the package-level defaults.
func NewSubgraphCache(opts SubgraphCacheOptions) *SubgraphCache {
	max := opts.MaxEntries
	if max <= 0 {
		max = DefaultMaxEntries
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &SubgraphCache{
		entries:    make(map[types.Hash]*entry, 64),
		maxEntries: max,
		ttl:        ttl,
	}
}

// Get returns the cached value for key if it exists and has not expired.
// Returns (nil, false) on miss or expiry.
func (c *SubgraphCache) Get(key types.Hash) ([]byte, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		// Expired: remove it and report a miss.
		c.mu.Lock()
		delete(c.entries, key)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return e.value, true
}

// Put stores value under key, overwriting any existing entry.
// When the cache is at capacity, a random existing entry is evicted to
// make room. The eviction strategy is intentionally simple: a random
// entry from the map is removed, which is O(1) and avoids the overhead
// of an LRU list for this use case.
func (c *SubgraphCache) Put(key types.Hash, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict one entry if we are at capacity (and this is a new key).
	_, exists := c.entries[key]
	if !exists && len(c.entries) >= c.maxEntries {
		// Remove an arbitrary entry from the map (Go map iteration is random).
		for k := range c.entries {
			delete(c.entries, k)
			c.evictions++
			break
		}
	}

	c.entries[key] = &entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes the entry for key if it exists.
func (c *SubgraphCache) Invalidate(key types.Hash) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidatePackages removes cache entries for a set of changed packages.
// For each changed package, it looks up the package's root hash in tree
// and evicts any entry stored under that key. This is the daemon
// invalidation path: after each index run, call this with the list of
// packages that changed and the new hierarchical tree.
func (c *SubgraphCache) InvalidatePackages(packages []string, tree *snapshot.HierarchicalTree) {
	if len(packages) == 0 || tree == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, pkg := range packages {
		root, ok := tree.PackageRoots[pkg]
		if !ok {
			continue
		}
		delete(c.entries, root)
	}
}

// Clear removes all entries from the cache.
func (c *SubgraphCache) Clear() {
	c.mu.Lock()
	c.entries = make(map[types.Hash]*entry, 64)
	c.mu.Unlock()
}

// Stats returns a snapshot of cache metrics. The returned struct is a
// value copy and is safe to read without holding any lock.
func (c *SubgraphCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Hits:       c.hits,
		Misses:     c.misses,
		Size:       len(c.entries),
		Evictions:  c.evictions,
		MaxEntries: c.maxEntries,
	}
}
