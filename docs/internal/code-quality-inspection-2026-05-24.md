# Code Quality Inspection: internal/ packages

**Date:** 2026-05-24  
**Scope:** internal/context, internal/store, internal/indexer, internal/mcp, internal/daemon, internal/wire, internal/snapshot  
**Methodology:** Manual structural analysis of source files with grep-based cross-reference verification

---

## 1. internal/context/ (retrieval pipeline)

### DUPLICATION

#### D1. Duplicated edgeWeight map in walk.go (HIGH)
- **File:** internal/context/walk.go, lines 222-243 and 387-408
- **Category:** Duplication
- **Severity:** HIGH
- **Issue:** The `edgeWeight` map (22 entries mapping edge types to float64 multipliers) is copy-pasted identically in both `RandomWalkWithRestartWeighted` and `CommunityFilteredRWR`. If a new edge type is added or weights are tuned, both must be updated in lockstep. This is already mentioned in the user's prompt as a known problem.
- **Recommendation:** Extract to a package-level `var edgeWeights = map[string]float64{...}` constant. Both functions reference it.

#### D2. Entire RWR iteration loop duplicated between RandomWalkWithRestartWeighted and CommunityFilteredRWR (HIGH)
- **File:** internal/context/walk.go, lines 255-335 vs 415-493
- **Category:** Duplication
- **Severity:** HIGH
- **Issue:** The 80-line iteration loop (restart component, walk component, convergence check, top-K stability check) is copied verbatim between the two functions. The ONLY difference is how the adjacency map is built (filtered vs unfiltered). The iteration logic, normalization, and early termination are identical.
- **Recommendation:** Extract a `rwr(seedVec map[types.Hash]float64, alpha float64, maxIter int, adjFrom, adjTo map[types.Hash][]types.Edge) (map[types.Hash]float64, error)` function that takes pre-built adjacency maps and performs the iteration. Both `RandomWalkWithRestartWeighted` and `CommunityFilteredRWR` become thin wrappers that build their adjacency maps then call `rwr(...)`.

#### D3. HITS setup pattern duplicated across ForTask, ForFiles, ForPR (MEDIUM)
- **File:** internal/context/context.go, lines 652-663, 870-883, 984-1000
- **Category:** Duplication
- **Severity:** MEDIUM
- **Issue:** The "sort by CallerCount, take top-200, extract hashes, call ComputeHITS" pattern is repeated in three places with identical logic.
- **Recommendation:** Extract `computeHITSFromInputs(ctx, store, inputs []ScoringInput) map[types.Hash]HITSScores` helper.

#### D4. splitIdentifier (context.go) vs splitCamelCase (store/sqlite.go) (MEDIUM)
- **File:** internal/context/context.go:1287 and internal/store/sqlite.go:1440
- **Category:** Duplication
- **Severity:** MEDIUM
- **Issue:** Two independent CamelCase splitting implementations with slightly different behavior. `splitIdentifier` also handles snake_case; `splitCamelCase` handles uppercase runs (e.g., "HTMLParser" -> ["HTML", "Parser"]). Neither handles all cases the other does. The context package uses `splitIdentifier`, the store uses `splitCamelCase`.
- **Recommendation:** Unify into a single function in an internal/text or internal/ident utility package. Both callers should use the same logic for consistency.

#### D5. extractShortName, extractSymbolName, terminalName: three implementations of "get last name component" (MEDIUM)
- **File:** internal/context/implicit.go:249, internal/store/sqlite.go:1203, internal/indexer/indexer.go:1204, internal/types/verify.go:40
- **Category:** Duplication
- **Severity:** MEDIUM
- **Issue:** Four separate functions across four packages all extract the "last component" from a qualified name, each with subtly different stripping logic:
  - `extractShortName`: splits on last dot, falls back to last slash
  - `extractSymbolName` (store): strips `://`, last slash, file extensions
  - `extractSymbolName` (types): splits on last dot
  - `terminalName`: splits on last dot only
- **Recommendation:** Consolidate into a single `types.TerminalName(qn string) string` in the types package with well-defined semantics.

#### D6. communityProvider interface defined twice (LOW)
- **File:** internal/context/walk.go:679 and internal/context/context.go:456
- **Category:** Duplication
- **Severity:** LOW
- **Issue:** The same interface (with the same method signature) is defined as a local type in two places within the same package.
- **Recommendation:** Define once at package level.

#### D7. "batch store or individual put" pattern repeated 5 times in indexer.go (MEDIUM)
- **File:** internal/indexer/indexer.go, lines 548-565, 573-581, 636-651, 899-928
- **Category:** Duplication
- **Severity:** MEDIUM
- **Issue:** The pattern `if bs, ok := idx.store.(batchStore); ok { bs.BatchPutNodes/Edges... } else { for range { idx.store.PutNode... } }` appears 5 times.
- **Recommendation:** Add a helper method `(idx *Indexer) batchStore(ctx, nodes, edges, files)` that encapsulates the type assertion and fallback.

---

### USELESS ABSTRACTIONS

#### U1. Custom sqrt function in hits.go (HIGH)
- **File:** internal/context/hits.go:131-141
- **Category:** Useless abstraction / reinventing stdlib
- **Severity:** HIGH
- **Issue:** A hand-rolled Newton's method `sqrt` function (20 iterations) when `math.Sqrt` is available in the standard library. The `math` package is already imported in `session.go` within the same package. The hand-rolled version is slower and less accurate.
- **Recommendation:** Replace with `math.Sqrt`.

#### U2. Custom abs function in walk.go (LOW)
- **File:** internal/context/walk.go:793-798
- **Category:** Useless abstraction
- **Severity:** LOW
- **Issue:** A trivial `abs(x float64) float64` helper. Go 1.26 has `math.Abs` in the standard library.
- **Recommendation:** Replace with `math.Abs`. Same function exists in store/feedback_test.go.

#### U3. Custom max function in tokens.go (LOW)
- **File:** internal/context/tokens.go:40-45
- **Category:** Useless abstraction
- **Severity:** LOW
- **Issue:** Go 1.21+ has the built-in `max()` function. Go 1.26 is in use per go.mod.
- **Recommendation:** Delete the function and use the built-in `max`.

#### U4. isCommonShortName uses a map literal on every call (LOW)
- **File:** internal/context/context.go:1433-1439
- **Category:** Useless abstraction
- **Severity:** LOW
- **Issue:** This function is called inside a loop (in `filterNoisySymbols`) and allocates a new map on every invocation. The map has 7 static entries.
- **Recommendation:** Promote to a package-level `var commonShortNames = map[string]bool{...}` to avoid repeated allocation.

---

### MISSING ABSTRACTIONS

#### M1. No shared "qualified name parser" type (HIGH)
- **File:** internal/context/, internal/store/, internal/indexer/, internal/mcp/
- **Category:** Missing abstraction
- **Severity:** HIGH
- **Issue:** Qualified names follow the format `"repoURL://pkgPath.Type.Method"` but every package implements its own ad-hoc parsing: `extractQualifiedPackage` (mcp/handlers.go:281), `ExtractPackagePath` (snapshot/), `extractShortName` (context/implicit.go), `extractSymbolName` (store/sqlite.go, types/verify.go), `terminalName` (indexer/indexer.go), inline parsing in `filterNoisySymbols`. There is no canonical parser.
- **Recommendation:** Create `types.ParseQualifiedName(qn string) (repoURL, pkgPath, typeName, methodName string)` that all packages call. This eliminates 6+ independent parsing implementations and guarantees consistent behavior.

#### M2. No constant for the default 0.3 edge weight fallback (MEDIUM)
- **File:** internal/context/walk.go, lines 284 and 294 (and duplicated at 446, 454)
- **Category:** Missing abstraction (magic number)
- **Severity:** MEDIUM
- **Issue:** The default edge weight for unknown edge types is hardcoded as `0.3` in four places. If this policy changes, all four must be found.
- **Recommendation:** `const defaultEdgeWeight = 0.3`

#### M3. No constant for RWR parameters (MEDIUM)
- **File:** internal/context/walk.go and internal/context/context.go
- **Category:** Missing abstraction (magic numbers)
- **Severity:** MEDIUM
- **Issue:** `alpha = 0.2`, `maxIter = 20`, `maxDepth = 4`, `earlyTopK = 10`, probability threshold `0.0001`, convergence threshold `0.001`, RWR score threshold `0.02`, seed cap `15`, HITS top-N `200` are all magic numbers scattered across multiple functions.
- **Recommendation:** Group into a `type RWRConfig struct { Alpha float64; MaxIter int; ... }` with `DefaultRWRConfig()`.

#### M4. Repeated "comma-split + trim" pattern in MCP handlers (LOW)
- **File:** internal/mcp/context_handlers.go:117-123, 160-166; internal/mcp/testscope.go:46-53
- **Category:** Missing abstraction
- **Severity:** LOW
- **Issue:** The pattern `strings.Split(arg, ",")` followed by trim+filter is repeated in three handlers.
- **Recommendation:** Extract `splitCSV(s string) []string` helper in the mcp package.

---

### DEAD CODE

#### DC1. extractKeywords wrapper function barely used (LOW)
- **File:** internal/context/context.go:1050-1052
- **Category:** Dead code / near-dead
- **Severity:** LOW
- **Issue:** `extractKeywords(desc string) []string` is a one-line wrapper around `extractKeywordSet(desc).All()`. It is only called from `task_memory.go:129` and `context_test.go`. The structured `extractKeywordSet` should be used directly.
- **Recommendation:** Inline at call sites, remove the wrapper.

#### DC2. Separate extractKeywords in mcp/planturn.go shadows context version (MEDIUM)
- **File:** internal/mcp/planturn.go:120
- **Category:** Duplication / shadowing
- **Severity:** MEDIUM
- **Issue:** The mcp package has its own independent `extractKeywords` function with different stop-word lists and logic from the one in the context package. This is confusing and the two could drift.
- **Recommendation:** Either reuse `context.extractKeywordSet(...).All()` (if the import is acceptable) or rename to `planTurnKeywords` to make the divergence explicit.

---

### PERFORMANCE

#### P1. Map allocation inside RWR iteration loop (HIGH)
- **File:** internal/context/walk.go, lines 256 and 416
- **Category:** Performance
- **Severity:** HIGH
- **Issue:** `next := make(map[types.Hash]float64)` is allocated every iteration (up to 20 iterations). On large graphs with 1000+ reachable nodes, this creates significant GC pressure. The map is discarded at the end of each iteration.
- **Recommendation:** Reuse two alternating maps (double-buffer pattern): allocate `mapA` and `mapB` before the loop, clear+swap each iteration instead of allocating.

#### P2. `append(adjFrom[node], adjTo[node]...)` allocates a new slice every node every iteration (HIGH)
- **File:** internal/context/walk.go, lines 270 and 430
- **Category:** Performance
- **Severity:** HIGH
- **Issue:** Inside the hot RWR loop, `edges := append(adjFrom[node], adjTo[node]...)` creates a new combined slice for EVERY node on EVERY iteration. With 1000 nodes and 20 iterations, this is 20,000 temporary slice allocations.
- **Recommendation:** Iterate over adjFrom[node] and adjTo[node] separately (two for-range loops) instead of merging. No allocation needed.

#### P3. topKFromProb scans entire probability map every iteration (MEDIUM)
- **File:** internal/context/walk.go, lines 803-821
- **Category:** Performance
- **Severity:** MEDIUM
- **Issue:** `topKFromProb` iterates over the entire `prob` map (potentially 1000+ entries) with O(n*k) insertion sort on every iteration of the RWR loop.
- **Recommendation:** Since this is only for early termination (k=10), this is acceptable for now, but a min-heap would reduce to O(n*log(k)).

#### P4. loadExternalHashes called redundantly in buildFromCache (MEDIUM)
- **File:** internal/context/walk.go, line 624
- **Category:** Performance
- **Severity:** MEDIUM
- **Issue:** `loadExternalHashes` queries the database for all nodes matching "%external%". It is called once in `buildAdjacencyMap` (line 534) and AGAIN in `buildFromCache` (line 624). If the cache path is taken (via `buildAdjacencyMap` calling `buildFromCache`), it runs twice.
- **Recommendation:** Pass the externals set as a parameter from `buildAdjacencyMap` to `buildFromCache` instead of recomputing.

#### P5. ForTask calls store.GetNode for every node above RWR threshold (MEDIUM)
- **File:** internal/context/context.go, lines 527-530
- **Category:** Performance
- **Severity:** MEDIUM
- **Issue:** After RWR, for every node hash with score > 0.02 (potentially hundreds), `GetNode` is called individually. Even with the node cache, this is hundreds of sync.Map lookups where a batch query (single SQL IN clause) would be faster.
- **Recommendation:** Collect all qualifying hashes, batch-fetch with a single `SELECT ... WHERE node_hash IN (...)` query (add `BatchGetNodes` to the store).

#### P6. BFS in handleFlowBetween has no visited set (MEDIUM)
- **File:** internal/mcp/dataflow.go, lines 99-148
- **Category:** Performance
- **Severity:** MEDIUM
- **Issue:** The BFS path finder has no `visited` set. On cyclic graphs (which are common: A calls B, B calls A), the BFS queue grows exponentially until maxDepth is reached. With depth=5 and average degree 4, this explores up to 4^5 = 1024 states without deduplication.
- **Recommendation:** Add a `visited map[types.Hash]bool` to prevent re-enqueueing nodes already explored.

---

### SIMPLIFICATION

#### S1. ForTask is 500+ lines (HIGH)
- **File:** internal/context/context.go, lines 216-727
- **Category:** Simplification
- **Severity:** HIGH
- **Issue:** The `ForTask` method is over 500 lines and handles: cache lookup, persistent cache lookup with staleness, keyword extraction, tiered search, BM25, vector search, equivalence matching, graph aliases, RRF fusion, interface-aware seeding, noise filtering, community detection, RWR, score building, feedback, session boosts, task memory, HITS, ranking, packing, session recording, cache storage, and persistent cache storage.
- **Recommendation:** Split into pipeline stages:
  1. `seedRetrieval(ctx, ks) []types.Node` (channels 1-4 + RRF)
  2. `rwr(ctx, seeds) map[types.Hash]float64`
  3. `buildScoringInputs(ctx, rwrScores, seeds) []ScoringInput`
  4. `applyBoosts(ctx, inputs) []ScoringInput`
  5. `rankAndPack(inputs, budget, format) *ContextBlock`

---

## 2. internal/store/ (SQLite store)

### DUPLICATION

#### D8. scanNode and scanNodes have identical field-scanning logic (LOW)
- **File:** internal/store/sqlite.go, lines 1028-1039 and 1073-1086
- **Category:** Duplication
- **Severity:** LOW
- **Issue:** The scan logic (column ordering, hash copy) is written once for `*sql.Row` (single) and once for `*sql.Rows` (multi), though `scanNode` technically works on the `scannable` interface.
- **Recommendation:** This is acceptable given the `scannable` interface, but `scanNodes` could call `scanNode` internally for each row to reduce duplication.

#### D9. Cache eviction pattern repeated twice (node + edge) (LOW)
- **File:** internal/store/sqlite.go, lines 363-369 and 393-399
- **Category:** Duplication
- **Severity:** LOW
- **Issue:** Both `GetNode` and `GetEdge` have identical "check count, range-delete all, reset counter" eviction logic.
- **Recommendation:** Extract a generic `evictIfFull[K comparable, V any](cache *sync.Map, count *atomic.Int64, maxEntries int64)` helper.

### MISSING ABSTRACTIONS

#### M5. SQL column lists repeated in 10+ queries (MEDIUM)
- **File:** internal/store/sqlite.go, throughout
- **Category:** Missing abstraction
- **Severity:** MEDIUM
- **Issue:** The column list `node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct` appears in at least 8 different queries (GetNode, NodesByName, NodesByFilePath, StaleNodesByFiles, NodesByQualifiedName, SearchBM25Nodes, TransitiveCallers, TransitiveCallees). Same for edges (9 columns, 7+ queries).
- **Recommendation:** Define `const nodeColumns = "node_hash, file_hash, qualified_name, kind, line, signature, doc, last_author, last_commit_at, coverage_pct"` and `const edgeColumns = "..."` as package-level constants.

#### M6. BlastRadius does N+1 queries for repo URL lookup (HIGH)
- **File:** internal/store/sqlite.go, lines 620-632
- **Category:** Performance / missing abstraction
- **Severity:** HIGH
- **Issue:** For every caller in the blast radius result (could be 50+), `BlastRadius` executes a separate query joining files to repos to get the repo URL. This is an N+1 query pattern.
- **Recommendation:** Do a single JOIN in the `TransitiveCallers` CTE that includes `r.repo_url`, or batch the repo URL lookups afterward.

### PERFORMANCE

#### P7. CommunitiesForNodes chunks at 99 but builds query string per chunk (LOW)
- **File:** internal/store/sqlite.go, lines 1576-1633
- **Category:** Performance
- **Severity:** LOW
- **Issue:** Each chunk builds a new placeholder string and args slice. For a typical call with ~200 hashes (2 chunks), this is fine, but the approach could use a prepared statement.
- **Recommendation:** Acceptable as-is given typical usage. Note for future optimization if batch sizes grow.

---

## 3. internal/indexer/

### DUPLICATION

#### D10. "Check if batch store, else individual puts" pattern repeated 5x (MEDIUM)
- (See D7 above in the context section; the same issue appears in indexer.go)

#### D11. isGeneratedContent could live in a shared util (LOW)
- **File:** internal/indexer/indexer.go:1038-1065
- **Category:** Duplication potential
- **Severity:** LOW
- **Issue:** This function checks file content for generated markers. It's only called from the indexer, so no active duplication, but it is the kind of utility that could be needed by other tools (linters, CI).
- **Recommendation:** Fine where it is for now. Move to a shared util only if a second caller appears.

### SIMPLIFICATION

#### S2. IndexRepo is 200+ lines of sequential logic (MEDIUM)
- **File:** internal/indexer/indexer.go, lines 200-798
- **Category:** Simplification
- **Severity:** MEDIUM
- **Issue:** This method handles: repo setup, module map building, existing file lookup, directory walking, generated file detection, change detection, cleanup, extraction dispatch, batch storage, CODEOWNERS, inheritance propagation, authorship extraction, snapshot computation, FTS rebuild, edge event recording, resolve edges, and auto-GC. It is 600 lines of orchestration logic.
- **Recommendation:** Factor out named stages: `(idx *Indexer) walkFiles(...)`, `(idx *Indexer) extractAndStore(...)`, `(idx *Indexer) finalizeSnapshot(...)`. The current code already has section comments indicating these stages.

### PERFORMANCE

#### P8. IndexFilesIncremental does sequential file extraction (MEDIUM)
- **File:** internal/indexer/indexer.go, lines 837-893
- **Category:** Performance
- **Severity:** MEDIUM
- **Issue:** Unlike `IndexRepo` (which uses a worker pool), `IndexFilesIncremental` processes files sequentially. On a 10-file incremental update this is fine, but on a large rebase (50+ files) it could be slow.
- **Recommendation:** Use the same worker pool pattern from `IndexRepo` for extraction, keeping the batch store writes sequential.

---

## 4. internal/mcp/

### DUPLICATION

#### D12. stopWords defined independently in mcp/planturn.go and context/context.go (MEDIUM)
- **File:** internal/mcp/planturn.go (inline map) and internal/context/context.go:158-178
- **Category:** Duplication
- **Severity:** MEDIUM
- **Issue:** Two independent stop-word lists with different entries, used for the same conceptual purpose (filtering non-meaningful words from task descriptions).
- **Recommendation:** Export the canonical stop-words from the context package (`context.StopWords`) and reuse in planturn.go, or at minimum document that they are intentionally different.

### MISSING ABSTRACTIONS

#### M7. Handler result pattern repeated ~15 times (LOW)
- **File:** internal/mcp/handlers.go, throughout
- **Category:** Missing abstraction
- **Severity:** LOW
- **Issue:** The pattern `result, err := mcp.NewToolResultJSON(x); if err != nil { return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil }; return result, nil` appears in at least 12 handlers.
- **Recommendation:** Extract `jsonResult(v any) (*mcp.CallToolResult, error)` helper.

### PERFORMANCE

#### P9. handleOwnership loads ALL nodes for a repo with NodesByName (MEDIUM)
- **File:** internal/mcp/handlers.go, lines 431-433
- **Category:** Performance
- **Severity:** MEDIUM
- **Issue:** `NodesByName(ctx, repo.RepoURL)` uses a LIKE query with the repo URL as prefix, which returns ALL nodes in the repo (potentially 10K+) into memory to group by file. This should use a JOIN in SQL.
- **Recommendation:** Add a purpose-built `NodesGroupedByFile(ctx, repoHash)` query that does the grouping in SQLite.

---

## 5. internal/daemon/

### MISSING ABSTRACTIONS

#### M8. MCPServer interface is overly narrow (LOW)
- **File:** internal/daemon/daemon.go:36-39
- **Category:** Missing abstraction
- **Severity:** LOW
- **Issue:** The interface declares `ServeStdio` and `ServeHTTP` but the daemon only uses one at a time (based on config). This is fine as an abstraction boundary.
- **Recommendation:** Acceptable as-is.

---

## 6. internal/wire/

### DEAD CODE

#### DC3. Components struct in wire/gcf.go omits Feedback and Session fields (LOW)
- **File:** internal/wire/gcf.go:28-33
- **Category:** Incomplete model
- **Severity:** LOW
- **Issue:** The wire `Components` struct has only 4 fields (BlastRadius, Confidence, Recency, Distance) while the context `ScoreComponents` has 6 (adds Feedback, Session). The wire format silently drops feedback and session score components.
- **Recommendation:** Either add the missing fields to the wire format or document that they are intentionally omitted for bandwidth savings.

---

## 7. internal/snapshot/

No critical issues identified from the file listing and cross-references. The package is well-scoped with clear responsibilities (manager, hierarchical tree, Merkle proofs, GC, verification).

---

## Summary by Severity

| Severity | Count | Key Impact |
|----------|-------|------------|
| HIGH     | 8     | Performance in hot loops, major duplications, missing canonical parser |
| MEDIUM   | 15    | Code smell, maintenance burden, moderate perf issues |
| LOW      | 11    | Style, minor allocations, single-use helpers |

## Top 5 Actions by Impact

1. **Extract RWR iteration loop** (D2 + P1 + P2): Eliminates 80 lines of duplication AND fixes the two worst hot-path allocation issues in a single refactor. Estimated: 30 min.

2. **Create types.ParseQualifiedName** (M1): Eliminates 6+ ad-hoc parsing implementations across 4 packages. Reduces bug surface for qualified name handling. Estimated: 1 hour.

3. **Extract edgeWeight to package-level var** (D1): 2-minute fix that removes a known maintenance trap.

4. **Replace hand-rolled sqrt/abs/max** (U1, U2, U3): Trivial changes that improve correctness and performance for free.

5. **Fix BlastRadius N+1 query** (M6): Single SQL change eliminates O(N) queries in a user-facing MCP tool.
