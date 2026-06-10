# Phase M792 — per-agent workdir: every identity gets a home directory

**Date:** 2026-06-10 · **Status:** DONE · **Arc:** multi-agent identity,
step 10 (the last stored-but-unwired profile field goes live).

## What

Profile `Workdir` (validated since M783) now applies: runs AS the agent —
run-as, chat, delegate, standing firings — do their relative file work and
shell commands inside `<workspace>/<workdir>`.

## Design

- `kernel/agent/toolctx.go`: `WithWorkdir`/`WorkdirFromContext` — the same
  leaf-ctx pattern as the run correlation; the setter REJECTS abs/`..`
  shapes (defense in depth over the profile validation).
- file tool: one touch point in Invoke — relative input paths rebase under
  the ctx workdir; empty list/glob path = the agent's directory; absolute
  paths and ALL existing containment (symlink-resolved root prefix, M252/
  M253 ancestor checks) untouched.
- shell tool: effective WorkDir = configured workspace + ctx workdir
  (lazily MkdirAll); ignored when no workspace anchor is configured.
- Wiring: `WithAgentProfile` (standing runner), handleRun inline, delegate
  childCtx — every identity path now sets it.

## Tests (3 new)

- file: scoped write/read/list land under the workdir; unscoped run still
  sees the root (no cross-identity leak); `../..` out of the workspace
  refused through the rebase.
- setter escape table (abs, .., ../up, a/../../b, a/..) + clean passthrough.
- runtime e2e on a real kernel: a run AS a workdir-bearing profile wrote via
  the real file tool → the file landed in `<workspace>/research/notes.txt`.

## Smoke

Isolated daemon: `agt agent add --workdir research` → `agent show` printed
it → `run --agent` clean; 0 panics. (Tool-driven write proven by the runtime
e2e — demo echo calls no tools.)

## Gate

Full suite + vet + staticcheck green; linux cross-build OK; go.mod
unchanged; no frontend change (Roster view already renders workdir). CI
org-billing still blocked → local battery + arc-authority merge.

## Arc remaining

per-agent daily budget ledger (the last gap-analysis item in this arc).
