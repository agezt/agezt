# AGEZT — Missing Parts Report (Raw Inventory of Missing Pieces)

> **Date:** 2026-07-04 (last updated: 2026-07-06)
> **Branch:** `main` (HEAD: `ef7b412d`)
> **Status:** ARCHIVED — Branch `refactor/c4-agentdetail-phase0` merged into `main` and deleted. Content retained for historical reference; see `docs/SYSTEM-AUDIT-REPORT.md` for the current audit state and `docs/MISSING-PARTS-PLAN.md` for the action plan.
> **Other references:** `docs/SYSTEM-AUDIT-REPORT.md`, `docs/MISSING-PARTS-PLAN.md`, `docs/OPENCLAW-HERMES-ROADMAP.md`, `docs/JARVIS-VISION-2026.md`, `docs/REFACTORING-INDEX.md`, `docs/GRAVEYARD-POLICY.md`.

---

## 0. Conventions

This report is the **canonical data sheet**. Each item follows this schema:

```
### F-NN <item name>
- **Priority:** P0 | P1 | P2 | P3
- **Phase:** SPEC-YY §X | Jarv-P0/P1/P2 | NEXT §N | Phase-0 hygiene
- **Status:** open | in-progress | done | deferred | needs-design
- **Owner (role):** <role name>
- **Evidence (code/doc):**
  - `<file path>`
  - `<path-to-doc>.md §X`
- **M-report (if any):** `PHASE-M???-...-REPORT.md` (verified on disk by a disk scan)
- **Dependencies:** <other F-/N- items, PRs>
- **Description:** <one paragraph>
- **Last note:** <YYYY-MM-DD: free text; pin when closed or deferred>
```

### Abbreviations

| Abbreviation | Meaning |
|---|---|
| `F-` | Functional gap — out-of-scope/unaddressed feature (defined in a SPEC). |
| `N-` | NEXT.md tail item — a follow-up item explicitly marked in `NEXT.md`. |
| `H-` | Hygiene — an item related to repo hygiene (worktree, docs, CI). |
| `D-` | Doc gap — documentation debt only. |
| `P0`/`P1`/`P2`/`P3` | Priority (matches the definition in the Phases, see `MISSING-PARTS-PLAN.md`). |
| **Jarv** | §6 Axis A/B reference in `JARVIS-VISION-2026.md`. |

### State Machine

```
open ──▶ in-progress ──▶ done
     │                  ▶ deferred (with rationale)
     │                  ▶ needs-design (spec blocked)
     └──▶ deferred (rationale: e.g. external fetch failed, owner sign-off required)
```

`done` is pinned only with a commit hash: `done → commit <hash>`.

---

## 1. Counter

| Category | open | in-progress | needs-design | done | deferred | Total |
|---|---|---|---|---|---|---|
| F (functional gap) | 20 | 2 | 2 | 0 | 4 | 28 |
| N (NEXT.md tail)    | 5  | 0 | 0 | 0 | 1 | 6 |
| H (hygiene)         | 0  | 0 | 0 | 6 | 0 | 6 |
| D (doc gap)         | 3  | 0 | 0 | 0 | 0 | 3 |
| **Total**          | **28** | **2** | **2** | **6** | **5** | **43** |

> **Note:** This count must be updated during a file revision. For "done" items, the closing commit hash is recorded in §6.

---

## 2. Functional Gaps (F-N)

### F-01 Mobile companion (PWA)
- **Priority:** P0 (Jarv P0-1)
- **Phase:** JARVIS-VISION §6 P0-1, `docs/OPENCLAW-HERMES-ROADMAP.md` Phase 0
- **Status:** open
- **Owner:** webui-owner (on F-01)
- **Evidence:** `pwa`/`mobile`/`companion` keyword matches are near zero in the code base; only `.dev-home/.../artifacts/index/art-01KWBV0T4C03GXR12WPWABGV7G.json` (an artifact JSON).
- **M-report:** None
- **Dependencies:** F-02 (the node registry core already exists in `kernel/peer`)
- **Description:** OpenClaw's distinguishing strength — an iOS/Android push node (PWA or native), approvals/inbox/voice/run-status, share-page webhook target. Absent in AGEZT.
- **Last note:** 2026-07-04 — can be started with the `frontend/manifest.json` and a Service Worker pattern.

### F-02 Desktop tray / menu-bar companion
- **Priority:** P0 (Jarv P0-1)
- **Phase:** JARVIS-VISION §6 P0-1
- **Status:** open
- **Owner:** cli-owner
- **Evidence:** The "tray" keyword has 0 hits in the code base; however, the `kernel/peer` peer infrastructure exists.
- **M-report:** None
- **Dependencies:** `kernel/peer` (existing)
- **Description:** Start/stop, health, approvals, push-to-talk, tunnel status.
- **Last note:** 2026-07-04 — a new `cmd/tray/` is recommended.

### F-03 Live browser tab session
- **Priority:** P0 (Jarv P0-2)
- **Phase:** JARVIS-VISION §6 P0-2, OPENCLAW-HERMES Phase 1
- **Status:** in-progress
- **Owner:** browser-tool-owner + browseruse-skill-owner
- **Evidence:**
  - `plugins/tools/browser/browser.go` (346 lines) — wrappers exist (`open/snapshot/click/type/wait/screenshot/downloads/cookies/tabs/close`)
  - `plugins/builtinskills/browseruse/scripts/browse.mjs` (395 lines) — 0 `stale` references
- **M-report:** None (yet)
- **Dependencies:** Browser action provider
- **Description:** A persistent live Chromium tab process + DOM stale-ref invalidation + a multi-step tab lifecycle are missing; E2E fixtures are missing.
- **Last note:** 2026-07-04 — the driver part of `browse.mjs` will be looked at for the `BrowserTool.Driver` interface.

### F-04 Skill LLM Curator (shadow mode)
- **Priority:** P0 (Jarv P0-3)
- **Phase:** JARVIS-VISION §6 P0-3
- **Status:** open
- **Owner:** skill-kernel-owner
- **Evidence:**
  - `kernel/skill/` (exists)
  - `PHASE-M401-SKILL-AUTOPROMOTE-REPORT.md` (auto-promote, deterministic)
  - Hermes's distinguishing strength.
- **M-report:** None (yet)
- **Dependencies:** M399-M401 infrastructure
- **Description:** Only a deterministic curator exists. Shadow-eval and auto-promote exist, but they are not LLM-judged. Hermes parity.
- **Last note:** 2026-07-04 — the LLM judge runs in the shadow as a helper model; it never deletes (always archive/revertable).

### F-05 Populated skill marketplace (ClawHub/agentskills.io)
- **Priority:** P1 (Jarv P1-5)
- **Phase:** JARVIS-VISION §6 P1-5
- **Status:** open
- **Owner:** market-owner
- **Evidence:**
  - `plugins/builtinmarket/` (infrastructure)
  - `kernel/market/` (registry)
  - Signed/BLAKE3-verified installation is ready via `plugins/builtinmarket/...` and `kernel/market`.
- **M-report:** None
- **Dependencies:** Trust UX (signature, risk card, scanner findings)
- **Description:** There is no living hub with thousands of packages.

### F-06 VSCode IDE extension (minimal)
- **Priority:** P1 (Jarv P1-6)
- **Phase:** JARVIS-VISION §6 P1-6
- **Status:** open
- **Owner:** acp-owner + ext-owner
- **Evidence:** The `kernel/acp/` ACP surface exists; no shipped extension.
- **M-report:** None
- **Dependencies:** ACP server
- **Description:** A package publishable to the VSCode marketplace; connects over ACP.

### F-07 Batch processing surface (100-1000 parallel)
- **Priority:** P1 (Jarv P1-7)
- **Phase:** OPENCLAW-HERMES §6 P1-7
- **Status:** open
- **Owner:** workflow-owner
- **Evidence:** Indirect via workflow; no direct `batch_*.go` in the code base.
- **M-report:** None
- **Dependencies:** workboard (M-*)
- **Description:** No direct surface; indirect via workflow.

### F-08 Credential pool + automatic rotation
- **Priority:** P1
- **Phase:** OPENCLAW-HERMES matrix 🟡
- **Status:** open
- **Owner:** creds-owner
- **Evidence:** `kernel/creds/keyring.go` + 28 files (`aws.go`, `sigv4.go`, `machineid_*.go`, `pbkdf2_test.go`, `kdf_known_answer_internal_test.go`, etc.); no multi-key pool, no automatic rotation.
- **M-report:** None
- **Dependencies:** M172 PBKDF2, M303 nonce
- **Description:** A keyring that uses a single key; a pooled multi-key keyring + time-based rotation is missing.

### F-09 Context `@file/folder/diff/URL` references
- **Priority:** P1 (Jarv P1-4)
- **Phase:** JARVIS-VISION §6 P1-4
- **Status:** open
- **Owner:** chat-owner + context-kernel-owner
- **Evidence:** `chat_summarize` exists in the code base; no `@mention` parser; no injection-scanned import of AGENTS.md/CLAUDE.md/SOUL.md.
- **M-report:** None
- **Dependencies:** `kernel/runtime/context_budget`
- **Description:** Hermes's distinguishing strength; currently partial or absent.

### F-10 `agt migrate openclaw|hermes` command
- **Priority:** P1
- **Phase:** SPEC-13 §1.3, ROADMAP Phase 9
- **Status:** open
- **Owner:** cmd-owner
- **Evidence:** `cmd/agt/vault_migrate_test.go` exists; no real `cmd/agt/migrate.go`.
- **M-report:** None
- **Dependencies:** SPEC-13 §1.3
- **Description:** Only vault migration exists; no OpenClaw/Hermes profile+memory+skill import command.

### F-11 Deep research harness
- **Priority:** P2 (Jarv P2-8)
- **Phase:** JARVIS-VISION §6 P2-8; `docs/DEEP-RESEARCH-HARNESS-PLAN.md`, `docs/DEER-FLOW-IMPLEMENTATION-PLAN.md`
- **Status:** needs-design
- **Owner:** research-tool-owner + council-owner + conductor-owner
- **Evidence:**
  - `plugins/tools/research/`, `plugins/tools/council/`, `plugins/tools/conductor/` (partial)
  - `docs/DEEP-RESEARCH-HARNESS-PLAN.md` (plan)
  - `docs/DEER-FLOW-IMPLEMENTATION-PLAN.md` (plan)
- **M-report:** None
- **Dependencies:** Pulse + worldmodel + workflow (existing)
- **Description:** Multi-source fan-out → deep reading → contradiction/adversarial verification → cited synthesis. **The biggest strategic gap.**

### F-12 Anticipatory autonomy
- **Priority:** P2 (Jarv P2-9)
- **Phase:** JARVIS-VISION §6 P2-9
- **Status:** needs-design
- **Owner:** pulse-owner
- **Evidence:** Pulse observer + Initiative (M999), Reaper (M903) exist.
- **M-report:** None
- **Dependencies:** worldmodel decay
- **Description:** pulse + worldmodel + memory → preparing the user's next need in advance (briefing/draft/alert).

### F-13 Society-of-agents productization
- **Priority:** P2 (Jarv P2-10)
- **Phase:** JARVIS-VISION §6 P2-10
- **Status:** in-progress (UI exists)
- **Owner:** workflow-owner + frontend-owner
- **Evidence:** The `frontend/src/views/Council.tsx` and `frontend/src/views/Conductor.tsx` UI exist.
- **M-report:** None
- **Dependencies:** F-11 (closely linked to the research harness)
- **Description:** To be matured with live multi-agent reasoning + a delegation graph + workboard lane integration.

### F-14 K8s job lifecycle
- **Priority:** P2 (Jarv P2-11)
- **Phase:** OPENCLAW-HERMES Phase 1 §Terminal/sandbox; JARVIS-VISION P2-11
- **Status:** open
- **Owner:** exec-profile-owner
- **Evidence:** `kernel/runtime/exec_profile/` (partial).
- **M-report:** None
- **Dependencies:** Local/SSH/Daytona exec-profile (existing)
- **Description:** The pod lifecycle for shell/code_exec routing is not complete.

### F-15 First-class saga / compensation
- **Priority:** P3
- **Phase:** SPEC-14 §1 Phase 6
- **Status:** open
- **Owner:** runtime-owner + workflow-owner
- **Evidence:** `kernel/runtime`, `kernel/workflow`, `kernel/resume`. No declarative saga model.
- **M-report:** None
- **Dependencies:** workflow engine
- **Description:** Basic retry/checkpoint exists; full saga reverse-invocation is still partial.

### F-16 Multi-tenant RBAC granular role model
- **Priority:** P3
- **Phase:** SPEC-14 §4 Phase 6; SPEC-09 §6
- **Status:** open
- **Owner:** tenant-owner + edict-owner
- **Evidence:** `kernel/tenant/tenant.go`. No granular roles.
- **M-report:** None
- **Dependencies:** Edict user-dimension (F-17)
- **Description:** Multi-tenant exists; there is no "non-burdensome role for a single person".

### F-17 Single-instance RBAC + Edict user-dimension
- **Priority:** P3
- **Phase:** SPEC-14 §4 Phase 6
- **Status:** deferred (overlaps with F-16; reopen when F-16 closes)
- **Owner:** edict-owner
- **Evidence:** The `kernel/edict/` policy engine exists.
- **M-report:** None
- **Dependencies:** F-16
- **Description:** A multi-user model within a single instance.

### F-18 Escalation chains
- **Priority:** P3
- **Phase:** SPEC-14 §6 Phase 8
- **Status:** open
- **Owner:** alerter-owner + pulse-owner
- **Evidence:** `kernel/alerter/alerter.go`, `kernel/alerter/alerter_test.go` (existing).
- **M-report:** None
- **Dependencies:** channel push (existing, M922)
- **Description:** Alert → ack tracking → channel-to-channel escalation; ack tracking.

### F-19 External vault integration
- **Priority:** P3
- **Phase:** SPEC-14 §7 Phase 8
- **Status:** open
- **Owner:** creds-owner + plugin-owner
- **Evidence:** `kernel/creds/creds.go`; vault enc + rotate exist. No pluggable secret-provider interface.
- **M-report:** None
- **Dependencies:** F-28 (rotation)
- **Description:** A pluggable secret backend for orgs (HashiCorp Vault, AWS Secrets Manager, etc.).

### F-20 Widget SDK + Sandbox render
- **Priority:** P3
- **Phase:** SPEC-12 §5-7 (Phase 5+7+8)
- **Status:** open
- **Owner:** webui-owner + runtime-owner
- **Evidence:** SPEC-12; the frontend `views/Chat.tsx` uses Markdown, not widget rendering.
- **M-report:** None
- **Dependencies:** Frontend i18n (F-25)
- **Description:** Sandboxed (iframe + strict CSP); widgets are interactive elements that enrich the conversation.

### F-21 Widget marketplace
- **Priority:** P3
- **Phase:** SPEC-12 §4-5 (Phase 8)
- **Status:** deferred (dependent on F-20)
- **Owner:** market-owner
- **Evidence:** SPEC-12 §4-5.
- **M-report:** None
- **Dependencies:** F-20
- **Description:** A marketplace for widgets (shared infrastructure with F-05).

### F-22 Widget scaffold (`create-agezt-plugin` widget mode)
- **Priority:** P3
- **Phase:** SPEC-12 §5 (Phase 8)
- **Status:** deferred (dependent on F-20-F-21)
- **Owner:** sdk-owner
- **Evidence:** SPEC-12 §5.
- **M-report:** None
- **Dependencies:** F-20, F-21
- **Description:** The scaffolder generates a template for widgets.

### F-23 Capability eval harness
- **Priority:** P3
- **Phase:** SPEC-14 §3 (Phase 5)
- **Status:** open
- **Owner:** eval-owner
- **Evidence:** SPEC-14 §3.
- **M-report:** None
- **Dependencies:** M399 shadow-eval
- **Description:** Tool/skill success-rate measurement; scenarios derived from the journal.

### F-24 Eval-driven reflection closure
- **Priority:** P3
- **Phase:** SPEC-14 §3 (Phase 8)
- **Status:** deferred (dependent on F-23)
- **Owner:** reflect-owner
- **Evidence:** SPEC-14 §3.
- **M-report:** None
- **Dependencies:** F-23
- **Description:** Reflection consumes eval results.

### F-25 UI i18n (in addition to the TR default)
- **Priority:** P3
- **Phase:** SPEC-14 §8 (Phase 8)
- **Status:** open
- **Owner:** webui-owner
- **Evidence:** No i18n library in `frontend/package.json` (i18next, react-intl, @formatjs all absent).
- **M-report:** None
- **Dependencies:** None
- **Description:** Currently hardcoded English; English default + Turkish-ready + locale-aware formatting.

### F-26 OpenTelemetry export
- **Priority:** P3
- **Phase:** SPEC-14 §9 (Phase 5-8)
- **Status:** open
- **Owner:** observability-owner
- **Evidence:** No `otel`/`opentelemetry` in `go.sum`; SPEC-14 §9.
- **M-report:** None
- **Dependencies:** None
- **Description:** Export traces/metrics/logs to an external collector (otel/jaeger/tempo).

### F-27 FinOps views / cost attribution
- **Priority:** P3
- **Phase:** SPEC-14 §9 + SPEC-10 §6
- **Status:** open
- **Owner:** governor-owner
- **Evidence:** `kernel/controlplane/budget.go` + `kernel/governor/*budget*.go` (existing); 0 "finops"/"cost attribution" keywords.
- **M-report:** None
- **Dependencies:** Cost aggregation store
- **Description:** Cost attribution and trends per tenant/agent/task.

### F-28 Codec/encryption auto-rotation lifecycle
- **Priority:** P3
- **Phase:** SPEC-14 §7 (Phase 7)
- **Status:** open
- **Owner:** creds-owner
- **Evidence:** `kernel/creds/rotate_test.go` exists; no automation policy.
- **M-report:** None
- **Dependencies:** F-19, F-08
- **Description:** An automatic rotation policy for vault encryption keys and API keys.

---

## 3. NEXT.md Tail Items (N-N)

### N-01 Workflow → agent-node wake
- **Priority:** P1
- **Phase:** NEXT §2 end; MISSING-PARTS-PLAN P1-E
- **Status:** open
- **Owner:** controlplane-owner + workflow-owner
- **Evidence:** `kernel/workflow`, `kernel/controlplane` exist; no `workflow_fired` subject analogous to `standing_fired`.
- **M-report:** None
- **Dependencies:** M-1 autonomy runbook
- **Description:** There is no path for a workflow to wake an agent directly.

### N-02 "Why quieted" audit event
- **Priority:** P3
- **Phase:** NEXT §6; MISSING-PARTS-PLAN P4-D
- **Status:** open
- **Owner:** guardian-owner + alerter-owner
- **Evidence:** `plugins/builtinguardians/`, `cmd/agt/doctor.go`.
- **M-report:** None
- **Dependencies:** F-18
- **Description:** A `policy.quiet_patch_fired` event for doctor quiet patch fires.

### N-03 Guardian schedule low-frequency verification
- **Priority:** P3
- **Phase:** NEXT §6
- **Status:** open
- **Owner:** guardian-owner + scheduler-owner
- **Evidence:** `plugins/builtinguardians/SeedAll` 8h cooldown.
- **M-report:** None
- **Dependencies:** None
- **Description:** Periodic verification that no guardian fires continuously (seeder 8h).

### N-04 Auto-archive (graveyard) destructive path — sign-off
- **Priority:** P3 → deferred (owner sign-off required)
- **Phase:** NEXT §7 end; `docs/GRAVEYARD-POLICY.md`
- **Status:** deferred (owner sign-off)
- **Owner:** roster-owner + owner
- **Evidence:** `docs/GRAVEYARD-POLICY.md`, `kernel/controlplane/roster.go`.
- **M-report:** None
- **Dependencies:** Compliance — `docs/GRAVEYARD-POLICY.md`
- **Description:** Sign-off for destructive auto-archive. Currently only a **report** (graveyard_scan system task).
- **Last note:** 2026-07-04 — defer until sign-off arrives.

### N-05 Config center per-agent visibility (the quad)
- **Priority:** P1
- **Phase:** NEXT §5; MISSING-PARTS-PLAN P1-F
- **Status:** open
- **Owner:** configcenter-owner + frontend-owner
- **Evidence:** `kernel/controlplane/configcenter_handler.go`, `frontend/src/components/AgentDetail.tsx`.
- **M-report:** None
- **Dependencies:** None
- **Description:** The owned / shared-allowlisted / hidden secrets / excluded quad.

### N-06 High-risk tool APPROVALS per-agent surface
- **Priority:** P3
- **Phase:** NEXT §5; MISSING-PARTS-PLAN P4-B
- **Status:** open
- **Owner:** controlplane-owner + frontend-owner
- **Evidence:** `kernel/agent/agent.go`, the `/api/approvals` log; per-agent denies exist (M-…); no per-agent approvals.
- **M-report:** None
- **Dependencies:** F-17 (user-dimension)
- **Description:** An approvals surface in addition to denies.

---

## 4. Hygiene Items (H-N)

### H-01 Stale worktrees (22 → 1 locked)
- **Priority:** P0 (Phase 0 hygiene)
- **Phase:** MISSING-PARTS-PLAN P0-1
- **Status:** done 2026-07-04 (destructive approval obtained; 1 locked remnant — cleanable after a reboot/lock release)
- **Owner:** ops-hygiene
- **Evidence:**
  - `git worktree list` → 3 worktrees (1 main + deep-research + rebased-main)
  - Folder scan (previously): 22 folders
  - 2026-07-04 round 1: 16 empty orphans (0-byte) deleted.
  - 2026-07-04 round 2 (after destructive approval): 3 populated orphans deleted — `anim` (10 MB / 0.1s), `m951-webui-modernize` (161 MB / 1.7s), `ci-verify` (187 MB / 3.5s). **358 MB freed in total.**
  - **Remnant:** `.worktrees/m1002-resume` (0-byte, locked by another process — possibly Windows Defender or SearchService; can be retried after a reboot or a lock release).
- **M-report:** None
- **Dependencies:** None
- **Description:** `git worktree prune` + recursive deletion of unlisted worktrees. 19 orphans deleted in total. The main goal — "clean `git worktree list` + reduce 22 → 2 orphans" — is complete; the last remnant is harmless.

### H-02 SPEC headers canonicalized — complete
- **Priority:** P0 (Phase 0 hygiene)
- **Phase:** MISSING-PARTS-PLAN P0-2
- **Status:** done → 2026-07-04
- **Owner:** docs-curator
- **Evidence:** `.project/SPEC-01..16-*.md` — `Status: Active · Domain: github.com/agezt/agezt · License: MIT` (16/16).
- **Last note:** 2026-07-04 — all 16 SPECs canonicalized.

### H-03 SPEC-IMPLEMENTATION-STATUS.md created — complete
- **Priority:** P1
- **Phase:** MISSING-PARTS-PLAN P0-4
- **Status:** done → 2026-07-04
- **Owner:** docs-curator
- **Evidence:** `docs/SPEC-IMPLEMENTATION-STATUS.md` (exists, 17.5 KB / 317 lines); 13 shipped + 2 partial + 1 design-only + 0 not-started; SPEC-12 widget is an outlier with 0 M-reports; §3 cross matrix (SPEC ↔ F-/N-/H-/D-) added.
- **Dependencies:** None
- **Description:** A 16 SPEC × {complete/partial/missing} matrix.

### H-04 CHANGELOG.md reorg plan — complete (plan only)
- **Priority:** P2
- **Phase:** MISSING-PARTS-PLAN P0-5
- **Status:** done → 2026-07-04 (plan created; implementation is a 4-PR incremental sprint)
- **Owner:** release-mgr
- **Evidence:** `docs/CHANGELOG-REORG-PLAN.md` (12.4 KB / 312 lines). Plan: slicing into M-ranges of 100; PR-1 `tools/changelog-split`, PR-2 `tools/changelog-lint`, PR-3 file writing, PR-4 migration helper.
- **Dependencies:** None
- **Description:** Split the CHANGELOG into per-milestone files. **Implementation** (PR-3) happens per the steps in this plan; H-04 is done within the scope of the plan.

### H-05 `make check` green verification — complete
- **Priority:** P0
- **Phase:** MISSING-PARTS-PLAN P0-6
- **Status:** done → 2026-07-04
- **Owner:** every agent (PR gate)
- **Evidence:**
  - `go run ./tools/jsonschemagen` — exit 0
  - `go vet ./...` — exit 0
  - `go run ./tools/depscheck` — "OK: 24 core dependencies, all justified"
  - `go run ./tools/sdkparity -check docs/SDK-PARITY.md` — exit 0 (after re-generate)
  - `npm test` (vitest) — 1453/1453 passed, 176 test files (~26 s)
  - `npm run typecheck` — exit 0
  - `npm run build` — 390 ms, 2167 modules
  - `go test -count=1 -p=1 -short ./...` — **all packages green** (~180+ packages)
  - `go run ./tools/deadcodecheck` — **OK: no unexpected dead code**
  - `staticcheck ./...` — **clean**
- **Dependencies:** None
- **Description:** Verify the PowerShell equivalent is green. The main CI gate (test+build+lint+deadcode) is green. Since the first parallel `go test ./...` on Windows can produce a socket-buffer error, it was run with `-p=1`; consistent with NEXT.md §Current Validation Commands.

### H-06 `.dev-home/.gitignore` verification — complete (already ignored)
- **Priority:** P1
- **Phase:** MISSING-PARTS-PLAN P0-7
- **Status:** done → 2026-07-04 (the root `.gitignore` already ignores `.dev-home/`, verified)
- **Owner:** ops-hygiene
- **Evidence:** Root `.gitignore` line 101: the `.dev-home/` pattern. `git check-ignore -v` marks all of the 12 different `.dev-home/{config.json,creds.json,agentgw.secret,sandbox/,journal/,datalake/,memory/,roster/,artifacts/...}` paths as IGNORED; `git ls-files` shows them untracked. The estimate in SYSTEM-AUDIT-REPORT §5.3 is verified.
- **Dependencies:** None
- **Description:** Verify with `git check-ignore`; add to `.gitignore` if not ignored. **Already ignored — no additional action needed.**

---

## 5. Doc Gaps (D-N)

### D-01 `.project/TASKS.md` v0.1 draft refresh
- **Priority:** P2
- **Phase:** SYSTEM-AUDIT-REPORT §4.3
- **Status:** open
- **Owner:** docs-curator
- **Evidence:** `.project/TASKS.md` (exists, "Status: Draft v0.1"); all checklist items are `[ ]` todos.
- **Dependencies:** None
- **Description:** Not in sync with the 1000+ M-phase reports; either rewrite it or mark it "archived".

### D-02 Competing SPEC summaries (README/SPEC bridge)
- **Priority:** P2
- **Phase:** SYSTEM-AUDIT-REPORT §4.5
- **Status:** open
- **Owner:** docs-curator
- **Evidence:** `JARVIS-VISION-2026.md`, `OPENCLAW-HERMES-ROADMAP.md` have competitor-parity matrices; but there is no AGEZT-internal SPEC ↔ code status matrix.
- **Dependencies:** shared with H-03
- **Description:** A short summary table of SPEC-01..16 in the README.

### D-03 Competing transition tables (`STATUS-*.md`)
- **Priority:** P3
- **Phase:** SYSTEM-AUDIT-REPORT §4.4
- **Status:** open
- **Owner:** docs-curator
- **Evidence:** `.project/STATUS-2026-06-03-POST-M{249,255,257,265}.md` (exists); a canonical format for new milestone transitions.
- **Dependencies:** None
- **Description:** A status snapshot at milestone transitions.

---

## 6. Status Log (Closed / Deferred Items)

This section keeps the dated record of **done** and **deferred** items. Newly closed items are moved here.

### Closed (done)

| ID | Date | Commit | Closing note |
|---|---|---|---|
| **H-02** SPEC header canonicalization | 2026-07-04 | (this session) | 16/16 SPECs `Active · Domain: github.com/agezt/agezt · License: MIT` (`Language: English` added except for SPEC-09). |
| **H-01** Stale worktrees | 2026-07-04 | (this session) | 19 orphans deleted (16 empty + 3 populated, 358 MB total). `git worktree list` clean: main + `deep-research` + `rebased-main`. `m1002-resume` (0-byte) could not be deleted because of a Windows process lock (harmless remnant; cleanable after a reboot/lock release). |
| **H-03** SPEC-IMPLEMENTATION-STATUS.md | 2026-07-04 | (this session) | `docs/SPEC-IMPLEMENTATION-STATUS.md` (17.5 KB / 317 lines); 13 shipped + 2 partial + 1 design-only. |
| **H-04** CHANGELOG.md reorg plan | 2026-07-04 | (this session) | `docs/CHANGELOG-REORG-PLAN.md` (12.4 KB / 312 lines) created. A strategy to slice the 646 KB single file into M-ranges of 100; a 4-PR implementation plan. |
| **H-05** `make check` green verification | 2026-07-04 | (this session) | Main CI gate (test+build+lint+deadcode) GREEN: jsonschemagen, vet, depscheck (24 OK), sdkparity (after re-gen), vitest 1453/1453, typecheck, build 390ms, `go test -count=1 -p=1 -short ./...` all packages green, `go run ./tools/deadcodecheck` clean, `staticcheck ./...` clean. |
| **H-06** `.dev-home/.gitignore` verification | 2026-07-04 | (this session) | With the `.dev-home/` pattern at line 101 of the root `.gitignore`, all runtime state (config.json, creds.json, agentgw.secret, journal, datalake, sandbox, etc.) is already ignored. For 12 different files/dirs, `git check-ignore` marks all IGNORED; `git ls-files` untracked. |

### Deferred

| ID | Date | Rationale | Reopen condition |
|---|---|---|---|
| **F-17** Single-instance RBAC + Edict user-dimension | 2026-07-04 | Overlaps with F-16; reopen when F-16 closes | When F-16 completes in a PR |
| **F-21** Widget marketplace | 2026-07-04 | Dependent on F-20 | When F-20 is done |
| **F-22** Widget scaffold | 2026-07-04 | Dependent on F-20 + F-21 | When F-20 and F-21 are done |
| **F-24** Eval-driven reflection | 2026-07-04 | Dependent on F-23 | When F-23 is done |
| **N-04** Graveyard destructive auto-archive | 2026-07-04 | Consistent with `docs/GRAVEYARD-POLICY.md`; owner sign-off required for the destructive path | Owner sign-off + clarification of the `docs/GRAVEYARD-POLICY.md` policy |

### Awaiting on disk (notes/limitations)

- External fetch failed (project memory `#fetch #github #undici #network`); competitor documents (`openclaw.ai`, `hermes-agent.nousresearch.com`) were matched via the in-repo `OPENCLAW-HERMES-ROADMAP.md`.
- "Owner (role)" fields are **operational role** names; a single person/agent is assigned when the PR is opened.

---

## 7. Cross-Document Map

| This report | ↔ | Target |
|---|---|---|
| F- / N- | ↔ | `SYSTEM-AUDIT-REPORT.md` §3 (the auditor summary of the inventory) |
| F- / N- | ↔ | `MISSING-PARTS-PLAN.md` §3-7 (the Phases) |
| H- / D- | ↔ | `MISSING-PARTS-PLAN.md` §2 (Phase 0 hygiene) |
| Closed items | ↔ | `MISSING-PARTS-PLAN.md` §2 "CLOSED date" note |
| TBD → canonical conversions | ↔ | `H-02` §6 log + commit hash |

As new items are added here, a **cross-update in the other documents** is included in the PR.

---

## 8. Item Statistics (snapshot 2026-07-04)

- **Total items:** 43 (28 F + 6 N + 6 H + 3 D)
- **Open:** 28 (F=20, N=5, H=0, D=3)
- **In-progress:** 2 (F-03 browser tab, F-13 society-of-agents)
- **Needs-design:** 2 (F-11 deep research, F-12 widgets)
- **Done:** 6 (H-01 worktree, H-02 SPEC canonicalization, H-03 SPEC-IMPLEMENTATION-STATUS, H-04 CHANGELOG reorg plan, H-05 make check, H-06 .dev-home gitignore)
- **Deferred:** 5 (F-17 RBAC, F-21 widget market, F-22 widget scaffold, F-24 eval reflection, N-04 graveyard destructive)
- **Highest M-report on disk:** M923 (M1002 is referenced in the CHANGELOG)
- **Per-file hit totals for F-03 through F-28:** verified in SYSTEM-AUDIT-REPORT §3.

> **2026-07-04 spotlight correction:** In the previous writeup of the §1 Counter table, F=24/2/0/2, N=5/0/0/1, H=0/0/6/0, D=2/0/0/0 was shown; the total row (36/2/1/3/42) was inconsistent. The real values: F=20/2/2/0/4=28, N=5/0/0/0/1=6, H=0/0/0/6/0=6, D=3/0/0/0/0=3; total=43. The `needs-design` column was added.

---

*This inventory is a living document. As new items are added here, their counterparts in `SYSTEM-AUDIT-REPORT.md` §3 and `MISSING-PARTS-PLAN.md` are cross-updated. Double-record keeping is out of policy.*
