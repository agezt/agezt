# Phase M795 — Tool Forge console view

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** script-tool forge
(M794 backend → this is the console surface).

## What

`frontend/src/views/Toolforge.tsx` (+ "Tool Forge" nav item under Agents,
Hammer icon): the whole governed script-tool lifecycle from the web UI —
draft a script (language select, description, code with the stdin.txt
contract stated inline, optional input schema), **run a sandbox test from
the row** (sample JSON input → PASS/FAIL badge + raw output), **promote**
(confirm dialog; disabled until a test of the current code passed —
button title says why), **quarantine** (kill switch, confirm), **edit**
(fetches the full record because the list strips code; warns that a code
change demotes), **remove**.

## Backend delta (one line)

`/api/toolforge/show` added to webui readArgsRoutes (GET, ref) — the list
deliberately strips code bodies; the editor fetches one full record on
demand. Read-only, mirrors `/api/standing/why`.

## Exports for tests/reuse (M714 recipe)

`NewToolForm`, `EditToolForm`, pure `toolNameOk` (kernel name rule
mirrored), pure `statusBadge` (active→good, quarantined→bad).

## Tests

8 new vitest (465 total green): name-rule + badge mapping tables; draft
form gates on name+desc+code and posts the wire shape; Promote disabled
for untested drafts while live tools show Quarantine instead; test panel
posts ref+input and renders PASS + output; editor fetches /show, prefills
code, posts the edit; empty state.

## Browser e2e (isolated AGEZT_HOME, real daemon, real Python sandbox)

Via the UI alone: drafted `greet` → row showed draft+untested, Promote
DISABLED → ran a test with `{"name":"ersin"}` → real sandbox PASS,
output "hello ersin" → Promote (confirm) → row active+tested+forge_greet
+ success toast → Quarantine (confirm) → row quarantined. CLI cross-check:
`agt toolforge list` saw QUARANTINED/tested; journal showed
created→tested→promoted→tested→quarantined. **0 console errors during the
live session.** Graceful shutdown, smoke dir removed.

## Gate

Full `GOMAXPROCS=3 go test -p 2 ./...` green; vet + staticcheck clean;
linux cross-build OK; 465 vitest; dist rebuilt + committed (LF); go.mod
unchanged; no new env vars. CI org-billing still blocked → local battery +
arc-authority merge.

LESSON (process): the shell's cwd persisted into a deleted smoke dir, so
`go test ./...` silently matched no packages while exiting 0 through the
grep pipeline — re-ran the whole gate from the repo root. Always anchor
gate commands with an absolute `cd`.

## Next

Gap #3 governed self-install (runtime MCP/CLI install, Edict-gated), then
#5 vector memory, #6 brain distiller.
