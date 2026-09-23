# Multi-Agent Feedback Semantics

How knowing's learned-feedback layer behaves when two or more agents or harnesses
share one graph — the common case once an instance is served over HTTP
(`knowing mcp --http`) instead of one stdio process per client.

## The split: what is safely shared vs. what is stateful

knowing's graph is **content-addressed**: every node and edge is identified by
the hash of its content, so the graph is deterministic and safely shared by any
number of readers with no coordination. The **feedback layer is stateful** —
it records observations about which symbols were useful — so it is where
concurrency and semantics actually matter.

The design principle is the same one the graph uses: **treat feedback as
provenance-carrying, append-only observations. Never discard information at
write time; decide scope at read time.** That resolves the "shared vs. isolated"
question without a lossy commitment — the same data expresses both.

## Two layers, two rules

### 1. Ephemeral attribution state is per-connection (correctness)

Deciding *what* feedback to record is inherently per-client. The attribution
pipeline tracks which symbols were returned to a specific agent, and under which
task keywords, so that when that agent later edits code, the symbols it uses are
credited to *its* query — not another agent's.

This state lives in a `sessionState` keyed by the MCP session id
(`server.ClientSessionFromContext(ctx).SessionID()`), created on first use and
evicted on `OnUnregisterSession`. It holds:

- `lastTaskKeywords` — the client's most recent `context_for_task` keywords
- `lastPacks` — the client's returned packs (for delta encoding)
- `ctxSession` — session-recency retrieval boosts
- `implicit` — the pending-attribution tracker (returned → used/unused)
- `gcfSession` — GCF cross-call dedup for that client

Sharing this across clients is a bug, not a policy choice: unsynchronized maps
race (a concurrent Go map write panics the whole process), and cross-client
attribution silently records wrong `(keyword → symbol)` associations —
poisoning the learned signal. Per-connection isolation makes attribution
correct by construction. A single stdio client (one session) behaves exactly as
before.

### 2. Durable feedback is pooled by default, with provenance (semantics)

The persisted signal — the `feedback` and `vocab_associations` tables — is
**pooled across all agents**: `FeedbackBoosts` aggregates with `SUM(useful),
COUNT(*)` over every contributor, so a symbol another agent found useful helps
the next agent too. This collective learning is the point ("gets smarter with
scale"), and it is already concurrency-safe: feedback rows are append-only,
vocab rows use atomic `ON CONFLICT` upserts, and WAL + `busy_timeout` serialize
writers.

Every record now carries **provenance**: the client/harness identity from the
MCP `initialize` handshake (`GetClientInfo().Name`, e.g. `claude-code`,
`hermes`) is stored as the feedback `session_id` instead of the former
hardcoded `"implicit"`. This is non-lossy: because the contributor is recorded,
a future per-agent or trust-weighted read policy is a read-path change with no
migration. Pooled-shared and per-agent-namespaced both remain expressible from
the same data — neither is foreclosed.

## Why pooling is safe (threat model)

Pooling means one agent's observations affect another's ranking, which raises
the question of poisoning. Two existing mechanisms bound it:

- **Confidence weighting.** `weightedFeedbackScore` pulls a symbol's score
  toward neutral (0.5) in proportion to `1/(1+sqrt(total))`. A symbol needs
  *many* consistent observations before its weighted score moves meaningfully,
  so a single agent cannot cheaply swing a symbol with a few noisy signals.
- **Merkle-root expiration.** Vocab associations are anchored to the per-package
  Merkle root at record time; when the package changes, the root differs and the
  association is filtered out at read. Stale learning self-expires rather than
  compounding.

So the realistic failure mode is *slow drift* under sustained adversarial input,
not instant poisoning — and provenance makes such a contributor identifiable
after the fact. Isolation (a per-agent read namespace) remains available as a
future policy for untrusted-agent or CI-vs-dev separation, enabled by the
provenance already being stored.

## What was deliberately not built

No namespace-selection API or per-agent read policy ships today — no client has
needed one, and speculatively building it would be gold-plating. The rigorous,
non-lossy move is narrower: fix the concurrency/attribution bug at its root
(per-connection state) and stop discarding the provenance that keeps every
future policy a read-path change. If isolation becomes a real requirement, the
data is already there to add it without a migration.

## Disabling feedback

`--no-feedback` / `KNOWING_NO_FEEDBACK=1` (or `DisableImplicitFeedback()`) turns
off implicit recording process-wide: the flag clears `implicit` on existing
sessions and every session created afterward starts with it off.
