# Agezt — Build Guide (START HERE)

> This is the single entry point for building Agezt with an AI coding agent (Claude Code). Read this first, in full, before reading anything else or writing any code. It tells you what to read, what's binding, what to ignore, what to produce, and in what order.

---

## 0. What you are building

Agezt — an open-source (MIT) **agentic operating system** written in Go (kernel/plugins) with a TypeScript/React web UI. It turns user intent into auditable, reversible action; runs autonomous agents under a policy/trust system; proactively watches and informs the user (Pulse); and extends via plugins. Full concept: `README.md` and `AGEZT-VISION-MASTER.md`.

You are not designing it — the design is done. You are **implementing** it from the documents in this repository.

---

## 1. Document authority (read in this order)

**Tier 1 — binding, read first, never override:**
1. `BUILD-GUIDE.md` (this file) — how to proceed.
2. `DECISIONS.md` — **the supreme authority.** Every frozen technical decision. If any other document conflicts with this, DECISIONS wins. Pay special attention to **B0–B0d** (the foundational revisions below).
3. `POLICY.md` — dependency, packaging/binary, versioning, license policy.
4. `agezt-contract.jsonc` — the contract source of truth (JSON-RPC 2.0 + JSON Schema).
5. `STRUCTURE.md` — the exact repository layout to produce.
6. `ROADMAP.md` — the build order (M0.5 → MVP → growth) with success tests.

**Tier 2 — the design specs (read the ones relevant to the milestone you're on):**
`SPEC-01`..`SPEC-16`, `IMPLEMENTATION.md`, `TASKS.md`. Each carries an AUTHORITY NOTICE pointing back to DECISIONS.

**Tier 3 — supporting:** `BRANDING.md`, `README.md`, `INDEX.md`, `PROMPT.md`.

**Ignore entirely:** anything under `_ARCHIVE/` (old gRPC `agezt.proto`, deprecated files, old vision). These are superseded and kept only for history. **Do not generate code from them.**

---

## 2. The five foundational decisions that override the older specs

Several SPEC files were written before these revisions and still use older language. **These five decisions (DECISIONS B0–B0d) are binding and supersede any contradicting text:**

1. **Transport = stdio + JSON-RPC 2.0** (newline-delimited JSON), **NOT gRPC/protobuf.** The contract is `agezt-contract.jsonc` (JSON Schema). Ignore every `.proto`, gRPC, and protobuf reference in the specs.
2. **Plugins default IN-PROCESS.** Core trusted tools (shell/file/http) run inside the kernel for speed and simple debugging. Out-of-process (subprocess over stdio/JSON-RPC) is used ONLY when isolation is required (untrusted/third-party/heavy plugins). Ignore "all plugins out-of-process."
3. **Contract is MINIMAL and grows append-only.** Start with `Event`, the Kernel callback API, and the `Tool` + `Provider` interfaces only. Add the other five plugin interfaces and the rest of the event kinds as their layers are built. Do not implement the whole contract surface up front.
4. **Mutable state store is first-class** alongside the event log. The event log is the audit/replay/revert truth; frequently-read state is read directly from a mutable store (not by folding the log every time).
5. **DAG is a SECOND layer** over a first-party single-agent tool-loop. Build the tool-loop orchestration core first (entirely first-party, no third-party agent SDK). Add the DAG scheduler on top later.

If you ever feel a spec is telling you to use gRPC or make everything out-of-process or fold the log for every read — stop and re-read this section. The specs' architecture/intent is correct; only this plumbing changed.

---

## 3. Engineering rules (from POLICY + DECISIONS)

- **Language:** Go (latest stable; target 1.22+), `CGO_ENABLED=0`, static, multi-arch (amd64/arm64). Web UI: React 19 + TypeScript + Vite + Tailwind 4 + shadcn/ui + React Flow.
- **Dependencies:** stdlib-first. A dependency is allowed only when stdlib is genuinely insufficient, it's pure-Go (no CGO in the core), maintained, MIT-compatible license, and justified in `DEPENDENCIES.md`. Heavy deps live in plugins, not the core.
- **License:** MIT. Add `// SPDX-License-Identifier: MIT` to every source file. `LICENSE` is at repo root.
- **Identity:** ULID for time-ordered entities, BLAKE3 content-address for immutable content (DECISIONS B2).
- **Everything is an event:** every meaningful action is journaled to the append-only, BLAKE3-hash-chained log. Durable-before-publish (append to journal, fsync, then publish to the bus).
- **Security before autonomy:** policy engine (Edict) + trust ladder + `agt halt` + secret redaction must work before any autonomous action is enabled.
- **Brand in one place:** name/paths/strings live in one constants file (`internal/brand`) so the name can change in one edit. Repo/domain is TBD — never hardcode it elsewhere.
- **Test discipline is mandatory:** unit + contract-conformance + replay/property (fold determinism, hash-chain integrity, durable-before-publish) + security (injection, sandbox, redaction) + chaos. Details: `SPEC-16` §2.

---

## 4. What to produce, in order (milestones)

Follow `ROADMAP.md`. Summary of the build sequence; each milestone ends with a runnable success test — do not advance until it passes.

### Milestone 0 — Repository foundation
- Init the Go module + repo layout per `STRUCTURE.md`; add `LICENSE` (MIT), SPDX headers, `DEPENDENCIES.md`, `internal/brand`.
- Move the spec suite under `docs/`.
- Set up the JSON Schema → SDK-type generation from `agezt-contract.jsonc` (build-time, DECISIONS G2).
- CI skeleton (GitHub Actions): build (multi-arch), test, lint, dependency-justification check; image build/sign to GHCR on tag.
- **Exit:** `make build test` green; empty kernel binary prints version.

### Milestone 0.5 — Minimal working core ("core-core")
Build the smallest thing that proves the foundation (ROADMAP §0.5):
- **Event** type; **Journal** (append-only JSONL + BLAKE3 hash chain + ULID + recover + verify); **mutable State store**; in-process **Bus** (subject routing, durable-before-publish).
- First-party **single-agent tool-loop** (canonical dialect-free `Message`/`ToolDef`/`ToolCall`; bounded iterations; journals every step; honors halt).
- One **Provider** (Anthropic, with canonical↔dialect translation) + one offline mock provider for tests.
- One in-process **Tool** (shell).
- **Control plane:** `agt halt`, `agt why <event>`, `agt journal verify`.
- The `agt` CLI with `agt run "<intent>"` running the loop end-to-end.
- **Exit (success test):** `agt run "list the files here and tell me what this project is"` → the agent loops (LLM ↔ shell), answers, every step is journaled, `agt why` explains it, `agt halt` stops it. One process. Chain verifies clean.

### Milestone 1 — MVP (the usable Jarvis)
Per ROADMAP §2 and the MVP cut in `AGEZT-VISION-MASTER.md` §17:
- Governor v1 (subscription-first → cost → latency → local fallback; USD-microcent budget; SPEC-02/10/15) + catalog sync (SPEC-15) + second provider (Ollama).
- DAG scheduler layer over the loop (tool/llm/loop/gate nodes; SPEC-02).
- Planner (intent → DAG; SPEC-02).
- 4 tools: shell, file, http, browser (browser in `container`; SPEC-04/06).
- Edict v1 (policy + trust ladder) + Warden namespace/container (SPEC-06).
- Telegram channel (duplex) + the out-of-process plugin host over stdio/JSON-RPC (SPEC-04, DECISIONS B0a).
- Pulse v1: heartbeat + 2 observers (repo/CI, system-health) + salience + initiative (inform/ask first) + briefing to Telegram (SPEC-03).
- Migration runner + plugin contributions (SPEC-08); ULID identity everywhere; `agt doctor`; zero-config first run.
- **Exit (success test):** the MVP paragraph in ROADMAP §2.2 is true end-to-end.

### Milestones 2–8 — Growth
Memory & Forge → channels & inbox → Web UI (Flow Studio etc.) → hardening & coding agents → reach (tunnels/SDKs/ambient/API) → ecosystem (reflection/marketplace/migration) → scale (mesh/multi-tenant). Each maps to phases in `ROADMAP.md` §3 and `INDEX.md` §3, detailed in the relevant SPECs.

---

## 5. How to work each task

1. Identify the current milestone/task (ROADMAP + TASKS).
2. Read the relevant SPEC sections (Tier 2) plus DECISIONS for any binding choice.
3. Implement to the smallest reviewable unit. Wire it to the journal/bus. Confirm the right events fire.
4. Write tests (unit + the relevant discipline from §3).
5. Run the milestone's success test before advancing.
6. Keep `docs/` and code in sync; if a spec gap appears, note it and pick the simplest choice consistent with DECISIONS — do not invent scope.
7. Commit referencing the task ID (e.g. `P1-CONDUIT-02`).

---

## 6. What success looks like

A user can: state an intent from CLI/Telegram/UI; watch a visible, policy-checked plan execute (or be asked to approve); have Agezt act with real tools and proactively notice & brief what matters without spam; inspect/revert anything via `agt why` / journal / Memory Explorer; extend it with a small plugin; and stop everything with `agt halt`. Static Go binaries, runs on a $5 VPS, security on by default, MIT, open source.

---

## 7. The one rule above all

**When in doubt, DECISIONS.md wins, and §2 of this guide wins over any older spec language.** Build the smallest working core first (M0.5), prove it, then layer. The greatest risk is not missing features — it's never shipping a working core because scope kept growing. Ship M0.5, then the MVP, then grow.

Start at Milestone 0.
