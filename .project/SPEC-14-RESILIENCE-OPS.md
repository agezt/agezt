# Agezt — Resilience, Human-in-the-Loop, Eval, RBAC & Operational Maturity (SPEC-14)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-02, SPEC-05, SPEC-06, SPEC-07, SPEC-10
> Collects the cross-cutting concerns that make an autonomous system survivable, trustworthy, usable, and operable in the real world. Nothing here is "later" — each is placed in a phase. These are the gaps that separate a demo from a product.

---

## 1. Resilience & failure recovery

Agents fail constantly (tool errors, bad LLM output, timeouts, loops). "Supervisor restart" handles crashes; this handles **partial failure** — the harder, more common case.

- **Saga / compensation:** a multi-step task that fails midway runs compensating steps to undo completed side-effects where possible (e.g. close a PR it opened, delete a temp resource). Compensation steps are part of the DAG, journaled.
- **Checkpoint & resume:** tasks checkpoint at node boundaries; on failure/restart they resume from the last good checkpoint (state folded from the journal by `correlation_id`) rather than restarting.
- **Partial-result delivery:** when full success is impossible, return what completed + a clear account of what failed and why, instead of silent failure or total loss.
- **Per-node failure policy:** retry (transient) / compensate / skip-and-continue / fail-task — declared per node, defaulted sensibly.
- **Loop/timeout guards:** `loop-node` max-iterations + budget ceilings + a watchdog that flags stalls (feeds anomaly auto-halt, SPEC-06).
- **Graceful degradation:** no internet / provider down → fall back to local provider + cached memory; degraded mode is announced, not hidden. The system keeps doing what it still can.

**Phase:** core retry/checkpoint **Phase 1–2**; saga/compensation **Phase 6**; graceful degradation **Phase 1+** (rides on Governor local fallback).

---

## 2. Human-in-the-loop (the flow, not just approvals)

Beyond Edict approvals (SPEC-06), agents need to *pause and ask*, then resume.

- **Clarify mid-task:** an agent stuck on ambiguity emits a clarify request (rendered as a choice/form widget, SPEC-12) and **suspends**; the user's answer resumes it from that point. (Hermes's `clarify` tool, but with durable suspend/resume via the journal.)
- **Long-lived, pausable tasks:** a task can wait hours/days for human input without holding resources (suspended, state in journal, resumed on reply across any channel).
- **Approval scoping:** "once / for this task / raise trust for this capability" (SPEC-06 §3.4).
- **Interruption:** the user can inject guidance into a running task ("actually, prioritize X") — a steering message the agent incorporates at the next node boundary. `/stop` halts the current run (SPEC-02 control plane).
- **Contextual decision bundles:** related high-impact or irreversible actions are grouped into one approval packet with predicted effects, confidence, affected resources, cost/latency estimate, and rollback/compensation notes. The user can approve, reject, or modify the bundle instead of answering isolated prompts.

**Phase:** clarify + suspend/resume **Phase 3**; interruption/steering **Phase 5**.

---

## 2.1 Effect classification and routing

Every effectful action declares one of four effect classes:

- `read_only` — no durable external side effect;
- `reversible` — Agezt has a tested inverse operation;
- `compensable` — Agezt can run a best-effort compensation, but cannot guarantee perfect undo;
- `irreversible` — no reliable undo exists.

Effect class is an action/tool property, not a planner wish. Edict, the planner, workflows,
and HITL all consume the same metadata. Routing rules:

- read-only actions can usually run at low trust;
- reversible actions may run autonomously at the appropriate trust level;
- compensable actions require the compensation path to be declared and journaled;
- irreversible or high-blast-radius actions require HITL unless an explicit hard policy
  grants bounded autonomy for that action.

This model is the bridge between saga/compensation and the trust ladder.

---

## 3. Agent evaluation & quality (eval for behavior, not just code)

Code has tests (IMPLEMENTATION §8); agent *behavior* needs evaluation too.

- **Capability eval harness:** does a skill/tool actually succeed at its job? Scenario suites with expected outcomes; success-rate metrics per capability.
- **Regression eval:** did a skill patch (Forge) or prompt change degrade performance? Shadow-testing (SPEC-05 §5.2) is the per-skill gate; this is the system-wide suite.
- **Behavioral metrics:** task success rate by type, time-to-completion, cost-per-task, intervention rate, false-alarm rate (Pulse salience). Fed by the journal.
- **Eval as a feedback source:** the reflection loop (SPEC-05 §6) consumes eval results to recalibrate. (Hermes uses Atropos for RL eval; Agezt focuses on behavioral/regression eval — we're a runtime, not a training framework, per VISION non-goals.)
- **Stochasticity-aware replay:** behavioral re-runs record model, provider, temperature, seed when supported, context snapshot, tool mocks, and expected outcome bands. For `temperature > 0`, byte-for-byte equality is not the oracle; assertions combine schema validity, semantic checks, cost/latency bounds, safety invariants, and task-specific scoring.

**Phase:** eval harness **Phase 5**; integrated into reflection **Phase 8**.

---

## 4. Multi-user & access control (RBAC)

Even single-instance, multiple people: family, team, "my spouse can use this agent but not my banking tool."

- **Identities & roles:** users/roles with scoped permissions over agents, tools, channels, providers, memory.
- **Edict user dimension:** policies condition on *who* (user/role), not just *what* (SPEC-06 §3) — e.g. `role: guest → tool: shell → deny`.
- **Per-user world model / memory scoping:** whose preferences/context apply; sensitive entities restricted by role (SPEC-05 §8).
- **Audit by actor:** every action's `actor` is already journaled; RBAC adds *authorization* on top of *attribution*.
- **Multi-tenant** (SPEC-09 §6) is the strong-isolation extension of this.

**Phase:** single-instance RBAC **Phase 6**; multi-tenant **Phase 9**.

---

## 5. Onboarding & first-run experience

The first 10 minutes decide whether a user stays. Avoid the "still feels like a chatbot" trap (OpenClaw/Hermes weakness).

- **Zero-config start** works immediately (embedded DB, local-model autodetect) — but then a **guided onboarding** offers to: connect providers/subscriptions, connect channels (esp. Telegram for the Jarvis loop), point at repos/projects, and set salience preferences + quiet hours.
- **World-model bootstrap:** an optional onboarding interview seeds the context graph (who you are, your projects, your preferences) so the system understands references and judges salience from day one (SPEC-05 §3.3).
- **First standing wake rule:** suggest a starter durable agent plus wake rule ("wake repo-watch each morning and on CI failures") so proactive value appears immediately without putting identity instructions in the scheduler.
- **Progressive disclosure:** advanced surfaces (Flow Studio, policy editor, trust ladder) are discoverable, not forced.

**Phase:** zero-config **Phase 1**; guided onboarding + world-model bootstrap **Phase 5** (with UI); starter agent wake rule **Phase 3–4**.

---

## 6. Notifications, alerting & escalation

Beyond Pulse briefings (SPEC-03 §6):

- **Escalation chains:** "alert me on Telegram; if no ack in 15 min, email; if still nothing, SMS." For genuinely urgent autonomous situations.
- **Acknowledgement tracking:** did the user see/act on a critical brief? Drives escalation and reflection.
- **Notification preferences:** per-category channel + priority + quiet hours, unified with the salience dial.

**Phase:** **Phase 4** (with channels), escalation chains **Phase 8**.

---

## 7. Secrets lifecycle (beyond at-rest encryption)

SPEC-06 covers storage/redaction; this covers lifecycle:

- **Rotation:** scheduled/triggered credential rotation; OAuth refresh handled in the Conduit.
- **External vaults:** optional integration (e.g. a vault backend) for orgs that require it — a Storage/secret-provider plugin.
- **Scoped, short-lived issuance** to plugins at call time (SPEC-02 §3.4), never long-lived keys on disk in plugin processes.
- **Revocation:** revoke a credential → dependent capabilities degrade gracefully and surface the gap.

**Phase:** rotation/refresh **Phase 7**; vault integration **Phase 8**.

---

## 8. Internationalization & locale

The owner works in Turkish + English; many users are non-English.

- **Multi-language operation:** the agent reasons and replies in the user's language; per-user/channel language preference in the world model.
- **i18n UI/CLI:** externalized copy (English default; Turkish-ready), localizable.
- **Timezone & locale:** cron, briefing timing ("each morning"), and date/number formatting respect the user's timezone/locale — critical for a scheduled, proactive system.

**Phase:** timezone/locale correctness **Phase 3** (Pulse and typed schedules need it); UI i18n **Phase 8**.

---

## 9. Operability & observability (operator-facing)

- **OpenTelemetry export** (SPEC-11 §5): traces/metrics/logs to existing stacks; the journal is the source.
- **Health/readiness** endpoints; **`agt doctor`** diagnostics (config/providers/plugins/version skew).
- **FinOps views** (SPEC-10 §6): cost attribution and trends.
- **System changelog/timeline** (SPEC-08 §4): tamper-evident record of what changed.

**Phase:** `agt doctor` **Phase 1**; OTel/FinOps **Phase 5–8**.

---

## 10. Project/strategy realities (not architecture, but must be planned)

These affect the project's survival as much as any feature:
- **License:** decide early (POLICY §5) — permissive (MIT/Apache, ecosystem growth) vs weak-copyleft (keep improvements open). Owner's call; competitors use MIT/Apache.
- **Community & governance:** OpenClaw reached ~247k★ with a contributor army; a small team cannot match that surface area by force. Implication: ruthless prioritization (the phase plan), leaning on ecosystem interop (SPEC-13) instead of rebuilding, and making contribution easy (SDK) are survival strategies, not nice-to-haves.
- **Documentation & teaching:** the capability army is worthless if no one knows how to use it; docs/tutorials are a deliverable (Phase 8), not an afterthought.
- **Scope discipline:** the single greatest risk is not missing features — it's never shipping a working core because scope kept expanding. The phase plan with demo gates is the antidote; honor it.

---

## 11. Open questions

1. Saga/compensation: how to express "undo" for inherently irreversible actions (the answer is often "escalate before doing", not "undo after").
2. Eval scenario authoring: hand-written suites vs generated from journal traces of successful runs.
3. RBAC granularity vs simplicity for the common single-user case (don't burden solo users).
4. Onboarding depth: how much interview is worth it before it becomes friction.
5. License choice (owner decision, but blocks open-sourcing).

---

*This completes the specification suite (SPEC-01..14) plus POLICY. Next: cross-document updates (SPEC-01 consolidated event kinds, SPEC-06 container modes, SPEC-07 conversation surface) and a refreshed master index tying everything together.*
