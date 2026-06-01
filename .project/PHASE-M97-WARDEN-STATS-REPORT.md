# Phase Report — Milestone M97 (`agt warden stats`)

> Status: **shipped** · Date: 2026-06-01 · SPEC-08 / security observability.

## Why

M96 added `agt warden log` (per-execution sandbox audit). The aggregate was
missing: "what's my sandbox posture — how often does the warden have to downgrade
isolation, and is anything hitting limits?". M97 adds `agt warden stats`,
completing the warden `log`/`stats` pair and the security-triad's stats trio
(`edict stats`, `approvals stats`, `warden stats`).

## What shipped

- **Server `handleWardenStats`** — folds `warden.executed` + `warden.limit_exceeded`
  into total executions, downgraded count + rate, timed-out count, limit-breach
  count, and a by-effective-profile breakdown. `since_ms` windows by event time.
- **CLI `agt warden stats [--since <dur>] [--json]`** — renders the posture
  summary + a name-sorted by-profile table.

## Design decisions

- **Downgrade rate is the headline.** A high downgrade rate means the requested
  isolation isn't available on the host (e.g. no user namespaces) — a sandbox
  silently running weaker than intended is exactly what an operator must notice.
- **by-profile shows the truth.** The effective-profile breakdown reveals what
  isolation actually ran, not what was requested.

## Tests

- `TestWardenStats_Aggregates` — 3 execs (1 namespace, 2 none/downgraded) + 1
  limit breach → executions 3, downgraded 2, rate ≈ 0.667, limit_breaches 1,
  by_profile[none] = 2.

Test count: **1343 → 1344**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean.

## Live proof

```
$ agt run "summarize"      # the mock's shell call runs under the warden
$ agt warden stats
  sandbox executions (over 1):
    downgraded    : 1
    downgrade     : 100.0%
    timed out     : 0
    limit breaches: 0
    by effective profile:
      none         1
```
