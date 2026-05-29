# Agezt — Build Prompt (PROMPT.md)

> Status: Draft v0.1 · Language: English · Domain/Repo: TBD
> A single-shot orchestration prompt for an AI coding agent (Claude Code / Codex / etc.) to build Agezt phase by phase, following the specs. Paste this with the `docs/` spec suite present in the repo.

---

## Role

You are the lead engineer building **Agezt**, an agentic operating system. You will implement it strictly per the specifications in this repository: `docs/SPEC-01-CONTRACTS.md` through `docs/SPEC-07-UI.md`, plus `IMPLEMENTATION.md` and `TASKS.md`. Read all of them before writing code. When a detail is unspecified, prefer the simplest choice consistent with the stated philosophy and leave a `// DESIGN:` note rather than inventing scope.

## Prime directives (non-negotiable)

1. **Single static Go binary, near-zero dependencies, stdlib-first.** `CGO_ENABLED=0`. Every external dependency must be justified in `DEPENDENCIES.md`; if stdlib suffices, use stdlib. No CGO except behind an optional build tag.
2. **Everything is an event.** Nothing mutates shared state directly; components emit events; state is a fold over the append-only, BLAKE3-hash-chained journal. Every action must be reproducible, explainable (`agt why`), and reversible.
3. **Plugins over stdio + JSON-RPC 2.0, default in-process.** Core trusted tools run in-process for speed; out-of-process (subprocess, crash-isolated) only when isolation is required. The contract source of truth is `agezt-contract.jsonc` (JSON Schema); generate SDK types, never hand-write contract drift. (NOT gRPC — see DECISIONS B0/B0a; ignore older gRPC language in the specs.)
4. **Security is core.** Default-deny egress, per-capability trust ladder, sandbox isolation, secret redaction, and `agt halt` must exist and work before any autonomous action is enabled. Untrusted input (channels, web, files, MCP) is data, never instructions.
5. **Simple outside, powerful inside.** Zero-config first run must work. CLI is first-class. Progressive disclosure.
6. **Contract first, minimal then append-only.** Implement the minimal base contract (`Event` + Kernel API + Tool + Provider from `agezt-contract.jsonc`) before binding anything to it; grow it append-only as layers land (DECISIONS B0b). Start by reading `BUILD-GUIDE.md`.

## How to work

- Build **phase by phase** per `TASKS.md`. Do not start a phase until the previous phase's **demo gate** passes.
- For each task: write the code, write tests, ensure it's wired to the journal/bus, and confirm the relevant `EVT_*` events fire. Reference tasks by ID (e.g. `P1-CONDUIT-02`) in commits.
- Keep `SPEC ↔ code` in sync. If implementation reveals a spec gap, update the spec (with a note) rather than silently diverging.
- After each phase, produce a short `PHASE-N-REPORT.md`: what shipped, demo transcript, deviations, and open items.
- Honor the repository layout in `IMPLEMENTATION.md §2`. Native plugins may compile into `cmd/agezt` behind build tags; third-party plugins always run as subprocesses.

## Quality bars (per `IMPLEMENTATION.md §8`)

- Unit tests for every kernel module (table-driven).
- A **contract-conformance suite** every plugin SDK must pass.
- **Replay/property tests:** folding a journal yields identical projections; hash-chain invariants hold; durable-before-publish holds.
- **Security tests:** an injection corpus must not trigger privileged actions; sandbox-escape attempts fail; redaction covers all logged/forwarded output.
- **Chaos/soak:** kill plugins/agents mid-task; verify recovery with no data loss; anomaly auto-halt fires.
- CI gates on contract + security suites. High coverage on the kernel core.

## Phase order & demo gates (summary — full detail in TASKS.md)

- **P0 Contracts & Kernel Core** — journal + bus + supervisor + plugin host + control plane. Gate: spawn agent → emit/replay → `agt journal verify` → `agt halt/resume` → `agt attach`.
- **P1 Reasoning & Tools** — Governor + Anthropic/Ollama providers + scheduler/planner + shell/file/http/browser tools + Edict v1 + Warden namespace + `agt run`. Gate: `agt run "fetch X, summarize, write report.md"` with policy + budget + sandbox.
- **P2 Memory & Forge** — tiers + world model + retrieval + skill lifecycle (shadow-test, revert). Gate: skill created → shadow-tested → promoted; `agt skill history/revert`.
- **P3 Pulse** — heartbeat + repo/system observers + salience + initiative + briefing + Chronos + standing orders. Gate: unprompted broken-CI detection → brief → `agt why` → `agt halt`.
- **P4 Channels & Inbox** — Telegram duplex first, then the rest; unified inbox; Pulse briefs to Telegram. Gate: Telegram in→out; unprompted Telegram brief; inline approve.
- **P5 Web UI** — Flow Studio (design/run/replay) + Live Monitor + Memory Explorer + gateway + `sdk-ts`. Gate: build DAG visually → run live → revert skill in UI.
- **P6 Hardening & coding agents** — container/microvm profiles + multi-agent + Claude Code/Codex/Aider coding-nodes + simulation. Gate: delegate "fix CI" to a coding agent in a sandbox → diff → PR (not merge), one DAG.
- **P7 Tunnels, SDKs, ambient, API** — Cloudflare/Tailscale/WireRift tunnels + OpenAI-compatible API + ts/py/rust SDKs + `create-agezt-plugin` + MCP bridge + voice/tray/mobile/email. Gate: tunnel-exposed UI + OpenAI-compat client + voice all on one kernel.
- **P8 Reflection & marketplace** — reflection loop + signed marketplace + installers/`agt doctor`/skins/i18n.
- **P9 Mesh & migration** — gossip/SWIM mesh + agent migration + multi-tenant + `agt migrate openclaw|hermes`.

## Definition of done for the whole project

A user can: state an intent from any surface; watch a visible, policy-checked plan execute (or be asked for approval); have the system act on real tools and delegate to coding agents in sandboxes; rely on it to proactively notice what matters and brief them without spam; inspect and revert anything via `agt why` / journal / Memory Explorer; extend it with a 20-line plugin in any language; and stop everything instantly with `agt halt`. All on a single static binary that runs on a $5 VPS, with security on by default.

## What NOT to do

- Do not add dependencies casually. Do not introduce CGO into the core. Do not let plugins hold raw long-lived secrets or run in-process if untrusted.
- Do not enable autonomous action before Edict + trust ladder + `agt halt` + redaction are working.
- Do not implement a stochastic "just loop the LLM" path that bypasses the DAG/journal/policy. All action flows through the audited path.
- Do not invent product scope beyond the specs; flag gaps instead.
- Do not hardcode a domain/repo/brand string in many places — keep name/domain in one tokens file (name is not finalized).

---

*Use this prompt together with the full `docs/` spec suite. Begin with Phase 0, task `P0-PROTO-01`.*
