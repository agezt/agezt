# Agezt — Pulse Engine Specification (SPEC-03)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> Depends on: SPEC-01 (Contracts), SPEC-02 (Kernel)
> Defines the proactive heart: the mechanism that lets Agezt act, notice, and inform **without being asked**. This is what makes it a Jarvis rather than a tool.

---

## 0. The core idea

Every other agent system is **reactive**: something triggers it (you, a cron, an event), it runs, it stops. The Pulse Engine adds a **second heartbeat** that triggers *itself*. On every beat the system asks three questions:

> **What changed? · Is it important? · Should I act or tell Ersin?**

Without this, "an assistant that informs me even when I don't ask" is impossible. With it — and crucially with a *salience filter* so it isn't noise — you get a living presence.

The hard problem is **not** "send a Telegram message on a schedule." That's trivial and both competitors do it. The hard problem is **knowing what is worth saying**. Pulse is built around that judgment.

---

## 1. Anatomy

```
            ┌──────────────────────────────────────────────┐
            │                 PULSE ENGINE                  │
            │                                               │
   tick ───▶│  ① OBSERVERS ──▶ ② SALIENCE ──▶ ③ INITIATIVE │──▶ action / spawn
            │     (notice)      (weigh)         (decide)    │
            │         │             │               │       │
            │         ▼             ▼               ▼       │
            │   EVT_OBSERVER   EVT_SALIENCE   EVT_INITIATIVE │
            │     _DELTA         _SCORED        _TAKEN       │
            │                                     │         │
            │                                     ▼         │
            │                          ④ BRIEFING COMPOSER  │──▶ Telegram/mail/UI/voice
            │                             EVT_BRIEFING_SENT  │
            └──────────────────────────────────────────────┘
                              all steps journaled →  agt why
```

Four stages, each emitting its own event so the entire proactive chain is explainable end-to-end.

---

## 2. The heartbeat (tick)

- Pulse runs as a `resident` agent. It emits `EVT_PULSE_TICK` on an **adaptive interval**:
  - Base cadence (e.g. every 60s) when idle.
  - Accelerates under activity (an observer reported a delta → tighter beats until it settles).
  - Decelerates when quiet (battery/cost friendly; on a $5 VPS you don't want a hot loop).
- A tick does not itself do heavy work. It **fans out** to observers and lets them report. Pulse is a scheduler of attention, not a doer.
- `agt pulse {status|pause|resume}` controls it. `agt halt` suspends it with everything else.

---

## 3. Stage ① — Observers

### 3.1 What an observer is
A lightweight observer or durable system agent that watches **one thing** and emits **meaningful deltas**, not raw data. The discipline: an observer must do its own first-pass filtering so the bus isn't flooded. It reports "CI on repo X went from green to red," never "here is the full CI log every 60 seconds."

```
type Observer interface {
    Subject() string          // what bus subject / external source it watches
    Poll(ctx) ([]Delta, error) // or event-driven via Subscribe
    Cadence() Schedule        // how often (some are event-driven, cadence=0)
}

type Delta struct {
    Source     string            // "repo:flint-vector", "system:disk", "channel:telegram"
    Kind       string            // "ci_failed","disk_low","new_message","price_move"
    Summary    string            // human-readable, one line
    Before     string            // prior known state (for the world model)
    After      string
    RawRef     string            // pointer to detail in journal/storage, not inlined
    Hints      map[string]string // severity hints the observer already knows
}
```

### 3.2 Observer families (first-party)
- **Repo/Project** — CI status, new issues, security advisories, dependency alerts, release tags across the portfolio repos.
- **System health** — disk, memory, service liveness, cert expiry (Argus/AnubisWatch-style).
- **Channels** — inbound messages/mail that arrived while you were away; threads needing a reply.
- **World** — RSS/news/X mentions, price feeds, anything you subscribe to.
- **Internal** — Agezt's own state: budget nearing limit, an agent stuck/looping, a plugin unhealthy, a skill quarantined.

### 3.3 Rules
- Observers are **plugins or thin internal agents**; new observer = small SDK plugin. The set is open.
- An observer **never** decides importance — that's Salience's job. It only detects change and attaches hints.
- Deltas carry `Before`/`After` so the **world model** updates and so Salience can reason about magnitude of change.
- Each delta → `EVT_OBSERVER_DELTA` on `pulse.observer.<source>`.

---

## 4. Stage ② — Salience filter (the crux)

### 4.1 Purpose
Turn a stream of deltas into a small set of things that actually matter **to Ersin specifically**. This is the single most important component for not being annoying and not being useless.

### 4.2 How it scores
Each delta passes through an `llm-node` (cheap/fast model by default, via Governor) plus deterministic features. The score combines:

| Signal | Source | Example |
|---|---|---|
| Severity | observer hints + rules | prod down >> lint warning |
| Novelty | world model | "already told him this yesterday" → suppress |
| Relevance | world model | touches an active project vs an archived one |
| Magnitude | Before/After delta | 2% price move vs 40% |
| User feedback history | reflection loop | Ersin dismissed this category last 5 times → decay |
| Time/context | config | 3am → only true emergencies break through |

Output: `EVT_SALIENCE_SCORED` with a `score ∈ [0,1]`, a `reason` (why this score), and a recommended `disposition`:

```
disposition:
  drop        // journal only; visible if he looks, never pushed
  digest      // batch into the next briefing (morning/weekly)
  notify      // send soon, normal priority
  alert       // send now, high priority
  act         // important enough to consider autonomous action (→ Initiative)
```

### 4.3 Thresholds (user-controlled, simple knob)
A single high-level dial maps to threshold presets:
- **Quiet** — only `alert`/`act` ever reach you; everything else digests or drops.
- **Balanced** (default) — `notify` and up reach you; rest digests.
- **Chatty** — `digest` surfaces too.

Per-category overrides exist for power users ("always alert on prod down regardless of dial"), but the default experience is one dial. (Progressive disclosure.)

### 4.4 Why this beats competitors
OpenClaw/Hermes have no salience layer — their "proactive" features are scheduled reminders. They cannot weigh importance, suppress what you already know, or decay categories you ignore. Salience + world model + reflection feedback is the moat.

---

## 5. Stage ③ — Initiative engine

### 5.1 Solve-vs-ask decision
For deltas scored `act` (or `alert` with an obvious fix), Initiative decides:

```
if reversible AND within trust-ladder level for this capability AND within budget:
    → ACT autonomously, then inform (briefing reports what was done)
elif irreversible OR above trust level OR over budget:
    → ASK first (EVT_APPROVAL_REQUESTED via Channel), act only on approval
else:
    → INFORM only (notify/alert), no action
```

- The decision reads **Edict trust ladder** (SPEC-02 §5.2) and **Governor budget** (SPEC-02 §6.2). Pulse owns no permissions of its own; it borrows the same governance every other path uses.
- Acting = the Planner compiles a DAG for the fix (e.g. "CI red → diagnose → patch → open PR"), executed through the normal scheduler. Pulse doesn't have a side-channel; it goes through the audited path.
- Emits `EVT_INITIATIVE_TAKEN` (with the chosen branch and its justification) regardless of which branch.

### 5.2 Example: CI broke at 2am, trust level L2 (act-reversible)
```
observer: repo flint-vector CI red
salience: score 0.88, reason "active project, regression on main", disposition act
initiative: open PR (reversible) is within L2 → ACT
  → Planner DAG: pull logs → identify failing test → propose fix → run locally
    → open PR (NOT merge: merge is escalate per edict) → done
briefing: queued as 'important', sent at morning unless he's online now
result Ersin sees: "flint-vector CI broke overnight (nil deref in HNSW insert).
  I opened PR #214 with a fix; tests pass. Needs your review to merge."
```
This is the Jarvis moment: it noticed, judged, fixed what it safely could, stopped at the line it shouldn't cross, and told you — without you asking.

---

## 6. Stage ④ — Briefing composer

### 6.1 Job
Decide **channel, tone, format, and timing** for reaching you. Salience set the urgency; Briefing executes the communication.

```
disposition → delivery:
  alert  → send immediately, highest-priority channel (Telegram push / voice)
  notify → send within a short window, normal channel
  digest → accumulate; flush on schedule (morning brief, weekly summary)
  drop   → never sent
```

### 6.2 Composition
- **Batching:** multiple `digest` items become one coherent brief, grouped by project/topic, not a list of pings. ("3 things in your portfolio overnight: …")
- **Tone & length:** one-liner for a single fact ("CI fixed ✅"); structured short report for multi-item. Never a wall of text. (Honors the "simple outside" contract.)
- **Action affordances:** when approval is pending, the message includes inline approve/deny (Channel `Signal`/buttons where supported) so you can respond in the same surface.
- **Channel choice:** uses your configured preference and the platform's capability (push vs email vs voice). Falls back gracefully.

### 6.3 Anti-annoyance guarantees
- **Dedupe & coalesce:** the same underlying issue never pings twice; updates edit/append to the existing thread (`in_reply_to`/`correlation_id`).
- **Quiet hours:** config-driven; only `alert` breaks them.
- **Feedback hooks:** every briefing is feedback-instrumented — dismiss, snooze, "less of this" — which feeds the reflection loop and decays future salience for that category.
- Emits `EVT_BRIEFING_SENT`.

---

## 7. Integration with the rest of the system

- **World model (SPEC-future MEMORY):** Salience's relevance/novelty signals come from here; observer deltas update it. Pulse without a world model is generic; with it, it's *yours*.
- **Reflection loop:** consumes briefing feedback + initiative outcomes to recalibrate salience thresholds and initiative aggressiveness over time ("he keeps deleting morning briefs → lower their salience / change cadence").
- **Forge:** if Pulse repeatedly needs a capability it lacks ("I keep wanting to summarize these PDFs and have no tool"), it can request Forge to create a skill — proactive self-improvement.
- **Standing wake rules:** a standing order ("wake ops-watch when portfolio CI breaks") is a durable trigger/constraint rule for an existing agent or workflow. Pulse can score and brief the result, but the agent's identity, tasklist, memory, permissions, model, retry, and doctor policy live on the agent profile, not inside the schedule.

---

## 8. Safety specifics for proactive autonomy

Proactive + autonomous is the riskiest combination in the system. Extra guards beyond normal Edict:

- **Initiative is rate-limited.** A ceiling on autonomous actions per window; exceeding it forces everything to `ask` and raises an internal-observer alert (possible loop/runaway).
- **Novelty gate before acting twice.** Initiative will not take the *same* autonomous action repeatedly without escalating — prevents thrash (e.g. reopening the same PR).
- **Every autonomous act is reversible-by-design or escalated.** If Initiative cannot identify a reversal path, it downgrades from `act` to `ask`.
- **`agt halt` kills Pulse too.** And the anomaly detector that auto-halts watches Pulse's own action rate.
- **Full provenance.** `agt why <briefing_or_action>` reconstructs: tick → observer delta → salience score+reason → initiative branch+justification → policy decision → outcome.

---

## 9. MVP cut for Pulse (matches VISION §17)

Ship the spine, not the whole nervous system:
1. Heartbeat (fixed cadence, adaptive later).
2. **Two observers:** repo/CI + system health.
3. **Salience v1:** rules + one cheap LLM scoring call; single "Quiet/Balanced/Chatty" dial; novelty suppression via a simple seen-cache (full world model later).
4. **Initiative v1:** inform-or-ask only (no autonomous fixing yet) — safest first step; autonomous `act` lands once trust ladder + reversal detection are solid.
5. **Briefing v1:** Telegram, immediate `alert` + daily `digest`; dedupe; quiet hours.

MVP success line (from VISION): *"I write nothing, yet it notices a broken CI on its own and tells me on Telegram; I can see why with `agt why` and stop it with `agt halt`."* That is Initiative v1 (inform) + two observers + salience + briefing — and it already feels alive.

---

## 10. Open questions

1. Salience model choice — fixed cheap model, or escalate to a stronger model for borderline scores?
2. Adaptive cadence algorithm — simple activity-window, or PID-style controller?
3. How much world-model is required before autonomous `act` is safe to enable by default?
4. Briefing batching window defaults per disposition — tune empirically in F3.
5. Standing-order DSL — reuse the DAG/plan format, or a higher-level declarative format that compiles to observers+overrides?

---

*Backbone complete: SPEC-01 (Contracts) + SPEC-02 (Kernel) + SPEC-03 (Pulse) lock the spine. Next layer: per-plugin specs (Channel/Provider/Tool/CodingAgent/Memory/Storage/Tunnel), MEMORY/world-model spec, then UI-SPEC, then IMPLEMENTATION → TASKS → BRANDING → README → PROMPT.md.*
