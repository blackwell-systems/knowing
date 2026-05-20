# Proposal: Restricted Product Spin-offs

## Problem

knowing has one README, one install command, one set of GitHub topics. It speaks to four audiences simultaneously:

1. AI agent builders (context retrieval, MCP tools)
2. CI/CD engineers (test selection, architecture gates)
3. Compliance/security teams (proofs, audit reports)
4. Platform engineers (ownership routing, drift detection)

Each audience sees 75% irrelevant surface area. The CI engineer opens the README looking for test selection and finds Random Walk with Restart scoring. The auditor wants proofs and finds MCP tool documentation. The agent builder wants ranked context and finds CODEOWNERS parsing.

This dilutes discoverability (wrong keywords in search), increases cognitive load (which of these 27 subcommands do I care about?), and makes the first-five-minutes experience worse for everyone.

## Proposal

Extract three restricted-surface products from the knowing codebase. Each presents a single face to a single audience. All share the same core engine (graph store, extraction pipeline, Merkle tree) as a Go module dependency.

### Product 1: knowing (the engine)

What it is today, minus nothing. The full system. MCP server, CLI, daemon, all 27+ tools. For power users who want the whole graph.

- Audience: developers who want full code intelligence
- Install: `brew install blackwell-systems/tap/knowing`
- Keywords: code-graph, mcp, ai-agents, code-intelligence, static-analysis

### Product 2: knowing-scope

Smart test selection for CI pipelines. One job: "given these changed files, which tests must run?"

- Audience: CI/CD engineers, platform teams
- Install: GitHub Action (`blackwell-systems/knowing-scope-action@v1`) or binary
- Surface: one command (`knowing-scope <changed-files>`), one output (test list)
- Keywords: test-selection, ci, github-actions, smart-testing, affected-tests, pr-checks

What it exposes:
- `knowing-scope run --changed <files>` (outputs test packages/files to run)
- `knowing-scope diff --base main` (semantic diff of what relationships changed)
- `knowing-scope gate --deny-edge "payments->userdb"` (architecture enforcement)

What it hides:
- MCP server, context retrieval, feedback, learning, proofs, daemon, watch mode

README hook: "Run only the tests that matter. knowing-scope traces your code graph to find exactly which tests cover your changed code. 60-80% fewer test runs on large repos."

### Product 3: knowing-audit

Cryptographic proofs about code structure. For compliance reports, architecture enforcement, and CI gates that need mathematical certainty.

- Audience: security teams, compliance (SOC2, HIPAA), architecture reviewers
- Install: `brew install blackwell-systems/tap/knowing-audit` or Docker image
- Surface: four commands (prove, prove-absent, verify, report)
- Keywords: compliance, soc2, architecture-enforcement, merkle-proof, code-audit, dependency-verification

What it exposes:
- `knowing-audit prove --source X --target Y --type calls` (inclusion proof)
- `knowing-audit prove-absent --source X --target Y --type calls` (absence proof)
- `knowing-audit verify <proof-file>` (offline verification)
- `knowing-audit report --policy <policy.yaml>` (batch compliance report)
- `knowing-audit gate --deny <policy.yaml>` (CI gate, exits non-zero on violation)

What it hides:
- Context retrieval, feedback, MCP, RWR scoring, communities, session memory

README hook: "Prove code structure with cryptographic certainty. Not grep. Not 'I searched and didn't find it.' Mathematical proof that a dependency exists or doesn't exist, tied to a specific commit, verifiable offline by anyone with SHA-256."

### Product 4: knowing-own (maybe)

Code ownership queries. Who owns this file? Who wrote this function? Route this incident to the right team.

- Audience: incident response, on-call engineers, platform teams
- Install: MCP tool or CLI
- Surface: one query ("who owns X?"), one output (teams + authors)
- Keywords: codeowners, code-ownership, incident-routing, on-call, team-routing

This one is smaller and might not justify a separate repo. Could be a GitHub Action that wraps the ownership MCP tool. Evaluate after the P1 edge expansion ships.

## Architecture

```
┌─────────────────────────────────────────────────┐
│              knowing (full product)              │
│  MCP server, CLI, daemon, 27+ tools             │
└────────────────────┬────────────────────────────┘
                     │ imports
┌────────────────────▼────────────────────────────┐
│           knowing/pkg/engine (shared)            │
│  GraphStore, Extractor, MerkleTree, Snapshot    │
│  SQLite, tree-sitter, types, hashing            │
└──┬──────────────────┬───────────────────────┬───┘
   │                  │                       │
   ▼                  ▼                       ▼
knowing-scope     knowing-audit         knowing-own
(test selection)  (proofs + gates)      (ownership)
```

### Shared engine extraction

Move the core into an importable package (`pkg/engine` or a separate module `github.com/blackwell-systems/knowing-engine`). The restricted products import it and expose only their surface.

Option A: **Monorepo with multiple binaries** (`cmd/knowing`, `cmd/knowing-scope`, `cmd/knowing-audit`). Single repo, multiple install targets. Each binary imports from `internal/` but only exposes its commands. GitHub Actions wrappers live in separate repos for Marketplace listing.

Option B: **Separate repos** importing a shared module. Each repo has its own README, topics, stars, issues. More discoverability surface. More maintenance.

Option C: **Monorepo + separate "face" repos** that are thin wrappers. The face repos have READMEs, GitHub Actions workflow definitions, and installation docs. They import the monorepo as a module. Stars accumulate on the face repos (which people actually find), code lives in the monorepo.

**Recommendation: Option C.** Maximum discoverability (4 repos in search results instead of 1), minimum code duplication (face repos are <200 LOC each), single place to fix bugs (monorepo). The face repos are presentation layers, not implementation.

## Discoverability Impact

Current state (1 repo):
- 1 README competing for all keywords
- 1 set of GitHub topics (diluted across audiences)
- 1 search result for "test selection ci github action"

After split (4 repos):
- knowing-scope ranks for: test-selection, ci, affected-tests, smart-testing
- knowing-audit ranks for: compliance, soc2, code-audit, merkle-proof, architecture-enforcement
- knowing ranks for: code-graph, mcp, ai-agents, code-intelligence
- Each README speaks one language to one audience

## Cognitive Load Reduction

| Product | Commands | Concepts to learn | Time to first value |
|---------|----------|-------------------|---------------------|
| knowing (full) | 27+ | Graph, Merkle, RWR, HITS, MCP, feedback, proofs, daemon | 15-30 min |
| knowing-scope | 3 | Changed files in, test list out | 2 min |
| knowing-audit | 4 | Prove, verify | 5 min |
| knowing-own | 1 | Who owns X? | 1 min |

## Sequencing

1. **Now:** Ship the P1 edge expansion (tests, owned_by, authored_by). This is prerequisite for knowing-scope and knowing-own having real value.
2. **Next:** Extract `knowing-scope` as a GitHub Action. CI test selection is the most immediately valuable restricted product (saves real money on CI minutes, measurable ROI).
3. **Then:** Extract `knowing-audit`. Compliance use case is high-value but longer sales cycle.
4. **Later:** Evaluate knowing-own once ownership edges are battle-tested.

## What This Does NOT Change

- knowing itself stays fully featured. Nothing is removed.
- The restricted products are additive discoverability, not a fragmentation.
- All products share the same graph, same extraction, same Merkle tree.
- A user who discovers knowing-scope and outgrows it graduates to knowing naturally.

## Open Questions

1. Should the face repos contain any logic at all, or literally just a README + GitHub Action YAML + go.mod importing the monorepo?
2. Does knowing-scope need its own graph, or does it require knowing to be running (daemon dependency)? Probably: self-contained binary that indexes on first run, caches the graph in `.knowing/`, updates incrementally.
3. Naming: `knowing-scope` vs `test-scope` vs `affected-tests`? The `knowing-` prefix ties them together but might hurt independent discoverability.
4. Pricing: all MIT? Or freemium (open core for knowing, restricted products free, cloud offering paid)?
