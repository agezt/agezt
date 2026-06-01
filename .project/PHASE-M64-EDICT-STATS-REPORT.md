# Phase Report — Milestone M64 (`agt edict stats` — policy-decision aggregate)

> Status: **shipped** · Date: 2026-06-01 · Edict observability.

## Why

M63's `agt edict log` shows individual policy decisions; M64 aggregates them into
a security dashboard: how many tool calls were allowed vs denied, the denial rate,
and which capabilities get denied most. The autonomy/runs analogue is `agt runs
stats`; M64 brings the same to policy.

## What shipped

- **`CmdEdictStats` + `handleEdictStats` (`kernel/controlplane/policy_log.go`)** —
  folds `policy.decision` events into total / allowed / denied / hard_denied,
  `denial_rate`, and a `denied_by_capability` breakdown. Optional `since_ms`
  windows by decision time; tenant-scoped via `kernelFor`.
- **`agt edict stats [--since <dur>] [--tenant <id>] [--json]`** — renders the
  counts, denial rate, and a sorted denied-by-capability list.
- **Tenant-allowlisted** — joins `tenantTokenAllows`.

## Design decisions

- **show / log / stats triad.** `edict show` = rules, `edict log` = decisions,
  `edict stats` = the decisions aggregated. Three complementary read-only views.
- **Journal fold, reused renderers.** Same pattern as `agt runs stats` (counts +
  rate + by-key breakdown, windowed), no new state.

## Tests

- `TestEdictStats_Aggregates` — 1 allow + 3 denials (1 hard) across net/fs →
  total 4, allowed 1, denied 3, hard 1, denial_rate 0.75, denied_by_capability
  {net:2, fs:1}.

Test count: **1302 → 1303**. `go vet` clean, `GOOS=linux` builds, `go.mod`
unchanged, gofmt-clean (new `policy_log.go` normalized to LF).

## Live proof

```
$ agt edict stats
policy decisions (over 1):
  allowed   : 1
  denied    : 0 (hard 0)
  denial    : 0.0%
```
