# Audit and Compliance

knowing is a cryptographically verifiable record of code relationships. Every relationship, every snapshot, and every derivation step is content-addressed with SHA-256 and linked in a hierarchical Merkle tree. This makes knowing an audit primitive: you can prove claims about code structure, verify them offline, and detect modification. The guarantees are tamper-evident relative to a trusted root hash (exactly git's property): an auditor who knows the snapshot root can verify any claim by recomputation. The root becomes unforgeable when anchored to a signed git commit or external witness log.

## What You Can Prove

| Claim | How knowing proves it |
|-------|----------------------|
| "Service A calls service B" | `knowing prove` generates a Merkle proof path from the specific edge to the snapshot root. `knowing verify` checks it without database access. |
| "Prove a dependency does NOT exist" | `knowing prove-absent` generates a cryptographic absence proof using adjacent sorted leaves. Verifiable offline. |
| "Generate a complete audit report" | `knowing audit -proofs` produces integrity check + edge inventory + Merkle proofs in one JSON file |
| "This graph reflects commit abc123" | Every snapshot records the git commit hash. The snapshot's Merkle root is deterministic: same source = same root on any machine. |
| "The graph has not been modified since the snapshot" | `knowing fsck` recomputes every node hash, edge hash, and Merkle root from source data. Any mutation changes a hash, which propagates to the root. |
| "This feedback was valid when recorded" | Feedback records store the SubgraphRoot (Merkle root of the symbol's package). When code changes, the root changes and old feedback becomes invisible. Provable temporal validity. |
| "This dependency appeared between Tuesday and Wednesday" | Walk the snapshot chain backwards. Diff adjacent pairs (O(packages) per diff). The first snapshot containing the edge is the answer, tied to a specific git commit. |
| "These two teams' codebases have no shared dependencies" | Each team runs knowing independently. Disjoint community Merkle roots prove non-overlap at the identity level. |
| "No new cross-service calls were introduced in this PR" | CI indexes both branches, diffs the snapshots, and checks for new edges crossing service boundaries. Deterministic indexing means CI produces the same snapshot as any developer. |

## Core Properties

### Determinism

Same source code + same analyzer version = same snapshot hash. On any machine, at any time, by any operator. This is not a heuristic: it is a structural consequence of content-addressing. If two independent indexers produce different hashes for the same source, the canonicalization layer has a bug.

### Tamper Evidence

Every entity's hash is computed from its content:

```
NodeHash = SHA-256("node\0" + repoURL + package + name + kind)
EdgeHash = SHA-256("edge\0" + source + target + type + provenance)
SnapshotHash = HierarchicalMerkleRoot(all edge hashes, grouped by package and type)
```

Changing any field changes the hash. Changing any hash changes the parent. Changing any parent changes the root. An auditor who knows the root can verify any claim by recomputation.

Note: edge confidence scores are mutable metadata on immutable edge identity (not included in EdgeHash). Tampering with confidence does not break referential integrity but may degrade ranking quality. The integrity guarantee covers edge existence and provenance, not confidence scores.

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

For a single edge, use `knowing prove`:

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

For a full compliance artifact covering all cross-package edges, use `knowing audit -proofs` (see Workflow 6).

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
# Time: 98ms on a graph with 2,611 nodes and 13,103 edges
```

Exit code 0 = clean. Exit code 1 = corruption detected.

### 3. Change Attribution

"When did the dependency between billing and payments first appear?"

```bash
# Diff the last two snapshots using named refs
knowing diff @prev @latest

# Or diff specific snapshots by offset
knowing diff @3 @latest

# The diff shows which packages changed, which edge types changed,
# and which specific edges were added or removed.
# Each snapshot records the git commit that produced it.
```

Named refs: `@latest`, `@prev`, `@first`, `@N` (offset from latest), or raw hex hash.

For systematic bisection (finding the exact commit), walk the snapshot chain and diff adjacent pairs. Each diff is O(packages), not O(edges). Snapshots carry generation numbers (parent.generation + 1) enabling O(1) ancestry checks without walking the full chain.

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

For a complete artifact that includes the integrity check, full edge inventory, and proofs in one file, use `knowing audit -proofs` (see Workflow 6).

The snapshot hash is the audit artifact. It's 64 hex characters that uniquely identify the entire graph state. Two parties who agree on the hash agree on every relationship in the graph.

### 6. Batch Compliance Report

"Generate a single JSON artifact with the full audit picture: integrity, edge inventory, and proofs for all cross-package relationships."

```bash
# Generate a full compliance report with proofs for all cross-package edges
knowing audit -proofs -o audit-$(date +%Y-%m-%d).json

# The report includes:
#   - Integrity verification (fsck)
#   - Graph summary (nodes, edges, packages, edge types)
#   - All cross-package edges with provenance and confidence
#   - Merkle proofs for each cross-package relationship
#   - Snapshot hash tied to git commit

# Compare quarterly audit points (named refs or hex hashes)
knowing audit-diff @prev @latest -o latest-changes.json
knowing audit-diff $Q1_SNAPSHOT $Q2_SNAPSHOT -o q1-q2-changes.json
```

The diff report shows added and removed edge counts, classified by change type (`behavioral`, `structural`, `runtime_drift`, or `metadata_only`), making it straightforward to demonstrate what changed between audit periods.

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
| Graph completeness | Every edge has both endpoints; `knowing fsck` reports 0 errors | No verification mechanism |

### Graph Completeness

knowing's graph is provably complete: every edge has both a source node and a target node. This is enforced by phantom external nodes, which represent stdlib and external symbols that are never indexed. The Go tree-sitter extractor creates phantom nodes at extraction time for inferred stdlib targets. The LSP enricher creates phantom nodes in a post-enrichment sweep for any remaining dangling targets.

The result: `knowing fsck` on a correctly indexed repo reports 0 errors. This is a property no other code intelligence tool can state, because no other tool has a mechanism to verify it. Tools that do not run an integrity check cannot know whether their graph has dangling references; tools that do run an integrity check and suppress errors are accepting corruption silently. knowing's `fsck` is the verification layer, and 0 errors is the passing bar.

## Performance

| Operation | Latency | What it verifies |
|-----------|---------|-----------------|
| Proof generation | 72us | One specific edge exists in a snapshot |
| Proof verification | 1.2us | The proof is cryptographically valid |
| `knowing fsck` | 98ms | Entire graph integrity (2,611 nodes, 13,103 edges) |
| Snapshot diff | 6us | Which packages changed between two snapshots |
| Snapshot chain walk | O(N) snapshots, O(packages) per diff | When a relationship first appeared |

All operations are fast enough for interactive use, CI pipelines, and batch audit workflows.

## Limitations

- **Proof of absence is supported.** Use `knowing prove-absent` to generate a cryptographic proof that an edge does NOT exist. The proof uses adjacent sorted leaves from the Merkle tree; both neighbor inclusion proofs are verifiable offline against the same root.
- **Cross-snapshot proofs require one proof per snapshot.** To prove a relationship persisted from Tuesday to Friday, you need proofs from both snapshots.
- **Proofs verify identity, not semantic correctness.** A proof confirms the edge hash is in the tree. It does not confirm the extractor correctly identified the relationship in the source code. Correctness depends on the extraction pipeline.
- **Analyzer version affects determinism.** Different extractor versions may produce different edge sets from the same source. Snapshot determinism holds only when the analyzer version is fixed.
