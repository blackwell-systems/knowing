# Audit and Compliance

knowing is a cryptographically verifiable record of code relationships. Every relationship, every snapshot, and every derivation step is content-addressed with SHA-256 and linked in a hierarchical Merkle tree. This makes knowing an audit primitive: you can prove claims about code structure, verify them offline, and detect tampering.

## What You Can Prove

| Claim | How knowing proves it |
|-------|----------------------|
| "Service A calls service B" | `knowing prove` generates a Merkle proof path from the specific edge to the snapshot root. `knowing verify` checks it without database access. |
| "This graph reflects commit abc123" | Every snapshot records the git commit hash. The snapshot's Merkle root is deterministic: same source = same root on any machine. |
| "The graph has not been tampered with" | `knowing fsck` recomputes every node hash, edge hash, and Merkle root from source data. Any mutation changes a hash, which propagates to the root. |
| "This dependency appeared between Tuesday and Wednesday" | Walk the snapshot chain backwards. Diff adjacent pairs (O(packages) per diff). The first snapshot containing the edge is the answer, tied to a specific git commit. |
| "These two teams' codebases have no shared dependencies" | Each team runs knowing independently. Disjoint community Merkle roots prove non-overlap at the identity level. |
| "No new cross-service calls were introduced in this PR" | CI indexes both branches, diffs the snapshots, and checks for new edges crossing service boundaries. Deterministic indexing means CI produces the same snapshot as any developer. |

## Core Properties

### Determinism

Same source code + same analyzer version = same snapshot hash. On any machine, at any time, by any operator. This is not a heuristic: it is a structural consequence of content-addressing. If two independent indexers produce different hashes for the same source, the canonicalization layer has a bug.

### Tamper Evidence

Every entity's hash is computed from its content:

```
NodeHash = SHA-256("node\0" + repo + package + name + kind)
EdgeHash = SHA-256("edge\0" + source + target + type + provenance)
SnapshotHash = HierarchicalMerkleRoot(all edge hashes, grouped by package and type)
```

Changing any field changes the hash. Changing any hash changes the parent. Changing any parent changes the root. An auditor who knows the root can verify any claim by recomputation.

### Provenance Chain

```
Git commit hash (stored on snapshot)
  -> Snapshot Merkle root (computed from all edges)
    -> Package roots (one per package)
      -> Edge-type roots (calls, imports, implements per package)
        -> Individual edge hashes
          -> Source/target node hashes
            -> Repo hash + file content hash
```

Every step is a deterministic computation from content. The data is the audit trail.

## Workflows

### 1. Point-in-Time Proof

"Prove that `AuthService.ValidateToken` calls `UserRepo.FindByID` at snapshot `abc123`."

```bash
# Generate the proof
knowing prove \
  -source "%ValidateToken" \
  -target "%FindByID" \
  -type calls \
  -o auth-calls-user.proof.json

# Share the proof file (3KB) with the auditor

# Auditor verifies offline (no database, no network)
knowing verify auth-calls-user.proof.json
# VERIFIED
#   Edge:      7a3b910...
#   Package:   github.com/org/repo/internal/auth
#   Edge type: calls
#   Repo root: f479567d...
#   Snapshot:  abc123...
#   Proof:     16 steps
```

The proof is 3KB of JSON. The auditor needs only the `knowing` binary (or any SHA-256 implementation that understands the `"merkle\0"` domain prefix).

### 2. Full Integrity Verification

"Verify that the graph database has not been modified since indexing."

```bash
knowing fsck
# Checks:
#   1. Edge referential integrity (every edge's source/target exists)
#   2. Hash recomputation (every stored hash matches recomputed hash)
#   3. Snapshot chain continuity (every parent pointer is valid)
#   4. SQLite page integrity (PRAGMA integrity_check)
#
# Time: 98ms on a graph with 2,338 nodes and 11,664 edges
```

Exit code 0 = clean. Exit code 1 = corruption detected.

### 3. Change Attribution

"When did the dependency between billing and payments first appear?"

```bash
# Diff two snapshots
knowing diff <old-snapshot-hash> <new-snapshot-hash>

# The diff shows which packages changed, which edge types changed,
# and which specific edges were added or removed.
# Each snapshot records the git commit that produced it.
```

For systematic bisection (finding the exact commit), walk the snapshot chain and diff adjacent pairs. Each diff is O(packages), not O(edges).

### 4. CI Gate

"Block PRs that introduce new cross-service dependencies without review."

```yaml
# .github/workflows/dependency-gate.yml
- name: Index base branch
  run: knowing index -repo $REPO_URL $BASE_PATH

- name: Index PR branch
  run: knowing index -repo $REPO_URL $HEAD_PATH

- name: Check for new cross-service edges
  run: knowing diff $BASE_SNAPSHOT $HEAD_SNAPSHOT | grep "cross_repo"
```

Because indexing is deterministic, the CI snapshot hash will match what any developer produces locally. There is no "CI indexed it differently" problem.

### 5. Compliance Snapshot Archive

"Keep a verifiable record of the service graph at each quarterly audit point."

```bash
# At audit time: index and record the snapshot
knowing index ./repo
SNAPSHOT=$(knowing query --latest-snapshot)

# Store the snapshot hash alongside the git commit
echo "$SNAPSHOT $COMMIT $DATE" >> audit-log.txt

# Years later: verify the graph hasn't changed
knowing fsck

# Or: prove a specific claim about the archived state
knowing prove -source "%PaymentService" -target "%StripeClient" -type calls
```

The snapshot hash is the audit artifact. It's 64 hex characters that uniquely identify the entire graph state. Two parties who agree on the hash agree on every relationship in the graph.

## What Sets This Apart

Most code intelligence tools have no audit story at all. They produce ephemeral indexes that are regenerated on demand. There is no history, no provenance, no verification, and no proof mechanism.

| Property | knowing | Typical code intelligence |
|----------|---------|--------------------------|
| Deterministic output | Same source = same hash, always | Non-deterministic or undefined |
| Tamper detection | Hash recomputation from content | None |
| Point-in-time proof | Merkle proof (72us, 3KB, offline) | Not possible |
| Full integrity check | `knowing fsck` (98ms) | Not available |
| History | Snapshot chain tied to git commits | Ephemeral; regenerated each session |
| Provenance | Every edge records how it was discovered | Metadata, if any |

## Performance

| Operation | Latency | What it verifies |
|-----------|---------|-----------------|
| Proof generation | 72us | One specific edge exists in a snapshot |
| Proof verification | 1.2us | The proof is cryptographically valid |
| `knowing fsck` | 98ms | Entire graph integrity (2,338 nodes, 11,664 edges) |
| Snapshot diff | 6us | Which packages changed between two snapshots |
| Snapshot chain walk | O(N) snapshots, O(packages) per diff | When a relationship first appeared |

All operations are fast enough for interactive use, CI pipelines, and batch audit workflows.

## Limitations

- **Proof of absence is not yet supported.** You can prove an edge exists, but you cannot yet prove an edge does NOT exist. This requires an ordered Merkle trie (Phase 4 roadmap).
- **Cross-snapshot proofs require one proof per snapshot.** To prove a relationship persisted from Tuesday to Friday, you need proofs from both snapshots.
- **Proofs verify identity, not semantic correctness.** A proof confirms the edge hash is in the tree. It does not confirm the extractor correctly identified the relationship in the source code. Correctness depends on the extraction pipeline.
- **Analyzer version affects determinism.** Different extractor versions may produce different edge sets from the same source. Snapshot determinism holds only when the analyzer version is fixed.
