# AGEZT — Missing Parts Plan (Action Plan for Missing Pieces)

> **Date:** 2026-07-04 (last updated: 2026-07-06)
> **Branch:** `main` (HEAD: `ef7b412d`)
> **Status:** ARCHIVED — Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Content retained for historical reference; see `docs/SYSTEM-AUDIT-REPORT.md` for current audit state.
> **Other references:** `docs/OPENCLAW-HERMES-ROADMAP.md`, `docs/JARVIS-VISION-2026.md`, `docs/REFACTORING-INDEX.md`, `docs/GRAVEYARD-POLICY.md`, `docs/PLUGIN-SECURITY.md`, `docs/OPERATIONS.md`, `docs/COMPARISON.md`, `docs/THREAT-MODEL.md`.

---

## 0. Context

`docs/NEXT.md` §0 says:

> "Do not declare the project complete. Continue making concrete progress."
>
> "For the current missing-parts audit and execution plan, see `docs/MISSING-PARTS-REPORT.md` and `docs/MISSING-PARTS-PLAN.md`."

This file is the **execution plan**. The trio is now: `SYSTEM-AUDIT-REPORT.md` (audit), `MISSING-PARTS-REPORT.md` (raw inventory), `MISSING-PARTS-PLAN.md` (this file). The reference trio for a future agent:

- **`docs/SYSTEM-AUDIT-REPORT.md`** — auditor summary + numerical baseline + fix history.
- **`docs/MISSING-PARTS-REPORT.md`** — item-level raw inventory (F/N/H/D scheme, state machine).
- **`docs/MISSING-PARTS-PLAN.md`** — this file; prioritization, ownership, demo gates.

**The `NEXT.md` §0 reference is now complete:** both `MISSING-PARTS-REPORT.md` and `MISSING-PARTS-PLAN.md` exist on disk.

### 0.1 Usage Rule

- This document must be **living**: whenever a Phase is passed, a PR should be opened to update its status and any completed or replanned items.
- When an item becomes **CLOSED**, it must be pinned with a "→ CLOSED commit-hash" note.
- If a new item is added here, it must arrive **via PR** and must not land directly on main (in a multi-agent environment).

### 0.2 Limitations

- External fetch fails (project memory `#fetch #github #undici #network`). This plan is built on in-repo evidence + `OPENCLAW-HERMES-ROADMAP`/`JARVIS-VISION`.
- "Owner sign-off required" notes: make sure they do not conflict with the related documents (e.g. `GRAVEYARD-POLICY.md`).

---

## 1. Ownership and Roles

| Role | Responsibility |
|---|---|
| **Plan owner** | The next lead agent — keeps this document alive. |
| **Phase 0 hygiene** | First 5 days; low risk, no feature changes. |
| **Slice owners** | Each refactor/PR is taken on by a separate person/agent; closed out in this document after commit. |
| **Jarvis Axis-B** | A separate effort is started for F-11…F-14; triggered after P0 is taken. |

---

## 2. Phase 0 — Hygiene & Doc Reorg (During the week)

**Goal:** Move disk and documentation onto cleaner ground; reduce multi-agent errors; build the SPEC ↔ code matrix.

### 2.1 Tasks

| ID | Task | Scope | Output | Duration |
|---|---|---|---|---|
| P0-1 | Worktree prune | `git worktree prune`, then delete the 19 worktrees not shown in `git worktree list` with `--force` | `.claude/worktrees/` 20 → 1 (deep-research); `.worktrees/` 2 → 1 (rebased-main) | 0.5 d | **CLOSED 2026-07-04** (destructive approval obtained) — `git worktree prune -v` + 19 orphans deleted (16 empty + 3 populated: `anim` 10 MB, `m951-webui-modernize` 161 MB, `ci-verify` 187 MB → 358 MB freed in total). `m1002-resume` (0 bytes) is under a Windows process lock — can be cleaned after a reboot/lock release (harmless). |
| P0-2 | Update 16 SPEC headers | Change the first 4-7 lines of `.project/SPEC-{01..16}-*.md` from `Draft v0.1 · Domain/Repo: TBD` to `Active · Domain: github.com/agezt/agezt · License: MIT` | Sed/script or manual PR | 0.5 d | **CLOSED 2026-07-04** (TBD→canonical; language: "Language: English" added except for SPEC-09) |
| P0-3 | Create `MISSING-PARTS-REPORT.md` | Canonicalize the raw inventory from Section 3 of `SYSTEM-AUDIT-REPORT.md` | File (~24 KB / 596 lines) | 1 d | **CLOSED 2026-07-04** (43 items: 28 F + 6 N + 6 H + 3 D; §6 status log + §7 cross-links + §8 statistics). |
| P0-4 | Create `SPEC-IMPLEMENTATION-STATUS.md` | A 16 SPEC × {complete/partial/missing} matrix | Markdown table | 1 d | **CLOSED 2026-07-04** (13 shipped + 2 partial + 1 design-only; SPEC-12 widget is an outlier with 0 M-reports). |
| P0-5 | CHANGELOG.md reorg (planning) | Split the 646 KB CHANGELOG per milestone; "Unreleased" + Phase M1300s | Plan + the first split file | 1 d (plan), then incremental | **CLOSED 2026-07-04 (PLAN)** — `docs/CHANGELOG-REORG-PLAN.md` (12.4 KB / 312 lines) created. Target: slicing into M-ranges of 100, with tools/changelog-split + tools/changelog-lint tooling. **Implementation steps** (4 PRs, 2-4 d): (PR-1) `tools/changelog-split`, (PR-2) `tools/changelog-lint`, (PR-3) reorg, (PR-4) ops migration helper. Main CHANGELOG.md → 50 KB target. |
| P0-6 | Verify the CI gate | Is `make check` (Windows-safe equivalent) green? | `make check` output in the PR | 0.5 d | **CLOSED 2026-07-04** — `jsonschemagen`, `go vet ./...`, `depscheck` (24 deps OK), `sdkparity -check` (after regen), `npm test` (1453/1453 passed, 176 files), `npm run typecheck`, `npm run build` (390 ms, 2167 modules), `go test -count=1 -p=1 -short ./...` (all packages green), `tools/deadcodecheck` (**OK: no unexpected dead code**), `staticcheck ./...` (**clean**). NOTE: The first parallel `go test ./...` produced a socket-buffer error on Windows (`TestRunsList_RowCarriesAnswerPreview`); it was fixed with `-p=1` — expected behavior on Windows (NEXT.md §Current Validation Commands). |
| P0-7 | Verify `.dev-home/.gitignore` | Is `sandbox/projects/weather-card/.deps/` git-ignored? | Output + fix PR | 0.5 d | **CLOSED 2026-07-04** — With the `.dev-home/` pattern at line 101 of the root `.gitignore`, all runtime state (config.json, creds.json, agentgw.secret, journal, datalake, sandbox `.deps/`, etc.) is already covered. |

### 2.2 Demo Gate (between Phase 0 → Phase 1)

- `git worktree list` output shows **only 1 + 1** parallel to the main directory (plumbing).
- The 16 SPEC headers are in canonical form.
- The 3 main document trio (`REPORT`/`AUDIT`/`PLAN`) is PR-ready.
- The CHANGELOG reorg plan is produced; awaiting separate owner approval / implementation PR.

### 2.3 Ownership

| ID | Owner | Note |
|---|---|---|
| P0-1 | ops-hygiene | `git worktree prune` + (19) `git worktree remove --force`. Open a PR and clean `WORKTREE-ASSESSMENT.md` of its "stale" list. |
| P0-2 | docs-curator | Update the 16 file headers; verify SPEC-09 and SPEC-11 in particular (V0.1 TBD). |
| P0-3 | the agent in this session | `MISSING-PARTS-REPORT.md` can be produced immediately; copy-paste SYSTEM-AUDIT-REPORT §3. |
| P0-4 | docs-curator | New matrix; add cross-links to the SPECs. |
| P0-5 | release-mgr | For M1300+, first pin down the "Unreleased" block. |
| P0-6 | every agent | Automatic gate in the PR. |
| P0-7 | ops-hygiene | `.gitignore` test: `git check-ignore -v .dev-home/sandbox/projects/weather-card/.deps/numpy/__init__.py` |

---

## 3. Phase 1 — Slices and Refactors (Next 1-2 sprints)

**Goal:** Clean up the current branch (`refactor/c4-agentdetail-phase0`); pick the next refactor slice; close the NEXT follow-up items.

### 3.1 Slices (board)

| ID | Refactor / Item | Source plan | Output | Duration |
|---|---|---|---|---|
| P1-A | **C4 — Chat decomposition** | `docs/REFACTOR-C4-CHAT-DECOMPOSITION.md` (current branch) | PR mergeable | ongoing | **IN-PROGRESS 2026-07-04** — The P0 barrel shim is complete: `frontend/src/views/Chat.tsx` → barrel, real content in `frontend/src/views/Chat/Chat.tsx`, `frontend/src/views/Chat/index.tsx` added. Mechanical slices were then extracted: `frontend/src/views/Chat/message.tsx` (message group) + `frontend/src/views/Chat/context.tsx` (`barTone`, `ContextChip`, `ContextModal`, `CompactionNote`) + `frontend/src/views/Chat/pickers.tsx` (`ExecutionProfilePicker`, `ConversationPersona`, `PromptLauncher`, `FallbackNote`, `SummaryDivider`, `SteerNote`, `TurnMeta`) + `frontend/src/views/Chat/conversation.tsx` (`ConversationItem`, `QueuePanel`, `EmptyState`, `lastAssistantTools`). **P5 has also been started:** `frontend/src/views/Chat/useChatSession.ts` was added; sub-hook extraction then progressed: `frontend/src/views/Chat/useComposer.ts`, `frontend/src/views/Chat/useVoice.ts`, `frontend/src/views/Chat/useContextWindow.ts`, `frontend/src/views/Chat/useConversationRouting.ts`, `frontend/src/views/Chat/useSteering.ts`, `frontend/src/views/Chat/useConversationControls.ts` added. `Chat/Chat.tsx` now destructures most of the local UI state/effect/handler sets from these hooks. The root file preserves the test surface by importing/re-exporting all of these modules. Gate: `npm run typecheck`, 11 Chat tests, the **full `npm test` frontend suite**, and `npm run build` are green. **The planned sub-slices of P5 are complete.** Next step: `C4-clean` (leftover imports/comments) or, if needed, more micro-specific hooks such as `useExecutionProfile` / `usePersona`. |
| P1-B | **A3 — kernel/httpserver extraction** | `docs/REFACTOR-A3-HTTPSERVER-PLAN.md` | New package, backward-compatible import | 3-5 d |
| P1-C | **B5 — auth split** | `docs/REFACTOR-A3-B5-AUTH-HTTPSERVER-PLAN.md` | Auth middleware separation | 3-5 d |
| P1-D | **C2 — lib/ keep-vs-colocate** | `docs/REFACTOR-C2-LIB-CLASSIFICATION.md` | Classification rule, PR | 2 d |
| P1-E | **N-1 workflow→agent wake** | NEXT §2 end | New wake subject wiring | 1-2 d |
| P1-F | **N-5 config center per-agent** | NEXT §5 | `kernel/controlplane/configcenter_handler.go` UI extension + tests | 2 d |

### 3.2 Priority and Dependencies

```
P1-A (ongoing) → P1-D (classification) → P1-B (httpserver) → P1-C (auth split)
P1-E, P1-F (NEXT follow-up items) → can be done in parallel with any slice
P1-G (verification gate) — `make check` after every slice
```

### 3.3 Demo Gate

- C4 PR mergeable (with multiple sub-PRs if needed).
- A3 PR mergeable, B5 PR mergeable.
- N-1, N-5 closed.
- `make check` green on every slice.

### 3.4 Ownership

- **Slice owners** are separate agents/PRs; every PR carries a "phase" label.
- For P1-E, examine the wake-subject wiring between `kernel/controlplane/standing.go` and `kernel/workflow`; create a "workflow_fired" analogous to the runbook builder in `standing_fired`.
- For P1-F, a four-way explicit view in the Diagnostics tab of `frontend/src/components/AgentDetail.tsx`.

---

## 4. Phase 2 — Visible Gates (Axis A, 0-60 days)

**Goal:** Match OpenClaw's mobile/tray advantage, Hermes's LLM curator, and the live browser tab. The market window is narrowing.

### 4.1 P0 Priority Slices

| ID | Missing item | Sprint / duration | Output | Ownership |
|---|---|---|---|---|
| P2-A | **F-3 Live browser tab** | 8-10 d | Promote `browser.action` to a persistent Chromium tab process; DOM stale-ref invalidation; E2E fixtures | browser-tool owner + browseruse skill owner |
| P2-B | **F-4 LLM Skill Curator** (shadow mode) | 6-8 d | `kernel/skill/curator_llm.go`: an LLM job that **proposes** patches/consolidation based on usage metrics; never deletes | skill kernel owner |
| P2-C | **F-1 Mobile companion (PWA)** | 5-7 d | PWA from the Web UI; push notification opt-in; share-target; approvals/inbox/run-status | webui owner |
| P2-D | **F-2 Desktop tray companion** | 5-7 d | Small Go binary; connects to the daemon from the node registry; approvals, tunnel, HALT button | cli owner |

### 4.2 P1 Slices (parallel)

| ID | Missing item | Sprint / duration | Output | Ownership |
|---|---|---|---|---|
| P2-E | **F-6 VSCode extension (minimal)** | 8-10 d | A package publishable to the VSCode marketplace; connects over ACP | acp owner |
| P2-F | **F-9 Context `@` references + injection-scanned import of AGENTS.md/CLAUDE.md/SOUL.md** | 6-8 d | `chat_summarize`/`@mention` parser; secure loader (injection scanning) | chat owner |

### 4.3 Demo Gate

- **F-3**: E2E test: open page → inspect → click → type → wait → screenshot (with persistent tab session); via the M-phase (`PHASE-M???-BROWSER-LIVE-TAB-REPORT.md`).
- **F-4**: Shadow curator proposal → user approval → `skill.shadow_eval` → active; a full round trip on one skill.
- **F-1**: PWA installable in Android Chrome; share-target works.
- **F-2**: macOS menu bar + Windows tray binary work.
- **F-6**: VSCode `Agezt` view; chat & run drill.
- **F-9**: `@file.txt` in chat → file content injected into context; definitions loaded from AGENTS.md.

### 4.4 Ownership

- F-3 → `plugins/tools/browser/` + `plugins/builtinskills/browseruse/` (two packages; with the interface via a new `BrowserTool.Driver`).
- F-4 → `kernel/skill/curator*`; `cmd/agt` or a standalone CLI for the LLM judge.
- F-1 → `frontend/` PWA manifest; add a Service Worker.
- F-2 → New `cmd/tray/`.
- F-6 → New `ide-plugins/vscode/` (multi-repo).
- F-9 → `frontend/src/views/Chat.tsx` parser + `kernel/runtime/context_budget` loader.

---

## 5. Phase 3 — Jarvis Differentiators (Axis B, 60-120+ days)

**Goal:** Surpass the competitors — deep research, anticipatory autonomy, society-of-agents.

### 5.1 Slices

| ID | Missing item | Sprint | Output | Ownership |
|---|---|---|---|---|
| P3-A | **F-11 Deep research harness** | 12-15 d | `plugins/tools/research/` + `plugins/tools/council/` + `plugins/tools/conductor/` combined: multi-source fan-out → deep reading → adversarial verification → cited synthesis | research-tool owner |
| P3-B | **F-14 K8s job lifecycle** | 8-10 d | `kernel/runtime/exec_profile/k8s.go`; pod lifecycle, exit handling, artifact fetch | exec-profile owner |
| P3-C | **F-12 Anticipatory autonomy** | 10-12 d | A Pulse observer; derives a "ready draft" from worldmodel; publishes a proposal subject | pulse owner |
| P3-D | **F-13 Society-of-agents prod** | 12-15 d | Council.tsx + Conductor.tsx live multi-agent reasoning + workboard lanes + delegation graph | workflow owner + frontend owner |

### 5.2 Demo Gate

- **F-11**: A research task (e.g. "compare AGEZT with OpenClaw") → 5+ sources, a contradiction table, a cited answer.
- **F-14**: A run executing in a pod via `agt run --exec-profile k8s`, with artifacts fetched back from the pod.
- **F-12**: A pulse observer's "you have a meeting tomorrow" output reaches the user one hour in advance.
- **F-13**: A complex task (e.g. "research a book + turn the chapter summary into a PDF") is distributed via council + conductor, workboard lanes flow, and the result merges.

---

## 6. Phase 4 — Design-Stage Backlog (Closes Phase 0, then enters annual rotation)

**Entry:** Marked "Phase 6/8" inside SPEC-12/13/14/15/16; these are released as Phases 0–3 close.

### 6.1 Slices

| ID | Missing item | SPEC | Phase | Note |
|---|---|---|---|---|
| P4-A | F-15 First-class saga / compensation | SPEC-14 §1 | Phase 6 | `kernel/runtime/saga/`; declarative step + reverse step; orchestration via workflow |
| P4-B | F-16 Multi-tenant RBAC granular roles | SPEC-14 §4 | Phase 6 | `kernel/edict/dimension_user.go`; policy by role/group |
| P4-C | F-17 Single-instance RBAC + Edict user-dimension | SPEC-14 §4 | Phase 6 | (overlaps with P4-B) |
| P4-D | F-18 Escalation chains | SPEC-14 §6 | Phase 8 | `kernel/alerter/chain.go` |
| P4-E | F-19 External vault integration | SPEC-14 §7 | Phase 8 | Plug-gear vault adapter interface |
| P4-F | F-20 Widget SDK + Sandbox render | SPEC-12 | Phase 5+7+8 | `frontend/src/widgets/`; iframe + CSP |
| P4-G | F-21 Widget marketplace | SPEC-12 | Phase 8 | `kernel/market` widget loader |
| P4-H | F-22 Widget scaffold | SPEC-12 | Phase 8 | `tools/create-agezt-plugin` widget mode |
| P4-I | F-23 Capability eval harness | SPEC-14 §3 | Phase 5 | `cmd/eval/`; scenario + success rate |
| P4-J | F-24 Eval-driven reflection | SPEC-14 §3 | Phase 8 | `kernel/reflect/` → eval consumer |
| P4-K | F-25 UI i18n (TR) | SPEC-14 §8 | Phase 8 | `frontend/src/i18n/` + `react-i18next` |
| P4-L | F-26 OpenTelemetry export | SPEC-14 §9 | Phase 5-8 | `go.opentelemetry.io/otel` integration |
| P4-M | F-27 FinOps views / cost attribution | SPEC-14 §9 | Phase 5-8 | `cmd/agt finops` + dashboard |
| P4-N | F-28 Codec/encryption auto-rotation | SPEC-14 §7 | Phase 7 | `kernel/creds/rotation.go` policy |

### 6.2 Separate plan PRs are not expected for these slices; each progresses as it is merged with Phase 2/3.

---

## 7. NEXT.md Follow-up Items (whichever of the slices above they fall into)

| NEXT ID | Goal | Assigned Phase | Note |
|---|---|---|---|
| N-1 | Workflow → agent wake | Phase 1 (P1-E) | |
| N-2 | "Why quieted" audit event | Phase 4 (together with P4-D) | |
| N-3 | Guardian schedule low-frequency verification | Phase 4 (together with P4-D) | |
| N-4 | Auto-archive destructive path → owner sign-off | **DEFERRED** — consistent with `GRAVEYARD-POLICY.md`, separate PR | |
| N-5 | Config center per-agent visibility | Phase 1 (P1-F) | |
| N-6 | High-risk tool APPROVALS per-agent surface | Phase 4 (together with P4-B) | |

---

## 8. Strategy and "Beyond" Opportunities

### 8.1 F-11 (Deep Research Harness) — The Biggest Strategic Angle

`docs/JARVIS-VISION-2026.md` identifies it correctly:

> "The single biggest 'beyond' move: deep research + anticipatory proactivity. Both are weak in competitors, and AGEZT's ground (pulse/worldmodel/workflow/journal) is ready for them."

It can be started in Phase 3 BEFORE Phase 2, because it can be prototyped as a low-risk **research skill**. But its deployment order is in Phase 3.

### 8.2 Timing Windows

- **Sprint 0**: Phase 0 hygiene (4-5 d)
- **Sprint 1-2**: Phase 1 slices (10-20 d)
- **Sprint 3-8**: Phase 2 visible gates (60 d)
- **Sprint 9-12**: Phase 3 Jarvis differentiators (60+ d)
- **Phase 4 backlog**: in parallel with Sprint 5+

---

## 9. Cross-Documents

This plan must stay consistent with these sources:

- **`docs/SYSTEM-AUDIT-REPORT.md`** — raw inventory and numerical baseline.
- **`docs/MISSING-PARTS-REPORT.md`** — item-level inventory.
- **`docs/JARVIS-VISION-2026.md`** — strategy and Axis A/B.
- **`docs/OPENCLAW-HERMES-ROADMAP.md`** — competitor parity and Phase 1-2 ordering.
- **`docs/REFACTORING-INDEX.md`** — the A1/A2/A3/B5/C2/C4 slices.
- **`docs/GRAVEYARD-POLICY.md`** — the sign-off bar for N-4.
- **`docs/NEXT.md`** — top priorities + Immediate Context (neighbor guard).

If an item moves, **the other documents must be updated too**. A single-file change in a PR is out of policy.

---

## 10. Closing

This plan is the **first concrete roadmap** for AGEZT's evolution from an **agentic operating system** into a **Jarvis-class proactive operator assistant**. All items rest on code-base evidence, file references, and SPEC ↔ commit mapping. Since external fetch fails, competitor claims are limited to in-repo documents.

**First Sprint:** Phase 0 (4-5 days) → 1-2 of the Phase 1 slices (at minimum the continuation of C4 and N-1).

**Entry condition for Phase 2:** P0-A/B/C/D closed + `make check` green + no stale worktrees.

**Success metric (6 months out):** (a) F-3 + F-4 + F-6 live, (b) at least 1 stable release by October 2026, (c) at least 50 published skills in the external marketplace.

---

*This plan closes the `docs/NEXT.md` §0 reference. Revision notes live in the Item-Status table in `MISSING-PARTS-REPORT.md`.*
