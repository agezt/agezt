# Phase M802 — workflow copilot + agent workflow tool

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow engine, step 5 (FINALE).

## What — describe it, see it on the canvas; agents author workflows too

**Copilot (kernel/runtime/workflowdraft.go).** `DraftWorkflow(ctx, corr,
name, description)`: the provider sees the full node-type reference (all
14 types, configs, ports, template syntax, error-port semantics) as a
strict-JSON contract; the kernel extracts the object (fence/prose
tolerant, string-literal-aware brace matching), applies the caller's name
override, **auto-lays-out** the canvas (BFS depth from the trigger = row;
bounded so a cyclic draft can't spin it), and runs the SAME Validate the
save path uses. A draft that fails gets exactly ONE repair round-trip —
the model sees its own output and the exact validation error. The result
is returned **UNSAVED** (journaled `workflow.drafted`): the copilot can
never silently install automation. TaskType "workflow" → routed/metered
with the llm nodes.

**Surfaces.** controlplane `workflow_draft` {description, name?};
`agt workflow draft "DESC" [--name N] [--save]` (prints the JSON for
review; --save persists); webui POST `/api/workflows/draft`; console
**Copilot panel** in the canvas editor (Sparkles toggle): describe →
"Draft onto canvas" → the drafted graph replaces the canvas, node configs
and ports rendered, Save persists.

**Agent workflow tool (plugins/tools/workflowtool).** New `workflow` tool:
op=save (whole graph, upsert), run (payload → {{trigger.payload}},
per-node outputs back), enable, list, show — the SAME store the console
canvas and `agt workflow` edit. New Edict cap **workflow.manage**
(AskFirst; list/show → introspect): saving/arming standing automation is
an L2 autonomy grant like CapStanding. Two safety asymmetries: AGENT
saves arrive **disabled** (console saves stay enabled — an operator
clicked; the tool disarms fresh creates so arming takes an explicit
gated op=enable), and inside a run every tool node re-passes the regular
policy gate, so a workflow can't launder a forbidden call.

## Tests (4 draft + 2 tool suites Go, 3 vitest; 99 pkgs / 483 green)

- draft happy path: prose+fenced answer → validated graph, name override,
  depth-ordered layout, journaled, NOT saved
- repair round: cyclic first answer → exact error + bad answer go back;
  corrected second answer wins (2 provider calls)
- gives-up-after-repair; empty description refused with 0 provider calls
- tool lifecycle: save (arrives disabled) → run (ref+payload flow) →
  enable → list/show on a real store; 7 refusal/validation cases + unbound
- CopilotPanel: posts {description, name} and hands the graph back; empty
  description refused client-side; copilot failure surfaces as error

## Smoke (isolated AGEZT_HOME, real daemon, REAL provider MiniMax-M2)

- CLI: `agt workflow draft "her sabah 09:00'da https://… GET; OK yoksa
  LLM uyarısı…" --name status-watch` → real LLM designed a 5-node
  branching graph (cron 09:00 → http GET → condition contains-OK →
  true/false branches), auto-laid-out, validated.
- Browser: new workflow → Copilot panel → Turkish-adjacent plain-language
  ask → 4-node graph (event trigger task.failed → transform → llm →
  approval) appeared on the canvas → Save → list row shows
  "event (on task.failed)". 0 console errors live.
- Agent: `agt run "Use the workflow tool to save agent-made …, run it"`
  → real agent saved (arrived DISABLED), ran with payload, reported
  "agent says merhaba"; policy.decision journaled (AskFirst folded by the
  owner's AskAllow posture — the gate fired, the posture answered).
- Journal carried workflow.drafted for both copilot drafts.

## Gate

vitest 72 files / 483 green; full `GOMAXPROCS=3 go test -p 2 ./...`
(99 pkgs ok); vet + staticcheck clean; linux cross-build OK; dist rebuilt
LF; go.mod unchanged; no new env vars. CI org-billing blocked → local
battery + arc-authority merge.

## WORKFLOW ENGINE ARC COMPLETE (M798–M802)

Core token-flow engine · cron/event/manual triggers · 14-node library
with error ports · React Flow canvas with live run replay · copilot +
agent authoring. The owner's ask ("n8n'den hallice, copilot'lu,
iç araçlara erişen workflow") is shipped end-to-end.

## Next

Vision gaps: #5 vector memory, #6 brain distiller standing surface.
Workflow polish backlog: copilot refine ("change X" on an existing
graph), per-workflow run history view, workflow templates gallery.
