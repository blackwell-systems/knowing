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

## Remaining: Stdlib Classification (INCOMPLETE)

**Issue:** `knowing fsck` on module-a (no cross-repo deps) reports 18 "truly
dangling" errors. These are ALL stdlib references (calls to sha256.Sum256,
hex.EncodeToString, strings.Join, etc.). The stdlib heuristic only catches
`throws` and `imports` edge types, not `calls` to stdlib functions.

**Root cause:** Dangling edges only store a target hash. We can't reverse the
hash to determine what package it targets. Without the target node (it's never
indexed for stdlib), classification relies on heuristics that are incomplete
for `calls` edges.

**Options:**
1. Store the target qualified name on the edge (requires schema change)
2. Accept that `calls` to stdlib are "truly dangling" and adjust messaging
3. Maintain a stdlib function hash table (pre-compute known stdlib hashes)

**Impact:** Cosmetic. The edges are correctly extracted, correctly dangling
(stdlib is never indexed), and correctly reportable. The classification just
isn't precise enough to label them as "stdlib" in the fsck output.

**Verdict: KNOWN LIMITATION.** Not a correctness issue; a UX issue.
