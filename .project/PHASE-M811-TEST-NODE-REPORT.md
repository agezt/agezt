# Phase M811 — test this node (single-node probe on the canvas)

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** production-grade
workflows #4 — n8n's "execute node": probe ONE node with mock upstream
data before trusting the whole pipeline with it.

## What

**kernel/runtime/workflowtestnode.go**: `TestWorkflowNode(ctx, corr, w,
nodeID, data, payload)` — runs one node of a validated graph against a
caller-supplied data map (upstream node ids → their {"output": …}
shapes; the trigger's payload always resolves). The node executes under
the FULL run machinery — policy gates, M808 reliability settings,
governed tools, metered LLM calls — only the graph traversal is
skipped. Triggers refuse ("run the workflow instead"); unknown nodes
and invalid graphs are honest errors. The journal gets a workflow.node
event flagged **test:true**: the live canvas reacts (status ring +
Last-run card fill in), while the run-history fold SKIPS it — a probe
is not an arc.

**Surfaces**: controlplane `workflow_test_node` {workflow (the POSTED
canvas graph, unsaved edits included), node, data?, payload?} → sync,
3m outer cap (the node's own timeout_sec applies inside); webui
`POST /api/workflows/test_node`; **NodePanel "Test this node"** —
an upstream-data JSON textarea + Test button; the result lands in the
Last-run card via the SSE event the probe publishes, plus a toast with
the fired port / attempt count.

## Tests (1 engine + wire + 1 vitest; 492 vitest, full battery green)

- Engine: a transform probed with mock {fetch.output.status} resolves
  templates from the mock ("got 200 for ersin"); trigger and ghost-node
  refused; a DENIED tool still refuses inside a probe (policy holds);
  the journal event carries test:true + the output snippet
- Wire: probe returns the node's real output; run history count is
  IDENTICAL before and after the probe (the fold exclusion, pinned)
- vitest: selecting a node, filling Test data, clicking Test posts
  {workflow (canvas graph), node, data} to /api/workflows/test_node

## Smoke (isolated AGEZT_HOME, real daemon, REAL provider)

Built fetch→llm workflow; selected the **LLM node alone**, fed mock
upstream data ({"fetch":{"output":{"body":"AGEZT workflow motoru artık
node-bazında test edilebiliyor…"}}}), clicked Test node → the REAL
provider summarized it, the node turned green, and the Last-run card
showed the exact interpolated prompt as input and the LLM's Turkish
summary as output. `agt workflow runs probe-me` → "no runs recorded" —
the probe stayed out of history. 0 console errors.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 492; dist rebuilt LF; go.mod unchanged; no
new env vars.

## PRODUCTION-GRADE WORKFLOW ARC COMPLETE (M808–M811)

Per-node retry/timeout + data inspection · webhook trigger · async runs
· single-node test. The owner's bar ("aşırı detaylı ve gerçekten
çalışıyor") is met end to end: nodes are resilient, inspectable,
individually testable; workflows start from cron, events, hand, or any
external system; long runs never die at the wire.

## Remaining backlog (all optional)

Webhook reply mode (sync response with outputs); provider embeddings
opt-in (memory); forge promotion queue; alert per-channel routing.
Owner-gated: CI billing → green badge → v1.0.0 tag.
