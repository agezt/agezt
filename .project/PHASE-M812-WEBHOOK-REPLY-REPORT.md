# Phase M812 — webhook reply mode

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** the last workflow
backlog item — n8n's "respond to webhook": the caller waits and receives
the run's outputs.

## What

- **TriggerConfig gains `reply: true`** (webhook kind): the external POST
  holds until the run finishes and the response carries the outputs.
  Declarative, per-workflow — request/response workflows opt in; long
  pipelines stay async.
- **Control plane**: in reply mode the authenticated call runs the
  workflow SYNC under a 2-minute cap (`workflowWebhookReplyTimeout` —
  reply workflows are request/response, not pipelines) and returns
  {correlation_id, executed, outputs}. Post-auth run failures are HONEST
  errors with the correlation (the caller proved knowledge of the
  secret); only the auth gate stays uniform.
- **webui /hooks/<name>**: a result carrying outputs answers **200**
  {ok, outputs, executed, correlation_id}; the async shape keeps its
  202; a post-auth run failure maps to **502** with the message; auth
  refusals stay the uniform 403. The handler ctx is widened to 130s so
  sync runs aren't cut by the relay.
- **Canvas**: the trigger panel gains a bool "Reply mode" field (new
  `bool` field kind — renders false/true, true rides as a real boolean,
  false stays omitted). The copilot contract teaches `reply`.

## Tests (wire + webui; 492 vitest, full battery green)

- Wire: a reply-mode workflow returns outputs in-line ("pong 42") and
  NOT the async accept shape
- webui: outputs → 200 with ok+outputs; post-auth run failure → 502
  with the message; auth refusal still uniform 403; async still 202
- Full Go suite, vet, staticcheck, linux cross-build green

## Smoke (isolated AGEZT_HOME, real daemon, real curl)

A BRANCHING reply workflow (condition on body.n): `curl -d '{"n":9}'` →
200 with `outputs.big = {"verdict":"BIG","n":9}` and executed
[start,check,big]; `-d '{"n":2}'` → the small branch's outputs. Wrong
secret stayed a uniform 403. The caller sees exactly which branch ran
and what it produced — request/response automation, governed end to end.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 492; dist rebuilt LF; go.mod unchanged; no
new env vars.

## WORKFLOW SURFACE: NOTHING LEFT ON THE BACKLOG

M798–M812: engine · triggers (manual/cron/event/webhook±reply) · 14-node
library · canvas · copilot draft+refine · agent tool · run history ·
templates · per-node reliability + data inspection · async runs ·
single-node test. Remaining project backlogs are outside workflows
(provider embeddings opt-in, forge promotion queue, alert routing) plus
the owner-gated CI/billing items.
