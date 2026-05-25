# Proposal: Product Split (Open Source Engine + Enterprise Platform)

## Decision (Session 15, 2026-05-25)

Two products. One public open-source engine, one private enterprise platform.
Supply chain protection is the wedge feature; the platform expands to full code
structure intelligence for security and compliance teams.

## The Split

```
blackwell-systems/knowing       (public, MIT/Apache)  — the engine
blackwell-systems/<platform>    (private, proprietary) — the enterprise product
```

**knowing** stays the open-source engine for AI agents and developers. All extraction,
graph primitives, MCP tools, proofs, retrieval. The community builds here.

**The platform** (name TBD, working title: "suppleye" or similar) is a private repo
that imports knowing as a Go module and adds: continuous monitoring, policy engine,
managed registries, alerting, multi-repo federation, compliance reporting, customer
dashboard. This is where revenue comes from.

### Architectural Justification

This split follows the Artifact-Boundary Productization framework
(see: blog/content/posts/artifact-boundary-productization.md).

**The artifact contract:** knowing produces content-addressed graph snapshots (Merkle
roots, edge hashes, hierarchical trees). These are durable, self-contained artifacts
that survive after indexing stops. The enterprise product consumes these artifacts
to produce intelligence (anomaly detection, compliance reports, policy evaluation).

**The tests pass:**
- Air-Gap Test: the enterprise product can analyze a snapshot on any machine with
  only the graph DB file. No runtime connection to knowing needed.
- Shutdown Test: if knowing never indexes again, the enterprise product still generates
  reports from existing snapshots.
- Control Flow Test: the enterprise product never affects indexing decisions or
  retrieval results. It observes, it never participates.
- Trust Test: users trust knowing (OSS) regardless of whether the enterprise product
  exists. The platform produces correct graphs without any commercial dependency.

**The planes:**
- Data Plane: tree-sitter extractors, file walking, git operations (knowing)
- Control Plane: retrieval pipeline, MCP server, daemon (knowing)
- Intelligence Plane: continuous monitoring, policy engine, alerting, compliance (enterprise)

The intelligence plane operates entirely on artifacts (snapshots) produced by the
control plane. The dependency is one-directional: enterprise imports knowing, never
the reverse.

### Identity

The platform is: **"continuous code structure intelligence for security and compliance teams."**

Supply chain protection is the first headline feature (timely, differentiated, urgent).
But the product is the platform, not the feature. Modules expand over time:

```
<platform>/
├── cmd/<platform>          # main binary
├── modules/
│   ├── supply-chain/       # supply chain monitoring (ships first, the wedge)
│   ├── architecture/       # architecture enforcement gates (ships second)
│   ├── compliance/         # temporal compliance reports (ships third)
│   └── drift/              # runtime vs static divergence (ships fourth)
├── policy/                 # policy engine (shared across modules)
├── alerting/               # notification integrations (shared)
├── registry/               # managed sink/rule registry (shared)
├── federation/             # multi-repo graph sync protocol
├── dashboard/              # customer-facing web UI / API
└── licensing/              # license key enforcement
```

### What lives where

| Capability | knowing (public) | Platform (private) |
|-----------|-----------------|-------------------|
| Extraction (all languages, all edge types) | Yes | Imports from knowing |
| `prove`, `prove-absent`, `diff`, `fsck`, `audit` | Yes | Imports from knowing |
| `reads_env`, `executes_process` edge types | Yes | Imports from knowing |
| Isolation score computation | Yes | Imports from knowing |
| Basic `knowing audit-supply-chain` (manual, single repo) | Yes | Uses same logic |
| Continuous monitoring daemon | No | Yes |
| Policy engine (custom rules per org) | No | Yes |
| Managed sink registry (auto-updated) | No | Yes |
| Multi-repo federated proofs | No | Yes |
| Temporal compliance reports | No | Yes |
| Alerting (Slack, PagerDuty, Jira, webhook) | No | Yes |
| SBOM enrichment (CycloneDX/SPDX) | No | Yes |
| Architecture enforcement (continuous) | No | Yes |
| Runtime drift detection (continuous) | No | Yes |
| Customer dashboard / API | No | Yes |
| License key enforcement | No | Yes |

### Pricing tiers (draft)

| Tier | What | Buyer |
|------|------|-------|
| **Open source** | knowing CLI, all primitives, basic supply chain detection (manual) | Individual developers, OSS projects |
| **Team** | Platform with supply chain module, basic policy, Slack alerts | Small security teams (5-20 eng) |
| **Enterprise** | All modules, federation, temporal compliance, SBOM, SLA, support | Large orgs, regulated industries |

### Analogy

Datadog started as infrastructure monitoring (the wedge), but the identity was always
"observability platform." The first feature got them in the door; the platform is what
made them $2B ARR. We start with supply chain (the wedge, because TanStack just
happened and everyone is scared) and expand to full code structure intelligence.

---

## Open Source Product Faces (within knowing repo)

The open-source repo also benefits from focused entry points for different audiences.
These are thin CLI wrappers over the same engine, not separate products.

## Problem (audience fragmentation)

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

## Architecture (as-built, no extraction needed)

```
cmd/knowing           # full CLI (27 subcommands, MCP server, daemon)
cmd/knowing-scope     # focused: test selection + architecture gates
cmd/knowing-audit     # focused: proofs + compliance reports
internal/             # shared engine (graph, extractors, Merkle, store)
```

All binaries live in one repo, import from `internal/`, share one CI pipeline, one issue tracker, one set of docs. The restricted binaries are ~100 LOC each (flag parsing + edge filter + subset of commands).

## Cognitive Load Reduction

| Product | Commands | Concepts to learn | Time to first value |
|---------|----------|-------------------|---------------------|
| knowing (full) | 27+ | Graph, Merkle, RWR, HITS, MCP, feedback, proofs, daemon | 15-30 min |
| knowing-scope | 3 | Changed files in, test list out | 2 min |
| knowing-audit | 4 | Prove, verify | 5 min |
| knowing-own | 1 | Who owns X? | 1 min |

## Recommendation: Staged Approach

Don't fragment repos before validating which wedge has traction. Stars, issues, contributors, and docs all compound in one place. Splitting early trades compound value for speculative discoverability.

### Stage 1: Focused binaries in this repo

Build restricted entry points inside the existing repo:

```
cmd/knowing          # full CLI (existing)
cmd/knowing-scope    # test selection only
cmd/knowing-audit    # proofs + compliance only
```

Each binary imports from `internal/` but exposes only its surface. One repo, multiple `go install` targets. The README keeps its "one architecture" positioning.

### Stage 2: GitHub Action wrappers

Publish `blackwell-systems/knowing-scope-action` as a thin Action wrapper that downloads and runs `knowing-scope`. The Action repo is presentation (README + action.yml + Dockerfile), not implementation. This is the first external discoverability surface.

### Stage 3: Product pages and SEO-targeted docs

Create `docs/products/` with focused landing pages:

```
docs/products/test-selection.md    # "Run only the tests that matter"
docs/products/compliance.md        # "Prove code structure with cryptographic certainty"
docs/products/ownership.md         # "Who owns this code?"
```

Each page speaks one language to one audience, links to the relevant binary, and targets distinct search keywords. These are internal to the repo but can be deployed as standalone pages.

### Stage 4: Extract public APIs only where needed

If a face repo needs to import knowing logic, extract the minimum into `pkg/`:

```
pkg/scope/    # test scope computation (graph walk + test detection)
pkg/proof/    # proof generation + verification
```

Only do this when a face repo actually exists and needs the code. Not speculatively.

### Stage 5: Create separate repos only for proven channels

If `knowing-scope-action` gets real adoption (installs, stars, issues), THEN create `knowing-scope` as a standalone repo with its own README and topics. By then you know the messaging works, the audience exists, and the split is justified.

## Why This Order

- Stage 1 costs nothing (new cmd/ directories, same CI, same tests)
- Stage 2 tests discoverability with minimal maintenance burden (one action.yml)
- Stage 3 tests messaging without code changes
- Stage 4 and 5 only happen with evidence of demand

Premature repo splitting fragments stars/issues/docs, doubles CI maintenance, and solves a problem that might not exist. The compound value of one well-positioned repo with multiple entry points outweighs the discoverability gain of four repos with 0 stars each.

## Edge-Type Filtering as Product Differentiator

The restricted products map directly to edge-type visibility:

| Product | Visible edges | What disappears |
|---------|--------------|-----------------|
| knowing (full) | All | Nothing |
| knowing-scope | calls, tests | Ownership, runtime, proofs, routes |
| knowing-audit | calls, handles_route, consumes_endpoint | Feedback, learning, runtime, ownership |
| knowing-own | owned_by, authored_by, calls | Runtime, routes, proofs |

Same graph, different projections. The binary determines which edges participate in queries. This means the restricted binaries are trivially thin: they set the edge filter and expose a focused CLI surface. No separate data model, no separate extraction.

A future visualization with edge-type checkboxes makes this tangible to users: toggle "runtime_*" off and the graph shows design intent. Toggle everything off except "gated_by_flag" and you see your feature surface. The product IS the filter.

## What This Does NOT Change

- knowing itself stays fully featured. Nothing is removed.
- The restricted binaries are additive entry points, not a fragmentation.
- All binaries share the same graph, same extraction, same Merkle tree.
- A user who discovers knowing-scope and outgrows it graduates to knowing naturally.

## Open Questions

1. Naming: `knowing-scope` vs `test-scope` vs `affected-tests`? The `knowing-` prefix ties them together but might hurt independent discoverability for the Action.
2. Does `knowing-scope` need its own index, or assume `knowing` daemon is running? Likely: self-contained binary that indexes on first run (CI use case has no daemon).
3. When does a focused doc page in `docs/products/` become worth deploying as a standalone site?
