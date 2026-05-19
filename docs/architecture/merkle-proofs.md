# Merkle Proofs

Merkle proofs let you prove that a specific relationship exists in a knowing snapshot without sharing the full graph. A proof is a compact JSON object (~3KB) that anyone can verify offline with a few SHA-256 computations.

## Why

Three use cases:

1. **Audit and compliance.** "Prove that service A called service B at the snapshot we deployed on Tuesday." The proof is cryptographic: if the edge existed, the proof verifies. If it didn't, no valid proof can be constructed.

2. **CI gates.** "This PR introduces a cross-repo dependency. Prove it." A CI step generates the proof and attaches it to the PR. Reviewers verify without database access.

3. **Federated trust.** Two knowing instances exchange proofs about shared edges. Each side verifies against its own snapshot root. Agreement on the root means agreement on the graph.

## How It Works

The proof follows the hierarchical Merkle tree from a specific edge up to the repo root. At each level, the verifier needs the target hash and its sibling to recompute the parent.

```
Level 1: edge hash -> edge-type root
  (sibling edge hashes within the same package + edge type)

Level 2: edge-type root -> package root
  (sibling edge-type roots within the same package)

Level 3: package root -> repo root
  (sibling package roots)
```

Each level is a standard binary Merkle tree. The proof records the sibling hash and whether it's on the left or right at each step.

## Proof Format

```json
{
  "source": "ForTask",
  "target": "ComputeHITS",
  "edge_type": "calls",
  "snapshot_hash": "f479567d...",
  "proof": {
    "edge_hash": "6ea630b9...",
    "package_path": "github.com/blackwell-systems/knowing/internal/context",
    "edge_type": "calls",
    "edge_to_edge_type_root": [
      {"sibling": "6e917a7f...", "is_left": true},
      {"sibling": "c5ad6cbb...", "is_left": false}
    ],
    "edge_type_root": "420eb578...",
    "edge_type_to_package_root": [
      {"sibling": "a1b2c3d4...", "is_left": false}
    ],
    "package_root": "1c5c84e7...",
    "package_to_repo_root": [
      {"sibling": "de194914...", "is_left": true},
      {"sibling": "b759198e...", "is_left": false}
    ],
    "repo_root": "1cc3b4c6..."
  }
}
```

### Fields

| Field | What it is |
|-------|-----------|
| `edge_hash` | The content-addressed hash of the edge being proved |
| `package_path` | The source symbol's package |
| `edge_type` | The relationship type (`calls`, `imports`, etc.) |
| `edge_to_edge_type_root` | Binary proof steps from the edge to its edge-type Merkle root |
| `edge_type_root` | The Merkle root of all edges of this type in this package |
| `edge_type_to_package_root` | Binary proof steps from the edge-type root to the package root |
| `package_root` | The Merkle root of all edge-type roots in this package |
| `package_to_repo_root` | Binary proof steps from the package root to the repo root |
| `repo_root` | The Merkle root of the entire repository snapshot |

### Proof Steps

Each step contains:
- `sibling`: the hash of the node paired with the target at this tree level
- `is_left`: whether the sibling is on the left (target is right) or right (target is left)

The verifier combines the target with the sibling using `ComputeMerkleNodeHash(left, right)` (which applies the `"merkle\0"` domain prefix) and uses the result as the target for the next level.

## Verification

Verification is O(proof steps): typically 10-20 SHA-256 computations.

```
computed = edge_hash
for each step in edge_to_edge_type_root:
    if step.is_left:
        computed = hash("merkle\0" || step.sibling || computed)
    else:
        computed = hash("merkle\0" || computed || step.sibling)
assert computed == edge_type_root

(repeat for edge_type_to_package_root -> package_root)
(repeat for package_to_repo_root -> repo_root)
```

If all three levels match, the edge existed in the snapshot.

## Performance

Measured on the knowing live graph (12,604 edges, 115 packages):

| Metric | Value |
|--------|-------|
| Proof generation | 72us median |
| Proof verification | 1.2us median |
| Proof size | 16-18 steps, ~3KB JSON |
| Proof generation contract | < 10ms |
| Proof verification contract | < 100us |

## CLI

```bash
# Generate a proof
knowing prove -source "%ForTask" -target "%ComputeHITS" -type calls -o proof.json

# Verify offline (no database)
knowing verify proof.json
```

See [CLI reference](../guide/cli.md#prove) for full flag documentation.

## Implementation

- `internal/snapshot/proof.go`: `GenerateProof`, `VerifyProof`, `binaryProof`
- `internal/snapshot/proof_test.go`: 9 tests including tamper detection
- `bench/merkle-diff/proof_bench_test.go`: performance benchmark with contracts
- `cmd/knowing/prove.go`, `cmd/knowing/verify.go`: CLI commands

## What Proofs Do Not Cover

- **Proof of absence.** The current tree is not an ordered trie, so you cannot prove an edge does NOT exist. This requires a tree restructuring (Phase 4 roadmap).
- **Cross-snapshot proofs.** A proof is valid for one snapshot. To prove a relationship persisted across snapshots, you need proofs from each.
- **Semantic correctness.** A proof proves the edge hash existed in the tree, not that the edge correctly represents the source code. Correctness depends on the extractor that produced the edge.

## Relationship to Other Features

| Feature | How proofs interact |
|---------|-------------------|
| `knowing fsck` | Fsck verifies the entire graph; proofs verify one edge. Complementary. |
| Federated sync | Proofs are the trust primitive: "prove your graph includes this edge." |
| Bisection | Binary search uses proof verification at each step. |
| Context pack dedup | PackRoot is a different kind of proof: "this context selection is identical." |
