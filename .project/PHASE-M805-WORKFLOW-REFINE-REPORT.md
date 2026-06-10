# Phase M805 — workflow copilot refine

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow polish #1.

## What — "change X" on an existing graph

M802's copilot designs from scratch; M805 closes the loop: **refine** an
existing workflow with a plain-language change request.

- **kernel/runtime/workflowdraft.go**: `RefineWorkflow(ctx, corr, base,
  instruction)` — the provider sees the CURRENT graph JSON (the canvas's
  truth, unsaved edits included) plus the instruction, under the same
  node-type contract, told to return the COMPLETE revision keeping
  unrelated nodes/ids/positions unchanged. The base's name always wins;
  base must validate (garbage in refused before any provider call). The
  shared `draftLoop` (refactored out of DraftWorkflow) does the
  two-attempt repair conversation and journals `workflow.drafted` with
  `mode: draft|refine`. Positions survive (auto-layout only fires on a
  fully position-less graph), so a refinement doesn't scramble the canvas.
- **Surfaces**: controlplane `workflow_refine` {instruction, workflow?
  (posted graph) | ref? (stored)}, `agt workflow refine <ref> "CHANGE"
  [--save]`, `POST /api/workflows/refine`.
- **Console**: the Copilot panel grows a mode — when the canvas holds
  more than the trigger, **"Refine canvas"** (primary) posts the current
  canvas graph + instruction; "Draft fresh" stays available. Trigger-only
  canvas keeps the original draft-only panel. Result replaces the canvas
  UNSAVED, as ever.
- **Drive-by honesty fix**: `agt workflow draft --save` claimed
  "saved (disabled)" but the store creates new workflows ENABLED — the
  message now reads the persisted flag ("enabled — triggers armed").

## Tests (2 new Go suites + 2 vitest; full battery green)

- RefineWorkflow happy path: prompt carries base graph + instruction;
  revision keeps the base's name and the model's positions; not saved
- Refusals: empty instruction / invalid base → zero provider calls
- CopilotPanel refine mode: posts {workflow, instruction} to
  /api/workflows/refine and hands the revision back
- Trigger-only canvas hides Refine
- (485 vitest total; full Go suite, vet, staticcheck, linux build green)

## Smoke (isolated AGEZT_HOME, real daemon, REAL provider MiniMax-M2)

- CLI: drafted "greeter" (TR description), then `agt workflow refine
  greeter "selamlamadan önce bir onay adımı ekle…" --save` → the real
  LLM inserted the approval node between trigger and transform, exact
  Turkish description preserved ("selamlansın mı?"), Turkish labels.
- Browser: opened greeter on the canvas → Copilot → "Refine canvas"
  with a Turkish instruction → a new `poetic_greet` LLM node appeared
  after the transform, existing ids/positions kept → Save → CLI shows
  4 node(s)/3 edge(s). 0 console errors.
- Live HITL bonus: a (human-clicked) Save & Run blocked on the approval
  node; `agt approve` released it — the gate works end-to-end on a
  copilot-built graph.
- Headed-browser note (M797 prior confirmed again): the owner's desktop
  clicks raced the script mid-smoke (a stray node + a run appeared);
  recovered by reloading and running the whole flow in one script.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 485; dist rebuilt LF; go.mod unchanged; no
new env vars.

## Next

Workflow polish #2: per-workflow run history (replay past runs on the
canvas from the journal). Then templates gallery; provider embeddings
opt-in on the memory side.
