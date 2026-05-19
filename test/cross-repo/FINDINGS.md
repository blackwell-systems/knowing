# Cross-Repo Test Findings

Tested: 2026-05-19
Fixture: 3 Go modules (module-a shared library, module-b imports A, module-c imports A+B)

## What This Proves About the Architecture

### 1. Content-addressed identity works across repositories

Two independent indexers (one for module-a, one for module-b) produce matching
hashes for the same symbol without any communication between them. Module-b's
edge to `modulea.NewRegistry` has target hash `1e49ea8957f38632...`, which is
the exact hash stored in module-a's database. Global identity without
coordination: the core promise of content-addressing.

### 2. The hash formula is correct and sufficient

`SHA-256("node\0" + repoURL + "\0" + packagePath + "\0" + symbolName + "\0" + symbolKind)`
produces unique, deterministic, globally-agreeing hashes. The `"node\0"` domain
prefix, the repo URL component, and the package path component all contribute to
correctness. Removing any one would break cross-repo identity.

### 3. Canonicalization is the correctness boundary

The original failure proved this by counterexample: when the extractor used the
wrong repo URL (local instead of target), hashes diverged silently. The system
produced edges that looked correct but pointed to nonexistent targets. No runtime
error, no crash, just quiet incorrectness. Canonicalization bugs are the most
dangerous failure mode in a content-addressed system because they're invisible.

### 4. Per-repo isolation requires cross-repo identity infrastructure

Separate databases per repo is the right isolation model (community detection,
BM25, HITS all operate on scoped data). But isolation breaks cross-repo identity
unless the indexer can see ALL registered repos' URLs. The roster is that
infrastructure: it's the bridge between isolation and global identity.

### 5. The Merkle tree is repo-scoped, not org-scoped

Each repo has its own hierarchical Merkle tree and snapshot chain. Cross-repo
edges exist as hash references (like symlinks), not as entries in a shared tree.
This means: proofs are per-repo, fsck is per-repo, and a cross-repo proof
requires proving the edge in the source repo and then proving the target node
in the target repo. Federated sync (Phase 4) will formalize this.

## Test Results

### TEST 1: Indexing (PASS)
All 3 modules index successfully into separate databases.

| Module | Nodes | Edges | LSP upgraded | LSP discovered |
|--------|-------|-------|-------------|---------------|
| A (shared library) | 15 | 20 | 19 | 5 |
| B (imports A) | 6 | 16 | 15 | 1 |
| C (imports A+B) | 8 | 13 | 11 | 0 |

### TEST 2: Edge extraction (PASS)
Cross-repo call edges are extracted. Module-b has edges to module-a symbols.
Module-c has edges to both module-a and module-b symbols.

### TEST 3: Export filtering (EXPECTED BEHAVIOR)
`knowing export` filters edges where the target node is not in the exported
node set. Since cross-repo targets live in a different database, they are
excluded from the export. This is correct for single-repo export but means
cross-repo edges are invisible in the default export path.

### TEST 4: Cross-repo hash resolution (FAIL)

**Root cause:** The tree-sitter extractor computes target node hashes using
the LOCAL repo URL, not the TARGET repo URL. When module-b indexes a call to
`modulea.NewRegistry`, the target hash is:

```
SHA-256("node\0" + module-b-repo-url + "\0" + module-a-package + "\0" + NewRegistry + "\0" + function)
```

But the actual node in module-a's database has hash:

```
SHA-256("node\0" + module-a-repo-url + "\0" + module-a-package + "\0" + NewRegistry + "\0" + function)
```

These produce different hashes because the repo URL differs. The cross-repo
edge target `087cdc945a37ee50...` does not match module-a's actual node hash
`1e49ea8957f38632...`.

**Why this happens:** `ModuleToRepoURL` is populated from the repos table in
the same database. With per-repo isolation (separate databases per repo),
module-b's database has no entry for module-a's repo, so the mapping is empty.
The extractor falls back to using the local repo URL for all targets.

**Impact:** Cross-repo edges exist in the database but point to non-existent
hashes. The resolver cannot match them because the hashes are fundamentally
different. `knowing prove` across repos would fail because the target hash
in module-b doesn't correspond to any real node.

### TEST 5-8: Not tested (blocked by TEST 4)

The following tests are blocked by the cross-repo hash resolution failure:
- `knowing prove` across repos
- `knowing prove-absent` across repos
- `knowing audit` with cross-repo edges
- Incremental invalidation across repos

## Fix Required

The extractor needs access to a cross-database repo mapping so it can compute
target hashes with the correct repo URL. Options:

1. **Shared roster lookup at extraction time.** The extractor reads the global
   roster (or a shared index DB) to map Go module paths to stored repo URLs,
   even when indexing into a per-repo database. This is the simplest fix:
   populate `ModuleToRepoURL` from the roster, not from the local repos table.

2. **Post-index resolution pass.** After indexing all repos, run a cross-database
   resolver that retargets dangling edges using node lookups across all roster
   databases. This is the existing resolver design but adapted for per-repo isolation.

3. **Shared cross-repo database.** A separate `cross-repo.db` that stores only
   repo records and node identity mappings. Each per-repo database reads from
   it to populate `ModuleToRepoURL`.

**Recommended:** Option 1 (roster-based module mapping) is the smallest change
and directly addresses the root cause. The roster already knows every registered
repo URL; the extractor just needs to read it.

## What Works Today

- Single-repo indexing, proofs, audit, fsck: fully functional
- Per-repo isolation: each repo gets its own database, community detection is scoped
- Edge extraction: cross-repo calls ARE extracted (correct source, wrong target hash)
- LSP enrichment: works per-repo (19/19 upgraded in module-a, 15/15 in module-b)

## Fix Applied: Roster-Based Module Mapping

**Commit:** Moved roster to `internal/roster` shared package. Indexer's
`buildModuleToRepoMap` now merges the global roster's module map on top of
the local database's repos. This gives the extractor access to all registered
repo URLs, even when indexing into a per-repo database.

### Results After Fix

| Source | Target DB | Resolved | Dangling |
|--------|-----------|----------|----------|
| Module B edges | Module A nodes | **5** (ValidateNonEmpty, NewRegistry, NewEntity, Deduplicate, Normalize) | 13 |
| Module C edges | Module A nodes | **5** | 8 |
| Module C edges | Module B nodes | **1** | - |

Cross-repo edges now resolve correctly. The 13 dangling edges in module-b are
self-references (method calls on types within the same package) where the
tree-sitter extractor generates target hashes that don't match any node. This
is a separate extractor accuracy issue, not a cross-repo identity issue.

## Audit Tooling Results

### knowing fsck on module-b

```
16 errors (dangling edges), 4 warnings (hash mismatches)
```

**Dangling edges (16):** Expected. Cross-repo edge targets live in module-a's
database, not module-b's. Per-repo fsck correctly flags these as dangling
because it only sees one database. A cross-repo fsck would need to query
across databases to distinguish "dangling because cross-repo" from "dangling
because corrupted."

**Hash mismatches (4):** 4 of 6 nodes have stored hashes that don't match
recomputed hashes. This indicates the node fields were modified after the
initial hash computation (likely by LSP enrichment adding signature/doc data).
This is a real correctness issue: node hashes should be recomputed after
enrichment, or enrichment should not modify fields that affect the hash.

### What the audit tooling proves

1. **fsck detects cross-repo dangling edges.** The tool correctly identifies
   that edges point to nonexistent nodes. In a single-repo context, this is
   corruption. In a cross-repo context, this is expected. Future work: teach
   fsck about the roster so it can distinguish the two cases.

2. **fsck detects hash integrity issues.** The 4 hash mismatches are a real
   finding that we would not have caught without running fsck on the fixture.
   The enrichment pipeline modifies node fields after hash computation.

3. **The audit tooling works on the fixture.** knowing fsck runs, produces
   structured output, and correctly classifies issues as ERROR or WARN.

## Remaining Work

- Export path still filters cross-repo edges (target not in exported node set)
- `knowing prove` across repos needs multi-database proof generation
- `knowing audit` needs to query across databases for a combined report
- knowing-viz needs a combined export to visualize cross-repo topology
- fsck needs roster awareness to distinguish cross-repo dangling from corruption
- Hash recomputation after LSP enrichment (4 node hash mismatches)
