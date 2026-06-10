# Phase M800 — workflow node library

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** workflow engine, step 3.

## What — 8 new node types + the error port

- **http** {method GET|POST, url, headers?, body?} — rides the REGISTERED
  http tool: its host allowlist / egress guard and the CapHTTPGet/Post
  policy mapping apply exactly as for an agent's own call.
- **code** {language, code, input?} — the M794 sandbox runner; interpolated
  input lands as ./stdin.txt; gated by the same code.exec policy axis.
- **map** {items, template} / **filter** {items, left, op, right} —
  per-item templates with {{item}}, {{item.field}}, {{index}}; items
  accepts "{{a.output.list}}" or a bare path; 1000-element cap.
- **switch** {value, cases:[{equals, port}]} — multi-way branch; declared
  case ports + "default"; edges from a switch must use declared ports
  (validated), case ports can't squat ""/default/error.
- **merge** {mode all|any} — join: "any" runs on the first incoming token
  (the M798 default), "all" waits for a token on EVERY incoming edge
  (token counting in the engine); output = upstream outputs in edge order.
- **approval** {description, capability?} — the HITL gate: blocks on the
  approval registry (`agt approvals` / channels / console); grant
  continues, deny/timeout fails the node.
- **subworkflow** {workflow, payload?} — runs another stored workflow
  (payload template → its {{trigger.payload}}, output = {executed,
  outputs}); nesting depth-capped at 3 (a self-recursive workflow is
  refused, not run forever).
- **error port** — failable nodes (tool/llm/http/code/approval/subworkflow)
  may wire port "error": on failure the run SURVIVES, the message lands in
  {{node.output.error}}, and the error branch runs instead of the default
  one. The workflow.node journal event carries ok=false + handled=true.

Engine: token counting per node (merge-all readiness re-checked on every
arriving token); invokeWorkflowTool shared by tool/http nodes (one policy
gate, one dynamic-map lookup).

## Tests (6 engine e2e + 14 validation cases, all packages green)

- map+filter+switch+merge-all composition (untaken switch branch never
  runs; merge collected both branches in edge order)
- error port rescues the run; {{call.output.error}} flows; default branch
  suppressed
- code node via stub runner (interpolated input, structured output)
- http node bridges the registered tool (method uppercased, url templated)
- approval gate granted via the real registry (pending → Resolve →
  "granted by tester" flows downstream)
- subworkflow output bubbling + ouroboros depth-cap refusal
- validation: per-type config rejects, switch undeclared-edge-port, error
  port on non-failable nodes rejected

## Smoke (isolated AGEZT_HOME, real daemon, real python sandbox)

10-node "triage" pipeline in one run: filter kept sev>2 tickets, map
shaped titles, switch took the "ops" port (dev/fallback never executed),
merge-all combined both branches, then a deliberately broken python code
node FAILED in the real sandbox and the error port rescued the run —
"kurtarıldı: code failed: …SyntaxError…". 8/10 nodes executed, per-node
outputs printed with the correlation.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; frontend untouched; go.mod unchanged; no new env
vars. CI org-billing still blocked → local battery + arc-authority merge.

## Next

M801: the React Flow canvas editor — node palette, drag-and-connect,
side-panel config, run with live node status from the workflow.* arc.
