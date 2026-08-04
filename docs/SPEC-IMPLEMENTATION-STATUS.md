# AGEZT — SPEC Implementation Status (SPEC ↔ Code Status Matrix)

> **Date:** 2026-07-04 (last updated: 2026-07-06)
> **Branch:** `main` (HEAD: `ef7b412d`)
> **Status:** ARCHIVED — Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Content retained for historical reference.
> **Other references:** `docs/SYSTEM-AUDIT-REPORT.md`, `docs/MISSING-PARTS-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/JARVIS-VISION-2026.md`, `docs/OPENCLAW-HERMES-ROADMAP.md`.

---

## 0. Conventions

This matrix presents the 16 SPECs in a single table. Each row:

```
SPEC-XX <title>
- **Status:** shipped | partial | design-only | not-started
- **Coverage %:** 0-100 (percentage of production code, from an empirical code-base scan)
- **Last M-report:** the highest M-number on disk (or referenced in the CHANGELOG)
- **M-report count:** number of PHASE-MXXX reports on disk related to the SPEC
- **Code sites:** number of related Go packages (src/test)
- **Major gaps:** the main missing items
- **Linked items:** linked F-* / N-* / H-* / D-* items
- **Commit hint:** a notable commit hash when known
```

### Status Dictionary

| Status | Meaning |
|---|---|
| **shipped** | All items met; remaining items in the spec's scope are either absent or in the P3 backlog. |
| **partial** | Significant portions (≥50%) done; clear P0/P1 items still open. |
| **design-only** | Design only; little or no code-base presence. |
| **not-started** | Implementation has not begun; in the P3 backlog. |

### Coverage % (empirical)

The simple average of three signals:

1. M-report density (proportional, max 50)
2. Go source code (proportional, max 30)
3. Frontend/UI presence (proportional, max 20)

The numbers are **approximate**; corrected by manual review. Coverage % alone is not enough to decide; it is read together with "Major gaps" and "Linked items".

---

## 1. Matrix: Status of the 16 SPECs

### SPEC-01 Plugin Contracts & Event Schema
- **Status:** shipped
- **Coverage:** ~95%
- **Last M-report:** M518 (disk), M1001 (CHANGELOG)
- **M-report count:** 18
- **Code sites:** `kernel/agentgw`, `kernel/bus`, `kernel/event`, `kernel/journal`, `kernel/ulid` (total src ~7, test ~9)
- **Major gaps:** Very few — schema fuzzing continues.
- **Linked items:** —
- **Commit hint:** `agezt-contract.jsonc` audit-gated (in PR)

### SPEC-02 Kernel (runtime)
- **Status:** shipped
- **Coverage:** ~95%
- **Last M-report:** M923 (disk), M1002 (CHANGELOG)
- **M-report count:** 92
- **Code sites:** `kernel/agent` (9 src, 19 test), `kernel/runtime` (27 src, 53 test), `kernel/controlplane` (87 src, 103 test), `kernel/governor` (7 src, 27 test), `kernel/workflow` (4 src, 3 test), `kernel/planner` (3 src, 4 test)
- **Major gaps:** Weak spot: a very high file count (`controlplane` 87 src) — being sliced via `REFACTOR-A1-CONTROLPLANE-PLAN.md`.
- **Linked items:** N-1 (workflow→agent wake), H-01 (stale refactor index)
- **Commit hint:** `5c4f7c53` (feat(resume): survive daemon restart)

### SPEC-03 Pulse
- **Status:** shipped
- **Coverage:** ~88%
- **Last M-report:** M903 (Reaper)
- **M-report count:** 13
- **Code sites:** `kernel/pulse` (10 src, 8 test), `kernel/alerter` (1+1), `kernel/anomaly` (2+2), `kernel/workboard` (1+2)
- **Major gaps:** Initiative (M999) shipped; Reaper (M903) shipped; salience scoring (M523-527) shipped. **Anticipatory autonomy (F-12)** is missing — preparing the user's next need in advance.
- **Linked items:** F-12 (anticipatory), N-2 (why quieted audit), N-3 (guardian schedule)
- **Commit hint:** `PHASE-M903-AUTONOMOUS-REAPER-REPORT.md`

### SPEC-04 Plugin Interfaces
- **Status:** shipped
- **Coverage:** ~90%
- **Last M-report:** M912 (MCP catalog library 43 preset)
- **M-report count:** 44
- **Code sites:** `kernel/plugin` (7 src, 23 test), `kernel/mcp` (3+3), `plugins/sdk` (1+2)
- **Major gaps:** **The `tools_box` package has src=0** (only test=1) — the actual plugins live under `plugins/tools/*`. The library-style plugin registry is good. There is still a **`plugins/builtinmarket/` plugin fork** speculation.
- **Linked items:** F-19 (external vault — plugin side), F-11 (research harness — deep plugin)
- **Commit hint:** `PHASE-M912-MCP-CATALOG-LIBRARY-REPORT.md`

### SPEC-05 Memory, World Model, Skills, Forge
- **Status:** shipped (parity and beyond)
- **Coverage:** ~85%
- **Last M-report:** M902 (Forge bias), M896 (Office), M890 (Archive tools), M889 (SQL DB), M894 (Crypto), M893 (SSH), M892 (Email), M891 (HTTP API), M890 (Archive), M889 (SQL), M866 (PDF), M865 (Web research), M864 (Git ops), M863 (Docker), M861 (Data analysis), M859 (Overseer dashboard)
- **M-report count:** 56 (most for skill lifecycles)
- **Code sites:** `kernel/memory` (7 src, 12 test), `kernel/worldmodel` (4 src, 6 test), `kernel/skill` (7 src, 15 test), `kernel/reflect` (1 src, 1 test), `kernel/brain` (M804 distiller), `plugins/builtinskills` (16 packages)
- **Major gaps:** **F-4 LLM skill curator** (Hermes parity) is missing. Auto-promote and shadow-eval are deterministic; not LLM-judged.
- **Linked items:** F-4 (LLM curator), F-12 (anticipatory)
- **Commit hint:** `PHASE-M902-FORGE-BIAS-REPORT.md`

### SPEC-06 Security, Sandbox & Warden
- **Status:** shipped (governance moat)
- **Coverage:** ~95%
- **Last M-report:** M495 (CREDS KDF), M494 (CREDS KDF known answer), M476, M474 (TENANT-BLANK-TOKEN-HEAL)
- **M-report count:** 52
- **Code sites:** `kernel/edict` (3 src, 8 test), `kernel/warden` (7 src, 6 test), `kernel/netguard` (1+1), `kernel/redact` (1+6), `kernel/creds` (13 src, 14 test)
- **Major gaps:** **Windows/macOS warden** caveat (system-audit note); Linux complete. **F-26 OpenTelemetry** missing. **F-16 RBAC** missing.
- **Linked items:** F-16, F-17 (RBAC), F-26 (OpenTelemetry)
- **Commit hint:** `PHASE-M494-CREDS-KDF-KNOWN-ANSWER-REPORT.md`

### SPEC-07 UI & Surfaces
- **Status:** shipped (beyond parity)
- **Coverage:** ~92%
- **Last M-report:** M916 (Tools capability gallery), M913 (Attention+approvals bell), M911 (Roster visual cards), M909 (Agents visual gallery)
- **M-report count:** 72
- **Code sites:** `kernel/webui` (9 src, 13 test), `frontend/src/views/` (71 tsx), `frontend/src/components/` (48 tsx), `frontend/src/views/Council.tsx`, `frontend/src/views/Conductor.tsx`, `frontend/src/views/Workboard.tsx`, `frontend/src/views/World.tsx`, `frontend/src/views/Autonomy.tsx`
- **Major gaps:** **F-1 mobile companion** (PWA, P0), **F-2 desktop tray** (P0), **F-20 widget SDK** (P3).
- **Linked items:** F-1, F-2, F-20
- **Commit hint:** `PHASE-M916-TOOLS-CAPABILITY-GALLERY-REPORT.md`

### SPEC-08 Operability (updates, migrations, contributions, changelog)
- **Status:** shipped
- **Coverage:** ~80%
- **Last M-report:** M585 (certify-main), M509 (sjari), M422 (plugin zombie pin)
- **M-report count:** 15
- **Code sites:** `kernel/market` (8 src, 3 test), `kernel/plugin` (cross-cutting), `kernel/state` (cross-cutting)
- **Major gaps:** **Cross-document debt**: `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md` now exist (2026-07-04, P0-2/P0-3 closed). **CHANGELOG reorg (H-04)** is still open (646 KB).
- **Linked items:** H-04 (CHANGELOG reorg), F-21 (widget marketplace — shared market infrastructure)
- **Commit hint:** `PHASE-M585-...-REPORT.md`

### SPEC-09 Identity, Export/Import & Backup
- **Status:** shipped
- **Coverage:** ~85%
- **Last M-report:** M847 (Skill bundles), M846 (Agent graveyard), M557 (TENANT)
- **M-report count:** 30
- **Code sites:** `kernel/tenant` (1+2), `kernel/roster` (2+1), `kernel/standing` (3+3), `kernel/ulid` (1+1)
- **Major gaps:** **`agt migrate openclaw|hermes` (F-10)** is missing (only vault_migrate exists). **N-4 graveyard destructive auto-archive** awaits owner sign-off.
- **Linked items:** F-10 (migrate command), N-4 (graveyard destructive)
- **Commit hint:** `PHASE-M846-AGENT-GRAVEYARD-REPORT.md`

### SPEC-10 LLM, Context & Routing
- **Status:** shipped (differentiator)
- **Coverage:** ~95%
- **Last M-report:** M907 (OpenAI toolname collision), M877 (Chat timestamp), M825 (Chat Markdown links)
- **M-report count:** 72
- **Code sites:** Cross-cutting — `kernel/runtime` (27+53), `kernel/controlplane` (87+103), `kernel/governor` (7+27)
- **Major gaps:** **The `tools_box` package has src=0** (noted in SPEC-04); `kernel/chatgptauth` already exists (M937, M935). In very good shape.
- **Linked items:** —
- **Commit hint:** `PHASE-M907-PHASE-M7-CHAT-GENRES-REPORT.md` ... various

### SPEC-11 Deployment & Runtime Environments
- **Status:** partial
- **Coverage:** ~70%
- **Last M-report:** M863 (Docker services skill), M541 (Peer federation), M532 (RUNS costband)
- **M-report count:** 13
- **Code sites:** related to `kernel/peer` (cross-cutting)
- **Major gaps:** **F-14 K8s job lifecycle** missing. **Windows/macOS warden** caveat. **Linux prlimit64** shipped but limited on other OSes.
- **Linked items:** F-14 (K8s job), H-04 (implicit CHANGELOG reorg)
- **Commit hint:** `PHASE-M863-DOCKER-SERVICES-SKILL-REPORT.md`

### SPEC-12 Widget System & SDK
- **Status:** design-only
- **Coverage:** ~5%
- **Last M-report:** None
- **M-report count:** 0
- **Code sites:** **No widget directory/code at all** — no `frontend/src/widgets/`, no `kernel/widget*`, no M-reports.
- **Major gaps:** **The entire widget ecosystem is missing**: F-20 (widget SDK + sandbox render), F-21 (widget marketplace), F-22 (widget scaffold).
- **Linked items:** F-20, F-21, F-22
- **Commit hint:** —
- **Note:** The SPEC is designed for Phases 5+7+8; implementation has not begun.

### SPEC-13 Capability Army (Ecosystem Interop)
- **Status:** shipped (infrastructure)
- **Coverage:** ~75%
- **Last M-report:** M916, M848, M847
- **M-report count:** 12
- **Code sites:** `kernel/market` (8+3), `plugins/builtinskills` (16 packages), `kernel/skill` (cross-cutting)
- **Major gaps:** **`agt migrate openclaw|hermes` (F-10)** missing. **A populated hub (F-5)** missing (infrastructure exists). **agentskills.io/ClawHub adapter** shipped (M377 SKILL-MD), but the marketplace UX is missing.
- **Linked items:** F-5, F-10, F-21, F-22
- **Commit hint:** `PHASE-M377-SKILLMD-ADAPTER-REPORT.md`

### SPEC-14 Resilience, HITL, Eval, RBAC, Operational Maturity
- **Status:** partial (weak)
- **Coverage:** ~50%
- **Last M-report:** M552 (E2E), M400 (Skill shadow-eval), M391 (HITL), M367 (Anomaly), M262 (SDK examples)
- **M-report count:** 7
- **Code sites:** Cross-cutting — `kernel/anomaly` (2+2), `kernel/standing` (3+3), `kernel/runtime` (27+53 cross-cutting)
- **Major gaps:** **First-class F-15 Saga/compensation** missing. **F-16 RBAC**, **F-17 user-dimension**, **F-18 escalation chains**, **F-19 external vault**, **F-23 capability eval harness**, **F-24 reflection closure**, **F-25 UI i18n**, **F-26 OpenTelemetry**, **F-27 FinOps**, **F-28 codec auto-rotation** — all in the Phase 6/8 backlog.
- **Linked items:** F-15, F-16, F-17, F-18, F-19, F-23, F-24, F-25, F-26, F-27, F-28
- **Commit hint:** `PHASE-M367-ANOMALY-AUTOHALT-REPORT.md`

### SPEC-15 Provider Ecosystem (Catalog Sync, Tool-Calling, ACP)
- **Status:** shipped (differentiator, above parity)
- **Coverage:** ~95%
- **Last M-report:** M912 (MCP catalog), M897 (MCP catalog), M879 (image tools), M845 (artifact collector)
- **M-report count:** 77
- **Code sites:** `plugins/providers/` (15 families), `plugins/providers/openairesponses` (ChatGPT), `kernel/chatgptauth` (M937/M935 OAuth), `plugins/providers/vertex`, `plugins/providers/bedrock`, `plugins/providers/google`, `plugins/providers/openai`, `plugins/providers/anthropic`, `plugins/providers/cohere`, `plugins/providers/ollama`, `plugins/providers/image`, `plugins/providers/voice`, `plugins/providers/rerank`, `plugins/providers/embed`, `plugins/providers/compat`, `plugins/providers/mock`, `plugins/providers/internal`
- **Major gaps:** **Very few**. ACP server exists (kernel/acp), ACP client (acpagent tool). Ollama auto-discovery shipped.
- **Linked items:** —
- **Commit hint:** `PHASE-M912-MCP-CATALOG-LIBRARY-REPORT.md`

### SPEC-16 Concrete Detail Specifications (API, test, config, DSL, onboarding)
- **Status:** shipped
- **Coverage:** ~85%
- **Last M-report:** M891 (HTTP-API skill), M808 (workflow reliability), M807 (workflow templates)
- **M-report count:** 31
- **Code sites:** Cross-cutting — `kernel/restapi`, `kernel/webui` (9+13), `cmd/agt`, `cmd/agezt`
- **Major gaps:** **Onboarding flow** shipped (Setup wizard); **agent wake-rule DSL** shipped (STANDING type). **API reference generation** is partial — `docs/SDK-PARITY.md` exists.
- **Linked items:** H-03 (SPEC-IMPLEMENTATION-STATUS.md — this file!)
- **Commit hint:** `PHASE-M808-WORKFLOW-RELIABILITY-REPORT.md`

---

## 2. Summary Table

| SPEC | Status | Coverage | M-count | Main gaps |
|---|---|---|---|---|
| SPEC-01 Plugin Contracts | shipped | ~95% | 18 | — |
| SPEC-02 Kernel | shipped | ~95% | 92 | controlplane refactor slice |
| SPEC-03 Pulse | shipped | ~88% | 13 | F-12 anticipatory |
| SPEC-04 Plugins | shipped | ~90% | 44 | F-19 external vault (plugin side) |
| SPEC-05 Memory/World/Skills | shipped | ~85% | 56 | F-4 LLM curator |
| SPEC-06 Security | shipped | ~95% | 52 | F-16 RBAC, F-26 OTel |
| SPEC-07 UI | shipped | ~92% | 72 | F-1/F-2/F-20 |
| SPEC-08 Operability | shipped | ~80% | 15 | H-04 CHANGELOG reorg |
| SPEC-09 Identity | shipped | ~85% | 30 | F-10 migrate, N-4 graveyard |
| SPEC-10 LLM/Context | shipped | ~95% | 72 | — |
| SPEC-11 Deployment | partial | ~70% | 13 | F-14 K8s job |
| **SPEC-12 Widgets** | **design-only** | **~5%** | **0** | F-20/F-21/F-22 (all widgets) |
| SPEC-13 Capability Army | shipped | ~75% | 12 | F-5 populated hub, F-10 |
| SPEC-14 Resilience | partial | ~50% | 7 | F-15–F-28 (10 items) |
| SPEC-15 Providers | shipped | ~95% | 77 | — |
| SPEC-16 Details | shipped | ~85% | 31 | — |

### Total Statistics

- **shipped:** 13 SPECs (1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 13, 15, 16)
- **partial:** 2 SPECs (11, 14)
- **design-only:** 1 SPEC (12)
- **not-started:** 0 SPECs
- **total M-reports on disk:** ~628 SPEC-related (out of 697 total)
- **total Go src files in SPEC areas:** ~187K LOC (verified in SYSTEM-AUDIT)

---

## 3. SPEC ↔ Missing-Parts (F-/N-/H-/D-) Cross Matrix

Distribution of each SPEC across the inventory:

| SPEC | F- | N- | H- | D- | Total |
|---|---|---|---|---|---|
| SPEC-01 | 0 | 0 | 0 | 0 | **0** |
| SPEC-02 | 0 | 1 (N-1) | 0 | 0 | **1** |
| SPEC-03 | 1 (F-12) | 2 (N-2, N-3) | 0 | 0 | **3** |
| SPEC-04 | 1 (F-19) | 0 | 0 | 0 | **1** |
| SPEC-05 | 1 (F-4) | 0 | 0 | 0 | **1** |
| SPEC-06 | 4 (F-16, F-17, F-26, F-28) | 0 | 0 | 0 | **4** |
| SPEC-07 | 3 (F-1, F-2, F-20) | 0 | 0 | 0 | **3** |
| SPEC-08 | 0 | 0 | 1 (H-04) | 0 | **1** |
| SPEC-09 | 1 (F-10) | 1 (N-4) | 0 | 0 | **2** |
| SPEC-10 | 0 | 0 | 0 | 0 | **0** |
| SPEC-11 | 1 (F-14) | 0 | 0 | 0 | **1** |
| **SPEC-12** | **3 (F-20, F-21, F-22)** | 0 | 0 | 0 | **3** |
| SPEC-13 | 3 (F-5, F-10, F-21) | 0 | 0 | 0 | **3** |
| SPEC-14 | 10 (F-15, F-16, F-17, F-18, F-19, F-23, F-24, F-25, F-26, F-27, F-28) | 0 | 0 | 0 | **11** |
| SPEC-15 | 0 | 0 | 0 | 0 | **0** |
| SPEC-16 | 0 | 0 | 1 (H-03) | 1 (D-03) | **2** |

### Most Loaded SPECs (in inventory terms)

1. **SPEC-14 Resilience** — 11 open items (F-15…F-28). Crowded backlog.
2. **SPEC-06 Security** — 4 open items (RBAC, OTEL, encryption rotation, user-dim).
3. **SPEC-03, SPEC-07, SPEC-12, SPEC-13** — 3 open items each.
4. **SPEC-04, SPEC-05, SPEC-09, SPEC-11** — 1-2 open items.
5. **SPEC-01, SPEC-10, SPEC-15** — zero open items (cleanest state).

> **Note:** The same F-/N-/H-/D- item can be linked to more than one SPEC (e.g. F-16 is linked to both SPEC-06 and SPEC-14). In the context of §3, "open item count" means the number of linked SPECs for a single item; for the total unique item count, see the §1 counter table in `MISSING-PARTS-REPORT.md`.

---

## 4. Known Biases

This matrix can be biased:

- **Coverage % estimate**: a simple average of three signals, quite rough. Empirical test durations and dashboard coverage measure more accurately.
- **M-report count** uses title keywords; it can overlap.
- **"shipped"** = all P0/P1 closed; not every section of the SPEC may be met (P3 backlog remains).
- **The M-number on disk** = by file name; the CHANGELOG may have other M-numbers.

---

## 5. Update Policy

- If a new M-report file is added: **M-report count** is incremented on the relevant SPEC row and **Last M-report** is updated.
- As new F-/N-/H-/D- items are added, the §3 cross matrix is updated.
- This matrix must be revised with a one-line record after `make check` or any significant PR.
- Closing a SPEC in "shipped" state **requires two records**: here + `MISSING-PARTS-REPORT.md` §6.

---

## 6. Snapshot (2026-07-04)

| Metric | Value |
|---|---|
| PHASE-M reports on disk | 697 files |
| Highest M-number on disk | M923 |
| Highest M-number referenced in the CHANGELOG | M1002 |
| SPEC × status coverage | 13 shipped + 2 partial + 1 design-only |
| Go src files (production) | 575 (187,195 LOC) |
| Go test files | 751 |
| Go test/source ratio | 1.30 |
| Frontend views/components | 71 + 48 |
| Frontend tests (vitest) | 180 |
| SPEC-12 widget directory/code | 0 |
| Environment hygiene H-* | 5 open + 1 closed (H-02) |

---

*This matrix is living. SPEC-12 (Widgets) is an outlier with zero M-reports — the biggest "not yet started" SPEC. SPEC-14 (Resilience) has the densest inventory linkage.*
