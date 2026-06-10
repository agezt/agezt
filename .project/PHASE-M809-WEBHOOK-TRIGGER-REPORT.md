# Phase M809 — webhook trigger

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** production-grade
workflows #2 (after M808 reliability) — n8n's bread-and-butter entry
point: external systems start workflows over HTTP.

## What

**Trigger kind "webhook"** (kernel/workflow): TriggerConfig gains
`secret`; validation requires ≥12 chars — a short secret is a refused
config, not a silently-accepted risk. The trigger runner ignores the
kind (it's passive); the daemon banner counts armed webhooks.

**The fire path, two layers:**
- **controlplane `workflow_webhook`** {ref, secret, payload} — THE
  single source of truth for the gate: workflow exists + ENABLED +
  trigger kind webhook + constant-time secret compare. Refusals are
  deliberately uniform ("webhook refused") so a prober cannot
  distinguish unknown-name / bad-secret / disabled. On accept the run
  fires **async** (own 15m deadline; failures land in workflow.failed)
  and {accepted, correlation_id} returns immediately.
- **webui `POST /hooks/<name>`** — the ONE deliberately
  console-token-free path (mounted with `secure`, not `auth`): the
  per-workflow secret (X-Agezt-Secret header, or ?secret= for callers
  that can't set headers) is the auth. All it can ever do is ask "fire
  workflow <name>" — no reads, no other writes. Body capped at 256KB;
  JSON body → `{{trigger.payload.body}}`, query params (minus secret) →
  `{{trigger.payload.query}}`, non-JSON bodies ride verbatim. Responds
  202 + correlation. Refusal maps to a uniform 403; GET is 405.

**Visibility**: list/console rows show "webhook (POST /hooks/<name>)";
the trigger panel gains the secret field with the calling convention in
its label; the copilot contract teaches the kind ("when my CI posts
here…" now drafts correctly); the banner reads "N webhook trigger(s)
armed".

## Tests (wire + webui + 1 vitest; full battery green)

- Wire: webhook workflow saved (short secret refused with the
  validator's reason); right secret → accepted + the ASYNC run's
  workflow.completed observed on the bus; wrong secret / unknown name /
  disabled all uniformly "webhook refused"
- webui: tokenless POST forwards {ref, secret, payload{kind,body,query}}
  to workflow_webhook; the secret never rides into the payload;
  control-plane refusal → uniform 403; GET → 405 with zero
  control-plane calls
- vitest: trigger summarize shows the webhook hint
- 490 vitest; full Go suite, vet, staticcheck, linux build green

## Smoke (isolated AGEZT_HOME, real daemon, real curl)

Saved `deploy-note` (webhook trigger → transform → memory tool).
`curl -X POST /hooks/deploy-note?env=prod -H "X-Agezt-Secret: …"
-d '{"version":"v1.2.3","who":"ersin"}'` → 202 + correlation; the async
run completed in 6ms and the templates resolved BOTH body fields and the
query param into a real memory write: **"deploy v1.2.3 by ersin
(env=prod)"**. Wrong secret → 403; unknown workflow → 403 (uniform);
GET → 405. Restart banner: "1 webhook trigger(s) armed".

## Security notes (SPEC-06 posture)

- The console token never travels to external callers; a leaked hook
  secret can fire exactly one workflow and nothing else.
- Constant-time compare in the control plane; uniform refusals at both
  layers; body size cap; the webhook payload is data, not instructions —
  it meets the same template/policy machinery as every other trigger.
- The path inherits the loopback-by-default bind: exposing hooks to the
  internet is the operator's explicit choice (AGEZT_WEB_ADDR/tunnel).

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 490; dist rebuilt LF; go.mod unchanged; no
new env vars.

## Next (production-grade backlog)

Async run records for LONG manual runs (the webhook path is already
async); per-node "test this node" on the canvas.
