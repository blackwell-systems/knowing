# Proposal: Supply Chain Attack Detection

## Status: PROPOSED

## Commercial Strategy

Three-tier go-to-market built on the same engine:

### Tier 1: Attack Registry (free, public, open source)
A growing public collection of detected supply chain attacks. Each entry includes the
knowing detection output, a reproducible GitHub Actions workflow, and Merkle proofs.
Anyone can fork and run. This is marketing, credibility, and community contribution.

```
demos/supply-chain-attacks/
├── 2018-event-stream/          # crypto key exfiltration
├── 2021-coa-rc/                # npm account takeover
├── 2022-colors-faker/          # protest sabotage
├── 2022-ua-parser-js/          # cryptominer injection
├── 2024-xz-utils/              # multi-year backdoor
├── 2026-tanstack-shai-hulud/   # CI credential theft via OIDC
├── 2026-opensearch/            # same campaign
├── 2026-mistral-ai/            # same campaign (PyPI)
├── 2026-guardrails-ai/         # same campaign (PyPI)
└── README.md                   # "Registry of detected supply chain attacks"
```

Each entry: reproducible CI workflow + detection.json + narrative README.
Grows over time. Community can submit new attacks. SEO magnet (every attack name
is a search term). Proves the tech works publicly.

**Key distinction:** detects KNOWN, HISTORICAL attacks on OTHER people's code.
This is a demo, not protection.

### Tier 2: GitHub Actions Marketplace (self-serve, paid per-repo)
A premium GHA Marketplace item that detects NEW, UNKNOWN attacks on YOUR dependencies.

What it does:
- Runs on every PR and dependency update (continuous, not manual)
- Scans your actual dependency tree for structural anomalies
- Compares against your stored baseline (knows what "normal" looks like for your repo)
- Managed sink registry (auto-updated as new attack patterns emerge)
- Alerts when isolation score exceeds threshold
- README badge ("supply chain verified")

Pricing: $X/repo/month via GitHub Marketplace billing (no Stripe, no infra).

**Key distinction:** detects TOMORROW's attack on YOUR code. That's the product.

### Tier 3: Enterprise Platform (direct sales, contract pricing)
The full platform (private repo). For orgs that need:
- Multi-repo federation (org-wide dependency graph)
- Custom policy engine (rules specific to their architecture)
- Temporal compliance reports (prove isolation held over date ranges)
- SBOM enrichment (attach proofs to CycloneDX/SPDX)
- Alerting integrations (Slack, PagerDuty, Jira)
- SLA and dedicated support
- Dashboard and API

**Key distinction:** enterprise-grade continuous protection with governance.

### Conversion funnel

```
Developer sees attack in the news
  -> finds our registry entry (SEO, "tanstack supply chain knowing")
  -> forks the workflow, runs it (free, proves it works)
  -> thinks "I want this on MY deps"
  -> installs GHA Marketplace item (self-serve, credit card)
  -> org grows, needs federation/policy/SLA
  -> enterprise contract (sales call)
```

Registry -> Marketplace -> Enterprise. Each tier feeds the next.

---

## Motivation

The TanStack/Mini Shai-Hulud supply chain attack (May 2026) compromised 84 npm
package artifacts across TanStack, OpenSearch, Mistral AI, Guardrails AI, and UiPath.
The attack used a chained exploit: `pull_request_target` Pwn Request, GitHub Actions
cache poisoning, and runtime OIDC token extraction to publish malicious versions
through the project's own trusted-publisher binding.

The payload stole CI credentials (GITHUB_TOKEN, NPM_TOKEN, AWS keys, Vault tokens,
K8s service account tokens, EC2 metadata) and injected itself into `.github/workflows/`
for persistence. Socket detected it within 6 minutes via static analysis of the
obfuscated `router_init.js` file.

**What knowing adds that Socket cannot:** structural proof. Socket detects malicious
patterns in code. knowing proves what changed in the dependency graph between the clean
and compromised versions, cryptographically. An auditor can verify offline that a specific
outbound edge did not exist before version X and does exist after, with a Merkle proof
anchored to a snapshot root.

## Safety Model

knowing's supply chain detection is safe by design: it parses AST structure without
executing code. Tree-sitter reads the syntax tree; it never runs the JavaScript/Python/Go
it analyzes.

**Safe operations (used in this demo):**
- `git clone` at a compromised tag (source files on disk, nothing executes)
- `npm pack` (downloads tarball, no lifecycle hooks)
- `tar xzf` (extract source files)
- `knowing index` (tree-sitter AST parsing, no code execution)
- `knowing diff`, `knowing prove`, `knowing audit-supply-chain` (read-only graph queries)

**Dangerous operations (never used in this demo):**
- `npm install` (executes `prepare`/`postinstall` hooks, which is the attack vector)
- `bun install` (same: runs lifecycle hooks)
- Running any `.js` file from the compromised package directly
- Executing deobfuscated payload to "see what it does"

**Demo script safety:** All scripts use `--ignore-scripts` or `npm pack` (tarball only).
Never run on a machine with real credentials in environment variables as an extra
precaution. The demo should run in a clean CI container or disposable VM.

## Demo Outline

### 1. Index the clean version

```bash
# Clone @tanstack/react-router at the last known-good version
git clone --branch v1.120.3 https://github.com/TanStack/router clean-router
knowing index clean-router
knowing audit -proofs -o clean-audit.json
CLEAN_SNAPSHOT=$(knowing query --latest-snapshot)
```

### 2. Index the compromised version

```bash
# Clone at the compromised version (v1.120.4 or whichever carried router_init.js)
git clone --branch v1.120.4 https://github.com/TanStack/router compromised-router
knowing index compromised-router
knowing audit -proofs -o compromised-audit.json
COMPROMISED_SNAPSHOT=$(knowing query --latest-snapshot)
```

### 3. Diff: what changed structurally

```bash
knowing diff $CLEAN_SNAPSHOT $COMPROMISED_SNAPSHOT -format json -o diff.json
```

Expected findings:
- **New file:** `router_init.js` (2.3MB, no edges to any existing code)
- **New edges:** `router_init.js` -> `process.env.GITHUB_TOKEN` (reads)
- **New edges:** `router_init.js` -> `fetch("https://api.github.com/...")` (calls)
- **New edges:** `router_init.js` -> `fetch("http://169.254.169.254/...")` (calls)
- **New edges:** `router_init.js` -> `child_process.spawn` (calls, daemonization)
- **New optionalDependency:** `@tanstack/setup` pointing to a standalone git commit
- **New lifecycle hook:** `prepare` script executing `bun run tanstack_runner.js`
- **Zero edges from existing code to router_init.js** (the malicious file is isolated
  from the legitimate codebase; it only executes via install hooks)

### 4. Prove absence in clean version

```bash
# Prove that the clean version had NO edge to the exfiltration endpoint
knowing prove-absent \
  -source "%router_init" \
  -target "%getsession.org" \
  -type calls \
  -snapshot $CLEAN_SNAPSHOT \
  -o absence-proof.json

# Verify offline
knowing verify absence-proof.json
# VERIFIED: edge does not exist in snapshot
```

### 5. Prove presence in compromised version

```bash
# Prove the compromised version DOES have the credential-stealing edge
knowing prove \
  -source "%router_init" \
  -target "%github_api_user" \
  -type calls \
  -snapshot $COMPROMISED_SNAPSHOT \
  -o presence-proof.json
```

### 6. CI gate: would this have been caught?

```bash
# Simulate a CI gate that blocks new outbound network edges
knowing diff $CLEAN_SNAPSHOT $COMPROMISED_SNAPSHOT \
  | jq '.edges_added[] | select(.cross_repo == true)'

# Non-empty output = new cross-boundary dependencies = block the PR
```

## What This Proves

| Claim | How knowing proves it |
|-------|----------------------|
| Clean version had no exfiltration code | `prove-absent` with Merkle proof |
| Compromised version added credential-stealing edges | `diff` shows new edges |
| The malicious code is structurally isolated (no legitimate code calls it) | Zero inbound edges to `router_init.js` from existing modules |
| Install hooks are the sole execution vector | `prepare` lifecycle hook is the only path to `router_init.js` |
| The proof is cryptographically verifiable offline | Standard Merkle proof, 3KB JSON, any SHA-256 implementation can verify |

## Technical Requirements

### Extraction coverage needed

The demo requires knowing to extract edges from:

1. **JavaScript/TypeScript dynamic calls** (already supported via tree-sitter TS extractor)
2. **`fetch()` / `http.get()` with URL literals** (partially supported via `consumes_endpoint`)
3. **`process.env.*` access** (not currently extracted; would need a new edge type or
   annotation on the node)
4. **`child_process.spawn`** (captured as `calls` edge)
5. **`package.json` lifecycle hooks** (`prepare`, `postinstall`, etc., extracted by
   packagejsonextractor)
6. **`optionalDependencies` with git refs** (partially supported via `depends_on`)

### Gaps to fill

| Gap | Effort | Priority |
|-----|--------|----------|
| Extract `process.env.*` reads as edges | Low (tree-sitter pattern match) | High for demo |
| Extract `fetch()`/`http.get()` URL targets more broadly | Low (extend consumes_endpoint) | High for demo |
| Handle obfuscated JS (string-array rotation, control-flow flattening) | High (deobfuscation) | Low (index the deobfuscated version for the demo) |
| Extract git-ref dependencies from optionalDependencies | Low | Medium |

### Demo approach for obfuscated code

The `router_init.js` file uses `javascript-obfuscator` with string-array rotation,
hex identifiers, and control-flow flattening. Tree-sitter can parse it (valid JS) but
the extracted edges will be to obfuscated identifiers, not readable function names.

Two options:
1. **Deobfuscate first, then index.** Use a JS deobfuscator to recover readable code,
   then index the deobfuscated version. This gives clean, readable edges in the diff.
2. **Index as-is, highlight structural anomalies.** The obfuscated file will show as
   a new file with zero edges to existing code and unusual call patterns (hex identifiers,
   `eval`, `Function()` constructor). The structural isolation itself is a signal.

Recommend option 1 for the demo (clearer narrative), with option 2 as a complementary
"what does this look like in the wild without deobfuscation" section.

## Case Study 2: event-stream (2018)

The classic supply chain attack. An attacker gained maintainer access to event-stream
(2M weekly downloads), added a dependency on `flatmap-stream`, which contained obfuscated
code that targeted a specific Bitcoin wallet library (copay-dash) and exfiltrated private
keys via HTTPS.

### Why this case study matters

- Went undetected for 2 months despite massive download volume
- Demonstrates a different attack pattern: dependency injection (not CI compromise)
- The malicious code only activates in the presence of a specific other package
- Proves the system works on historical attacks, not just recent ones

### Setup

```bash
# Clean version (no flatmap-stream dependency)
git clone --branch v3.3.3 https://github.com/dominictarr/event-stream clean-es
knowing index clean-es --db clean-es.db

# Compromised version (adds flatmap-stream)
git clone --branch v3.3.6 https://github.com/dominictarr/event-stream compromised-es
knowing index compromised-es --db compromised-es.db
# Also index flatmap-stream (the injected package)
knowing index compromised-es/node_modules/flatmap-stream --db compromised-es.db --append
```

### Expected findings from `knowing audit-supply-chain`

```json
{
  "summary": {
    "new_dependencies": 1,
    "new_files": 3,
    "suspicious_files": 1,
    "new_outbound_edges": 3,
    "max_isolation_score": 0.95
  },
  "suspicious_files": [
    {
      "file": "node_modules/flatmap-stream/index.js",
      "isolation_score": 0.95,
      "inbound_edges": 1,
      "outbound_edges": 3,
      "endpoints_contacted": ["copay.io"],
      "processes_spawned": [],
      "env_vars_read": [],
      "note": "Only inbound edge is from event-stream (require). Outbound edges reach crypto and https."
    }
  ],
  "capability_paths": [
    {
      "from": "flatmap-stream/index.js::ReedSolomonDecoder",
      "to": "crypto.createDecipher",
      "via": ["flatmap-stream/index.js::process"],
      "type": "calls"
    },
    {
      "from": "flatmap-stream/index.js::process",
      "to": "https.request",
      "via": [],
      "type": "calls"
    }
  ]
}
```

### Proofs

```bash
# Prove clean version has NO path from event-stream to network APIs
knowing prove-absent \
  --db clean-es.db \
  --module "event-stream" \
  --sinks "https.request,http.request,net.Socket.write" \
  -o clean-isolation.json
# SUCCESS: event-stream is capability-isolated from network

# Prove compromised version HAS a path
knowing prove \
  --db compromised-es.db \
  -source "%flatmap-stream" \
  -target "%https.request" \
  -type calls \
  -o attack-path.json
# FOUND: flatmap-stream -> crypto.createDecipher -> https.request
```

### What makes this different from TanStack

| Aspect | event-stream (2018) | TanStack (2026) |
|--------|-------------------|-----------------|
| Attack vector | Social engineering (gained maintainer access) | CI exploit (Pwn Request + cache poison + OIDC) |
| Payload delivery | New dependency (`flatmap-stream`) | Modified existing files (`router_init.js`) |
| Activation | Conditional (requires copay-dash present) | Unconditional (runs on install) |
| Target | Cryptocurrency private keys | CI credentials (tokens, keys, secrets) |
| Detection by knowing | New dependency adds path to `crypto` + `https` | New file with zero inbound edges, many outbound |
| Key signal | `prove-absent` fails (new capability path) | Isolation score = 1.0 (structurally isolated file) |

### Demo script

```bash
#!/bin/bash
# demos/supply-chain-event-stream/run.sh
set -euo pipefail

echo "=== event-stream Supply Chain Detection Demo ==="
echo "=== Proving: clean version is capability-isolated ==="
echo "=== Proving: compromised version has attack path  ==="

# 1. Set up
npm pack event-stream@3.3.3 && tar xzf event-stream-3.3.3.tgz && mv package clean-es
npm pack event-stream@3.3.6 && tar xzf event-stream-3.3.6.tgz && mv package compromised-es
cd compromised-es && npm install --ignore-scripts && cd ..

# 2. Index
knowing index clean-es --db clean-es.db
knowing index compromised-es --db compromised-es.db

# 3. Diff
echo ""
echo "=== STRUCTURAL DIFF ==="
knowing diff \
  $(knowing query --db clean-es.db --latest-snapshot) \
  $(knowing query --db compromised-es.db --latest-snapshot) \
  --db combined.db

# 4. Isolation proof (clean)
echo ""
echo "=== CLEAN: Capability isolation proof ==="
knowing prove-absent \
  --db clean-es.db \
  --module "event-stream" \
  --sinks "https.request,http.request,crypto.createDecipher" \
  -o clean-proof.json && echo "PROVED: event-stream v3.3.3 is isolated from network+crypto"

# 5. Attack path proof (compromised)
echo ""
echo "=== COMPROMISED: Attack path detected ==="
knowing audit-supply-chain \
  --db compromised-es.db \
  --base $(knowing query --db clean-es.db --latest-snapshot) \
  --head $(knowing query --db compromised-es.db --latest-snapshot) \
  -o attack-report.json

jq '.suspicious_files[], .capability_paths[]' attack-report.json
```

## Deliverables

1. **Two reproducible demo scripts:**
   - `demos/supply-chain-tanstack/run.sh` (2026 attack, CI credential theft)
   - `demos/supply-chain-event-stream/run.sh` (2018 attack, crypto key exfiltration)
2. **Blog post / conference talk:** both case studies, side-by-side comparison
3. **CI workflow template:** `.github/workflows/supply-chain-gate.yml`
4. **Update to whitepaper:** TanStack as Section 5b alongside event-stream in Section 5

## Timeline

- Day 1: Identify exact compromised versions, clone clean + compromised, verify indexing works
- Day 2: Fill extraction gaps (env access, fetch URLs), generate diff and proofs
- Day 3: Write blog post / demo script, create CI workflow template
- Day 4: Update whitepaper, record demo video (optional)

## Exact Implementation Plan

### Step 1: Add `reads_env` edge type

New edge type connecting a function to an environment variable it reads. This is the
critical signal for detecting credential exfiltration (the malware reads `GITHUB_TOKEN`,
`NPM_TOKEN`, `AWS_ACCESS_KEY_ID`, etc.).

```go
// internal/edgetype/constants.go
ReadsEnv = "reads_env"  // function -> env var node it reads via process.env or os.Getenv
```

Node kind for env vars: `"env_var"`. QN pattern: `"env://GITHUB_TOKEN"`.

**Extraction (TypeScript):** In `internal/indexer/tsextractor/extractor.go`, detect
`member_expression` nodes where:
- Object is `process` (identifier)
- First property is `env` (property_identifier)
- Second property is the env var name (property_identifier or bracket access with string)

Pattern: `process.env.GITHUB_TOKEN` or `process.env["GITHUB_TOKEN"]`

Tree-sitter AST:
```
member_expression
  object: member_expression
    object: identifier "process"
    property: property_identifier "env"
  property: property_identifier "GITHUB_TOKEN"
```

**Extraction (Go):** Detect `os.Getenv("VAR")` calls. Already partially captured as
`calls` edges to `os.Getenv`, but the env var name is lost. Extract the string argument
to create `reads_env` edge to `env://VAR`.

**Extraction (Python):** Detect `os.environ["VAR"]`, `os.environ.get("VAR")`, `os.getenv("VAR")`.

Implementation: ~100 LOC per language, similar pattern to `consumes_endpoint`.

### Step 2: Add `executes_process` edge type

New edge type for `child_process.spawn`, `child_process.exec`, `subprocess.run`, etc.
Differentiates legitimate process spawning from suspicious daemonization.

```go
// internal/edgetype/constants.go
ExecutesProcess = "executes_process"  // function -> process it spawns
```

**Extraction (TypeScript):** Detect calls to `spawn`, `exec`, `execSync`, `fork` on
`child_process` or `require("child_process")`.

**Extraction (Python):** Detect `subprocess.run`, `subprocess.Popen`, `os.system`, `os.exec*`.

Implementation: ~80 LOC per language.

### Step 3: Structural isolation score

Add a computed metric to `knowing diff` output: **isolation score**. A new file that has
zero inbound edges from existing code and only executes via install hooks scores 1.0
(maximally suspicious). Normal new files that are imported/called by existing code score 0.0.

```go
// internal/diff/isolation.go
type IsolationAnalysis struct {
    File            string
    InboundEdges    int     // edges FROM existing code TO this file's symbols
    OutboundEdges   int     // edges FROM this file TO external systems
    HookExecuted    bool    // runs via package.json lifecycle hook
    IsolationScore  float64 // 0.0 (well-connected) to 1.0 (completely isolated)
}
```

Formula:
```
isolation_score = (1.0 - min(inbound_edges, 5) / 5.0) * outbound_factor * hook_factor
  where outbound_factor = min(outbound_edges, 10) / 10.0
  where hook_factor = 1.5 if hook_executed else 1.0
  clamped to [0.0, 1.0]
```

A file with 0 inbound edges, 10+ outbound edges, and lifecycle hook execution = 1.0.
A file with 5+ inbound edges from existing code = 0.0 regardless of other factors.

### Step 4: `knowing audit-supply-chain` command

New CLI command that combines diff + isolation analysis + proof generation:

```bash
knowing audit-supply-chain --base <clean-snapshot> --head <current-snapshot> -o report.json
```

Output:
```json
{
  "summary": {
    "new_files": 2,
    "suspicious_files": 1,
    "new_outbound_edges": 14,
    "new_env_reads": 6,
    "new_process_spawns": 2,
    "max_isolation_score": 1.0
  },
  "suspicious_files": [
    {
      "file": "router_init.js",
      "isolation_score": 1.0,
      "inbound_edges": 0,
      "outbound_edges": 14,
      "env_vars_read": ["GITHUB_TOKEN", "NPM_TOKEN", "AWS_ACCESS_KEY_ID", ...],
      "processes_spawned": ["node", "bun"],
      "endpoints_contacted": ["api.github.com", "169.254.169.254", "getsession.org"],
      "hook_execution": "prepare: bun run tanstack_runner.js"
    }
  ],
  "proofs": {
    "absence_in_base": [...],
    "presence_in_head": [...]
  }
}
```

### Step 5: CI gate workflow

```yaml
# .github/workflows/supply-chain-gate.yml
name: Supply Chain Gate
on: [pull_request]
jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: blackwell-systems/knowing-action@v1
      - run: |
          knowing index .
          knowing audit-supply-chain \
            --base $(knowing query --snapshot-at HEAD~1) \
            --head $(knowing query --latest-snapshot) \
            --threshold 0.7 \
            --fail-on-suspicious
```

`--fail-on-suspicious` exits non-zero if any file has isolation_score > threshold.

### Step 6: Demo script

```bash
#!/bin/bash
# demos/supply-chain-tanstack/run.sh
set -euo pipefail

echo "=== TanStack Supply Chain Detection Demo ==="

# 1. Clone clean and compromised versions
git clone --depth 1 --branch v1.120.3 https://github.com/TanStack/router clean
git clone --depth 1 --branch v1.120.4 https://github.com/TanStack/router compromised

# 2. Index both
knowing index clean --db clean.db
knowing index compromised --db compromised.db

# 3. Merge into single DB for diff
knowing merge clean.db compromised.db --into combined.db

# 4. Run supply chain audit
knowing audit-supply-chain \
  --db combined.db \
  --base $(knowing query --db clean.db --latest-snapshot) \
  --head $(knowing query --db compromised.db --latest-snapshot) \
  -o report.json

# 5. Print findings
echo ""
echo "=== FINDINGS ==="
jq '.suspicious_files[] | "SUSPICIOUS: \(.file) (score: \(.isolation_score))\n  Env vars: \(.env_vars_read | join(", "))\n  Endpoints: \(.endpoints_contacted | join(", "))"' report.json

# 6. Generate proofs
echo ""
echo "=== PROOFS ==="
knowing prove-absent \
  --db clean.db \
  -source "%router_init" \
  -target "%getsession" \
  -type consumes_endpoint
echo "CLEAN VERSION: No exfiltration edge (proved absent)"

knowing prove \
  --db compromised.db \
  -source "%router_init" \
  -target "%getsession" \
  -type consumes_endpoint
echo "COMPROMISED VERSION: Exfiltration edge exists (proved present)"
```

### Implementation Order

| # | Task | Effort | Depends on |
|---|------|--------|-----------|
| 1 | `reads_env` edge type + TS extraction | 2h | Nothing |
| 2 | `executes_process` edge type + TS extraction | 2h | Nothing |
| 3 | Isolation score computation in `internal/diff/` | 3h | Nothing |
| 4 | `knowing audit-supply-chain` CLI command | 4h | 1, 2, 3 |
| 5 | Demo script with real TanStack versions | 2h | 4 |
| 6 | CI gate workflow template | 1h | 4 |
| 7 | Blog post / narrative | 4h | 5 |

Total: ~18h of implementation work. Can be parallelized (1+2+3 in parallel, then 4, then 5+6+7).

## References

- Socket report: https://socket.dev/blog/tanstack-npm-packages-compromised-mini-shai-hulud-supply-chain-attack
- TanStack postmortem: chained `pull_request_target` + cache poisoning + OIDC extraction
- Campaign tracker: https://socket.dev/supply-chain-attacks/mini-shai-hulud
- Existing whitepaper outline: `docs/research/whitepapers/supply-chain-proof-of-absence.md`
- Existing proof infrastructure: `internal/snapshot/proof.go`, `cmd/knowing/prove.go`
- Affected packages include: @tanstack/react-router, @opensearch-project/opensearch,
  PyPI mistralai, PyPI guardrails-ai, @squawk/* packages
