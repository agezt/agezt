# Phase Report — Milestone M96 (`agt warden log`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / security observability.

## Why

Agezt's security model has three pillars: **Edict** (capability policy), **HITL
approvals**, and the **Warden** (the OS sandbox that runs shell/process tool
calls under a resource+isolation profile). M63/M87 surfaced the first two
(`agt edict log`, `agt approvals log`); the Warden — which journals every
execution, every profile downgrade (requested isolation unavailable on this
host), and every limit breach — had no surface at all. An operator couldn't see
what the sandbox actually did. M96 adds `agt warden log`, completing the
security-observability triad.

## What shipped

- **`agt warden` command + `handleWardenLog`** — folds `warden.executed` (argv0,
  effective profile, exit code, duration, downgraded/timed-out flags),
  `warden.profile_downgraded` (requested → effective + reason), and
  `warden.limit_exceeded` (which limit, on what) into one newest-first timeline,
  with `--issues` (only downgrades + limit breaches) and the shared `--since`
  window.
- **CLI rendering** — `exec <argv0> profile=<p> exit=<n> <dur> [downgraded]
  [TIMED OUT]`, `DOWNGRADE <req> → <eff> (reason)`, `LIMIT <argv0> <limit>
  exceeded`.

## Design decisions

- **One timeline, three event kinds.** Executions, downgrades, and limit breaches
  interleave as they happened; `--issues` isolates the security-relevant ones
  when execution volume is noise.
- **Completes the triad.** Edict (was it allowed?), Approvals (did a human ok
  it?), Warden (how was it sandboxed?) — three `*_log` surfaces, same shape.

## Tests

- `TestWardenLog_FoldsExecAndIssues` — an exec + a downgrade + a limit: all three
  newest-first; `--issues` drops the plain exec, keeping downgrade + limit.

Test count: **1342 → 1343**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run "summarize"               # the mock's shell call runs under the warden
$ agt warden log
  2026-06-01 18:49:03  exec       cmd              profile=none       exit=0  12ms  [downgraded]
  2026-06-01 18:49:03  DOWNGRADE  namespace → none  (linux full-namespace backend … not built; …)
```
