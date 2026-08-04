# AGEZT → Jarvis: Strategic Status and Roadmap Report

> Date: 2026-07-02
> Scope: The entire project (Go core + CLI + embedded React web console) against
> OpenClaw and Hermes Agent; guided by the goal "do what both can do, and be a
> Jarvis beyond either".
> Method: Claims were verified **against the code**, not the documentation (two
> independent discovery scans + the competitors' official docs). Numbers are
> taken from this checkout.

---

## 1. Executive Summary

**Conclusion:** AGEZT is not a "demo skeleton"; it is a genuinely implemented,
exceptionally broad *agentic operating system*. Scale verified:

- ~**170,000 lines** of Go (excluding tests), 5,583 `.go` files, **63 kernel packages**
- **25 channels**, **15 provider families**, **30 tools**, **16 built-in skill packages**, **~70 CLI command groups**
- Embedded React console: **71 real views**, ~73,000 lines of TS/TSX, **~473 Vitest tests** (~1:1 coverage per view), single-EventSource live stream
- **Zero** `panic("unimplemented")`, a handful of TODOs in the core — no placeholder screens

**Position against competitors:**

- **Governance / auditability / reversibility** (Edict policy, BLAKE3
  hash-chained journal, `agt why`, rollback, durable agent identity): AGEZT is
  **clearly ahead of both competitors**. This is the cornerstone of a trustworthy Jarvis.
- **Channel / provider / tool / workflow / scheduling / memory breadth**: **parity
  or better**.
- **Proactivity (Pulse + Initiative), world model (worldmodel), multi-agent
  council/conductor**: **present** in AGEZT, absent or weak in competitors — the real
  differentiating ground.

**Below-parity gaps that must be closed (in priority order):**

1. **Local device experience** — no mobile (iOS/Android) or desktop tray/menu-bar
   companion. OpenClaw's most visible advantage. *(marked "missing" in its own
   parity ledger)*
2. **A popular/populated skill marketplace** — the infrastructure exists, but there is
   no living hub with thousands of packages like ClawHub.
3. **Live persistent browser tab** — Playwright is real, but it is not a persistent
   multi-step live tab + DOM stale-ref session (partial).
4. **LLM-based skill curator** — Hermes's automatic consolidation; AGEZT has only a
   deterministic curator.
5. **Context ergonomics** — `@file/folder/diff/URL` references and
   AGENTS.md/CLAUDE.md/SOUL.md import are not first-class.

**The single biggest opportunity to move beyond parity:** **there is no deep
research harness** (verified in the code — no multi-source, contradiction-verified
research engine exists). For a Jarvis this is as critical as proactivity, and it is
an area where competitors are weak too.

---

## 2. Verified Current State (evidence based)

### 2.1 Autonomy stack — all real, multi-file code

| Capability | Evidence | Maturity |
|---|---|---|
| Memory (vector + consolidation + retention + profile) | `kernel/memory/` (19 files) | Mature |
| World model (entity/relation + decay + resolve) | `kernel/worldmodel/` (10) | Real, **differentiator** |
| Pulse (proactive engine: briefing/health/observers/salience/reaper) | `kernel/pulse/` (18) | Mature |
| Pulse Initiative (observation → action, governed) | `pulse-initiative` (M999) | Real, **differentiator** |
| Standing orders (cron-triggered persistent instructions) | `kernel/standing/` (6) | Real |
| Cadence / typed schedules | `kernel/cadence/` (9) + `kernel/scheduler/` | Real |
| Workflow engine (DAG + template) | `kernel/workflow/` (7) + FlowStudio | Real |
| Guardians / self-repair / watchdog | `plugins/builtinguardians/` + `cmd/agezt/{auto_repair,watchdog}.go` | Real |
| Voice (STT/TTS: cartesia/deepgram/elevenlabs + hands-free mode) | `kernel/stt/` + `plugins/providers/voice/` + `Voice.tsx` | Real |
| User profile (learning the operator) | `user-profile` (M1000) | Real, **differentiator** |
| Governed autonomy (trust ceiling, edict, approvals) | `kernel/controlplane/autonomy.go` + `kernel/edict/` | Real, **moat** |

### 2.2 Browser automation — real Playwright (not just fetch)

- `browser.read`: SSRF-protected stdlib fetch (netguard, allowlist on every redirect
  hop, 4MB cap). The cheap default.
- `browser.action`: a **real headless Chromium Playwright bridge**
  (`plugins/builtinskills/browseruse/scripts/browse.mjs`, 395 lines). 4 profiles
  (isolated/session/user-attached/remote-cdp), verb tools
  (`browser.open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close`),
  screenshots + downloads → artifact store. Requires a Node/Playwright installation
  (opt-in). **Missing:** a persistent live-tab process + DOM stale-ref invalidation.

### 2.3 Web console — shippable, broad, no placeholders

67 views; all connected to live API/SSE. Notable ones: `Jarvis.tsx` (ambient
assistant), `Council.tsx` + `Conductor.tsx` (multi-agent orchestration), `Voice.tsx`,
`Overseer.tsx`, `Workboard.tsx`, `World.tsx`, `Autonomy.tsx`. React 19 + Tailwind
v4 (oklch tokens), `@xyflow/react` graphs, an in-house chart library, a custom router.
A dual-theme consistent design system. ~473 curated tests.

---

## 3. Competitor Analysis

### 3.1 OpenClaw (personal AI assistant gateway — "the lobster way")

Its strength: personal gateway packaging + device experience + skill distribution.

- **Channels:** Discord, iMessage, Signal, Slack, Telegram, WhatsApp, WebChat +
  plugins (Matrix, Teams, Twitch, Nostr, Zalo…)
- **Browser:** real Chrome/CDP (Playwright), JS-heavy sites, session-based login,
  multi-step flows
- **Skills + ClawHub:** SKILL.md based, a public marketplace with **thousands of skills**
- **Memory:** Markdown + search backends + Honcho
- **Automation:** cron + heartbeat scheduler, standing orders, taskflow
- **Media/voice:** generation + understanding + TTS/STT (multi-provider)
- **35+ providers**, multiple web search providers (Brave/DDG/Exa/Tavily)
- **DEVICES (distinguishing):** iOS/Android nodes (camera, device commands,
  screen recording, location) + a macOS menu bar companion + Windows Hub
- Home automation, local-first privacy, DM allowlist/pairing security, multi-agent
  routing

### 3.2 Hermes Agent (NousResearch — agent CLI/gateway)

Its strength: skill self-improvement + terminal backend ergonomics + a durable
multi-agent Kanban.

- **Skills + Curator:** a helper-model job that periodically reviews agent-produced
  skills **with an LLM**, prunes them, consolidates them, and moves them active→stale→archived
- **Memory:** built-in + Honcho + Mem0 + RetainDB (pluggable)
- **Context files:** automatic discovery of `.hermes.md`, `AGENTS.md`, `CLAUDE.md`, `SOUL.md`,
  `.cursorrules`; **`@` context references** (file/folder/diff/URL)
- **Checkpoints & rollback:** automatic snapshot before file changes + `/rollback`
- **Kanban (distinguishing):** a separate SQLite board per project; profile lanes,
  dependencies, heartbeat, comments, blocking, crash recovery; the `kanban_*` toolset
- **Batch processing:** hundreds/thousands of prompts in parallel
- Subagent delegation, sandboxed Python code execution (programmatic tool access),
  event hooks, voice, browser (Browserbase/BrowserUse/local CDP), vision/image paste,
  image generation, **IDE integration (VSCode/Zed/JetBrains via ACP)**, SOUL.md personality,
  skins/themes, plugins

---

## 4. Comparison Matrix

Legend: 🟢 AGEZT ahead · 🟡 parity · 🔴 AGEZT behind

| Area | AGEZT | OpenClaw | Hermes | Status |
|---|---|---|---|---|
| Governance/policy (Edict) | ✔ core | weak | medium (hooks) | 🟢 |
| Audit trail (hash-chain journal + `why`) | ✔ | log | log | 🟢 |
| Durable agent identity | ✔ roster | session | profile | 🟢 |
| Undo/rollback | ✔ (file/skill/workflow/config) | partial | ✔ (/rollback) | 🟡 |
| Typed schedules + standing | ✔ | ✔ | ✔ | 🟡 |
| Workflow/DAG engine | ✔ + visual canvas | Lobster | — | 🟢 |
| Memory | ✔ vector+consolidation | Markdown/Honcho | pluggable | 🟡 |
| World model | ✔ | — | — | 🟢 |
| Channels | 25 | ~7+plugins | messaging | 🟢/🟡 |
| Providers | 15 families | 35+ | many + pools | 🟡 |
| Browser automation | Playwright (opt-in) | Chrome/CDP always | many backends | 🟡 |
| Skill lifecycle + forge | ✔ content-addressed | SKILL.md | ✔ | 🟢 |
| Skill **LLM curator** | deterministic | — | ✔ LLM | 🔴 |
| Skill **marketplace (populated hub)** | infrastructure exists | ClawHub (thousands) | hub | 🔴 |
| Multi-agent work queue | Workboard (partial UX) | routing | Kanban (mature) | 🟡 |
| Batch processing (100-1000 parallel) | indirect via workflow | — | ✔ | 🔴 |
| Credential pools (key rotation) | multi-key keyring | — | ✔ | 🟡 |
| Voice/STT/TTS | ✔ 3 vendors + hands-free | ✔ | ✔ 10 | 🟡 |
| Image generation | image provider | ✔ | FAL.ai 9 | 🟡 |
| MCP | ✔ + 43-preset catalog | ✔ | ✔ | 🟡 |
| OpenAI-compatible API + SDKs | ✔ (Py/TS/Rust) + ACP | — | ✔ | 🟢 |
| Multi-tenant isolation | ✔ | — | — | 🟢 |
| **Mobile device node** | — | ✔ (camera/location/screen) | — | 🔴 |
| **Desktop tray/companion** | web-only | ✔ (menu bar/Hub) | dashboard | 🔴 |
| **IDE extension** | ACP surface exists | — | ✔ shipped | 🔴 |
| Context `@` references + context-file import | partial | — | ✔ | 🔴 |
| **Proactivity + acting Initiative** | ✔ (Pulse) | heartbeat | scheduled | 🟢 |
| **Deep research harness** | — | web search | web search | 🔴 (weak in both → opportunity) |

---

## 5. Beyond Parity: The Real Jarvis Differentiators

What separates a "Jarvis" from a competing chat agent is not breadth, but
**trustworthy proactive autonomy + deep reasoning + ambient presence**. The ground
AGEZT already stands on is unique in this direction:

1. **Governed proactivity = the moat.** Pulse observes, Initiative can *act*,
   every action is bounded by Edict and traceable in the journal. Neither
   OpenClaw's heartbeat cron nor Hermes's scheduled tasks have *governed initiative*
   at this level. Jarvis "doing the right thing before you ask" is only possible with
   trust — and the audit/rollback layer that provides that trust exists in AGEZT.

2. **World model + memory → anticipation.** worldmodel (entity/relation/decay) alone
   is absent in competitors. Wiring it to proactivity makes "anticipatory" behavior
   possible (preparing the user's next need in advance).

3. **Society-of-agents (Council/Conductor).** A *reasoning* layer beyond Kanban: a
   structure where multiple agents debate and reach agreement, orchestrated by a
   conductor, already exists in the UI. Productizing it is the way to surpass Hermes's Kanban.

4. **Ambient presence.** Jarvis.tsx + hands-free voice + wake word + multi-channel
   access. Once a device companion is added, the "everywhere" experience is complete.

5. **Auditable reversibility.** "Authorization before the action; causality/evidence/undo
   after the action." This is the fundamental difference between enterprise and personal trust.

---

## 6. Priority Roadmap

### P0 — The most visible parity gaps (0–60 days)

1. **Device/companion layer (highest impact).**
   - The node registry already exists → build a lightweight **desktop tray companion**
     on top of it (start/stop, health, approvals, push-to-talk, notifications, tunnel status).
   - **PWA/mobile companion**: approvals, inbox/alerts, voice messages, run status,
     share-sheet webhook target. (A functional answer to OpenClaw's mobile node.)
   - Device routing policy: which node may run browser/shell/HA/media.

2. **Live browser tab session.** Promote the `browser.action` verbs to a persistent live
   Chromium process + DOM-level stale-ref invalidation; E2E fixtures for
   login/iframe/download/SPA/cookie transfer. (An answer to OpenClaw's "always-on browser"
   experience.)

3. **Skill LLM Curator (in shadow mode).** On top of the deterministic curator, a
   helper-model job that looks at usage metrics and **proposes** consolidation/patches;
   it never deletes, always archive/revertable. (Hermes Curator parity.)

### P1 — Ergonomics + marketplace (60–120 days)

4. **Context ergonomics.** `@file/folder/diff/URL` context references +
   **injection-scanned import** of AGENTS.md/CLAUDE.md/SOUL.md/.cursorrules.
   Migration: OpenClaw workspace + Hermes MEMORY/USER/SOUL importers (dry-run,
   never overwrite without a backup).

5. **Marketplace trust + distribution.** A unified marketplace UI for
   skills/plugins/MCP/channels/exec-profiles/workflows + a **per-package trust card**
   (publisher identity, signature, BLAKE3, requested permissions, files, install script,
   network domains, scanner findings, quarantine, update policy). Optional
   ClawHub/agentskills import + pre-scan.

6. **IDE extension.** The ACP surface exists → ship a (minimal) VSCode extension.

7. **Batch + credential pools.** A named batch-processing surface (hundreds of prompts,
   on top of workboard); automatic rotation/distribution for the multi-key keyring.

### P2 — Differentiators that surpass Jarvis (120 days+)

8. **Deep research harness (the biggest "beyond" opportunity).** Multi-source fan-out
   search → deep reading → **contradiction/adversarial verification** → cited synthesis.
   It sits on top of Pulse + worldmodel + workflow; the patterns from the DeerFlow report
   (middleware + deferred tool discovery + citation) are the guide. Strong in neither
   OpenClaw nor Hermes.

9. **Anticipatory autonomy.** worldmodel + memory + pulse → preparing the user's next
   need in advance (briefing, ready draft, alert) — within the governance boundary.

10. **Productizing society-of-agents.** Mature Council/Conductor with live multi-agent
    reasoning + a delegation graph + Workboard lane integration; graph-dependency view,
    inline artifact/diff preview, an "ask a human" flow.

11. **Cloud terminal artifact lifecycle.** Complete the K8s job lifecycle loop
    (close exec-profile parity).

---

## 7. The Three Most Critical Points

1. **The code is ready; productization and device reach are missing.** The places where
   AGEZT falls behind are almost entirely *runtime/distribution* (mobile, tray, a populated
   marketplace, live tab) — not core capability. These are visible gaps, but fast to close.

2. **AGEZT's winning position is not "more tools".** It is governance + causality +
   undo + durable identity + proactivity. Competitors are structurally weak on this axis;
   messaging and demos should foreground that advantage.

3. **The single biggest "beyond" move: deep research + anticipatory proactivity.**
   Both are weak in competitors, and AGEZT's ground (pulse/worldmodel/workflow/journal) is
   ready for them. The real "Jarvis" feeling comes from here.

---

## 8. Conclusion

Today AGEZT is a mature agentic OS that is **very close to parity and ahead on many
axes**. The work required to "do what both do" is not building capabilities from scratch,
but **productizing five visible gaps**: device/companion, live browser, LLM
curator, context ergonomics, and a populated, secure marketplace. And to "be a Jarvis
beyond both", the ground is already unique: add a **deep research harness** and
**anticipatory autonomy** on top of **governed proactivity + world model + auditable
undo**.
