# P0 Fix Verification

Tested: 2026-05-19

## P0 #1: Removed-Edge Diffs (FIXED, VERIFIED)

**Test:** Index module-a, add a function, re-index. Check that SnapshotDiff
returns removed edges.

**Result:** `knowing diff` shows "Edges Removed (10)" with full edge data
(source, target, edge type, confidence). Previously this returned 0 removed
edges because the JOIN to deleted edges returned nothing.

**Verification:**
- 10 removed edge events with `has_full_data=True`
- Migration 013 adds source_hash, target_hash, edge_type, confidence, provenance
  to edge_events table
- RecordEdgeEvent stores full edge data for both added and removed events
- SnapshotDiff uses COALESCE to read from event first, falling back to edges
  table for pre-migration events

**Verdict: PASS.** The "what relationships were removed?" audit claim is now correct.

## P0 #2: Synthetic File Nodes (FIXED, VERIFIED)

**Test:** Index module-a, check for file nodes and dangling import sources.

**Result:** 1 file node stored (helpers.go), 0 dangling import edge sources.
Previously 0 file nodes were stored and 4 import sources were dangling.

**Verification:**
- Go tree-sitter extractor creates file node (kind="file") when import edges exist
- File node hash matches the hash used as import edge source
- `knowing fsck` no longer reports import edge sources as dangling

**Verdict: PASS.** Import edges now have valid source nodes.

## Stdlib Classification (RESOLVED via phantom external nodes)

**Previous issue:** `knowing fsck` on module-a reported 18 "truly dangling"
errors for stdlib references (sha256.Sum256, hex.EncodeToString, strings.Join,
etc.). The stdlib heuristic only caught `throws` and `imports` edge types, not
`calls` to stdlib functions.

**Resolution:** Phantom external nodes eliminate all dangling edges. When the
Go tree-sitter extractor encounters a call to an inferred stdlib or external
target, it creates a `kind="external"` node with `file_hash=EmptyHash` as the
edge target. The LSP enricher runs a post-enrichment sweep and creates phantom
nodes for any remaining dangling targets. Both paths produce the same result:
every edge has a valid target node in the database.

**Current state on module-a:**
- Zero dangling targets (0 errors)
- Zero dangling sources (0 errors)
- 25 phantom external nodes (stdlib and external call targets)
- 1 warning: file node hash recomputation mismatch (cosmetic; stored before enrichment)

**Verdict: RESOLVED.** The graph is complete. `knowing fsck` reports 0 errors.
