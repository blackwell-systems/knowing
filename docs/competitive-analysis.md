# Competitive Analysis: knowing

Internal strategy document. Last updated: 2026-05-18.
Do NOT commit this file (gitignored).

---

## What knowing Is

knowing is a content-addressed graph artifact where every entity (symbols, relationships, files, repos, and graph snapshots) is identified by its SHA-256 hash. It indexes code via 25 extractor types covering 12 programming languages + 13 infrastructure/cloud formats, enriches edges via multi-language LSP (gopls, typescript-language-server, pyright, jdtls, rust-analyzer, OmniSharp) and SCIP, ingests runtime traces via OTLP, and exposes the graph through 23 MCP tools + 3 prompts. ~60K LOC Go, single binary, SQLite. Published as v0.1.2 across Homebrew, npm, PyPI, Docker, and MCP Registry. It targets AI coding agents and platform teams that need structural, versioned, provenance-scored relationships, not just file contents.

Key differentiators:
- **Content-addressed Merkle-DAG** with O(1) staleness detection, full history, provable integrity
- **25 extractors**: 12 languages (Go, TS, Python, Ruby, Rust, Java, C#, Terraform, SQL, K8s YAML, CSS, Protocol Buffers) + 13 infrastructure/cloud formats (Event/MQ patterns, OpenAPI/JSON Schema, CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework, Dockerfile, Makefile, Helm Charts, GitLab CI, package.json/npm, GraphQL, Ansible)
- **5-tier provenance model** (ast_inferred 0.7 -> lsp_resolved 0.9 -> scip_resolved 0.95 -> ast_resolved 1.0 -> otel_trace 0.2-0.95)
- **SCIP index ingestion** for compiler-accurate external dependency symbols (0.95 confidence)
- **Runtime trace ingestion** (OTLP gRPC, observation-count confidence, hourly decay)
- **GCF wire format** (84% token savings vs JSON, session statefulness for cross-call dedup)
- **Graph-aware context packing** (5-tier seeding + RWR + HITS reranking + density-ranked knapsack + feedback-aware scoring)
- **23 MCP tools** including feedback loop, test scope, flow analysis, plan routing, community detection, retrieval explainability
- **Louvain community detection** with multi-repo graph visualization (Sigma.js + Three.js)
- **5 Claude Code hooks** (SessionStart, PreEdit, PreCompact, PostTask, Subagent)
- **Graph-native test selection** (`knowing test-scope`: BFS backward from changed symbols to test functions)
- **Cross-repo edge resolution**, **multi-extractor dispatch**, and **KNOWING_DB** global config
- **Single Go binary**, SQLite, no external dependencies

---

## Tier 1: Knowledge Graph Engines

### GitNexus

**Source:** [github.com/abhigyanpatwari/GitNexus](https://github.com/abhigyanpatwari/GitNexus)
**Verified from GitHub API + README on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 38,536 |
| Language | TypeScript |
| License | PolyForm Noncommercial 1.0 (NOT open source for commercial use) |
| Created | 2025-08-02 |
| Last push | 2026-05-15 |

**What it does:** Client-side knowledge graph creator. Indexes any codebase into a knowledge graph (dependencies, call chains, clusters, execution flows). Two modes: (1) Web UI that runs entirely in-browser via WASM (Tree-sitter WASM + LadybugDB WASM); (2) CLI + MCP server with native Tree-sitter and LadybugDB.

**Key features (verified):**
- Browser-based and CLI+MCP modes
- Tree-sitter parsing (native or WASM)
- LadybugDB graph database (embedded, purpose-built)
- MCP server for Claude Code, Cursor, Codex, Windsurf, OpenCode
- Claude Code hooks (PreToolUse + PostToolUse) for auto-context enrichment
- Agent skills system
- Enterprise offering (SaaS/self-hosted) via akonlabs.com with PR review, auto-reindexing, multi-repo support
- Auto-generates AGENTS.md/CLAUDE.md context files
- Bridge mode: `gitnexus serve` lets web UI browse CLI-indexed repos

**What they have that knowing doesn't:**
- Massive traction (38k stars)
- Browser-based zero-install mode
- ~~Agent hooks integration~~ **CLOSED** (5 hooks, proven net-positive)
- ~~Skills/prompt system~~ **CLOSED** (3 MCP prompts)
- Enterprise product with PR blast-radius review
- ~~Auto-reindexing~~ **CLOSED** (daemon git watcher)
- Auto-generated agent context files (AGENTS.md, CLAUDE.md)
- Broader language support (mentions Dart, Proto, OCaml enterprise)

**What knowing has that they don't:**
- Multi-provenance confidence model (ast_inferred -> lsp_resolved -> otel_trace)
- Multi-language LSP enrichment (Go/TS/Python/Java: edge upgrade from 0.7 to 0.9, tested 83-99% upgrade rates)
- Runtime trace ingestion (OTLP gRPC, confidence decay, dead route detection)
- Content-addressed Merkle-DAG snapshots with diff capability
- Edge event history (temporal "when did this edge appear/disappear")
- Cross-repo edge resolver
- Snapshot-based semantic diff and PR impact tools
- GCF wire format (84% token savings; their format gets ~27%)
- Session statefulness (47% dedup on repeated symbols across calls)
- Graph-aware context packing with RWR + HITS scoring
- 25 extractor types with 18 framework route detectors across 6 languages
- MIT licensing (GitNexus is noncommercial only for OSS)

**Assessment:** GitNexus is the most direct competitor and has enormous traction. Its noncommercial license is a significant weakness for enterprise adoption, but its enterprise offering through akonlabs.com addresses that. knowing's advantages are depth (provenance tiers, LSP enrichment, runtime traces, temporal snapshots) vs GitNexus's breadth (browser mode, agent hooks, massive community). GitNexus is optimized for the "agent context" use case; knowing is optimized for being a durable system of record.

---

### CodeGraphContext (CGC)

**Source:** [github.com/CodeGraphContext/CodeGraphContext](https://github.com/CodeGraphContext/CodeGraphContext)
**Verified from GitHub API + README on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 3,279 |
| Language | Python |
| License | MIT |
| Created | 2025-08-16 |
| Last push | 2026-05-16 |

**What it does:** MCP server + CLI that indexes local code into a graph database. Tree-sitter based parsing. Supports multiple graph database backends (KuzuDB, FalkorDB, Neo4j, Nornic DB).

**Key features (verified):**
- 20 programming languages supported
- Multiple graph DB backends (KuzuDB embedded default, FalkorDB, Neo4j)
- MCP server mode for AI assistants
- CLI toolkit with callers/callees/complexity/dead-code analysis
- Live file watching with auto-update
- SCIP indexing option for higher accuracy (C/C++ via scip-clang, C# via scip-dotnet)
- Pre-indexed bundles (.cgc files) for famous repos
- Interactive visualization (HTML-based graph explorer)
- Cypher query support (via Neo4j/graph DB)

**What they have that knowing doesn't:**
- 20 language support (vs knowing's 25 extractor types across 12 languages + 13 infra formats)
- Multiple graph database backends (KuzuDB, FalkorDB, Neo4j)
- Pre-indexed bundle distribution
- Dead code detection CLI
- Complexity analysis

**What knowing has that they don't:**
- Multi-language LSP enrichment (edge confidence upgrade via gopls, tsserver, pyright, jdtls)
- Runtime trace ingestion (OTLP)
- Multi-provenance confidence model
- Merkle-DAG snapshots with temporal diff
- Edge event history
- Cross-repo edge resolution
- Native Go implementation (CGC is Python; knowing is a compiled binary)
- Daemon with background enrichment

**Assessment:** CGC is closer to knowing's architecture than GitNexus (both are graph-for-code tools exposed via MCP). CGC's key advantages are language breadth (20 languages) and database flexibility. knowing's advantages are provenance sophistication, LSP enrichment, and runtime trace integration. Both now support SCIP ingest (knowing via `knowing ingest-scip` at 0.95 confidence).

---

### Axon Code Intelligence

**Verified: NOT FOUND.** Searched GitHub for "axon code intelligence", "axon-ai code", "axon code intelligence graph". No relevant repository found.

[UNVERIFIED] There may be a product by this name that is not on GitHub, or it may be too new/niche to have public presence, or it may go by a different name. Could not verify any claims. Marking this entire section as unverifiable.

---

## Tier 2: MCP Code Search

### Octocode MCP

**Source:** [github.com/bgauryy/octocode-mcp](https://github.com/bgauryy/octocode-mcp)
**Verified from GitHub API + README on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 828 |
| Language | TypeScript |
| License | MIT |
| Created | 2025-06-05 |
| Last push | 2026-05-13 |

**What it does:** MCP server for semantic code research and context generation. Searches across public and private GitHub/GitLab repos. Combines remote code search (GitHub API) with local tools (LSP, file browsing) and a skills system for agent workflows.

**Key features (verified):**
- GitHub + GitLab code search (public and private, permission-based)
- Local tools: code search, directory browsing, file finding
- LSP intelligence: go-to-definition, find-references, call hierarchy
- Agent Skills marketplace (Researcher, Engineer, Plan, RFC Generator, PR Reviewer, Doc Writer, etc.)
- Multi-phase research sessions with state persistence
- CLI installer with OAuth setup

**What they have that knowing doesn't:**
- Remote code search (GitHub/GitLab API integration)
- Agent skills/workflow system (9 pre-built skills)
- PR review skill
- RFC generation skill
- GitHub OAuth integration
- Broader scope (search + research + planning, not just graph)

**What knowing has that they don't:**
- Persistent knowledge graph (Octocode is search-oriented, not graph-oriented)
- Incremental indexing with change detection
- Multi-provenance edge model
- Runtime trace ingestion
- Temporal snapshots and semantic diff
- Cross-repo edge resolution
- Blast radius analysis from graph structure
- Confidence scoring on relationships

**Assessment:** Octocode is more of a "search + skills" platform than a knowledge graph. It complements rather than competes with knowing. An agent could use Octocode for remote search and knowing for local structural analysis. The skills system is a good product pattern worth noting.

---

### CodePathFinder

**Source:** [github.com/grabowskit/codepathfinder](https://github.com/grabowskit/codepathfinder)
**Verified from GitHub API on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 1 |
| Language | Python |
| License | NOASSERTION |
| Created | 2026-03-10 |
| Last push | 2026-04-08 |

**What it does:** Semantic code indexing, search, and AI-powered chat platform. Indexes GitHub repos, searches by intent with Elasticsearch ELSER, integrates with LibreChat + MCP.

**Assessment:** Extremely early stage (1 star, 1 month old). Not a meaningful competitive threat. Uses Elasticsearch for semantic search rather than building a code graph. Noted for completeness but not worth tracking.

---

## Tier 3: Context Packing

### Repomix

**Source:** [github.com/yamadashy/repomix](https://github.com/yamadashy/repomix)
**Verified from GitHub API + README on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 24,897 |
| Language | TypeScript |
| License | MIT |
| Created | 2024-07-13 |
| Last push | 2026-05-16 |

**What it does:** Packs an entire repository into a single AI-friendly file (XML, Markdown, or plain text). Designed for feeding codebases to LLMs as context.

**Key features (verified):**
- One-command repo packing (`npx repomix`)
- Token counting per file and total
- `.gitignore` / `.repomixignore` awareness
- Secretlint integration for sensitive data detection
- `--compress` mode using Tree-sitter to extract signatures only (reduces tokens)
- MCP server mode (`repomix --mcp`)
- Web interface at repomix.com
- Browser extension (Chrome + Firefox)
- VSCode extension (community)
- Output formats: XML, Markdown, plain text
- Remote repo support (`repomix --remote user/repo`)

**What they have that knowing doesn't:**
- Massive adoption (25k stars)
- Dead-simple UX (one command, one file)
- Web interface, browser extension, VSCode extension
- Token counting and context window awareness
- MCP server mode
- Secret detection
- Multiple output formats

**What knowing has that they don't:**
- Structural understanding (Repomix is text packing, not relationship analysis)
- Persistent graph (Repomix output is ephemeral)
- Incremental updates (Repomix regenerates from scratch every time)
- Edge relationships (calls, imports, implements, references)
- Provenance and confidence scoring
- Runtime trace integration
- Temporal snapshots and diff
- Blast radius analysis
- Cross-repo edge resolution

**Assessment:** Repomix and knowing solve fundamentally different problems. Repomix is "dump everything into the context window." knowing is "build a queryable graph so you only need to look up what matters." They are complementary, not competitive. However, Repomix's `--compress` mode (Tree-sitter signature extraction) is interesting because it represents a halfway point: structural awareness without a persistent graph. For small-to-medium repos where the whole thing fits in context, Repomix is "good enough" and much simpler. knowing's value proposition only kicks in when repos are too large for context packing, or when you need temporal/runtime/cross-repo analysis.

---

### code2prompt

**Source:** [github.com/mufeedvh/code2prompt](https://github.com/mufeedvh/code2prompt)
**Verified from GitHub API + README on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 7,343 |
| Language | Rust |
| License | MIT |
| Created | 2024-03-09 |
| Last push | 2026-04-14 |

**What it does:** CLI tool to convert codebases into a single LLM prompt. Built in Rust for speed. Has a broader ecosystem: Rust core library, CLI, Python SDK, and MCP server.

**Key features (verified):**
- CLI + TUI (terminal user interface)
- Rust core library (high performance)
- Python SDK (PyPI: code2prompt-rs) for AI agent integration
- MCP server mode
- Smart filtering with glob patterns and .gitignore
- Handlebars templates for prompt customization
- Token tracking
- Git integration (diffs, logs, branch comparisons in prompts)
- Smart file reading (CSV, Notebooks, JSONL)

**What they have that knowing doesn't:**
- Python SDK for programmatic use
- Handlebars templating for custom prompts
- Git diff/log inclusion in context
- TUI interface
- Smart file format reading (CSV, notebooks)
- Blazing fast Rust implementation

**What knowing has that they don't:**
- Same as Repomix comparison: structural graph vs text dump
- All graph capabilities (edges, provenance, snapshots, runtime, etc.)

**Assessment:** Similar to Repomix. Context packing, not structural analysis. code2prompt's Python SDK and MCP server make it a viable "context feeding" tool for agents, but it has no concept of relationships between code symbols. Not a direct competitor; potentially complementary.

---

### Aider repo-map

**Source:** [github.com/Aider-AI/aider](https://github.com/Aider-AI/aider) (repo-map is a feature, not a standalone tool)
**Verified from GitHub API + aider.chat/docs/repomap.html on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 44,872 |
| Language | Python |
| License | Apache-2.0 |
| Created | 2023-05-09 |
| Last push | 2026-05-16 |

**What it does:** Aider is an AI pair programming tool. Its repo-map feature creates a concise map of the git repository showing the most important classes and functions with their signatures. Uses a graph ranking algorithm to select the most relevant portions of the map for the current task.

**Key features (verified from docs):**
- Extracts classes, functions, methods with type signatures from all repo files
- Graph-based relevance ranking (files as nodes, dependency edges, PageRank-style algorithm)
- Sends only the most relevant portions to the LLM (context window aware)
- LLM can request to see full files based on map entries
- Uses Tree-sitter for parsing

**What they have that knowing doesn't:**
- Relevance ranking (graph algorithm to select what matters for the current task)
- Tight integration with an AI coding workflow (repo-map is part of the edit loop)
- Massive adoption (45k stars for Aider overall)
- Context-window-aware truncation

**What knowing has that they don't:**
- Persistent graph (Aider's repo-map is regenerated per session)
- Edge types and relationship semantics (Aider's graph is for ranking, not querying)
- Multi-provenance confidence model
- Runtime trace integration
- Temporal snapshots and change history
- Cross-repo analysis
- MCP server for third-party tool access
- Blast radius, semantic diff, PR impact tools

**Assessment:** Aider's repo-map is the most intellectually interesting comparison because it uses a graph internally but for a different purpose: selecting the right context, not providing a queryable system of record. The PageRank-style ranking algorithm is clever and something knowing could learn from. However, Aider's graph is ephemeral and task-specific; knowing's graph is persistent and temporal. They solve different problems: Aider answers "what code is relevant to this edit?" while knowing answers "what are the structural relationships in this codebase, and how do they change over time?"

---

## Tier 4: Platform Solutions

### Sourcegraph Cody

**Source:** [sourcegraph.com/cody](https://sourcegraph.com/cody) and [sourcegraph.com/pricing](https://sourcegraph.com/pricing)
**Verified from website on 2026-05-15.**

| Field | Value |
|-------|-------|
| Type | Commercial platform |
| Pricing | Enterprise: starting at $16K (includes credits, scales with team size) |
| Deployment | Single-tenant cloud or self-hosted |

**What it does:** AI coding assistant backed by Sourcegraph's code search infrastructure. Uses Sourcegraph's Search API to pull context from local and remote codebases. Supports VS Code, JetBrains, Visual Studio, web app, and CLI.

**Key features (verified):**
- AI chat with codebase context
- Auto-edit (suggests changes based on cursor movement)
- Customizable prompts
- Code Search (symbol search, code navigation)
- Deep Search (AI-powered, agentic natural language search)
- Batch Changes (large-scale cross-repo changes)
- Code Insights (high-level metrics and analytics)
- MCP Server
- GraphQL and REST APIs
- CLI access
- Works with Claude Code, Cursor, Codex, Amp
- Context Filters (control what context is used)
- Enterprise admin and security

**What they have that knowing doesn't:**
- Massive enterprise platform with search, insights, batch changes
- AI-powered deep search (natural language)
- Code navigation across entire organizations
- IDE integrations (VS Code, JetBrains, Visual Studio)
- Enterprise security, SSO, compliance
- Funded commercial product with enterprise sales
- MCP server for agent integration
- Cross-repository search at scale

**What knowing has that they don't:**
- Content-addressed graph with explicit edge types and provenance
- Runtime trace ingestion (static + dynamic relationship analysis)
- Temporal snapshots with Merkle-DAG diff
- Multi-provenance confidence model
- Edge event history
- Open source and self-hostable without enterprise licensing
- Lightweight: single binary, SQLite, no infrastructure dependencies
- Focused on "system of record" for code relationships, not "AI assistant"

**Assessment:** Sourcegraph is the 800-pound gorilla. Cody is one feature of a much larger platform. knowing cannot compete with Sourcegraph on breadth, enterprise features, or search capability. However, knowing occupies a different niche: a lightweight, embeddable, persistent graph that any tool (including Cody) could consume. Sourcegraph's graph is implicit in their search index; knowing's graph is explicit and queryable. The key question is whether explicit, typed, confidence-scored relationships provide enough incremental value over Sourcegraph's search-based context retrieval.

---

### Greptile

**Source:** [greptile.com](https://www.greptile.com)
**Verified from website on 2026-05-15.**

| Field | Value |
|-------|-------|
| Type | Commercial SaaS |
| Pricing | Pro: $30/seat/month (50 reviews/seat included, $1/additional). Enterprise: custom. |
| Focus | AI code review |

**What it does:** AI code review platform. Builds a graph index of the codebase, then uses a swarm of agents to review PRs and catch issues that go beyond the diff.

**Key features (verified):**
- Graph index of codebase (files, functions, dependencies)
- Swarm of parallel review agents
- Learns coding standards from team PR comments over time
- Custom review rules (plain English)
- IDE integration: one-click send issues to Claude Code, Cursor, Codex, Devin
- Greptile MCP (share review context with any agent)
- Claude Code Plugin
- `/greploop` command for iterative agent review
- TREX: autonomous test generation agent (early access)
- Free for OSS projects
- 50% startup discount (pre-Series A)

**What they have that knowing doesn't:**
- PR review workflow (the core product)
- Learning from human PR comments
- Custom rules in plain English
- Agent swarm architecture
- TREX test generation
- Commercial product with sales, customers (Brex case study)
- IDE integrations for fix-in-place

**What knowing has that they don't:**
- Open source, self-hostable
- Multi-provenance confidence model
- Runtime trace ingestion
- Temporal snapshots with Merkle diff
- Not limited to review workflow (general-purpose graph)
- MCP tools for arbitrary graph queries (blast radius, trace dataflow, etc.)

**Assessment:** Greptile uses a code graph internally but as an implementation detail of their review product, not as a queryable system of record. Their "graph index" powers agent context, similar to how knowing's graph powers MCP tools. The key insight from Greptile is that the graph-based review use case has clear commercial value ($30/seat/month). knowing could potentially power a similar review workflow but would need a product layer on top.

---

### DeepWiki

**Source:** [github.com/AsyncFuncAI/deepwiki-open](https://github.com/AsyncFuncAI/deepwiki-open)
**Verified from GitHub API + README on 2026-05-15.**

| Field | Value |
|-------|-------|
| Stars | 16,356 |
| Language | Python |
| License | MIT |
| Created | 2025-04-30 |
| Last push | 2026-04-21 |

**What it does:** AI-powered wiki generator for GitHub/GitLab/Bitbucket repositories. Clones a repo, creates embeddings, generates documentation with AI, creates Mermaid diagrams, organizes into an interactive wiki. Supports multiple LLM providers.

**Key features (verified):**
- Instant documentation generation from any repo
- Private repository support (PAT-based)
- AI-powered code structure analysis
- Automatic Mermaid diagrams (architecture, data flow)
- "Ask" feature: RAG-powered chat with repo
- DeepResearch: multi-turn investigation of complex topics
- Multiple model providers (Gemini, OpenAI, OpenRouter, Ollama, Azure)
- Flexible embeddings (OpenAI, Google AI, Ollama)
- Docker deployment or manual setup

**What they have that knowing doesn't:**
- Documentation generation (the core product)
- Mermaid diagram generation
- RAG-powered Q&A chat with codebase
- Multiple LLM provider support
- DeepResearch multi-turn investigation
- Visual, user-friendly web interface
- Wide provider flexibility (including local Ollama)

**What knowing has that they don't:**
- Persistent, incremental graph (DeepWiki regenerates from scratch)
- Typed edge relationships (calls, imports, implements, references)
- Runtime trace ingestion
- Temporal snapshots
- Multi-provenance confidence model
- Cross-repo edge resolution
- Blast radius and impact analysis
- MCP tools for AI agent consumption

**Assessment:** DeepWiki is about documentation generation, not structural code analysis. It uses AI to summarize code, not to build a queryable relationship graph. Complementary, not competitive. However, DeepWiki's popularity (16k stars) shows strong demand for "understand this codebase quickly," which is a use case knowing could serve through its MCP tools if the UX were more accessible.

---

### Graphiti (Zep)

**Source:** [github.com/getzep/graphiti](https://github.com/getzep/graphiti)
**Verified from GitHub API + README on 2026-05-16.**

| Field | Value |
|-------|-------|
| Stars | 26,141 |
| Language | Python |
| License | Apache-2.0 |
| Created | 2024-08-08 |
| Last push | 2026-05-14 |

**What it does:** Framework for building temporal context graphs for AI agents. Tracks how facts change over time with validity windows, maintains provenance to source data (episodes), supports both prescribed and learned ontology. Designed for agent memory: user preferences, conversation history, enterprise data that evolves. Backed by Zep (commercial offering for managed graph infrastructure).

**Key features (verified):**
- Temporal fact management with bi-temporal validity windows (valid_from, valid_until)
- Episodes (provenance): every entity and relationship traces to raw ingestion data
- Prescribed + learned ontology (define types via Pydantic or let structure emerge)
- Incremental graph construction (no batch recomputation)
- Hybrid retrieval: semantic embeddings + BM25 keyword + graph traversal
- Multiple graph backends (Neo4j, FalkorDB, Kuzu, Amazon Neptune)
- MCP server for Claude/Cursor integration
- Community detection for entity clustering
- LLM-powered entity extraction from unstructured text
- Contradiction handling with automatic fact invalidation
- arXiv paper (2501.13956) establishing academic credibility
- Commercial offering (Zep) with sub-200ms retrieval, dashboard, SDKs

**What they have that knowing doesn't:**
- Massive traction (26k stars)
- Temporal fact validity windows (explicit "this was true from X to Y")
- LLM-powered entity extraction from unstructured/conversational data
- arXiv paper for academic credibility
- Commercial product with enterprise features
- Multiple graph DB backends
- Dashboard with visualization
- Learned ontology (structure emerges from data)
- Community detection (Graphiti had it before us)

**What knowing has that they don't:**
- Code-specific deterministic extraction (no LLM hallucination in the graph)
- 12 language extractors with tree-sitter + multi-language LSP enrichment (Go, TS, Python, Java confirmed)
- Content-addressed identity (O(1) staleness via Merkle root, no explicit invalidation needed)
- Single binary, no external dependencies (they require Neo4j/FalkorDB)
- Runtime trace ingestion (OTLP gRPC for production call verification)
- Wire format efficiency (GCF 84% savings, they use standard JSON)
- Framework route detection (18 web frameworks)
- Code-aware context packing (RWR + HITS + density knapsack)
- Feedback-influenced reranking with natural expiration via CAS

**Assessment:** Graphiti is the strongest conceptual validation of knowing's architecture. They independently arrived at many of the same conclusions: temporal graphs, provenance, incremental updates, communities, MCP integration. The key differences are domain (general-purpose vs code-specific) and architecture (LLM-extracted entities + external graph DB vs deterministic extractors + embedded SQLite). 

Graphiti's temporal model is more explicit (validity windows on facts) while knowing's is more structural (Merkle snapshot chain, edge events). Both achieve temporal reasoning but from different primitives. Graphiti's approach requires LLM inference on every ingestion (expensive, hallucinatable). knowing's approach is deterministic (tree-sitter is always correct for what it can parse).

The strategic insight: Graphiti proves that temporal knowledge graphs for AI agents are a viable, fundable category (26k stars, hired engineers, arXiv paper, enterprise product). knowing occupies the same category but for a different data domain (code relationships vs general knowledge). This is complementary, not head-to-head: an agent could use Graphiti for user/business context and knowing for codebase structure.

---

## Competitive Summary Matrix

| Capability | knowing | GitNexus | CGC | Graphiti | Octocode | Repomix | code2prompt | Aider | Cody | Greptile | DeepWiki |
|-----------|---------|----------|-----|----------|----------|---------|-------------|-------|------|----------|----------|
| Persistent graph | Yes | Yes | Yes | Yes | No | No | No | No | Implicit | Implicit | No |
| Incremental indexing | Yes | Yes | Yes (watch) | Yes | N/A | No | No | No | Yes | Yes | No |
| Multi-provenance confidence | Yes | No | No | No | No | No | No | No | No | No | No |
| LSP enrichment | Yes | No | No | No | No | No | No | No | No | No | No |
| Runtime trace ingestion | Yes | No | No | No | No | No | No | No | No | No | No |
| Temporal model | Merkle snapshots | No | No | Validity windows | No | No | No | No | No | No | No |
| Cross-repo edges | Yes | Enterprise | No | N/A | Yes (search) | No | No | No | Yes | No | No |
| MCP server | Yes (23 tools) | Yes | Yes | Yes | Yes | Yes | Yes | No | Yes | Yes | No |
| Community detection | Yes (Louvain) | No | No | Yes | No | No | No | No | No | No | No |
| Feedback loop | Yes | No | No | No | No | No | No | No | No | No | No |
| Content-addressed | Yes (Merkle DAG) | No | No | No | No | No | No | No | No | No | No |
| Language breadth | 25 extractors | Many | 20 | N/A (general) | N/A | N/A | N/A | Many | Many | Many | N/A |
| Browser/Web UI | No | Yes | Yes (viz) | Dashboard | No | Yes | No | No | Yes | Yes | Yes |
| Traction (stars) | <100 | 38.5k | 3.3k | 26.1k | 828 | 24.9k | 7.3k | 44.9k | N/A | N/A | 16.4k |
| License | MIT | NC only | MIT | Apache | MIT | MIT | MIT | Apache | Proprietary | Proprietary | MIT |

---

## knowing's Unique Advantages

1. **Multi-provenance confidence model.** No other tool in this landscape scores edges with graduated confidence based on extraction method. ast_inferred (0.7) -> lsp_resolved (0.9) -> ast_resolved (1.0) -> otel_trace (variable). This is unique and architecturally significant for agents that need to know how much to trust a relationship.

2. **Runtime trace integration.** knowing is the only tool that bridges static analysis and runtime observation in a single graph. OTLP ingestion, confidence decay, dead route detection, and the ability to see "this function is called in production 1000 times/day" alongside "this function calls these other functions" is a genuine differentiator.

3. **Temporal snapshots with Merkle-DAG.** Content-addressed snapshots that chain together and support diff. No other tool in this space offers "show me what edges changed between this commit and that commit" or "when did this dependency first appear."

4. **Edge event history.** Recording when edges were added or removed, tied to specific commits and snapshots. This supports temporal reasoning that no competitor offers.

5. **Cross-repo edge resolution.** Automatic retargeting of dangling edges across repository boundaries. Combined with multi-repo indexing, this enables organization-wide dependency graphs.

6. **Lightweight, self-contained deployment.** Single Go binary, SQLite, no external databases or services required. Contrast with CGC (Python + graph DB) or Sourcegraph (entire platform).

7. **Multi-language LSP enrichment.** Automatically detects and uses language servers (gopls, typescript-language-server, pyright, jdtls, rust-analyzer, OmniSharp) to upgrade tree-sitter edges from 0.7 to 0.9 confidence and discover new cross-file reference edges. Tested: 83-99% upgrade rates across Go, TS, Python, Java. No other tool in this space does LSP-based edge enrichment.

8. **Retrieval explainability.** `knowing why <symbol>` and `explain_symbol` MCP tool show exactly why a symbol ranked where it did: seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback, session boost, and equivalence class matches. No other tool in this space offers ranking transparency.

---

## knowing's Remaining Gaps vs the Field

1. **Traction and community (CRITICAL).** GitNexus has 38.5k stars. Repomix has 25k. Aider has 45k. knowing has effectively zero public traction. The window to establish mindshare is closing as GitNexus and CGC grow rapidly.

2. **No browser/web UI or visualization.** GitNexus, CGC, Repomix, and DeepWiki all have visual interfaces. CGC generates interactive HTML graph visualizations. DeepWiki generates Mermaid diagrams. knowing is CLI + MCP only.

3. **No PR review automation.** Greptile proves the commercial value of graph-powered PR review ($30/seat/month). knowing has `pr_impact` and `semantic_diff` MCP tools but no GitHub Action or PR comment automation.

4. ~~**No SCIP support.**~~ **CLOSED.** `knowing ingest-scip` shipped.

5. ~~**No feedback loop.**~~ **CLOSED.** `feedback` MCP tool shipped with FeedbackProvider wired into context engine.

6. ~~**Event and schema edges.**~~ **CLOSED.** Event/MQ + OpenAPI/JSON Schema extractors shipped.

### Gaps Closed (no longer competitive weaknesses)

| Gap | Was | Resolution |
|-----|-----|-----------|
| Language support | "Only Go and Python" | 12 languages + 13 infra formats (25 extractors), 18 framework detectors, all registered |
| Agent hooks | "MCP tools are passive" | 5 hooks, proven net-positive (+305 tokens after HITS fix) |
| Relevance ranking | "Returns all results without prioritization" | RWR + HITS reranking + density-ranked knapsack packing |
| Auto-generated context | "No CLAUDE.md generation" | `knowing init` produces progressive-disclosure breadcrumbs |
| Test selection | "Nobody has this" | `knowing test-scope` (BFS backward from changed symbols) |
| Wire format | "Generic JSON" | GCF (84% savings) + GCB (74% byte savings) + session statefulness |
| PR-scoped context | "No PR analysis" | `context_for_pr` MCP tool with RWR scoring |

---

## Strategic Recommendations

### Immediate (next 4 weeks)

1. **Ship v0.1.0 release.** The system is feature-complete for a first release. Homebrew tap, npm/pypi wrappers, Docker images, and goreleaser config all exist. Needs CI secrets configured. Getting distribution live is the #1 adoption blocker.

2. **Create a GitHub Action for PR impact comments.** Greptile charges $30/seat for this. knowing can do it for free with a thin wrapper around `pr_impact` + `blast_radius`. Ship it and market it. This is the highest-signal demonstration of the graph's value.

3. ~~**Add agent feedback loop tool.**~~ **CLOSED.** `feedback` MCP tool shipped.

### Medium-term (next quarter)

4. ~~**Add SCIP ingest for external dependency edges.**~~ **CLOSED.** `knowing ingest-scip` at 0.95 confidence.

5. ~~**Build event edges (Kafka/NATS/SQS).**~~ **CLOSED.** Event/MQ extractor covers Kafka/NATS/SQS/RabbitMQ across Go/TS/Python/Java.

6. **Build a minimal web visualization.** Even a static HTML export showing the graph topology would differentiate from "just another MCP server." Consider `knowing export --format dot` for Graphviz or a D3-based explorer.

### Long-term (next 6 months)

7. **Position knowing as the "data layer" for agent workflows.** The competitive landscape shows a proliferation of agent tools (GitNexus, Octocode, Greptile) that all need graph context. knowing could be the graph backend that powers multiple front-end tools, rather than competing with each one directly.

8. **Explore commercial model.** Greptile at $30/seat/month and Sourcegraph at $16k+ show that code intelligence has enterprise willingness-to-pay. A "knowing Cloud" offering with managed graph hosting, PR review bots, and team-wide dashboards could be viable.

---

## The Incremental Real-Time Graph Update Insight

[NOTE: Could not locate a specific "Ry Walker article" via web search. The Greptile blog was searched but no article matching "Ry Walker incremental real-time graph updates" was found. The following synthesizes the principle from the competitive landscape.]

The key insight that separates knowing from context-packing tools (Repomix, code2prompt) and from regenerate-from-scratch tools (DeepWiki, Aider repo-map) is **incremental, real-time graph updates**.

Most tools in this space treat codebase understanding as a batch operation: scan everything, generate output, discard state, repeat next time. This is fundamentally wasteful and prevents temporal reasoning.

knowing's architecture is built around incremental updates:
- fsnotify-based GitWatcher detects commits as they happen
- Content-hash comparison skips unchanged files
- Edge events record what changed and when
- Snapshots chain together via Merkle DAG
- Runtime traces continuously update confidence based on live traffic

This means knowing's graph is always current (seconds after a commit) rather than stale (regenerated on demand). For AI agents making multi-step edits across files, having a real-time view of how the codebase's dependency structure changes after each edit is dramatically more useful than a static snapshot.

The competitors who understand this (GitNexus with auto-reindexing, CGC with file watching, Greptile with continuous PR context) are converging on the same insight. knowing was architected for it from the start, but needs to execute on language breadth and adoption to capitalize on the advantage.

The remaining gap is **sub-commit granularity**: knowing currently only updates on git commits, not on file saves. Adding `--working-tree` support (on the roadmap but unimplemented) would enable truly real-time graph updates during active development, which no competitor currently offers.

---

## Context Delivery Mechanisms (Cross-Market Research)

Updated 2026-05-16. Research focused on HOW tools deliver context to LLMs, not just what they index.

### Delivery Model Taxonomy

Six architectural patterns exist in the market:

| Pattern | How it works | Tools using it | Token cost model |
|---------|-------------|----------------|-----------------|
| **Graph + PageRank** | Build reference graph from tree-sitter, run PageRank personalized to current task | Aider, CodeStory/Aide | Auto-push every turn, budget-fitted via binary search |
| **Embeddings + Reranker** | Chunk code, embed, retrieve nearest neighbors, rerank | Cursor, Codeium, Sourcegraph Cody | Auto or triggered, fixed top-k |
| **Hybrid retrieval** | Combine FTS + embeddings + recency + structural | Continue.dev, Sourcegraph Cody | Configurable, fills half context window |
| **Static packaging** | Dump entire repo (or selection) into one file | Repomix, code2prompt, Mentat | Manual paste, user manages budget |
| **Server-side opaque** | Proprietary backend handles all retrieval | Augment Code, Codeium/Windsurf | Server-managed, zero user control |
| **API-as-infrastructure** | Provide context retrieval as a service | Greptile | Pull-based, consumer controls budget |

### Push vs Pull Breakdown

| Tool | Push (auto-inject) | Pull (agent asks) | Notes |
|------|-------------------|-------------------|-------|
| Aider | Always pushes repo-map every turn | Agent can request full files | PageRank-personalized, 1024 default tokens |
| Cursor | Pushes in agent mode, pull via @codebase | Hybrid | Server-side, opaque pipeline |
| Continue.dev | Configurable | @codebase trigger or tool-based | Most composable architecture |
| Codeium/Windsurf | Always pushes | No explicit pull needed | Fully automatic, zero transparency |
| Augment Code | Always pushes | No explicit pull | Markets "never manually select context" |
| Sourcegraph Cody | Auto-retrieves per message | @mentions for explicit context | Uses existing search infrastructure |
| Greptile | N/A | API call | Infrastructure, not end-user tool |
| Repomix | Manual paste | N/A | Not agentic |

### Retrieval Pipeline Comparison (verified 2026-05-17)

| Tool | Retrieval Method | Embedding Model | Ranking | Unique Aspect |
|------|-----------------|-----------------|---------|---------------|
| Aider | Personalized PageRank on code reference graph | None | PageRank scores personalized to chat context | Zero-ML; 50x boost for files in conversation, 10x for identifiers in user message |
| CGC | Cypher queries + Lucene full-text index | None | Lucene BM25 + Levenshtein fuzzy | LLM composes own multi-hop Cypher queries via MCP |
| GitNexus | BM25 + semantic embeddings, RRF fusion | snowflake-arctic-embed-xs (22M, 384d, local) | RRF (k=60) | Fully client-side with community detection |
| Cody (OSS) | BM25 (symf/bluge) + LLM query rewriting | Deprecated locally | bluge BM25 + heuristic boosts | LLM translates NL to keyword queries before search |
| Continue | FTS + embeddings + repo map + recency | User-configurable (LanceDB) | Configurable reranker model | Most modular; four parallel sources with graceful degradation |
| Augment | Unknown (closed source, $252M raised) | Unknown | Unknown | Claims "full repo understanding" |
| **knowing** | 4-channel weighted RRF: tiered keywords (3x) + equivalence classes (2x) + BM25 FTS5 (1x) + embeddings (0x) | BGE-small-en-v1.5 (local, infra shipped, weight 0 pending code-tuned model) | RWR + HITS + 6-signal scoring (blast radius, confidence, recency, distance, feedback, session) | Equivalence classes (41 local concepts replacing LLM query rewriting), task memory (passive learning), 3-14x information density vs grep, multi-language LSP auto-detection |

**Key patterns observed:**

1. **Embeddings + graph walk is hard to make work together.** GitNexus/CGC have graphs but use BM25+embeddings for retrieval, not the graph for discovery. Aider uses graph (PageRank) but no embeddings. We shipped embedding infrastructure (BGE-small-en-v1.5, local ONNX) but weight it at 0 because generic models underperform equivalence classes on code vocabulary. The win will come from a code-tuned model, not a general-purpose one.

2. **Equivalence classes are a viable local alternative to both embeddings and LLM rewriting for vocabulary bridging.** Cody uses LLM query rewriting ("authentication handling" to ["auth", "middleware", "session"]). We chose equivalence classes instead: 41 curated concept groups that expand queries deterministically, locally, with no API cost and no model download. They outperform both generic embeddings and single-keyword seeding on our benchmarks.

3. **Session personalization is the strongest signal.** Aider boosts 50x for files in conversation. knowing now has this via the session tracker: exponential decay weighting on recently accessed files and symbols, session-aware scoring as one of 6 ranking signals, and symbol deduplication across multi-tool workflows.

4. **Reranking (Continue) is a second pass.** Retrieve broadly, then a model scores each result against the query. Expensive but accurate.

5. **Equivalence classes bridge the vocabulary gap without ML.** No other tool in the competitive set uses deterministic concept expansion. This is a novel approach that sidesteps the cost/complexity of embeddings for the specific problem of vocabulary mismatch in code queries.

**knowing's unique position:** Six layers no one else combines:
- Equivalence classes for vocabulary bridging (41 local concepts; no other tool has this)
- Graph walk + equivalence + BM25 + RRF (most complete local fusion pipeline)
- RWR + HITS + 6-signal ranking (blast radius, confidence, recency, distance, feedback, session)
- Task memory for passive learning (compounds across sessions without user action)
- Multi-language LSP enrichment with auto-detection (25 extractors, 12 languages + 13 infra formats)
- Information density: 3-14x vs grep (measured across 22 experiments, not claimed)

### How knowing compares

**Our current model:** Pull-only. Agent must explicitly call `context_for_task`, `context_for_files`, or `context_for_pr`. This is the simplest and most transparent model but requires the agent to know the tools exist and decide when to use them.

**Our hook experiment results (PreToolUse context injection):**
- Initial attempt: Precision 33%, net cost -910 tokens (push model failed)
- After tiered seed matching + HITS reranking: net +305 tokens (push model viable)
- Conclusion: push model works when ranking precision is high enough. The fix was not in the hook architecture but in the context engine's seed quality and HITS differentiation.

**What Aider does differently:** Aider's repo-map is PageRank-personalized to the current user message AND the files currently in chat. This dual-personalization gives tight relevance. Their binary search over ranked symbols to fit a budget is comparable to our density-ranked knapsack approach.

**Key insight from research:** The tools that successfully push context (Aider, Codeium, Augment) all use signals beyond just the file path:
- What the user just typed (Aider uses the current message as a PageRank seed)
- What files are already in the conversation (Aider boosts 50x for references from chat files)
- Session history (Codeium tracks recent edits)

Our hook sees the tool input (file path + old_string) and uses it as a seed. The HITS reranking ensures the symbols returned are structurally important to that file, not just proximate. This closed the precision gap enough to make hooks net-positive.

### Wire Format Comparison

| Tool | Format | Token savings vs JSON |
|------|--------|---------------------|
| knowing (GCF) | Text, graph-native, positional encoding | **84%** |
| Aider | Plain text tree-context (signatures + structure) | ~60% vs full file content |
| Repomix | XML/Markdown/plain (full file content) | 0% (dumps everything) |
| Cursor | Server-side formatting (opaque) | Unknown |
| Greptile | JSON API responses | 0% |
| Continue.dev | Markdown code blocks | ~10% vs raw content |

knowing's GCF format is significantly more token-efficient than any competitor's delivery format because it's the only one designed specifically for graph-structured data (local IDs, edge references, group headers). Others either dump full file content or use generic JSON.

### Strategic Implications

1. **The push model works when you have conversation-level signals.** Aider succeeds because it sees the full chat and personalizes to it. Our hooks fire too late (at tool-call time) to have this context.

2. **MCP prompts are the right integration point for knowing.** Instead of pushing context automatically, encode "call context_for_task first" into MCP prompts (refactor_safely, review_pr, investigate_dead_code). The agent uses the prompt, which includes the context call. This is the Continue.dev model: the tool is excellent, the agent learns to use it.

3. **GCF is a genuine competitive advantage.** No other tool has a dedicated token-efficient wire format for graph data. This matters most for large responses (PR impact, blast radius) where 84% savings means 6x more information in the same budget.

4. **Session statefulness is unique.** No competitor deduplicates symbols across multi-tool workflows. For an agent calling context_for_task then blast_radius then context_for_files, knowing's session dedup compounds savings that no other tool provides.

5. **The PreCompact hook is the highest-value integration.** No competitor solves the "agent forgets after compaction" problem. This is a hook that fires rarely, costs minimal tokens, and solves a real pain point.

---

## Remaining Competitive Gaps (Priority Order)

Updated 2026-05-17 after closing SCIP, cloud extractors, event/schema extractors, feedback tool, communities, viz, context engine improvements.

### Gap 1: PR Review GitHub Action
**Who has it:** Greptile ($30/seat/month), GitNexus Enterprise
**Why it matters:** Demonstrates graph value to non-agent users. Marketing surface. Revenue potential.
**Effort:** Medium (thin wrapper around pr_impact + blast_radius + GitHub API)
**Impact:** High (visible, shareable, measurable)

### Gap 2: Code-tuned Embedding Model
**Who has it:** Gortex (BM25 + ONNX MiniLM), CGC (via graph DB)
**Why it matters:** Generic BGE-small-en-v1.5 underperforms equivalence classes on code vocabulary. A code-tuned model would make the embedding channel (currently weight 0) contribute meaningfully to RRF fusion.
**Effort:** Medium (infra shipped: BGE model, ONNX runtime, FTS5 BM25, RRF fusion all working; need fine-tuned model)
**Impact:** Medium (equivalence classes + BM25 already cover most queries well)

### Gap 3: ~~Eval Framework~~ CLOSED
Shipped: 55 eval fixtures, 23 experiments, cross-repo eval on external codebases. See eval/EXPERIMENTS.md.

### Gap 4: Negative Feedback
**Who has it:** Nobody explicitly
**Why it matters:** The feedback loop only records "this was relevant." No way to say "this was noise." Negative signals sharpen ranking faster than positive-only.
**Effort:** Medium
**Impact:** High (precision improvement)

### All Gaps Closed to Date

| Gap | Was | Now |
|-----|-----|-----|
| Language support | "Only Go and Python" | 12 languages + 13 infra/cloud formats (25 extractors total) |
| Agent hooks | GitNexus had it | 5 hooks, proven net-positive (+305 tokens) |
| Wire format efficiency | Competitors use JSON | GCF at 84% savings, GCB at 74% byte savings |
| Session statefulness | Nobody had it | 47% dedup on repeated symbols |
| Relevance ranking | "No prioritization" | RWR + HITS reranking (0.35 score spread) + density-ranked knapsack |
| PR-scoped context | GitNexus had blast-radius review | context_for_pr with RWR scoring |
| Framework detection | GitNexus had more | 25 extractors (18 frameworks across 6 languages + 7 new infra extractors) |
| MCP prompts | GitNexus had "skills" | 3 prompts (refactor, review, dead code) |
| Context engine precision | Nobody measured | Proved with benchmarks, fixed with tiered seeds + HITS |
| Auto-generated CLAUDE.md | GitNexus had it | `knowing init` with progressive-disclosure breadcrumbs |
| Test selection | Nobody had it | `knowing test-scope` (BFS from changed symbols to tests) |
| Distribution | Not ready | v0.1.2 LIVE: Homebrew, npm, PyPI, Docker (GHCR + Hub), MCP Registry, go install |
| SCIP ingest | CGC had it | `knowing ingest-scip` with compiler-accurate 0.95 confidence |
| Cloud/infra extractors | Gortex had K8s/Docker | CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework |
| Event edges | Gortex had Kafka patterns | Kafka/NATS/SQS/RabbitMQ across Go/TS/Python/Java |
| Schema edges | Nobody complete | OpenAPI 3.x, Swagger 2.x, JSON Schema |
| Feedback loop | Gortex had feedback tool | `feedback` MCP tool + FeedbackProvider wired into context engine |
| Community detection | Gortex, Graphiti had it | Louvain clustering + community-annotated export + visualization |
| Interactive visualization | CGC, GitNexus had it | Sigma.js 2D + Three.js 3D, 5 views, GitHub Pages |
| Context engine precision | Nobody measured | 5-tier seeding (exact, prefix, substring, file-path, interface-aware) |
| Global config | Gortex had it | KNOWING_DB env var, global MCP config in ~/.claude.json |
| Reproducible benchmarks | Gortex had eval subcommand | 6 harnesses with auto-generated FINDINGS.md (feedback, relevance, tokens, edges, test-scope, wire) |
| v0.1.0 release | Nobody shipped yet | v0.1.2 live across 6 channels + MCP Registry |
| Multi-language LSP | "LSP enrichment centers on Go" | 4 language servers confirmed: gopls (Go), typescript-language-server (TS, 98.9% upgrade), pyright (Python, 83.1% upgrade + 15K new edges), jdtls (Java, 83.2% upgrade). Auto-detection for 6 servers. |
| Retrieval explainability | Nobody has it | `knowing why` CLI + `explain_symbol` MCP tool: full scoring breakdown (seed tier, RWR, HITS, blast radius, confidence, recency, distance, feedback, session) |

---

## Data Verification Summary

| Competitor | Verified from source? | Data quality |
|-----------|----------------------|-------------|
| GitNexus | Yes (GitHub API + README) | High |
| CodeGraphContext | Yes (GitHub API + README) | High |
| Graphiti (Zep) | Yes (GitHub API + README + arXiv) | High |
| Axon | NOT FOUND | No data |
| Octocode MCP | Yes (GitHub API + README) | High |
| CodePathFinder | Yes (GitHub API) | High (trivial project, 1 star) |
| Repomix | Yes (GitHub API + README) | High |
| code2prompt | Yes (GitHub API + README) | High |
| Aider repo-map | Yes (GitHub API + aider.chat docs) | High |
| Sourcegraph Cody | Yes (sourcegraph.com website) | High |
| Greptile | Yes (greptile.com website) | High |
| DeepWiki | Yes (GitHub API + README) | High |
