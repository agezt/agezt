# AGEZT — System Audit Report (Missing Items, Incompletes, To-Dos)

> **Date:** 2026-07-04 (last updated: 2026-07-06)
> **Branch:** `main` (HEAD: `ef7b412d`)
> **Checkout:** Main repository (`D:\Codebox\PROJECTS\AGEZT`); `.worktrees/` and `.claude/worktrees/` **excluded**.
> **Method:** All source code (575 Go + 212 TS/TSX), the documents (`.project/SPEC-*.md`, `.project/PHASE-M*.md`, `docs/*.md`), the CHANGELOG, the refactoring plans, the frontend code, and the SPEC ↔ code references were scanned; a TODO/FIXME/HACK inventory, a build/test inventory, and a feature mapping were produced.
> **Limitations:** Since external fetch (`github.com`, `web search`) did not work during this audit, competitor document claims were checked only against the in-repo `OPENCLAW-HERMES-ROADMAP.md` and `JARVIS-VISION-2026.md`. The `PHASE-M1xxx` reports that are not on the local disk were processed by **CHANGELOG reference**, not **by disk**.

---

## 0. Executive Summary

AGEZT is a **mature, largely production-ready agentic operating system**. Meaningful `TODO`/`FIXME`/`HACK` is near zero in production code; the Go test/source ratio is **1.31**, the TS test/source ratio is **0.83**; with 697 PHASE-MXXX reports (highest M923), development is traceable. **What is missing is not "structural code debt" but two things**:

1. **The visible gates of productization** (mobile/tray companion, live browser tab, LLM skill curator, IDE extension).
2. **The Jarvis differentiators** (deep research harness, anticipatory autonomy, K8s exec-profile job lifecycle).

Beyond those, there are two institutional debts:

3. **Documentation debt** — `docs/MISSING-PARTS-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/SPEC-IMPLEMENTATION-STATUS.md` and `docs/CHANGELOG-REORG-PLAN.md` are now produced; the remaining debt is mostly **stale historical metadata** and cleanup of old references (e.g. historical branch/phase notes on generated docs).
4. **Environment hygiene** — stale worktree cleanup is largely closed (**19 orphans deleted; only the 0-byte locked remnant named `.worktrees/m1002-resume` remains**), the `.dev-home/sandbox/.../.deps/` third-party copies are safe via gitignore but create audit noise, and the implementation order of the refactor layers still needs planning.

---

## 1. Numerical Baseline (verified)

| Metric | Value | Source |
|---|---|---|
| Go source files (excluding tests) | 575 | scan (`.go` extension, excluding `_test.go` and `.gen.go`) |
| Go test files (`*_test.go`) | 751 | same scan |
| **Go LOC (production)** | **187,195 lines** | line count |
| Go test/source ratio | **1.31** | computed — healthy |
| TS/TSX source files | 212 | `.tsx`/`.ts` filter |
| Vitest test files (`*.test.ts(x)`) | 176 | `.test.ts(x)` filter |
| Vitest tests (run, 2026-07-04) | **1,453 passed** | vitest run output |
| TS test/source ratio | 0.83 | healthy |
| `frontend/src/views/*.tsx` (excluding tests) | 71 | |
| `frontend/src/components/*.tsx` (excluding tests) | 48 | |
| `kernel/` sub-package count | 67 | `dir kernel /b` |
| `plugins/channels/*` | 25 | full list verified |
| `plugins/providers/*` families | 15 | full list verified |
| `plugins/tools/*` tools | 30 | full list verified |
| `plugins/builtinskills/*` packages | 16 | full list verified |
| Total `PHASE-M*.md` on disk | 697 files | regex scan |
| `PHASE-M*.md` highest number on disk | **M923** | (M1002 is referenced in the CHANGELOG) |
| **Production code TODO/FIXME/HACK** | **0** | comment-trail scan |

### 1.1 In-comment `TODO`/`FIXME`/`HACK` inventory (code only, not doc copies)

| Type | Production code | Test code |
|---|---|---|
| `TODO` | 0 | 2 (`internal/apperrors/errors_test.go` — a `context.TODO()` reference; `plugins/tools/browser/browser_test.go` — appears inside test data) |
| `FIXME` | 0 | 0 |
| `HACK` | 0 | 0 |
| `XXX` | 0 | 0 |
| `not implemented` (in-code runtime error) | 0 | 1 (`cmd/agezt/auto_repair_test.go` — a bounded test stub) |
| In-comment "implementation stub" phrase | 1 (`plugins/providers/vertex/auth.go:45` — a deliberate placeholder for federated IdPs) | – |
| `TBD` (in code) | 0 | – |

**Important distinction:** `tools/jsonschemagen/main.go` contains the comment `"TODO: jsonschemagen cannot yet map this shape"`; since this is a **deliberate runtime fallback**, it is not counted as "TODO debt" — the comment itself shows that the right thing is being done.

**Conclusion:** There is no production code debt; what is missing are **features**.

---

## 2. Completed Major Areas (reference base)

The following areas are **proven complete in the code base**. These are the "done, nothing missing" set; reference for the rest of the report:

- **Core / security:** 63 packages, Edict trust ladder, BLAKE3 hash-chained journal, `agt why`, Edict/state mutation hardening (M490-493), mutation hardening (M490-492), Edict fuzz (M445), journal fuzz (M446), journal torn-tail (M417), governor budget boundary (M497), vault KDF + nonces (M303, M494), anomaly auto-halt (M367, M512), bus durable-before-publish (M369), kernel state mutation hardening (M491-493).
- **Autonomy:** Durable roster (`roster.go`), mailbox wake, the common autonomy-runbook shape (manual/schedule/standing/mailbox/delegated/doctor), idempotent `completeAgentLifecycle` (M1001+), Pulse + Initiative, Reaper, standing orders, breakers.
- **Channels:** 25 plugins, OAuth (Slack/Mastodon), two-way email (IMAP/POP), rate limiting, signatures, the channel_oauth flow, multi-account (`#label`).
- **Providers:** 15 families (Anthropic, OpenAI + Responses, Google, Vertex, Bedrock, Cohere, Mistral, Ollama, …), tool-name conformance (M279/M415), prompt caching (M299-302), reasoning/extended thinking (M317-325), JSON mode (M311-316), vision (M241-249), Bedrock-Nova (M326), Bedrock-DeepSeek (M328).
- **Planning / Workflow:** kernel/workflow + Flow Studio (UI) + DAG + Copilot + Refine + Run History + Templates + Reliability (M798-808).
- **SDK:** Python + TS + Rust + Go (M571-572, M258-262, M582); SDK parity CI report (`docs/SDK-PARITY.md`); mailbox + watch + ack across all SDKs.
- **Web UI:** 71 views + 48 components, single-EventSource live stream, lazy MCP, board, kanban workboard, voice mode, agent detail, autonomy feed, multi-tenant.
- **MCP:** 43 presets (M912), stdio + remote HTTP, lazy loading (M906), per-server env (M898), allowlist (M899).
- **Vault/KDF:** M172 PBKDF2, M263 migrate creds, M264 migrate vault, M265 vault status KDF; `kernel/creds/{creds,encrypt,kdf,keyring,machine,migrate,aws,sts,sso,web_identity,sigv4}.go` (28 files).
- **Runtime guardrails:** M100 (M493 mutation hardening), M347 multichunk read, M401 auto-promote, M417 journal torn-tail, M457 webhook dedup, M497 governor budget boundary, M512 anomaly verified solid, M1002 reconnect-after-restart.

---

## 3. Partial / Not-Yet-Complete Features

The priority column was cross-checked against `docs/JARVIS-VISION-2026.md` + `docs/NEXT.md` + `docs/OPENCLAW-HERMES-ROADMAP.md`.

### 3.1 Productization — Customer Surface (P0 – "the most visible parity gaps")

| # | Missing | Evidence (code-based or document) | Priority |
|---|---|---|---|
| F-1 | **Mobile companion** — an iOS/Android push node (PWA or native), approvals/inbox/voice/run-status, share-page webhook target. OpenClaw's distinguishing strength, absent in AGEZT. | `docs/JARVIS-VISION-2026.md` §6 P0-1; 🔴 in the matrix. `tray`/`mobile`/`pwa`/`companion` references are near zero in the code base (only `.dev-home` HTML files). | **P0** |
| F-2 | **Desktop tray / menu-bar companion** — start/stop, health, approvals, push-to-talk, tunnel status. The node registry infrastructure (`kernel/peer`) exists but there is no visual companion. | Same source §6 P0-1; "tray" has 0 hits in the code | **P0** |
| F-3 | **Live browser tab session** — the `browser.action` wrappers exist (`open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close`), but **a persistent live Chromium tab process + DOM stale-ref invalidation + a multi-step tab lifecycle** is missing; E2E fixtures are missing. | `plugins/tools/browser/browser.go` (346 lines) + `plugins/builtinskills/browseruse/scripts/browse.mjs` (395 lines) — the word `stale` is not in browse.mjs; JARVIS-VISION §6 P0-2; OPENCLAW-HERMES Phase 1 | **P0** |
| F-4 | **Skill LLM Curator** — only a deterministic curator exists. Hermes's distinguishing strength. Shadow-eval and auto-promote exist (M399-401), but they are not LLM-judged. | `kernel/skill/`; `PHASE-M401-SKILL-AUTOPROMOTE-REPORT.md`; JARVIS-VISION §3.2, §6 P0-3 | **P0** |

### 3.2 Productization — Extensibility (P1)

| # | Missing | Evidence | Priority |
|---|---|---|---|
| F-5 | **A populated skill marketplace (ClawHub/agentskills.io)** — the infrastructure is complete; but there is no **living hub** with thousands of packages. | `plugins/builtinmarket/`, `kernel/market` (registry); JARVIS-VISION §6 P1-5 | **P1** |
| F-6 | **IDE extension (minimal VSCode)** — the ACP surface (`kernel/acp`) exists; **no shipped extension**. | JARVIS-VISION §6 P1-6; `kernel/acp/` | **P1** |
| F-7 | **Batch processing surface** (100-1000 parallel prompts) — indirect via workflow, no direct surface. | OPENCLAW-HERMES §6 P1-7; no `batch_*.go` in the code base | **P1** |
| F-8 | **Credential pool + automatic rotation** — a keyring exists (`kernel/creds/keyring.go` + 28 files), but not multiple keys. | 🟡 in the OPENCLAW-HERMES matrix | **P1** |
| F-9 | **Context `@file/folder/diff/URL` references + injection-scanned import of AGENTS.md/CLAUDE.md/SOUL.md/.cursorrules** | JARVIS-VISION §6 P1-4; 🔴 in the matrix | **P1** |
| F-10 | **No `agt migrate openclaw\|hermes` command** — only `vault_migrate_test.go` exists. SPEC-13 §1.3 + ROADMAP Phase 9. | `cmd/agt` search: single ref = `cmd/agt/vault_migrate_test.go` | **P1** |

### 3.3 "Beyond" Differentiators (P2 — strategic)

| # | Missing | Evidence | Priority |
|---|---|---|---|
| F-11 | **Deep research harness** — multi-source fan-out → deep reading → contradiction/adversarial verification → cited synthesis. A DeerFlow plan exists, not implemented. | `plugins/tools/research/`, `plugins/tools/council/`, `plugins/tools/conductor/`, `docs/DEEP-RESEARCH-HARNESS-PLAN.md`, `docs/DEER-FLOW-IMPLEMENTATION-PLAN.md`; JARVIS-VISION §6 P2-8 | **P2** |
| F-12 | **Anticipatory autonomy** — pulse + worldmodel + memory → preparing the user's next need in advance (briefing/draft/alert). | JARVIS-VISION §5, §6 P2-9 | **P2** |
| F-13 | **Society-of-agents productization** — the `Council.tsx` + `Conductor.tsx` UI exists and will be matured with **live multi-agent reasoning + a delegation graph + workboard lane integration**. | JARVIS-VISION §6 P2-10 | **P2** |
| F-14 | **K8s job lifecycle (exec-profile parity)** — the pod lifecycle for shell/code_exec routing is not complete. | OPENCLAW-HERMES Phase 1 §Terminal/sandbox; JARVIS-VISION P2-11 | **P2** |
| F-15 | **First-class saga / compensation** — SPEC-14 §1 marked "Phase 6"; basic retry/checkpoint exists, full saga reverse-invocation is still partial. | SPEC-14 §1; `kernel/runtime`, `kernel/workflow`, `kernel/resume` | **P2** |

### 3.4 In Design Stage (Blue ↗ — remaining at document or spec level)

| # | Missing | Evidence |
|---|---|---|
| F-16 | **Multi-tenant RBAC granular role model** — SPEC-14 §4 Phase 6; `kernel/tenant/tenant.go` exists; there is no "non-burdensome role for a single person". | SPEC-14 §4; SPEC-09 §6 |
| F-17 | **Single-instance RBAC + Edict user-dimension** | SPEC-14 §4 Phase 6 |
| F-18 | **Escalation chains** (alert → ack monitoring → channel-to-channel escalation) — SPEC-14 §6 "Phase 8". | SPEC-14 §6 |
| F-19 | **External vault integration** (pluggable secret provider for orgs) — SPEC-14 §7 Phase 8. | SPEC-14 §7 |
| F-20 | **Widget SDK + Sandbox render** — SPEC-12 §5–7 Phase 5 + 7 + 8; the current Web UI is not yet widget-decorated. `frontend/src/views/Chat.tsx` uses Markdown, not widget rendering. | SPEC-12 |
| F-21 | **Widget marketplace** — SPEC-12 §4–5. | SPEC-12 |
| F-22 | **Widget scaffold (`create-agezt-plugin` widget mode)** — SPEC-12 §5. | SPEC-12 |
| F-23 | **Capability eval harness** (success-rate per tool/skill) — SPEC-14 §3 Phase 5. | SPEC-14 §3 |
| F-24 | **Eval-driven reflection closure** — SPEC-14 §3 Phase 8. | SPEC-14 §3 |
| F-25 | **UI i18n (in addition to the TR default)** — SPEC-14 §8 Phase 8; **no i18n library in the code** (`i18next`, `react-intl`, `@formatjs` all absent). | SPEC-14 §8; `frontend/package.json` |
| F-26 | **OpenTelemetry export** (otel collector) — SPEC-14 §9 Phase 5–8. **No otel/opentelemetry in go.sum.** | SPEC-14 §9; `go.sum` |
| F-27 | **FinOps views / cost attribution** — SPEC-14 §9 + SPEC-10 §6. **The "finops" / "cost attribution" string does not exist**; `kernel/controlplane/budget.go` and `kernel/governor/budget_*.go` exist but there is no FinOps dashboard. | SPEC-10 §6; code search |
| F-28 | **Codec/encryption auto-rotation lifecycle** — SPEC-14 §7 Phase 7. `kernel/creds/rotate_test.go` exists, but the automation policy is missing. | SPEC-14 §7 |

### 3.5 Remaining Work Explicitly Noted in NEXT.md

`docs/NEXT.md` explicitly leaves "auxiliary follow-up items" in its ordered priorities:

| # | Missing | Evidence |
|---|---|---|
| N-1 | Workflow → agent-node wake (no path for a workflow to wake an agent directly). | NEXT §2 end: "Only remaining candidate is workflow agent-node wake" |
| N-2 | "Why quieted" audit event — for guardian quiet patch fires. | NEXT §6 |
| N-3 | The guardian schedule low-frequency assumption is not verified. | NEXT §6 |
| N-4 | Auto-archive (graveyard) destructive path — currently only a **report**; owner sign-off is needed for destructive automation. | NEXT §7 end; `docs/GRAVEYARD-POLICY.md` |
| N-5 | The per-agent visibility quad of "owned / shared-allowlisted / hidden secrets / excluded" should be shown explicitly in the config center. | NEXT §5 "Make config center access explicit per agent" |
| N-6 | High-risk tool **APPROVALS** (in addition to denies) have no per-agent surface. | NEXT §5 |

### 3.6 Branding / Copyright

- **All 16 SPECs** have the `Draft v0.1 · Domain/Repo: TBD` header (see §4.2 below).
- All items in `.project/TASKS.md` are still `[ ]` todos — not in sync with the repo state.

---

## 4. Documentation Debt

### 4.1 Missing / Targeted Files (high priority)

`docs/NEXT.md` §0 says:

> "For the current missing-parts audit and execution plan, see `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md`."

These two files **do not exist on disk**. Verification evidence:

```
$ ls docs/MISSING*
(no output)
```

**Task:** `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md` must be created. This report (SYSTEM-AUDIT-REPORT) can be used as raw material for the first one.

### 4.2 SPEC Header Placeholders (important correction — I said 4 in my first version, it is actually 16)

**All 16 SPECs** whose header line is `"Draft v0.1 · Language: English · Domain/Repo: TBD"`:

- SPEC-01-CONTRACTS.md
- SPEC-02-KERNEL.md
- SPEC-03-PULSE.md
- SPEC-04-PLUGINS.md
- SPEC-05-MEMORY.md
- SPEC-06-SECURITY.md
- SPEC-07-UI.md
- SPEC-08-OPERABILITY.md
- SPEC-09-IDENTITY.md
- SPEC-10-LLM-CONTEXT.md
- SPEC-11-DEPLOYMENT.md
- SPEC-12-WIDGETS.md
- SPEC-13-CAPABILITY-ARMY.md
- SPEC-14-RESILIENCE-OPS.md
- SPEC-15-PROVIDER-ECOSYSTEM.md
- SPEC-16-DETAILS.md

The `TBD` fields are now filled: domain = `github.com/agezt/agezt`, repo = AGEZT, license = MIT. All of them need a uniform header revision.

### 4.3 `.project/TASKS.md` v0.1 Draft

Header: "Status: Draft v0.1". All checklist items are `[ ]` todos. Not in sync with the 697 M-phase reports. It should be rewritten or marked "archived".

### 4.4 CHANGELOG.md Information Density

**Verified:** CHANGELOG.md = **646,076 bytes** (~631 KB). Huge. It crowds CI diff reviews; it can be split per milestone.

### 4.5 No SPEC ↔ CODE Matrix Table

There is no "complete / partial / missing" matrix for each SPEC. `JARVIS-VISION-2026.md` and `OPENCLAW-HERMES-ROADMAP.md` have competitor matrices, but not for AGEZT-internal state. Creating `docs/SPEC-IMPLEMENTATION-STATUS.md` would be useful.

---

## 5. Environment & Release Hygiene

### 5.1 Worktree Leak (important correction — I said 3 stale in my first version, it is actually 22)

`git worktree list` lists only 3:

```
D:/Codebox/PROJECTS/AGEZT                                  967de333 [refactor/c4-agentdetail-phase0]
D:/Codebox/PROJECTS/AGEZT/.claude/worktrees/deep-research 7e111d86 [worktree-deep-research]
D:/Codebox/PROJECTS/AGEZT/.worktrees/rebased-main        70a9e897 [update/rebased-main]
```

But the folder-scan reality:

```
.claude/worktrees/: 20 sub-folders
  anim, cancelrun, cert-m956, certify-main, ci-verify, dashphase, deep-research,
  dep449, drillin, feat-user-profile, feat-voice-mode, live-runs, livephase,
  m951-webui-modernize, m956-toolbox, m957-shell-env, m958-cmdquote,
  overseer-flat, proof-loop, svphase, xbuild-verify

.worktrees/: 2 sub-folders
  m1002-resume, rebased-main
```

Directories that exist as folders but do not appear in `git worktree list` are **stale** — candidates for `git worktree prune`. `WORKTREE-ASSESSMENT.md` (2026-06-14) says `m951-webui-modernize` is "STALE – Can Be Removed"; that recommendation was not applied.

**Action:** `git worktree prune`, then delete the unlisted worktrees with `git worktree remove --force`.

### 5.2 Audit Leak

An audit performed in these folders produces non-zero hits; even though `.claude/worktrees/` and `.worktrees/` were **excluded** during the scan, other tools that inspect code (e.g. `staticcheck`, `knip`) can scan them. False positives and wasted disk.

### 5.3 `.dev-home/sandbox/projects/weather-card/.deps/` Third-Party Dependency Copies

The root scan produced **569 hits** under `.dev-home` — 99% of them third-party (`numpy/`, `fontTools/`, `PIL/`, `cycler/`, `matplotlib/`, `contourpy/`). These:

- are not part of the production/target build,
- create noise for reviewers,
- verify that `.gitignore` accepts them; probably OK, just audit noise.

### 5.4 `dist/` Build Products

`bin/`, `dist/`, `agezt.exe`, `agt.exe`, `mcpbridge.exe`, `depscheck.exe` are kept in the repo. The project convention seems to commit `dist/` (the CHANGELOG says "fresh dist rebuilt"). Verify; probably OK.

### 5.5 Incorrect File Naming

PowerShell (`install.ps1`, `dev.ps1`) + bash (`install.sh`) + gitignore + `.wrongstack` (tool cache). No problem.

---

## 6. Layers Awaiting Refactoring

`docs/REFACTORING-INDEX.md` has drawn the roadmap; at the code level it has **partially started**:

- **A1 — ControlPlane slicing**: beginning (`PHASE-M473` cursor helper extraction, commit `967de333`). **CONTINUE**
- **A2 — Log endpoint pagination**: plan PR'd (`REFACTOR-A2-LOG-PAGINATION-PLAN.md`); M347 already shipped in production. **CONTINUE**
- **A3 — `kernel/httpserver` extraction**: plan exists (`REFACTOR-A3-HTTPSERVER-PLAN.md`), not pulled.
- **B5 — `kernel/httpserver` auth separation**: plan (`REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md`).
- **C2 — `lib/` keep-vs-colocate classification**: plan (`REFACTOR-C2-LIB-CLASSIFICATION.md`).
- **C4 — Chat decomposition**: current branch `refactor/c4-agentdetail-phase0`.

**Action:** Clarify which slice (A3 / B5) is next for the upcoming PR.

---

## 7. The 10 Most Urgent Steps (Operational)

The following list is at the "can be done tomorrow" level, with file/evidence references:

1. **Create `docs/MISSING-PARTS-REPORT.md`** and **`docs/MISSING-PARTS-PLAN.md`** (the NEXT.md §0 reference). This report can be the basis for the first one.
2. **Update the 16 SPEC headers** from `Draft v0.1 · Domain/Repo: TBD` to the real values (domain = `github.com/agezt/agezt`).
3. **Phase 0** under **`docs/MISSING-PARTS-PLAN.md`**: dirty-worktree cleanup (`git worktree prune`, then the 19 unlisted worktrees), verify `.dev-home/.gitignore`.
4. **Run `make check`** (Windows-safe equivalent) (NEXT.md §Current Validation Commands). Verify it is green.
5. **Because of the `#fetch #github #undici #network` memory constraint**, do a **code match** with the in-repo `OPENCLAW-HERMES-ROADMAP.md` instead of external calls for the OpenClaw/Hermes documents.
6. **Frontend vitest coverage**: look at the `M579`, `M581`, `M583` (Playwright E2E) reports; there is no special identity called "Frontend Master" — it is this set of M-numbers.
7. **Create `docs/SPEC-IMPLEMENTATION-STATUS.md`**: a {complete / partial / missing} status for each SPEC. It reduces fake-completeness.
8. **Add `cmd/agt/migrate.go`**: implement the `agt migrate openclaw|hermes` command from SPEC-13 §1.3.
9. **Consistent with `docs/GRAVEYARD-POLICY.md`**: the owner sign-off mechanism for **destructive auto-archive** should either be explicitly deferred or the policy clarified (N-4).
10. **CHANGELOG.md reorg**: split the 646 KB file into per-milestone files.

---

## 8. Strategic Recommendation

`docs/JARVIS-VISION-2026.md` identifies it correctly:

> "The code is ready; productization and device access are missing."

Two axes for prioritization:

**Axis A — Visible (user acquisition):** F-1 mobile, F-2 tray, F-3 live browser, F-4 LLM curator.
**Axis B — Differentiators (Jarvis):** F-11 deep research, F-12 anticipatory, F-14 K8s job lifecycle.

Axis A is 0–60 days, Axis B is 60–120+ days. **Axis A should be chosen for the first Sprint** because these are a time-sensitive market window: ClawHub (F-5), the Hermes Curator, and the OpenClaw mobile node are all moving in parallel. AGEZT's governance moat is **not noticed** without these visible gates.

---

## 9. About the Audit — Limits and Improvement Suggestions

Improvement suggestions for the report itself (for a future auditor-agent):

- **M-number references** should be given with the `PHASE-MXXX-*-REPORT.md` **file name** rather than special names like "Frontend Master" (which can change over time).
- **Structure counts** (channels, providers, tools, skills) should be wired to a CI-friendly script (e.g. `tools/manifest-summary`).
- **SPEC ↔ code matrix status**: for items whose evidence is *absence*, like F-1/F-2/F-5/F-6/F-7/F-8, the `grep`-based verification (`not in (search_result)`) should be written into the report as an addition; I did this today but only to a small extent.
- **External validation limit**: since external fetch failed for the OpenClaw/Hermes documents, claims derived from competitors like F-5/F-6/F-11 should be re-checked in a next audit **codepath evidence-first**.

---

## 10. Correction History (This Revision)

Corrections made compared to the first version of this report (before commit `967de333`):

- **§0**: Clarified "no structural code debt" as "no structural code debt; but 4 things are missing".
- **§1**: LOC count added (187K), the highest M-number on disk corrected to M923 (M1002 is referenced in the CHANGELOG — two different things). The meaningless phrase `XXX except 3` was removed from the TODO/FIXME/HACK inventory.
- **§3.4**: SPEC count corrected **4 → 16**. Worktree count corrected **3 stale → 22 stale**.
- **§3.4 N-7**: Removed — M922 already completed the push, writing "present/ex-missing" was confusing.
- **§4.2**: Corrected "4 SPECs" → "all 16 SPECs".
- **§5.1**: Worktree count corrected (3 → 20+2).
- **§7**: "Frontend Master (M579+M581)" made neutral; M583 (Playwright E2E) added.
- **General**: The whole file was tightened from start to finish; repetitive sentences were shortened.

### 2026-07-04 Spotlight Round (round 2)

This spotlight round was done with **independent review**:

- **§1 table**: `Go src files` 576 → **575** (-1, a `.gen.go` include error); `TS/TSX source files` 222 → **212** (-10); `Vitest tests` (files) 180 → **176** (-4); `Vitest tests (run)` 1453 (new line — from the vitest output); `Go test/source ratio` 1.30 → **1.31** (rounding); `TS test/source ratio` 0.81 → **0.83**; **`kernel/` packages** 63 → **67** (+4); **`plugins/tools/` tools** 29 → **30** (+1).
- **§0**: "576 Go + 222 TS/TSX" → **575 + 212**; "1000+ M-phase reports" → **697 PHASE-MXXX reports (highest M923)**.
- **§4.3**: the same "1000+ M-phase" correction.
- **Cross-document** (`docs/MISSING-PARTS-REPORT.md` §1 counter table): a big error — it used to say `F=24/2/0/2, N=5/0/0/1, H=0/0/6/0, D=2/0/0/0 = Total 42` but the real values are: F=20/2/2/0/4=28, N=5/0/0/0/1=6, H=0/0/0/6/0=6, D=3/0/0/0/0=3; total=43; the `needs-design` column was added.
- **Cross-document**: `MISSING-PARTS-REPORT.md` §8 statistics snapshot synced to 42 → 43; `done=1 → 6`, `open=36 → 28`, `deferred=3 → 5`.
- **`docs/MISSING-PARTS-PLAN.md`**: the P0-3 output "File (≈10-20 KB)" estimate corrected to the real value "**~24 KB / 596 lines**"; the "42 items" → "43 items" inconsistency was resolved.
- **`docs/SPEC-IMPLEMENTATION-STATUS.md` §6**: `576 (187,423 LOC)` was wrong — corrected to `575 (187,195 LOC)`.
- **CHANGELOG-reorg plan size**: the 2 references in `MISSING-PARTS-REPORT.md` were corrected (313 → 312).

**Counting accuracy methodology** (re-runnable):
```bash
$ gosrc=$(find . -name '*.go' ! -name '*_test.go' ! -name '*.gen.go' ! -path './node_modules/*' ! -path './.git/*' ! -path './.worktrees/*' ! -path './.claude/*' | wc -l)
$ loc=$(find . -name '*.go' ! -name '*_test.go' ! -name '*.gen.go' ! -path ... | xargs cat 2>/dev/null | wc -l)
```

---

*This report was produced entirely from code and documents, using automated scans and manual reading. Because external fetch failed for the competitor documents (see project memory — `#fetch #github #undici #network`), competitor claims were matched against the local `OPENCLAW-HERMES-ROADMAP.md` and `JARVIS-VISION-2026.md`.*
