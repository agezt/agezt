# Phase M808 — workflow reliability + per-node data inspection

**Date:** 2026-06-10 · **Status:** DONE · **Trigger:** owner verdict —
"workflows çok amatör kaldı… gerçekten çalışıyor olması en önemlisi."
This milestone is the production-grade answer: per-node retry/timeout
semantics and n8n-level data inspectability.

## What

**Per-node reliability (kernel/workflow + engine).** Node grows three
settings OUTSIDE config: `timeout_sec` (1..600 — bounds ONE attempt; any
non-trigger node), `retries` (0..5) and `retry_delay_sec` (0..60 —
failable nodes only; the validator refuses retries on a transform).
Engine (`execNodeWithReliability`): each attempt gets its own deadline;
a timed-out attempt fails with a NAMED error ("node timeout after Ns" —
not a bare context message); failable nodes re-run with the configured
pause; a cancelled RUN never retries; **the error port fires only after
retries are exhausted** (rescue is the last resort, not the first).
The journal records `attempts` when >1.

**Per-node data on the wire.** Every `workflow.node` event now carries
the node's resolved INPUT preview (interpolated args/prompt/method+url/
items/switch-value — `nodeInputPreview`, preview-only) and its OUTPUT
snippet (strings verbatim, else compact JSON; truncated at 2000 runes
with `output_truncated`). The `workflow_runs` fold passes input/output/
attempts through, so history is as inspectable as live.

**Console.** NodePanel grows a **Reliability** section (timeout for any
working node; retries+delay for failable ones; values round-trip through
toFlow/fromFlow as top-level node fields) and a **Last run** card: status
(ok/rescued/failed), fired port, attempt count, input and output as
scrollable pre blocks, error text — fed by live SSE events AND by
history replay. The copilot contract now teaches the reliability fields,
so "retry that flaky fetch twice" works in plain language.

## Tests (4 engine suites + 2 vitest; 490 vitest, full battery green)

- Retry recovers: flaky tool fails twice, retries=2 → run completes,
  tool called exactly 3×, journal event attempts=3 + output snippet
- Exhaustion then rescue: retries=1, always-failing tool → exactly 2
  calls, THEN the error port rescues with the message flowing
- Timeout bounds one attempt: a blocking tool with timeout_sec=1 fails
  in ~1s with the named timeout error (asserted under 10s, not 15m)
- Validator bounds: retries>5 / timeout>600 / delay>60 / retries on a
  non-failable node all refused; legal settings pass
- vitest: reliability settings round-trip losslessly through
  toFlow/fromFlow (unset stays absent — no zero-noise in saved graphs)

## Smoke (isolated AGEZT_HOME, real daemon, real python sandbox)

One workflow, two failure modes, all governed:
- http node (egress-denied URL, retries=2, timeout 5s) → **3 attempts**
  in the journal, error port rescued the run
- code node `time.sleep(30)` with timeout_sec=2 → **killed at 2s**
  ("node timeout after 2s"), its error port rescued too
- whole run: 5 nodes, completed in **2.07s wall clock** (the old
  behaviour would have hung 2 minutes on the sandbox default)
- Browser: replayed the run from history, clicked the fetch node —
  Last run panel showed "rescued · port error · 3 attempts · input GET
  http://127.0.0.1:1/x · output {error…}", and the Reliability fields
  showed the saved retries=2. 0 console errors.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 490; dist rebuilt LF; go.mod unchanged; no
new env vars. Memory updated: [[workflow-production-grade]] — the bar is
operational depth, not breadth.

## Next (production-grade backlog, in owner-priority order)

1. **Webhook trigger** — external HTTP POST starts a workflow (n8n's
   bread-and-butter entry point; we have cron/event/manual only).
2. Async runs — long workflows shouldn't hold a wire connection.
3. Per-node "test this node" on the canvas.
