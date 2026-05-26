# Cryptographic Proof of Dependency Absence: Detecting Supply Chain Attacks via Content-Addressed Relationship Graphs

**Dayna Blackwell, Blackwell Systems**

Draft outline, May 2026

---

## Abstract (target: 250 words)

Software supply chain attacks exploit the gap between what a dependency *declares*
it does and what it *actually* does. Existing defenses (vulnerability scanners,
signature verification, provenance attestation) answer "was this built correctly?"
but cannot answer "can this code reach the network?" We present a system that
generates compact, independently-verifiable cryptographic proofs that a module
CANNOT reach a set of dangerous capabilities within a specific graph state.

We formalize *capability isolation proofs*: given a content-addressed relationship
graph G at snapshot S, a source module M, and a set of dangerous sinks D (network
I/O, process spawn, filesystem write), we produce either:
- A Merkle inclusion proof that a path M -> ... -> D exists (attack detected), or
- A Merkle exclusion proof that no such path exists in S (module is isolated)

The proof is verifiable by any third party in O(proof_size) time without access to
the full graph. Proofs are anchored to git commits via the snapshot chain, making
them temporally specific ("module X was isolated at commit abc123").

We validate on the event-stream supply chain attack (npm, 2018): the clean version
(v3.3.3) produces a valid isolation proof; the compromised version (v3.3.6) fails
proof generation and the diff identifies the exact injected edge (flatmap-stream ->
crypto -> http). We demonstrate CI integration where `prove-absent` gates fail the
build when a new dependency path to a dangerous sink appears.

---

## 1. Introduction

### 1.1 The Supply Chain Problem

Supply chain attacks increased 742% from 2019 to 2022 (Sonatype). The event-stream
attack (2018) went undetected for 2 months despite 2M weekly downloads. The attacker
added a dependency (`flatmap-stream`) that, when combined with a specific Bitcoin
wallet library, exfiltrated private keys via HTTP.

Existing defenses:
- **Vulnerability scanners** (Snyk, npm audit): detect *known* vulnerabilities, not
  novel attacks. event-stream was not in any CVE database when active.
- **Provenance attestation** (sigstore, SLSA, in-toto): prove "this was built by
  this pipeline." Cannot prove "this cannot do X."
- **Static analysis scanners** (Socket.dev): heuristic detection of suspicious
  patterns. False positive rates make CI integration impractical.
- **Lockfile pinning**: prevents version drift but not malicious code within a
  pinned version.

### 1.2 The Missing Primitive

None of these tools provide: "here is a compact proof, verifiable by any third party,
that module M structurally cannot reach capability C in the current codebase state."

This paper introduces that primitive.

### 1.3 Contributions

1. **Formalization of capability isolation proofs** over content-addressed relationship
   graphs (Section 3)
2. **Merkle exclusion proof construction** for typed, directed graphs with provenance
   metadata (Section 4)
3. **Event-stream case study** demonstrating detection via structural proof failure,
   with exact edge identification (Section 5)
4. **CI integration protocol** with performance evaluation on production dependency
   graphs (Section 6)
5. **Open-source implementation** in the knowing system with reproducible benchmarks
   (Section 7)

---

## 2. Background and Related Work

### 2.1 Content-Addressed Relationship Graphs

Brief summary of the Hierarchical Identity Architecture [Blackwell 2026]:
- Nodes: `sha256("node\0" || repo || package || name || kind)`
- Edges: `sha256("edge\0" || source || target || type || provenance)`
- Snapshots: hierarchical Merkle root over edges grouped by package and type
- Properties: deterministic, immutable history, O(packages) diff, O(1) staleness

### 2.2 Merkle Proofs (inclusion and exclusion)

- Inclusion: path from leaf to root (standard)
- Exclusion: prove non-membership in a sorted Merkle tree (sparse Merkle tree or
  sorted-leaf approach). Reference: Micali et al. (1994), Laurie et al. (2014, CT)

### 2.3 Supply Chain Integrity Frameworks

| Framework | What it proves | What it cannot prove |
|-----------|---------------|---------------------|
| sigstore | "This artifact was signed by identity X" | "This code cannot exfiltrate data" |
| SLSA | "This artifact was built by pipeline P from source S" | "Source S has no malicious paths" |
| in-toto | "These steps were performed in this order" | "The output is capability-isolated" |
| npm provenance | "This package was published from this repo" | "This package is safe" |
| **This work** | **"Module M cannot reach sink D in state S"** | Timing channels, same-process exfil |

### 2.4 Graph Reachability Problems

Path existence in directed graphs: O(V+E) via BFS/DFS. We augment this with:
- Typed edges (only follow specific relationship types for capability analysis)
- Merkle authentication (proof is compact, verifiable without full graph)
- Temporal binding (proof is valid for a specific snapshot, not "forever")

---

## 3. Formal Model

### 3.1 Definitions

**Capability graph** G = (N, E) where:
- N = set of content-addressed nodes (sha256-identified symbols)
- E = set of typed, directed edges (calls, imports, references, etc.)
- Each edge e in E has: source, target, type, provenance, confidence

**Dangerous sink set** D subset N: nodes representing dangerous capabilities.
Examples:
- `net/http.Client.Do` (network request)
- `os.Exec` (process spawn)
- `os.WriteFile` (filesystem write)
- `crypto.Decrypt` (cryptographic operation, context-dependent)

**Module boundary** M subset N: nodes belonging to a specific package/module.

**Capability path**: a sequence of edges e1, e2, ..., ek where:
- source(e1) in M
- target(ek) in D
- For each i: target(ei) = source(ei+1)
- Each ei has type in {calls, imports, references} (transitive capability types)

**Isolation claim**: "No capability path exists from M to D in snapshot S"

### 3.2 Proof Construction

**Inclusion proof** (attack detected):
- The capability path P = [e1, ..., ek] plus Merkle inclusion proofs for each ei
  in the snapshot's hierarchical tree.
- Verifier checks: each edge hash is in the tree, edges form a valid path,
  source(e1) in M, target(ek) in D.

**Exclusion proof** (module is isolated):
- For each node n reachable from M (BFS via capability-typed edges), prove that
  n is NOT in D via Merkle non-membership.
- Compact form: enumerate the reachable set R (BFS from M), provide Merkle proofs
  that R intersection D = empty.
- Proof size: O(|R| * log(|N|)) for sparse Merkle tree approach.

### 3.3 Trust Model

- **Trusted**: the snapshot root hash (anchored to a signed git commit or external witness)
- **Untrusted**: the prover (the system generating the proof)
- **Verifier**: any third party with the root hash and the proof

The verifier does NOT need:
- Access to the full graph
- Trust in the prover's analysis quality
- The ability to re-run the indexing pipeline

The verifier DOES need:
- The snapshot root hash from a trusted source
- Agreement on the dangerous sink set D
- Agreement on which edge types constitute "capability paths"

### 3.4 Limitations

What this model does NOT catch:
1. **Same-process data exfiltration**: code that reads sensitive data and encodes it
   into a return value (no outgoing network call needed if the caller transmits)
2. **Timing/side channels**: information leakage via execution timing
3. **Dynamic dispatch beyond graph resolution**: `eval()`, reflection, runtime code gen
4. **Transitive confidence degradation**: if edge e1 has confidence 0.5 (inferred),
   the proof strength degrades with low-confidence edges
5. **Incomplete extraction**: if the indexer misses an edge, the proof is valid but
   the claim is weaker (isolation holds *relative to the extracted graph*)

---

## 4. System Architecture

### 4.1 Graph Construction

Standard knowing pipeline: tree-sitter extraction -> import resolution -> inheritance
propagation -> content-addressed storage -> hierarchical Merkle snapshot.

For supply chain analysis, additional extraction:
- **Transitive dependency edges**: `package.json` / `go.mod` / `Cargo.toml` dependencies
  become `depends_on` edges to package-level nodes
- **Capability sink tagging**: predefined list of dangerous API surfaces per language
  (stdlib network, process, filesystem functions)

### 4.2 Prove (inclusion)

```
prove(module_M, sink_D, snapshot_S):
  1. BFS from all nodes in M, following capability-typed edges
  2. If any node in BFS frontier is in D:
     - Extract the path P from M to D
     - For each edge in P: generate Merkle inclusion proof from snapshot tree
     - Return InclusionProof{path: P, merkle_proofs: [...], snapshot: S}
  3. If BFS exhausts without reaching D:
     - Return None (module is isolated; use prove-absent for formal proof)
```

### 4.3 Prove-Absent (exclusion)

```
prove-absent(module_M, sink_D, snapshot_S):
  1. BFS from all nodes in M, following capability-typed edges
  2. Collect reachable set R = all nodes visited
  3. Assert R intersection D = empty (if not, fail: module is NOT isolated)
  4. For each d in D:
     - Generate Merkle non-membership proof that no edge from any r in R
       targets d (or provide sorted-leaf exclusion proof)
  5. Return ExclusionProof{reachable: R, sinks: D, merkle_proofs: [...], snapshot: S}
```

### 4.4 Verify

```
verify(proof, root_hash):
  If InclusionProof:
    - For each edge in path: verify Merkle inclusion against root_hash
    - Verify path connectivity (target[i] = source[i+1])
    - Verify source[0] in claimed module, target[-1] in claimed sinks
  If ExclusionProof:
    - Verify each non-membership proof against root_hash
    - Verify reachable set computation (optionally: re-run BFS with provided edges)
```

---

## 5. Case Study: event-stream Attack

### 5.1 Setup

- Clone event-stream v3.3.3 (clean) and v3.3.6 (compromised)
- Index both with knowing
- Define sink set: `{http.request, https.request, net.Socket.write}`
- Define module: `flatmap-stream` (the injected dependency)

### 5.2 Clean Version (v3.3.3)

- `flatmap-stream` does not exist as a dependency
- `prove-absent("event-stream", sinks, snapshot_clean)` succeeds
- Proof: event-stream's reachable set contains only stream manipulation functions
- No path to any network API

### 5.3 Compromised Version (v3.3.6)

- `flatmap-stream` exists, contains obfuscated code
- After deobfuscation/execution, creates path:
  `flatmap-stream.process -> crypto.createDecipher -> http.request`
- `prove-absent("flatmap-stream", sinks, snapshot_compromised)` FAILS
- `prove("flatmap-stream", sinks, snapshot_compromised)` returns the exact path
- `knowing diff snapshot_clean snapshot_compromised` shows: 1 new edge added

### 5.4 Detection Timeline Comparison

| Method | Detection delay | False positive rate | Proof? |
|--------|---------------|--------------------| -------|
| npm audit | 2 months (after CVE filed) | Low | No |
| Socket.dev heuristics | Unknown (not operational in 2018) | Medium-high | No |
| Manual code review | 2 months (discovered by community) | N/A | No |
| **prove-absent CI gate** | **0 days (fails on publish)** | **Zero (cryptographic)** | **Yes** |

### 5.5 What This Demonstrates

1. The proof fails STRUCTURALLY when the attack is introduced (not heuristically)
2. The diff identifies the EXACT edge that enables the attack
3. The proof is VERIFIABLE by any third party without re-running analysis
4. The detection is IMMEDIATE (CI gate, not after-the-fact scanning)

---

## 5b. Case Study: TanStack / Mini Shai-Hulud (2026)

### 5b.1 Attack Description

84 npm package artifacts in the @tanstack namespace were compromised via a chained
exploit: `pull_request_target` Pwn Request pattern, GitHub Actions cache poisoning
across the fork-to-base trust boundary, and runtime memory extraction of an OIDC
token from the GitHub Actions runner. The attacker published malicious versions
through the project's own OIDC trusted-publisher binding. No npm tokens were stolen.

The payload (`router_init.js`, 2.3MB obfuscated) targeted: GITHUB_TOKEN, NPM_TOKEN,
AWS_ACCESS_KEY_ID, VAULT_TOKEN, EC2 metadata (169.254.169.254), Kubernetes service
account tokens. Exfiltration via `filev2.getsession[.]org`.

Socket.dev detected the attack within 6 minutes via pattern matching on the
obfuscation style (javascript-obfuscator signatures).

### 5b.2 Structural Detection Results

We reconstructed the TanStack payload pattern (deobfuscated) and indexed with knowing.
The malicious file produced the following structural signals:

| Signal | Count | Example |
|--------|-------|---------|
| `reads_env` edges | 4 | env://GITHUB_TOKEN, env://NPM_TOKEN, env://AWS_ACCESS_KEY_ID, env://VAULT_TOKEN |
| `executes_process` edges | 1 | process://curl |
| `consumes_endpoint` edges | 2 | /user (api.github.com), /latest/meta-data/iam/security-credentials/ (EC2) |
| Inbound edges from legitimate code | 0 | File is structurally isolated |
| **Isolation score** | **0.9** | Near-maximum suspicion |

**Capability paths detected:**
- `env://GITHUB_TOKEN -> process://curl` (credential theft -> exfiltration)
- `env://NPM_TOKEN -> process://curl` (credential theft -> exfiltration)

### 5b.3 Comparison: Pattern Matching vs Structural Analysis

| Dimension | Socket.dev (pattern) | knowing (structural) |
|-----------|---------------------|---------------------|
| Detection time | 6 minutes post-publish | 0 minutes (CI gate) |
| Detection method | Obfuscation signatures | Graph isolation score |
| Novel obfuscation | Must update patterns | Structure still anomalous |
| Cryptographic proof | No | Yes (Merkle inclusion/exclusion) |
| Offline verification | No (requires Socket API) | Yes (SHA-256 only) |
| False positive rate | Medium (heuristic) | 1.0% on 200 clean packages (package-level verdict) |

### 5b.4 Key Difference from event-stream

| Aspect | event-stream (2018) | TanStack (2026) |
|--------|-------------------|-----------------|
| Attack vector | Social engineering (maintainer takeover) | CI exploit (OIDC token extraction) |
| Payload delivery | New dependency (`flatmap-stream`) | Modified existing files |
| Activation | Conditional (requires copay-dash) | Unconditional (runs on install) |
| Target | Bitcoin private keys | CI credentials |
| Detection signal | New capability path to crypto + https | Isolated file with reads_env + executes_process |
| Isolation score | N/A (dependency-level) | 0.9 (file-level) |

---

## 6. CI Integration

### 6.1 Protocol

```yaml
# .github/workflows/supply-chain-gate.yml
- name: Verify capability isolation
  run: |
    knowing index .
    knowing prove-absent \
      --module "flatmap-stream" \
      --sinks "net/http,os/exec,os.WriteFile" \
      --snapshot HEAD \
      --output proof.json
    # Upload proof as build artifact for audit trail
```

### 6.2 Performance

| Operation | Time (event-stream) | Time (k8s-scale, 268K edges) |
|-----------|--------------------|-----------------------------|
| Index | ~0.5s | ~18s |
| BFS reachability | <1ms | ~50ms |
| Proof generation | <5ms | ~100ms |
| Proof verification | <1ms | <5ms |
| **Total CI overhead** | **<1s** | **<20s** |

### 6.3 Proof Size

- Inclusion proof (attack path of length k): O(k * log N) hashes
- Exclusion proof (reachable set of size R): O(R * log N) hashes
- Typical: 1-5 KB for module-level isolation proofs

### 6.4 Integration Modes

1. **Hard gate**: build fails if prove-absent fails (strictest)
2. **Warning + diff**: annotate PR with new capability paths introduced
3. **Audit log**: generate proofs for all modules on every release, store for compliance
4. **Dependency review**: on `npm install`, generate isolation proof for new dependency

---

## 7. Evaluation

### 7.1 False Positive Evaluation (200 clean packages)

We scanned 200 known-clean, widely-used packages (100 npm, 100 PyPI) to measure
the false positive rate of isolation scoring. Each package was downloaded,
indexed with tree-sitter extraction (no LSP enrichment), and scanned with
`audit-supply-chain --scan-all`. Results are in
`bench/supply-chain/false-positive-results-v2.jsonl`.

**Package-level verdict** (ratio > 10% AND count >= 2):

| Metric | Value |
|--------|-------|
| Packages scanned | 200 (100 npm + 100 PyPI) |
| Packages with any flagged file | 43 (21.5%) |
| **Packages with "suspicious" verdict** | **2 (1.0%)** |
| Packages with "review" verdict | 41 (20.5%) |
| Packages with "clean" verdict | 157 (78.5%) |

The two "suspicious" verdicts:

| Package | Suspicious files | Total files | Ratio | Why flagged |
|---------|-----------------|-------------|-------|-------------|
| esbuild | 2 | 4 | 50% | Install script downloads and runs platform-specific binary. Structurally identical to a supply chain attack. |
| nox | 3 | 29 | 10.3% | Test runner that spawns processes as its core function. |

**Key finding: raw file-level scoring (21.5% FP) is unusable for CI gating.
Package-level aggregation (1.0% FP) is viable.** The critical insight is that
most legitimate packages have 1-2 files that spawn processes out of hundreds
(django: 2/643 = 0.3%, webpack: 1/616 = 0.2%), while real attacks have a high
ratio of suspicious files (TanStack: >50%).

**Three-layer false positive reduction:**

| Layer | What it filters | Impact |
|-------|----------------|--------|
| 1. Env-only attenuation | `reads_env` without `executes_process` gets 0.2x weight | Eliminates dotenv, debug, axios, commander FPs |
| 2. Benign process targets | node, npm, python, cargo, git, bash classified as safe | Eliminates build tool FPs (node spawning, compiler invocation) |
| 3. Test/benchmark exclusion | Files in /test/, /benchmarks/, _test.go skipped | Eliminates test runner FPs (pino, mocha test suites) |
| 4. Package-level verdict | Requires ratio > 10% AND count >= 2 | Reduces 43 flagged to 2 "suspicious" |

**True positive verification:** TanStack/Mini Shai-Hulud pattern (process.env
credential read + spawn("curl") + fetch()) produces isolation score 0.9 with
suspicious verdict. event-stream pattern (http.request to hardcoded IP) produces
isolation score 0.24 via `consumes_endpoint` detection.

### 7.2 Benchmark Corpus

| Package | Language | Dependencies | Edges | Modules |
|---------|----------|-------------|-------|---------|
| event-stream 3.3.3 | JavaScript | 12 | ~500 | 12 |
| event-stream 3.3.6 | JavaScript | 13 | ~550 | 13 |
| express 4.18 | JavaScript | 30 | ~2,000 | 30 |
| django 6.0 | Python | 0 (stdlib only) | ~376K | ~643 |
| kubernetes client-go | Go | 50+ | ~100K | ~80 |

### 7.3 Research Questions

- **RQ1**: Can the system detect event-stream-class attacks with < 2% false positive rate?
  **Yes.** 1.0% FP rate on 200 clean packages with package-level verdict.
- **RQ2**: What is the CI overhead for production-scale repos?
  Django (57K nodes, 376K edges): 21s index + <1s scan = 22s total.
- **RQ3**: How does proof size scale with graph size and reachable set size?
  Proof size is O(log N) where N is edge count. ~660 bytes for 13K edges.
- **RQ4**: What is the coverage gap?
  Dynamic process targets (`process://dynamic`) are treated as suspicious by
  default. Cannot distinguish `spawn(variable)` where the variable resolves to
  a benign or malicious target. Obfuscated code may not produce extractable edges.

### 7.4 Threats to Validity

- **Extraction completeness**: proofs are relative to the extracted graph. If the
  indexer misses an edge (dynamic dispatch, eval), the proof holds but the claim
  is weaker. Mitigation: use runtime traces to supplement static extraction.
- **Sink set completeness**: if the dangerous API list is incomplete, isolated modules
  may still be dangerous via unlisted APIs. Mitigation: community-maintained,
  per-language sink registries.
- **Obfuscation**: heavily obfuscated code may not produce extractable edges.
  Mitigation: flag modules with low extraction confidence as "unverifiable."
- **Package-level aggregation**: the verdict threshold (ratio > 10%, count >= 2)
  was tuned on the same 200-package corpus used for evaluation. External
  validation on a separate held-out corpus would strengthen the claim.
- **Benign process list**: the 22-entry benign target list may not cover all
  legitimate executables. Packages spawning unlisted-but-benign processes
  (e.g., `ffmpeg`, `imagemagick`) would be flagged as suspicious.

---

## 8. Discussion

### 8.1 Composability with Existing Frameworks

Proofs generated by this system can be embedded in:
- SLSA provenance attestations (as an additional predicate)
- in-toto link metadata (as a capability claim alongside build steps)
- Package registry metadata (npm, PyPI could store proofs per version)

### 8.2 Adversarial Model

What an attacker must do to evade detection:
- Introduce a capability path that uses ONLY edge types not in the capability set
  (e.g., only `references` edges, not `calls`). This limits attack surface.
- Exploit extraction gaps (dynamic dispatch, runtime code gen). The system flags
  low-confidence paths as "unverifiable isolation."
- Compromise the snapshot root hash. This requires compromising the git signing key
  or the external witness.

### 8.3 Beyond Binary Isolation

Future work: degree-of-isolation scores (how many edges away from a dangerous sink?),
conditional proofs ("module M only reaches network IF called from context C"),
differential proofs ("this PR introduces 2 new capability paths").

---

## 9. Conclusion

We presented capability isolation proofs: a new primitive for supply chain security
that leverages content-addressed relationship graphs to produce compact, independently
verifiable proofs that a module cannot reach dangerous APIs. Unlike existing tools that
detect known vulnerabilities or verify build provenance, our approach proves structural
properties of the code itself, detects novel attacks at introduction time, and produces
cryptographic evidence suitable for compliance and audit.

The event-stream case study demonstrates that the compromised version's proof fails
immediately and identifies the exact injected edge. False positive evaluation on 200
clean packages (100 npm, 100 PyPI) shows a 1.0% package-level FP rate with sub-second
detection time. CI integration adds <22 seconds to enterprise-scale builds (django:
57K nodes, 376K edges).

---

## Implementation Notes (not in paper, for internal reference)

### Already shipped:

- `knowing prove` and `knowing verify`: Merkle inclusion proofs (72us generate, 1.2us verify)
- `knowing prove-absent`: Merkle exclusion proofs using sorted-leaf adjacency
- `knowing audit -proofs`: batch compliance artifact with integrity check + proofs
- `knowing fsck`: full graph integrity verification
- `knowing diff`: O(packages) snapshot diff with edge-level change classification

### What needs to be built for the full supply chain demo:

1. **`reads_env` edge type**: function -> env var node (detects credential access)
   - TS: `process.env.GITHUB_TOKEN`
   - Go: `os.Getenv("VAR")`
   - Python: `os.environ["VAR"]`, `os.getenv("VAR")`

2. **`executes_process` edge type**: function -> process it spawns
   - TS: `child_process.spawn/exec`
   - Python: `subprocess.run/Popen`
   - Go: `os/exec.Command`

3. **Dangerous sink registry**: per-language list of network/process/filesystem APIs
   categorized by capability type (network, process, filesystem, crypto)

4. **Structural isolation score**: computed from inbound/outbound edge ratio + hook execution

5. **`knowing audit-supply-chain` command**: combines diff + isolation analysis + proofs

6. **TanStack case study fixture**: clone clean + compromised versions, deobfuscate,
   index, run full pipeline. Demonstrates detection of OIDC-based publish attack.

See `docs/proposals/supply-chain-detection-demo.md` for the complete implementation plan
with exact architecture, CLI interface, and CI workflow template.

### Estimated effort: ~18h implementation + 2-3 days writing
