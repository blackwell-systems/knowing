# TanStack / Mini Shai-Hulud (May 2026)

## Summary

84 npm package artifacts in the @tanstack namespace were compromised with CI
credential-stealing malware. Part of the broader "Mini Shai-Hulud" campaign that
also hit OpenSearch, Mistral AI, Guardrails AI, and UiPath.

## Attack Vector

Chained exploit:
1. `pull_request_target` "Pwn Request" pattern (runs attacker code in base repo context)
2. GitHub Actions cache poisoning across fork-to-base trust boundary
3. Runtime memory extraction of OIDC token from GitHub Actions runner
4. Malicious npm publish via project's own OIDC trusted-publisher binding

No npm tokens were stolen. The attacker published directly through the legitimate
OIDC grant after their code ran during the workflow's test/cleanup phase.

## Payload

- `router_init.js` (2.3MB, heavily obfuscated with javascript-obfuscator)
- Targets: GITHUB_TOKEN, NPM_TOKEN, AWS_ACCESS_KEY_ID, VAULT_TOKEN, EC2 metadata,
  Kubernetes service account tokens
- Exfiltrates via `filev2.getsession[.]org` (Session network)
- Injects into `.github/workflows/` for CI persistence
- Also targets `.claude/` and `.vscode/` directories

## Detection by knowing

**Structural signals:**
- New file (`router_init.js`) with zero inbound edges from existing code
- Multiple `reads_env` edges (GITHUB_TOKEN, NPM_TOKEN, AWS_ACCESS_KEY_ID, etc.)
- `executes_process` edges (spawn with daemonization)
- `consumes_endpoint` edges to `api.github.com`, `169.254.169.254`, `getsession.org`
- Isolation score: 1.0 (maximally suspicious)
- Lifecycle hook execution: `prepare` script in optionalDependencies

**What knowing proves:**
- Clean version has NO path from any module to exfiltration endpoints (prove-absent)
- Compromised version HAS credential-reading + network-exfiltration paths (prove)
- The malicious file is structurally isolated (no legitimate code calls it)

## Timeline

- 2026-05-11: Malicious versions published to npm
- 2026-05-11: Socket AI Scanner flags all packages within 6 minutes
- 2026-05-11: TanStack team deprecates affected versions, engages npm security
- 2026-05-12: Campaign expands to OpenSearch, Mistral AI, Guardrails AI

## References

- Socket report: https://socket.dev/blog/tanstack-npm-packages-compromised-mini-shai-hulud-supply-chain-attack
- Campaign tracker: https://socket.dev/supply-chain-attacks/mini-shai-hulud
