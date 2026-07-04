# Agezt — Capability Army: Ecosystem Interop, Catalog & Growth (SPEC-13)
> ⚠️ **AUTHORITY NOTICE:** This document is subordinate to `DECISIONS.md`. Where anything here conflicts with DECISIONS, **DECISIONS wins** — especially the foundational revisions **B0 (transport = stdio + JSON-RPC 2.0, NOT gRPC/protobuf)**, **B0a (plugins default in-process; out-of-process only for isolation)**, **B0b (minimal contract, grows append-only)**, **B0c (mutable state store is first-class alongside the event log)**, and **B0d (DAG is a second layer over a first-party single-agent tool-loop)**. Any mention of gRPC, protobuf, or "all plugins out-of-process" in this file is superseded. The contract source of truth is `agezt-contract.jsonc`.


> Status: Active · Domain: github.com/agezt/agezt · License: MIT · Language: English
> Depends on: SPEC-04 (plugins, MCP bridge), SPEC-05 (skills/Forge), SPEC-08 (marketplace), SPEC-10 (coding delegation)
> Defines how Agezt ships and grows a large arsenal of capabilities — by embracing existing ecosystems (MCP, agentskills.io/ClawHub), shipping a categorized first-party catalog, and growing itself. A platform's real value is how much it can do on day one and how fast that grows.

---

## 0. Strategy: don't build the army from scratch — assemble it from three sources

1. **Embrace & extend existing ecosystems** (the strongest card): consume what OpenClaw/Hermes/the MCP world already built.
2. **A categorized first-party catalog** shipped in the box.
3. **Self-growth:** the system creates new capabilities on its own (governed).

The result substantiates the "does everything they do, and more" claim — literally, because we can *run their capabilities* while adding governance they lack.

---

## 1. Source 1 — Ecosystem interop

### 1.1 MCP universe
The `mcp-bridge` plugin (SPEC-04 §8) adapts any MCP server into Agezt `Tool` capabilities (stdio + HTTP transports, tool filtering, sampling). Consequence: the entire MCP ecosystem — Composio's 1000+ integrations, GitHub MCP, and every community MCP server — works on day one. Ours are journaled, policy-gated, sandboxed — a strict upgrade over raw MCP use.

### 1.2 agentskills.io / ClawHub compatibility
A `SKILL.md` adapter ingests skills written to the agentskills.io open standard (which Hermes/ClawHub use): parse frontmatter (name/description/triggers), index the body, and load it into Agezt's skill system (SPEC-05). Hundreds of existing skills become usable without rewriting. Agezt skills additionally get versioning, shadow-testing, and reversibility on top.

### 1.3 Migration as capability transfer
`agt migrate openclaw|hermes` (SPEC-09 §6) imports a user's existing settings/memories/skills via the standard import pipeline. A migrating user lands in a *populated* system, not an empty one.

### 1.4 Be a backend for other tools
Agezt's OpenAI-compatible API + subscription proxy (SPEC-07 §9) lets external tools (Cursor, Cline, VS Code, Aider) drive Agezt as if it were a model endpoint — and conversely Agezt drives them as coding agents (SPEC-04 §4). Two-way interop with the existing tool landscape.

---

## 2. Source 2 — First-party catalog (shipped in the box)

Capabilities come in two forms (like Hermes's tools + skills): **tools** (deterministic actions) and **skills** (LLM-injected procedures). Organized by category:

- **Dev / infra:** git, GitHub/GitLab, CI/CD, Docker, kubectl, SSH, terminal backends, code execution, coding-agent bridges (Claude Code/Codex/Aider).
- **Web / data:** browser automation, web search, scraping/extraction, HTTP, RSS, x_search.
- **Productivity:** email, calendar, Notion, Google Workspace, file management, document generation (docx/pdf/xlsx/pptx).
- **Communication:** all channels (Telegram/Discord/Slack/WhatsApp/Signal/SMS/Matrix/Teams/Home Assistant/Webhook).
- **Media:** image gen/analysis, STT/TTS, video gen, OCR.
- **Knowledge / AI:** RAG, memory query, embeddings, summarization, translation.
- **System / ops:** monitoring (uptime/health, Argus/AnubisWatch-style), backup/export, scheduling, alerting.
- **Domain verticals:** owner-specific — football analytics (Opta), SoccerLLM, and other vertical capabilities as plugins/skills.

The catalog is plugins (out-of-process, crash-isolated, polyglot) so it can grow without bloating the core (POLICY §1.2).

---

## 3. Source 3 — Self-growth (the real differentiator)

The army isn't static; it multiplies:
- **Planner** detects a missing capability for a task and proposes a plugin or skill (SPEC-02 §4.1).
- **Forge** authors a new skill from experience, shadow-tests it, and enlists it — governed and reversible (SPEC-05 §5).
- **Coding delegation:** for a missing *tool*, the system can have a coding-agent write the plugin's code against the SDK, build it, shadow-test, and add it (SPEC-10 §5, SPEC-04 §4).
- **Reflection** prunes/strengthens the arsenal over time (SPEC-05 §6).

You start with N capabilities; the system grows toward many more — auditably. Competitors' skill libraries are largely static or human-curated; Agezt's grows itself under governance.

---

## 4. SDK & CLI as growth engines

- **SDK** (`agezt-sdk-{go,ts,py,rust}` + `create-agezt-plugin`): a developer adds a capability in ~20 lines; polyglot and type-safe (vs OpenClaw's TS-only skills / Hermes's Python-only plugins). This is how third parties grow the army.
- **CLI** grows with capabilities: each plugin contributes subcommands (SPEC-08 §1), so `agt github pr list`, `agt telegram send`, etc. appear as plugins are added.
- **Marketplace** (SPEC-08 §7) distributes plugins/skills/workflows/standing-orders as signed, content-addressed artifacts.

---

## 5. Quality bar for the army

Volume without quality is noise. Every capability — first-party, imported, or self-grown — is subject to:
- **Contract conformance** (SPEC-04) — it behaves per the interface.
- **Capability eval** (SPEC-14) — does this skill/tool actually succeed at its job; regression-tested.
- **Sandboxing & policy** (SPEC-06) — runs isolated, governed; untrusted (imported/self-grown) starts in `container` with default-deny.
- **Provenance** — where it came from (first-party / marketplace / self-authored) is recorded; self-grown capabilities are clearly marked and enter via the trust ladder.

---

## 6. Phase placement

- `mcp-bridge` + first-party core tools/channels/providers: **Phase 1–4**.
- agentskills.io/ClawHub adapter: **Phase 4–5**.
- Self-growth (Forge skills): **Phase 2**; coding-authored plugins: **Phase 6**.
- SDK polyglot + scaffolder + subscription proxy: **Phase 7**.
- Marketplace + domain verticals: **Phase 8**.
- Competitor migration import: **Phase 9**.

---

## 7. Open questions

1. agentskills.io standard version tracking and how faithfully imported skills map to Agezt's lifecycle.
2. Trust model for self-authored plugins (coding-agent-written code) before enlistment — how much shadow-testing/sandbox is enough.
3. Quality gating for marketplace submissions (automated eval vs curation).
4. Avoiding capability sprawl: dedup/consolidation across imported + first-party + self-grown (Forge consolidation extends here).

---

*Next: SPEC-14 (Resilience, Human-in-the-Loop, Eval, RBAC, Onboarding & operational gaps), then cross-doc updates + refreshed master index.*
