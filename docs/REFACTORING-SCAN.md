# Refactoring Scan — AGEZT

> **Generated:** 2026-07-03 · **Branch:** `feat/cursor-pagination-agents` (clean tree)
> **Method:** cross-referenced `docs/ARCHITECTURE.md`, `docs/ARCHITECTURAL-REPORT.md`,
> `docs/DEAD-CODE-AUDIT.md`, the live file listing of every `kernel/`, `frontend/src/`,
> and `cmd/` directory, plus recent origin/main history (`dbff54ad`…`04ddbb0c`) which
> already cleaned up pagination, SSE state, modals, and dead code.
>
> **Note:** Several parallel sessions are actively editing the frontend UI slices,
> tunnel settings, and release hygiene. This scan avoids touching in-flight files;
> findings below are the *next* layer of cleanup — call sites not yet swept, subsystem
> drift, and architectural seams the architecture doc describes but the code doesn't enforce.

The codebase is in good shape for its age: a real `kernel/` ↔ `plugins/` ↔ `frontend/` ↔
`sdk/` separation, a single source of truth for identity (`internal/brand`), and the recent
S0–S4 series already consolidated `Modal`, `nav`, `MonacoView`, `useCursorPager`, and most
list-endpoint paginations.

---

## A. Kernel — controlplane footprint

### A1. `kernel/controlplane` is a 190-file god-package
- **Where:** `kernel/controlplane/*.go` (190 files, ~140 non-test).
- **Problem:** every read/write/mutation handler in the system lives here; sibling
  `kernel/<domain>` packages already own domain logic. Examples: `runs.go` owns cursor
  pagination + status filter + MS/seq cursor encoding for the run journal (belongs to
  `kernel/journal`); `roster.go` owns the wake/schedule/standing/delegated/doctor fold;
  `board.go`/`inbox.go`/`memory.go` own paginated domain views.
- **Fix:**
  1. Move read-and-fold logic into the matching domain package
     (`kernel/journal/runs.go`, `kernel/roster/wake_runbook.go`, `kernel/board/paginate.go`,
     `kernel/memory/paginate.go`). Keep only req→handler→envelope glue in controlplane.
  2. Make `kernel/controlplane/args.go` the canonical cursor/limit/filter decoder for
     every paginated handler (only ~7 endpoints use it today).
  3. Split `server.go` into per-domain dispatch tables (`dispatch_runs.go`, etc.); today
     it's one hand-written `ServeMux` literal listing ~80 routes.

### A2. Pagination applied to 7 endpoints; ~12 list endpoints still stream full slices
- **Where:** `kernel/controlplane/{approvals_log,tool_log,provider_log,policy_log,plan_history,webhook_log,ratelimit_log,world_log,memory_log,netguard_log,warden_log}.go`, `edict_overlay.go`, `schedule_fires.go`.
- **Problem:** `/api/runs`, `/api/agents`, `/api/agents/{activity,repair_status,escalations}`,
  `/api/{inbox,board,memory}` are paginated (cf9813bd → 1cf4bb01); the `*_log.go` and
  overlay endpoints still return unbounded slices and bypass `readArgsRoutes`/`useCursorPager`.
- **Fix:** move each to `args.go` cursor parsing + `next_cursor` emit, register on
  `readArgsRoutes`, add a wrapper in `frontend/src/lib/cursorPager.ts` + load-more footer.

### A3. Six overlapping HTTP surfaces
- **Where:** `kernel/webui/webui.go` + `kernel/webui/*_route.go`, `kernel/restapi/restapi.go`,
  `kernel/openaiapi/openaiapi.go`, `kernel/acp/acp.go`, `kernel/agentgw/gateway.go`,
  `kernel/restapi/mailbox.go`.
- **Problem:** routes for all surfaces coordinated by hand through `webui.go`'s `ServeMux`
  literal. Each new route is a four-file edit. `restapi`/`openaiapi` duplicate auth/token/
  unix-socket logic that's also in `webui`.
- **Fix:** promote a single `AddRoute(method, path, allowlist, handler)` helper owning the
  `apiRoutes` vs `readArgsRoutes` split and per-route body caps; group routes by prefix in
  sub-registrars (`files_route.go` already does this); extract `kernel/httpserver/auth.go`
  shared by all three surfaces.

### A4. Server bootstrap is one ~5,000-line function
- **Where:** `cmd/agezt/main.go`.
- **Problem:** every subsystem wired inline; recent security fixes had to thread `baseDir`
  through helper signatures because the wiring lives in `main.go`.
- **Fix:** each subsystem exposes `Register(s *kernel.Services)`; `main.go` becomes
  load-config → build-services → mount → banner → serve → drain. Move `writeAPIListenToken`
  to `kernel/apiutil/tokenfile.go`.

### A5. Test helpers cross-cut three packages
- **Where:** `kernel/{controlplane,runtime,agent}/mock_helpers_test.go`.
- **Problem:** near-identical `mockJournal`/`mockRoster`/`mockGovernor` duplicated.
- **Fix:** move shared fixtures to `kernel/internal/testfixtures` (test-only), keep
  domain-specific data in-package.

---

## B. Kernel — domain seams

- **B1.** `kernel/runtime` (60+ files) mixes orchestrator + tool implementations. Move
  `voicetool.go`/`markettool.go`/`mcptool.go`/`imagetool.go`/`reranktool.go`/`braindistill.go`
  to their plugin/domain homes; runtime keeps loop/retry/lifecycle/delegation/journal/context.
- **B2.** `kernel/streamlimit` is a governor concern; collapse into `kernel/governor/streamlimit`.
- **B3.** `kernel/edict/toolmap.go` is a second top-level surface; move under
  `kernel/edict/internal/` or split into named sub-packages.
- **B4.** `kernel/board` / `kernel/memory` / `kernel/workboard` overlap (all key by
  `agent_slug`, all emit on the bus, all expose List/Append/Recall). Define a shared
  `kernel/identity/scopedstore` interface.
- **B5.** No dedicated auth package; `s.auth()` + token/tenant/oauth logic scattered across
  controlplane/webui/restapi/openaiapi/agentgw. Extract `kernel/auth/{token,middleware,tenant,oauth}.go`.
- **B6.** Three scheduling subsystems (`standing`/`scheduler`/`cadence`) with similar trigger
  evaluation. Factor a shared `kernel/triggers` evaluator, keep queues separate.
- **B7.** `kernel/resume` split from `runtime/resume.go` incomplete; finish making
  `runtime/resume.go` a thin adapter; move the lifecycle test into `kernel/resume/`.
- **B8.** `kernel/workflow` (authoring) vs `kernel/runtime/workflowrun.go` (execution) vs
  `kernel/planner` (DSL) — runtime should call workflow via a small `Executor.RunPlan`
  interface, not concrete types. Document the boundary in package doc comments.
- **B9.** conductor/council/research implemented twice (runtime + controlplane) with the same
  loop→fallback→orchestrator→delegate pattern. Factor a `kernel/loop/orchestrator` skeleton.
- **B10.** First-party in-process channels bypass `kernel/plugin/host.go` and plug straight into
  `kernel/channel/registry.go`. Either migrate through the host under a `ChannelInProc` flag or
  document the dual model in `docs/PLUGIN-SECURITY.md`.
- **B11.** Env prefix / binary names sometimes hard-coded (`AGEZT_TUNNEL`, `agt`) instead of
  via `internal/brand`. Frontend has no central env-prefix constant.

---

## C. Frontend — module boundaries

- **C1.** View-god-files: ~140 view files, many > 1,000 lines mixing fetch + state + layout +
  inline sub-components. Split each into `views/<X>/{Panel*.tsx, use*.ts, types.ts}`; promote
  cross-view pieces to `components/<X>/`.
- **C2.** `lib/` has 130+ files, most view-scoped (`conductorStore.ts`, `agentrepair.ts`,
  `rundetail.ts`, etc.). Co-locate view-specific helpers under `views/<X>/lib/`; keep `lib/`
  for things used by 3+ views (`api`, `nav`, `cursorPager`, `markdown`, `language`, `monaco`,
  `files`, `utils`, `events`, `format`, `theme`).
- **C3.** `useCursorPager` has one consumer (Runs). Extend to every list view (Inbox, Board,
  Memory, Agents, AgentPage, Roster, Tools, Skills, Schedules, Workflows, ConfigCenter, Market,
  Council, Approvals, Alerts) — tracks the in-flight pagination work.
- **C4.** `views/Chat.tsx` is a router, not a page (9 separate `Chat.*.test.tsx` files).
  Decompose into `views/Chat/{Layout,Composer,Bubble,Summary,Fallback,Persona,ExecutionProfile}.tsx`.
- **C5.** Modal migration incomplete. `grep -rn "fixed inset-0 z-50" frontend/src` should
  return only the `components/ui/Modal.tsx` import — audit CommandPalette, QuickConnect,
  toggle drawers.
- **C6.** Path-safety logic duplicated across `lib/files.ts` (`isPathSafe`), `lib/markdown.ts`
  (mention regex), `components/FileMention.tsx`, and `kernel/webui/files_route.go`. Consolidate
  the frontend rules under `lib/files.ts`; document 1:1 with the kernel rule.
- **C7.** `lib/markdown.ts` `parseInline` is an if/else token ladder approaching a state machine.
  Convert to an explicit left-to-right tokenizer emitting `(type,start,end)` spans + one
  renderer per token.
- **C8.** SSE "still alive?" predicate reimplemented per indicator (AlertBell, ApprovalsBell,
  Fleet). Lift `connectionState(now, lastEventAt)` to `lib/events.ts`.
- **C9.** `useEvents`/`usePanel` exist but consumers still re-inline filter+slice. Route
  "latest N events" / "is event in window" through a `lib/events.ts` hook.

---

## D. CLI — `cmd/agt`

- **D1.** ~160 source files; each subcommand re-wires flags + output + `--json` negotiation.
  Group by wrapped kernel package (`runs.go`, `plan.go`, `provider.go`, …); a single
  `cli/output.go` owns `--json`/`--human`.
- **D2.** CLI reaches into `kernel/state`/`kernel/journal` for some commands. Every command
  must go through `kernel/controlplane/client.go`; audit non-contract kernel imports from `cmd/agt`.

---

## E. SDK

- **E1.** `sdk/{go,typescript,python,rust}` have hand-written code alongside generated. Add
  `go run ./tools/sdkparity -strict` that fails CI when hand-written code sits outside `gen/`.
- **E2.** Public SDK methods flagged by the dead-code audit were correctly kept (beta API).
  Add an `docs/API-STABILITY.md` rule for when a deprecated SDK method becomes removable.

---

## F. Infrastructure / repo hygiene

- **F1.** `install.sh`/`install.ps1` are a separate build path from `Makefile`/`scripts/build.sh`
  (which got the recent `-trimpath`/LDFLAGS/banner-stamp fix). Have the installers source
  `scripts/build.sh` (or `make build`) so stamping can't drift.
- **F2.** Two package managers: CI uses npm + committed lockfile, but `frontend/pnpm-workspace.yaml`
  exists. Pick one (npm lockfile is the more reviewable supply-chain surface). **Owner decision.**
- **F3.** `__scan_routes.cjs` left untracked in repo root by a prior scan — delete or move to `tools/`.
- **F4.** Confirm `sdk/python/agezt.egg-info/` and `sdk/rust/target/` are `.gitignore`d (build artefacts).
- **F5.** `tmp/` at repo root — untracked; move under the owning tool + `.gitignore` if it's tooling.

---

## G. Tests / quality

- **G1.** Frontend is component-test-heavy, integration-light. Add ~5 Playwright E2E flows
  (login, file open, agent wake, schedule fire, channel connect) against the embedded dist —
  catches Monaco CDN / MonacoView mount / SSE-stall breakage unit mocks miss.
- **G2.** Kernel per-package tests are dense but boundary tests (controlplane↔webui↔restapi↔
  openaiapi↔acp↔agentgw) are sparse. Add `kernel/integration_test/` HTTP-level tests per surface,
  target >60% boundary coverage.
- **G3.** No snapshot tests. Add vitest snapshots for `lib/markdown.ts` (one fixture per token)
  and `lib/format.ts`.

---

## H. Documentation

- **H1.** `docs/ARCHITECTURAL-REPORT.md` self-flags as stale ("phase M781+"). Regenerate or drop the note.
- **H2.** `README.md:19` links `docs/SYSTEM-REVIEW.md` which does not exist. Fix the link or add a stub.
- **H3.** `docs/AGENT-LOOP-INVARIANTS.md` invariants are enforced by `kernel/runtime/runtime.go`
  + `kernel/roster/roster.go` but not cross-referenced. Add 3-line package doc links at the top
  of each source file.

---

## Suggested sequencing

1. **C2 + C3** — finish lib/views split + extend cursorPager (mechanical, safe).
2. **C4** — Chat decomposition (highest UX impact; view-shell adapter).
3. **A2** — paginate remaining 12 endpoints (Go-side first).
4. **A1 + A3** — split controlplane into domain dispatchers; extract `kernel/httpserver`.
5. **B1** — move `runtime/*tool.go` to plugin homes (one file/PR).
6. **B6** — merge `streamlimit` into `governor`.
7. **B5** — extract `kernel/auth` (touches every HTTP surface; coordinate).
8. **A4** — de-function-ize `cmd/agezt/main.go`.
9. **D1 + D2** — refactor `cmd/agt` (easier after A1).
10. **H1 + H2** — docs drift (cheap).
11. **F2** — pnpm vs npm (owner decision).
12. **G1 + G2** — boundary integration tests.
13. **B7–B11** — finish seams already started.
14. **B2 + B3 + B4** — package consolidation (low risk).
15. **E1 + E2 + C5–C9 + G3** — polish.

Every step has an existing verification gate: `tsc --noEmit`, `vitest run`, `go build`,
`go vet`, `go test`, `npm run build`, `tools/depscheck`, `tools/sdkparity`,
`tools/deadcodecheck`, `make check`. No step requires a new gate.
