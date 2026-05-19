# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 2 | **~~`knowing why <symbol>`~~** | **Shipped.** Explains why a symbol ranked where it did: seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback weight, session boost, equivalence class matches. See [CLI reference](guide/cli.md#why). | Done |
| 3 | **Session memory persistence** | SessionTracker is ephemeral (lost on session end), task memory is coarse (keyword-level, 7-day decay). Persist session working sets to SQLite so resumed sessions pick up where they left off and cross-session patterns compound. Extends `internal/context/session.go` with a `session_events` table. | Medium |
| 4 | **~~Negative feedback~~** | **Already implemented.** `feedback` tool accepts `useful: false`, store computes useful/total ratio, ranking formula maps to [-0.15, +0.15] penalty/boost. Updated tool description to make negative feedback explicit. | Done |
| 5 | **Subgraph cache (Phase 2)** | Cache context_for_task, blast_radius, test_scope against hierarchical Merkle subgraph roots. Invalidate only changed packages on re-index. Subsumes the previous "traversal cache" item. See Merkle Tree Algorithms section. | Medium |
| 6 | **`knowing stats`** | Show session value: context calls, symbols served, symbols marked relevant, feedback rate. Lets users see the value accumulating. | Low |
| 7 | **MCP resources** | Lightweight context that doesn't cost a tool call. Resources are read directly by the MCP host for agent orientation. See detailed list below. | Medium |
| 8 | **~~v0.2.0 release~~** | **Shipped.** 25 extractors, retrieval pipeline, TOON, `knowing init`, multi-language LSP enrichment, `knowing why`, 84 equivalence classes, 23 MCP tools, ~60K LOC. | Done |

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| ~~`knowing watch`~~ | **Shipped.** Filesystem watcher (fsnotify) that re-indexes changed files on save with debounced batching and optional background LSP enrichment. | Done |
| ~~`knowing mcp --watch`~~ | **Shipped.** `knowing mcp --watch` combines MCP server + fsnotify watcher in one process. Also supports `--repo`, `--url`, `--no-enrich`, `--debounce`. | Done |
| ~~`knowing enrich blame`~~ | **Shipped.** Stamps last-author + last-commit-at on every symbol via `git blame --porcelain`. Migration 009 adds `last_author` and `last_commit_at` columns. | Done |
| ~~`knowing enrich coverage`~~ | **Shipped.** Stamps coverage percentage on symbols from Go cover profiles. Migration 010 adds `coverage_pct` column. Usage: `knowing enrich coverage -profile cover.out .` | Done |
| `knowing stats` | Show cumulative session value: context calls, symbols served, symbols marked relevant, feedback rate, token savings. Proves the value is accumulating. | P2 |
| Staleness reporting | Content-addressing makes staleness structurally detectable, but no command surfaces it. `knowing stale` should report "these N edges are stale because these files changed since the last snapshot." Free win from the architecture. | P2 |
| Daemon lifecycle | `knowing daemon start --detach`, `status`, `stop`, `restart`. Currently `knowing serve` blocks the terminal. Detached mode with PID file tracking for production use. | P2 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux) for always-on graph daemon. | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. Currently handled by CLI flags and `.gitignore` only. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges up/down/both from a type. Returns the full inheritance tree. Edges already exist in the graph; this is a traversal convenience wrapper. | P3 |
| `neighborhood` MCP tool | Seed-based dense neighborhood: "give me the N symbols most densely connected to X within radius R." Different from global Louvain communities. Wraps the existing RWR computation as a standalone tool. | P3 |
| GraphML/Cypher export | `knowing export -format graphml\|cypher` for loading the graph into Neo4j, Gephi, yEd, Cytoscape. GraphML is trivial (XML), Cypher enables visual graph exploration. | P3 |
| Snapshot diff workflows | Snapshot diffing exists but isn't wired into a "what changed in my architecture this sprint" workflow. | P3 |

## Multi-Repo Management

| Item | Description | Priority |
|------|-------------|----------|
| `untrack_repo` MCP tool + CLI | Evict a repo's nodes, edges, files, and snapshots from the graph. Currently requires manual SQL. | P2 |
| Active project scoping | Session-level "I'm working in repo X" default so agents don't pass `repo_url` on every call. `set_active_project` / `get_active_project` MCP tools. | P3 |
| `graph_stats` MCP tool | Total nodes/edges + per-repo breakdown + session token savings. Overlaps with `knowing stats` CLI. | P3 |

## MCP Resources (Planned)

Resources are read directly by the MCP host without a tool call. They provide lightweight orientation context at zero exchange cost.

| Resource | What it provides | Data source | Priority |
|----------|-----------------|-------------|----------|
| `knowing://report` | High-level orientation: graph size, top languages/kinds, hotspot count. The opening read of a new session. | Aggregate query over nodes/edges tables | P1 |
| `knowing://schema` | Graph schema reference: node kinds, edge kinds, provenance tiers, qualified-ID format. Helps agents form valid queries. | Static, derived from types package | P1 |
| `knowing://stats` | Node/edge counts, per-language and per-repo breakdown. Cheapest health check. | `AllRepos` + count queries | P1 |
| `knowing://repos` | Every tracked repo with node/edge counts. | `AllRepos` store method | P2 |
| `knowing://session` | Current session state: recent symbols, context calls, feedback rate, token savings. | SessionTracker + counters | P2 |
| `knowing://index-health` | Health score, parse failures, stale files. Subscribe for push updates after re-index. | CAS staleness detection | P2 |
| `knowing://communities` | Community list with cohesion scores. | Louvain output | P3 |
| `knowing://community/{id}` | Single community detail: members, key files, cross-community connections. | Filtered Louvain output | P3 |

## Underexploited Capabilities

These exist in the codebase but aren't wired into retrieval or workflows yet:

| Item | Status | Next step |
|------|--------|-----------|
| Community-aware retrieval | Communities computed, not used for scoping | Constrain RWR walk to seed communities (on roadmap) |
| ~~Modular community detection~~ | **Shipped.** `internal/community/` package with `Algorithm` interface and registry. Louvain (two resolution presets) + label propagation. MCP tool and export command accept `--algorithm` flag. Client-side viz has 6 grouping strategies. | Add Leiden when a Go implementation is available; it's a drop-in via the registry. |
| Edge event log | Events recorded, nothing reads them | Temporal queries: "when did this dependency appear?" |
| LSP enrichment (TS/Python/Java) | Shipped. TS: 98.9% upgrade rate. Python: 83% upgrade + 15K new edges. Java: working via jdtls with workspace readiness waiting. | Rust and C# enrichment available via rust-analyzer and OmniSharp when installed. |

## Retrieval Pipeline

Pipeline is shipped and measured (31.6% P@10, 55 fixtures, 23 experiments). See [retrieval-pipeline.md](architecture/retrieval-pipeline.md) for the authoritative reference.

**Next retrieval improvements (per local-first philosophy):**

| Item | Description | Status |
|------|-------------|--------|
| More equivalence concepts | Expand from 41 to 100+ as usage patterns emerge | Ongoing |
| Passive task memory compounding | Needs real agent sessions to accumulate data | Waiting on users |
| Session memory persistence | Persist session working sets to SQLite, replay on resume, compound cross-session patterns | Planned |
| Negative feedback signals | Penalize "this was noise" symbols in scoring, not just boost "this was relevant" | Planned |
| Code-tuned embedding model | Benchmark jina-code-v2 / bge-code when ONNX available | Planned (optional) |
| Community-aware retrieval | Constrain RWR walk to seed communities | Planned |

## Edge Type Expansion

### Runtime Intelligence

| Item | Description | Priority |
|------|-------------|----------|
| `runtime_queries` | Service/function queries database table/view/procedure | P1 |
| `runtime_connects_to` | Observed network connection beyond traced HTTP/RPC | P2 |
| `runtime_errors_at` | Symbol/route produces runtime errors | P3 |
| `runtime_uses_config` | Function reads config key or secret at runtime | P4 |
| `runtime_emits_metric` | Symbol emits a named metric | P5 |
| `runtime_logs_event` | Symbol emits a structured log event type | P5 |
| `runtime_writes` | Service/function writes table, bucket, queue, cache key, file, or object | Future |
| `runtime_reads` | Service/function reads table, bucket, cache key, config, secret, file, or object | Future |
| `runtime_scheduled` | Cron/job/workflow invoked function or service at runtime | Future |
| `runtime_allocates` | Service/function provisions or dynamically creates cloud resource | Future |
| `runtime_redirects_to` | HTTP route redirects/forwards/proxies to another route/service | Future |
| `runtime_authenticates_as` | Service acts as principal/role/user/client identity | Future |
| `runtime_authorizes` | Policy/permission check observed for route/function/action | Future |
| `runtime_depends_on` | Observed dependency inferred from runtime behavior when static linkage is absent | Future |

### Contract and API Edges

| Item | Description | Priority |
|------|-------------|----------|
| `implements_endpoint` | Handler function implements OpenAPI route | P1 |
| `consumes_endpoint` | Client code calls OpenAPI route | P1 |
| `implements_rpc` | Server implements proto RPC method | P2 |
| `consumes_rpc` | Client invokes proto RPC method | P2 |
| `publishes_event_schema` | Producer emits event matching a contract | P3 |
| `consumes_event_schema` | Consumer expects event matching a contract | P3 |
| `defines_schema` | Code/type defines schema or contract | Future |
| `validates_against` | Code validates payload against schema | Future |
| `serializes` / `deserializes` | Type crosses wire/storage boundary | Future |
| `breaking_change_for` | Derived edge from schema/API diff between versions | Future |

### Ownership and Governance

| Item | Description | Priority |
|------|-------------|----------|
| `owned_by` | Symbol/file/service owned by team/person (CODEOWNERS) | P1 |
| `classified_as` | Data classification (PII, PCI, PHI) | P2 |
| `secured_by` | Route/service protected by auth policy | P3 |
| `reviewed_by` | Code area requires specific reviewer | Future |
| `complies_with` | Maps component to compliance control | Future |
| `violates_policy` | Derived: symbol with PII classification lacks secured_by edge | Future |

### Static Semantic Edges

| Item | Description | Priority |
|------|-------------|----------|
| `extends` / `inherits` | Class inheritance (Java, C#, Python, TS) | P1 |
| `overrides` | Method overrides parent/interface method | P1 |
| `decorates` / `annotates` | Decorators, annotations, attributes | P2 |
| `throws` / `raises` | Error/exception relationships | P3 |
| `catches` / `handles_error` | Recovery paths for exceptions | Future |
| `generates` | Codegen source produces generated file/symbol | Future |

### Agent Workflow Edges

| Item | Description | Priority |
|------|-------------|----------|
| `suggested_for_task` | Symbol was included in agent context for a task | P1 |
| `used_by_agent` | Agent actually used/read/edited symbol | P1 |
| `validated_by_test` | Test verified symbol/change | P2 |
| `failed_in_ci` | Symbol associated with failing check | P2 |
| `changed_by_pr` | PR modifies symbol | Future |
| `reviewed_in_pr` | PR review comment targets symbol | Future |

### Deployment and Infrastructure Edges

| Item | Description | Priority |
|------|-------------|----------|
| `runs_on` | Service runs on deployment/node/runtime | P1 |
| `deployed_by` | Workflow/pipeline deploys service | P1 |
| `configured_by` | Config/secret/env var configures service | P2 |
| `exposes_port` | Service/container exposes port | Future |
| `mounts` | Workload mounts volume/secret/configmap | Future |
| `assumes_role` | Workload uses IAM role/service account | Future |
| `allowed_by` / `blocked_by` | Network/security/IAM policy permits or denies access | Future |

## Developer Visibility

| Item | Description |
|------|-------------|
| Ownership routing | "Who to notify" computed from graph edges (depends on ownership edges) |
| Staleness dashboard | Surface unverified edges and subgraphs |

## Merkle Tree Algorithms

Full specification in [architecture/merkle-algorithms.md](architecture/merkle-algorithms.md). Builds on the content-addressed graph structure to enable fine-grained invalidation, subgraph caching, incremental recompute, agent trust proofs, federated sync, and semantic change classification.

### Phase 1: Hierarchical Tree Structure -- SHIPPED

`internal/snapshot/hierarchical.go`. Repo root -> package roots -> edge-type roots -> edge leaves. Wired into `SnapshotManager.ComputeSnapshot`. Backward compatible (same root hash).

| Deliverable | Status |
|-------------|--------|
| `HierarchicalTree` struct (package roots, edge-type roots) | Shipped |
| `BuildHierarchicalTree` from `EdgeInput` slice | Shipped |
| `DiffHierarchicalTrees` (O(packages) instead of O(edges)) | Shipped |
| `SubgraphRoot` (cache key for any set of packages) | Shipped |
| `EdgeTypeRoot` ("did call edges change?" in one lookup) | Shipped |
| `ContextPackRoot` (content-addressed context pack identity) | Shipped |
| Wired into `SnapshotManager` (builds alongside flat tree) | Shipped |

Benchmarked: 283x faster diff at 10K edges, 517x faster at 100K edges. Build cost unchanged.

### Phase 2: Content-Addressed Context Packs + Community Rooting -- PARTIALLY SHIPPED

Two deliverables are shipped. Subgraph caching remains the primary outstanding item.

| Item | Description | Where | Status |
|------|-------------|-------|--------|
| `PackRoot` on `ContextBlock` | `hash(task_normalized + sorted(selected_node_hashes))`. Deterministic pack identity. 5 queries, 2 unique tasks = 2 unique PackRoots (perfect dedup). | `internal/context/context.go` | **Shipped** |
| Community `MerkleRoot` + `Packages` | `communityInfo` struct carries Merkle root over packages spanned. Returned by `communities` MCP tool. Community roots verified distinct per package set on live graph. | `internal/mcp/communities.go` | **Shipped** |
| Subgraph cache store | `map[Hash][]byte` keyed by subgraph roots, with TTL and size limits | `internal/cache/subgraph.go` (new) | Next |
| Cache `context_for_task` | Key: `hash(task + SubgraphRoot(seeded packages))`. Same subgraph root = same result. | `internal/context/context.go` | Next |
| Cache `blast_radius` | Key: `hash(symbol + SubgraphRoot(symbol pkg + neighbor pkgs))`. | `internal/mcp/handlers.go` | Next |
| Cache `test_scope` | Key: `hash(changed files + SubgraphRoot(affected pkgs))`. | `internal/mcp/testscope.go` | Next |
| Daemon invalidation | On re-index: `DiffHierarchicalTrees` -> only evict cache entries for changed packages. | `internal/daemon/watcher.go` | Next |

### Phase 3: Incremental Recompute + Context Packs

Use root diffs to scope downstream recomputation.

| Item | Description |
|------|-------------|
| Incremental Louvain | If only one package root changed, recompute community membership locally instead of global re-detection |
| Incremental HITS/BM25 | Only rebuild text indexes for changed packages |
| Context pack deduplication | Agents reference prior `ContextPackRoot` instead of resending content; GCF chunk-level deduplication |
| Context pack comparison | "What changed in the context this agent would see?" between two snapshots |
| Semantic change classification | Diff edge-type roots to classify: only calls changed (behavioral), only imports (structural), runtime drift (runtime roots changed, static unchanged) |

### Phase 4: Proofs, Sync, Bisection

| Feature | Description |
|---------|-------------|
| Merkle proofs | Prove a caller relationship existed in snapshot X; for CI, review, compliance |
| Federated sync | Exchange root hashes, descend only differing branches; sync without shipping the full DB |
| Snapshot-aware retrieval | Prefer symbols with stable neighborhood roots; boost recently changed subgraphs |
| Merkleized feedback validity | Feedback expires when `neighborhood_root` changes, not just by age |
| Merkle-based bisection | Binary search on snapshot chain to find when a subgraph property first diverged |
| Proof of absence | Prove an edge does NOT exist with a compact proof path; security audits, agent confidence |
| Lazy materialization | Load only visited subtrees; 100K-symbol repos without proportional memory cost |
| File-level roots | Add file roots between package and edge-type for finer single-file invalidation |

## Lessons from Git Source (Examined 2026-05-18)

Cloned `github.com/git/git` and studied the C implementation of tree diff, delta compression, and notes. These are battle-tested patterns from 20 years of production use that directly apply to knowing's architecture.

### Linus's Regrets (applied to knowing)

Linus has publicly discussed things he would change about git. These inform our design choices:

1. **SHA-1 was a mistake.** Picked for speed, not security. SHAttered attack in 2017 forced a multi-year migration to SHA-256. knowing started on SHA-256 and added domain-type prefixes ("node\0", "edge\0", etc.) early, before any production databases exist. Lesson: make identity changes before users depend on the format.

2. **The staging area confuses users.** The three-state model (working tree / index / HEAD) is powerful but most developers don't need it. If knowing adds a proposed graph overlay (the staging-area equivalent for previewing blast radius of uncommitted changes), keep it optional and implicit. Never force users through an explicit staging step.

3. **The reflog should be more prominent.** Git's most important safety net is invisible to most users. When knowing adds snapshot refs and reflog (Rec 7.1, 7.2), surface the chain prominently in `knowing fsck` and a future `knowing history` command. Don't hide the audit trail.

4. **Submodules are broken by design.** They try to version-pin external repos by commit hash, but the abstraction leaks. knowing's multi-repo model (roster + per-repo DBs) is already better: each repo is independent with its own DB and snapshot chain. Cross-repo references use qualified names, not pinned commit hashes. Don't introduce submodule-like coupling.

### Notes Table (metadata without hash invalidation)

**Git pattern:** `refs/notes/commits` is a separate tree. Notes attach arbitrary metadata to any object without changing its hash. Stored as a 16-way radix tree keyed by object SHA1 for O(1) lookup. (`notes.c`, `struct int_node`, `struct leaf_node`)

**knowing application:** A `graph_notes` table (`object_hash -> note_key -> note_value`) that never affects Merkle computation. Stores feedback scores, agent annotations, quality assessments, usage counts, and context pack evaluations without invalidating any cached results. Currently, feedback lives in a separate table but blame/coverage are baked into the Node struct. A unified notes layer would let all intelligence accrue cleanly.

| Priority | Effort |
|----------|--------|
| P1 (next) | Low (one table, one interface) |

### Proposed Graph Overlay (staging area)

**Git pattern:** The index (staging area) sits between working directory and commit. `git add` stages changes; `git diff --cached` shows what would be committed. Nothing is permanent until `git commit`.

**knowing application:** An in-memory graph overlay that shows "here's what the graph would look like if I indexed these files." Agents could preview blast radius of proposed changes without running the indexer. Useful for: "if I change this function signature, what breaks?" without actually changing it. Compose with hierarchical diff: "which package roots would change?"

| Priority | Effort |
|----------|--------|
| P2 | Medium |

### Delta-Compressed Snapshots

**Git pattern:** `diff-delta.c` uses Rabin rolling hash fingerprinting to find similar regions between two buffers, then stores copy/insert instructions. Packfiles store chains of deltas against base objects. A repo with 10,000 commits doesn't store 10,000 full copies.

**knowing application:** Snapshot chains currently store full edge sets at each point. Delta compression would store "copy edges 0-10000 from parent, insert 50 new, delete 3" instead of re-storing all edges. Combined with hierarchical roots: delta per package (only changed packages store deltas, unchanged packages reference parent). Dramatically reduces storage for long snapshot chains.

| Priority | Effort |
|----------|--------|
| P3 | High (needs careful design for query performance) |

### N-Way Hierarchical Diff with Pathspec Filtering

**Git pattern:** `ll_diff_tree_paths` in `tree-diff.c` does N-way merge (comparing multiple parents simultaneously), has early exit (`diff_can_quit_early`), and supports pathspec filtering (`skip_uninteresting`). Only descends into subtrees where roots differ.

**knowing application:** Our `DiffHierarchicalTrees` is currently 2-way only. N-way diff would support "what changed across these 5 snapshots?" (sprint review). Pathspec filtering would support "diff only the `internal/mcp` subtree" without scanning other packages. Early exit would support "stop after finding N changes" for large diffs.

| Priority | Effort |
|----------|--------|
| P3 | Medium |

### Rerere (Reuse Recorded Resolution)

**Git pattern:** `rerere.c` records how merge conflicts were resolved and auto-applies the same resolution when the same conflict reappears.

**knowing application:** When two extractors disagree on a call target (tree-sitter says A calls B, LSP says A calls C), record the resolution. Next time the same conflict appears, apply it automatically instead of re-resolving. Makes enrichment idempotent and faster.

| Priority | Effort |
|----------|--------|
| P4 | Low |

### Transfer Protocol (Have/Want Negotiation)

**Git pattern:** Smart HTTP/SSH protocol: client says "I have these roots," server says "you need these objects," sends a minimal thin packfile. Neither side sends everything.

**knowing application:** Blueprint for federated graph sync. Local dev and CI exchange hierarchical Merkle roots, descend into differing package roots, transfer only changed subtrees. Combined with delta compression: transfer deltas for changed packages only. Already on the Phase 4 roadmap; git's protocol design (especially thin packs and multi-ack) is the implementation reference.

| Priority | Effort |
|----------|--------|
| P4 (Phase 4) | High |

### Replace/Grafts (Edge Correction Without Re-Snapshot)

**Git pattern:** `git replace` creates a redirect from one object hash to another. The original stays in the object store; queries transparently see the replacement.

**knowing application:** "This edge was wrong, here's the corrected version" without re-snapshotting. Old snapshots remain valid; new queries see the corrected edge. Useful for LSP enrichment corrections and manual edge fixes.

| Priority | Effort |
|----------|--------|
| P4 | Medium |

## Agent Coordination

| Item | Description |
|------|-------------|
| Pending mutations | Agents announce in-flight changes, others see proposed state (see: proposed graph overlay above) |
| Temporal reasoning | Walk snapshots backward to find when incompatibilities appeared |
| Federated graphs | Cross-instance queries via Merkle diff exchange (see: transfer protocol above) |

## Strategic Position

knowing is an intelligence versioning system. Git versions files; knowing versions the understanding of code: relationships, confidence, provenance, and what changes mean. Every snapshot captures not just structure but learned intelligence (feedback, session patterns, task memory) that compounds with use.

The retrieval pipeline uses equivalence classes (not embeddings) as the primary concept-matching mechanism. This is local, deterministic, inspectable, and compounds with use. See [retrieval-pipeline.md](architecture/retrieval-pipeline.md) for the design rationale.

The hierarchical Merkle tree (shipped) structures snapshots by semantic boundaries (package, edge type) instead of flat edge hashes. This enables 283-517x faster diffs, O(1) subgraph root lookups for cache keys, and scoped invalidation. Phase 2 (subgraph caching) will make most queries near-instant for unchanged subgraphs. No competitor uses content-addressed hierarchical graph Merkle trees.

**What's shipped (v0.2.0+):** ~60K LOC Go, 25 extractor types (12 languages + 13 infrastructure/cloud formats), 23 MCP tools, 5 wire formats (GCF/TOON/JSON/XML/markdown), 55 eval fixtures, 84 equivalence classes, multi-language LSP enrichment (Go, TS, Python, Java, Rust, C#), `knowing init` one-command setup, `knowing why` retrieval explainability, hierarchical Merkle tree with package/edge-type subtrees, modular community detection (Louvain + label propagation registry), React viz with 6 grouping strategies.
