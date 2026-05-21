# Data Flow

This section traces a single change from developer commit to fully-enriched graph state.

## End-to-End: One Commit, One Graph Update

```
Developer commits code
        │
        ▼
┌───────────────────────────────────────────────────────┐
│ 1. GitWatcher detects .git/HEAD change (fsnotify)      │
│    ├── Debounce timer fires after 500ms of quiet       │
│    ├── Read new HEAD commit hash from .git/HEAD        │
│    ├── Compare to last known commit (stored in repos)  │
│    └── If different: resolve file diff via git         │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ 2. GitDiffFiles resolves changed/added/deleted files   │
│    ├── Runs: git diff --name-status oldCommit newCommit│
│    ├── Parses status codes: M (modified), A (added),   │
│    │   D (deleted), R (renamed → delete old + add new) │
│    └── Returns three slices: changed, added, deleted   │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ 3. CommitEvent sent to watchLoop via GitWatcher.events │
│    ├── watchLoop combines changed + added + deleted    │
│    │   into a single indexRequest                      │
│    └── Sends indexRequest to indexCh (non-blocking)    │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ 4. indexWorker receives indexRequest from indexCh       │
│    ├── Resolves HEAD commit hash                       │
│    └── Acquires daemon write lock (d.mu.Lock())        │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ 5. IndexFunc runs (write lock held)                    │
│                                                       │
│  For deleted files:                                    │
│    ├── EdgesBySourceFile() to capture "removed" set    │
│    ├── DeleteEdgesBySourceFile()                       │
│    ├── DeleteNodesByFile()                             │
│    └── Record "removed" edge events                    │
│                                                       │
│  For changed files:                                    │
│    ├── Delete old nodes/edges (same as deleted)        │
│    ├── Re-extract via tree-sitter worker pool          │
│    ├── Compute edge diff (old vs. new)                 │
│    └── Record "added" and "removed" edge events        │
│                                                       │
│  For added files:                                      │
│    ├── Extract via tree-sitter worker pool             │
│    └── Record "added" edge events                      │
│                                                       │
│  Batch insert all new nodes, edges, and files          │
│  Compute new snapshot (hierarchical Merkle tree:       │
│    repo root -> package roots -> edge-type roots)      │
│  Link snapshot to parent; store commit hash            │
│  Resolve cross-repo dangling edges                     │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ 6. Release write lock (d.mu.Unlock())                  │
│    Graph is now queryable with ast_inferred edges.     │
│    MCP queries resume immediately.                     │
└───────────────────────────┬───────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────────┐
│ 7. Trigger scoped LSP enrichment (background goroutine)│
│    No write lock held; enrichment uses SQLite WAL mode │
│                                                       │
│    ├── For each detected language server (gopls,       │
│    │   pyright, tsserver, rust-analyzer, jdtls, etc.): │
│    ├── Open changed/added files (textDocument/didOpen) │
│    ├── Edge upgrade pass:                              │
│    │   For each ast_inferred edge in changed files:    │
│    │     Query GetDefinition at call-site position     │
│    │     If confirmed: delete old edge, insert         │
│    │     lsp_resolved edge (confidence 0.9)            │
│    ├── Edge discovery pass:                            │
│    │   For each changed file:                          │
│    │     GetDocumentSymbols                            │
│    │     For types: GetImplementation → implements     │
│    │     For funcs: GetReferences → references         │
│    ├── Close all files                                 │
│    └── Shutdown language server, repeat for next       │
└───────────────────────────────────────────────────────┘
```

## Timing Summary

| Phase | Duration (6,000-node repo) | Lock held | Queries blocked |
|-------|---------------------------|-----------|-----------------|
| Git diff resolution | ~10ms | None | No |
| Tier 1 extraction (tree-sitter, parallel) | ~1.8s | Write lock | Yes |
| Snapshot computation (hierarchical Merkle tree) | ~5ms | Write lock | Yes |
| Tier 2 enrichment (LSP) | ~8s | None (WAL) | No (background) |

The write lock is held only during Tier 1 extraction and snapshot computation. Queries are blocked for approximately 1.5 seconds per commit. Enrichment runs in the background without blocking anything.
