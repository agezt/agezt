# Phase M798 — workflow engine core (n8n-style typed-node graphs)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow engine, step 1
(owner ask: "n8n'den hallice", React Flow canvas, copilot-designed schemas,
internal-tool access, cron/event/manual starts).

Arc plan ratified in chat: M798 core engine → M799 triggers (cron/event) →
M800 node library (http/code/forEach/switch/parallel/merge/approval/
subworkflow/notify/error branch) → M801 React Flow canvas editor →
M802 copilot + agent `workflow` tool.

## What

`kernel/workflow`: durable, named graphs of TYPED nodes wired by edges —
unlike kernel/planner (intent → agent-loop per node), a workflow node is a
precise deterministic step. M798 node set: **trigger** (manual), **tool**
(one governed tool call: {tool, args}), **llm** (one completion: {prompt,
system, model}), **condition** ({left, op, right} → true/false ports),
**transform** ({template}), **delay** ({seconds ≤600}).

Data flows with **{{path}} templates** (kernel/workflow/template.go):
{{trigger.payload.city}}, {{node.output}}, deep JSON paths with array
indexes ({{fetch.output.items.0.title}}). Tool/transform outputs that parse
as JSON stay STRUCTURED so downstream templates reach into them; unknown
paths render "" (debuggable, never fatal).

## Engine (kernel/runtime/workflowrun.go)

Token-flow over the validated DAG: trigger fires, each completing node
fires its outgoing edges (a condition fires exactly one port), a node runs
once on its first token. Governance is inherited, not reinvented:

- **tool nodes pass k.policyHook** — the SAME Edict gate as agent-loop
  calls (deny refuses the node; ask blocks on the approval registry), and
  they see the full dynamic tool map (built-ins + forge_* + mcp_*).
- **llm nodes ride the Governor** (TaskType "workflow" → routable +
  metered like any other class).
- Every run journals workflow.started → workflow.node (ok/error/port) →
  workflow.completed|failed under subject workflow.<name> — the M801
  canvas will replay these live.
- Halted kernel refuses; ctx cancellation honored between/inside nodes;
  256-step defense cap atop cycle-free validation.

## Validation (the canvas's contract)

Exactly one trigger; unique ids; known types; edges resolve; condition
edges need port true/false, others default-port only; trigger has no
incoming edges; acyclic (Kahn); per-type config checks; ≤100 nodes,
≤300 edges, ≤32KiB config. Store.Save validates BEFORE disk and upserts
by name wholesale (ID/CreatedMS/Enabled survive an update).

## Surfaces

controlplane workflow_list/show/save/remove/set_enabled/run (run is
synchronous, 15m bound, returns {executed, outputs, correlation_id});
`agt workflow list|show|save --file|run --payload|enable|disable|remove`;
webui routes (GET /api/workflows + /show readArgs; save/run jsonRoutes;
enable/remove writeRoutes) — API parity ready for the M801 canvas.

## Tests (10 new across 3 packages)

- workflow: Validate accept + 17-case reject table; Interpolate/Lookup
  table (deep paths, arrays, objects→JSON, misses, dangling braces);
  store upsert-by-name (identity+enabled survive; invalid never touches
  disk); persistence round-trip.
- runtime e2e: linear data flow (payload→tool args→parsed JSON→transform→
  llm prompt, journal arc started/4×node/completed asserted); condition
  branching both ways (untaken branch never runs); policy gates tool nodes
  (default-deny, tool never invoked); tool failure fails the run
  (downstream never runs).
- controlplane wire round-trip: save/validate-refusal/list-stays-light/
  show-full-graph/run-with-payload/disable/remove/ghost refs.

## Smoke (isolated AGEZT_HOME, real daemon)

`agt workflow save --file flow.json` (6 nodes: trigger→transform→condition
→{delay→win | lose}) → run with score 80: TRUE branch, 1s delay, output
"KAZANDIN — merhaba ersin, skor: 80" → run with score 20: FALSE branch,
"bir dahaki sefere…". Both runs listed per-node outputs with correlations.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; frontend untouched (canvas is M801; dist unchanged);
go.mod unchanged; no new env vars. CI org-billing still blocked → local
battery + arc-authority merge.

## Next

M799: triggers — the trigger node gains {kind: manual|cron|event} config;
a workflow trigger runner (cadence + standing-runner patterns) fires
enabled workflows with the event payload as {{trigger.payload}}.
