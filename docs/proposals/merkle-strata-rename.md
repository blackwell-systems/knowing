# Proposal: Rename merkle-forest to merkle-strata

## Summary

Rename the `merkle-forest` library to `merkle-strata` to accurately reflect the data structure it implements.

## Motivation

"Forest" in graph theory means a collection of disconnected trees. The library implements a single connected tree with semantic layering: one root, intermediate roots at package and edge-type boundaries, and leaf edges at the bottom. This is a stratified tree, not a forest.

The current name causes a terminology mismatch between what the library claims to be and what it actually is. The data structure has distinct layers (strata) of Merkle hashes organized by semantic meaning:

```
Root (repo-level)
  └── Package roots (one per package)
       └── Edge-type roots (one per package+type)
            └── Leaf edges (individual relationship hashes)
```

Each layer represents something different: packages are organizational boundaries, edge types are relationship categories, leaves are individual facts. "Strata" captures this precisely: distinct horizontal layers of the same material at different semantic depths.

## Name Availability

Verified available across all registries:

| Registry | Status |
|----------|--------|
| GitHub (`blackwell-systems/merkle-strata`) | Available |
| pkg.go.dev (`github.com/blackwell-systems/merkle-strata`) | Available |
| npm (`merkle-strata`) | Available |
| PyPI (`merkle-strata`) | Available |
| crates.io (`merkle-strata`) | Not checked (Go library, not applicable) |

No existing GitHub repos named `merkle-strata`.

## Scope of Changes

### External (public-facing)

1. Create new repo `blackwell-systems/merkle-strata`
2. Archive `blackwell-systems/merkle-forest` with a redirect notice in README
3. Publish to pkg.go.dev under new import path
4. Update GitHub topics and description
5. Transfer banner/assets to new repo

### Internal (knowing)

1. Update `go.mod`: `github.com/blackwell-systems/merkle-forest` → `github.com/blackwell-systems/merkle-strata`
2. Update all import statements in `internal/snapshot/hierarchical.go`
3. Update `forestPrefix` variable name → `strataPrefix` (or keep as implementation detail)
4. Update documentation references (architecture.md, whitepaper, README)
5. Update blog OSS page entry

### Package API

No API changes. The exported types and functions remain identical:

- `Build(groups)` → `Build(groups)`
- `BuildMultiLevel(inputs)` → `BuildMultiLevel(inputs)`
- `Prove(group, leaf)` → `Prove(group, leaf)`
- `ProveAbsent(group, leaf)` → `ProveAbsent(group, leaf)`
- `Verify(proof, root)` → `Verify(proof, root)`
- `Diff(old, new)` → `Diff(old, new)`
- `SubRoot(groups)` → `SubRoot(groups)`

Only the import path changes. Zero breaking changes to the API surface.

### Package name in Go source

The Go package declaration changes from:

```go
package merkleforest
```

to:

```go
package merklestrata
```

Callers change from `forest.Build(...)` to `strata.Build(...)` (or whatever import alias they use).

## Migration Path

1. Publish `merkle-strata` v0.1.0 with identical code
2. Update knowing to use `merkle-strata`
3. Add deprecation notice to `merkle-forest` README: "This package has been renamed to merkle-strata. All future development happens there."
4. Keep `merkle-forest` archived (don't delete, pkg.go.dev references persist)

## Risks

- **Existing users**: The library has 2 stars. No known external consumers besides knowing. Risk is minimal.
- **pkg.go.dev caching**: Old import path will still resolve to the archived repo. Go module proxy caches indefinitely. Not a problem for new users.
- **Whitepaper references**: The published paper (DOI: 10.5281/zenodo.20342255) references `merkle-forest` by the current name. A v2 upload can update this, or the new README can note the rename.

## Decision

Rename. The precision is worth the one-time migration cost. The library is young enough that no significant ecosystem disruption occurs.

## Timeline

Execute in one session:
1. Create new repo
2. Copy code with updated package name
3. Push, tag v0.1.0
4. Update knowing's go.mod and imports
5. Archive old repo
6. Update docs and blog
