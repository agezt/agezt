# Agezt — Branding (BRANDING.md)

> Status: Draft v0.1 · Language: English · Domain/Repo: TBD (to be decided later)
> Defines naming, voice, visual direction, and the component naming system. This is a working draft; the final name is not locked — alternatives are listed so it can be swapped from one place.

---

## 1. The name

**Agezt** — in ancient Rome, the officer who walked ahead of a magistrate carrying the *fasces*, embodying and clearing the way for lawful authority. The metaphor: an agent that acts on your behalf, autonomously, but always **under your authority and order** — never a loose cannon. "Autonomous, but under authority" is the entire safety thesis of the system in one word.

It also sits naturally in the mythological/role-based naming tradition of the surrounding portfolio (Argus, Kronos, Karadul, Cerberus, Moirai, Hermod, …) and stands deliberately apart from competitor names (OpenClaw's lobster whimsy, Hermes's messenger-god). Where Hermes is the *messenger*, the Agezt is the one who *clears the path and keeps order* — a pointed contrast.

### 1.1 Alternatives (if Agezt is rejected)
- **Praxis** — "action/practice"; emphasizes the doing. Softer, modern.
- **Aedile** — Roman official of public works/oversight; "the one who maintains." Obscure but evocative.
- **Vala** — short, warm, brandable; less conceptually tied. (Note: avoid clashing with existing tools.)
- A Turkish/mythic option could be explored for a warmer, more personal "Jarvis" feel.

> Decision deferred per project owner. Everything below assumes "Agezt" and is written so the name lives in one tokens file for easy global change.

---

## 2. Positioning statement

> **Agezt is an agentic operating system: a single static Go binary that turns intent into auditable, reversible action — and, on its own initiative, watches your world and tells you what matters.** Out-of-process plugins in any language, a visual flow studio, deterministic-plus-LLM orchestration, and a proactive heart. The open, transparent Jarvis you actually control.

One-liner: **"Autonomous, under your authority."**

Alternative taglines:
- "The agent that clears the path."
- "Your world, watched. Your rules, kept."
- "An open Jarvis you can audit."

---

## 3. Voice & tone

- **Precise, calm, technical-but-human.** This is infrastructure that takes autonomous action; the voice must earn trust, not hype. No breathless AI-marketing.
- **Transparent.** We talk openly about limits, safety, and how decisions are made (it's our differentiator). Never overclaim autonomy.
- **Respectful of the user's control.** Language always frames the system as serving and answerable to the user ("under your authority").
- **Developer-native.** Speaks to engineers; assumes intelligence; explains the *why* behind design.
- Avoid: "magic", "effortless", "it just knows" — these undercut the trust/auditability story.

---

## 4. Component naming system

Internal component names follow the Roman-authority theme, giving the system a coherent vocabulary. Each is also the CLI/subsystem identifier.

| Component | Name | Meaning |
|---|---|---|
| The binary / platform | **Agezt** | the whole |
| CLI command | **agt** | short form |
| Policy engine | **Edict** | the law that governs action |
| Provider/limit governor | **Governor** | (provincial) authority over resources |
| Scheduler of triggers | **Chronos** | time/triggers (already in portfolio vocabulary) |
| Self-improvement | **Forge** | where skills are made & tempered |
| Sandbox/isolation | **Warden** | guards the boundary |
| Proactive heart | **Pulse** | the heartbeat |
| Observers | **Observers** | plain, descriptive |
| Reflection | **Reflect** | meta-cognition |

This vocabulary should be used consistently in docs, UI labels, logs, and CLI help.

---

## 5. Visual direction (Web UI & marks)

> Per the frontend philosophy: distinctive, not generic-AI. Dark-first "control room", precise, restrained — this is a serious instrument, not a toy. (Final design tokens decided during P5.)

### 5.1 Concept
A **control room / situation room** aesthetic: dark technical canvas, precise monospace-adjacent typography for data, a refined display face for headings, one decisive accent color for *live/active/authority* states. The fasces motif can inform a minimal mark (bundled rods → "many capabilities, one handle / unity under authority") without being literal or ornate.

### 5.2 Color (direction, tokens TBD)
- **Base:** deep near-black/charcoal ground; layered surfaces for depth (not flat).
- **Accent:** a single authoritative accent for active/live state (a decisive metallic/bronze nods to Rome and to "authority"; alternatively a sharp signal color for live execution). Avoid the cliché purple-on-white AI gradient.
- **Semantics:** clear, restrained states for queued/running/done/failed and for salience levels (info→urgent) — legibility over decoration.

### 5.3 Typography (direction)
- A characterful **display** face for headings/marks (not Inter/Roboto/Space Grotesk).
- A clean, technical **body/data** face; tabular numerals for cost/metrics; a mono for code/logs/journal.

### 5.4 Motion
- High-impact, purposeful: animated edge-flow during DAG execution (the signature moment), staggered reveals on load, restrained micro-interactions. Motion communicates *liveness* (the system is acting), never mere flourish.

---

## 6. Logo / mark (direction)

A minimal, geometric **fasces-inspired** mark — a bundle of vertical strokes unified by a single band — reading as "many plugins/capabilities, one governed core." Monochrome-first; works at favicon scale and as a CLI banner (ASCII variant for the TUI). No literal axe; keep it abstract and modern.

---

## 7. Naming the artifacts

- Binary: `agezt` (+ `agt` shim).
- Config dir: `~/.agezt/`.
- Plugins: `agezt-plugin-<name>` convention; first-party prefixed by type (`channel-telegram`, `tool-browser`, …).
- SDKs: `agezt-sdk-{go,ts,py,rust}`; scaffolder `create-agezt-plugin`.
- Env: `AGEZT_*`.

---

## 8. What we deliberately do NOT brand around

- No anthropomorphic mascot competing with OpenClaw's lobster — Agezt's brand equity is *trust, transparency, control*, not whimsy.
- No "fastest/smartest" model claims — Agezt is provider-agnostic; the brain is swappable. We brand the *runtime and governance*, not a model.
- No closed/enterprise-only framing — open, self-hostable, auditable is core identity.

---

## 9. Messaging pillars (for README/site/launch — TBD timing)

1. **Auditable autonomy** — every action is journaled, explainable (`agt why`), reversible. Hash-chained truth.
2. **Under your authority** — trust ladder, policy-as-code, one-command halt. You set how far it goes.
3. **Proactive, not noisy** — it notices and tells you what matters (salience), not everything (no spam).
4. **One binary, infinite reach** — static Go core, out-of-process polyglot plugins, any LLM/subscription, any channel.
5. **Visually programmable** — Flow Studio: see and shape what your agents do.

---

*Next: README.md (the public-facing project README) and PROMPT.md (the single-shot build prompt).*
