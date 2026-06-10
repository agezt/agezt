# Phase M797 — MCP console view

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** MCP self-install (M796
backend → this is the console surface). Frontend-only (+dist); the API
routes shipped with M796.

## What

`frontend/src/views/Mcp.tsx` (+ "MCP Servers" nav under Agents, Plug icon):
the MCP self-install lifecycle from the web UI — register a stdio server
(name with the no-underscore rule explained inline, command, space-
separated args, description), **attach** behind a confirm dialog that
states what attaching means (process spawned now, scrubbed env, tools live
as `mcp_<name>_<tool>`), success toast reports the discovered tool count,
**detach** (kill switch, confirm), **auto-attach toggle** (Power icon),
**remove** (detaches first, confirm). Rows show attached·N-tools / registered
badges, the full command line, and the description.

## Exports for tests/reuse (M714 recipe)

`NewServerForm`, pure `serverNameOk` (kernel rule mirrored — underscores/
dashes rejected, the toolmap-parse invariant), pure `splitArgs`.

## Tests

8 new vitest (473 total green): name-rule + splitArgs tables; register form
gates on name+command and posts the wire shape (args as a list); Attach
shown only for detached rows / Detach only for live ones; attach confirm →
POST + discovered-tool-count toast; auto-attach toggle posts the flipped
flag; empty state.

## Browser e2e (isolated AGEZT_HOME, real daemon, real python MCP server)

Via the UI alone: registered `fake` (python server.py) → row
"registered · auto-attach" → Attach + confirm → **real spawn**, row
"attached · 2 tools" → Detach + confirm → "registered". Journal:
mcp.added → mcp.attached → mcp.detached. **0 console errors.** Restart
positive control: boot auto-attach journaled mcp.attached after halt and
the banner read "1 attached of 1 registered".

## Anomaly note (honest reporting)

The FIRST browser session's journal showed one extra `mcp.attached` after
the UI detach, with no halt before it. A full re-run of the identical flow
with a live journal watch was clean (no phantom event, including 20s of
page polling), and the runtime test suite covers double-attach refusal +
detach semantics. The extra event's signature matches a manual attach; the
headed Playwright browser was visible on the owner's desktop during the
gap, so a stray human click is the most plausible source. Not a code path
we could implicate; documented here rather than silently dropped.

## Gate

473 vitest green; dist rebuilt + embedded (go build + webui tests green);
Go code untouched this milestone; go.mod unchanged; no new env vars.
CI org-billing still blocked → local battery + arc-authority merge.

## Next

Gap #5 vector memory, then #6 brain distiller standing surface.
