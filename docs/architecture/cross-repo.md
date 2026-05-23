# Cross-Repo Architecture

knowing tracks multiple repositories simultaneously and connects edges across them using content-addressed identity. This document describes the isolation model, the identity scheme, the roster infrastructure, and the current limitations.

## Per-Repo Isolation Model

Each indexed repository gets its own SQLite database at `~/.knowing/repos/<safe-name>.db`. Community detection, Random Walk with Restart (RWR), HITS authority/hub scoring, and BM25 full-text search are all scoped to a single repo's database. This prevents cross-repo noise from skewing relevance scores and allows databases to be backed up, shared, or deleted independently.

Cross-repo edges are not stored in a shared database. Instead, they are hash references: an edge in repo A whose `target_hash` points to a node that lives in repo B's database. From repo A's perspective, these are "dangling edges" whose target does not exist locally. This is by design.

## Cross-Repo Identity

Content-addressing provides global symbol identity without any coordination between indexers. The node hash formula is:

```
SHA-256("node\0" + repoURL + "\0" + packagePath + "\0" + symbolName + "\0" + symbolKind)
```

Two indexers that have never communicated will produce the same hash for the same symbol, as long as they use the same canonical repo URL. A knowing instance indexing `github.com/org/lib` produces hashes that match hashes produced by a separate instance indexing `github.com/org/consumer` when that consumer imports lib's symbols.

The `"node\0"` domain prefix ensures node hashes are structurally distinguishable from edge hashes, snapshot hashes, and Merkle interior node hashes. Removing any component of the formula (the repo URL, the package path, the symbol name, or the kind) would break cross-repo identity silently: edges would point to nonexistent targets with no runtime error.

This was proven by the cross-repo fixture test. See [test/cross-repo/FINDINGS.md](../../test/cross-repo/FINDINGS.md) for the full record.

## Roster

`internal/roster/roster.go` implements the global registry stored at `~/.knowing/roster.json`. The roster maps every registered repo path to its database path and canonical URL.

Key methods:

- `Add(repoPath, dbPath, repoURL)`: register a new repo.
- `List()`: return all registered entries.
- `ModuleMap()`: return a map from Go module paths to repo URLs for all registered repos. This is the bridge between Go's module import paths and knowing's canonical repo URLs.

The roster is the only global state in an otherwise per-repo-isolated system. It is the minimum coordination required for cross-repo identity to work.

## Module Mapping

When the Go tree-sitter extractor encounters an import of a foreign package, it needs to compute a target node hash using the correct repo URL for that package. The correct URL is not the local repo's URL; it is the URL of the repo that owns the package.

The indexer's `buildModuleToRepoMap` function merges two sources:

1. The local database's `repos` table (for repos already in scope).
2. The global roster's `ModuleMap()` (for all other registered repos).

The merged map is passed to the extractor so it can compute target hashes with the correct repo URL at extraction time. Without the roster, `ModuleToRepoURL` is empty for foreign packages and the extractor falls back to the local repo URL, producing hashes that will never match any real node.

## Cross-Repo Edge Resolution

When both repos are indexed and the roster is populated, cross-repo edges resolve correctly. The resolution pipeline (`internal/resolver/`) operates post-indexing:

1. `DanglingEdges()` finds all edges whose `target_hash` has no matching node in the local database.
2. For each dangling edge, the resolver queries other registered repos' databases for a matching node hash.
3. If found, the edge is confirmed resolved. If not found (and the target is not a known stdlib or external symbol), the edge is flagged as a potential canonicalization error.

Correct extraction (via roster-based module mapping) means most cross-repo edges are already correct before resolution runs. The resolver serves as a verification pass and handles edge cases.

## Phantom External Nodes

Edges that target stdlib symbols (like `fmt`, `strings`, `error`) or external packages that are not registered in the roster have no matching node in any database. Rather than leaving these edges dangling, knowing creates phantom nodes with `kind="external"`. These are created in two places:

- The Go tree-sitter extractor creates phantom nodes at extraction time for inferred stdlib or external targets.
- The LSP enricher runs a post-enrichment sweep to create phantom nodes for any remaining dangling edge targets.

Phantom nodes make the graph complete. `knowing fsck` distinguishes phantom-node edges (benign) from truly dangling edges (potential corruption) using roster data.

**Retrieval filtering:** Phantom external nodes are filtered from context retrieval results at two points: `filterNoisySymbols` (seed candidates) and the RWR result loop (before scoring). Without this filtering, repos with many unresolved LSP imports (e.g., Java projects without JDT) would have phantom nodes dominate the top-10 results via high RWR scores from inbound edge density.

## Limitations

**Language support:** All 6 language extractors now implement cross-repo identity:
- **Go**: `inferRepoURL` uses the roster's `ModuleMap()` for full module path resolution to canonical repo URLs.
- **Python, TypeScript, Rust, Java, C#**: `inferExternalRepoURL` detects external packages and computes target hashes with `"external://{packageName}"` prefix (e.g., `external://flask`, `external://express`) or `"stdlib"` for language standard libraries.

The Go extractor provides the strongest resolution (full module paths via roster). The other 5 provide package-level identity without registry lookups: all imports of `flask` across any repo produce the same `external://flask` prefix, enabling cross-repo caller discovery for third-party dependencies.

**Cross-repo proofs:** Generating a Merkle proof that spans two repos requires querying two databases: prove the edge exists in the source repo's database, then prove the target node exists in the target repo's database. `knowing prove` currently operates on a single database. Multi-database proof generation is planned.

**Cross-repo fsck:** `knowing fsck` on a single repo correctly flags cross-repo dangling edges as such (using roster data to classify them). A combined audit across all repos requires querying every registered database.

**Federated sync:** Exchanging edges between separate knowing instances on different machines is planned (Phase 4 roadmap). The content-addressed identity scheme means the hashes will agree once both instances index the same repos with the same canonical URLs; the missing piece is a sync protocol.

## Architectural Proofs

The cross-repo fixture test (`test/cross-repo/`) proved four properties about this architecture. See [FINDINGS.md](../../test/cross-repo/FINDINGS.md) for the full test record and results.

**Content-addressed identity works across repos.** Two independent indexers produced matching hashes for the same symbol without any communication. Module-b's edge to `modulea.NewRegistry` had the exact target hash stored in module-a's database.

**Canonicalization is the correctness boundary.** The original failure proved this by counterexample: when the extractor used the wrong repo URL (local instead of target), hashes diverged silently. No runtime error, no crash, just quiet incorrectness. Canonicalization bugs are the most dangerous failure mode in a content-addressed system because they are invisible.

**Per-repo isolation requires cross-repo identity infrastructure.** Separate databases per repo is the right isolation model for scoring and search. But isolation breaks cross-repo identity unless the indexer can see all registered repos' URLs. The roster is that infrastructure: the bridge between isolation and global identity.

**The Merkle tree is repo-scoped, not org-scoped.** Each repo has its own hierarchical Merkle tree and snapshot chain. Cross-repo edges exist as hash references, not as entries in a shared tree. A cross-repo proof requires proving the edge in the source repo and then proving the target node in the target repo.
