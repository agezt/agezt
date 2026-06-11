# PHASE M862 — Overseer supervisory dashboard

**Status:** shipped
**Milestone:** M862 (numbered to stay clear of concurrent in-progress
M858/M859 work in the tree).
**Theme:** A single read-only supervisory screen — the "brain that watches"
surface (#46 overseer / M850) — that folds *what is running now*, *who is on the
roster*, and *who has raised an unanswered call for help* into one view.

## What shipped

A new `Overseer` view (nav: **Agents → Overseer**, eye icon) that aggregates
three existing read routes and refreshes every 5s:

- `/api/runs?limit=200` → the **Active runs** panel (runs whose status is still
  `running`), each with model, iteration count, start time, and a sub-agent tag
  when the run was delegated (`parent_correlation`).
- `/api/agents` → an **enabled / total** roster headline (retired agents excluded).
- `/api/board/help` → the **Needs attention** panel: open (unanswered) help
  requests and broadcasts agents have raised, newest first.

Three headline stat cards (active runs, agents, open help) tint to accent/amber
when non-zero so the operator can triage at a glance.

## Why this milestone is conflict-free

Purely frontend. It touches **only** `frontend/src/views/Overseer.tsx` (new),
`frontend/src/App.tsx` (one import + one nav entry + the `Eye` icon import), and
the rebuilt `kernel/webui/dist` bundle. It adds **no new control-plane route** —
it reuses routes that already exist and are already in the `TestAPIReadOnly`
read-only allowlist — so no Go file changes, and the concurrent session's
in-flight kernel edits (M858/M859: parallel tools, async delegation, governor
retry, drain) are left completely untouched.

## Verification

- **Frontend gate:** `tsc --noEmit` clean; `vite build` emits the dist (1855
  modules, committed LF via `.gitattributes`). The view reuses proven primitives
  (`getJSON`, `Button`, `SkeletonList`, `Muted`/`ErrorText`) and the same poll
  pattern as `Board.tsx`.
- No Go change → the webui route guard and the contested kernel packages were
  not compiled. Full `go build ./...` was deliberately skipped (it would compile
  the concurrent in-progress Go edits).
- Each route call is individually `.catch`-guarded to an empty result, so a
  single failing endpoint degrades one panel instead of blanking the dashboard.

## Notes
- Read-only by design: the Overseer watches, it never mutates. Intervention
  controls (stop/modify a run) already live in their own views; this screen is
  the at-a-glance triage layer above them.
