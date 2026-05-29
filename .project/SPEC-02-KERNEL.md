# Agezt — Kernel Specification (SPEC-02)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01 (Contracts & Event Schema)
> Defines the six kernel responsibilities, the agent runtime, the DAG scheduler, the Governor, and the control plane.

---

## 0. What the kernel is (and is not)

The kernel is a **single static Go binary, near-zero dependencies, stdlib-first**. It is deliberately small. It does not contain channels, providers, tools, or memory backends — those are plugins. The kernel only orchestrates.

The kernel's job: **turn intents into journaled, policy-checked, reversible action, and supervise the agents that carry it out.**

Six responsibilities:
1. Lifecycle / Supervisor
2. Journal
3. Plugin Host
4. DAG Scheduler
5. Policy Gate (Edict)
6. Conduit Registry (+ Governor)

Plus two cross-cutting concerns owned by the kernel: the **internal event bus** and the **control plane** (`halt`, `why`, attach).

---

## 1. Lifecycle / Supervisor

### 1.1 The lightweight actor model
Every agent is a **goroutine + mailbox**, not an OS process. (Plugins are processes; agents are in-kernel actors.) This is the deliberate "lightweight, no full OTP supervision tree" decision.

```
type Agent struct {
    ID          string
    Kind        AgentKind        // resident | ephemeral | scheduled | reactive
    Mailbox     chan Envelope    // bounded; backpressure on full
    State       AgentState       // reconstructed from journal on restart
    Supervisor  *Supervisor
    correlation string           // lineage root
}
```

- **Mailbox:** bounded channel. When full, the sender gets backpressure (not a silent drop). Agent-to-agent messaging always goes through the bus → journal, never via shared memory.
- **Agent kinds:**
  - `resident` — lives 24/7, typically subscribes to a channel or a subject pattern.
  - `ephemeral` — spawned for one task, dies on completion, emits a result event.
  - `scheduled` — created by Chronos on a cron/interval; runs, then dies.
  - `reactive` — spawned by an event match (e.g. `channel.telegram.inbound`).

### 1.2 Supervision (lightweight)
No full supervision tree in v1. A flat supervisor per agent with a restart policy:

```
type RestartPolicy struct {
    Strategy   RestartStrategy // never | on_crash | always
    MaxRestarts int            // within Window
    Window      time.Duration
    Backoff     BackoffSpec    // exponential w/ jitter
}
```

- On crash: emit `EVT_AGENT_CRASHED`, consult policy. If restartable, **replay state from the journal** up to the agent's last consistent checkpoint, then resume. If `MaxRestarts` exceeded in `Window`, quarantine the agent and emit a high-salience event (Pulse will surface it).
- **State recovery is the key differentiator:** because state is a fold over journaled events scoped by `correlation_id`, a restarted agent picks up where it left off rather than starting blank. (Hermes loses the session on loop failure; Agezt replays.)

### 1.3 Spawn / suspend / resume / kill
```
Spawn(spec)   → EVT_AGENT_SPAWNED, returns agent_id
Suspend(id)   → EVT_AGENT_SUSPENDED (drains mailbox, persists, stops consuming)
Resume(id)    → EVT_AGENT_RESUMED (replays, re-subscribes)
Kill(id, why) → EVT_AGENT_DIED (graceful) or forced after drain timeout
```

`agent-node` in a DAG uses `Spawn` to fork a parallel sub-agent and awaits its result event.

### 1.4 Mesh-readiness (future, contract present now)
Agents carry a `home_node` field (default = local). The supervisor interface is written so a future mesh implementation can migrate an agent to another node by shipping its `correlation`-scoped event slice. Not implemented in v1; the seam exists so it won't require a rewrite.

---

## 2. Journal — the single source of truth

### 2.1 Model
Append-only log of `Event` (SPEC-01 §7). State is **never** authoritative on its own; it is always derivable by folding the journal. This gives time-travel, reproducibility, and reversibility.

### 2.2 Hash chain
Each event stores `prev_hash` and `hash` (BLAKE3 over canonical bytes). The chain is tamper-evident: altering or removing any past event breaks every subsequent hash. `agt journal verify` walks the chain and reports the first break.

### 2.3 Write path
```
emit → assign(id=ULID, seq, ts) → link(prev_hash) → compute(hash)
     → StoragePlugin.AppendJournal(batch)  [durable]
     → publish to internal bus              [in-memory fanout]
```
Writes are batched for throughput; the batch returns the new chain head. The bus publish happens **after** durable append, so subscribers never see an event that wasn't persisted.

### 2.4 State folds (projections)
The kernel maintains in-memory **projections** rebuilt on boot by replaying the journal:
- agent registry (alive/suspended/dead)
- task/DAG status
- budget ledger (Governor)
- memory index pointers
- pulse state (last tick, observer cursors)

Projections are caches; the journal is truth. A projection bug never corrupts data — just rebuild.

### 2.5 Compaction & snapshots
Replaying from genesis forever is impractical. The kernel periodically writes a **snapshot** (a serialized projection at sequence `N`) so boot replays only `N → head`. Snapshots are themselves journaled (`EVT`-tagged) and content-addressed. Original events are retained (configurable retention) for full time-travel; compaction only affects replay speed, not auditability unless the operator explicitly prunes.

### 2.6 Reversibility
"Undo" = compute the inverse projection at sequence `N-k` and emit compensating events (e.g. `EVT_SKILL_REVERTED`). The kernel never rewrites history; it appends the reversal. `agt journal revert <seq>` is policy-gated.

---

## 3. Plugin Host

### 3.1 Discovery
On boot and on `agt plugin add`, the host scans `${AGEZT_PLUGIN_DIR}` (default `~/.agezt/plugins/`) for plugin manifests:

```yaml
# plugin.yaml
id: telegram
binary: ./telegram-plugin
version: 0.1.0
protocol_version: 1
capabilities:
  - kind: CAP_CHANNEL
    attributes: { direction: duplex }
autostart: true
isolation: namespace   # how the plugin process itself is sandboxed
```

### 3.2 Process management
- Launch with bootstrap env (SPEC-01 §2.1), one-time token.
- Maintain the bidirectional Handshake stream; health-ping every `N`s.
- Restart per the same `RestartPolicy` model as agents. A plugin crash is isolated — kernel and other plugins continue.
- Native first-party plugins MAY be compiled into the main binary and/or run as WASM in-process; the host abstracts "where" behind the same call interface so callers don't care.

### 3.3 Capability routing
The host builds a routing table from advertised capabilities. When a DAG node needs `CAP_TOOL` with `tool_name=browser`, the host resolves it to a concrete plugin. Multiple plugins offering the same capability → Edict policy / explicit pin decides (e.g. prefer in-process WASM browser over remote).

### 3.4 Secrets boundary
Plugins never get raw provider keys. When a provider plugin needs to authenticate, the kernel's Conduit injects credentials at call time within the kernel process boundary, or proxies the call. (Exact mechanism per provider; the invariant is: a compromised plugin process cannot exfiltrate the user's API keys from disk.)

---

## 4. DAG Scheduler

### 4.1 Compilation
An intent becomes a DAG via the **Planner** (a kernel-resident meta-agent backed by an LLM). The Planner:
1. Reads the **capability inventory** (what plugins/tools/providers exist).
2. Reads relevant **world-model** context (what "portfolio", "the repos" mean).
3. Emits `EVT_PLAN_PROPOSED` with a DAG.
4. If a needed capability is missing → either request Forge to create a skill, or emit a "missing capability" event suggesting a plugin.

The plan is **visible and approvable before execution** (gate-node at the front for high-trust-cost plans). This is the core anti-stochastic property: you can see the plan, not just trust a loop.

### 4.2 Node types
Per SPEC-01 §6: `tool-node`, `llm-node`, `loop-node`, `gate-node`, `agent-node`, `coding-node`.

### 4.3 Execution
- Topological order with **parallel branches** run concurrently (bounded worker pool, default 8).
- Each node: `EVT_NODE_STARTED` → do work via kernel→plugin → `EVT_NODE_COMPLETED|FAILED`.
- `loop-node` enforces **max iterations** and a **budget ceiling** (Governor). The agentic reasoning lives here, bounded — it cannot run away.
- Failure handling: per-node retry policy; on terminal failure, either a compensation branch runs or the task fails and surfaces (high salience).
- Path-scoped concurrency: nodes touching independent resources run in parallel; nodes touching the same path serialize (mirrors the contract's isolation guarantees).

### 4.4 Determinism property
Given the same journal prefix and the same plan, re-execution is reproducible up to LLM nondeterminism, which is itself recorded (`EVT_LLM_RESPONSE` stores the actual output). So a *replay* uses recorded outputs → fully deterministic; a *re-run* may differ but is fully logged.

---

## 5. Policy Gate (Edict)

### 5.1 Policy-as-code
YAML policies evaluated at every kernel→plugin boundary and every gate-node:

```yaml
# edict.yaml (illustrative)
defaults:
  isolation: namespace
  approval: none
rules:
  - match: { tool: shell, command_glob: "rm -rf*" }
    decision: deny
  - match: { tool: browser, domain: "*.bank.*" }
    decision: escalate          # require human approval
  - match: { node: coding-node, action: merge }
    decision: escalate
  - match: { provider: "*", cost_usd_gt: 5 }
    decision: escalate
  - match: { channel: "*", action: send_on_behalf }
    decision: escalate
```

Every evaluation emits `EVT_POLICY_DECISION` (allow | deny | escalate). Escalation pauses the node and emits `EVT_APPROVAL_REQUESTED`, routed to the user via a Channel.

### 5.2 Trust ladder
A per-capability trust level the user raises over time:

```
L0 observe-only   — may read, may not act
L1 propose        — may draft/plan, requires approval to execute
L2 act-reversible — may do reversible actions autonomously, escalate irreversible
L3 act-bounded    — may act within budget/scope limits autonomously
L4 trusted        — broad autonomy; only hard-deny rules apply
```

The ladder is the answer to "how autonomous is Jarvis?" — it starts cautious and earns autonomy. Pulse's Initiative engine reads the ladder to decide solve-vs-ask.

### 5.3 Hard limits (non-overridable)
Some rules are not user-raisable regardless of trust level (e.g. credential exfiltration, destructive deletes without explicit per-action confirmation). These live in a separate immutable block.

---

## 6. Conduit Registry + Governor

### 6.1 Conduit Registry
The runtime directory of provider/tool/memory plugins and their capabilities. Resolves "I need an LLM that can do X" or "I need an embedder" to a concrete plugin.

### 6.2 Governor (subscription & budget brain — competitors lack this)
Sits in front of all `RequestCompletion` calls. Inputs per provider: auth mode (subscription | api-key), rate limits, declared budget, priority. Algorithm per request:

```
1. Filter providers that can serve the request (model family, modality).
2. Order by: subscription-first (use what's already paid for) → cost → latency.
3. Check budget & rate limit for the top choice.
   - within limits → use it.
   - limited → fall to next in the fallback chain (e.g. Anthropic → OpenRouter → local Ollama).
4. Record EVT_BUDGET_CONSUMED with actual cost/tokens.
5. If a budget ceiling is breached → emit budget.exceeded → Pulse surfaces it,
   and loop-nodes honoring that ceiling stop.
```

This realizes "use my subscriptions, respect limits if limited, otherwise push." Budget unit (USD vs tokens vs credits) is an open question from SPEC-01 §10.1 — resolve in F1.

---

## 7. Internal event bus

- In-process, Go channels, NATS-style subject routing (SPEC-01 §8).
- Patterns: pub/sub, request/reply (correlation_id), streaming (server-stream RPC to subscribers).
- **Every published event is also journaled** — the bus is the live view, the journal is the durable truth. One mechanism feeds Live Monitor, Flow Studio highlighting, agent coordination, and Pulse.
- Pluggable transport seam: the bus interface allows swapping the in-process implementation for an embedded NATS / Redis / ChimeraMQ backend later for multi-node, without changing publishers/subscribers. (Default stays in-process for single-node simplicity.)

---

## 8. Control plane

### 8.1 `agt halt` — dead-man's switch
Engages a global freeze: emit `EVT_HALT`, suspend all agents (drain mailboxes, persist), stop the DAG scheduler from starting new nodes, keep the journal and plugin host alive. Nothing is lost; nothing new runs. `agt resume` reverses it. Also auto-triggered by anomaly detectors (e.g. tool-call rate spike, budget runaway).

### 8.2 `agt why <event_id>` — explainability
Walks the `causation_id` chain backward and renders the decision lineage: triggering observer → salience score & reasoning → plan → node → policy decision → outcome. Because provenance is in the schema, this is a journal query, not special instrumentation.

### 8.3 `agt attach <agent_id|task_id>` — live tap
Subscribes to the relevant subject pattern over the socket and streams events to the terminal/UI in real time. The same mechanism powers Web UI live views.

### 8.4 Other control commands
`agt journal {replay|verify|revert}` · `agt plugin {list|add|disable}` · `agt agent {list|spawn|kill}` · `agt pulse {status|pause|resume}` · `agt policy {test|reload}`.

---

## 9. Boot sequence

```
1. Load config (defaults < file < env < flags).
2. Open Storage driver; load latest snapshot.
3. Replay journal snapshot→head; rebuild projections.
4. Start internal bus.
5. Start Plugin Host; discover + autostart plugins; build routing table.
6. Restore agents: respawn residents, reattach scheduled (Chronos), re-subscribe reactives.
7. Start Pulse (if enabled).
8. Open control-plane socket; CLI/UI/SDK can now attach.
9. Emit system.resume.
```

Crash recovery is the same path: the journal makes boot and recovery identical.

---

## 10. Failure & safety invariants

- A plugin crash never crashes the kernel. (Process isolation.)
- An agent crash never loses task state. (Journal replay.)
- No event is ever rewritten or silently dropped. (Append-only + hash chain + bounded mailboxes with backpressure.)
- No autonomous action exceeds the trust ladder / hard limits. (Edict on every boundary.)
- No runaway cost. (Governor ceilings + loop-node budgets + anomaly auto-halt.)
- The operator can always stop everything instantly without data loss. (`agt halt`.)

---

## 11. Open questions

1. Agent checkpoint granularity — per-node, per-N-events, or time-based? (Affects replay cost on restart.)
2. Projection rebuild time at scale — when does snapshot cadence need tuning?
3. In-process vs WASM vs subprocess decision policy for first-party tools — static config or adaptive?
4. Anomaly detector thresholds — fixed defaults vs learned from reflection loop?

---

*Next: PULSE-SPEC — the proactive heart (Observers, Salience, Initiative, Briefing) that turns this runtime into a Jarvis. Then per-plugin specs and UI-SPEC.*
