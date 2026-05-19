# Git Design Audit: knowing vs. git

This document is a systematic comparison of knowing's content-addressed design
against git's reference implementation. Git has 20 years of production hardening.
Every section follows the same structure: what git does, what knowing does, the
gap, and an actionable recommendation with exact file references, specific changes,
and effort estimates.

Items are ordered by severity: data-corruption and silent-incorrectness issues
first, then performance, then missing features.

**CRITICAL** means a bug that can produce silent data corruption, hash collisions,
or integrity failures. **HIGH** means a meaningful correctness or security gap.
**MED** means a performance or reliability gap. **LOW** means a missing feature
with no current correctness risk.

---

## 1. Hash Computation and Identity

### What git does

Git prepends a type-and-length header before hashing any object.
`object-file.c:format_object_header` (line 111) produces `"<type> <size>\0"` and
that header is fed into the hash context before the content bytes. So the hash of
a blob "hello\n" is not `SHA256("hello\n")` but `SHA256("blob 6\0hello\n")`. This
means two objects with identical byte content but different types (e.g., a blob
and a tree that happen to share byte representation) will always have different
object IDs. The format is validated by `check_object_signature`
(`object-file.c:122`) and `stream_object_signature` (`object-file.c:135`).

Additionally, git uses SHA-1 with collision detection (SHAttered attack mitigation)
via `sha1dc_git.c`. When a collision attack is detected during finalization,
`git_SHA1DCFinal` calls `die()` rather than silently producing a compromised hash
(`sha1dc_git.c:19-25`). For new repositories git now defaults to SHA-256
(`hash.h:55-73`), and the hash algorithm is stored per-repository.

### What knowing does

`internal/types/types.go:NewHash` calls `sha256.Sum256(data)` directly with no
type prefix. `ComputeNodeHash` (line 162) formats the input as
`repoURL\0packagePath\0symbolName\0symbolKind`. `ComputeEdgeHash` (line 171) uses
`sourceHash\0targetHash\0edgeType\0provenanceJSON`. The NUL separators prevent
adjacent-field collisions, which is good. However, there is no type prefix
distinguishing a Node hash from an Edge hash, and no type prefix distinguishing a
Snapshot hash from a Merkle intermediate node hash.

The snapshot hash is set to `tree.Root` in `snapshot/manager.go:81`, which is the
output of `BuildMerkleTree` over edge hashes. A Merkle intermediate node is also a
SHA-256 of 64 bytes of raw hash data (`snapshot/merkle.go:68`). This means a
snapshot root hash and a Merkle interior node hash both have the same format, so
they are indistinguishable from their hash value alone.

### Gap

**CRITICAL.** There is no domain-type prefix in any known hash. A snapshot root
and a Merkle interior node could share a hash value if an interior node happened to
be built from data that matched a snapshot's concatenation of edge hashes. More
importantly, there is nothing in the hash value itself that identifies whether it
belongs to a node, edge, file, repo, or snapshot. Downstream code that relies on
hashes being globally unique across types is silently unsafe. Git's
`"<type> <size>\0<content>"` format makes cross-type collisions structurally
impossible.

There is also no collision detection. Git terminates the process when it detects a
SHAttered-style attack (`sha1dc_git.c:23`). Knowing accepts whatever `crypto/sha256`
returns with no alert.

### Recommendations

**Rec 1.1 (CRITICAL, 2h):** Add a domain-type prefix to every hash computation.
File: `internal/types/types.go`.

Change `ComputeNodeHash` to prefix the data with `"node\0"` before hashing:
```
data := fmt.Sprintf("node\x00%s\x00%s\x00%s\x00%s", repoURL, packagePath, symbolName, symbolKind)
```

Change `ComputeEdgeHash` to prefix with `"edge\0"`:
```
data := fmt.Sprintf("edge\x00%s\x00%s\x00%s\x00%s", sourceHash, targetHash, edgeType, provenanceJSON)
```

Add a `ComputeSnapshotHash` function that prefixes with `"snapshot\0"` and is
called from `snapshot/manager.go` instead of using the raw Merkle root. Add
`ComputeMerkleNodeHash` used only inside `snapshot/merkle.go:combineHashes` that
prefixes with `"merkle\0"`.

WARNING: These changes break all existing hashes. Any database populated with the
old scheme must be re-indexed. Gate this behind a schema migration that drops and
re-creates all rows, or bump the migration version to force a full reindex.

**Rec 1.2 (HIGH, 1h):** Add a `VerifyHash` function in `internal/types/types.go`
that recomputes and compares a stored hash against its inputs, returning an error if
they do not match. This is the equivalent of git's `check_object_signature`. Call
it from `SQLiteStore.PutNode` and `SQLiteStore.PutEdge` in debug/verify mode, and
expose it as part of the integrity checker recommended in section 8.

Git reference: `object-file.c:122-133` (`check_object_signature`).

### Where knowing is better

Knowing's use of NUL byte field separators in `ComputeNodeHash` and
`ComputeEdgeHash` is a genuine improvement over git's implicit structure: it
prevents the "field boundary ambiguity" attack where `"ab" + "cd"` hashes
identically to `"a" + "bcd"`. Git's object header relies on the space and NUL as
implicit delimiters and the fixed-meaning fields (type names are enumerated), which
achieves the same goal but in a format that is more tightly coupled to git's type
system.

---

## 2. Object Storage and Retrieval

### What git does

Git uses a two-tier storage model:

**Loose objects** (`object-file.c`, `odb.c`): Each object is zlib-compressed and
stored as `.git/objects/XX/YYYY...` where `XX` is the first byte of the hash and
`YYYY...` is the remainder. Writes are fast: one `write(2)` call per object, no
index to update. The 2-byte directory split caps directory entry count at 256 dirs
of ~262k entries each, keeping filesystem directory scans O(1) in practice.
Loose objects are read-only after write (mode 0444, `odb.c:78`).

**Packfiles** (`packfile.c`, `pack-objects.c`): When there are too many loose
objects (`gc.auto.threshold` defaults to 6700, `builtin/gc.c:154`), `git gc`
packs them: objects are delta-compressed against each other, indexed by a `.idx`
file, and stored in a `.pack` file. Reads via the `.idx` binary search are fast
even with millions of objects. The multi-pack index (MIDX, `midx.c`) provides a
single index across all packfiles for even faster lookups.

Git freshens loose objects on access by calling `utime(fn, NULL)`
(`object-file.c:75-78`). This prevents `prune --expire` from deleting objects that
are being actively used.

### What knowing does

Everything is stored in a single SQLite database (`internal/store/sqlite.go`).
WAL mode is enabled (`sqlite.go:47`) which allows concurrent readers with one
writer. There is no distinction between "recently written" and "packed" data. Every
node, edge, and snapshot sits in a B-tree table, and SQLite manages its own page
cache.

### Gap

**MED.** There is no analog to git's loose/packed split. For knowing's workload
(mostly full-repo index runs that bulk-insert thousands of nodes and edges), SQLite
batch transactions already provide good write throughput. However, there is no
concept of "recently written objects are cheap to GC" because all objects share the
same table and there is no mtime or freshness signal.

The more important gap is that there is no read-side content cache. Git's
object database keeps a small in-memory `cached_objects` array (`odb.c:38-66`) and
the OS page cache covers loose objects. For knowing, repeated queries to SQLite for
the same node hash incur SQL round-trips. The hot-path for blast-radius queries
traverses hundreds of edges, each requiring a lookup.

### Recommendations

**Rec 2.1 (MED, 4h):** Add a generation or epoch field to the nodes and edges
tables (a migration-added `indexed_at INTEGER` column). When `GarbageCollect` runs
it can prune objects whose `indexed_at` is older than N snapshots ago. This is the
functional equivalent of git's mtime freshening: objects referenced by a recent
index run have a recent `indexed_at`; objects from a stale index run that are not
referenced anymore are safe to prune. File: `internal/store/migrations/` (new SQL
migration) and `internal/snapshot/manager.go:GarbageCollect`.

**Rec 2.2 (MED, 3h):** Add an in-process node and edge cache in
`internal/store/sqlite.go`, keyed by hash. A simple `sync.Map` or an LRU bounded
by count (e.g., 50k entries) would eliminate redundant SQL round-trips on hot-path
traversals. The cache can be invalidated at the start of each index run.
Git reference: `odb.c:38-66` (cached_objects array).

**Rec 2.3 (LOW, 1 day):** Consider a SQLite "packing" strategy: a
`compact_edges` migration that rewrites the edges table without the per-edge
`observation_count` and `last_observed` columns (moving them to a separate
`edge_observations` table). This reduces the row size for the common case
(static-only edges) analogously to git's delta compression reducing pack size. Not
urgent, but important for repos with >500k edges.

---

## 3. Tree Structure and Traversal

### What git does

Git represents a directory tree as a sorted list of `(mode, name, OID)` tuples
stored in binary in a tree object. The sort order uses `base_name_compare`
(`tree-diff.c:152`) which sorts files and directories as if directories had a
trailing `/`. This canonical sort means two trees are byte-for-byte identical if
and only if their contents are identical, enabling O(1) equality checks via OID
comparison.

The tree-walk (`tree-walk.c`) iterates entries in sorted order. `tree-diff.c`
exploits this sorting to merge-scan N parent trees simultaneously in `ll_diff_tree_paths`
(line 430): at each step it finds `imin = argmin(path across all parents)` and
advances only the trees that are at the minimum. This makes the N-parent diff
O(total unique paths) rather than O(paths * parents).

Trees are identified by the hash of their content, not their position in the
commit history. A subtree that does not change between commits shares its OID with
all previous versions.

### What knowing does

The `HierarchicalTree` in `snapshot/hierarchical.go` uses a three-level structure:
repo root, package roots, and edge-type roots. Packages are identified by their
string path; the root at each level is `BuildMerkleTree(sorted leaf hashes)`.
The `DiffHierarchicalTrees` function (`hierarchical.go:147`) compares maps of
package roots and edge-type roots.

### Gap

**LOW.** The hierarchical tree is already a sound design that enables O(packages)
diffs. The gap is narrower than it appears:

1. Knowing's diff is 2-tree only. There is no N-parent diff. This is appropriate
   today since knowing does not model merge commits as multi-parent snapshots.
2. The package path is extracted from the qualified name by
   `extractPackagePath` (`manager.go:182`). If a qualified name does not follow the
   `repoURL://pkgPath.SymbolName` format, it falls back to `""` (the `_root`
   bucket). This silent grouping of malformed names is a correctness risk.
3. The `EdgeTypeRoots` scan in `DiffHierarchicalTrees` (line 180) iterates all
   edge-type roots even for unchanged packages because the filter only excludes
   packages not in `changedPkgSet`. For a repo with many packages and few changes
   this is fine, but the comment says "only for changed/added packages to avoid
   full scan" while the code still iterates all keys. The inner `if !changedPkgSet[pkg]`
   check does short-circuit the work, but allocates the full map iteration.

### Recommendations

**Rec 3.1 (HIGH, 2h):** Make `extractPackagePath` return an error (or a sentinel
distinct from `_root`) when the qualified name does not match the expected format.
Log a warning and skip the edge rather than silently grouping it under `_root`.
File: `internal/snapshot/manager.go:182`. This prevents malformed nodes from
polluting the `_root` bucket and causing false positive diff results.

**Rec 3.2 (LOW, 1h):** Add an early-exit to `DiffHierarchicalTrees` when
`!diff.RootChanged` that also validates the tree is non-nil. Currently a nil
`oldTree` or `newTree` will panic. File: `internal/snapshot/hierarchical.go:148`.

**Rec 3.3 (LOW, 2h):** Add pathspec-style filtering to `DiffHierarchicalTrees`,
accepting an optional `[]string` of package prefixes to limit the diff scope.
This mirrors git's `skip_uninteresting` (`tree-diff.c:320`) and would allow tools
like `blast_radius` to request a diff scoped to a single package without
re-traversing the full tree.

---

## 4. Diff Algorithm

### What git does

`tree-diff.c:ll_diff_tree_paths` (line 430) implements an N-parent simultaneous
tree walk. Key optimizations:

1. **N-parent support**: merges are diffed against all parents at once.
2. **Early exit**: `diff_can_quit_early(opt)` (`tree-diff.c:462`) stops the walk
   as soon as the caller signals it has seen enough (e.g., `--max-count`).
3. **Pathspec filtering**: `skip_uninteresting` (`tree-diff.c:320`) advances a
   tree descriptor past entries that do not match the pathspec. The walk never
   descends into uninteresting subtrees.
4. **`S_IFXMIN_NEQ` bit** (`tree-diff.c:27`): a mode bit flag set on parent entries
   that do not equal the minimum-path parent. Entries with this bit set are skipped
   in `update_tp_entries` (line 422), so parents that are "behind" in the sort
   order are never advanced out of turn.
5. **Max changes limit**: `opt->max_changes` (line 465) hard-caps the result set
   so callers cannot be overwhelmed.

### What knowing does

`DiffHierarchicalTrees` (`hierarchical.go:147`) is a 2-tree map comparison with no
early exit, no pathspec filtering, and no max-results cap. `DiffMerkle`
(`merkle.go:77`) builds two `map[Hash]struct{}` sets and iterates both. This is
O(leaves) time and O(leaves) space with no way to stop early.

### Gap

**MED.** For large repos (>100k edges) a full diff scan on every index cycle
allocates a map of all edge hashes. There is no way for a caller to say "I only
care about changes in package X" without receiving all changes and filtering
afterward.

**LOW.** There is no max-results cap. A repo that adds 50k edges in one commit
will produce a 50k-entry slice from `DiffMerkle`.

### Recommendations

**Rec 4.1 (MED, 3h):** Add an `options` parameter to `DiffHierarchicalTrees` with
`PackageFilter []string` and `MaxChanges int` fields. Skip packages not in the
filter. Return early once `MaxChanges` changed edge types are found. File:
`internal/snapshot/hierarchical.go`. This mirrors git's pathspec and max-changes
early-exit from `tree-diff.c:462-465`.

**Rec 4.2 (LOW, 2h):** Add a `MaxEdges int` option to `DiffMerkle` that causes
it to return an `ErrTooManyChanges` sentinel when the result set exceeds the cap.
File: `internal/snapshot/merkle.go:77`. Callers in the MCP layer can return a
"diff too large, use package filter" message rather than materializing a 50k-item
slice.

---

## 5. Delta Compression

### What git does

`diff-delta.c` implements Rabin fingerprinting for rolling hash computation. The
core loop (`diff-delta.c:185-199`) slides a 16-byte window (RABIN_WINDOW) over the
source buffer, computing `val = ((val << 8) | data[i]) ^ T[val >> RABIN_SHIFT]`.
Every window position becomes an entry in a hash table indexed by its Rabin value.
When encoding a delta, the encoder looks up each 16-byte window of the target in
this table and, if found, emits a "copy from source at offset X, length Y"
instruction. Otherwise it emits a "insert these N bytes literally" instruction.

The result is a delta instruction stream (copy + insert opcodes) that, when applied
by `patch-delta.c`, reconstructs the target from the source with minimal data
transfer. For similar objects (e.g., two versions of the same file) this reduces
representation size by 60-90%.

Packfiles (`packfile.c`) store objects as deltas against a "base object" in the
same pack. The base can be identified by OID (OFS_DELTA) or by offset in the pack
file (OFS_DELTA), with the offset form being more compact.

### What knowing does

There is no delta compression. Each snapshot stores the full set of edge hashes,
and every edge record stores its full fields. If a repo has 100k edges and only 3
change between commits, knowing stores 100k edge rows regardless. The Merkle tree
is recomputed from all edge hashes on every snapshot. The `EdgeEvent` table
(`types.go:132`) records which edges were added or removed, which is the event-log
equivalent of a delta, but the actual edge data is never delta-compressed.

### Gap

**LOW for correctness.** There is no silent data loss. But for large repos the
storage cost scales linearly with total edges, not with the change rate. A repo
with 500k edges that changes 10 edges per commit will re-serialize and re-hash all
500k edges on every index run.

The `EdgeEvent` table already captures the delta at the event level. The gap is
only in the absence of storage compression.

### Recommendations

**Rec 5.1 (LOW, 1 week):** The functional analog to git's delta compression for
knowing is snapshot-relative edge sets: instead of storing the full edge set per
snapshot, store only the diff (added + removed edges) as events, and compute the
full edge set on demand by replaying events from the base snapshot. The
`EdgeEvent` table already records this information. The missing piece is a
`ReconstructEdgeSet(ctx, snapshotHash)` function in `snapshot/manager.go` that
replays events from the genesis snapshot forward. This is logically equivalent to
`patch-delta.c`.

File: `internal/snapshot/manager.go`. New function: `ReconstructEdgeSet`.

**Rec 5.2 (LOW, 3h):** The snapshot computation in `manager.go:ComputeSnapshot`
currently re-collects all edges by walking all nodes. For large repos, add an
incremental path: if a previous snapshot exists and only N files changed (from the
index run's `changedFiles` list), collect only edges from those files' nodes and
merge with the previous edge set. This halves the snapshot compute cost for
incremental index runs.

---

## 6. Garbage Collection

### What git does

`builtin/gc.c` implements multi-phase GC:

1. **Pack loose objects**: runs `git pack-objects` to pack all loose objects,
   producing a new `.pack` + `.idx` pair.
2. **Prune unreachable objects**: runs `git prune --expire=<time>`. Any loose
   object not reachable from any ref and older than `gc.pruneExpire` (default
   2 weeks) is deleted.
3. **Pack refs**: consolidates per-file loose refs into `packed-refs`.
4. **Repack**: consolidates multiple packfiles into one (or a small number).
5. **Multi-pack index**: regenerates the MIDX for fast cross-pack lookup.

The key insight is **reachability**: an object is only safe to delete if no live
ref (branch, tag, HEAD) can reach it, AND it has not been freshened recently
(freshening = calling `utime` on access, so `prune` sees a recent mtime).

### What knowing does

`SnapshotManager.GarbageCollect` (`snapshot/manager.go:106`) walks the snapshot
chain and deletes snapshots older than `keepCount`. It also calls
`store.DeleteSnapshot` which (depending on the implementation) deletes associated
edge events. However, the node and edge tables themselves are never pruned. A node
that was added in snapshot 1, removed in snapshot 2, and is now outside the
`keepCount` window will remain in the `nodes` table forever with no snapshot
pointing to it.

There is also no concept of "reachability from a named ref." Since there are no
named refs (see section 7), reachability is implied by the snapshot chain, but
after GC prunes old snapshots there is no way to know whether a surviving node was
referenced by the deleted snapshots.

### Gap

**HIGH.** Orphaned nodes and edges accumulate indefinitely. On a repo that is
refactored frequently (functions renamed, files deleted, packages restructured), the
`nodes` table will grow without bound because GC only removes snapshots, not the
underlying objects they referenced.

This is a storage leak, not a correctness bug, but it can degrade query performance
over time as SQLite must scan larger tables.

### Recommendations

**Rec 6.1 (HIGH, 4h):** After deleting old snapshots in `GarbageCollect`, run a
reachability sweep. Collect all node hashes and edge hashes referenced by the
surviving snapshots, then delete any row in `nodes` or `edges` that is not in the
reachable set. File: `internal/snapshot/manager.go:GarbageCollect`.

Pseudo-code:
```
reachableNodes := set{}
reachableEdges := set{}
for each surviving snapshot:
    edges = store.EdgesForSnapshot(ctx, snap.SnapshotHash)
    for each edge: reachableEdges.add(edge.EdgeHash)
    for each edge: reachableNodes.add(edge.SourceHash, edge.TargetHash)

store.DeleteNodesNotIn(ctx, reachableNodes)
store.DeleteEdgesNotIn(ctx, reachableEdges)
```

This requires two new methods on `types.GraphStore`: `DeleteNodesNotIn` and
`DeleteEdgesNotIn`, implemented as `DELETE FROM nodes WHERE node_hash NOT IN (?)`
with SQLite's `json_each` or a temporary table for the hash set.

**Rec 6.2 (MED, 2h):** Add a `GarbageCollectStats` return type that reports how
many nodes and edges were pruned, analogous to git GC's reporting of object counts.
File: `internal/snapshot/manager.go`. This makes the operation observable and
testable.

**Rec 6.3 (MED, 2h):** Add a guard against running GC while an index is in
progress. Currently `GarbageCollect` acquires no lock. If a concurrent index run
inserts new edges between the "collect reachable set" and "delete unreachable"
steps, freshly-written edges could be deleted. File: `internal/daemon/daemon.go`
(hold `mu.Lock()` during GC, same as indexing).

---

## 7. References and Naming

### What git does

`refs.c` implements a complete reference namespace: branches (`refs/heads/`),
tags (`refs/tags/`), HEAD (a symbolic ref pointing to the current branch), and
arbitrary notes (`refs/notes/`). The two storage backends (files and reftable)
both support atomic multi-ref transactions via `ref_transaction_commit`.

The reflog (`reflog.c`) records every mutation to a ref with a timestamp and
identity, enabling `git reflog` to recover from accidental resets. The format is
`<old-oid> <new-oid> <identity> <timestamp>\t<message>`.

Named refs provide human-readable, stable names for OIDs. You can tag a specific
commit as `v1.0` and the tag name remains stable even as the branch advances.

### What knowing does

Snapshots form a singly-linked chain via `ParentHash` in `types.Snapshot`. There
is no named ref. The only way to address a snapshot is by its hash. There is no
equivalent to a branch (a mutable pointer to the latest snapshot), no equivalent
to a tag (an immutable pointer to a specific snapshot), and no reflog (a history
of how the chain head has moved).

### Gap

**LOW for current use cases.** The daemon uses `LatestSnapshot` (a query for the
most recent snapshot by repo hash and timestamp) as its implicit HEAD. This is
equivalent to an unnamed branch. For the current single-writer, single-repo use
case this is sufficient.

However, the absence of named refs means:
- There is no way to mark "this was the graph at v2.0 release" without storing
  the snapshot hash externally.
- Bisection (finding the snapshot where a specific edge appeared) requires walking
  the full chain.
- Multi-repo federation (comparing snapshots from two remotes) has no stable
  addressable names.

### Recommendations

**Rec 7.1 (LOW, 4h):** Add a `snapshot_refs` table with columns
`(name TEXT PRIMARY KEY, snapshot_hash BLOB, repo_hash BLOB, created_at INTEGER)`.
Expose `CreateRef(ctx, name, snapshotHash)` and `ResolveRef(ctx, name)` on
`GraphStore`. Use ref names like `HEAD` (the latest), `v1.0` (a release tag), or
`pre-refactor` (a named checkpoint). File: new SQL migration plus two new
`GraphStore` interface methods in `internal/types/interfaces.go`.

**Rec 7.2 (LOW, 2h):** Add a reflog table `(ref_name, old_hash, new_hash, timestamp,
message)` updated on every `CreateSnapshot`. This provides the audit trail that
`GarbageCollect` needs to check "was this snapshot ever pointed to by a ref?"
before deleting it.

---

## 8. Integrity Verification

### What git does

`fsck.c` implements a comprehensive object graph checker. It walks every reachable
object from every ref and verifies:

- Hash consistency: recomputes each object's hash and compares it to the stored OID
  (`check_object_signature` called from `fsck_loose_object`).
- Type validity: each object's declared type matches its content.
- Structural correctness: tree entries are sorted, commit fields are well-formed.
- Dangling references: every OID referenced by a tree or commit exists in the store.
- Duplicate entries in trees.
- Encoding issues (NUL in headers, unterminated headers).

Errors are classified into FATAL, ERROR, and WARN levels (`fsck.h:FOREACH_FSCK_MSG_ID`),
allowing users to configure which checks are errors vs. warnings.

### What knowing does

There is no `fsck` equivalent. There is no command or function that walks the
graph, recomputes hashes, checks referential integrity (every edge's SourceHash and
TargetHash refers to an existing node), or verifies that snapshot chains are
unbroken.

### Gap

**HIGH.** Without an integrity checker, silent corruption is undetectable. A hash
collision, a partial write interrupted by a crash, or a manual database edit would
go unnoticed until a query produced wrong results. There is also no way to diagnose
"why is this edge in the database but not appearing in blast_radius?" without manual
SQL queries.

### Recommendations

**Rec 8.1 (HIGH, 1 day):** Add a `Verify(ctx context.Context, repoHash types.Hash) ([]VerifyError, error)`
function to `SnapshotManager` (file: `internal/snapshot/manager.go`) that performs:

1. For each edge in the reachable set: verify `edge.SourceHash` and `edge.TargetHash`
   exist in the nodes table.
2. For each node in the reachable set: verify `node.FileHash` exists in the files
   table.
3. For each snapshot in the chain: verify `snap.ParentHash` either is zero or points
   to an existing snapshot.
4. For each edge: recompute `ComputeEdgeHash(...)` and compare to stored hash. A
   mismatch means the row was mutated after insertion.
5. For each node: recompute `ComputeNodeHash(...)` and compare to stored hash.

Return a slice of `VerifyError` structs classifying issues as ERROR or WARN. Expose
this as a `knowing fsck` CLI subcommand. Git reference: `fsck.c` and `fsck.h`.

**Rec 8.2 (HIGH, 2h):** Add a `PRAGMA integrity_check` call to `NewSQLiteStore`
on startup (or as a separate `knowing fsck --quick` mode). This catches SQLite-level
corruption (truncated pages, mismatched page checksums) before the application layer
sees inconsistent data. File: `internal/store/sqlite.go:NewSQLiteStore`.

---

## 9. Concurrency and Locking

### What git does

`lockfile.c` implements file-level locking for any operation that mutates a file.
The protocol is: create `foo.lock` atomically (O_CREAT|O_EXCL), write the new
content, then rename `foo.lock` to `foo`. The rename is atomic on POSIX filesystems.
A PID file (`foo-pid.lock`) is written alongside the lock for debugging stale locks
(`lockfile.c:109-136`).

For refs specifically, `refs/files-backend.c` uses this lock per-ref plus a
transaction layer that stages all ref updates before committing them atomically.
`hold_locked_ref` acquires the lock; `commit_ref_transaction` renames all staged
lock files in one pass.

For concurrent readers, git uses `obj_read_lock` / `obj_read_unlock` around the
critical section in `unpack_loose_rest` (`object-file.c:311-315`). This is a
coarse mutex protecting the zlib inflation state.

### What knowing does

`daemon.go` uses a `sync.RWMutex` (`mu`) at the daemon level: the index worker
holds a write lock, and all MCP query handlers hold a read lock. This is a
correct and idiomatic Go pattern.

SQLite WAL mode (`sqlite.go:47`) allows concurrent reads with a single writer at
the database level, reinforcing the daemon-level RWMutex with an independent
storage-level guarantee.

However, there is no per-operation locking outside the daemon. The `SQLiteStore`
itself has no mutex. If two goroutines call `PutNode` concurrently without the
daemon's RWMutex in front of them (e.g., in a test or a future multi-writer
scenario), the SQLite connection is shared and the behavior depends on SQLite's
internal locking (WAL mode serializes writes at the page level, so this is safe in
practice, but the Go `database/sql` connection pool could route concurrent writes to
different connections, which WAL handles but which could cause unexpected write
contention).

There is no lockfile or PID file for the daemon itself. A second `knowing watch`
invocation on the same database will succeed in opening the SQLite file and will
compete with the first daemon at the SQLite WAL level, producing undefined behavior
(both daemons responding to the same commit events, double-indexing, interleaved
writes).

### Gap

**HIGH.** Multiple daemon instances on the same database are not prevented. This
is the most likely correctness bug in a real multi-user deployment (e.g., two
terminal sessions both running `knowing watch` on the same repo).

**LOW.** The `sync.RWMutex` in the daemon is process-local. It does not protect
against a second process accessing the same SQLite file. SQLite WAL provides
serialization at the storage level but not at the business-logic level (e.g., two
processes could each read the latest snapshot, compute a new one, and both try to
insert it, resulting in a duplicate snapshot hash that the `INSERT OR REPLACE`
silently handles but that wastes work).

### Recommendations

**Rec 9.1 (HIGH, 2h):** Add a SQLite advisory lock or a lockfile at daemon startup.
The simplest approach is a lockfile at `<db_path>.lock` written with the daemon's
PID on startup and removed on shutdown. If the file exists and the PID is alive,
refuse to start with a clear error message. File: `internal/daemon/daemon.go`
(in the `Daemon.Start` method or wherever the daemon is initialized) and
`cmd/` (in the `watch` subcommand). Mirror git's `lockfile.c:lock_file` approach:
create with `O_CREAT|O_EXCL` to avoid race conditions.

**Rec 9.2 (LOW, 1h):** Set `db.SetMaxOpenConns(1)` on the SQLite connection in
`NewSQLiteStore` (`internal/store/sqlite.go:41`). SQLite WAL supports a single
writer; multiple open connections from the same process serializing through the
connection pool is correct but wasteful. A single connection eliminates the
per-write connection acquisition overhead and makes write serialization explicit.

---

## 10. Transfer and Sync

### What git does

Git's transfer protocol (`fetch-pack.c`, `send-pack.c`, `upload-pack.c`) is built
around the "have/want" negotiation:

1. The receiver advertises which OIDs it already has ("haves").
2. The sender determines the minimal set of objects the receiver needs ("wants
   minus haves").
3. The sender builds a "thin pack": a packfile that references base objects the
   receiver already has, minimizing transfer size.
4. The receiver verifies the pack (fsck on transfer with `--fetch-fsck-objects`)
   and resolves thin deltas.

The protocol is versioned (v0, v1, v2) and supports partial clones (filtering by
blob size or path), shallow clones (truncating history at a given depth), and
multi-ack (the receiver sends ACKs as it processes each batch of haves, allowing
the sender to stop sending redundant haves early).

### What knowing does

There is no sync protocol. Knowing is single-node today: one database, one daemon.
There is no way to sync a knowing graph from one machine to another, to share a
graph between two engineers working on the same repo, or to push a graph to a
central server.

### Gap

**LOW for current use cases** (single developer, local index). Potentially HIGH
for team adoption. Without a sync protocol, every new machine must re-index from
scratch. For a large repo (1M+ LOC) this takes minutes. For a team of 10, this
multiplies the indexing cost by 10.

### Recommendations

**Rec 10.1 (LOW, 2 weeks):** Design a minimum viable sync protocol based on
Merkle proof exchange. The protocol is simpler than git's because knowing objects
are immutable once written (edge hashes never change) and the graph is append-only
within a snapshot:

1. Receiver sends its latest snapshot hash to sender.
2. Sender computes the diff using `DiffHierarchicalTrees` between receiver's
   snapshot and sender's latest.
3. Sender sends only the added edges and nodes.
4. Receiver inserts them and computes a new snapshot.

The hierarchical tree (`snapshot/hierarchical.go`) already provides the diff
primitive. The missing piece is a wire encoding for edge/node batches (the existing
GCF format in `internal/wire/gcf.go` could be repurposed) and an HTTP endpoint
that accepts a snapshot hash and returns a diff pack.

File: new `internal/sync/` package with `SyncServer` and `SyncClient` types.

**Rec 10.2 (LOW, 1 week):** Before implementing full sync, add a `knowing export`
command that serializes all nodes, edges, and snapshots for a repo into a single
binary file (GCF or MessagePack), and a `knowing import` command that loads it.
This unblocks the "share a graph with a teammate" use case without requiring a
running server on either side. The wire format infrastructure in
`internal/wire/` is already present.

---

## Summary Table

The table below lists every recommendation in priority order. "Effort" is a
rough estimate for a single engineer working on the knowing codebase.

| # | Rec | Severity | Effort | Files | Status |
|---|-----|----------|--------|-------|--------|
| 1.1 | Add domain-type prefix to all hash computations | CRITICAL | 2h + reindex | `internal/types/types.go`, `internal/snapshot/merkle.go` | **Shipped 2026-05-18** |
| 1.2 | Add `VerifyHash` recomputation function | HIGH | 1h | `internal/types/types.go` | **Shipped 2026-05-18** (`VerifyNodeHash`, `VerifyEdgeHash` in `internal/types/verify.go`) |
| 6.1 | GC: prune unreachable nodes and edges | HIGH | 4h | `internal/snapshot/manager.go` | **Shipped 2026-05-18** (`GarbageCollectFull` with reachability sweep) |
| 6.3 | GC: hold write lock during GC | HIGH | 2h | `internal/daemon/daemon.go` | **Shipped 2026-05-18** |
| 8.1 | Add `knowing fsck` integrity checker | HIGH | 1 day | `internal/snapshot/manager.go`, new `cmd/` subcommand | **Shipped 2026-05-18** (`cmd/knowing/fsck.go`, `internal/snapshot/verify.go`) |
| 8.2 | Add `PRAGMA integrity_check` on startup | HIGH | 2h | `internal/store/sqlite.go` | **Shipped 2026-05-18** (`IntegrityCheck` method) |
| 9.1 | Add lockfile to prevent multiple daemon instances | HIGH | 2h | `internal/daemon/daemon.go` | **Shipped 2026-05-18** (`internal/daemon/lockfile.go`) |
| 3.1 | Make `extractPackagePath` return error on malformed names | HIGH | 2h | `internal/snapshot/manager.go` | **Shipped 2026-05-18** |
| 2.1 | Add `indexed_at` epoch for GC freshness signal | MED | 4h | `internal/store/migrations/` | **Shipped 2026-05-18** (migration 011) |
| 2.2 | Add in-process node/edge LRU cache | MED | 3h | `internal/store/sqlite.go` | **Shipped 2026-05-18** (50K-entry `sync.Map` on `GetNode`/`GetEdge`) |
| 4.1 | Add package filter and max-changes cap to `DiffHierarchicalTrees` | MED | 3h | `internal/snapshot/hierarchical.go` | **Shipped 2026-05-18** (`DiffHierarchicalTreesWithOptions` with `DiffOptions`) |
| 6.2 | Add `GarbageCollectStats` return type | MED | 2h | `internal/snapshot/manager.go` | **Shipped 2026-05-18** (`GCStats` return type on `GarbageCollectFull`) |
| 9.2 | Set `MaxOpenConns(1)` on SQLite connection | LOW | 1h | `internal/store/sqlite.go` | Open |
| 3.2 | Guard `DiffHierarchicalTrees` against nil trees | LOW | 1h | `internal/snapshot/hierarchical.go` | **Shipped 2026-05-18** |
| 3.3 | Add package-prefix filtering to diff | LOW | 2h | `internal/snapshot/hierarchical.go` | **Shipped 2026-05-18** (via `DiffOptions.PackageFilter`) |
| 4.2 | Add max-edges cap to `DiffMerkle` | LOW | 2h | `internal/snapshot/merkle.go` | **Shipped 2026-05-18** (via `DiffOptions.MaxChanges`) |
| 5.1 | Add `ReconstructEdgeSet` from event log | LOW | 1 week | `internal/snapshot/manager.go` | Open |
| 5.2 | Add incremental snapshot computation for changed files | LOW | 3h | `internal/snapshot/manager.go` | Open |
| 7.1 | Add named snapshot refs (`snapshot_refs` table) | LOW | 4h | `internal/types/interfaces.go`, new SQL migration | Open |
| 7.2 | Add reflog table for snapshot chain audit trail | LOW | 2h | new SQL migration | Open |
| 2.3 | Consider per-row column split for edge observations | LOW | 1 day | `internal/store/migrations/` | Open |
| 10.1 | Design Merkle-diff-based sync protocol | LOW | 2 weeks | new `internal/sync/` package | Open |
| 10.2 | Add `knowing export` / `knowing import` commands | LOW | 1 week | `cmd/`, `internal/wire/` | Open |

---

## Where Knowing Is Already Better Than Git

The following are areas where knowing's design is genuinely superior to git's, and
should not be regressed when adopting the recommendations above.

**NUL separator field encoding.** `ComputeNodeHash` and `ComputeEdgeHash` use NUL
bytes as field separators, preventing the "prefix ambiguity" attack
(`"a/b"+"c"` vs. `"a"+"b/c"` hashing the same). Git's object header relies on
fixed-meaning fields separated by a space and a NUL, which works for its five object
types but is not generalizable to arbitrary key-value schemas.

**Hierarchical Merkle tree for semantic diffs.** Git's tree diff works at the
filesystem path level. Knowing's `HierarchicalTree` works at the semantic level
(package, edge-type). This means `DiffHierarchicalTrees` can answer "did any call
edges in package X change?" in O(packages) rather than requiring a full leaf scan.
Git has no analog to this because its tree structure is tied to filesystem paths,
not code semantics.

**Provenance and confidence on edges.** Git stores blobs and trees with no
provenance metadata. Knowing's `Edge.Confidence` and `Edge.Provenance` fields
allow consumers to distinguish `ast_resolved` (high confidence) from `ast_inferred`
(lower confidence) edges and to upgrade them over time. This is an original design
contribution with no git analog.

**Schema-versioned migrations.** Git's on-disk format evolves via capability
negotiation and format flags, which is complex and hard to reason about.
Knowing's `internal/store/migrate.go` applies versioned SQL migrations in
order, with per-migration transactions and a `schema_version` table. This is a
clean, auditable schema evolution strategy.

**WAL mode as default.** Git uses file-level locking for all writes, which
serializes every operation. Knowing defaults to SQLite WAL mode, which allows
concurrent readers during a write. For a knowledge graph where reads outnumber
writes 100:1, this is the right default.

**Content-addressed file hash distinct from file path.** `File.ContentHash`
(`types.go:100`) is separate from `File.FileHash` (which incorporates the path).
This means skip-if-unchanged checks use the content hash, while identity uses the
path-inclusive hash. Git conflates these because its blob OID is purely
content-addressed with no path component, forcing callers to track path-to-OID
mappings separately in tree objects. Knowing's design is cleaner for
change-detection use cases.
