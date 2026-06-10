# Phase M807 — workflow template gallery

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow polish #3 (arc
backlog complete).

## What — five validated starting points, shipped in the binary

**kernel/workflow/templates.go**: a curated gallery, one template per
engine capability worth learning. Authored in Go (a typo is a compile
error), every entry passes the SAME `Validate` the save path uses —
pinned by test, so schema drift breaks the build, not the user's first
click. Every node positioned (the gallery opens laid-out). The gallery
never writes to the store; instantiation is just a save under a new name.

1. **daily-status-check** (monitor) — cron daily → http GET → condition
   contains-OK → llm alert | all-good.
2. **failed-task-triage** (ops) — event task.failed → transform summary
   → llm fix → approval gate → conclusion.
3. **resilient-fetch** (demo) — http with an **error-port** rescue
   branch; the failure message flows into the fallback.
4. **team-router** (ops) — **switch** on payload.team (+default) → three
   branches → **merge any** → llm brief.
5. **list-pipeline** (data) — **filter** payload.items → **map** per-item
   template → memory tool remembers the digest.

**Surfaces**: controlplane `workflow_templates` (full graphs; a handful
of small entries); `agt workflow templates [--json]` lists,
`--use T --name N` instantiates (plain save; reports the real persisted
state); `GET /api/workflows/templates` (apiRoutes + readOnly guard map);
console **"Start from" picker** in the New-workflow form — pick a
template, the canvas opens on its graph under your name, UNSAVED, with
the template's description shown under the form.

## Tests (2 Go + 1 vitest; full battery green)

- TestTemplatesValidate: every entry validates, metadata complete, slugs
  unique, every node positioned, gallery ≥5
- TestTemplateByName: hit + ghost
- vitest: picker lists the gallery, instantiating opens the canvas under
  the NEW name with the template's nodes/edges (needed a no-op
  ResizeObserver stub — React Flow requires it under jsdom)
- 489 vitest total; full Go suite, vet, staticcheck, linux build green

## Smoke (isolated AGEZT_HOME, real daemon, REAL provider)

- `agt workflow templates` listed all five with categories.
- `--use resilient-fetch --name my-fetch` → ran with a loopback URL: the
  governed http tool REFUSED it (host allowlist — governance inheritance
  proven live) and the **error port rescued the run**: "fetch failed but
  the run survived: … host not in allowlist".
- `--use team-router` → payload team=dev took the dev port, merge joined,
  the real LLM briefed ("Development will deploy the fix.").
- Browser: "Start from" select showed all five; creating
  "status-from-tpl" from daily-status-check opened the canvas with
  5 node(s) · 4 edge(s) laid out. 0 console errors.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 489; dist rebuilt LF; go.mod unchanged; no
new env vars; workflow_templates added to the webui readOnly guard map.

## WORKFLOW POLISH ARC COMPLETE (M805–M807)

Copilot refine · run-history canvas replay · template gallery. Remaining
backlogs: provider embeddings opt-in (memory), forge promotion queue,
alert per-channel routing; owner-gated CI/billing items.
