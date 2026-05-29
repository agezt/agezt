# Agezt — Path to Product (ROADMAP.md)

> Status: v1.0 · Language: English · License: MIT · Open source
> The concrete path from the current design suite to a shipped, usable product. Decisions are frozen (DECISIONS.md); contracts are written and compile (`agezt.proto`). This is the execution plan.

---

## 0. Where we are

**Done (design):** full spec suite (POLICY + SPEC-01..16), IMPLEMENTATION, TASKS, BRANDING, README, PROMPT, INDEX, **DECISIONS (frozen)**, **contract (`agezt-contract.jsonc`, JSON-RPC)**, **LICENSE (MIT)**.

**Foundational revision (DECISIONS B0–B0d):** transport is **stdio + JSON-RPC 2.0** (not gRPC); plugins **default in-process**, out-of-process only for isolation; **mutable state store is first-class** alongside the event log; the **DAG is a second layer** over a first-party single-agent tool-loop core; the base contract is **minimal and grows append-only**. The old gRPC `agezt.proto` is superseded by `agezt-contract.jsonc`.

**Not done (build):** any Go/TS code. That's what this roadmap drives.

The strategy: build a **minimal working core (M0.5)** first, then the **MVP** (a real, usable Jarvis), then grow. The full vision is the destination; the smallest working core proves the foundation.

---

## 0.5. Milestone 0.5 — Minimal working core ("core-core")

Before the full MVP, prove the foundation with the smallest thing that runs:

> **Event log** (append-only JSONL + BLAKE3 chain) **+ mutable state store + a first-party single-agent tool-loop + one Tool plugin (shell, in-process) + one Provider (Anthropic or Ollama) + `agt halt` / `agt why`.** No DAG, no Pulse, no seven plugin types, no gRPC.

**Success test:** `agt run "list the files here and tell me what this project is"` → the agent loops (LLM ↔ shell tool), produces an answer, every step is journaled, `agt why` explains it, `agt halt` stops it. Runs as one process.

This validates the base (orchestration loop, journal, tool-calling, provider abstraction, control plane) before any complexity is layered on. **If this is clean, everything after layers on smoothly** — which is the whole point of getting the foundation right.

---

## 1. Milestone 0 — Repository foundation (days, not weeks)

Before feature code:

1. **Init repo** (TBD host): Go module, `LICENSE` (MIT), SPDX headers, `docs/` (move the spec suite in), `README.md`.
2. **Contracts pipeline:** add `agezt.proto`; set up `buf`/`protoc` codegen (build-time, per DECISIONS G2); generate `sdk-go` stubs; CI verifies codegen is in sync.
3. **CI skeleton (GitHub Actions):** build (multi-arch, `CGO_ENABLED=0`), test, lint, dependency-justification check; image build to GHCR on tag; cosign signing + SBOM.
4. **`DEPENDENCIES.md`** seeded with the justified core deps (grpc, protobuf, blake3, bubbletea/lipgloss).
5. **Brand constants** (`internal/brand`) — name/paths in one place.

**Exit:** `make build test` green; an empty kernel binary runs and prints version; contracts generate cleanly.

---

## 2. The MVP (Milestone 1) — "a Jarvis that works"

This is the product's first real form. Built from Phases 0–4 of the full plan, trimmed to the MVP cut (VISION §17).

### 2.1 Scope (the 7 essentials)
1. **Kernel core:** journal (JSONL + BLAKE3 chain + projections + snapshots), in-process bus, lightweight agent supervisor, plugin host (gRPC handshake/health/routing), control plane (`agt halt`/`why`/`attach`).
2. **DAG scheduler + simple Planner:** intent → DAG → execute; node types tool/llm/loop/gate.
3. **Governor v1 + 2 providers:** Anthropic (subscription + api-key) + Ollama (local fallback). USD-microcent budgeting, fallback chain.
4. **4 tools:** shell, file, http, browser — each sandboxed (`namespace`, browser in `container`).
5. **1 channel:** Telegram (duplex) — command in, proactive brief out.
6. **Pulse v1:** heartbeat + 2 observers (repo/CI, system-health) + salience (rules + cheap LLM) + Quiet/Balanced/Chatty dial + Initiative (inform/ask first) + Briefing to Telegram.
7. **Safety:** Edict v1 (policy + trust ladder), Warden namespace+container, secret redaction, `agt halt`, anomaly auto-halt.

Plus the always-on essentials from DECISIONS: ULID identity everywhere, migration runner, `agt doctor`, zero-config first run.

### 2.2 MVP success test (the definition of done)
> *"From Telegram I type 'check my portfolio repos' and it scans them and replies with a summary. Without me asking, it notices a broken CI on its own and tells me on Telegram. I can see why it did anything with `agt why`. I can stop everything instantly with `agt halt`. It runs as a single deployment on a $5 VPS."*

When that paragraph is true end-to-end, the MVP ships as **v0.1.0** — a real, open-source, usable product.

### 2.3 Suggested build order within the MVP
P0 kernel core → P1 providers+tools+scheduler+CLI (`agt run`) → memory-lite (enough for context, full Forge later) → P3 Pulse → P4 Telegram. Each sub-step has a demo gate (TASKS.md).

---

## 3. Post-MVP growth (Milestones 2+)

Each milestone is a release that adds a coherent capability layer, following the unified phase plan (INDEX §3). None blocks the MVP; each is independently valuable.

- **M2 — Memory & self-improvement (v0.2):** tiered memory + world model + Forge (skill lifecycle, shadow-test, revert) + context compression. *The system starts learning.*
- **M3 — More channels & inbox (v0.3):** the rest of the channels + Unified Inbox (needs the Web UI shell). *Manage everything from one place.*
- **M4 — Web UI (v0.4):** Flow Studio (design/run/replay), Live Monitor, Memory Explorer, conversation surface with tool-call debug + context inspector, first-party widgets. *See and shape what agents do.*
- **M5 — Hardening & coding agents (v0.5):** container/microvm sandbox, multi-agent, Claude Code/Codex/Aider coding-nodes, simulation, single-instance RBAC, saga/compensation. *Safe heavy autonomy.*
- **M6 — Reach (v0.6):** tunnels, OpenAI-compatible API, polyglot SDKs (ts/py/rust) + `create-agezt-plugin`, MCP bridge, ambient surfaces (voice/tray/mobile), secrets rotation. *Extend and embed anywhere.*
- **M7 — Ecosystem (v0.7):** reflection loop, marketplace, agentskills.io/ClawHub adapters, escalation chains, i18n, docs site, `agt migrate openclaw|hermes`. *Grow the army; pull in their users.*
- **M8 — Scale (v1.0):** federated mesh, multi-tenant. *One Agezt across many nodes.*

---

## 4. Engineering guardrails (apply at every milestone)

- **Contracts freeze first; evolve append-only.** `agezt.proto` is the dependency root.
- **Test discipline is mandatory:** unit + contract-conformance + replay/property (fold determinism, hash-chain) + security (injection/sandbox/redaction) + chaos (kill plugins/agents, verify recovery). CI gates on contract + security suites. This is what keeps modularity maintainable.
- **Security defaults on before autonomy:** Edict + trust ladder + `agt halt` + redaction must work before Initiative can act autonomously.
- **Dependency discipline (POLICY):** stdlib-first core; heavy deps live in plugins; every dep justified.
- **Scope discipline:** honor the MVP boundary. The greatest risk is never shipping because scope kept growing. Ship v0.1, then grow.

---

## 5. Realistic expectations (honest)

- **The MVP is achievable by a focused solo developer or small team** with disciplined scope — it's a few well-defined subsystems, not the whole suite.
- **The full vision (M2–M8) is large** — comparable to what teams/communities built for OpenClaw and Hermes. Reaching it depends on: (a) a working MVP that attracts contributors, (b) leaning on ecosystem interop (run their capabilities rather than rebuild), (c) the SDK making third-party contribution easy, (d) ruthless prioritization.
- **Open source + MIT is a force multiplier here:** the plugin architecture + polyglot SDK means the community can build the capability army; the core team's job is to keep the kernel small, the contracts stable, and the safety model sound.

---

## 6. Immediate next actions (in order)

1. **Init the repo** with the structure from IMPLEMENTATION §2, MIT LICENSE, and the spec suite under `docs/`.
2. **Land `agezt.proto`** + codegen + CI.
3. **Build P0 kernel core** to its demo gate (spawn agent → emit/replay → verify chain → halt/resume → attach).
4. **Build P1** to `agt run "..."` working end-to-end with one provider + sandboxed tools.
5. Proceed through the MVP build order (§2.3) to the MVP success test (§2.2) → tag **v0.1.0**.
6. Announce, gather contributors, grow along M2+.

---

*The design is complete and frozen enough to build. The contract compiles. The license is MIT. The path is: foundation → MVP → grow. Start at Milestone 0, then `P0-PROTO-01` is already done — generate from `agezt.proto` and write the kernel.*
