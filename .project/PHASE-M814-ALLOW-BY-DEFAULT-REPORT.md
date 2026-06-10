# Phase M814 — allow-by-default policy posture

**Date:** 2026-06-10 · **Status:** DONE · **Trigger:** owner law —
"default olarak kapatmadıkça her şeye izni var bu agezt içinde her şeyin"
(everything is allowed unless you turn it off).

## What

`edict.DefaultLevels()` now returns **LevelAllow for every capability**
(built from `AllCapabilities()`, so a new capability is allow-by-default
automatically and a forgotten entry can't silently land somewhere
stricter). The pre-M814 ladder mixed Allow/AskFirst/Ask per DECISIONS
F3, but the owner already ran AskPolicy=AskAllow — which folded every
ask to allow in practice; this makes the real posture the explicit,
durable one.

**Restriction is the operator's opt-OUT**, unchanged and fully working:
the Policy center, `agt edict level <cap> <L0..L4>`, `AGEZT_EDICT_DENY`,
and durable overlays all still bind on top of the allow defaults.

**Deliberately NOT relaxed** (these are guards, not permission levels):
- the F4 hard-deny strings (fork bombs, `rm -rf /`, raw-device writes) —
  they bite even when the capability is L4;
- the http/browser SSRF guards (loopback/private-net egress);
- Governor budget ceilings + per-agent daily caps;
- EXPLICIT HITL surfaces the operator wires on purpose — the workflow
  approval node and the forge promotion queue (M813) block on the
  approval registry regardless of capability level.

Banner now reads "edict (allow-by-default — every capability on unless
you opt out; …)".

## Tests (edict + controlplane; full battery green)

- `TestDefaultLevels_MaxAutonomy`: EVERY capability in `AllCapabilities()`
  defaults to LevelAllow (a new/forgotten cap fails here), and an
  explicit override still beats the default
- Updated the three posture-pinned control-plane tests (edict show /
  set-level / set-mode) from the old F3 shell=L2 expectation to the
  L4 default; the set-mode test now opts shell INTO ask-first first to
  exercise AskDeny folding (nothing is ask-class out of the box anymore)
- Full Go suite, vet, staticcheck, linux cross-build green; frontend
  untouched (492 vitest)

## Smoke (isolated AGEZT_HOME, real daemon)

`agt edict test` on shell / file.write / file.delete / http.post /
mcp.install / workflow.manage / config.write → **all allow** out of the
box. Opt-OUT proven: `agt edict level shell L0` → "shell: L4 → L0",
and a probe then **denied**. Hard-deny proven: `rm -rf /` on shell
**denied even at L4**. Banner shows the new posture line.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; vitest 492; go.mod unchanged; no new env vars.
Memory recorded: [[default-allow-posture]].

## Next

Remaining optional backlog: alert per-channel routing + mute window.
Owner-gated: CI billing → v1.0.0.
