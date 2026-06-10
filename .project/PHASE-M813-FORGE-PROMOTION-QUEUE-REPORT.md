# Phase M813 — forge promotion queue

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** forge polish — the
agent can now ASK for its tool to go live; the operator decides from the
same approval surface as everything else.

## What

M794 deliberately made promotion operator-only (CLI/console) — but the
agent couldn't even request it; the loop ended with "ask the operator"
prose. M813 closes the loop with the HITL registry:

- **kernel/runtime `RequestToolPromotion(ctx, corr, ref)`** — checks the
  invariants UP FRONT (exists, not already active, **TestedOK** — an
  untested draft never reaches the operator's queue, preserving "only
  tested code goes live"), then blocks on `approvals.Submit` (capability
  `toolforge.promote`, input carries name/language/description). A grant
  promotes through the EXACT path the operator CLI uses
  (PromoteScriptTool → scripttool.promoted journaled); a deny/timeout
  returns the decision + reason as a verdict, not an error.
- **tool_forge `op=request_promotion {ref}`** — granted → "promoted —
  callable as forge_<name> from the next run"; denied → an error result
  carrying the operator's reason ("improve the tool or move on");
  timeout → "the operator did not decide in time". The op=test verdict
  now points at request_promotion. Edict: rides the existing
  CapToolForge mapping (the registry is the real gate).
- The request appears in `agt approvals` and the console's Approvals
  view automatically — zero new surfaces, the queue IS the registry.

## Tests (runtime e2e + tool suite; full battery green)

- Runtime, REAL registry: deny → verdict+reason back, draft stays draft;
  grant → ACTIVE via the operator path; untested refused with
  ErrUntested BEFORE any registry submit; already-active and ghost
  refused up front
- forgetool: denied → IsError with the operator's reason; granted →
  forge_<name> message + ACTIVE; untested → "no passing test"; missing
  ref refused
- 492 vitest; full Go suite, vet, staticcheck, linux cross-build green

## Smoke (isolated AGEZT_HOME, real daemon, REAL provider MiniMax-M2)

One `agt run`: the real agent drafted a python `slugify` tool, tested it
in the real sandbox ("Merhaba Dunya 42"), then requested promotion and
BLOCKED. `agt approvals` showed "promote slugify (python): Convert a
text string into a URL-friendly slug format"; `agt approve … "test
gecti, lgtm"` released it; the agent reported "The tool is now active
and callable as forge_slugify". `agt toolforge list` → ACTIVE; a FRESH
run then called `forge_slugify` successfully. The whole loop — author,
test, request, human gate, go-live, use — in one session.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 492 (frontend untouched); go.mod unchanged;
no new env vars; no new event kinds (the registry's approval.* arc
already covers the queue).

## Next

Remaining optional backlog: alert per-channel routing + mute window;
provider embeddings opt-in (needs an embeddings-capable keyed provider —
verify before building). Owner-gated: CI billing → v1.0.0.
