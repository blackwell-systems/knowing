# Supply Chain Attack Detection Registry

A growing collection of supply chain attacks detected by knowing's structural analysis.
Each entry includes reproducible detection scripts, knowing output, and Merkle proofs.

## How It Works

knowing indexes code without executing it (tree-sitter AST parsing). It detects structural
anomalies: new files with zero inbound edges from existing code, outbound edges to
network/process/credential APIs, and execution via lifecycle hooks. The detection is
cryptographic: Merkle proofs verify offline that a dependency path exists or doesn't.

## Attacks Detected

| Year | Attack | Vector | Target | Detection Signal |
|------|--------|--------|--------|-----------------|
| 2026 | [TanStack/Mini Shai-Hulud](2026-tanstack-shai-hulud/) | CI OIDC token extraction | CI credentials | Isolated file, reads_env edges, executes_process |
| 2018 | [event-stream](2018-event-stream/) | Dependency injection | Crypto keys | New capability path to network API |

## Running a Detection

Each attack directory contains a `run.sh` that:
1. Downloads the clean and compromised versions (no install, no execution)
2. Indexes both with `knowing index` (AST parsing only)
3. Runs `knowing audit-supply-chain` to produce the detection report
4. Generates Merkle proofs of presence/absence

```bash
cd demos/supply-chain-attacks/2026-tanstack-shai-hulud
./run.sh
```

## Safety

All scripts use `npm pack` (tarball download) or `git clone` (source files only).
No lifecycle hooks execute. No JavaScript/Python/Rust code runs. knowing's extractors
parse the AST without execution. See the safety model in each attack's README.

## Contributing

Found a supply chain attack that knowing should detect? Open a PR with:
1. A directory named `YYYY-attack-name/`
2. A `run.sh` that downloads clean + compromised versions safely
3. A `README.md` with the attack narrative and detection signals
4. Expected `detection.json` output from `knowing audit-supply-chain`

## Related

- [Supply chain detection proposal](../../docs/proposals/supply-chain-detection-demo.md)
- [Proof of absence whitepaper](../../docs/research/whitepapers/supply-chain-proof-of-absence.md)
- [Audit and compliance guide](../../docs/guide/audit-compliance.md)
