# AGEZT System-Wide Review Report

Generated: 2026-06-21  
Branch: `main`  
Scope: repository structure, backend kernel/CLI, REST/API/SDK surfaces, frontend Web UI, docs/governance, validation gates, and current working-tree state.

## Executive summary

AGEZT is an active Go-first agentic operating system with a large kernel surface, CLI (`agt`), daemon (`agezt`), embedded React Web UI, plugin ecosystem, SDKs, operational docs, and runnable demos. This file is the explicit latest system-wide review artifact, last updated 2026-06-22 after missing-parts reconciliation and a fresh local validation gate.

The codebase has dirty working-tree state bucketed by ownership in `docs/MISSING-PARTS-PLAN.md` Phase 0. Fresh validation passed on 2026-06-22; remaining work is release hygiene (scoped commits/discards).

## Current repository status

Observed via `git status` on 2026-06-22:

- Modified: `Makefile`, `dev.ps1`, docs (MISSING-PARTS-PLAN/REPORT, SDK-PARITY, SYSTEM-REVIEW, OPERATIONS), frontend Agent/analytics/UI/test files, `tools/sdkparity/main.go`, and generated Web UI dist assets.
- Untracked tests: frontend missing-smoke/imports/API/ACP/agent/market/AgentPage tests, `kernel/intervention/intervention_test.go`, `kernel/restapi/update_handlers_test.go`, and `tools/sdkparity/main_test.go`.
- Ownership buckets and commit guidance are documented in `docs/MISSING-PARTS-PLAN.md` Phase 0.

## Existing report inventory

- `ARCHITECTURAL-REPORT.md` exists and is a large monorepo architecture report.
- Its header says generated on 2026-06-20 and latest phase M781+.
- Its current-state section still references 2026-06-10, M781, PR #224, and older test counts.
- Later commits and mailbox status indicate additional work after M781, including AgentDetail comms lineage, protocol versioning, SDK parity, docs/index, comparison priorities, and frontend auth hardening.
- Therefore `ARCHITECTURAL-REPORT.md` is useful background but partially stale; it now carries a current-state note pointing to this file and the missing-parts docs.

## System surface reviewed

### Backend and kernel

The backend is organized around `cmd/agezt`, `cmd/agt`, and many `kernel/*` packages. Major implemented areas visible in the tree include runtime, agent loop, governor, edict policy, scheduler, planner, approval, anomaly, memory, world model, skill/Forge, pulse, cadence, standing orders, channels, webhooks, catalog, OpenAI-compatible API, REST API, control plane, ACP, warden, credentials, netguard, redaction, plugins, tenancy, settings, artifacts, tunnel, webui embedding, workflow, and update handling.

The Go module currently declares Go `1.26.4` and direct dependencies on BLAKE3, coder/websocket, go-imap, and btcec, with a small indirect graph. This matches the repository’s stdlib-first posture more closely than older “single external dep” statements in stale report sections.

### CLI and daemon

The CLI surface is extensive under `cmd/agt`, covering run/status/budget/cache/provider/vault/journal/why/edict/tenant/plugin/skill/schedule/world/workflow and related commands. Recent commit history also shows `agt agent authority --explain`, skill hygiene, and world audit commands.

The Makefile now includes check gates for code generation, vet, Go tests, depscheck, SDK parity, and frontend tests. On this Windows environment, prior mailbox status says `make` itself was unavailable, so equivalent commands were run manually.

### REST/API/SDK

`docs/API-STABILITY.md` defines the stability model. It classifies REST `/api/v1/*` SDK surface as beta, Web UI private APIs as internal, OpenAI-compatible `/v1/*` as a compatibility target, control-plane as internal, plugin protocol and registries as beta, and SDK APIs as beta.

`docs/SDK-PARITY.md` is generated/checkable via `go run ./tools/sdkparity -check docs/SDK-PARITY.md`. It currently tracks `/api/v1` route-string coverage and explicitly says it is not behavioral conformance. Current summary says Python, TypeScript, and Rust cover 9/9 SDK-intended REST routes, while Go is n/a for route-string coverage because it uses the local control-plane protocol. `/api/v1/update` and `/api/v1/update/apply` are intentionally unsupported in SDKs as admin self-update endpoints.

### Frontend Web UI

The frontend is a Vite/React SPA under `frontend/`. Current package versions include React `^19.2.7`, TypeScript `^6.0.3`, Vite `^8.0.16`, Vitest `^4.1.8`, and an `undici` override `^7.28.0`.

Modified frontend files show recent AgentDetail/AgentPage simplification and test coverage, Dashboard/Activity/Insights analytics additions, ConfigCenter test stabilization, and missing coverage smoke tests. Mailbox reports indicate frontend vitest eventually passed after timeout adjustments, but the worktree still contains uncommitted frontend changes.

### Security and governance

Security/governance documentation exists in `docs/THREAT-MODEL.md`, `docs/PLUGIN-SECURITY.md`, `docs/API-STABILITY.md`, and `docs/OPERATIONS.md`, with a docs index at `docs/index.md`. The system includes policy/edict, trust ceilings, BLAKE3 plugin pinning, plugin allowlists, journal auditability, token/header guidance, tenant isolation, netguard, redaction, and vault support.

A previous security scan reported frontend token-in-query issues and undici advisory exposure. Later mailbox statuses say the frontend auth issue was fixed by moving fetch-based calls to bearer headers and retaining query token fallback only where EventSource requires it; npm audit was reported clean after the undici override. Those fixes appear in recent commits and frontend package changes.

### Documentation and demos

`docs/index.md` now links comparison, architecture, architectural report, threat model, plugin security, dependencies, operations, connect, console, API stability, SDK parity, and runnable positioning demos.

Runnable demos exist under `examples/autonomous/` for:

- `policy-denial-audit`
- `mailbox-delegation`
- `typed-schedule-system-task`
- `plugin-governance`

Mailbox statuses describe these demos as committed by another agent. They are part of the current tree.

## Validation status

Reported by mailbox from other agents:

- Codegen no drift: passed.
- `go vet ./...`: passed.
- `depscheck`: passed.
- SDK parity check: passed.
- Go tests: split package runs passed, then warm-cache `go test ./...` passed.
- Frontend vitest: eventually passed after ConfigCenter save test stabilization and timeout increases.
- Frontend typecheck: passed for focused changes.
- Frontend npm audit: reported 0 vulnerabilities after undici override.

Fresh local validation gate was run on 2026-06-22 (see `docs/MISSING-PARTS-REPORT.md` item 13 for full command list and results). The gate is no longer mailbox-only evidence.

## Key findings

### 1. System-wide review artifact now exists

The mailbox contained “System-wide review completed” with “No files changed.” A full architecture report existed, but no new report artifact captured that review. This file serves as the explicit latest review artifact and is updated as the audit progresses.

### 2. `ARCHITECTURAL-REPORT.md` is partially stale

The existing architecture report references M781 and older current-state/test-count data. It now carries a prominent current-state note pointing to this file and the missing-parts docs. Regenerate it only when a fresh comprehensive architecture sweep is desired.

### 3. Working tree contains uncommitted changes

The shared working tree has modified and untracked files bucketed by ownership in `docs/MISSING-PARTS-PLAN.md` Phase 0. Commits should use explicit path staging per bucket to avoid capturing unrelated work.

### 4. SDK parity separates route coverage from behavioral evidence

`docs/SDK-PARITY.md` is now generated by `tools/sdkparity` and lists both static route-string coverage and behavioral test evidence per SDK language.

### 5. Frontend coverage and UX work is validated

AgentDetail/AgentPage, Dashboard, Activity, Insights, ConfigCenter, and missing smoke/import tests are validated. The compact inbox priority summary is shipped. Fresh frontend test run: 138 files / 1194 tests green.

### 6. Fresh local validation gate passed

Full Makefile `check` equivalent passed on 2026-06-22: codegen, vet, depscheck, SDK parity, frontend tests/typecheck/build, and `go test ./...`.

## Recommended next actions

1. `docs/SYSTEM-REVIEW.md` is the latest review artifact; regenerate `ARCHITECTURAL-REPORT.md` only if a fresh comprehensive architecture sweep is desired.
2. Commit or discard dirty-worktree buckets using explicit path staging (see `docs/MISSING-PARTS-PLAN.md` Phase 0 ownership snapshot).
3. Preserve the fresh validation results or rerun the gate if the tree changes before release/merge.

## Appendix: files that look related to current review state

- `ARCHITECTURAL-REPORT.md` — existing broad architecture report, partially stale.
- `docs/SYSTEM-REVIEW.md` — this explicit latest review artifact.
- `docs/index.md` — documentation index.
- `docs/API-STABILITY.md` — public/internal API stability policy.
- `docs/SDK-PARITY.md` — generated SDK route coverage report.
- `DEPENDENCIES.md` — dependency inventory.
- `Makefile` — check gate definitions.
- `tools/sdkparity/main.go` — SDK parity checker.
- `frontend/package.json` — current frontend stack and undici override.
