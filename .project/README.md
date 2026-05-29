<div align="center">

# Agezt

**An agentic operating system — autonomous, under your authority.**

A single static Go binary that turns intent into auditable, reversible action — and, on its own initiative, watches your world and tells you what matters.

*Out-of-process plugins in any language · deterministic-plus-LLM orchestration · a proactive heart · a visual flow studio · the open Jarvis you actually control.*

`Status: pre-alpha / in design` · `License: TBD` · `Domain & repo: TBD`

</div>

---

## Why Agezt

Today's autonomous agents (OpenClaw, Hermes Agent, and friends) are powerful but share three gaps: they're heavy, language-locked runtimes; their "autonomy" is a stochastic LLM loop with no visible plan; and their self-improvement and actions aren't truly auditable or reversible. They also only act when triggered.

Agezt is built differently:

- **Deterministic DAG + bounded LLM-loop.** The plan is compiled, visible, and approvable before it runs — not a black-box loop you have to trust.
- **Event-sourced truth.** Everything is an append-only, hash-chained event. You can replay history, ask `agt why` for any decision's full provenance, and **revert** anything — including what the agent learned.
- **A proactive heart (Pulse).** It watches your repos, systems, channels, and the world; a **salience filter** decides what actually matters *to you*; and it informs or acts on its own initiative — without spamming you.
- **One static binary, infinite reach.** A near-zero-dependency Go core; every channel, provider, tool, memory backend, and tunnel is an **out-of-process plugin** (any language). A crashing plugin never takes down the kernel.
- **Under your authority.** A per-capability **trust ladder**, **policy-as-code** (Edict), sandbox isolation (Warden), and a one-command **dead-man's switch** (`agt halt`). Autonomy is earned, never assumed.
- **Visually programmable.** A React Flow **Studio** to design, run (live), and replay agent workflows.

> Where Hermes is the messenger, the Agezt clears the path and keeps order. **Autonomous, but under your authority.**

---

## What it can do

Give it an intent — from the CLI, Telegram, the Web UI, a schedule, or an event — and Agezt will:

- Plan the work as a DAG and run it, reasoning with **any LLM** (your subscriptions first, then API keys, then local models — respecting your limits and budgets).
- Use real tools: **shell, files, HTTP, a browser, image/audio/video, document generation, data analysis, web search** — each sandboxed.
- **Spawn sub-agents** for parallel work; delegate coding to **Claude Code / Codex / Aider** as first-class steps.
- Remember across sessions with a tiered **memory** and a **world model** of your projects, people, and preferences.
- **Improve itself** — create and refine skills through a governed, shadow-tested, reversible pipeline (not opaque markdown).
- Reach you on **Telegram, WhatsApp, Slack, Discord, Email, SMS**, and more — unified into one inbox.
- And, unprompted, **notice things and brief you**: "flint-vector CI broke overnight; I opened a fix in PR #214 — needs your review to merge."

---

## Architecture at a glance

```
        REACTIVE                                   PROACTIVE (Pulse)
   you / cron / event                         the system triggers itself
        │                                              │
   ┌────▼────┐                          ┌──────────────▼──────────────┐
   │ Planner │ intent → DAG             │ Observers → Salience → Initiative │
   └────┬────┘                          └──────────────┬──────────────┘
        └───────────────┬─────────────────────────────┘
                        ▼
   ┌───────────────────────────────────────────────────────────────┐
   │  KERNEL (single static Go binary)                              │
   │  Lifecycle · Journal(event truth+BLAKE3) · Plugin Host(gRPC) · │
   │  DAG Scheduler · Edict(policy/trust) · Conduit+Governor        │
   └───────────────────────────────────────────────────────────────┘
                        │ gRPC / stdio (out-of-process)
   ┌────────────────────▼──────────────────────────────────────────┐
   │ PLUGINS  Channel · Provider · Tool · CodingAgent · Memory ·    │
   │          Storage · Tunnel        (any language, crash-isolated)│
   └───────────────────────────────────────────────────────────────┘
     CLI (agt) · Web UI (Flow Studio · Inbox · Monitor · Memory) · SDKs
```

See [`docs/`](docs/) for the full specification suite (Contracts, Kernel, Pulse, Plugins, Memory, Security, UI).

---

## Status & roadmap

Agezt is in **active design**; implementation proceeds in phases, each ending in a demoable slice:

| Phase | Theme |
|---|---|
| 0 | Contracts & kernel core (journal, bus, supervisor, plugin host, control plane) |
| 1 | Reasoning & tools — one task end-to-end (providers, Governor, scheduler, sandboxed tools) |
| 2 | Memory, world model & Forge (self-improvement) |
| 3 | Pulse — the proactive heart |
| 4 | Channels & unified inbox |
| 5 | Web UI — Flow Studio + Live Monitor + Memory Explorer |
| 6 | Sandbox hardening, multi-agent, coding-agent delegation, simulation |
| 7 | Tunnels, full polyglot SDKs, ambient surfaces (voice/tray/mobile), OpenAI-compatible API |
| 8 | Reflection loop, marketplace, polish |
| 9 | Federated mesh & migration (`agt migrate openclaw\|hermes`) |

Full breakdown in [`TASKS.md`](TASKS.md).

---

## Quick start (planned)

```bash
# install (planned)
curl -fsSL <TBD>/install.sh | bash

# zero-config first run: embedded DB + local-model auto-detect
agt                       # interactive TUI
agt run "check my portfolio repos and summarize what changed"

# see what it did, and why
agt why <event-id>

# stop everything, instantly, without losing state
agt halt
```

---

## Design principles

- **Single static binary, near-zero dependencies, stdlib-first.** Runs on a $5 VPS.
- **Everything is an event.** Append-only, hash-chained, reversible.
- **Out-of-process plugins.** Crash isolation; any language; a 20-line plugin via the SDK.
- **Pluggable everything.** Storage (embedded → Postgres/Redis/Flint Vector), providers, channels, tunnels.
- **Simple outside, powerful inside.** One command to start; every layer available to power users (progressive disclosure).
- **Security is core, not optional.** Default-deny, trust ladder, sandboxing, redaction, audit — by default.

---

## Documentation

- `docs/SPEC-01-CONTRACTS.md` — plugin contracts & the canonical event schema
- `docs/SPEC-02-KERNEL.md` — the six kernel responsibilities
- `docs/SPEC-03-PULSE.md` — the proactive heart
- `docs/SPEC-04-PLUGINS.md` — the seven plugin interfaces
- `docs/SPEC-05-MEMORY.md` — memory, world model, skills & Forge
- `docs/SPEC-06-SECURITY.md` — threat model, sandbox, policy
- `docs/SPEC-07-UI.md` — surfaces (Flow Studio, Inbox, Monitor, Memory Explorer)
- `IMPLEMENTATION.md` · `TASKS.md` · `BRANDING.md`

---

## License

TBD.

---

<div align="center">
<sub>Agezt — autonomous, under your authority.</sub>
</div>
