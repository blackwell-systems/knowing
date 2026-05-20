# Shared Intelligence Layer: Communities as Multi-Agent Infrastructure

**Dayna Blackwell, Blackwell Systems**

---

## Abstract

AI coding agents operate on code but learn nothing from each other. Each session starts cold, explores the same codebase from scratch, and discards its discoveries at the end. We describe an architecture where a content-addressed code graph, partitioned into emergent communities via Louvain clustering, becomes a shared learning substrate for multi-agent coordination. Agents contribute feedback about which symbols were useful for which tasks; that feedback compounds by community, making subsequent sessions in the same architectural region progressively sharper. The result is implicit specialization without configuration: the system learns which parts of the codebase matter for which types of work, organized by boundaries it discovers rather than boundaries humans declare.

---

## 1. The Cold Start Problem at Scale

Every AI coding session begins with the same question: "What part of this codebase is relevant to my task?" Current solutions fall into two categories:

**Context packing tools** (Repomix, code2prompt) answer with "everything": dump the repo into the context window and let the model sort it out. This works for small repos but collapses at scale.

**Graph tools** (code intelligence MCP servers) answer with "whatever you ask for": the agent must know which tools to call, which symbols to look up, and how to navigate the graph. This requires the agent to already understand the codebase's architecture before it can ask useful questions about it.

Neither approach learns. Session N+1 is no better than session N. The agent that spent 20 minutes exploring the context engine yesterday starts from zero today. The insight that `ForTask` calls `RandomWalkWithRestart` which calls `EdgesFrom` is discovered, used, and forgotten.

---

## 2. Communities as Emergent Architecture

The code graph already contains all the information needed to understand a codebase's architecture. Functions call functions. Types implement interfaces. Files import packages. These edges form clusters: groups of symbols that interact heavily with each other and less with the outside.

Louvain community detection discovers these clusters without configuration. Given a graph of 1500 nodes with call/import/reference edges, it partitions them into 8-15 communities in milliseconds. Each community corresponds to what a developer would call a "module" or "subsystem": the context engine, the MCP server, the indexer pipeline, the store layer.

The key insight: **communities are the natural organizational unit for agent intelligence.** They correspond to:
- The scope of a typical task (most edits stay within one community)
- The blast radius boundary (changes within a community rarely break other communities)
- The expertise boundary (understanding one community is useful for all tasks within it)
- The coordination boundary (two agents in different communities don't conflict)

---

## 3. The Learning Loop

Five components form a self-reinforcing cycle:

### 3.1 Routing (plan_turn)

An agent receives a task: "fix the bug in context ranking." The `plan_turn` tool suggests which MCP tools to use for the task. It returns tool recommendations (e.g., "Start with `context_for_task`, then use `flow_between` to trace from `RankSymbols` to `packIntoBudget`").

The agent skips exploration. It starts with the right tools and the right symbols. *(Note: `plan_turn` currently suggests tools based on task keywords; community-based routing is planned.)*

### 3.2 Context Scoping (community-aware RWR) *(Planned)*

The context engine seeds its Random Walk with Restart from the task's keywords. *(Note: the current `RandomWalkWithRestart` implementation has no community parameter and walks the full graph. Community-constrained RWR, where the walk is limited to the relevant community's symbols, is a planned enhancement. The design goal is a tighter, more relevant score distribution that doesn't leak probability mass into unrelated modules.)*

### 3.3 Work (agent uses context, modifies code)

The agent works. It reads symbols, makes edits, runs tests. Standard agent behavior.

### 3.4 Feedback (what was useful)

After the task, the agent reports which symbols were useful and which were irrelevant:

```
feedback(action: "record", symbol: "RankSymbols", useful: true)
feedback(action: "record", symbol: "NewDaemon", useful: false)
```

This feedback is persisted with merkleized expiration (v0.5.0). Feedback records use `neighborhood_root` (the package SubgraphRoot) for expiration, so feedback naturally invalidates when the package's edges change. *(Note: feedback is currently scoped by package SubgraphRoot, not by community ID. Community-scoped feedback is a planned enhancement.)*

### 3.5 Compounding (feedback-aware reranking)

The next agent that works on a context-engine task benefits from all prior feedback. FeedbackBoosts are wired into the context engine ranking pipeline: symbols with positive history get a ranking boost. The context window fills with symbols that historically mattered for this type of work.

No configuration. No manual curation. The system learns from use. *(Note: feedback boosts are currently applied globally per-symbol, not community-scoped. Community-scoped reranking is a planned enhancement.)*

---

## 4. Implicit Specialization *(Design Goal)*

Over time, as community-scoped feedback is implemented, the feedback-per-community pattern will create something that looks like specialization:

- Tasks in the "context engine" community will consistently surface `RWR`, `HITS`, `packIntoBudget`, `RankSymbols`
- Tasks in the "MCP server" community will consistently surface `handleBlastRadius`, `requireStringArg`, `Server.registerTools`
- Tasks in the "indexer" community will consistently surface `ExtractWithPackage`, `BulkLoad`, `Register`

*(Note: today, feedback boosts are global per-symbol rather than community-scoped. The per-symbol feedback already compounds across sessions, just without community boundaries.)*

The graph discovers the communities. Agents discover which symbols matter. When community-scoped feedback is complete, the two discoveries will compound into a system that routes agents to the right context faster with each session.

This is collaborative filtering applied to code intelligence: "agents who worked on similar tasks in this community found these symbols useful."

---

## 5. Multi-Agent Coordination *(Planned)*

Communities provide the infrastructure for the coordination problem for parallel agents without a central scheduler. The following describes the design; these features are not yet implemented.

### 5.1 Conflict Avoidance *(Design)*

Two agents working in the same community are likely to conflict (editing the same files, calling the same functions). Two agents in different communities are unlikely to conflict. Community membership becomes the scheduling primitive: assign at most one agent per community for parallel work.

### 5.2 Pending Mutations *(Design)*

When an agent announces it's modifying symbols in community 3, other agents working in community 3 can see the pending changes. Agents in communities 5 and 7 ignore the notification entirely. Communities scope the "who needs to know?" question.

### 5.3 Cross-Community Edges as Coordination Points *(Design)*

The edges that span communities are API boundaries. When agent A modifies a function in community 3 that has callers in community 5, only the agent working in community 5 needs to be notified. The graph provides exact notification scoping: "symbol X was modified; it has callers in communities 5 and 8."

### 5.4 Temporal Awareness *(Design)*

By tracking community structure across snapshots, the system could detect architectural drift:
- A community splits: a module became too complex
- Two communities merge: coupling increased
- A symbol moves between communities: a refactor changed module boundaries

Agents could be warned: "This file was recently moved from community 3 to community 5. Its callers in community 3 may need updating."

*(Note: temporal community tracking across snapshots is not implemented. CommunityRoot computation currently uses SubgraphRoot over package sets, not community-specific sets.)*

---

## 6. Why Content-Addressing Matters Here

The feedback loop requires trust: feedback recorded yesterday must still be valid today. Content-addressing provides this guarantee:

- **Feedback is anchored to content-addressed symbols.** If a symbol is renamed, it gets a new hash. Old feedback naturally expires because it points at a hash that no longer exists in the current graph. Feedback uses merkleized expiration (v0.5.0): the `neighborhood_root` (package SubgraphRoot) invalidates feedback when surrounding code changes.
- **Community membership is recomputable.** Run Louvain on any snapshot to get the community structure at that point in time. *(Note: feedback is not currently linked to community membership; it uses package SubgraphRoot for expiration.)*
- **Staleness is detectable.** If the graph's Merkle root hasn't changed, all cached community structures and feedback mappings are still valid. One hash comparison, not a full recomputation.

Without content-addressing, feedback accumulates indefinitely against mutable state, growing stale without detection. With it, feedback naturally decomposes as the codebase evolves: renamed symbols, restructured modules, and moved functions all invalidate their associated feedback through hash changes.

---

## 7. The Architecture Stack

```
Layer 4: Agent Coordination     pending mutations, cross-community notifications
Layer 3: Learning Loop          plan_turn -> context -> work -> feedback -> compound
Layer 2: Community Structure    Louvain clustering, community-scoped queries
Layer 1: Code Graph             content-addressed nodes, edges, snapshots
Layer 0: Source Code            git commits, file changes, runtime traces
```

Each layer depends on the one below:
- Coordination needs community boundaries (Layer 2)
- Learning needs community scoping (Layer 2) and persistent feedback (Layer 1)
- Communities need edge data (Layer 1)
- The graph needs source analysis (Layer 0)

The critical observation: **layers 0-2 are complete and operational.** Layer 3 is partially functional: `plan_turn` (tool suggestion), `feedback` (recording with merkleized expiration), and FeedbackBoosts (wired into context engine ranking) are implemented. Community-scoped feedback and community-constrained RWR walks are not yet implemented. Layer 4 (multi-agent coordination) is entirely planned, and communities provide the infrastructure it will need.

---

## 8. Comparison with Existing Approaches

| Approach | Scope of learning | Persistence | Architectural awareness |
|----------|------------------|-------------|------------------------|
| Aider repo-map | Per-session PageRank | None (regenerated) | Implicit (ranking, not explicit) |
| Cursor context | Per-session embeddings | Server-side (opaque) | None |
| CLAUDE.md files | Manual, project-wide | File (human-maintained) | None (flat text) |
| Session memory (lean-ctx) | Per-session CCP | Cross-session facts | None |
| **knowing communities** | Per-package feedback (community-scoped: planned) | Content-addressed, hash-anchored | Emergent (Louvain) |

The unique combination: learning that is both **persistent** (survives across sessions via merkleized expiration) and **structurally scoped** (feedback expires when the package SubgraphRoot changes). Community-level scoping is a planned enhancement.

---

## 9. What This Enables

### For individual developers:
"The system knows what I usually need when I work on the context engine. It stops showing me daemon code."

### For teams:
"New engineers get effective context immediately because the system learned from 100 prior sessions what matters in each module."

### For multi-agent workflows *(Planned)*:
"Five parallel agents self-coordinate by community. No central scheduler. No conflicts. Each agent's work makes the next agent smarter."

### For architectural governance *(Planned)*:
"We can see when modules are drifting apart, when coupling is increasing, and when refactors change community boundaries, all from the snapshot chain."

---

## 10. Conclusion

The progression from "code graph" to "shared intelligence layer" requires three primitives:

1. **Content-addressed relationships** (trust: is this still valid?)
2. **Emergent community structure** (scope: where does this apply?)
3. **Persistent, structurally-scoped feedback** (learning: what works here?)

Git proved that content-addressing makes source code trustworthy. GCF proved that graph-native wire formats make relationship data efficient for agents. Communities provide the emergent structure for agent intelligence to compound.

**What is implemented today:** Louvain community detection, label propagation, feedback recording with merkleized expiration (package SubgraphRoot), `plan_turn` tool suggestions, and FeedbackBoosts wired into the context engine ranking pipeline.

**What is planned:** Community-scoped feedback, community-constrained RWR walks, CommunityRoot computation over community-specific package sets, multi-agent coordination via communities, temporal community tracking (splits/merges across snapshots), and pending-mutation broadcasting.

The hidden insight: the graph is not a database to query. It is a substrate that agents collectively improve by using it. Each session deposits feedback. Each feedback record is anchored to content-addressed symbols with structural expiration. Each community is discovered from the graph itself. As community-scoped feedback is completed, the system will teach itself which code matters for which work, organized by boundaries no human declared.

Content-addressing solves the trust problem. GCF solves the consumption problem. Communities provide the infrastructure for the learning problem.

Together, they form the foundation of a shared intelligence layer for software development.
