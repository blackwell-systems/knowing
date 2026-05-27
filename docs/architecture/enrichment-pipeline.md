# LSP Enrichment

The enrichment system (`internal/enrichment/`) upgrades the knowledge graph by querying
language servers (LSP) for higher-confidence edge resolution and new edges that
tree-sitter extraction cannot discover. It runs automatically after indexing (unless
`-no-enrich` is passed) or standalone via `knowing enrich lsp`.

Enrichment creates three categories of graph improvements:

1. **Confidence upgrades**: existing `ast_inferred` edges (confidence 0.7) are upgraded
   to `lsp_resolved` (confidence 0.9) when GetDefinition confirms the call target.
2. **New edges**: `implements` and `references` edges discovered via GetImplementation
   and GetReferences that tree-sitter missed entirely.
3. **Phantom external nodes**: stub nodes (kind=`external`) created for stdlib and
   dependency types referenced by edges. These nodes have no source code but serve as
   graph connectors.

**Current status:** enrichment is worth +0.040 P@10 on Python repos (django+flask:
0.222 enriched vs 0.210 non-enriched). The value comes from phantom nodes and
`type_hint_of` edges creating new reachability paths, not from confidence upgrades
(which are neutral for RWR, since it weights by edge type, not confidence). See
[retrieval-pipeline.md](retrieval-pipeline.md) for how RWR uses edge weights.

---

## Why It Matters for Retrieval

Enrichment's retrieval value is indirect and was not always present. Session 13 measured
enrichment as neutral because `type_hint_of` edges did not exist yet. After `type_hint_of`
was added (session 14), phantom external nodes became reachable through those edges, and
enrichment started helping.

The mechanism: when two functions both reference the same external type (e.g.,
`http.Request`), enrichment creates a phantom node for that type. If `type_hint_of` edges
connect both functions to the phantom node, RWR can walk between them through the shared
type. This "shared-type reachability" creates paths that did not exist with tree-sitter
alone.

Confidence upgrades (0.7 to 0.9) are neutral for P@10 because RWR weights transitions by
edge type (`calls`=1.0, `imports`=0.5), not by confidence. The edges already exist;
upgrading their confidence does not change walk behavior.

Enrichment remains useful for non-retrieval purposes: blast radius confidence display,
dead code detection requiring high-confidence edges, and audit workflows that need
`lsp_resolved` provenance. See [context-engine.md](context-engine.md) for how confidence
is used in the scoring formula.

---

## Architecture

### Three Phases Per Language Server

The enrichment pipeline runs three phases for each detected language server, followed by
a phantom creation pass. Here is what each phase produces:

| Phase | What it does | Nodes created | Edges created/modified |
|-------|-------------|---------------|----------------------|
| 1. Workspace Readiness | Opens a file, waits for server to load | None | None |
| 2. Upgrade Call Edges | Confirms tree-sitter edges via GetDefinition | None (uses existing nodes) | Replaces `ast_inferred` with `lsp_resolved`. May retarget to a different node if LSP resolves differently than tree-sitter predicted. |
| 3. Discover New Edges | Finds implements + references via GetImplementation/GetReferences | None directly | New `lsp_resolved` edges for relationships tree-sitter missed (interface implementations, cross-package references) |
| Phantom Creation (after all phases) | Scans all edges, creates stub nodes for missing targets | Creates phantom external nodes for stdlib/dependency types | None (edges already exist; phantoms fill in missing endpoints) |

The key sequencing insight: **upgrades** (Phase 2) never create nodes; they only change
provenance and confidence on edges between nodes that already exist. **Discovery**
(Phase 3) creates new edges whose targets may point to locations with no corresponding
node record (e.g., a stdlib function or a type in an unindexed dependency). **Phantom
creation** then fills in those gaps by scanning every edge and creating stub
`external` nodes for any hash that has no node record in the database. This ordering
means discovery does not need to worry about whether target nodes exist yet; it inserts
edges freely, and the phantom pass cleans up afterward.

**Phase 1: Workspace Readiness**

Wait for the language server to finish indexing. The enricher sends
`textDocument/didOpen` for a probe file, then retries `GetDefinition` with increasing
timeouts (5s, 10s, 30s, 60s, 120s) until the server responds. This readiness probe
prevents flooding the server with thousands of requests while it is still loading.

Files are NOT bulk-opened upfront. gopls reads from disk for workspace indexing, so
sending thousands of `didOpen` requests would flood stdin and waste memory (50MB+ for
large repos). Instead, files are opened in batches of 50 during the discovery phase.

**Phase 2: Upgrade Call Edges**

For each `ast_inferred` edge with call-site position data (file, line, column):

1. Query `GetDefinition` at the call site.
2. If the server returns a location, resolve it to a known node in the database
   (matching by file path and line number, within 2-line tolerance).
3. Delete the original `ast_inferred` edge (provenance is part of the edge hash).
4. Insert a new `lsp_resolved` edge with confidence 0.9, potentially retargeted if
   LSP resolved to a different node than tree-sitter predicted.

Edges that already have an `lsp_resolved` counterpart are skipped. LSP calls run
concurrently (128 workers post-warmup); DB writes are serialized through a single
writer goroutine to avoid SQLite lock contention.

**Phase 3: Discover New Edges**

For each source file (processed in batches of 50):

1. Open the file via `textDocument/didOpen` (required for `GetDocumentSymbols`).
2. Query `GetDocumentSymbols` to enumerate types, interfaces, functions, and methods.
3. For types and interfaces: query `GetImplementation` to find `implements` edges.
4. For functions and methods: query `GetReferences` to find `references` edges.
5. Close the batch to release LSP server memory before opening the next batch.

New edges are inserted as `lsp_resolved` with confidence 0.9. Source and target hashes
are computed from LSP URIs and positions, since not every location has a matching Node
record. Test files are skipped during discovery.

A position correction step (`resolveNamePosition`) handles language servers like pyright
that set `SelectionRange` to the keyword (`def`, `class`) rather than the identifier.
The enricher finds the symbol name on the declaration line and uses that column instead.

### Phantom Node Creation

After all phases complete, the enricher scans every edge in the repo and creates phantom
external nodes for any target or source hash that does not exist in the database. This
ensures every edge has both endpoints in the graph.

Phantom nodes have:
- `Kind`: `"external"`
- `FileHash`: `EmptyHash` (no backing file)
- `QualifiedName`: `"external://[edge_type].target"` or `"external://[edge_type].source"`

Phantom nodes enable shared-type reachability: functions referencing the same external
type (e.g., `io.Reader`, `HttpRequest`) become connected through the phantom node when
`type_hint_of` edges exist. See [edge-types.md](edge-types.md) for the full edge type
catalog.

Counterintuitively, keeping phantom nodes in the FTS index helps P@10. Removing them
degrades IDF distribution: without phantom nodes, common terms become artificially rare,
distorting BM25 scoring.

### Cross-Repo Definition Resolution

When `GetDefinition` returns a location outside the current workspace (e.g., a dependency
installed in `GOPATH` or `site-packages`), the enricher checks the global roster
(`internal/roster/`) to find which indexed repo owns that file. If found, it queries that
repo's database for a matching node, enabling cross-repo edge retargeting.

---

## Two-Phase Warmup for Slow Servers

gopls uses lazy package loading: it does not load packages until `didOpen` is sent for a
file in that package. Without warmup, all `GetDefinition` requests return "no package
metadata" immediately because no packages have been loaded.

The warmup protocol:

**Phase A (sequential, up to 300s):** Open one file via `didOpen` to trigger package
loading. Retry `GetDefinition` at a safe position (line 5) with 30s timeouts until the
server responds. This blocks until gopls has loaded at least one package and can serve
requests.

**Phase B (concurrent, 128 workers):** Once the server is warm, blast through all
remaining edges with high concurrency and 30s per-request timeout. The server is loaded,
so responses are fast (typically <100ms per request after warmup).

The warmup is necessary because gopls on large repos (terraform: 367 dependencies) needs
5+ minutes to load the dependency graph. Without the sequential warmup phase, all 128
workers would fire requests simultaneously, all would timeout, and the enrichment run
would produce zero upgrades.

---

## Multi-Module Go Support

For Go workspaces with `go.work`, the enricher spawns one gopls instance per module.
`DiscoverModules` parses `go.work` to find all module directories and reads each
module's `go.mod` for its module path.

Processing order:
1. The root module (workspace root, typically the largest) is processed first, solo, to
   limit peak memory.
2. Sub-modules are processed in parallel (up to 4 concurrent gopls instances). Sub-modules
   are typically small (200-500 files), so 4 simultaneous gopls instances use approximately
   800MB total (vs 1.2GB for the root alone).

Progress is tracked in `.knowing/enrich-progress.json` so interrupted runs can resume.
Each module's completion status (success or error) is persisted atomically after the
module finishes. On restart, already-completed modules are skipped.

---

## Supported Language Servers

Language servers are auto-detected from project markers in the workspace root. Detection
checks for marker files (e.g., `go.mod`, `package.json`) and verifies the server binary
is on `PATH`. Detection can be overridden via a `knowing-lsp.json` configuration file.

| Language | Server | Marker files | Notes |
|----------|--------|-------------|-------|
| Go | `gopls` | `go.mod` | Needs didOpen warmup for large repos; multi-module via `go.work` |
| Python | `pylsp` or `pyright-langserver` | `pyproject.toml`, `setup.py`, `requirements.txt` | pylsp preferred; pyright as fallback. Fast, no warmup needed |
| TypeScript | `typescript-language-server --stdio` | `tsconfig.json`, `package.json` | GC-bound on large repos; set `NODE_OPTIONS="--max-old-space-size=8192"` |
| Rust | `rust-analyzer` | `Cargo.toml` | Fast, no warmup needed |
| Java | `jdtls` | `pom.xml`, `build.gradle`, `build.gradle.kts` | Needs Gradle/Maven build first |
| C# | `OmniSharp --languageserver` or `csharp-ls` | `*.csproj`, `*.sln` | OmniSharp preferred; csharp-ls as fallback. Needs `DOTNET_ROOT` set for csharp-ls |

For C#, if neither OmniSharp nor csharp-ls is on `PATH`, the enricher also checks
`~/.dotnet/tools/csharp-ls` (dotnet tool install location).

---

## CLI Usage

Enrichment runs automatically during `knowing index` unless `-no-enrich` is passed. For
standalone enrichment on an already-indexed database:

```bash
# Run LSP enrichment (auto-detects language servers)
knowing enrich lsp <repo-path>

# With explicit database and concurrency
knowing enrich lsp -db /path/to/knowing.db -concurrency 16 <repo-path>

# With explicit repo URL (auto-detected from git remote if omitted)
knowing enrich lsp -url https://github.com/org/repo <repo-path>
```

The database must already contain nodes from a prior `knowing index` run. The enricher
verifies this before starting and exits with an error if the database is empty.

Other enrichment passes (non-LSP):
- `knowing enrich blame <repo-path>`: stamps `last_author` and `last_commit_at` on
  symbols via `git blame`.
- `knowing enrich coverage <repo-path>`: stamps coverage percentage on symbols from a
  Go cover profile.

---

## Per-Symbol Timeout

Each LSP call is wrapped with `WithSymbolTimeout` (default: 10 seconds). If a single
`GetDefinition`, `GetImplementation`, or `GetReferences` call exceeds the timeout, it is
cancelled without aborting the parent context. The enricher continues with the next symbol.
This prevents a single hung symbol from blocking the entire enrichment run.

Post-warmup edge upgrades use a 30-second timeout per request (longer than the default,
because definition resolution on cross-package symbols can be slow on large repos).

---

## Performance Characteristics

Enrichment time varies widely by language server performance and repo size:

| Repo | Language | Files | Time | Edges upgraded | New edges | Phantom nodes | Notes |
|------|----------|-------|------|---------------|-----------|--------------|-------|
| django | Python | 2,771 | ~10 min | - | - | 79K | pyright, fast |
| vscode | TypeScript | 3,958 | ~34 min | - | - | 468K | tsserver, GC-bound |
| cargo | Rust | 950 | ~1 min | - | - | 72K | rust-analyzer, fast |
| ocelot | C# | 768 | ~6 min | - | - | 10K | csharp-ls |
| terraform | Go | 2,242 | 12 min | 5,850 | 82,721 | 73K | gopls, two-phase warmup |
| kubernetes | Go | 2,956 | 58 min | 39,678 | 192,271 | 169K | gopls, 64 concurrent post-warmup. Root module covers all 30 sub-modules. |

---

## Inspecting Enrichment Results

After enrichment completes, you can query the SQLite database directly to verify
what changed. The queries below are grouped by purpose.

### Basic Statistics

```bash
# Total nodes and edges in the graph
sqlite3 graph.db "SELECT 'nodes', COUNT(*) FROM nodes UNION ALL SELECT 'edges', COUNT(*) FROM edges"

# Breakdown of edges by provenance (ast_inferred vs lsp_resolved)
sqlite3 graph.db "SELECT provenance, COUNT(*) FROM edges GROUP BY provenance ORDER BY COUNT(*) DESC"

# Breakdown of edges by type
sqlite3 graph.db "SELECT edge_type, COUNT(*) FROM edges GROUP BY edge_type ORDER BY COUNT(*) DESC"
```

### Enrichment Progress

```bash
# How many edges were upgraded by enrichment?
sqlite3 graph.db "SELECT COUNT(*) FROM edges WHERE provenance='lsp_resolved'"

# How many edges are still ast_inferred (not yet upgraded)?
sqlite3 graph.db "SELECT COUNT(*) FROM edges WHERE provenance='ast_inferred'"

# Check enrichment progress mid-run (run while enrichment is active)
sqlite3 graph.db "SELECT provenance, COUNT(*) FROM edges GROUP BY provenance"

# Edges discovered by enrichment (new, not upgrades)
sqlite3 graph.db "SELECT edge_type, COUNT(*) FROM edges WHERE provenance='lsp_resolved' AND edge_type IN ('implements','references') GROUP BY edge_type"
```

### Phantom Nodes

Phantom nodes are stub `external` nodes with no backing source file. They serve
as graph connectors for stdlib and dependency types.

```bash
# How many phantom external nodes were created?
sqlite3 graph.db "SELECT COUNT(*) FROM nodes n LEFT JOIN files f ON n.file_hash = f.file_hash WHERE f.file_hash IS NULL"

# How many real (non-phantom) nodes exist?
sqlite3 graph.db "SELECT COUNT(*) FROM nodes n JOIN files f ON n.file_hash = f.file_hash"

# Sample phantom node names (check for quality)
sqlite3 graph.db "SELECT qualified_name FROM nodes n LEFT JOIN files f ON n.file_hash = f.file_hash WHERE f.file_hash IS NULL LIMIT 10"
```

A phantom node count of zero means enrichment either did not run or produced no
new edges pointing to external targets.

---

## Known Issues

1. **gopls lazy loading on large Go repos.** Terraform (367 dependencies) needs 5+ minutes
   for gopls to load its dependency graph on-demand. The two-phase warmup mitigates this,
   but enrichment still takes 5-15 minutes. Repos with fewer dependencies are much faster.

2. **jdtls + Gradle 9.4 compatibility.** Annotation processor resolution fails with an
   exclusive lock error. Workaround: use Gradle 9.3 or earlier, or use Maven.

3. **tsserver GC thrashing on vscode-scale repos.** The TypeScript language server hits
   garbage collection pressure on repos with 3,000+ files. Set
   `NODE_OPTIONS="--max-old-space-size=8192"` to mitigate. Even with this, vscode takes
   ~34 minutes.

4. **Phantom nodes in FTS index.** Counterintuitively, keeping phantom external nodes in
   the BM25 index helps P@10 (IDF distribution effect). Removing them makes common terms
   artificially rare, distorting BM25 scoring. This was validated in the cross-system
   benchmark.

5. **pyright position quirk.** pyright sets `SelectionRange` to the keyword (`def`,
   `class`, `async def`) instead of the identifier. The `resolveNamePosition` function
   works around this by finding the symbol name on the source line.

---

## Troubleshooting / Debugging

### "Enrichment seems stuck"

Use `sample <gopls_pid> 1` (macOS) to check if gopls is CPU-bound or idle. If the
sampled stacks show active package loading (type checking, import resolution), gopls is
still working; wait for it. If the stacks show `pthread_cond_wait` (idle), the server
finished loading but is not receiving requests, or the file content was not sent correctly.
Check that `OpenDocument` is called with the correct argument order: `uri`, `content`,
`languageID`. Swapping `content` and `languageID` causes the server to receive a
single-word "file" and silently produce no results.

### "How do I know if enrichment worked?"

Check `knowing stats` for node count increase. Enriched repos have phantom external nodes:
django goes from ~55K to ~128K nodes after enrichment. To count phantom nodes directly:

```bash
sqlite3 <db> "SELECT COUNT(*) FROM nodes n LEFT JOIN files f ON n.file_hash = f.file_hash WHERE f.file_hash IS NULL"
```

Phantom nodes have no backing file, so they appear in `nodes` but have no match in
`files`. A count of zero means enrichment either did not run or produced no new edges.

### "Enrichment produced zero upgrades"

gopls needs `didOpen` to trigger package loading (lazy loading). Without it, all
`GetDefinition` requests return "no package metadata" instantly because gopls has not
loaded any packages. Check the enrichment log for the "server warmed up" message. If that
message never appears, the warmup phase timed out after 300 seconds without getting a
successful response.

Common causes: gopls binary is outdated (update with `go install golang.org/x/tools/gopls@latest`),
the workspace has build errors preventing package loading, or `go.sum` is incomplete
(run `go mod tidy` first).

### "gopls crashed during enrichment"

Check memory usage with `ps aux | grep gopls`. Large Go repos (terraform: 367
dependencies) can use 2GB+ of memory. If gopls is at 0% CPU but still alive (status SN
on macOS), it stopped loading and is effectively frozen. Kill the process and retry.

For repos with very large dependency trees, consider running enrichment on a machine with
16GB+ RAM, or use `-no-enrich` and accept the tree-sitter-only graph.

### "Should I skip enrichment?"

Use `-no-enrich` for fast iteration during development or supply chain scanning.
Enrichment is strongly positive for retrieval: +0.040 P@10 on Python repos, and
dramatically larger on Go repos (kubernetes 0.000 -> 0.159, terraform ~0.095 -> 0.265).
The tree-sitter extraction pipeline is self-sufficient for basic retrieval, but enrichment
creates phantom nodes and cross-package edges that significantly expand RWR reachability.
See [retrieval-pipeline.md](retrieval-pipeline.md) for measured impact.

For Go repos, the two-phase warmup protocol makes enrichment reliable. Expect 12-58 min
depending on repo size (terraform: 12 min, kubernetes: 58 min). For Rust repos,
rust-analyzer is fast (~1 min). For Python, pyright is fast (~10 min).

---

## FAQ

**Why does enrichment take 10+ minutes on vscode?**

tsserver GC thrashing. The discovery phase creates 468K phantom nodes, causing heavy
string serialization in Node.js. Each `GetDocumentSymbols` + `GetReferences` cycle
generates large JSON payloads that pressure the garbage collector. Mitigation: set
`NODE_OPTIONS="--max-old-space-size=8192"` before running enrichment. This increases the
V8 heap limit and reduces GC pauses, but vscode-scale repos will still take 30+ minutes.

**Why are phantom nodes in the FTS index?**

Removing them was tested and hurt P@10 (0.222 to 0.213 on the cross-system benchmark).
When 80K+ phantom nodes are removed from the FTS index, the IDF (inverse document
frequency) distribution shifts: terms that were common across phantom and real nodes
become artificially rare, distorting BM25 scoring for real symbols. Keeping phantom nodes
in the index preserves the natural term frequency distribution.

**Can I enrich just one file?**

Yes. `RunScoped` accepts a list of changed file paths and only processes edges
originating from those files. This is used by the daemon in watch mode for incremental
enrichment after file saves. From the CLI, standalone scoped enrichment is available
via the `--files` flag on `knowing enrich lsp`.

**Does enrichment change the snapshot hash?**

No. Enrichment modifies edges (upgrades provenance, inserts new edges) and creates
phantom nodes, but it does not recompute the Merkle snapshot. The snapshot hash reflects
the tree-sitter extraction state at index time. This means two identical `knowing index`
runs produce the same snapshot hash regardless of whether enrichment ran. The snapshot
is used for staleness detection in the pack cache (see
[retrieval-pipeline.md](retrieval-pipeline.md)), so enrichment changes are picked up
on the next query without cache invalidation.

---

## Source Files

| File | What it contains |
|------|-----------------|
| `internal/enrichment/enricher.go` | `Enricher`, `Run`, `RunScoped`, `upgradeCallEdges`, `discoverNewEdgesBatched`, `createPhantomNodes`, `resolveDefinitionToNode` |
| `internal/enrichment/config.go` | `LSPConfig`, `LSPServerConfig`, `DetectLSPServers`, `LoadLSPConfig` |
| `internal/enrichment/multimodule.go` | `DiscoverModules`, `ModuleInfo`, `FilesForModule` |
| `internal/enrichment/progress.go` | `EnrichProgress`, `LoadProgress`, `SaveProgress` |
| `internal/enrichment/timeout.go` | `WithSymbolTimeout`, `ErrSymbolTimeout`, `DefaultSymbolTimeout` |
| `cmd/knowing/enrich.go` | `cmdEnrich`, `cmdEnrichLSP`, `cmdEnrichBlame`, `cmdEnrichCoverage` |

## Related Documents

- [Extraction Pipeline](extraction-pipeline.md): the tree-sitter extraction stage that runs before enrichment; produces the baseline graph
- [Retrieval Pipeline](retrieval-pipeline.md): how RWR uses edges from enrichment; measured impact of enrichment on P@10
- [Embedding Re-ranker](embedding-reranker.md): the re-ranking stage that operates after enrichment-augmented graph walks
- [Edge Types](edge-types.md): full catalog of the 38 edge types, including `lsp_resolved` provenance
- [Data Flow](data-flow.md): how commits flow through indexing and enrichment into the graph
- [Context Engine](context-engine.md): how confidence from `lsp_resolved` edges feeds the scoring formula
